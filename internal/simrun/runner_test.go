package simrun

import (
	"context"
	"testing"
	"time"

	"ccvar.com/web3quant/internal/core"
	"ccvar.com/web3quant/internal/exchange"
	"ccvar.com/web3quant/internal/risk"
)

func TestStepApprovesAndFillsSimulation(t *testing.T) {
	now := time.Date(2026, 6, 21, 15, 30, 0, 0, time.UTC)
	runner := New(exchange.NewRegistry(simAdapter{}))
	runner.Now = func() time.Time { return now }
	result, err := runner.Step(context.Background(), "SimX", "BTCUSDT")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Decision.Approved {
		t.Fatalf("expected approval: %+v", result.Decision)
	}
	if result.Fill == nil {
		t.Fatal("expected simulated fill")
	}
	if len(result.Events) != 3 {
		t.Fatalf("expected decision, risk, fill events; got %d", len(result.Events))
	}
	if result.Intent.TTLSeconds != 120 {
		t.Fatalf("unexpected ttl: %d", result.Intent.TTLSeconds)
	}
}

func TestStepUsesRiskProviderLimits(t *testing.T) {
	now := time.Date(2026, 6, 21, 15, 30, 0, 0, time.UTC)
	runner := New(exchange.NewRegistry(simAdapter{}))
	runner.Now = func() time.Time { return now }
	runner.RiskProvider = func(context.Context) (risk.Limits, error) {
		return risk.Limits{
			MinConfidence:        0.1,
			MaxOrderUSDT:         50,
			MaxSymbolExposure:    8000,
			MaxTotalExposure:     12000,
			MaxDailyDrawdownPct:  3,
			MaxConsecutiveLosses: 3,
			MaxSpreadPct:         0.08,
			RequireLiveUnlock:    true,
		}, nil
	}

	result, err := runner.Step(context.Background(), "SimX", "BTCUSDT")
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision.Approved {
		t.Fatalf("expected risk rejection, got %#v", result.Decision)
	}
	if result.Decision.ReasonText() != "order size 500.00 exceeds 50.00" {
		t.Fatalf("reason = %q", result.Decision.ReasonText())
	}
	if result.Fill != nil || len(result.Events) != 2 {
		t.Fatalf("result = %#v", result)
	}
}

func TestStepUsesStrategyProviderIntentSettings(t *testing.T) {
	now := time.Date(2026, 6, 21, 15, 30, 0, 0, time.UTC)
	runner := New(exchange.NewRegistry(simAdapter{}))
	runner.Now = func() time.Time { return now }
	runner.StrategyProvider = func(context.Context) (StrategyConfig, error) {
		return StrategyConfig{
			Name:          "QA Reversal",
			Exchange:      "SimX",
			Symbol:        "ETHUSDT",
			Side:          core.SideSell,
			OrderSizeUSDT: 250,
		}, nil
	}

	result, err := runner.Step(context.Background(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Decision.Approved || result.Fill == nil {
		t.Fatalf("result = %#v", result)
	}
	if result.Intent.Symbol != "ETHUSDT" || result.Intent.Side != "sell" || result.Intent.SizeUSDT != 250 {
		t.Fatalf("intent = %#v", result.Intent)
	}
	if result.Events[0].Action != "SELL" || result.Intent.Model != "Local AI Policy" || result.Intent.PolicyVersion != "v0.2.0" {
		t.Fatalf("events=%#v reason=%q", result.Events, result.Intent.Reason)
	}
	if result.AI.Signal != "SELL" || result.AI.Model != "Local AI Policy" || len(result.AI.Features) == 0 {
		t.Fatalf("ai trace = %#v", result.AI)
	}
}

func TestPaperExecutionRecordFromResult(t *testing.T) {
	now := time.Date(2026, 6, 21, 15, 30, 0, 0, time.UTC)
	runner := New(exchange.NewRegistry(simAdapter{}))
	runner.Now = func() time.Time { return now }
	result, err := runner.Step(context.Background(), "SimX", "BTCUSDT")
	if err != nil {
		t.Fatal(err)
	}
	record, err := PaperExecutionRecordFromResult(result, "paper", "autopilot", 9, time.Time{})
	if err != nil {
		t.Fatalf("PaperExecutionRecordFromResult() error = %v", err)
	}
	if record.RunID != 9 || record.Mode != "paper" || record.Source != "autopilot" || record.IntentID != result.Intent.ID {
		t.Fatalf("record = %#v", record)
	}
	if record.RiskStatus != "approved" || record.FillStatus != "filled" || record.FillPrice == 0 || record.FeeUSDT == 0 {
		t.Fatalf("record risk/fill = %#v", record)
	}
	if string(record.FillJSON) == "null" || len(record.EventsJSON) == 0 {
		t.Fatalf("json fields = fill:%s events:%s", string(record.FillJSON), string(record.EventsJSON))
	}
	if record.CreatedAt != now.Format(time.RFC3339) {
		t.Fatalf("CreatedAt = %q, want %q", record.CreatedAt, now.Format(time.RFC3339))
	}
}

type simAdapter struct{}

func (simAdapter) Name() string { return "SimX" }

func (simAdapter) FetchSnapshot(_ context.Context, symbol string) (core.MarketSnapshot, error) {
	return core.MarketSnapshot{
		Exchange:      "SimX",
		Symbol:        symbol,
		BestBid:       67000,
		BestAsk:       67001,
		Last:          67000.5,
		SpreadPct:     0.0015,
		LiquidityUSDT: 2000000,
	}, nil
}

func (simAdapter) FetchCandles(context.Context, string, string, int) ([]core.Candle, error) {
	return nil, nil
}

func (simAdapter) PlaceOrder(context.Context, core.OrderRequest) (core.Fill, error) {
	return core.Fill{}, exchange.ErrTradingDisabled
}

func (simAdapter) CancelOrder(context.Context, string) error {
	return exchange.ErrTradingDisabled
}
