package ai

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"ccvar.com/web3quant/internal/core"
)

type Context struct {
	Account  core.AccountState   `json:"account"`
	Market   core.MarketSnapshot `json:"market"`
	Mode     core.TradingMode    `json:"mode"`
	Strategy Strategy            `json:"strategy"`
	Candles  []core.Candle       `json:"candles,omitempty"`
	Now      time.Time           `json:"now,omitempty"`
}

type Strategy struct {
	Name          string    `json:"name"`
	Side          core.Side `json:"side"`
	OrderSizeUSDT float64   `json:"orderSizeUsdt"`
}

type Feature struct {
	Name   string  `json:"name"`
	Value  float64 `json:"value"`
	Impact string  `json:"impact"`
}

type Trace struct {
	Model            string        `json:"model"`
	PolicyVersion    string        `json:"policyVersion"`
	Signal           string        `json:"signal"`
	Confidence       float64       `json:"confidence"`
	Uncertainty      string        `json:"uncertainty"`
	UncertaintyScore float64       `json:"uncertaintyScore"`
	Regime           string        `json:"regime"`
	RiskOverride     string        `json:"riskOverride"`
	TTL              time.Duration `json:"ttl"`
	ExpiresAt        time.Time     `json:"expiresAt"`
	Reason           string        `json:"reason"`
	Features         []Feature     `json:"features"`
}

type Decision struct {
	Intent core.TradeIntent `json:"intent"`
	Trace  Trace            `json:"trace"`
}

type Engine interface {
	GenerateIntent(context.Context, Context) (Decision, error)
}

type StaticEngine struct {
	Intent core.TradeIntent
	Trace  Trace
}

func (engine StaticEngine) GenerateIntent(_ context.Context, ctx Context) (Decision, error) {
	intent := engine.Intent
	intent.Mode = ctx.Mode
	intent.Exchange = ctx.Market.Exchange
	intent.Symbol = ctx.Market.Symbol
	if intent.Price == 0 {
		intent.Price = ctx.Market.Last
	}
	trace := engine.Trace
	if trace.Model == "" {
		trace.Model = "Static Intent"
	}
	if trace.PolicyVersion == "" {
		trace.PolicyVersion = "static"
	}
	if trace.Signal == "" {
		trace.Signal = strings.ToUpper(string(intent.Side))
	}
	if trace.Confidence == 0 {
		trace.Confidence = intent.Confidence
	}
	if trace.TTL == 0 {
		trace.TTL = intent.TTL
	}
	if trace.ExpiresAt.IsZero() && !intent.GeneratedAt.IsZero() && trace.TTL > 0 {
		trace.ExpiresAt = intent.GeneratedAt.Add(trace.TTL)
	}
	return Decision{Intent: intent, Trace: trace}, nil
}

type LocalPolicy struct {
	Model         string
	PolicyVersion string
	TTL           time.Duration
}

func NewLocalPolicy() LocalPolicy {
	return LocalPolicy{
		Model:         "Local AI Policy",
		PolicyVersion: "v0.2.0",
		TTL:           2 * time.Minute,
	}
}

func (policy LocalPolicy) GenerateIntent(_ context.Context, ctx Context) (Decision, error) {
	strategy := normalizeStrategy(ctx.Strategy)
	now := ctx.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	ttl := policy.TTL
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}
	features := policyFeatures(ctx.Market, ctx.Candles, strategy.Side)
	confidence := policyConfidence(features)
	trace := Trace{
		Model:            defaultString(policy.Model, "Local AI Policy"),
		PolicyVersion:    defaultString(policy.PolicyVersion, "v0.2.0"),
		Signal:           strings.ToUpper(string(strategy.Side)),
		Confidence:       confidence,
		Uncertainty:      uncertaintyLabel(confidence),
		UncertaintyScore: round4(1 - confidence),
		Regime:           regimeLabel(features),
		RiskOverride:     "None",
		TTL:              ttl,
		ExpiresAt:        now.Add(ttl),
		Features:         features,
	}
	trace.Reason = policyReason(trace, strategy, ctx.Market)
	intent := core.TradeIntent{
		ID:          fmt.Sprintf("ai-%d", now.UnixNano()),
		Mode:        ctx.Mode,
		Exchange:    ctx.Market.Exchange,
		Symbol:      ctx.Market.Symbol,
		Side:        strategy.Side,
		OrderType:   core.OrderMarket,
		Price:       ctx.Market.Last,
		SizeUSDT:    strategy.OrderSizeUSDT,
		Confidence:  confidence,
		MaxSlippage: 0.001,
		Reason:      trace.Reason,
		TTL:         ttl,
		GeneratedAt: now,
	}
	return Decision{Intent: intent, Trace: trace}, nil
}

func normalizeStrategy(strategy Strategy) Strategy {
	if strategy.Name == "" {
		strategy.Name = "AI Momentum Pro"
	}
	if strategy.Side != core.SideSell {
		strategy.Side = core.SideBuy
	}
	if strategy.OrderSizeUSDT <= 0 {
		strategy.OrderSizeUSDT = 500
	}
	return strategy
}

func policyFeatures(market core.MarketSnapshot, candles []core.Candle, side core.Side) []Feature {
	spread := 0.5
	if market.SpreadPct > 0 {
		spread = clamp((0.08-market.SpreadPct)/0.08, -1, 1)
	}
	liquidity := 0.0
	if market.LiquidityUSDT > 0 {
		liquidity = clamp((market.LiquidityUSDT-250000)/1750000, -1, 1)
	}
	momentum := candleMomentum(candles)
	trend := candleTrend(candles)
	funding := clamp(-math.Abs(market.FundingRatePct)/0.05, -1, 0)
	if side == core.SideSell {
		momentum = -momentum
		trend = -trend
	}
	return []Feature{
		{Name: "Spread Quality", Value: round4(spread), Impact: impact(spread)},
		{Name: "Liquidity Depth", Value: round4(liquidity), Impact: impact(liquidity)},
		{Name: "Momentum", Value: round4(momentum), Impact: impact(momentum)},
		{Name: "Trend Alignment", Value: round4(trend), Impact: impact(trend)},
		{Name: "Funding Pressure", Value: round4(funding), Impact: impact(funding)},
	}
}

func policyConfidence(features []Feature) float64 {
	score := 0.56
	for _, feature := range features {
		value := feature.Value
		switch feature.Name {
		case "Spread Quality":
			score += max(value, 0) * 0.08
		case "Liquidity Depth":
			score += max(value, 0) * 0.08
		case "Momentum":
			score += max(value, 0) * 0.08
			score += min(value, 0) * 0.06
		case "Trend Alignment":
			score += max(value, 0) * 0.07
			score += min(value, 0) * 0.05
		case "Funding Pressure":
			score += value * 0.03
		}
	}
	return round4(clamp(score, 0.35, 0.91))
}

func candleMomentum(candles []core.Candle) float64 {
	if len(candles) < 2 {
		return 0
	}
	first := candles[0].Close
	last := candles[len(candles)-1].Close
	if first <= 0 || last <= 0 {
		return 0
	}
	return clamp(((last-first)/first)*12, -1, 1)
}

func candleTrend(candles []core.Candle) float64 {
	if len(candles) < 6 {
		return 0
	}
	window := candles[len(candles)-6:]
	var sum float64
	for _, candle := range window {
		sum += candle.Close
	}
	avg := sum / float64(len(window))
	last := window[len(window)-1].Close
	if avg <= 0 || last <= 0 {
		return 0
	}
	return clamp(((last-avg)/avg)*24, -1, 1)
}

func uncertaintyLabel(confidence float64) string {
	switch {
	case confidence >= 0.78:
		return "Low"
	case confidence >= 0.62:
		return "Medium"
	default:
		return "High"
	}
}

func regimeLabel(features []Feature) string {
	momentum := featureValue(features, "Momentum")
	trend := featureValue(features, "Trend Alignment")
	switch {
	case momentum > 0.2 && trend > 0.2:
		return "Trend Continuation"
	case momentum < -0.2 && trend < -0.2:
		return "Counter Trend"
	case math.Abs(momentum)+math.Abs(trend) < 0.2:
		return "Range / Neutral"
	default:
		return "Mixed Momentum"
	}
}

func policyReason(trace Trace, strategy Strategy, market core.MarketSnapshot) string {
	return fmt.Sprintf(
		"%s %s evaluated public market features for %s; %s intent uses %.0f USDT with %.0f%% confidence. Spread %.4f%%, liquidity %.0f USDT. Simulation policy only unless routed through Live Guard.",
		trace.Model,
		trace.PolicyVersion,
		strategy.Name,
		trace.Signal,
		strategy.OrderSizeUSDT,
		trace.Confidence*100,
		market.SpreadPct,
		market.LiquidityUSDT,
	)
}

func featureValue(features []Feature, name string) float64 {
	for _, feature := range features {
		if feature.Name == name {
			return feature.Value
		}
	}
	return 0
}

func impact(value float64) string {
	if value < 0 {
		return "negative"
	}
	return "positive"
}

func clamp(value, low, high float64) float64 {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

func round4(value float64) float64 {
	return math.Round(value*10000) / 10000
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func min(a, b float64) float64 {
	if a < b {
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
