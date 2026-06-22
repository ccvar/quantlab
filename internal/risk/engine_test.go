package risk

import (
	"testing"
	"time"

	"ccvar.com/web3quant/internal/core"
)

func TestEvaluateApprovesSafeShadowIntent(t *testing.T) {
	engine := testEngine()
	decision := engine.Evaluate(validIntent(core.ModeShadow), account(), market())
	if !decision.Approved {
		t.Fatalf("expected approval, got %v", decision.Reasons)
	}
}

func TestEvaluateRejectsLockedLiveIntent(t *testing.T) {
	engine := testEngine()
	decision := engine.Evaluate(validIntent(core.ModeLive), account(), market())
	if decision.Approved {
		t.Fatal("expected live intent to be rejected while live trading is locked")
	}
	if decision.ReasonText() != "live trading is locked" {
		t.Fatalf("unexpected reason: %s", decision.ReasonText())
	}
}

func TestEvaluateRejectsOversizedIntent(t *testing.T) {
	engine := testEngine()
	intent := validIntent(core.ModeShadow)
	intent.SizeUSDT = 2500
	decision := engine.Evaluate(intent, account(), market())
	if decision.Approved {
		t.Fatal("expected oversized intent to be rejected")
	}
}

func TestEvaluateRejectsInsufficientAvailableBalance(t *testing.T) {
	engine := testEngine()
	account := account()
	account.AvailableUSDT = 0
	decision := engine.Evaluate(validIntent(core.ModeShadow), account, market())
	if decision.Approved || decision.ReasonText() != "insufficient available balance" {
		t.Fatalf("decision = %#v", decision)
	}
}

func TestEvaluateRejectsExpiredIntent(t *testing.T) {
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	engine := NewEngine(defaultLimits()).WithClock(func() time.Time { return now })
	intent := validIntent(core.ModeShadow)
	intent.GeneratedAt = now.Add(-2 * time.Minute)
	intent.TTL = time.Minute
	decision := engine.Evaluate(intent, account(), market())
	if decision.Approved {
		t.Fatal("expected expired intent to be rejected")
	}
}

func defaultLimits() Limits {
	return Limits{
		MinConfidence:        0.65,
		MaxOrderUSDT:         1000,
		MaxSymbolExposure:    8000,
		MaxTotalExposure:     12000,
		MaxDailyDrawdownPct:  3,
		MaxConsecutiveLosses: 3,
		MaxSpreadPct:         0.08,
		RequireLiveUnlock:    true,
	}
}

func testEngine() Engine {
	now := time.Date(2026, 6, 21, 12, 0, 30, 0, time.UTC)
	return NewEngine(defaultLimits()).WithClock(func() time.Time { return now })
}

func validIntent(mode core.TradingMode) core.TradeIntent {
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	return core.TradeIntent{
		ID:          "intent-1",
		Mode:        mode,
		Exchange:    "Binance",
		Symbol:      "BTCUSDT",
		Side:        core.SideBuy,
		OrderType:   core.OrderLimit,
		Price:       67200,
		SizeUSDT:    500,
		Confidence:  0.78,
		MaxSlippage: 0.001,
		Reason:      "momentum continuation",
		TTL:         2 * time.Minute,
		GeneratedAt: now,
	}
}

func account() core.AccountState {
	return core.AccountState{
		EquityUSDT:         100000,
		AvailableUSDT:      20000,
		DailyDrawdownPct:   -0.73,
		OpenNotionalUSDT:   5000,
		SymbolExposureUSDT: map[string]float64{"BTCUSDT": 2500},
	}
}

func market() core.MarketSnapshot {
	return core.MarketSnapshot{
		Exchange:      "Binance",
		Symbol:        "BTCUSDT",
		BestBid:       67199,
		BestAsk:       67201,
		Last:          67200,
		SpreadPct:     0.003,
		LiquidityUSDT: 1250000,
		ObservedAt:    time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC),
	}
}
