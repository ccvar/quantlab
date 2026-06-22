package binance

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"ccvar.com/web3quant/internal/core"
	"ccvar.com/web3quant/internal/exchange"
)

func TestFetchSnapshotParsesBookAndTicker(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v3/ticker/bookTicker":
			w.Write([]byte(`{"symbol":"BTCUSDT","bidPrice":"67000.00","bidQty":"1.2","askPrice":"67001.00","askQty":"0.9"}`))
		case "/api/v3/ticker/24hr":
			w.Write([]byte(`{"lastPrice":"67000.50","quoteVolume":"1234567.89","closeTime":1782050000000}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	adapter := Adapter{BaseURL: server.URL, Client: server.Client()}
	snapshot, err := adapter.FetchSnapshot(context.Background(), "BTC-USDT")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Symbol != "BTCUSDT" || snapshot.Last != 67000.50 {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
	if snapshot.SpreadPct <= 0 {
		t.Fatalf("expected positive spread: %+v", snapshot)
	}
	if snapshot.LiquidityUSDT != 1234567.89 {
		t.Fatalf("unexpected liquidity: %.2f", snapshot.LiquidityUSDT)
	}
}

func TestFetchCandlesParsesKlines(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/klines" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Write([]byte(`[[1782050000000,"1.0","2.0","0.5","1.5","42.0",1782050899999,"0",1,"0","0","0"]]`))
	}))
	defer server.Close()

	adapter := Adapter{BaseURL: server.URL, Client: server.Client()}
	candles, err := adapter.FetchCandles(context.Background(), "BTCUSDT", "15m", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(candles) != 1 {
		t.Fatalf("expected one candle, got %d", len(candles))
	}
	if candles[0].Time != 1782050000 || candles[0].Close != 1.5 || candles[0].Volume != 42 {
		t.Fatalf("unexpected candle: %+v", candles[0])
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
