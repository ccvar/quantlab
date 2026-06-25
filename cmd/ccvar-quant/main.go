package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"ccvar.com/web3quant/internal/ai"
	"ccvar.com/web3quant/internal/aiprovider"
	"ccvar.com/web3quant/internal/audit"
	"ccvar.com/web3quant/internal/autopilot"
	"ccvar.com/web3quant/internal/backtest"
	"ccvar.com/web3quant/internal/core"
	"ccvar.com/web3quant/internal/exchange"
	"ccvar.com/web3quant/internal/exchange/binance"
	"ccvar.com/web3quant/internal/exchange/okx"
	"ccvar.com/web3quant/internal/killswitch"
	"ccvar.com/web3quant/internal/liveexec"
	"ccvar.com/web3quant/internal/liveguard"
	"ccvar.com/web3quant/internal/livereconcile"
	"ccvar.com/web3quant/internal/livesync"
	"ccvar.com/web3quant/internal/market"
	"ccvar.com/web3quant/internal/netclient"
	"ccvar.com/web3quant/internal/paperaccount"
	"ccvar.com/web3quant/internal/risk"
	"ccvar.com/web3quant/internal/simrun"
	"ccvar.com/web3quant/internal/storage"
	"ccvar.com/web3quant/internal/vault"
)

func main() {
	config, err := loadAppConfig(os.Args[1:])
	if err != nil {
		log.Fatal(err)
	}
	if config.ShowVersion {
		fmt.Println(serviceVersion)
		return
	}

	listener, err := net.Listen("tcp", config.Addr)
	if err != nil {
		if config.OpenBrowser && openExistingClient(config) {
			return
		}
		log.Fatalf("listen %s: %v", config.Addr, err)
	}
	defer listener.Close()

	startedAt := time.Now().UTC()
	store, err := storage.Open(config.DBPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer store.Close()
	marketHTTPClient := netclient.New(7 * time.Second)
	privateHTTPClient := netclient.New(10 * time.Second)
	log.Printf("Exchange HTTP proxy: %s", netclient.ProxySummary("https://www.okx.com/api/v5/public/time"))
	binanceAdapter := binance.New()
	binanceAdapter.Client = marketHTTPClient
	okxAdapter := okx.New()
	okxAdapter.Client = marketHTTPClient
	registry := exchange.NewRegistry(
		binanceAdapter,
		okxAdapter,
	)
	marketService := market.Service{
		Store:    store,
		Registry: registry,
	}
	riskProvider := func(ctx context.Context) (risk.Limits, error) {
		profile, err := store.RiskProfile(ctx)
		if err != nil {
			return risk.Limits{}, err
		}
		return riskLimitsFromProfile(profile), nil
	}
	strategyProvider := func(ctx context.Context) (simrun.StrategyConfig, error) {
		profile, err := store.StrategyProfile(ctx)
		if err != nil {
			return simrun.StrategyConfig{}, err
		}
		return strategyConfigFromProfile(profile), nil
	}
	marketService.AI = ai.NewLocalPolicy()
	marketService.StrategyProvider = func(ctx context.Context) (ai.Strategy, error) {
		config, err := strategyProvider(ctx)
		if err != nil {
			return ai.Strategy{}, err
		}
		return ai.Strategy{
			Name:          config.Name,
			Side:          config.Side,
			OrderSizeUSDT: config.OrderSizeUSDT,
		}, nil
	}
	simRunner := simrun.New(registry)
	simRunner.RiskProvider = riskProvider
	simRunner.StrategyProvider = strategyProvider
	simRunner.AI = ai.NewLocalPolicy()
	guard := liveguard.New()
	killSwitch := killswitch.New()
	privateMocks := config.PrivateExchangeMocks
	if privateMocks.Enabled {
		log.Printf("Loopback private exchange mocks enabled: binance=%t okx=%t", privateMocks.BinanceBaseURL != "", privateMocks.OKXBaseURL != "")
	}
	liveExecutor := liveexec.New(store, registry, guard, map[string]liveexec.Executor{
		"Binance": liveexec.BinanceExecutor{BaseURL: privateMocks.BinanceBaseURL, Client: privateHTTPClient},
		"OKX":     liveexec.OKXExecutor{BaseURL: privateMocks.OKXBaseURL, Client: privateHTTPClient},
	})
	liveExecutor.Halted = killSwitch.Active
	liveExecutor.RiskProvider = riskProvider
	accountSync := livesync.New(store, map[string]livesync.Client{
		"Binance": livesync.BinanceClient{BaseURL: privateMocks.BinanceBaseURL, Client: privateHTTPClient},
		"OKX":     livesync.OKXClient{BaseURL: privateMocks.OKXBaseURL, Client: privateHTTPClient},
	})
	liveReconcile := livereconcile.New(store, map[string]livereconcile.Client{
		"Binance": livereconcile.BinanceClient{BaseURL: privateMocks.BinanceBaseURL, Client: privateHTTPClient},
		"OKX":     livereconcile.OKXClient{BaseURL: privateMocks.OKXBaseURL, Client: privateHTTPClient},
	})
	autoPilot := autopilot.New(
		store,
		simRunner.Step,
		func(ctx context.Context, request livesync.Request) (livesync.Result, error) {
			if strings.TrimSpace(request.Environment) == "" {
				request.Environment = guard.State().Environment
			}
			return accountSync.Sync(ctx, request)
		},
		liveExecutor.Execute,
		killSwitch.Active,
	)
	autoPilot.WithLivePlanner(func(ctx context.Context, request autopilot.LivePlanRequest) (autopilot.LivePlan, error) {
		return buildLiveAutopilotPlan(ctx, registry, store, request)
	})

	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", withCORS(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"ok":        true,
			"service":   "ccvar-quant",
			"version":   serviceVersion,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	}))
	mux.HandleFunc("/api/app-info", withCORS(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeErrorJSON(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}
		writeJSON(w, buildAppInfo(config, startedAt))
	}))
	mux.HandleFunc("/api/preflight", withCORS(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeErrorJSON(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), preflightReportTimeout)
		defer cancel()
		report, err := buildPreflightReport(ctx, preflightDeps{
			Config:          config,
			Store:           store,
			Registry:        registry,
			GuardState:      guard.State(),
			KillSwitchState: killSwitch.State(),
			AutopilotState:  autoPilot.State(),
		})
		if err != nil {
			writeErrorJSON(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, report)
	}))
	mux.HandleFunc("/api/ai-providers", withCORS(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeErrorJSON(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
		defer cancel()
		writeJSON(w, aiprovider.Detect(ctx))
	}))
	mux.HandleFunc("/api/kill-switch", withCORS(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, killSwitch.State())
		case http.MethodPost:
			var request killSwitchRequest
			reader := http.MaxBytesReader(w, r.Body, 1<<20)
			decoder := json.NewDecoder(reader)
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&request); err != nil {
				writeErrorJSON(w, http.StatusBadRequest, err)
				return
			}
			switch strings.ToLower(strings.TrimSpace(request.Action)) {
			case "activate":
				state := killSwitch.Activate(killswitch.Request{
					Operator: request.Operator,
					Reason:   request.Reason,
				})
				guardState := guard.Lock()
				if autoPilot.State().Running {
					autoPilot.Stop(r.Context(), request.Operator, "kill switch activated")
				}
				if _, err := store.AppendAudit(r.Context(), audit.Record{
					Actor:   defaultString(state.Operator, "local"),
					Action:  "kill_switch.activate",
					Entity:  "kill_switch",
					Status:  "approved",
					Summary: state.Message,
					Payload: map[string]any{
						"reason":          state.Reason,
						"activatedAt":     state.ActivatedAt,
						"liveGuardLocked": !guardState.Unlocked,
					},
				}); err != nil {
					writeErrorJSON(w, http.StatusInternalServerError, err)
					return
				}
				writeJSON(w, state)
			case "resume":
				state := killSwitch.Resume(killswitch.Request{
					Operator: request.Operator,
					Reason:   request.Reason,
				})
				if _, err := store.AppendAudit(r.Context(), audit.Record{
					Actor:   defaultString(state.Operator, "local"),
					Action:  "kill_switch.resume",
					Entity:  "kill_switch",
					Status:  "approved",
					Summary: state.Message,
					Payload: map[string]any{
						"reason":    state.Reason,
						"resumedAt": state.ResumedAt,
					},
				}); err != nil {
					writeErrorJSON(w, http.StatusInternalServerError, err)
					return
				}
				writeJSON(w, state)
			default:
				writeErrorJSON(w, http.StatusBadRequest, errors.New("action must be activate or resume"))
			}
		default:
			writeErrorJSON(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		}
	}))
	mux.HandleFunc("/api/lab-state", withCORS(func(w http.ResponseWriter, r *http.Request) {
		state, err := marketService.LabState(
			r.Context(),
			r.URL.Query().Get("exchange"),
			r.URL.Query().Get("symbol"),
		)
		if err != nil {
			writeErrorJSON(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, state)
	}))
	mux.HandleFunc("/api/simulate/step", withCORS(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeErrorJSON(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}
		if killSwitch.Active() {
			exchangeName := defaultString(r.URL.Query().Get("exchange"), "Binance")
			symbol := defaultString(r.URL.Query().Get("symbol"), "BTCUSDT")
			_, _ = store.AppendAudit(r.Context(), audit.Record{
				Actor:   "local",
				Action:  "simulate_step.kill_switch",
				Entity:  "kill_switch",
				Status:  "rejected",
				Summary: liveexec.ErrKillSwitchActive.Error(),
				Payload: map[string]any{
					"exchange": exchangeName,
					"symbol":   symbol,
				},
			})
			writeErrorJSON(w, http.StatusLocked, liveexec.ErrKillSwitchActive)
			return
		}
		exchangeName := r.URL.Query().Get("exchange")
		symbol := r.URL.Query().Get("symbol")
		mode := simulationMode(r.URL.Query().Get("mode"))
		result, err := simRunner.Step(r.Context(), exchangeName, symbol)
		if err != nil {
			writeErrorJSON(w, http.StatusBadGateway, err)
			return
		}
		record, err := simrun.PaperExecutionRecordFromResult(result, mode, "manual", 0, time.Now())
		if err != nil {
			writeErrorJSON(w, http.StatusInternalServerError, err)
			return
		}
		if _, err := store.SavePaperExecution(r.Context(), record); err != nil {
			writeErrorJSON(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, result)
	}))
	mux.HandleFunc("/api/paper-executions", withCORS(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeErrorJSON(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		records, err := store.ListPaperExecutions(r.Context(), limit)
		if err != nil {
			writeErrorJSON(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, map[string]any{"records": records})
	}))
	mux.HandleFunc("/api/paper-executions/reset", withCORS(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeErrorJSON(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}
		var request paperResetRequest
		reader := http.MaxBytesReader(w, r.Body, 1<<20)
		decoder := json.NewDecoder(reader)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			writeErrorJSON(w, http.StatusBadRequest, err)
			return
		}
		operator := defaultString(request.Operator, "local")
		reason := defaultString(request.Reason, "operator reset")
		if autoPilot.State().Running {
			err := errors.New("stop autopilot before resetting paper ledger")
			_, _ = store.AppendAudit(r.Context(), audit.Record{
				Actor:   operator,
				Action:  "paper.reset",
				Entity:  "paper_execution_records",
				Status:  "rejected",
				Summary: err.Error(),
				Payload: map[string]any{"reason": reason},
			})
			writeErrorJSON(w, http.StatusConflict, err)
			return
		}
		if strings.TrimSpace(request.Phrase) != paperResetPhrase {
			err := errors.New("confirmation phrase must be RESET PAPER")
			_, _ = store.AppendAudit(r.Context(), audit.Record{
				Actor:   operator,
				Action:  "paper.reset",
				Entity:  "paper_execution_records",
				Status:  "rejected",
				Summary: err.Error(),
				Payload: map[string]any{"reason": reason},
			})
			writeErrorJSON(w, http.StatusBadRequest, err)
			return
		}
		deleted, entry, err := store.ResetPaperExecutions(r.Context(), audit.Record{
			Actor:   operator,
			Action:  "paper.reset",
			Entity:  "paper_execution_records",
			Status:  "approved",
			Summary: "paper simulation ledger reset",
			Payload: map[string]any{"reason": reason},
		})
		if err != nil {
			writeErrorJSON(w, http.StatusInternalServerError, err)
			return
		}
		snapshot, err := buildPaperAccountSnapshot(r.Context(), store)
		if err != nil {
			writeErrorJSON(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, map[string]any{
			"deletedRecords": deleted,
			"auditId":        entry.ID,
			"account":        snapshot,
		})
	}))
	mux.HandleFunc("/api/paper-account", withCORS(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeErrorJSON(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}
		snapshot, err := buildPaperAccountSnapshot(r.Context(), store)
		if err != nil {
			writeErrorJSON(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, snapshot)
	}))
	mux.HandleFunc("/api/credentials", withCORS(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			credentials, err := store.ListCredentials(r.Context())
			if err != nil {
				writeErrorJSON(w, http.StatusInternalServerError, err)
				return
			}
			writeJSON(w, map[string]any{"credentials": credentials})
		case http.MethodPost:
			var request vault.CredentialInput
			reader := http.MaxBytesReader(w, r.Body, 1<<20)
			decoder := json.NewDecoder(reader)
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&request); err != nil {
				writeErrorJSON(w, http.StatusBadRequest, err)
				return
			}
			exchangeName, ok := supportedExchangeName(request.Exchange)
			if !ok {
				writeErrorJSON(w, http.StatusBadRequest, errors.New("unsupported exchange"))
				return
			}
			request.Exchange = exchangeName
			encrypted, err := vault.EncryptCredential(request, time.Now())
			if err != nil {
				writeErrorJSON(w, http.StatusBadRequest, err)
				return
			}
			meta, err := store.SaveCredential(r.Context(), encrypted)
			if err != nil {
				writeErrorJSON(w, http.StatusInternalServerError, err)
				return
			}
			writeJSON(w, meta)
		case http.MethodDelete:
			id, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
			if err != nil || id <= 0 {
				writeErrorJSON(w, http.StatusBadRequest, errors.New("valid id is required"))
				return
			}
			if err := store.DeleteCredential(r.Context(), id); err != nil {
				writeErrorJSON(w, http.StatusInternalServerError, err)
				return
			}
			writeJSON(w, map[string]any{"deleted": id})
		default:
			writeErrorJSON(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		}
	}))
	mux.HandleFunc("/api/live-guard", withCORS(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, guard.State())
		case http.MethodPost:
			var request liveGuardRequest
			reader := http.MaxBytesReader(w, r.Body, 1<<20)
			decoder := json.NewDecoder(reader)
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&request); err != nil {
				writeErrorJSON(w, http.StatusBadRequest, err)
				return
			}
			switch strings.ToLower(strings.TrimSpace(request.Action)) {
			case "unlock":
				state, err := guard.Unlock(liveguard.UnlockRequest{
					Operator:     request.Operator,
					Environment:  request.Environment,
					Phrase:       request.Phrase,
					TTLSeconds:   request.TTLSeconds,
					MaxOrderUSDT: request.MaxOrderUSDT,
					Reason:       request.Reason,
				})
				recordStatus := "approved"
				statusCode := http.StatusOK
				if err != nil {
					recordStatus = "rejected"
					statusCode = http.StatusBadRequest
				}
				_, auditErr := store.AppendAudit(r.Context(), audit.Record{
					Actor:    defaultString(request.Operator, "local"),
					Action:   "live_guard.unlock",
					Entity:   "live_guard",
					EntityID: state.SessionID,
					Status:   recordStatus,
					Summary:  liveGuardSummary(err, state),
					Payload: map[string]any{
						"environment":  request.Environment,
						"ttlSeconds":   request.TTLSeconds,
						"maxOrderUsdt": request.MaxOrderUSDT,
						"error":        errorString(err),
					},
				})
				if auditErr != nil {
					writeErrorJSON(w, http.StatusInternalServerError, auditErr)
					return
				}
				if err != nil {
					writeErrorJSON(w, statusCode, err)
					return
				}
				writeJSON(w, state)
			case "lock":
				state := guard.Lock()
				if _, err := store.AppendAudit(r.Context(), audit.Record{
					Actor:   defaultString(request.Operator, "local"),
					Action:  "live_guard.lock",
					Entity:  "live_guard",
					Status:  "approved",
					Summary: "manual live guard lock",
					Payload: map[string]any{"reason": request.Reason},
				}); err != nil {
					writeErrorJSON(w, http.StatusInternalServerError, err)
					return
				}
				writeJSON(w, state)
			default:
				writeErrorJSON(w, http.StatusBadRequest, errors.New("action must be unlock or lock"))
			}
		default:
			writeErrorJSON(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		}
	}))
	mux.HandleFunc("/api/risk-profile", withCORS(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			profile, err := store.RiskProfile(r.Context())
			if err != nil {
				writeErrorJSON(w, http.StatusInternalServerError, err)
				return
			}
			writeJSON(w, profile)
		case http.MethodPut:
			var request storage.RiskProfileRecord
			reader := http.MaxBytesReader(w, r.Body, 1<<20)
			decoder := json.NewDecoder(reader)
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&request); err != nil {
				writeErrorJSON(w, http.StatusBadRequest, err)
				return
			}
			profile, err := store.SaveRiskProfile(r.Context(), request)
			if err != nil {
				writeErrorJSON(w, http.StatusInternalServerError, err)
				return
			}
			if _, err := store.AppendAudit(r.Context(), audit.Record{
				Actor:   "local",
				Action:  "risk_profile.update",
				Entity:  "risk_profile",
				Status:  "approved",
				Summary: "risk profile updated",
				Payload: profile,
			}); err != nil {
				writeErrorJSON(w, http.StatusInternalServerError, err)
				return
			}
			writeJSON(w, profile)
		default:
			writeErrorJSON(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		}
	}))
	mux.HandleFunc("/api/strategy-profile", withCORS(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			profile, err := store.StrategyProfile(r.Context())
			if err != nil {
				writeErrorJSON(w, http.StatusInternalServerError, err)
				return
			}
			writeJSON(w, profile)
		case http.MethodPut:
			var request storage.StrategyProfileRecord
			reader := http.MaxBytesReader(w, r.Body, 1<<20)
			decoder := json.NewDecoder(reader)
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&request); err != nil {
				writeErrorJSON(w, http.StatusBadRequest, err)
				return
			}
			if strings.TrimSpace(request.Exchange) != "" {
				exchangeName, ok := supportedExchangeName(request.Exchange)
				if !ok {
					writeErrorJSON(w, http.StatusBadRequest, errors.New("unsupported exchange"))
					return
				}
				request.Exchange = exchangeName
			}
			profile, err := store.SaveStrategyProfile(r.Context(), request)
			if err != nil {
				writeErrorJSON(w, http.StatusInternalServerError, err)
				return
			}
			if _, err := store.AppendAudit(r.Context(), audit.Record{
				Actor:   "local",
				Action:  "strategy_profile.update",
				Entity:  "strategy_profile",
				Status:  "approved",
				Summary: "strategy profile updated",
				Payload: profile,
			}); err != nil {
				writeErrorJSON(w, http.StatusInternalServerError, err)
				return
			}
			writeJSON(w, profile)
		default:
			writeErrorJSON(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		}
	}))
	mux.HandleFunc("/api/backtest/run", withCORS(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeErrorJSON(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}
		var request backtestRequest
		reader := http.MaxBytesReader(w, r.Body, 1<<20)
		decoder := json.NewDecoder(reader)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			writeErrorJSON(w, http.StatusBadRequest, err)
			return
		}
		profile, err := store.StrategyProfile(r.Context())
		if err != nil {
			writeErrorJSON(w, http.StatusInternalServerError, err)
			return
		}
		config := backtestConfigFromProfile(profile, request)
		exchangeName := config.Exchange
		if strings.TrimSpace(request.Exchange) != "" {
			var ok bool
			exchangeName, ok = supportedExchangeName(request.Exchange)
			if !ok {
				writeErrorJSON(w, http.StatusBadRequest, errors.New("unsupported exchange"))
				return
			}
			config.Exchange = exchangeName
		}
		adapter, ok := registry.Get(exchangeName)
		if !ok {
			writeErrorJSON(w, http.StatusBadRequest, errors.New("unsupported exchange"))
			return
		}
		limit := request.Limit
		if limit <= 0 {
			limit = 200
		}
		if limit > 1000 {
			limit = 1000
		}
		marketDataSource := "live public"
		warning := ""
		candles, err := adapter.FetchCandles(r.Context(), config.Symbol, config.Interval, limit)
		if err != nil {
			seedState, seedErr := store.LabState(r.Context())
			if seedErr != nil || len(seedState.Candles) == 0 {
				writeErrorJSON(w, http.StatusBadGateway, err)
				return
			}
			candles = seedState.Candles
			marketDataSource = "local seed"
			warning = err.Error()
		}
		result, err := backtest.Run(config, candles, time.Now())
		if err != nil {
			if errors.Is(err, backtest.ErrNotEnoughCandles) {
				writeErrorJSON(w, http.StatusBadRequest, err)
				return
			}
			writeErrorJSON(w, http.StatusInternalServerError, err)
			return
		}
		result.Summary.MarketDataSource = marketDataSource
		result.Summary.Warning = warning
		resultJSON, err := json.Marshal(result)
		if err != nil {
			writeErrorJSON(w, http.StatusInternalServerError, err)
			return
		}
		savedRun, err := store.SaveBacktestRun(r.Context(), storage.BacktestRunRecord{
			StrategyName:       result.Summary.StrategyName,
			Exchange:           result.Summary.Exchange,
			Symbol:             result.Summary.Symbol,
			Interval:           result.Summary.Interval,
			MarketDataSource:   result.Summary.MarketDataSource,
			CandleCount:        result.Summary.CandleCount,
			TradeCount:         result.Summary.TradeCount,
			EndingEquityUSDT:   result.Summary.EndingEquityUSDT,
			ReturnPct:          result.Summary.ReturnPct,
			BenchmarkReturnPct: result.Summary.BenchmarkReturnPct,
			MaxDrawdownPct:     result.Summary.MaxDrawdownPct,
			FeesUSDT:           result.Summary.FeesUSDT,
			ResultJSON:         resultJSON,
			CreatedAt:          result.Summary.GeneratedAt,
		})
		if err != nil {
			writeErrorJSON(w, http.StatusInternalServerError, err)
			return
		}
		result.RunID = savedRun.ID
		writeJSON(w, result)
	}))
	mux.HandleFunc("/api/backtest-runs", withCORS(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeErrorJSON(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		records, err := store.ListBacktestRuns(r.Context(), limit)
		if err != nil {
			writeErrorJSON(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, map[string]any{"records": records})
	}))
	mux.HandleFunc("/api/live-execute", withCORS(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeErrorJSON(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}
		var request liveexec.Request
		reader := http.MaxBytesReader(w, r.Body, 1<<20)
		decoder := json.NewDecoder(reader)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			writeErrorJSON(w, http.StatusBadRequest, err)
			return
		}
		if strings.TrimSpace(request.Exchange) != "" {
			exchangeName, ok := supportedExchangeName(request.Exchange)
			if !ok {
				writeErrorJSON(w, http.StatusBadRequest, errors.New("unsupported exchange"))
				return
			}
			request.Exchange = exchangeName
		}
		result, err := liveExecutor.Execute(r.Context(), request)
		if err != nil {
			writeErrorJSON(w, liveExecuteStatus(err), err)
			return
		}
		writeJSON(w, result)
	}))
	mux.HandleFunc("/api/live-executions", withCORS(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeErrorJSON(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		records, err := store.ListLiveExecutions(r.Context(), limit)
		if err != nil {
			writeErrorJSON(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, map[string]any{"records": records})
	}))
	mux.HandleFunc("/api/autopilot", withCORS(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, autoPilot.State())
		case http.MethodPost:
			var request autopilot.Request
			reader := http.MaxBytesReader(w, r.Body, 1<<20)
			decoder := json.NewDecoder(reader)
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&request); err != nil {
				writeErrorJSON(w, http.StatusBadRequest, err)
				return
			}
			switch strings.ToLower(strings.TrimSpace(request.Action)) {
			case "start":
				profile, err := store.StrategyProfile(r.Context())
				if err != nil {
					writeErrorJSON(w, http.StatusInternalServerError, err)
					return
				}
				request = applyStrategyDefaults(request, profile)
				if strings.EqualFold(request.Mode, "live") && strings.TrimSpace(request.Environment) == "" {
					request.Environment = guard.State().Environment
				}
				state, err := autoPilot.Start(r.Context(), request)
				if err != nil {
					writeErrorJSON(w, autopilotStatus(err), err)
					return
				}
				writeJSON(w, state)
			case "stop":
				writeJSON(w, autoPilot.Stop(r.Context(), request.Operator, request.Reason))
			case "step":
				state, err := autoPilot.Step(r.Context())
				if err != nil {
					writeErrorJSON(w, autopilotStatus(err), err)
					return
				}
				writeJSON(w, state)
			default:
				writeErrorJSON(w, http.StatusBadRequest, errors.New("action must be start, stop, or step"))
			}
		default:
			writeErrorJSON(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		}
	}))
	mux.HandleFunc("/api/autopilot-runs", withCORS(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeErrorJSON(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		records, err := autoPilot.History(r.Context(), limit)
		if err != nil {
			writeErrorJSON(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, map[string]any{"records": records})
	}))
	mux.HandleFunc("/api/autopilot-steps", withCORS(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeErrorJSON(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		runID, _ := strconv.ParseInt(r.URL.Query().Get("runId"), 10, 64)
		records, err := autoPilot.Steps(r.Context(), runID, limit)
		if err != nil {
			writeErrorJSON(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, map[string]any{"records": records})
	}))
	mux.HandleFunc("/api/live-reconcile", withCORS(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeErrorJSON(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}
		var request livereconcile.Request
		reader := http.MaxBytesReader(w, r.Body, 1<<20)
		decoder := json.NewDecoder(reader)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			writeErrorJSON(w, http.StatusBadRequest, err)
			return
		}
		result, err := liveReconcile.Reconcile(r.Context(), request)
		if err != nil {
			writeErrorJSON(w, liveReconcileStatus(err), err)
			return
		}
		writeJSON(w, result)
	}))
	mux.HandleFunc("/api/live-reconciliations", withCORS(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeErrorJSON(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		liveExecutionID, _ := strconv.ParseInt(r.URL.Query().Get("liveExecutionId"), 10, 64)
		records, err := liveReconcile.Latest(r.Context(), liveExecutionID, limit)
		if err != nil {
			writeErrorJSON(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, map[string]any{"records": records})
	}))
	mux.HandleFunc("/api/account-sync", withCORS(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			credentialID, _ := strconv.ParseInt(r.URL.Query().Get("credentialId"), 10, 64)
			result, err := accountSync.Latest(r.Context(), livesync.LatestRequest{
				CredentialID: credentialID,
				Exchange:     r.URL.Query().Get("exchange"),
				Environment:  r.URL.Query().Get("environment"),
				Symbol:       r.URL.Query().Get("symbol"),
			})
			if err != nil {
				writeErrorJSON(w, http.StatusInternalServerError, err)
				return
			}
			writeJSON(w, result)
		case http.MethodPost:
			var request livesync.Request
			reader := http.MaxBytesReader(w, r.Body, 1<<20)
			decoder := json.NewDecoder(reader)
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&request); err != nil {
				writeErrorJSON(w, http.StatusBadRequest, err)
				return
			}
			if strings.TrimSpace(request.Exchange) != "" {
				exchangeName, ok := supportedExchangeName(request.Exchange)
				if !ok {
					writeErrorJSON(w, http.StatusBadRequest, errors.New("unsupported exchange"))
					return
				}
				request.Exchange = exchangeName
			}
			result, err := accountSync.Sync(r.Context(), request)
			if err != nil {
				writeErrorJSON(w, accountSyncStatus(err), err)
				return
			}
			writeJSON(w, result)
		default:
			writeErrorJSON(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		}
	}))
	mux.HandleFunc("/api/audit-log", withCORS(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeErrorJSON(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		entries, err := store.ListAudit(r.Context(), limit)
		if err != nil {
			writeErrorJSON(w, http.StatusInternalServerError, err)
			return
		}
		verification, err := store.VerifyAudit(r.Context())
		if err != nil {
			writeErrorJSON(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, map[string]any{
			"entries":      entries,
			"verification": verification,
		})
	}))
	mux.HandleFunc("/api/local-data", withCORS(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeErrorJSON(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}
		summary, err := store.LocalDataSummary(r.Context())
		if err != nil {
			writeErrorJSON(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, map[string]any{
			"summary":   summary,
			"keep":      localDataPruneDefaults(),
			"protected": localDataProtectedSets(),
		})
	}))
	mux.HandleFunc("/api/local-data/prune", withCORS(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeErrorJSON(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}
		var request localDataPruneRequest
		reader := http.MaxBytesReader(w, r.Body, 1<<20)
		decoder := json.NewDecoder(reader)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			writeErrorJSON(w, http.StatusBadRequest, err)
			return
		}
		operator := defaultString(request.Operator, "local")
		reason := defaultString(request.Reason, "operator retention prune")
		if autoPilot.State().Running {
			err := errors.New("stop autopilot before pruning local data")
			_, _ = store.AppendAudit(r.Context(), audit.Record{
				Actor:   operator,
				Action:  "local_data.prune",
				Entity:  "local_data",
				Status:  "rejected",
				Summary: err.Error(),
				Payload: map[string]any{"reason": reason},
			})
			writeErrorJSON(w, http.StatusConflict, err)
			return
		}
		if strings.TrimSpace(request.Phrase) != localDataPrunePhrase {
			err := errors.New("confirmation phrase must be PRUNE LOCAL DATA")
			_, _ = store.AppendAudit(r.Context(), audit.Record{
				Actor:   operator,
				Action:  "local_data.prune",
				Entity:  "local_data",
				Status:  "rejected",
				Summary: err.Error(),
				Payload: map[string]any{"reason": reason},
			})
			writeErrorJSON(w, http.StatusBadRequest, err)
			return
		}
		options := localDataPruneDefaults()
		if request.KeepBacktestRuns > 0 {
			options.KeepBacktestRuns = request.KeepBacktestRuns
		}
		if request.KeepAutopilotRuns > 0 {
			options.KeepAutopilotRuns = request.KeepAutopilotRuns
		}
		if request.KeepPaperExecutions > 0 {
			options.KeepPaperExecutions = request.KeepPaperExecutions
		}
		if request.KeepAccountSnapshots > 0 {
			options.KeepAccountSnapshots = request.KeepAccountSnapshots
		}
		report, _, err := store.PruneLocalData(r.Context(), options, audit.Record{
			Actor:   operator,
			Action:  "local_data.prune",
			Entity:  "local_data",
			Status:  "approved",
			Summary: "local research data pruned",
			Payload: map[string]any{"reason": reason},
		})
		if err != nil {
			writeErrorJSON(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, report)
	}))
	mux.HandleFunc("/api/export", withCORS(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeErrorJSON(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}
		payload, err := buildWorkspaceExport(r.Context(), store, autoPilot)
		if err != nil {
			writeErrorJSON(w, http.StatusInternalServerError, err)
			return
		}
		filename := "ccvar-quant-export-" + time.Now().UTC().Format("20060102T150405Z") + ".json"
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
		writeJSON(w, payload)
	}))
	if err := mountWeb(mux); err != nil {
		log.Fatalf("mount web: %v", err)
	}

	uiURL := config.UIURL()
	log.Printf("CCVar Quant API listening on http://%s", config.Addr)
	log.Printf("CCVar Quant UI available at %s", uiURL)
	if config.OpenBrowser {
		go func() {
			time.Sleep(300 * time.Millisecond)
			if err := openBrowser(uiURL); err != nil {
				log.Printf("open browser: %v", err)
			}
		}()
	}
	if err := http.Serve(listener, mux); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func withCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			if !isAllowedLocalOrigin(origin) {
				writeErrorJSON(w, http.StatusForbidden, errors.New("origin not allowed"))
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Expose-Headers", "Content-Disposition")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func writeErrorJSON(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{
		"error": err.Error(),
	})
}

type appConfig struct {
	Addr                 string
	DBPath               string
	OpenBrowser          bool
	ShowVersion          bool
	PrivateExchangeMocks privateExchangeMockConfig
}

type privateExchangeMockConfig struct {
	Enabled        bool
	BinanceBaseURL string
	OKXBaseURL     string
}

type appInfo struct {
	Service   string          `json:"service"`
	Version   string          `json:"version"`
	Address   string          `json:"address"`
	URL       string          `json:"url"`
	StartedAt string          `json:"startedAt"`
	Runtime   appRuntimeInfo  `json:"runtime"`
	Database  appDatabaseInfo `json:"database"`
	Docs      appDocsInfo     `json:"docs"`
	Security  appSecurityInfo `json:"security"`
	Exchanges []string        `json:"exchanges"`
}

type appRuntimeInfo struct {
	GOOS      string `json:"goos"`
	GOARCH    string `json:"goarch"`
	GoVersion string `json:"goVersion"`
}

type appDatabaseInfo struct {
	Path      string `json:"path"`
	Dir       string `json:"dir"`
	Exists    bool   `json:"exists"`
	SizeBytes int64  `json:"sizeBytes"`
}

type appDocsInfo struct {
	Available bool       `json:"available"`
	Runbook   appDocInfo `json:"runbook"`
	Safety    appDocInfo `json:"safety"`
}

type appDocInfo struct {
	Path      string `json:"path"`
	Exists    bool   `json:"exists"`
	SizeBytes int64  `json:"sizeBytes"`
}

type appSecurityInfo struct {
	LocalOriginOnly              bool     `json:"localOriginOnly"`
	ProductionTradingEnabled     bool     `json:"productionTradingEnabled"`
	ProductionAccountSyncEnabled bool     `json:"productionAccountSyncEnabled"`
	LoopbackExchangeMocksEnabled bool     `json:"loopbackExchangeMocksEnabled"`
	LiveEnvironments             []string `json:"liveEnvironments"`
}

type preflightDeps struct {
	Config          appConfig
	Store           *storage.Store
	Registry        exchange.Registry
	GuardState      liveguard.State
	KillSwitchState killswitch.State
	AutopilotState  autopilot.State
}

type preflightReport struct {
	GeneratedAt string           `json:"generatedAt"`
	Overall     string           `json:"overall"`
	Ready       int              `json:"ready"`
	Warn        int              `json:"warn"`
	Block       int              `json:"block"`
	Checks      []preflightCheck `json:"checks"`
}

type preflightCheck struct {
	ID      string         `json:"id"`
	Label   string         `json:"label"`
	Status  string         `json:"status"`
	Summary string         `json:"summary"`
	Details map[string]any `json:"details,omitempty"`
}

func loadAppConfig(args []string) (appConfig, error) {
	config := appConfig{
		Addr:        env("CCVAR_ADDR", "127.0.0.1:8787"),
		DBPath:      env("CCVAR_DB_PATH", filepath.Join("data", "ccvar_quant.db")),
		OpenBrowser: boolEnv("CCVAR_OPEN_BROWSER", false),
	}
	flags := flag.NewFlagSet("ccvar-quant", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&config.Addr, "addr", config.Addr, "HTTP listen address")
	flags.StringVar(&config.DBPath, "db", config.DBPath, "SQLite database path")
	flags.BoolVar(&config.OpenBrowser, "open", config.OpenBrowser, "open the local client in the default browser")
	flags.BoolVar(&config.ShowVersion, "version", false, "print version and exit")
	if err := flags.Parse(args); err != nil {
		return appConfig{}, err
	}
	config.Addr = defaultString(config.Addr, "127.0.0.1:8787")
	config.DBPath = defaultString(config.DBPath, filepath.Join("data", "ccvar_quant.db"))
	if config.ShowVersion {
		return config, nil
	}
	privateMocks, err := loadPrivateExchangeMockConfig()
	if err != nil {
		return appConfig{}, err
	}
	config.PrivateExchangeMocks = privateMocks
	return config, nil
}

func (config appConfig) UIURL() string {
	return "http://" + browserAddress(config.Addr)
}

func browserAddress(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	switch host {
	case "", "0.0.0.0", "::":
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}

func buildAppInfo(config appConfig, startedAt time.Time) appInfo {
	return appInfo{
		Service:   "ccvar-quant",
		Version:   serviceVersion,
		Address:   config.Addr,
		URL:       config.UIURL(),
		StartedAt: startedAt.Format(time.RFC3339),
		Runtime: appRuntimeInfo{
			GOOS:      runtime.GOOS,
			GOARCH:    runtime.GOARCH,
			GoVersion: runtime.Version(),
		},
		Database: databaseInfo(config.DBPath),
		Docs:     buildAppDocsInfo(),
		Security: appSecurityInfo{
			LocalOriginOnly:              true,
			ProductionTradingEnabled:     false,
			ProductionAccountSyncEnabled: false,
			LoopbackExchangeMocksEnabled: config.PrivateExchangeMocks.Enabled,
			LiveEnvironments:             []string{"testnet", "demo"},
		},
		Exchanges: []string{"Binance", "OKX"},
	}
}

func buildAppDocsInfo() appDocsInfo {
	executablePath, _ := os.Executable()
	workingDir, _ := os.Getwd()
	return buildAppDocsInfoFor(executablePath, workingDir)
}

func buildAppDocsInfoFor(executablePath, workingDir string) appDocsInfo {
	runbook := firstDocInfo(docCandidates(executablePath, workingDir, "operator-runbook.zh-CN.md"))
	safety := firstDocInfo(docCandidates(executablePath, workingDir, "safety.md"))
	return appDocsInfo{
		Available: runbook.Exists && safety.Exists,
		Runbook:   runbook,
		Safety:    safety,
	}
}

func docCandidates(executablePath, workingDir, filename string) []string {
	candidates := []string{}
	if strings.TrimSpace(executablePath) != "" {
		executableDir := filepath.Dir(executablePath)
		candidates = append(candidates,
			filepath.Join(executableDir, "docs", filename),
			filepath.Clean(filepath.Join(executableDir, "..", "Resources", "docs", filename)),
		)
	}
	if strings.TrimSpace(workingDir) != "" {
		candidates = append(candidates, filepath.Join(workingDir, "docs", filename))
	}
	return uniqueStrings(candidates)
}

func firstDocInfo(candidates []string) appDocInfo {
	if len(candidates) == 0 {
		return appDocInfo{}
	}
	for _, candidate := range candidates {
		info := docInfo(candidate)
		if info.Exists {
			return info
		}
	}
	return docInfo(candidates[0])
}

func docInfo(path string) appDocInfo {
	absolutePath := path
	if resolved, err := filepath.Abs(path); err == nil {
		absolutePath = resolved
	}
	info := appDocInfo{Path: absolutePath}
	if stat, err := os.Stat(path); err == nil && !stat.IsDir() {
		info.Exists = true
		info.SizeBytes = stat.Size()
	}
	return info
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	unique := []string{}
	for _, value := range values {
		if strings.TrimSpace(value) == "" || seen[value] {
			continue
		}
		seen[value] = true
		unique = append(unique, value)
	}
	return unique
}

func loadPrivateExchangeMockConfig() (privateExchangeMockConfig, error) {
	enabled := boolEnv(loopbackExchangeMocksEnv, false)
	config := privateExchangeMockConfig{Enabled: enabled}
	binanceURL := strings.TrimSpace(os.Getenv(binancePrivateMockURLEnv))
	okxURL := strings.TrimSpace(os.Getenv(okxPrivateMockURLEnv))
	if !enabled {
		if binanceURL != "" || okxURL != "" {
			return privateExchangeMockConfig{}, fmt.Errorf("%s must be true before private mock URLs are accepted", loopbackExchangeMocksEnv)
		}
		return config, nil
	}
	if binanceURL != "" {
		normalized, err := normalizeLoopbackBaseURL(binanceURL, binancePrivateMockURLEnv)
		if err != nil {
			return privateExchangeMockConfig{}, err
		}
		config.BinanceBaseURL = normalized
	}
	if okxURL != "" {
		normalized, err := normalizeLoopbackBaseURL(okxURL, okxPrivateMockURLEnv)
		if err != nil {
			return privateExchangeMockConfig{}, err
		}
		config.OKXBaseURL = normalized
	}
	return config, nil
}

func normalizeLoopbackBaseURL(rawURL, envKey string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("%s must be an absolute http(s) URL", envKey)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("%s must use http or https", envKey)
	}
	if parsed.User != nil {
		return "", fmt.Errorf("%s must not include user info", envKey)
	}
	if parsed.EscapedPath() != "" && parsed.EscapedPath() != "/" {
		return "", fmt.Errorf("%s must not include a path", envKey)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("%s must not include query or fragment", envKey)
	}
	if !isLoopbackHost(parsed.Hostname()) {
		return "", fmt.Errorf("%s must point to localhost, 127.0.0.1, or ::1", envKey)
	}
	return strings.TrimRight(parsed.Scheme+"://"+parsed.Host, "/"), nil
}

func isLoopbackHost(host string) bool {
	host = strings.Trim(strings.ToLower(strings.TrimSpace(host)), "[]")
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func buildPreflightReport(ctx context.Context, deps preflightDeps) (preflightReport, error) {
	if deps.Store == nil {
		return preflightReport{}, errors.New("store is required")
	}
	checks := []preflightCheck{
		preflightOriginCheck(),
		preflightDatabaseCheck(deps.Config.DBPath),
		preflightProductionCheck(),
		preflightKillSwitchCheck(deps.KillSwitchState),
		preflightLiveGuardCheck(deps.GuardState),
		preflightAutopilotCheck(deps.AutopilotState),
	}

	auditCheck, err := preflightAuditCheck(ctx, deps.Store)
	if err != nil {
		return preflightReport{}, err
	}
	checks = append(checks, auditCheck)

	vaultCheck, err := preflightVaultCheck(ctx, deps.Store)
	if err != nil {
		return preflightReport{}, err
	}
	checks = append(checks, vaultCheck)

	riskCheck, err := preflightRiskCheck(ctx, deps.Store)
	if err != nil {
		return preflightReport{}, err
	}
	checks = append(checks, riskCheck)

	strategyCheck, err := preflightStrategyCheck(ctx, deps.Store)
	if err != nil {
		return preflightReport{}, err
	}
	checks = append(checks, strategyCheck)

	marketChecks := preflightMarketChecks(ctx, deps.Registry)
	checks = append(checks, marketChecks...)
	liveAutopilotCheck, err := preflightLiveAutopilotCheck(ctx, deps.Store, deps.GuardState, deps.KillSwitchState, deps.AutopilotState, vaultCheck, riskCheck, strategyCheck, marketChecks)
	if err != nil {
		return preflightReport{}, err
	}
	checks = append(checks, liveAutopilotCheck)

	report := preflightReport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Checks:      checks,
	}
	for _, check := range checks {
		switch check.Status {
		case preflightStatusReady:
			report.Ready++
		case preflightStatusWarn:
			report.Warn++
		case preflightStatusBlock:
			report.Block++
		}
	}
	report.Overall = preflightStatusReady
	if report.Warn > 0 {
		report.Overall = preflightStatusWarn
	}
	if report.Block > 0 {
		report.Overall = preflightStatusBlock
	}
	return report, nil
}

func preflightOriginCheck() preflightCheck {
	return preflightCheck{
		ID:      "origin",
		Label:   "Local Origin",
		Status:  preflightStatusReady,
		Summary: "local browser origins only",
		Details: map[string]any{"allowed": []string{"127.0.0.1", "localhost", "::1"}},
	}
}

func preflightDatabaseCheck(path string) preflightCheck {
	info := databaseInfo(path)
	if !info.Exists {
		return preflightCheck{
			ID:      "database",
			Label:   "SQLite",
			Status:  preflightStatusBlock,
			Summary: "database file is not available",
			Details: map[string]any{"path": info.Path},
		}
	}
	return preflightCheck{
		ID:      "database",
		Label:   "SQLite",
		Status:  preflightStatusReady,
		Summary: "local store ready",
		Details: map[string]any{"path": info.Path, "sizeBytes": info.SizeBytes},
	}
}

func preflightProductionCheck() preflightCheck {
	return preflightCheck{
		ID:      "production",
		Label:   "Production Gate",
		Status:  preflightStatusReady,
		Summary: "mainnet trading disabled",
		Details: map[string]any{"liveEnvironments": []string{"testnet", "demo"}},
	}
}

func preflightKillSwitchCheck(state killswitch.State) preflightCheck {
	if state.Active {
		return preflightCheck{
			ID:      "kill_switch",
			Label:   "Kill Switch",
			Status:  preflightStatusBlock,
			Summary: "AI execution halted",
			Details: map[string]any{"reason": state.Reason, "activatedAt": state.ActivatedAt},
		}
	}
	return preflightCheck{
		ID:      "kill_switch",
		Label:   "Kill Switch",
		Status:  preflightStatusReady,
		Summary: "clear",
	}
}

func preflightLiveGuardCheck(state liveguard.State) preflightCheck {
	if state.Unlocked {
		return preflightCheck{
			ID:      "live_guard",
			Label:   "Live Guard",
			Status:  preflightStatusWarn,
			Summary: "testnet/demo session unlocked",
			Details: map[string]any{"environment": state.Environment, "expiresAt": state.ExpiresAt, "remainingSeconds": state.RemainingSeconds},
		}
	}
	return preflightCheck{
		ID:      "live_guard",
		Label:   "Live Guard",
		Status:  preflightStatusReady,
		Summary: "locked",
	}
}

func preflightAutopilotCheck(state autopilot.State) preflightCheck {
	if state.Running {
		return preflightCheck{
			ID:      "autopilot",
			Label:   "Autopilot",
			Status:  preflightStatusWarn,
			Summary: "AI loop running",
			Details: map[string]any{"runId": state.RunID, "mode": state.Mode, "completedSteps": state.CompletedSteps},
		}
	}
	return preflightCheck{
		ID:      "autopilot",
		Label:   "Autopilot",
		Status:  preflightStatusReady,
		Summary: "idle",
	}
}

func preflightAuditCheck(ctx context.Context, store *storage.Store) (preflightCheck, error) {
	verification, err := store.VerifyAudit(ctx)
	if err != nil {
		return preflightCheck{}, err
	}
	if !verification.Valid {
		return preflightCheck{
			ID:      "audit",
			Label:   "Audit Chain",
			Status:  preflightStatusBlock,
			Summary: "hash verification failed",
			Details: map[string]any{"checked": verification.Checked, "error": verification.Error},
		}, nil
	}
	return preflightCheck{
		ID:      "audit",
		Label:   "Audit Chain",
		Status:  preflightStatusReady,
		Summary: "hash ok",
		Details: map[string]any{"checked": verification.Checked},
	}, nil
}

func preflightVaultCheck(ctx context.Context, store *storage.Store) (preflightCheck, error) {
	credentials, err := store.ListCredentials(ctx)
	if err != nil {
		return preflightCheck{}, err
	}
	if len(credentials) == 0 {
		return preflightCheck{
			ID:      "vault",
			Label:   "Vault",
			Status:  preflightStatusWarn,
			Summary: "no saved trade key",
			Details: map[string]any{"credentials": 0},
		}, nil
	}
	tradeKeys := 0
	for _, credential := range credentials {
		if credential.Permissions.Trade {
			tradeKeys++
		}
	}
	status := preflightStatusReady
	summary := "trade key available"
	if tradeKeys == 0 {
		status = preflightStatusWarn
		summary = "read-only keys saved"
	}
	return preflightCheck{
		ID:      "vault",
		Label:   "Vault",
		Status:  status,
		Summary: summary,
		Details: map[string]any{"credentials": len(credentials), "tradeKeys": tradeKeys},
	}, nil
}

func preflightRiskCheck(ctx context.Context, store *storage.Store) (preflightCheck, error) {
	profile, err := store.RiskProfile(ctx)
	if err != nil {
		return preflightCheck{}, err
	}
	profile = storage.NormalizeRiskProfile(profile)
	if profile.MaxOrderUSDT <= 0 || profile.MaxTotalExposureUSDT <= 0 || profile.MinConfidence <= 0 || profile.MinConfidence > 1 {
		return preflightCheck{
			ID:      "risk_profile",
			Label:   "Risk Profile",
			Status:  preflightStatusBlock,
			Summary: "invalid risk limits",
		}, nil
	}
	return preflightCheck{
		ID:      "risk_profile",
		Label:   "Risk Profile",
		Status:  preflightStatusReady,
		Summary: "limits loaded",
		Details: map[string]any{"maxOrderUsdt": profile.MaxOrderUSDT, "minConfidence": profile.MinConfidence},
	}, nil
}

func preflightStrategyCheck(ctx context.Context, store *storage.Store) (preflightCheck, error) {
	profile, err := store.StrategyProfile(ctx)
	if err != nil {
		return preflightCheck{}, err
	}
	profile = storage.NormalizeStrategyProfile(profile)
	if _, ok := supportedExchangeName(profile.Exchange); !ok || strings.TrimSpace(profile.Symbol) == "" || profile.OrderSizeUSDT <= 0 {
		return preflightCheck{
			ID:      "strategy_profile",
			Label:   "Strategy",
			Status:  preflightStatusBlock,
			Summary: "invalid strategy defaults",
		}, nil
	}
	return preflightCheck{
		ID:      "strategy_profile",
		Label:   "Strategy",
		Status:  preflightStatusReady,
		Summary: profile.Exchange + " / " + profile.Symbol,
		Details: map[string]any{"side": profile.Side, "orderSizeUsdt": profile.OrderSizeUSDT},
	}, nil
}

func preflightLiveAutopilotCheck(ctx context.Context, store *storage.Store, guard liveguard.State, killSwitch killswitch.State, autopilotState autopilot.State, vaultCheck, riskCheck, strategyCheck preflightCheck, marketChecks []preflightCheck) (preflightCheck, error) {
	status := preflightStatusReady
	type liveIssue struct {
		status string
		text   string
	}
	issues := []liveIssue{}
	details := map[string]any{
		"guard":       "locked",
		"snapshot":    "not_checked",
		"marketReady": countPreflightStatus(marketChecks, preflightStatusReady),
		"marketTotal": len(marketChecks),
	}
	addIssue := func(nextStatus, issue string) {
		if nextStatus == preflightStatusBlock {
			status = preflightStatusBlock
		} else if status != preflightStatusBlock && nextStatus == preflightStatusWarn {
			status = preflightStatusWarn
		}
		issues = append(issues, liveIssue{status: nextStatus, text: issue})
	}

	if killSwitch.Active {
		addIssue(preflightStatusBlock, "kill switch active")
	}
	if riskCheck.Status == preflightStatusBlock {
		addIssue(preflightStatusBlock, "risk profile blocked")
	}
	if strategyCheck.Status == preflightStatusBlock {
		addIssue(preflightStatusBlock, "strategy blocked")
	}
	if anyPreflightStatus(marketChecks, preflightStatusBlock) {
		addIssue(preflightStatusBlock, "market adapter blocked")
	}
	if autopilotState.Running {
		addIssue(preflightStatusWarn, "autopilot already running")
	}

	tradeKeys := intFromDetails(vaultCheck.Details, "tradeKeys")
	details["tradeKeys"] = tradeKeys
	if tradeKeys <= 0 {
		addIssue(preflightStatusWarn, "trade key required")
	}
	if !guard.Unlocked {
		addIssue(preflightStatusWarn, "guard locked")
	} else {
		details["guard"] = guard.Environment
	}

	if tradeKeys > 0 && guard.Unlocked && strategyCheck.Status == preflightStatusReady {
		profile, err := store.StrategyProfile(ctx)
		if err != nil {
			return preflightCheck{}, err
		}
		profile = storage.NormalizeStrategyProfile(profile)
		record, err := store.LatestAccountSnapshot(ctx, storage.AccountSnapshotFilter{
			Exchange:    profile.Exchange,
			Environment: guard.Environment,
			Symbol:      profile.Symbol,
		})
		switch {
		case errors.Is(err, sql.ErrNoRows):
			details["snapshot"] = "missing"
			addIssue(preflightStatusWarn, "account sync required")
		case err != nil:
			return preflightCheck{}, err
		default:
			details["snapshot"] = "fresh"
			details["snapshotId"] = record.ID
			details["snapshotCreatedAt"] = record.CreatedAt
			createdAt, parseErr := time.Parse(time.RFC3339, record.CreatedAt)
			if parseErr != nil || time.Since(createdAt) > 5*time.Minute {
				details["snapshot"] = "stale"
				addIssue(preflightStatusWarn, "account sync stale")
			}
		}
	}
	if !anyPreflightStatus(marketChecks, preflightStatusBlock) && anyPreflightStatus(marketChecks, preflightStatusWarn) {
		addIssue(preflightStatusWarn, "market warning")
	}

	summary := "ready for live autopilot"
	if len(issues) > 0 {
		summary = issues[0].text
		if status == preflightStatusBlock {
			for _, issue := range issues {
				if issue.status == preflightStatusBlock {
					summary = issue.text
					break
				}
			}
		}
	}
	issueTexts := make([]string, 0, len(issues))
	for _, issue := range issues {
		issueTexts = append(issueTexts, issue.text)
	}
	details["issues"] = issueTexts
	return preflightCheck{
		ID:      "live_autopilot",
		Label:   "Live Autopilot",
		Status:  status,
		Summary: summary,
		Details: details,
	}, nil
}

func anyPreflightStatus(checks []preflightCheck, status string) bool {
	for _, check := range checks {
		if check.Status == status {
			return true
		}
	}
	return false
}

func countPreflightStatus(checks []preflightCheck, status string) int {
	count := 0
	for _, check := range checks {
		if check.Status == status {
			count++
		}
	}
	return count
}

func intFromDetails(details map[string]any, key string) int {
	switch value := details[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}

func preflightMarketCheck(ctx context.Context, registry exchange.Registry, exchangeName, symbol string) preflightCheck {
	adapter, ok := registry.Get(exchangeName)
	id := "market_" + strings.ToLower(exchangeName)
	if !ok {
		return preflightCheck{
			ID:      id,
			Label:   exchangeName + " Market",
			Status:  preflightStatusBlock,
			Summary: "adapter unavailable",
		}
	}
	checkCtx, cancel := context.WithTimeout(ctx, preflightMarketTimeout)
	defer cancel()
	start := time.Now()
	snapshot, err := adapter.FetchSnapshot(checkCtx, symbol)
	elapsedMS := int(time.Since(start).Milliseconds())
	if elapsedMS < 1 {
		elapsedMS = 1
	}
	if err != nil {
		return preflightCheck{
			ID:      id,
			Label:   exchangeName + " Market",
			Status:  preflightStatusWarn,
			Summary: "public data unavailable",
			Details: map[string]any{"symbol": symbol, "error": err.Error(), "latencyMs": elapsedMS},
		}
	}
	return preflightCheck{
		ID:      id,
		Label:   exchangeName + " Market",
		Status:  preflightStatusReady,
		Summary: "public data ready",
		Details: map[string]any{"symbol": snapshot.Symbol, "last": snapshot.Last, "latencyMs": elapsedMS},
	}
}

func preflightMarketChecks(ctx context.Context, registry exchange.Registry) []preflightCheck {
	exchanges := []string{"Binance", "OKX"}
	checks := make([]preflightCheck, len(exchanges))
	var waitGroup sync.WaitGroup
	for index, exchangeName := range exchanges {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			checks[index] = preflightMarketCheck(ctx, registry, exchangeName, "BTCUSDT")
		}()
	}
	waitGroup.Wait()
	return checks
}

func databaseInfo(path string) appDatabaseInfo {
	absolutePath := path
	if resolved, err := filepath.Abs(path); err == nil {
		absolutePath = resolved
	}
	info := appDatabaseInfo{
		Path: absolutePath,
		Dir:  filepath.Dir(absolutePath),
	}
	if stat, err := os.Stat(path); err == nil {
		info.Exists = true
		info.SizeBytes = stat.Size()
	}
	return info
}

func openBrowser(rawURL string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", rawURL)
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	default:
		command = exec.Command("xdg-open", rawURL)
	}
	return command.Start()
}

func openExistingClient(config appConfig) bool {
	uiURL := config.UIURL()
	if !existingClientHealthy(uiURL) {
		return false
	}
	log.Printf("CCVar Quant is already running at %s; opening existing client", uiURL)
	if err := openBrowser(uiURL); err != nil {
		log.Printf("open existing client: %v", err)
	}
	return true
}

func existingClientHealthy(uiURL string) bool {
	healthURL := strings.TrimRight(uiURL, "/") + "/api/health"
	client := http.Client{Timeout: existingClientHealthTimeout}
	response, err := client.Get(healthURL)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return false
	}
	var payload struct {
		OK      bool   `json:"ok"`
		Service string `json:"service"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload); err != nil {
		return false
	}
	return payload.OK && payload.Service == "ccvar-quant"
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func boolEnv(key string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func isAllowedLocalOrigin(origin string) bool {
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func supportedExchangeName(name string) (string, bool) {
	switch {
	case strings.EqualFold(strings.TrimSpace(name), "Binance"):
		return "Binance", true
	case strings.EqualFold(strings.TrimSpace(name), "OKX"):
		return "OKX", true
	default:
		return "", false
	}
}

func localDataPruneDefaults() storage.LocalDataPruneOptions {
	return storage.LocalDataPruneOptions{
		KeepBacktestRuns:     30,
		KeepAutopilotRuns:    20,
		KeepPaperExecutions:  500,
		KeepAccountSnapshots: 50,
	}
}

func localDataProtectedSets() []string {
	return []string{
		"credentials",
		"audit_log",
		"live_execution_records",
		"live_reconciliation_records",
		"risk_profile",
		"strategy_profile",
	}
}

const (
	serviceVersion               = "0.1.0"
	preflightStatusReady         = "ready"
	preflightStatusWarn          = "warn"
	preflightStatusBlock         = "block"
	preflightReportTimeout       = 5 * time.Second
	preflightMarketTimeout       = 2500 * time.Millisecond
	existingClientHealthTimeout  = 1 * time.Second
	workspaceExportRunLimit      = 12
	workspaceExportStepLimit     = 200
	workspaceExportStepsPerRun   = 40
	workspaceExportBacktestLimit = 20
	workspaceExportPaperLimit    = 200
	workspaceExportLiveLimit     = 100
	workspaceExportAuditLogLimit = 200
	paperAccountRecordLimit      = 1000
	paperResetPhrase             = "RESET PAPER"
	localDataPrunePhrase         = "PRUNE LOCAL DATA"
	loopbackExchangeMocksEnv     = "CCVAR_ENABLE_LOOPBACK_EXCHANGE_MOCKS"
	binancePrivateMockURLEnv     = "CCVAR_BINANCE_PRIVATE_MOCK_URL"
	okxPrivateMockURLEnv         = "CCVAR_OKX_PRIVATE_MOCK_URL"
)

type workspaceExport struct {
	GeneratedAt         string                             `json:"generatedAt"`
	Service             string                             `json:"service"`
	Version             string                             `json:"version"`
	Safety              workspaceExportSafety              `json:"safety"`
	StrategyProfile     storage.StrategyProfileRecord      `json:"strategyProfile"`
	RiskProfile         storage.RiskProfileRecord          `json:"riskProfile"`
	Autopilot           workspaceExportAutopilot           `json:"autopilot"`
	BacktestRuns        []storage.BacktestRunRecord        `json:"backtestRuns"`
	PaperAccount        paperaccount.Snapshot              `json:"paperAccount"`
	PaperExecutions     []storage.PaperExecutionRecord     `json:"paperExecutions"`
	LiveExecutions      []storage.LiveExecutionRecord      `json:"liveExecutions"`
	LiveReconciliations []storage.LiveReconciliationRecord `json:"liveReconciliations"`
	Audit               workspaceExportAudit               `json:"audit"`
}

type workspaceExportSafety struct {
	IncludesCredentialData    bool     `json:"includesCredentialData"`
	IncludesSensitiveMaterial bool     `json:"includesSensitiveMaterial"`
	Excluded                  []string `json:"excluded"`
}

type workspaceExportAutopilot struct {
	State     autopilot.State               `json:"state"`
	Runs      []storage.AutopilotRunRecord  `json:"runs"`
	Steps     []storage.AutopilotStepRecord `json:"steps"`
	RunLimit  int                           `json:"runLimit"`
	StepLimit int                           `json:"stepLimit"`
}

type workspaceExportAudit struct {
	Verification audit.Verification `json:"verification"`
	Entries      []audit.Entry      `json:"entries"`
	EntryLimit   int                `json:"entryLimit"`
}

func buildWorkspaceExport(ctx context.Context, store *storage.Store, autoPilot *autopilot.Service) (workspaceExport, error) {
	generatedAt := time.Now().UTC().Format(time.RFC3339)
	strategyProfile, err := store.StrategyProfile(ctx)
	if err != nil {
		return workspaceExport{}, err
	}
	riskProfile, err := store.RiskProfile(ctx)
	if err != nil {
		return workspaceExport{}, err
	}
	autopilotRuns, err := autoPilot.History(ctx, workspaceExportRunLimit)
	if err != nil {
		return workspaceExport{}, err
	}
	autopilotSteps := []storage.AutopilotStepRecord{}
	remainingSteps := workspaceExportStepLimit
	for _, run := range autopilotRuns {
		if remainingSteps <= 0 {
			break
		}
		limit := workspaceExportStepsPerRun
		if remainingSteps < limit {
			limit = remainingSteps
		}
		steps, err := autoPilot.Steps(ctx, run.ID, limit)
		if err != nil {
			return workspaceExport{}, err
		}
		autopilotSteps = append(autopilotSteps, steps...)
		remainingSteps -= len(steps)
	}
	backtestRuns, err := store.ListBacktestRuns(ctx, workspaceExportBacktestLimit)
	if err != nil {
		return workspaceExport{}, err
	}
	paperExecutions, err := store.ListPaperExecutions(ctx, workspaceExportPaperLimit)
	if err != nil {
		return workspaceExport{}, err
	}
	paperAccountSnapshot := paperaccount.Build(paperExecutions)
	liveExecutions, err := store.ListLiveExecutions(ctx, workspaceExportLiveLimit)
	if err != nil {
		return workspaceExport{}, err
	}
	liveReconciliations, err := store.ListLiveReconciliations(ctx, 0, workspaceExportLiveLimit)
	if err != nil {
		return workspaceExport{}, err
	}
	auditEntries, err := store.ListAudit(ctx, workspaceExportAuditLogLimit)
	if err != nil {
		return workspaceExport{}, err
	}
	auditVerification, err := store.VerifyAudit(ctx)
	if err != nil {
		return workspaceExport{}, err
	}
	return workspaceExport{
		GeneratedAt: generatedAt,
		Service:     "ccvar-quant",
		Version:     serviceVersion,
		Safety: workspaceExportSafety{
			IncludesCredentialData:    false,
			IncludesSensitiveMaterial: false,
			Excluded: []string{
				"saved exchange credentials",
				"private authentication material",
				"local vault unlock phrases",
				"encrypted credential payload material",
				"credential key-derivation material",
				"credential encryption counters",
			},
		},
		StrategyProfile: strategyProfile,
		RiskProfile:     riskProfile,
		Autopilot: workspaceExportAutopilot{
			State:     autoPilot.State(),
			Runs:      autopilotRuns,
			Steps:     autopilotSteps,
			RunLimit:  workspaceExportRunLimit,
			StepLimit: workspaceExportStepLimit,
		},
		BacktestRuns:        backtestRuns,
		PaperAccount:        paperAccountSnapshot,
		PaperExecutions:     paperExecutions,
		LiveExecutions:      liveExecutions,
		LiveReconciliations: liveReconciliations,
		Audit: workspaceExportAudit{
			Verification: auditVerification,
			Entries:      auditEntries,
			EntryLimit:   workspaceExportAuditLogLimit,
		},
	}, nil
}

func buildPaperAccountSnapshot(ctx context.Context, store *storage.Store) (paperaccount.Snapshot, error) {
	records, err := store.ListPaperExecutions(ctx, paperAccountRecordLimit)
	if err != nil {
		return paperaccount.Snapshot{}, err
	}
	return paperaccount.Build(records), nil
}

func simulationMode(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), "paper") {
		return "paper"
	}
	return "shadow"
}

func riskLimitsFromProfile(profile storage.RiskProfileRecord) risk.Limits {
	profile = storage.NormalizeRiskProfile(profile)
	return risk.Limits{
		MinConfidence:        profile.MinConfidence,
		MaxOrderUSDT:         profile.MaxOrderUSDT,
		MaxSymbolExposure:    profile.MaxSymbolExposureUSDT,
		MaxTotalExposure:     profile.MaxTotalExposureUSDT,
		MaxDailyDrawdownPct:  profile.MaxDailyDrawdownPct,
		MaxConsecutiveLosses: profile.MaxConsecutiveLosses,
		MaxSpreadPct:         profile.MaxSpreadPct,
		RequireLiveUnlock:    true,
	}
}

func strategyConfigFromProfile(profile storage.StrategyProfileRecord) simrun.StrategyConfig {
	profile = storage.NormalizeStrategyProfile(profile)
	side := core.SideBuy
	if strings.EqualFold(profile.Side, "sell") {
		side = core.SideSell
	}
	return simrun.StrategyConfig{
		Name:            profile.Name,
		Exchange:        profile.Exchange,
		Symbol:          profile.Symbol,
		Side:            side,
		OrderSizeUSDT:   profile.OrderSizeUSDT,
		IntervalSeconds: profile.IntervalSeconds,
		MaxSteps:        profile.MaxSteps,
	}
}

func backtestConfigFromProfile(profile storage.StrategyProfileRecord, request backtestRequest) backtest.Config {
	profile = storage.NormalizeStrategyProfile(profile)
	side := core.SideBuy
	if strings.EqualFold(defaultString(request.Side, profile.Side), "sell") {
		side = core.SideSell
	}
	config := backtest.Config{
		StrategyName:  profile.Name,
		Exchange:      profile.Exchange,
		Symbol:        profile.Symbol,
		Side:          side,
		Interval:      "15m",
		OrderSizeUSDT: profile.OrderSizeUSDT,
		FastWindow:    request.FastWindow,
		SlowWindow:    request.SlowWindow,
		FeeRate:       0.0005,
		SlippagePct:   0.0002,
	}
	if strings.TrimSpace(request.Symbol) != "" {
		config.Symbol = strings.ToUpper(strings.TrimSpace(request.Symbol))
	}
	if strings.TrimSpace(request.Interval) != "" {
		config.Interval = strings.TrimSpace(request.Interval)
	}
	if request.OrderSizeUSDT > 0 {
		config.OrderSizeUSDT = request.OrderSizeUSDT
	}
	return config
}

func applyStrategyDefaults(request autopilot.Request, profile storage.StrategyProfileRecord) autopilot.Request {
	profile = storage.NormalizeStrategyProfile(profile)
	if !strings.EqualFold(request.Mode, "live") {
		if strings.TrimSpace(request.Exchange) == "" {
			request.Exchange = profile.Exchange
		}
		if strings.TrimSpace(request.Symbol) == "" {
			request.Symbol = profile.Symbol
		}
	}
	if strings.TrimSpace(request.Side) == "" {
		request.Side = profile.Side
	}
	if request.SizeUSDT <= 0 {
		request.SizeUSDT = profile.OrderSizeUSDT
	}
	if request.IntervalSeconds <= 0 {
		request.IntervalSeconds = profile.IntervalSeconds
	}
	if request.MaxSteps <= 0 && profile.MaxSteps > 0 {
		request.MaxSteps = profile.MaxSteps
	}
	return request
}

func buildLiveAutopilotPlan(ctx context.Context, registry exchange.Registry, store *storage.Store, request autopilot.LivePlanRequest) (autopilot.LivePlan, error) {
	exchangeName, ok := supportedExchangeName(request.Exchange)
	if !ok {
		return autopilot.LivePlan{}, fmt.Errorf("unknown exchange %q", request.Exchange)
	}
	adapter, ok := registry.Get(exchangeName)
	if !ok {
		return autopilot.LivePlan{}, fmt.Errorf("missing exchange adapter %q", exchangeName)
	}
	symbol := strings.ToUpper(defaultString(request.Symbol, "BTCUSDT"))
	market, err := adapter.FetchSnapshot(ctx, symbol)
	if err != nil {
		return autopilot.LivePlan{}, err
	}
	candles, _ := adapter.FetchCandles(ctx, symbol, "15m", 96)
	profile, err := store.StrategyProfile(ctx)
	if err != nil {
		return autopilot.LivePlan{}, err
	}
	profile = storage.NormalizeStrategyProfile(profile)
	side := core.Side(strings.ToLower(defaultString(request.Side, profile.Side)))
	if side != core.SideSell {
		side = core.SideBuy
	}
	size := request.SizeUSDT
	if size <= 0 {
		size = profile.OrderSizeUSDT
	}
	now := time.Now().UTC()
	decision, err := ai.NewLocalPolicy().GenerateIntent(ctx, ai.Context{
		Account: aiAccountStateFromSync(request.AccountSync.Snapshot),
		Market:  market,
		Mode:    core.ModeLive,
		Strategy: ai.Strategy{
			Name:          profile.Name,
			Side:          side,
			OrderSizeUSDT: size,
		},
		Candles: candles,
		Now:     now,
	})
	if err != nil {
		return autopilot.LivePlan{}, err
	}
	return autopilot.LivePlan{
		Intent: decision.Intent,
		AI:     decision.Trace,
		Events: []storage.Event{{
			Time:   now.Format("15:04:05"),
			Type:   "AI Live Plan",
			Symbol: decision.Intent.Symbol,
			Action: strings.ToUpper(string(decision.Intent.Side)),
			Price:  decision.Intent.Price,
			Result: fmt.Sprintf("Conf: %.0f%%", decision.Intent.Confidence*100),
			Note:   decision.Trace.Model + " " + decision.Trace.PolicyVersion,
			Level:  "info",
		}},
	}, nil
}

func aiAccountStateFromSync(snapshot livesync.AccountSnapshot) core.AccountState {
	equity := 0.0
	available := 0.0
	for _, balance := range snapshot.Balances {
		asset := strings.ToUpper(strings.TrimSpace(balance.Asset))
		switch {
		case balance.USD > 0:
			equity += balance.USD
		case asset == "USDT" || asset == "USDC" || asset == "USD":
			equity += balance.Total
		}
		if asset == "USDT" || asset == "USDC" || asset == "USD" {
			available += balance.Free
		}
	}
	return core.AccountState{
		EquityUSDT:         equity,
		AvailableUSDT:      available,
		SymbolExposureUSDT: map[string]float64{},
	}
}

type liveGuardRequest struct {
	Action       string  `json:"action"`
	Operator     string  `json:"operator"`
	Environment  string  `json:"environment"`
	Phrase       string  `json:"phrase"`
	TTLSeconds   int     `json:"ttlSeconds"`
	MaxOrderUSDT float64 `json:"maxOrderUsdt"`
	Reason       string  `json:"reason"`
}

type killSwitchRequest struct {
	Action   string `json:"action"`
	Operator string `json:"operator"`
	Reason   string `json:"reason"`
}

type paperResetRequest struct {
	Operator string `json:"operator"`
	Reason   string `json:"reason"`
	Phrase   string `json:"phrase"`
}

type localDataPruneRequest struct {
	Operator             string `json:"operator"`
	Reason               string `json:"reason"`
	Phrase               string `json:"phrase"`
	KeepBacktestRuns     int    `json:"keepBacktestRuns"`
	KeepAutopilotRuns    int    `json:"keepAutopilotRuns"`
	KeepPaperExecutions  int    `json:"keepPaperExecutions"`
	KeepAccountSnapshots int    `json:"keepAccountSnapshots"`
}

type backtestRequest struct {
	Exchange      string  `json:"exchange"`
	Symbol        string  `json:"symbol"`
	Side          string  `json:"side"`
	Interval      string  `json:"interval"`
	Limit         int     `json:"limit"`
	FastWindow    int     `json:"fastWindow"`
	SlowWindow    int     `json:"slowWindow"`
	OrderSizeUSDT float64 `json:"orderSizeUsdt"`
}

func liveGuardSummary(err error, state liveguard.State) string {
	if err != nil {
		return err.Error()
	}
	return "testnet live unlock active until " + state.ExpiresAt
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func defaultString(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func liveExecuteStatus(err error) int {
	switch {
	case errors.Is(err, liveexec.ErrKillSwitchActive),
		errors.Is(err, liveexec.ErrLiveGuardLocked):
		return http.StatusLocked
	case errors.Is(err, liveexec.ErrCredentialRequired),
		errors.Is(err, liveexec.ErrCredentialPassRequired),
		errors.Is(err, liveexec.ErrCredentialExchange),
		errors.Is(err, liveexec.ErrTradePermissionRequired),
		errors.Is(err, liveexec.ErrUnsupportedExchange):
		return http.StatusBadRequest
	case errors.Is(err, liveexec.ErrAccountSnapshotRequired),
		errors.Is(err, liveexec.ErrAccountSnapshotStale),
		errors.Is(err, liveexec.ErrAccountCannotTrade):
		return http.StatusPreconditionRequired
	default:
		return http.StatusBadGateway
	}
}

func accountSyncStatus(err error) int {
	switch {
	case errors.Is(err, livesync.ErrCredentialRequired),
		errors.Is(err, livesync.ErrCredentialPassRequired),
		errors.Is(err, livesync.ErrCredentialExchange),
		errors.Is(err, livesync.ErrUnsupportedExchange),
		errors.Is(err, livesync.ErrUnsupportedEnvironment):
		return http.StatusBadRequest
	default:
		return http.StatusBadGateway
	}
}

func liveReconcileStatus(err error) int {
	switch {
	case errors.Is(err, livereconcile.ErrLiveExecutionRequired),
		errors.Is(err, livereconcile.ErrCredentialPassRequired),
		errors.Is(err, livereconcile.ErrCredentialExchange),
		errors.Is(err, livereconcile.ErrUnsupportedExchange),
		errors.Is(err, livereconcile.ErrUnsupportedEnvironment):
		return http.StatusBadRequest
	case errors.Is(err, livereconcile.ErrLiveExecutionNotFound):
		return http.StatusNotFound
	case errors.Is(err, livereconcile.ErrValidationOnlyExecution),
		errors.Is(err, livereconcile.ErrUnsubmittedExecution):
		return http.StatusPreconditionRequired
	default:
		return http.StatusBadGateway
	}
}

func autopilotStatus(err error) int {
	switch {
	case errors.Is(err, autopilot.ErrKillSwitchActive):
		return http.StatusLocked
	case errors.Is(err, autopilot.ErrAlreadyRunning),
		errors.Is(err, autopilot.ErrNotRunning):
		return http.StatusConflict
	case errors.Is(err, autopilot.ErrUnsupportedMode),
		errors.Is(err, autopilot.ErrLiveCredentialNeeded):
		return http.StatusBadRequest
	default:
		return http.StatusBadGateway
	}
}
