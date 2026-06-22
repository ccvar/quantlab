package market

import (
	"context"
	"errors"
	"testing"
	"time"

	"ccvar.com/web3quant/internal/core"
	"ccvar.com/web3quant/internal/exchange"
	"ccvar.com/web3quant/internal/storage"
)

func TestLabStateUsesLivePublicMarketData(t *testing.T) {
	service := Service{
		Store:    fakeStore{},
		Registry: exchange.NewRegistry(fakeAdapter{}),
		Now:      fixedNow,
	}
	state, err := service.LabState(context.Background(), "FakeX", "BTCUSDT")
	if err != nil {
		t.Fatal(err)
	}
	if state.Meta.DataSource != "FakeX" {
		t.Fatalf("unexpected data source: %s", state.Meta.DataSource)
	}
	if state.Meta.SelectedMarket != "BTCUSDT" {
		t.Fatalf("unexpected selected market: %s", state.Meta.SelectedMarket)
	}
	if len(state.Candles) != 1 || state.Candles[0].Close != 67000 {
		t.Fatalf("expected live candles to replace seed: %+v", state.Candles)
	}
	if state.Events[0].Result != "Live public" || state.Events[0].Level != "success" {
		t.Fatalf("expected success market event: %+v", state.Events[0])
	}
	if state.Meta.Model != "Local AI Policy v0.2.0" || state.Verdict.Signal != "BUY" {
		t.Fatalf("expected local AI verdict, meta=%#v verdict=%#v", state.Meta, state.Verdict)
	}
	if len(state.Features) != 5 || state.Features[0].Name != "Spread Quality" {
		t.Fatalf("features = %#v", state.Features)
	}
}

func TestLabStateFallsBackWhenAdapterFails(t *testing.T) {
	service := Service{
		Store:    fakeStore{},
		Registry: exchange.NewRegistry(fakeAdapter{err: errors.New("network unavailable")}),
		Now:      fixedNow,
	}
	state, err := service.LabState(context.Background(), "FakeX", "BTCUSDT")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Candles) != 1 || state.Candles[0].Close != 42 {
		t.Fatalf("expected seed candles to remain: %+v", state.Candles)
	}
	if state.Events[0].Result != "Fallback" || state.Events[0].Level != "warn" {
		t.Fatalf("expected fallback warning event: %+v", state.Events[0])
	}
}

type fakeStore struct{}

func (fakeStore) LabState(context.Context) (storage.LabState, error) {
	return storage.LabState{
		Meta: storage.Meta{
			DataSource:     "Seed",
			SelectedMarket: "BTCUSDT",
			SelectedSymbol: "BTCUSDT Seed",
		},
		Candles: []storage.Candle{{Time: 1, Open: 40, High: 43, Low: 39, Close: 42, Volume: 10}},
		Events:  []storage.Event{{Time: "00:00:00", Type: "Seed", Symbol: "BTCUSDT", Result: "Seed", Level: "info"}},
	}, nil
}

type fakeAdapter struct {
	err error
}

func (adapter fakeAdapter) Name() string { return "FakeX" }

func (adapter fakeAdapter) FetchSnapshot(context.Context, string) (core.MarketSnapshot, error) {
	if adapter.err != nil {
		return core.MarketSnapshot{}, adapter.err
	}
	return core.MarketSnapshot{
		Exchange:  "FakeX",
		Symbol:    "BTCUSDT",
		BestBid:   66999,
		BestAsk:   67001,
		Last:      67000,
		SpreadPct: 0.003,
	}, nil
}

func (adapter fakeAdapter) FetchCandles(context.Context, string, string, int) ([]core.Candle, error) {
	if adapter.err != nil {
		return nil, adapter.err
	}
	return []core.Candle{{Time: 2, Open: 66000, High: 67100, Low: 65900, Close: 67000, Volume: 100}}, nil
}

func (fakeAdapter) PlaceOrder(context.Context, core.OrderRequest) (core.Fill, error) {
	return core.Fill{}, exchange.ErrTradingDisabled
}

func (fakeAdapter) CancelOrder(context.Context, string) error {
	return exchange.ErrTradingDisabled
}

func fixedNow() time.Time {
	return time.Date(2026, 6, 21, 15, 4, 5, 0, time.UTC)
}
