package liveexec

import (
	"strings"
	"testing"
	"time"

	"ccvar.com/web3quant/internal/ai"
	"ccvar.com/web3quant/internal/core"
	"ccvar.com/web3quant/internal/liveguard"
)

func TestBuildIntentUsesInternalPlannedIntent(t *testing.T) {
	now := time.Date(2026, 6, 22, 10, 30, 0, 0, time.UTC)
	planned := core.TradeIntent{
		ID:          "ai-plan",
		Side:        core.SideSell,
		SizeUSDT:    321,
		Confidence:  0.77,
		MaxSlippage: 0.001,
		Reason:      "Local AI Policy v0.2.0 planned live intent",
		TTL:         90 * time.Second,
		GeneratedAt: now,
	}
	intent := buildIntent(now, core.MarketSnapshot{
		Exchange: "Binance",
		Symbol:   "BTCUSDT",
		Last:     67000,
	}, Request{
		Side:          "buy",
		SizeUSDT:      50,
		PlannedIntent: &planned,
		AITrace: &ai.Trace{
			Model:         "Local AI Policy",
			PolicyVersion: "v0.2.0",
			Signal:        "SELL",
		},
	}, liveguard.State{MaxOrderUSDT: 1000})

	if !strings.HasPrefix(intent.ID, "CCVAR") {
		t.Fatalf("intent.ID = %q, want CCVAR prefix", intent.ID)
	}
	if intent.Side != core.SideSell || intent.SizeUSDT != 321 || intent.Confidence != 0.77 {
		t.Fatalf("intent = %#v", intent)
	}
	if intent.Price != 67000 || intent.TTL != 90*time.Second || !strings.Contains(intent.Reason, "Local AI Policy") {
		t.Fatalf("intent = %#v", intent)
	}
	view := viewIntent(intent, &ai.Trace{Model: "Local AI Policy", PolicyVersion: "v0.2.0", Signal: "SELL"})
	if view.Model != "Local AI Policy" || view.PolicyVersion != "v0.2.0" || view.Signal != "SELL" {
		t.Fatalf("view = %#v", view)
	}
	payload := intentPayload(intent, liveguard.State{Environment: "testnet"}, true, &ai.Trace{
		Model:         "Local AI Policy",
		PolicyVersion: "v0.2.0",
		Signal:        "SELL",
		Confidence:    0.77,
	})
	aiPayload, ok := payload["ai"].(map[string]any)
	if !ok || aiPayload["policyVersion"] != "v0.2.0" {
		t.Fatalf("payload = %#v", payload)
	}
}
