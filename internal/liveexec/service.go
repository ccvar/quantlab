package liveexec

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"ccvar.com/web3quant/internal/ai"
	"ccvar.com/web3quant/internal/audit"
	"ccvar.com/web3quant/internal/core"
	"ccvar.com/web3quant/internal/exchange"
	"ccvar.com/web3quant/internal/liveguard"
	"ccvar.com/web3quant/internal/risk"
	"ccvar.com/web3quant/internal/storage"
	"ccvar.com/web3quant/internal/vault"
)

var (
	ErrLiveGuardLocked         = errors.New("live guard is locked")
	ErrCredentialRequired      = errors.New("credential id is required")
	ErrCredentialPassRequired  = errors.New("credential passphrase is required")
	ErrCredentialExchange      = errors.New("credential exchange does not match request")
	ErrTradePermissionRequired = errors.New("credential trade permission is required")
	ErrUnsupportedExchange     = errors.New("unsupported live execution exchange")
	ErrAccountSnapshotRequired = errors.New("recent account snapshot is required before live execution")
	ErrAccountSnapshotStale    = errors.New("account snapshot is stale; sync account before live execution")
	ErrAccountCannotTrade      = errors.New("account snapshot indicates trading is disabled")
	ErrKillSwitchActive        = errors.New("kill switch is active")
)

const maxAccountSnapshotAge = 5 * time.Minute

type Executor interface {
	Execute(context.Context, ExecuteRequest) (ExecutionReport, error)
}

type Service struct {
	Store        *storage.Store
	Registry     exchange.Registry
	Guard        *liveguard.Guard
	Executors    map[string]Executor
	Risk         risk.Engine
	RiskProvider func(context.Context) (risk.Limits, error)
	Halted       func() bool
	Now          func() time.Time
}

type Request struct {
	Operator       string            `json:"operator"`
	CredentialID   int64             `json:"credentialId"`
	Passphrase     string            `json:"passphrase"`
	Exchange       string            `json:"exchange"`
	Symbol         string            `json:"symbol"`
	Side           string            `json:"side"`
	SizeUSDT       float64           `json:"sizeUsdt"`
	ValidationOnly bool              `json:"validationOnly"`
	PlannedIntent  *core.TradeIntent `json:"-"`
	AITrace        *ai.Trace         `json:"-"`
}

type ExecuteRequest struct {
	Environment    string
	ValidationOnly bool
	Intent         core.TradeIntent
	Order          core.OrderRequest
	Market         core.MarketSnapshot
	Credential     vault.PlainCredential
	Now            time.Time
}

type Result struct {
	Intent      IntentView       `json:"intent"`
	AI          *ai.Trace        `json:"ai,omitempty"`
	Decision    risk.Decision    `json:"decision"`
	Execution   *ExecutionReport `json:"execution,omitempty"`
	Events      []storage.Event  `json:"events"`
	Guard       liveguard.State  `json:"guard"`
	Credential  CredentialView   `json:"credential"`
	Environment string           `json:"environment"`
	LedgerID    int64            `json:"ledgerId,omitempty"`
}

type IntentView struct {
	ID            string  `json:"id"`
	Exchange      string  `json:"exchange"`
	Symbol        string  `json:"symbol"`
	Side          string  `json:"side"`
	OrderType     string  `json:"orderType"`
	Price         float64 `json:"price"`
	SizeUSDT      float64 `json:"sizeUsdt"`
	Confidence    float64 `json:"confidence"`
	TTLSeconds    int64   `json:"ttlSeconds"`
	GeneratedAt   string  `json:"generatedAt"`
	Reason        string  `json:"reason"`
	Model         string  `json:"model,omitempty"`
	PolicyVersion string  `json:"policyVersion,omitempty"`
	Signal        string  `json:"signal,omitempty"`
}

type CredentialView struct {
	ID         int64  `json:"id"`
	Exchange   string `json:"exchange"`
	Label      string `json:"label"`
	APIKeyMask string `json:"apiKeyMask"`
}

type ExecutionReport struct {
	Exchange        string         `json:"exchange"`
	Environment     string         `json:"environment"`
	Endpoint        string         `json:"endpoint"`
	ValidationOnly  bool           `json:"validationOnly"`
	ClientOrderID   string         `json:"clientOrderId"`
	ExchangeOrderID string         `json:"exchangeOrderId,omitempty"`
	Status          string         `json:"status"`
	Message         string         `json:"message"`
	SentAt          string         `json:"sentAt"`
	ReceivedAt      string         `json:"receivedAt"`
	Raw             map[string]any `json:"raw,omitempty"`
}

type accountSnapshot struct {
	Exchange     string             `json:"exchange"`
	Environment  string             `json:"environment"`
	AccountType  string             `json:"accountType"`
	CanTrade     bool               `json:"canTrade"`
	Balances     []accountBalance   `json:"balances"`
	OpenOrders   []accountOpenOrder `json:"openOrders"`
	RawUpdatedAt string             `json:"rawUpdatedAt,omitempty"`
	SyncedAt     string             `json:"syncedAt"`
}

type accountBalance struct {
	Asset  string  `json:"asset"`
	Free   float64 `json:"free"`
	Locked float64 `json:"locked"`
	Total  float64 `json:"total"`
	USD    float64 `json:"usd,omitempty"`
}

type accountOpenOrder struct {
	Symbol        string  `json:"symbol"`
	OrderID       string  `json:"orderId"`
	ClientOrderID string  `json:"clientOrderId"`
	Side          string  `json:"side"`
	Type          string  `json:"type"`
	Status        string  `json:"status"`
	Price         float64 `json:"price"`
	OrigQty       float64 `json:"origQty"`
	ExecutedQty   float64 `json:"executedQty"`
	QuoteQty      float64 `json:"quoteQty"`
	UpdatedAt     string  `json:"updatedAt"`
}

func New(store *storage.Store, registry exchange.Registry, guard *liveguard.Guard, executors map[string]Executor) Service {
	return Service{
		Store:     store,
		Registry:  registry,
		Guard:     guard,
		Executors: executors,
		Risk:      risk.NewEngine(defaultLimits(1000)),
		Now:       time.Now,
	}
}

func (service Service) Execute(ctx context.Context, request Request) (Result, error) {
	now := service.clock()
	operator := defaultString(request.Operator, "local")
	guardState := service.guardState()
	if service.isHalted() {
		_ = service.audit(ctx, audit.Record{
			Actor:   operator,
			Action:  "live_execute.kill_switch",
			Entity:  "kill_switch",
			Status:  "rejected",
			Summary: ErrKillSwitchActive.Error(),
			Payload: safeRequestPayload(request, guardState, ErrKillSwitchActive.Error()),
		})
		return Result{Guard: guardState, Environment: guardState.Environment}, ErrKillSwitchActive
	}
	if !guardState.Unlocked {
		_ = service.audit(ctx, audit.Record{
			Actor:   operator,
			Action:  "live_execute.rejected",
			Entity:  "live_execution",
			Status:  "rejected",
			Summary: ErrLiveGuardLocked.Error(),
			Payload: safeRequestPayload(request, guardState, ""),
		})
		return Result{Guard: guardState, Environment: guardState.Environment}, ErrLiveGuardLocked
	}

	if request.CredentialID <= 0 {
		_ = service.audit(ctx, audit.Record{
			Actor:   operator,
			Action:  "live_execute.credential",
			Entity:  "credential",
			Status:  "rejected",
			Summary: ErrCredentialRequired.Error(),
			Payload: safeRequestPayload(request, guardState, ErrCredentialRequired.Error()),
		})
		return Result{Guard: guardState, Environment: guardState.Environment}, ErrCredentialRequired
	}
	if strings.TrimSpace(request.Passphrase) == "" {
		_ = service.audit(ctx, audit.Record{
			Actor:    operator,
			Action:   "live_execute.credential",
			Entity:   "credential",
			EntityID: strconv.FormatInt(request.CredentialID, 10),
			Status:   "rejected",
			Summary:  ErrCredentialPassRequired.Error(),
			Payload:  safeRequestPayload(request, guardState, ErrCredentialPassRequired.Error()),
		})
		return Result{Guard: guardState, Environment: guardState.Environment}, ErrCredentialPassRequired
	}

	encrypted, err := service.Store.GetEncryptedCredential(ctx, request.CredentialID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Result{Guard: guardState, Environment: guardState.Environment}, fmt.Errorf("credential %d not found", request.CredentialID)
		}
		return Result{Guard: guardState, Environment: guardState.Environment}, err
	}
	plain, err := vault.DecryptCredential(encrypted, request.Passphrase)
	if err != nil {
		_ = service.audit(ctx, audit.Record{
			Actor:    operator,
			Action:   "live_execute.credential",
			Entity:   "credential",
			EntityID: strconv.FormatInt(encrypted.ID, 10),
			Status:   "rejected",
			Summary:  "credential decrypt failed",
			Payload: map[string]any{
				"exchange": encrypted.Exchange,
				"label":    encrypted.Label,
			},
		})
		return Result{Guard: guardState, Environment: guardState.Environment, Credential: credentialView(encrypted)}, err
	}

	exchangeName := canonicalExchange(defaultString(request.Exchange, encrypted.Exchange))
	if exchangeName == "" {
		exchangeName = encrypted.Exchange
	}
	if !strings.EqualFold(exchangeName, encrypted.Exchange) {
		_ = service.audit(ctx, audit.Record{
			Actor:    operator,
			Action:   "live_execute.credential",
			Entity:   "credential",
			EntityID: strconv.FormatInt(encrypted.ID, 10),
			Status:   "rejected",
			Summary:  ErrCredentialExchange.Error(),
			Payload: map[string]any{
				"requestExchange":    exchangeName,
				"credentialExchange": encrypted.Exchange,
				"credentialId":       encrypted.ID,
				"environment":        guardState.Environment,
			},
		})
		return Result{Guard: guardState, Environment: guardState.Environment, Credential: credentialView(encrypted)}, ErrCredentialExchange
	}
	if !encrypted.Permissions.Trade {
		_ = service.audit(ctx, audit.Record{
			Actor:    operator,
			Action:   "live_execute.credential",
			Entity:   "credential",
			EntityID: strconv.FormatInt(encrypted.ID, 10),
			Status:   "rejected",
			Summary:  ErrTradePermissionRequired.Error(),
			Payload: map[string]any{
				"exchange":     encrypted.Exchange,
				"credentialId": encrypted.ID,
				"environment":  guardState.Environment,
			},
		})
		return Result{Guard: guardState, Environment: guardState.Environment, Credential: credentialView(encrypted)}, ErrTradePermissionRequired
	}

	adapter, ok := service.Registry.Get(exchangeName)
	if !ok {
		return Result{Guard: guardState, Environment: guardState.Environment, Credential: credentialView(encrypted)}, ErrUnsupportedExchange
	}
	executor, ok := service.Executors[exchangeName]
	if !ok {
		return Result{Guard: guardState, Environment: guardState.Environment, Credential: credentialView(encrypted)}, ErrUnsupportedExchange
	}

	symbol := defaultString(request.Symbol, "BTCUSDT")
	market, err := adapter.FetchSnapshot(ctx, symbol)
	if err != nil {
		return Result{Guard: guardState, Environment: guardState.Environment, Credential: credentialView(encrypted)}, err
	}
	intent := buildIntent(now, market, request, guardState)
	_ = service.audit(ctx, audit.Record{
		Actor:    operator,
		Action:   "live_execute.signal",
		Entity:   "trade_intent",
		EntityID: intent.ID,
		Status:   "generated",
		Summary:  intent.Reason,
		Payload:  intentPayload(intent, guardState, request.ValidationOnly, request.AITrace),
	})

	account, snapshotRecord, err := service.accountFromSnapshot(ctx, encrypted.ID, exchangeName, guardState.Environment, symbol, intent, market, guardState, now)
	if err != nil {
		_ = service.audit(ctx, audit.Record{
			Actor:    operator,
			Action:   "live_execute.account",
			Entity:   "account_snapshot",
			EntityID: strconv.FormatInt(encrypted.ID, 10),
			Status:   "rejected",
			Summary:  err.Error(),
			Payload:  accountSnapshotPayload(request, guardState, snapshotRecord, err),
		})
		return Result{
			Intent:      viewIntent(intent, request.AITrace),
			AI:          request.AITrace,
			Guard:       guardState,
			Credential:  credentialView(encrypted),
			Environment: guardState.Environment,
		}, err
	}
	riskEngine, err := service.riskForGuard(ctx, guardState)
	if err != nil {
		return Result{
			Intent:      viewIntent(intent, request.AITrace),
			AI:          request.AITrace,
			Guard:       guardState,
			Credential:  credentialView(encrypted),
			Environment: guardState.Environment,
		}, err
	}
	decision := riskEngine.WithClock(func() time.Time { return now }).Evaluate(intent, account, market)
	events := executionEvents(now, intent, decision, nil)
	riskStatus := "approved"
	if !decision.Approved {
		riskStatus = "rejected"
	}
	_ = service.audit(ctx, audit.Record{
		Actor:    operator,
		Action:   "live_execute.risk",
		Entity:   "trade_intent",
		EntityID: intent.ID,
		Status:   riskStatus,
		Summary:  decision.ReasonText(),
		Payload: map[string]any{
			"exchange":       intent.Exchange,
			"symbol":         intent.Symbol,
			"side":           intent.Side,
			"sizeUsdt":       intent.SizeUSDT,
			"environment":    guardState.Environment,
			"validationOnly": request.ValidationOnly,
			"reasons":        decision.Reasons,
		},
	})
	result := Result{
		Intent:      viewIntent(intent, request.AITrace),
		AI:          request.AITrace,
		Decision:    decision,
		Events:      events,
		Guard:       guardState,
		Credential:  credentialView(encrypted),
		Environment: guardState.Environment,
	}
	if !decision.Approved {
		record, ledgerErr := service.recordExecution(ctx, executionRecordInput{
			Now:             now,
			Intent:          intent,
			AITrace:         request.AITrace,
			CredentialID:    encrypted.ID,
			Environment:     guardState.Environment,
			ValidationOnly:  request.ValidationOnly,
			Decision:        decision,
			ExecutionStatus: "not_submitted",
		})
		if ledgerErr == nil {
			result.LedgerID = record.ID
		}
		return result, nil
	}

	order := core.OrderRequest{
		ClientOrderID: intent.ID,
		Exchange:      intent.Exchange,
		Symbol:        intent.Symbol,
		Side:          intent.Side,
		OrderType:     intent.OrderType,
		Price:         intent.Price,
		SizeUSDT:      intent.SizeUSDT,
	}
	report, err := executor.Execute(ctx, ExecuteRequest{
		Environment:    guardState.Environment,
		ValidationOnly: request.ValidationOnly,
		Intent:         intent,
		Order:          order,
		Market:         market,
		Credential:     plain,
		Now:            now,
	})
	if err != nil {
		_ = service.audit(ctx, audit.Record{
			Actor:    operator,
			Action:   "live_execute.order",
			Entity:   "order",
			EntityID: order.ClientOrderID,
			Status:   "failed",
			Summary:  err.Error(),
			Payload:  orderPayload(order, guardState, request.ValidationOnly, nil),
		})
		failedReport := ExecutionReport{Status: "failed", Message: err.Error()}
		record, ledgerErr := service.recordExecution(ctx, executionRecordInput{
			Now:             now,
			Intent:          intent,
			AITrace:         request.AITrace,
			CredentialID:    encrypted.ID,
			Environment:     guardState.Environment,
			ValidationOnly:  request.ValidationOnly,
			Decision:        decision,
			Report:          &failedReport,
			ExecutionStatus: "failed",
		})
		if ledgerErr == nil {
			result.LedgerID = record.ID
		}
		result.Events = executionEvents(now, intent, decision, &failedReport)
		return result, err
	}
	_ = service.audit(ctx, audit.Record{
		Actor:    operator,
		Action:   "live_execute.order",
		Entity:   "order",
		EntityID: order.ClientOrderID,
		Status:   report.Status,
		Summary:  report.Message,
		Payload:  orderPayload(order, guardState, request.ValidationOnly, &report),
	})
	record, ledgerErr := service.recordExecution(ctx, executionRecordInput{
		Now:             now,
		Intent:          intent,
		AITrace:         request.AITrace,
		CredentialID:    encrypted.ID,
		Environment:     guardState.Environment,
		ValidationOnly:  request.ValidationOnly,
		Decision:        decision,
		Report:          &report,
		ExecutionStatus: report.Status,
	})
	if ledgerErr == nil {
		result.LedgerID = record.ID
	}
	result.Execution = &report
	result.Events = executionEvents(now, intent, decision, &report)
	return result, nil
}

type executionRecordInput struct {
	Now             time.Time
	Intent          core.TradeIntent
	AITrace         *ai.Trace
	CredentialID    int64
	Environment     string
	ValidationOnly  bool
	Decision        risk.Decision
	Report          *ExecutionReport
	ExecutionStatus string
}

func (service Service) recordExecution(ctx context.Context, input executionRecordInput) (storage.LiveExecutionRecord, error) {
	if service.Store == nil {
		return storage.LiveExecutionRecord{}, nil
	}
	intentView := viewIntent(input.Intent, input.AITrace)
	intentJSON, err := json.Marshal(intentView)
	if err != nil {
		return storage.LiveExecutionRecord{}, err
	}
	decisionJSON, err := json.Marshal(input.Decision)
	if err != nil {
		return storage.LiveExecutionRecord{}, err
	}
	var executionJSON []byte
	exchangeOrderID := ""
	if input.Report != nil {
		executionJSON, err = json.Marshal(input.Report)
		if err != nil {
			return storage.LiveExecutionRecord{}, err
		}
		exchangeOrderID = input.Report.ExchangeOrderID
	}
	riskStatus := "approved"
	if !input.Decision.Approved {
		riskStatus = "rejected"
	}
	now := input.Now.UTC().Format(time.RFC3339)
	return service.Store.SaveLiveExecution(ctx, storage.LiveExecutionRecord{
		IntentID:        input.Intent.ID,
		ClientOrderID:   input.Intent.ID,
		ExchangeOrderID: exchangeOrderID,
		CredentialID:    input.CredentialID,
		Exchange:        input.Intent.Exchange,
		Environment:     input.Environment,
		Symbol:          input.Intent.Symbol,
		Side:            string(input.Intent.Side),
		SizeUSDT:        input.Intent.SizeUSDT,
		ValidationOnly:  input.ValidationOnly,
		RiskStatus:      riskStatus,
		ExecutionStatus: defaultString(input.ExecutionStatus, "unknown"),
		IntentJSON:      intentJSON,
		DecisionJSON:    decisionJSON,
		ExecutionJSON:   executionJSON,
		CreatedAt:       now,
		UpdatedAt:       now,
	})
}

func (service Service) accountFromSnapshot(
	ctx context.Context,
	credentialID int64,
	exchangeName string,
	environment string,
	filterSymbol string,
	intent core.TradeIntent,
	market core.MarketSnapshot,
	guard liveguard.State,
	now time.Time,
) (core.AccountState, storage.AccountSnapshotRecord, error) {
	if service.Store == nil {
		return core.AccountState{}, storage.AccountSnapshotRecord{}, ErrAccountSnapshotRequired
	}
	record, err := service.Store.LatestAccountSnapshot(ctx, storage.AccountSnapshotFilter{
		CredentialID: credentialID,
		Exchange:     exchangeName,
		Environment:  environment,
		Symbol:       strings.ToUpper(strings.TrimSpace(filterSymbol)),
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return core.AccountState{}, storage.AccountSnapshotRecord{}, ErrAccountSnapshotRequired
		}
		return core.AccountState{}, storage.AccountSnapshotRecord{}, err
	}
	var snapshot accountSnapshot
	if err := json.Unmarshal(record.SnapshotJSON, &snapshot); err != nil {
		return core.AccountState{}, record, err
	}
	if !snapshot.CanTrade {
		return core.AccountState{}, record, ErrAccountCannotTrade
	}
	syncedAt := parseSnapshotTime(snapshot.SyncedAt, record.CreatedAt)
	if syncedAt.IsZero() || now.Sub(syncedAt) > maxAccountSnapshotAge {
		return core.AccountState{}, record, ErrAccountSnapshotStale
	}
	return accountStateFromSnapshot(snapshot, intent, market, guard), record, nil
}

func accountStateFromSnapshot(snapshot accountSnapshot, intent core.TradeIntent, market core.MarketSnapshot, guard liveguard.State) core.AccountState {
	baseAsset, quoteAsset := splitSymbolAssets(market.Symbol)
	availableUSDT := freeBalance(snapshot.Balances, quoteAsset)
	if intent.Side == core.SideSell {
		availableUSDT = freeBalance(snapshot.Balances, baseAsset) * positivePrice(market.Last)
	}
	return core.AccountState{
		EquityUSDT:          equityUSDT(snapshot.Balances, baseAsset, market.Last),
		AvailableUSDT:       availableUSDT,
		DailyDrawdownPct:    0,
		OpenNotionalUSDT:    openOrderNotional(snapshot.OpenOrders, market),
		SymbolExposureUSDT:  map[string]float64{intent.Symbol: symbolExposureUSDT(snapshot, baseAsset, intent.Symbol, market)},
		LiveTradingUnlocked: guard.Unlocked,
		ConsecutiveLosses:   0,
	}
}

func equityUSDT(balances []accountBalance, baseAsset string, lastPrice float64) float64 {
	total := 0.0
	price := positivePrice(lastPrice)
	for _, balance := range balances {
		asset := normalizedAsset(balance.Asset)
		switch {
		case balance.USD > 0:
			total += balance.USD
		case isUSDLike(asset):
			total += balance.Total
		case asset == baseAsset:
			total += balance.Total * price
		}
	}
	return total
}

func symbolExposureUSDT(snapshot accountSnapshot, baseAsset string, symbol string, market core.MarketSnapshot) float64 {
	price := positivePrice(market.Last)
	exposure := totalBalance(snapshot.Balances, baseAsset) * price
	for _, order := range snapshot.OpenOrders {
		if symbolsEqual(order.Symbol, symbol) {
			exposure += orderNotional(order, market)
		}
	}
	return exposure
}

func openOrderNotional(orders []accountOpenOrder, market core.MarketSnapshot) float64 {
	total := 0.0
	for _, order := range orders {
		total += orderNotional(order, market)
	}
	return total
}

func orderNotional(order accountOpenOrder, market core.MarketSnapshot) float64 {
	remaining := order.OrigQty - order.ExecutedQty
	if remaining < 0 {
		remaining = 0
	}
	price := order.Price
	if price <= 0 {
		price = market.Last
	}
	notional := remaining * price
	if notional <= 0 && order.QuoteQty > 0 {
		return order.QuoteQty
	}
	return notional
}

func splitSymbolAssets(symbol string) (string, string) {
	normalized := normalizedSymbol(symbol)
	for _, quote := range []string{"USDT", "USDC", "USD", "BTC", "ETH"} {
		if strings.HasSuffix(normalized, quote) && len(normalized) > len(quote) {
			return normalized[:len(normalized)-len(quote)], quote
		}
	}
	return normalized, "USDT"
}

func freeBalance(balances []accountBalance, asset string) float64 {
	asset = normalizedAsset(asset)
	for _, balance := range balances {
		if normalizedAsset(balance.Asset) == asset {
			return balance.Free
		}
	}
	return 0
}

func totalBalance(balances []accountBalance, asset string) float64 {
	asset = normalizedAsset(asset)
	for _, balance := range balances {
		if normalizedAsset(balance.Asset) == asset {
			return balance.Total
		}
	}
	return 0
}

func normalizedAsset(asset string) string {
	return strings.ToUpper(strings.TrimSpace(asset))
}

func normalizedSymbol(symbol string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(symbol), "-", ""))
}

func symbolsEqual(a, b string) bool {
	return normalizedSymbol(a) == normalizedSymbol(b)
}

func isUSDLike(asset string) bool {
	return asset == "USDT" || asset == "USDC" || asset == "USD"
}

func positivePrice(price float64) float64 {
	if price > 0 {
		return price
	}
	return 0
}

func parseSnapshotTime(values ...string) time.Time {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		parsed, err := time.Parse(time.RFC3339, value)
		if err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}

func (service Service) isHalted() bool {
	if service.Halted == nil {
		return false
	}
	return service.Halted()
}

func (service Service) guardState() liveguard.State {
	if service.Guard == nil {
		return liveguard.State{Unlocked: false, Message: "live trading locked"}
	}
	return service.Guard.State()
}

func (service Service) riskForGuard(ctx context.Context, state liveguard.State) (risk.Engine, error) {
	limits := defaultLimits(state.MaxOrderUSDT)
	if service.RiskProvider != nil {
		profileLimits, err := service.RiskProvider(ctx)
		if err != nil {
			return risk.Engine{}, err
		}
		limits = profileLimits
	}
	if state.MaxOrderUSDT > 0 && (limits.MaxOrderUSDT <= 0 || state.MaxOrderUSDT < limits.MaxOrderUSDT) {
		limits.MaxOrderUSDT = state.MaxOrderUSDT
	}
	if limits.MaxOrderUSDT <= 0 {
		limits.MaxOrderUSDT = 1000
	}
	return risk.NewEngine(limits), nil
}

func (service Service) clock() time.Time {
	if service.Now == nil {
		return time.Now().UTC()
	}
	return service.Now().UTC()
}

func (service Service) audit(ctx context.Context, record audit.Record) error {
	if service.Store == nil {
		return nil
	}
	_, err := service.Store.AppendAudit(ctx, record)
	return err
}

func buildIntent(now time.Time, market core.MarketSnapshot, request Request, guard liveguard.State) core.TradeIntent {
	size := request.SizeUSDT
	if size <= 0 {
		size = minPositive(500, guard.MaxOrderUSDT)
	}
	side := core.Side(strings.ToLower(strings.TrimSpace(request.Side)))
	if side != core.SideSell {
		side = core.SideBuy
	}
	confidence := confidenceFromMarket(market)
	reason := "AI live intent for guarded testnet/demo validation."
	ttl := 2 * time.Minute
	if request.PlannedIntent != nil {
		planned := *request.PlannedIntent
		if planned.Side == core.SideSell {
			side = core.SideSell
		} else {
			side = core.SideBuy
		}
		if planned.SizeUSDT > 0 {
			size = planned.SizeUSDT
		}
		if planned.Confidence > 0 {
			confidence = planned.Confidence
		}
		if planned.Reason != "" {
			reason = planned.Reason
		}
		if planned.TTL > 0 {
			ttl = planned.TTL
		}
	}
	return core.TradeIntent{
		ID:          "CCVAR" + strings.ToUpper(strconv.FormatInt(now.UnixNano(), 36)),
		Mode:        core.ModeLive,
		Exchange:    market.Exchange,
		Symbol:      market.Symbol,
		Side:        side,
		OrderType:   core.OrderMarket,
		Price:       market.Last,
		SizeUSDT:    size,
		Confidence:  confidence,
		MaxSlippage: 0.001,
		Reason:      reason,
		TTL:         ttl,
		GeneratedAt: now,
	}
}

func defaultLimits(maxOrderUSDT float64) risk.Limits {
	return risk.Limits{
		MinConfidence:        0.65,
		MaxOrderUSDT:         maxOrderUSDT,
		MaxSymbolExposure:    8000,
		MaxTotalExposure:     12000,
		MaxDailyDrawdownPct:  3,
		MaxConsecutiveLosses: 3,
		MaxSpreadPct:         0.08,
		RequireLiveUnlock:    true,
	}
}

func confidenceFromMarket(snapshot core.MarketSnapshot) float64 {
	confidence := 0.74
	if snapshot.SpreadPct <= 0.01 {
		confidence += 0.04
	}
	if snapshot.LiquidityUSDT > 1000000 {
		confidence += 0.03
	}
	if confidence > 0.88 {
		return 0.88
	}
	return confidence
}

func executionEvents(now time.Time, intent core.TradeIntent, decision risk.Decision, report *ExecutionReport) []storage.Event {
	note := "guarded testnet/demo"
	if strings.Contains(intent.Reason, "Local AI Policy") {
		note = "Local AI Policy / guarded testnet-demo"
	}
	events := []storage.Event{
		{
			Time:   now.Format("15:04:05"),
			Type:   "AI Live Intent",
			Symbol: intent.Symbol,
			Action: strings.ToUpper(string(intent.Side)),
			Price:  intent.Price,
			Result: fmt.Sprintf("Conf: %.0f%%", intent.Confidence*100),
			Note:   note,
			Level:  "info",
		},
		{
			Time:   now.Format("15:04:05"),
			Type:   "Risk Check",
			Symbol: intent.Symbol,
			Action: "-",
			Result: riskResult(decision),
			Note:   decision.ReasonText(),
			Level:  riskLevel(decision),
		},
	}
	if report != nil {
		level := "success"
		if report.Status == "failed" {
			level = "danger"
		}
		events = append(events, storage.Event{
			Time:   now.Format("15:04:05"),
			Type:   "Live Execute",
			Symbol: intent.Symbol,
			Action: strings.ToUpper(string(intent.Side)),
			Price:  intent.Price,
			Result: report.Status,
			Note:   report.Message,
			Level:  level,
		})
	}
	return events
}

func riskResult(decision risk.Decision) string {
	if decision.Approved {
		return "Approved"
	}
	return "Rejected"
}

func riskLevel(decision risk.Decision) string {
	if decision.Approved {
		return "success"
	}
	return "danger"
}

func viewIntent(intent core.TradeIntent, traces ...*ai.Trace) IntentView {
	view := IntentView{
		ID:          intent.ID,
		Exchange:    intent.Exchange,
		Symbol:      intent.Symbol,
		Side:        string(intent.Side),
		OrderType:   string(intent.OrderType),
		Price:       intent.Price,
		SizeUSDT:    intent.SizeUSDT,
		Confidence:  intent.Confidence,
		TTLSeconds:  int64(intent.TTL.Seconds()),
		GeneratedAt: intent.GeneratedAt.Format(time.RFC3339),
		Reason:      intent.Reason,
	}
	if len(traces) > 0 && traces[0] != nil {
		view.Model = traces[0].Model
		view.PolicyVersion = traces[0].PolicyVersion
		view.Signal = traces[0].Signal
	}
	return view
}

func credentialView(credential vault.EncryptedCredential) CredentialView {
	return CredentialView{
		ID:         credential.ID,
		Exchange:   credential.Exchange,
		Label:      credential.Label,
		APIKeyMask: credential.APIKeyMask,
	}
}

func intentPayload(intent core.TradeIntent, guard liveguard.State, validationOnly bool, trace *ai.Trace) map[string]any {
	payload := map[string]any{
		"id":             intent.ID,
		"exchange":       intent.Exchange,
		"symbol":         intent.Symbol,
		"side":           intent.Side,
		"orderType":      intent.OrderType,
		"price":          intent.Price,
		"sizeUsdt":       intent.SizeUSDT,
		"confidence":     intent.Confidence,
		"environment":    guard.Environment,
		"validationOnly": validationOnly,
	}
	if trace != nil {
		payload["ai"] = map[string]any{
			"model":         trace.Model,
			"policyVersion": trace.PolicyVersion,
			"signal":        trace.Signal,
			"confidence":    trace.Confidence,
		}
	}
	return payload
}

func orderPayload(order core.OrderRequest, guard liveguard.State, validationOnly bool, report *ExecutionReport) map[string]any {
	payload := map[string]any{
		"clientOrderId":  order.ClientOrderID,
		"exchange":       order.Exchange,
		"symbol":         order.Symbol,
		"side":           order.Side,
		"orderType":      order.OrderType,
		"price":          order.Price,
		"sizeUsdt":       order.SizeUSDT,
		"environment":    guard.Environment,
		"validationOnly": validationOnly,
	}
	if report != nil {
		payload["endpoint"] = report.Endpoint
		payload["exchangeOrderId"] = report.ExchangeOrderID
		payload["status"] = report.Status
	}
	return payload
}

func accountSnapshotPayload(request Request, guard liveguard.State, record storage.AccountSnapshotRecord, err error) map[string]any {
	payload := safeRequestPayload(request, guard, "")
	payload["error"] = err.Error()
	payload["snapshotId"] = record.ID
	payload["snapshotCreatedAt"] = record.CreatedAt
	payload["maxAgeSeconds"] = int64(maxAccountSnapshotAge.Seconds())
	return payload
}

func safeRequestPayload(request Request, guard liveguard.State, errorText string) map[string]any {
	return map[string]any{
		"exchange":       request.Exchange,
		"symbol":         request.Symbol,
		"side":           request.Side,
		"sizeUsdt":       request.SizeUSDT,
		"credentialId":   request.CredentialID,
		"environment":    guard.Environment,
		"validationOnly": request.ValidationOnly,
		"error":          errorText,
	}
}

func canonicalExchange(value string) string {
	switch {
	case strings.EqualFold(strings.TrimSpace(value), "Binance"):
		return "Binance"
	case strings.EqualFold(strings.TrimSpace(value), "OKX"):
		return "OKX"
	default:
		return strings.TrimSpace(value)
	}
}

func minPositive(a, b float64) float64 {
	if a <= 0 {
		return b
	}
	if b <= 0 || a < b {
		return a
	}
	return b
}

func defaultString(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
