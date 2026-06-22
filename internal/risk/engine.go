package risk

import (
	"fmt"
	"strings"
	"time"

	"ccvar.com/web3quant/internal/core"
)

type Limits struct {
	MinConfidence        float64
	MaxOrderUSDT         float64
	MaxSymbolExposure    float64
	MaxTotalExposure     float64
	MaxDailyDrawdownPct  float64
	MaxConsecutiveLosses int
	MaxSpreadPct         float64
	RequireLiveUnlock    bool
}

type Decision struct {
	Approved bool     `json:"approved"`
	Reasons  []string `json:"reasons"`
}

type Engine struct {
	limits Limits
	now    func() time.Time
}

func NewEngine(limits Limits) Engine {
	return Engine{
		limits: limits,
		now:    time.Now,
	}
}

func (engine Engine) WithClock(now func() time.Time) Engine {
	engine.now = now
	return engine
}

func (engine Engine) Evaluate(intent core.TradeIntent, account core.AccountState, market core.MarketSnapshot) Decision {
	var reasons []string
	limits := engine.limits

	if intent.Side == core.SideHold {
		return Decision{Approved: false, Reasons: []string{"hold intent is not executable"}}
	}
	if intent.Exchange == "" || intent.Symbol == "" {
		reasons = append(reasons, "exchange and symbol are required")
	}
	if intent.SizeUSDT <= 0 {
		reasons = append(reasons, "order size must be positive")
	}
	if intent.OrderType == core.OrderLimit && intent.Price <= 0 {
		reasons = append(reasons, "limit order requires a positive price")
	}
	if intent.TTL > 0 && !intent.GeneratedAt.IsZero() && engine.now().After(intent.GeneratedAt.Add(intent.TTL)) {
		reasons = append(reasons, "intent expired")
	}
	if limits.RequireLiveUnlock && intent.Mode == core.ModeLive && !account.LiveTradingUnlocked {
		reasons = append(reasons, "live trading is locked")
	}
	if limits.MinConfidence > 0 && intent.Confidence < limits.MinConfidence {
		reasons = append(reasons, fmt.Sprintf("confidence %.2f below minimum %.2f", intent.Confidence, limits.MinConfidence))
	}
	if limits.MaxOrderUSDT > 0 && intent.SizeUSDT > limits.MaxOrderUSDT {
		reasons = append(reasons, fmt.Sprintf("order size %.2f exceeds %.2f", intent.SizeUSDT, limits.MaxOrderUSDT))
	}
	if account.AvailableUSDT >= 0 && intent.SizeUSDT > account.AvailableUSDT {
		reasons = append(reasons, "insufficient available balance")
	}
	if limits.MaxDailyDrawdownPct > 0 && account.DailyDrawdownPct <= -limits.MaxDailyDrawdownPct {
		reasons = append(reasons, "daily drawdown limit reached")
	}
	if limits.MaxConsecutiveLosses > 0 && account.ConsecutiveLosses >= limits.MaxConsecutiveLosses {
		reasons = append(reasons, "consecutive loss limit reached")
	}
	if limits.MaxSpreadPct > 0 && market.SpreadPct > limits.MaxSpreadPct {
		reasons = append(reasons, "spread exceeds limit")
	}

	symbolExposure := account.SymbolExposureUSDT[intent.Symbol] + signedExposure(intent)
	if limits.MaxSymbolExposure > 0 && abs(symbolExposure) > limits.MaxSymbolExposure {
		reasons = append(reasons, "symbol exposure limit exceeded")
	}
	totalExposure := account.OpenNotionalUSDT + abs(signedExposure(intent))
	if limits.MaxTotalExposure > 0 && totalExposure > limits.MaxTotalExposure {
		reasons = append(reasons, "total exposure limit exceeded")
	}

	return Decision{Approved: len(reasons) == 0, Reasons: reasons}
}

func (decision Decision) ReasonText() string {
	if len(decision.Reasons) == 0 {
		return "approved"
	}
	return strings.Join(decision.Reasons, "; ")
}

func signedExposure(intent core.TradeIntent) float64 {
	if intent.Side == core.SideSell {
		return -intent.SizeUSDT
	}
	return intent.SizeUSDT
}

func abs(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}
