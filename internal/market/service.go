package market

import (
	"context"
	"fmt"
	"math"
	"time"

	"ccvar.com/web3quant/internal/ai"
	"ccvar.com/web3quant/internal/core"
	"ccvar.com/web3quant/internal/exchange"
	"ccvar.com/web3quant/internal/storage"
)

type Store interface {
	LabState(context.Context) (storage.LabState, error)
}

type Service struct {
	Store            Store
	Registry         exchange.Registry
	AI               ai.Engine
	StrategyProvider func(context.Context) (ai.Strategy, error)
	Now              func() time.Time
}

func (service Service) LabState(ctx context.Context, exchangeName, symbol string) (storage.LabState, error) {
	state, err := service.Store.LabState(ctx)
	if err != nil {
		return storage.LabState{}, err
	}
	if exchangeName == "" {
		exchangeName = state.Meta.DataSource
	}
	if symbol == "" {
		symbol = state.Meta.SelectedMarket
	}

	adapter, ok := service.Registry.Get(exchangeName)
	if !ok {
		state.Events = prependEvent(state.Events, storage.Event{
			Time:   clock(service.Now).Format("15:04:05"),
			Type:   "Market Data",
			Symbol: symbol,
			Action: "-",
			Result: "Fallback",
			Note:   fmt.Sprintf("No adapter for %s", exchangeName),
			Level:  "warn",
		})
		return state, nil
	}

	start := time.Now()
	snapshot, snapErr := adapter.FetchSnapshot(ctx, symbol)
	candles, candleErr := adapter.FetchCandles(ctx, symbol, "15m", 96)
	if snapErr != nil || candleErr != nil || len(candles) == 0 {
		note := "public market data unavailable"
		if snapErr != nil {
			note = snapErr.Error()
		} else if candleErr != nil {
			note = candleErr.Error()
		}
		state.Meta.DataSource = exchangeName
		state.Events = prependEvent(state.Events, storage.Event{
			Time:   clock(service.Now).Format("15:04:05"),
			Type:   "Market Data",
			Symbol: symbol,
			Action: "-",
			Result: "Fallback",
			Note:   note,
			Level:  "warn",
		})
		return state, nil
	}

	elapsedMS := int(time.Since(start).Milliseconds())
	if elapsedMS < 1 {
		elapsedMS = 1
	}
	state.Meta.DataSource = exchangeName
	state.Meta.SelectedMarket = snapshot.Symbol
	state.Meta.SelectedSymbol = snapshot.Symbol + " Spot"
	state.Meta.DataLatencyMS = elapsedMS
	state.Meta.LastUpdated = clock(service.Now).Format("15:04:05")
	state.Candles = candles
	if trace, strategy, err := service.aiTrace(ctx, snapshot, candles); err == nil {
		state.Meta.Strategy = strategy.Name
		state.Meta.Model = trace.Model + " " + trace.PolicyVersion
		state.Verdict = verdictFromTrace(trace)
		state.Features = featuresFromTrace(trace.Features)
	} else {
		state.Events = prependEvent(state.Events, storage.Event{
			Time:   state.Meta.LastUpdated,
			Type:   "AI Policy",
			Symbol: snapshot.Symbol,
			Action: "-",
			Price:  snapshot.Last,
			Result: "Fallback",
			Note:   err.Error(),
			Level:  "warn",
		})
	}
	state.Events = prependEvent(state.Events, storage.Event{
		Time:   state.Meta.LastUpdated,
		Type:   "Market Data",
		Symbol: snapshot.Symbol,
		Action: "SYNC",
		Price:  snapshot.Last,
		Result: "Live public",
		Note:   fmt.Sprintf("bid %.2f / ask %.2f / spread %.4f%%", snapshot.BestBid, snapshot.BestAsk, snapshot.SpreadPct),
		Level:  "success",
	})
	return state, nil
}

func (service Service) aiTrace(ctx context.Context, snapshot core.MarketSnapshot, candles []core.Candle) (ai.Trace, ai.Strategy, error) {
	strategy := ai.Strategy{
		Name:          "AI Momentum Pro",
		Side:          core.SideBuy,
		OrderSizeUSDT: 500,
	}
	if service.StrategyProvider != nil {
		provided, err := service.StrategyProvider(ctx)
		if err != nil {
			return ai.Trace{}, strategy, err
		}
		strategy = provided
	}
	engine := service.AI
	if engine == nil {
		engine = ai.NewLocalPolicy()
	}
	decision, err := engine.GenerateIntent(ctx, ai.Context{
		Account: core.AccountState{
			EquityUSDT:         100000,
			AvailableUSDT:      20000,
			DailyDrawdownPct:   -0.73,
			OpenNotionalUSDT:   5000,
			SymbolExposureUSDT: map[string]float64{snapshot.Symbol: 2500},
		},
		Market:   snapshot,
		Mode:     core.ModeShadow,
		Strategy: strategy,
		Candles:  candles,
		Now:      clock(service.Now).UTC(),
	})
	if err != nil {
		return ai.Trace{}, strategy, err
	}
	return decision.Trace, strategy, nil
}

func verdictFromTrace(trace ai.Trace) storage.Verdict {
	return storage.Verdict{
		Signal:           trace.Signal,
		Confidence:       math.Round(trace.Confidence * 100),
		Uncertainty:      trace.Uncertainty,
		UncertaintyScore: trace.UncertaintyScore,
		Regime:           trace.Regime,
		RiskOverride:     trace.RiskOverride,
		TTL:              formatTTL(trace.TTL),
		ExpiresAt:        trace.ExpiresAt.Format("15:04:05"),
		Reasoning:        trace.Reason,
	}
}

func featuresFromTrace(features []ai.Feature) []storage.FeatureImpact {
	result := make([]storage.FeatureImpact, 0, len(features))
	for _, feature := range features {
		result = append(result, storage.FeatureImpact{
			Name:   feature.Name,
			Value:  feature.Value,
			Impact: feature.Impact,
		})
	}
	return result
}

func formatTTL(duration time.Duration) string {
	if duration <= 0 {
		return "00:00"
	}
	total := int(duration.Round(time.Second).Seconds())
	return fmt.Sprintf("%02d:%02d", total/60, total%60)
}

func prependEvent(events []storage.Event, event storage.Event) []storage.Event {
	result := append([]storage.Event{event}, events...)
	if len(result) > 12 {
		return result[:12]
	}
	return result
}

func clock(now func() time.Time) time.Time {
	if now == nil {
		return time.Now()
	}
	return now()
}
