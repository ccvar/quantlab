package okx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"ccvar.com/web3quant/internal/core"
	"ccvar.com/web3quant/internal/exchange"
)

func TestFetchSnapshotParsesTicker(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v5/market/ticker" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Write([]byte(`{"code":"0","msg":"","data":[{"instId":"BTC-USDT","last":"67000.5","askPx":"67001","bidPx":"67000","volCcy24h":"2345678.9","ts":"1782050000000"}]}`))
	}))
	defer server.Close()

	adapter := Adapter{BaseURL: server.URL, Client: server.Client()}
	snapshot, err := adapter.FetchSnapshot(context.Background(), "BTCUSDT")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Symbol != "BTC-USDT" || snapshot.Last != 67000.5 {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
	if snapshot.LiquidityUSDT != 2345678.9 {
		t.Fatalf("unexpected liquidity: %.2f", snapshot.LiquidityUSDT)
	}
}

func TestFetchCandlesParsesAndSortsCandles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v5/market/candles" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Write([]byte(`{"code":"0","msg":"","data":[["1782050900000","1.5","2.5","1.2","2.0","11","0","0","1"],["1782050000000","1.0","2.0","0.5","1.5","42","0","0","1"]]}`))
	}))
	defer server.Close()

	adapter := Adapter{BaseURL: server.URL, Client: server.Client()}
	candles, err := adapter.FetchCandles(context.Background(), "BTC-USDT", "15m", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(candles) != 2 {
		t.Fatalf("expected two candles, got %d", len(candles))
	}
	if candles[0].Time != 1782050000 || candles[1].Time != 1782050900 {
		t.Fatalf("candles were not sorted ascending: %+v", candles)
	}
}

func TestTradingMethodsAreDisabled(t *testing.T) {
	adapter := New()
	if _, err := adapter.PlaceOrder(context.Background(), core.OrderRequest{}); err != exchange.ErrTradingDisabled {
		t.Fatalf("expected trading disabled, got %v", err)
	}
	if err := adapter.CancelOrder(context.Background(), "order-1"); err != exchange.ErrTradingDisabled {
		t.Fatalf("expected trading disabled, got %v", err)
	}
}
