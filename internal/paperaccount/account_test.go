package paperaccount

import (
	"math"
	"testing"

	"ccvar.com/web3quant/internal/storage"
)

func TestBuildComputesCashEquityAndPnL(t *testing.T) {
	snapshot := Build([]storage.PaperExecutionRecord{
		{
			ID:         1,
			Symbol:     "BTCUSDT",
			Side:       "buy",
			SizeUSDT:   1000,
			RiskStatus: "approved",
			FillStatus: "filled",
			FillPrice:  100,
			FeeUSDT:    1,
			CreatedAt:  "2026-06-22T10:00:00Z",
		},
		{
			ID:         2,
			Symbol:     "BTCUSDT",
			Side:       "sell",
			SizeUSDT:   600,
			RiskStatus: "approved",
			FillStatus: "filled",
			FillPrice:  120,
			FeeUSDT:    0.6,
			CreatedAt:  "2026-06-22T10:01:00Z",
		},
		{
			ID:         3,
			Symbol:     "ETHUSDT",
			Side:       "buy",
			SizeUSDT:   500,
			RiskStatus: "rejected",
			FillStatus: "not_filled",
			CreatedAt:  "2026-06-22T10:02:00Z",
		},
	})

	if snapshot.FilledCount != 2 || snapshot.RejectedCount != 1 {
		t.Fatalf("counts = filled %d rejected %d", snapshot.FilledCount, snapshot.RejectedCount)
	}
	assertNear(t, snapshot.CashUSDT, 99598.4)
	assertNear(t, snapshot.EquityUSDT, 100198.4)
	assertNear(t, snapshot.RealizedPnLUSDT, 100)
	assertNear(t, snapshot.UnrealizedPnLUSDT, 100)
	assertNear(t, snapshot.TotalPnLUSDT, 198.4)
	assertNear(t, snapshot.FeesUSDT, 1.6)
	if snapshot.WinCount != 1 || snapshot.LossCount != 0 {
		t.Fatalf("win/loss = %d/%d", snapshot.WinCount, snapshot.LossCount)
	}
	if len(snapshot.Positions) != 1 {
		t.Fatalf("positions = %#v", snapshot.Positions)
	}
	position := snapshot.Positions[0]
	if position.Symbol != "BTCUSDT" || position.Side != "long" {
		t.Fatalf("position = %#v", position)
	}
	assertNear(t, position.Quantity, 5)
	assertNear(t, position.AveragePrice, 100)
	assertNear(t, position.MarkPrice, 120)
	assertNear(t, position.UnrealizedPnLUSDT, 100)
	assertNear(t, position.PnLPct, 20)
	if snapshot.UpdatedAt != "2026-06-22T10:02:00Z" {
		t.Fatalf("UpdatedAt = %q", snapshot.UpdatedAt)
	}
}

func TestBuildSupportsShortPositions(t *testing.T) {
	snapshot := Build([]storage.PaperExecutionRecord{
		{
			ID:         1,
			Symbol:     "ETHUSDT",
			Side:       "sell",
			SizeUSDT:   1000,
			RiskStatus: "approved",
			FillStatus: "filled",
			FillPrice:  200,
			FeeUSDT:    1,
		},
		{
			ID:         2,
			Symbol:     "ETHUSDT",
			Side:       "buy",
			SizeUSDT:   500,
			RiskStatus: "approved",
			FillStatus: "filled",
			FillPrice:  180,
			FeeUSDT:    0.5,
		},
	})
	if len(snapshot.Positions) != 1 || snapshot.Positions[0].Side != "short" {
		t.Fatalf("positions = %#v", snapshot.Positions)
	}
	assertNear(t, snapshot.RealizedPnLUSDT, 55.55555555555556)
	assertNear(t, snapshot.UnrealizedPnLUSDT, 44.44444444444444)
	assertNear(t, snapshot.Positions[0].PnLPct, 10)
}

func assertNear(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.0001 {
		t.Fatalf("got %.8f, want %.8f", got, want)
	}
}
