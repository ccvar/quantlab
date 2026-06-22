package ai

import (
	"context"
	"strings"
	"testing"
	"time"

	"ccvar.com/web3quant/internal/core"
)

func TestLocalPolicyGeneratesAuditableIntent(t *testing.T) {
	now := time.Date(2026, 6, 22, 10, 30, 0, 0, time.UTC)
	decision, err := NewLocalPolicy().GenerateIntent(context.Background(), Context{
		Mode:   core.ModeShadow,
		Market: market(),
		Strategy: Strategy{
			Name:          "QA Momentum",
			Side:          core.SideBuy,
			OrderSizeUSDT: 250,
		},
		Candles: risingCandles(),
		Now:     now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Intent.ID == "" || decision.Intent.Mode != core.ModeShadow || decision.Intent.SizeUSDT != 250 {
		t.Fatalf("intent = %#v", decision.Intent)
	}
	if decision.Intent.Confidence < 0.65 {
		t.Fatalf("confidence = %.4f, want risk-eligible", decision.Intent.Confidence)
	}
	if decision.Trace.Model != "Local AI Policy" || decision.Trace.PolicyVersion != "v0.2.0" {
		t.Fatalf("trace identity = %#v", decision.Trace)
	}
	if decision.Trace.Signal != "BUY" || decision.Trace.ExpiresAt != now.Add(2*time.Minute) {
		t.Fatalf("trace = %#v", decision.Trace)
	}
	if len(decision.Trace.Features) != 5 || !strings.Contains(decision.Intent.Reason, "QA Momentum") {
		t.Fatalf("trace features/reason = %#v / %q", decision.Trace.Features, decision.Intent.Reason)
	}
}

func TestLocalPolicyPenalizesMisalignedMomentum(t *testing.T) {
	now := time.Date(2026, 6, 22, 10, 30, 0, 0, time.UTC)
	buy, err := NewLocalPolicy().GenerateIntent(context.Background(), Context{
		Mode:     core.ModeShadow,
		Market:   market(),
		Strategy: Strategy{Name: "Long", Side: core.SideBuy, OrderSizeUSDT: 500},
		Candles:  risingCandles(),
		Now:      now,
	})
	if err != nil {
		t.Fatal(err)
	}
	sell, err := NewLocalPolicy().GenerateIntent(context.Background(), Context{
		Mode:     core.ModeShadow,
		Market:   market(),
		Strategy: Strategy{Name: "Short", Side: core.SideSell, OrderSizeUSDT: 500},
		Candles:  risingCandles(),
		Now:      now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if sell.Intent.Side != core.SideSell || sell.Trace.Signal != "SELL" {
		t.Fatalf("sell decision = %#v", sell)
	}
	if sell.Intent.Confidence >= buy.Intent.Confidence {
		t.Fatalf("sell confidence %.4f should be below buy %.4f", sell.Intent.Confidence, buy.Intent.Confidence)
	}
}

func market() core.MarketSnapshot {
	return core.MarketSnapshot{
		Exchange:      "SimX",
		Symbol:        "BTCUSDT",
		BestBid:       67000,
		BestAsk:       67001,
		Last:          67000.5,
		SpreadPct:     0.0015,
		LiquidityUSDT: 2000000,
		ObservedAt:    time.Date(2026, 6, 22, 10, 29, 55, 0, time.UTC),
	}
}

func risingCandles() []core.Candle {
	return []core.Candle{
		{Time: 1, Close: 66000},
		{Time: 2, Close: 66200},
		{Time: 3, Close: 66400},
		{Time: 4, Close: 66600},
		{Time: 5, Close: 66800},
		{Time: 6, Close: 67000},
	}
}
