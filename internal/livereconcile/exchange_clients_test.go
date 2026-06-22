package livereconcile

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"ccvar.com/web3quant/internal/vault"
)

func TestBinanceClientReconcileSignsQueryOrder(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/api/v3/order" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.Header.Get("X-MBX-APIKEY") != "BINANCEKEY123456" {
			t.Errorf("missing Binance API key header")
		}
		query := r.URL.Query()
		if query.Get("symbol") != "BTCUSDT" || query.Get("orderId") != "98765" {
			t.Errorf("query = %s", r.URL.RawQuery)
		}
		if query.Get("signature") == "" || query.Get("timestamp") == "" {
			t.Errorf("missing signed query in %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"symbol":              "BTCUSDT",
			"orderId":             "98765",
			"clientOrderId":       "CCVAR123",
			"status":              "FILLED",
			"executedQty":         "0.002",
			"price":               "50000",
			"cummulativeQuoteQty": "101.5",
		})
	}))
	defer server.Close()

	report, err := (BinanceClient{BaseURL: server.URL, Client: server.Client()}).Reconcile(context.Background(), ClientRequest{
		Environment:     "testnet",
		Symbol:          "BTCUSDT",
		ClientOrderID:   "CCVAR123",
		ExchangeOrderID: "98765",
		Now:             fixedNow(),
		Credential: vault.PlainCredential{
			APIKey: "BINANCEKEY123456",
			Secret: "BINANCESECRET789",
		},
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if report.Status != "filled" || report.ClientOrderID != "CCVAR123" || report.ExchangeOrderID != "98765" || report.FilledUSDT != 101.5 {
		t.Fatalf("report = %#v", report)
	}
}

func TestBinanceClientReconcileFallsBackToClientOrderID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("origClientOrderId") != "CCVAR123" || r.URL.Query().Get("orderId") != "" {
			t.Errorf("query = %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"clientOrderId": "CCVAR123",
			"status":        "NEW",
			"executedQty":   "0.002",
			"price":         "50000",
		})
	}))
	defer server.Close()

	report, err := (BinanceClient{BaseURL: server.URL, Client: server.Client()}).Reconcile(context.Background(), ClientRequest{
		Environment:   "demo",
		Symbol:        "BTC-USDT",
		ClientOrderID: "CCVAR123",
		Now:           fixedNow(),
		Credential: vault.PlainCredential{
			APIKey: "BINANCEKEY123456",
			Secret: "BINANCESECRET789",
		},
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if report.Status != "new" || report.FilledUSDT != 100 {
		t.Fatalf("report = %#v", report)
	}
}

func TestOKXClientReconcileUsesDemoHeaderAndParsesOrder(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/api/v5/trade/order" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.URL.Query().Get("instId") != "BTC-USDT" || r.URL.Query().Get("clOrdId") != "CCVAR123" {
			t.Errorf("query = %s", r.URL.RawQuery)
		}
		if r.Header.Get("OK-ACCESS-KEY") != "OKXKEY123456" ||
			r.Header.Get("OK-ACCESS-SIGN") == "" ||
			r.Header.Get("OK-ACCESS-TIMESTAMP") == "" ||
			r.Header.Get("OK-ACCESS-PASSPHRASE") != "okx-api-passphrase" {
			t.Errorf("missing OKX auth headers")
		}
		if r.Header.Get("x-simulated-trading") != "1" {
			t.Errorf("missing demo trading header")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": "0",
			"msg":  "",
			"data": []map[string]string{{
				"instId":          "BTC-USDT",
				"ordId":           "okx-order-1",
				"clOrdId":         "CCVAR123",
				"state":           "filled",
				"accFillSz":       "0.002",
				"avgPx":           "50000",
				"fillNotionalUsd": "101.5",
			}},
		})
	}))
	defer server.Close()

	report, err := (OKXClient{BaseURL: server.URL, Client: server.Client()}).Reconcile(context.Background(), ClientRequest{
		Environment:   "demo",
		Symbol:        "BTCUSDT",
		ClientOrderID: "CCVAR123",
		Now:           fixedNow(),
		Credential: vault.PlainCredential{
			APIKey:        "OKXKEY123456",
			Secret:        "OKXSECRET789",
			APIPassphrase: "okx-api-passphrase",
		},
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if report.Status != "filled" || report.ExchangeOrderID != "okx-order-1" || report.FilledUSDT != 101.5 {
		t.Fatalf("report = %#v", report)
	}
}

func TestOKXClientReconcileRejectsNonDemoEnvironment(t *testing.T) {
	_, err := (OKXClient{}).Reconcile(context.Background(), ClientRequest{
		Environment: "testnet",
		Credential: vault.PlainCredential{
			APIKey:        "OKXKEY123456",
			Secret:        "OKXSECRET789",
			APIPassphrase: "okx-api-passphrase",
		},
	})
	if err == nil {
		t.Fatal("Reconcile() error = nil, want error")
	}
}
