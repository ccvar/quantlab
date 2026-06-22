package exchange

import (
	"context"
	"errors"

	"ccvar.com/web3quant/internal/core"
)

var ErrTradingDisabled = errors.New("live trading is disabled for this adapter")

type Adapter interface {
	Name() string
	FetchSnapshot(context.Context, string) (core.MarketSnapshot, error)
	FetchCandles(context.Context, string, string, int) ([]core.Candle, error)
	PlaceOrder(context.Context, core.OrderRequest) (core.Fill, error)
	CancelOrder(context.Context, string) error
}

type Registry struct {
	adapters map[string]Adapter
}

func NewRegistry(adapters ...Adapter) Registry {
	registry := Registry{adapters: map[string]Adapter{}}
	for _, adapter := range adapters {
		registry.adapters[adapter.Name()] = adapter
	}
	return registry
}

func (registry Registry) Get(name string) (Adapter, bool) {
	adapter, ok := registry.adapters[name]
	return adapter, ok
}
