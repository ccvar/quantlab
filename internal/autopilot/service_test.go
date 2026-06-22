package autopilot

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ccvar.com/web3quant/internal/ai"
	"ccvar.com/web3quant/internal/core"
	"ccvar.com/web3quant/internal/liveexec"
	"ccvar.com/web3quant/internal/livesync"
	"ccvar.com/web3quant/internal/risk"
	"ccvar.com/web3quant/internal/simrun"
	"ccvar.com/web3quant/internal/storage"
)

func TestStartRunsImmediateShadowStepAndStopsAtMaxSteps(t *testing.T) {
	store := openTestStore(t)
	calls := 0
	service := New(store, func(context.Context, string, string) (simrun.Result, error) {
		calls++
		return simResult(), nil
	}, nil, nil, nil)
	service.WithClock(fixedNow)

	state, err := service.Start(context.Background(), Request{
		Operator:        "qa",
		Mode:            "shadow",
		Exchange:        "Binance",
		Symbol:          "BTCUSDT",
		IntervalSeconds: 1,
		MaxSteps:        1,
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
	if state.Running {
		t.Fatalf("state.Running = true, want false after max steps")
	}
	if state.RunID == 0 {
		t.Fatal("state.RunID was not assigned")
	}
	if state.CompletedSteps != 1 || state.LastStatus != "completed" || state.IntervalSeconds != 5 {
		t.Fatalf("state = %#v", state)
	}
	if strings.Contains(string(state.LastResult), "passphrase") {
		t.Fatalf("LastResult leaked passphrase: %s", string(state.LastResult))
	}
	entries, err := store.ListAudit(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	if len(entries) != 2 || entries[1].Action != "autopilot.start" || entries[0].Action != "autopilot.step" {
		t.Fatalf("entries = %#v", entries)
	}
	runs, err := store.ListAutopilotRuns(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListAutopilotRuns() error = %v", err)
	}
	if len(runs) != 1 || runs[0].ID != state.RunID || runs[0].Status != "completed" || runs[0].CompletedSteps != 1 {
		t.Fatalf("runs = %#v", runs)
	}
	steps, err := store.ListAutopilotSteps(context.Background(), state.RunID, 10)
	if err != nil {
		t.Fatalf("ListAutopilotSteps() error = %v", err)
	}
	if len(steps) != 1 || steps[0].StepNumber != 1 || steps[0].Status != "ok" || strings.Contains(string(steps[0].ResultJSON), "passphrase") {
		t.Fatalf("steps = %#v", steps)
	}
	paperRecords, err := store.ListPaperExecutions(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListPaperExecutions() error = %v", err)
	}
	if len(paperRecords) != 1 || paperRecords[0].RunID != state.RunID || paperRecords[0].Source != "autopilot" || paperRecords[0].FillStatus != "filled" {
		t.Fatalf("paperRecords = %#v", paperRecords)
	}
}

func TestStartRejectsKillSwitchActive(t *testing.T) {
	service := New(nil, func(context.Context, string, string) (simrun.Result, error) {
		t.Fatal("sim step should not be called")
		return simrun.Result{}, nil
	}, nil, nil, func() bool { return true })

	_, err := service.Start(context.Background(), Request{Mode: "shadow"})
	if !errors.Is(err, ErrKillSwitchActive) {
		t.Fatalf("Start() err = %v, want ErrKillSwitchActive", err)
	}
}

func TestLiveStepSyncsAccountBeforeExecute(t *testing.T) {
	store := openTestStore(t)
	var syncRequest livesync.Request
	var executeRequest liveexec.Request
	service := New(
		store,
		nil,
		func(_ context.Context, request livesync.Request) (livesync.Result, error) {
			syncRequest = request
			return livesync.Result{
				Snapshot: livesync.AccountSnapshot{
					Exchange:    "OKX",
					Environment: "demo",
					CanTrade:    true,
					SyncedAt:    fixedNow().Format(time.RFC3339),
				},
			}, nil
		},
		func(_ context.Context, request liveexec.Request) (liveexec.Result, error) {
			executeRequest = request
			return liveexec.Result{
				Decision: risk.Decision{Approved: true},
				Events: []storage.Event{{
					Time:   "10:30:00",
					Type:   "Live Execute",
					Symbol: "BTCUSDT",
					Result: "submitted",
					Level:  "success",
				}},
				Environment: "demo",
			}, nil
		},
		nil,
	)
	service.WithClock(fixedNow)

	state, err := service.Start(context.Background(), Request{
		Operator:        "qa",
		Mode:            "live",
		Exchange:        "OKX",
		Environment:     "demo",
		Symbol:          "BTCUSDT",
		IntervalSeconds: 5,
		MaxSteps:        1,
		CredentialID:    77,
		Passphrase:      "correct horse battery",
		Side:            "sell",
		SizeUSDT:        123,
		ValidationOnly:  true,
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if syncRequest.Environment != "demo" || syncRequest.Passphrase != "correct horse battery" || syncRequest.CredentialID != 77 {
		t.Fatalf("syncRequest = %#v", syncRequest)
	}
	if executeRequest.Exchange != "OKX" || executeRequest.Passphrase != "correct horse battery" || executeRequest.Side != "sell" || executeRequest.SizeUSDT != 123 {
		t.Fatalf("executeRequest = %#v", executeRequest)
	}
	if !executeRequest.ValidationOnly {
		t.Fatalf("executeRequest.ValidationOnly = false, want true")
	}
	if state.CredentialID != 77 || state.Environment != "demo" || strings.Contains(string(state.LastResult), "correct horse battery") {
		t.Fatalf("state = %#v result=%s", state, string(state.LastResult))
	}
	runs, err := store.ListAutopilotRuns(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListAutopilotRuns() error = %v", err)
	}
	if len(runs) != 1 || runs[0].CredentialID != 77 || runs[0].Status != "completed" {
		t.Fatalf("runs = %#v", runs)
	}
}

func TestLiveStepUsesPlannerForExecutionIntent(t *testing.T) {
	store := openTestStore(t)
	var plannerRequest LivePlanRequest
	var executeRequest liveexec.Request
	service := New(
		store,
		nil,
		func(_ context.Context, request livesync.Request) (livesync.Result, error) {
			return livesync.Result{
				Snapshot: livesync.AccountSnapshot{
					Exchange:    "Binance",
					Environment: "testnet",
					CanTrade:    true,
					Balances: []livesync.Balance{{
						Asset: "USDT",
						Free:  5000,
						Total: 5000,
					}},
					SyncedAt: fixedNow().Format(time.RFC3339),
				},
			}, nil
		},
		func(_ context.Context, request liveexec.Request) (liveexec.Result, error) {
			executeRequest = request
			return liveexec.Result{
				AI:       request.AITrace,
				Decision: risk.Decision{Approved: true},
				Events: []storage.Event{{
					Time:   "10:30:00",
					Type:   "Live Execute",
					Symbol: request.Symbol,
					Result: "validated",
					Level:  "success",
				}},
				Environment: "testnet",
			}, nil
		},
		nil,
	)
	service.WithClock(fixedNow)
	service.WithLivePlanner(func(_ context.Context, request LivePlanRequest) (LivePlan, error) {
		plannerRequest = request
		return LivePlan{
			Intent: core.TradeIntent{
				ID:          "ai-live-plan",
				Mode:        core.ModeLive,
				Exchange:    "Binance",
				Symbol:      "BTCUSDT",
				Side:        core.SideSell,
				OrderType:   core.OrderMarket,
				Price:       67000,
				SizeUSDT:    321,
				Confidence:  0.77,
				MaxSlippage: 0.001,
				Reason:      "Local AI Policy v0.2.0 planned live intent",
				TTL:         2 * time.Minute,
				GeneratedAt: fixedNow(),
			},
			AI: ai.Trace{
				Model:         "Local AI Policy",
				PolicyVersion: "v0.2.0",
				Signal:        "SELL",
				Confidence:    0.77,
				Reason:        "planned live intent",
			},
			Events: []storage.Event{{
				Time:   "10:30:00",
				Type:   "AI Live Plan",
				Symbol: "BTCUSDT",
				Result: "Conf: 77%",
				Level:  "info",
			}},
		}, nil
	})

	state, err := service.Start(context.Background(), Request{
		Operator:        "qa",
		Mode:            "live",
		Exchange:        "Binance",
		Environment:     "testnet",
		Symbol:          "BTCUSDT",
		IntervalSeconds: 5,
		MaxSteps:        1,
		CredentialID:    88,
		Passphrase:      "correct horse battery",
		Side:            "buy",
		SizeUSDT:        50,
		ValidationOnly:  true,
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if plannerRequest.AccountSync.Snapshot.Balances[0].Free != 5000 || plannerRequest.SizeUSDT != 50 {
		t.Fatalf("plannerRequest = %#v", plannerRequest)
	}
	if executeRequest.Side != "sell" || executeRequest.SizeUSDT != 321 {
		t.Fatalf("executeRequest side/size = %#v", executeRequest)
	}
	if executeRequest.PlannedIntent == nil || executeRequest.AITrace == nil || executeRequest.AITrace.PolicyVersion != "v0.2.0" {
		t.Fatalf("executeRequest plan = %#v trace=%#v", executeRequest.PlannedIntent, executeRequest.AITrace)
	}
	resultJSON := string(state.LastResult)
	if !strings.Contains(resultJSON, `"aiPlan"`) || !strings.Contains(resultJSON, "Local AI Policy") {
		t.Fatalf("LastResult missing AI plan: %s", resultJSON)
	}
	if strings.Contains(resultJSON, "correct horse battery") {
		t.Fatalf("LastResult leaked passphrase: %s", resultJSON)
	}
	if len(state.LastEvents) != 2 || state.LastEvents[0].Type != "AI Live Plan" {
		t.Fatalf("events = %#v", state.LastEvents)
	}
}

func TestLiveStartRequiresCredentialAndPassphrase(t *testing.T) {
	service := New(nil, nil, nil, nil, nil)
	_, err := service.Start(context.Background(), Request{Mode: "live"})
	if !errors.Is(err, ErrLiveCredentialNeeded) {
		t.Fatalf("Start() err = %v, want ErrLiveCredentialNeeded", err)
	}
}

func TestStopClearsRunningState(t *testing.T) {
	service := New(nil, func(context.Context, string, string) (simrun.Result, error) {
		return simResult(), nil
	}, nil, nil, nil)
	service.WithClock(fixedNow)
	state, err := service.Start(context.Background(), Request{Mode: "shadow", MaxSteps: 2})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if !state.Running {
		t.Fatalf("expected running state after maxSteps 2: %#v", state)
	}
	stopped := service.Stop(context.Background(), "qa", "manual")
	if stopped.Running || stopped.LastStatus != "stopped" || stopped.NextRunAt != "" {
		t.Fatalf("stopped = %#v", stopped)
	}
}

func openTestStore(t *testing.T) *storage.Store {
	t.Helper()
	store, err := storage.Open(filepath.Join(t.TempDir(), "ccvar-test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	return store
}

func simResult() simrun.Result {
	return simrun.Result{
		Intent: simrun.IntentView{
			ID:          "sim-qa",
			Exchange:    "Binance",
			Symbol:      "BTCUSDT",
			Side:        "buy",
			OrderType:   "market",
			Price:       67000,
			SizeUSDT:    500,
			Confidence:  0.78,
			TTLSeconds:  120,
			GeneratedAt: fixedNow().Format(time.RFC3339),
		},
		Decision: risk.Decision{Approved: true},
		Fill: &core.Fill{
			OrderID: "fill-1",
			Symbol:  "BTCUSDT",
			Side:    core.SideBuy,
			Price:   67000,
		},
		Events: []storage.Event{{
			Time:   "10:30:00",
			Type:   "AI Decision",
			Symbol: "BTCUSDT",
			Result: "Approved",
			Level:  "success",
		}},
	}
}

func fixedNow() time.Time {
	return time.Date(2026, 6, 22, 10, 30, 0, 0, time.UTC)
}

func TestStateJSONDoesNotIncludePassphrase(t *testing.T) {
	state := State{Mode: "live", CredentialID: 1}
	payload, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "passphrase") {
		t.Fatalf("state JSON contains passphrase field: %s", string(payload))
	}
}
