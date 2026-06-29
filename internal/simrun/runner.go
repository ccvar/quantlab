package simrun

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"ccvar.com/web3quant/internal/ai"
	"ccvar.com/web3quant/internal/core"
	"ccvar.com/web3quant/internal/exchange"
	"ccvar.com/web3quant/internal/risk"
	"ccvar.com/web3quant/internal/simulator"
	"ccvar.com/web3quant/internal/storage"
)

type Runner struct {
	Registry         exchange.Registry
	Risk             risk.Engine
	RiskProvider     func(context.Context) (risk.Limits, error)
	AccountProvider  func(context.Context, core.MarketSnapshot) (core.AccountState, error)
	Strategy         StrategyConfig
	StrategyProvider func(context.Context) (StrategyConfig, error)
	AI               ai.Engine
	Fill             simulator.FillModel
	Now              func() time.Time
}

type StrategyConfig struct {
	Name            string
	Exchange        string
	Symbol          string
	Side            core.Side
	OrderSizeUSDT   float64
	IntervalSeconds int
	MaxSteps        int
}

type Result struct {
	Intent   IntentView      `json:"intent"`
	AI       ai.Trace        `json:"ai"`
	Decision risk.Decision   `json:"decision"`
	Fill     *core.Fill      `json:"fill,omitempty"`
	Events   []storage.Event `json:"events"`
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

func New(registry exchange.Registry) Runner {
	return Runner{
		Registry: registry,
		Risk: risk.NewEngine(risk.Limits{
			MinConfidence:        0.65,
			MaxOrderUSDT:         1000,
			MaxSymbolExposure:    8000,
			MaxTotalExposure:     12000,
			MaxDailyDrawdownPct:  3,
			MaxConsecutiveLosses: 3,
			MaxSpreadPct:         0.08,
			RequireLiveUnlock:    true,
		}),
		Fill: simulator.FillModel{
			FeeRate:     0.0005,
			SlippagePct: 0.0002,
		},
		Strategy: StrategyConfig{
			Name:            "AI Momentum Pro",
			Exchange:        "Binance",
			Symbol:          "BTCUSDT",
			Side:            core.SideBuy,
			OrderSizeUSDT:   500,
			IntervalSeconds: 15,
		},
		AI:  ai.NewLocalPolicy(),
		Now: time.Now,
	}
}

func (runner Runner) Step(ctx context.Context, exchangeName, symbol string) (Result, error) {
	strategy, err := runner.strategy(ctx)
	if err != nil {
		return Result{}, err
	}
	if exchangeName == "" {
		exchangeName = strategy.Exchange
	}
	if symbol == "" {
		symbol = strategy.Symbol
	}
	adapter, ok := runner.Registry.Get(exchangeName)
	if !ok {
		return Result{}, fmt.Errorf("unknown exchange %q", exchangeName)
	}
	now := runner.clock()
	snapshot, err := adapter.FetchSnapshot(ctx, symbol)
	if err != nil {
		return Result{}, err
	}
	account, err := runner.account(ctx, snapshot)
	if err != nil {
		return Result{}, err
	}
	aiDecision, err := runner.aiEngine().GenerateIntent(ctx, ai.Context{
		Account: account,
		Market:  snapshot,
		Mode:    core.ModeShadow,
		Strategy: ai.Strategy{
			Name:          strategy.Name,
			Side:          strategy.Side,
			OrderSizeUSDT: strategy.OrderSizeUSDT,
		},
		Now: now,
	})
	if err != nil {
		return Result{}, err
	}
	intent := aiDecision.Intent
	trace := aiDecision.Trace
	riskEngine, err := runner.riskEngine(ctx)
	if err != nil {
		return Result{}, err
	}
	decision := riskEngine.WithClock(func() time.Time { return now }).Evaluate(intent, account, snapshot)
	events := []storage.Event{
		{
			Time:   now.Format("15:04:05"),
			Type:   "AI Decision",
			Symbol: intent.Symbol,
			Action: stringUpper(intent.Side),
			Price:  intent.Price,
			Result: fmt.Sprintf("Conf: %.0f%%", intent.Confidence*100),
			Note:   fmt.Sprintf("%s %s / TTL: %s", trace.Model, trace.PolicyVersion, mmss(trace.TTL)),
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
	if !decision.Approved {
		return Result{Intent: viewIntent(intent, trace), AI: trace, Decision: decision, Events: events}, nil
	}
	fillModel := runner.Fill
	fillModel.Now = func() time.Time { return now }
	fill, err := fillModel.Fill(intent, snapshot, decision)
	if err != nil {
		return Result{}, err
	}
	events = append(events, storage.Event{
		Time:   now.Format("15:04:05"),
		Type:   "Sim Fill",
		Symbol: intent.Symbol,
		Action: stringUpper(intent.Side),
		Price:  fill.Price,
		Result: "Simulated",
		Note:   fmt.Sprintf("fee %.4f USDT / slippage %.4f USDT", fill.FeeUSDT, fill.SlippageUSDT),
		Level:  "success",
	})
	return Result{Intent: viewIntent(intent, trace), AI: trace, Decision: decision, Fill: &fill, Events: events}, nil
}

func (runner Runner) clock() time.Time {
	if runner.Now == nil {
		return time.Now().UTC()
	}
	return runner.Now().UTC()
}

func (runner Runner) riskEngine(ctx context.Context) (risk.Engine, error) {
	if runner.RiskProvider == nil {
		return runner.Risk, nil
	}
	limits, err := runner.RiskProvider(ctx)
	if err != nil {
		return risk.Engine{}, err
	}
	return risk.NewEngine(limits), nil
}

func (runner Runner) account(ctx context.Context, snapshot core.MarketSnapshot) (core.AccountState, error) {
	if runner.AccountProvider != nil {
		account, err := runner.AccountProvider(ctx, snapshot)
		if err != nil {
			return core.AccountState{}, err
		}
		if account.SymbolExposureUSDT == nil {
			account.SymbolExposureUSDT = map[string]float64{}
		}
		return account, nil
	}
	return core.AccountState{
		EquityUSDT:         100000,
		AvailableUSDT:      100000,
		DailyDrawdownPct:   0,
		OpenNotionalUSDT:   0,
		SymbolExposureUSDT: map[string]float64{},
	}, nil
}

func (runner Runner) aiEngine() ai.Engine {
	if runner.AI == nil {
		return ai.NewLocalPolicy()
	}
	return runner.AI
}

func (runner Runner) strategy(ctx context.Context) (StrategyConfig, error) {
	config := runner.Strategy
	if runner.StrategyProvider != nil {
		provided, err := runner.StrategyProvider(ctx)
		if err != nil {
			return StrategyConfig{}, err
		}
		config = provided
	}
	if config.Exchange == "" {
		config.Exchange = "Binance"
	}
	if config.Symbol == "" {
		config.Symbol = "BTCUSDT"
	}
	if config.Side != core.SideSell {
		config.Side = core.SideBuy
	}
	if config.OrderSizeUSDT <= 0 {
		config.OrderSizeUSDT = 500
	}
	if config.Name == "" {
		config.Name = "AI Momentum Pro"
	}
	return config, nil
}

func stringUpper(value core.Side) string {
	if value == core.SideSell {
		return "SELL"
	}
	return "BUY"
}

func confidenceFromMarket(snapshot core.MarketSnapshot) float64 {
	confidence := 0.72
	if snapshot.SpreadPct <= 0.01 {
		confidence += 0.04
	}
	if snapshot.LiquidityUSDT > 1000000 {
		confidence += 0.03
	}
	if confidence > 0.86 {
		return 0.86
	}
	return confidence
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

func viewIntent(intent core.TradeIntent, trace ai.Trace) IntentView {
	return IntentView{
		ID:            intent.ID,
		Exchange:      intent.Exchange,
		Symbol:        intent.Symbol,
		Side:          string(intent.Side),
		OrderType:     string(intent.OrderType),
		Price:         intent.Price,
		SizeUSDT:      intent.SizeUSDT,
		Confidence:    intent.Confidence,
		TTLSeconds:    int64(intent.TTL.Seconds()),
		GeneratedAt:   intent.GeneratedAt.Format(time.RFC3339),
		Reason:        intent.Reason,
		Model:         trace.Model,
		PolicyVersion: trace.PolicyVersion,
		Signal:        trace.Signal,
	}
}

func mmss(duration time.Duration) string {
	if duration <= 0 {
		return "00:00"
	}
	total := int(duration.Round(time.Second).Seconds())
	return fmt.Sprintf("%02d:%02d", total/60, total%60)
}

func PaperExecutionRecordFromResult(result Result, mode, source string, runID int64, createdAt time.Time) (storage.PaperExecutionRecord, error) {
	intentJSON, err := json.Marshal(result.Intent)
	if err != nil {
		return storage.PaperExecutionRecord{}, err
	}
	decisionJSON, err := json.Marshal(result.Decision)
	if err != nil {
		return storage.PaperExecutionRecord{}, err
	}
	eventsJSON, err := json.Marshal(result.Events)
	if err != nil {
		return storage.PaperExecutionRecord{}, err
	}
	fillStatus := "not_filled"
	fillJSON := []byte(`null`)
	var fillPrice float64
	var feeUSDT float64
	var slippageUSDT float64
	if result.Fill != nil {
		fillStatus = "filled"
		fillPayload, err := json.Marshal(result.Fill)
		if err != nil {
			return storage.PaperExecutionRecord{}, err
		}
		fillJSON = fillPayload
		fillPrice = result.Fill.Price
		feeUSDT = result.Fill.FeeUSDT
		slippageUSDT = result.Fill.SlippageUSDT
	}
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	if result.Intent.GeneratedAt != "" {
		if parsed, err := time.Parse(time.RFC3339, result.Intent.GeneratedAt); err == nil {
			createdAt = parsed.UTC()
		}
	}
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode != "paper" {
		mode = "shadow"
	}
	source = strings.TrimSpace(source)
	if source == "" {
		source = "manual"
	}
	riskStatus := "rejected"
	if result.Decision.Approved {
		riskStatus = "approved"
	}
	return storage.PaperExecutionRecord{
		RunID:        runID,
		Mode:         mode,
		Source:       source,
		IntentID:     result.Intent.ID,
		Exchange:     result.Intent.Exchange,
		Symbol:       result.Intent.Symbol,
		Side:         result.Intent.Side,
		SizeUSDT:     result.Intent.SizeUSDT,
		IntentPrice:  result.Intent.Price,
		Confidence:   result.Intent.Confidence,
		RiskStatus:   riskStatus,
		FillStatus:   fillStatus,
		FillPrice:    fillPrice,
		FeeUSDT:      feeUSDT,
		SlippageUSDT: slippageUSDT,
		IntentJSON:   intentJSON,
		DecisionJSON: decisionJSON,
		FillJSON:     fillJSON,
		EventsJSON:   eventsJSON,
		CreatedAt:    createdAt.Format(time.RFC3339),
	}, nil
}
