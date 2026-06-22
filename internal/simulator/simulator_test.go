package simulator

import (
	"math"
	"testing"
	"time"

	"ccvar.com/web3quant/internal/core"
	"ccvar.com/web3quant/internal/risk"
)

func TestFillAppliesFeeAndSlippage(t *testing.T) {
	now := time.Date(2026, 6, 21, 13, 0, 0, 0, time.UTC)
	model := FillModel{FeeRate: 0.0005, SlippagePct: 0.001, Now: func() time.Time { return now }}
	fill, err := model.Fill(core.TradeIntent{
		ID:        "intent-1",
		Symbol:    "BTCUSDT",
		Side:      core.SideBuy,
		OrderType: core.OrderMarket,
		SizeUSDT:  1000,
	}, core.MarketSnapshot{Last: 67000}, risk.Decision{Approved: true})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(fill.Price-67067) > 0.0001 {
		t.Fatalf("unexpected fill price: %.2f", fill.Price)
	}
	if fill.FeeUSDT != 0.5 {
		t.Fatalf("unexpected fee: %.4f", fill.FeeUSDT)
	}
	if fill.SlippageUSDT != 1 {
		t.Fatalf("unexpected slippage: %.4f", fill.SlippageUSDT)
	}
	if !fill.FilledAt.Equal(now) {
		t.Fatalf("unexpected fill time: %s", fill.FilledAt)
	}
}

func TestFillRefusesRejectedIntent(t *testing.T) {
	model := FillModel{}
	_, err := model.Fill(core.TradeIntent{ID: "intent-1"}, core.MarketSnapshot{}, risk.Decision{Approved: false, Reasons: []string{"daily drawdown limit reached"}})
	if err == nil {
		t.Fatal("expected rejected intent to fail")
	}
}
