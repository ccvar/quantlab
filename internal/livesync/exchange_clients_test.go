package livesync

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ccvar.com/web3quant/internal/vault"
)

func TestBinanceClientSyncSignsAccountAndOpenOrders(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Path != "/api/v3/time" {
			if r.Header.Get("X-MBX-APIKEY") != "BINANCEKEY123456" {
				t.Errorf("missing Binance API key header")
			}
			if r.URL.Query().Get("signature") == "" || r.URL.Query().Get("timestamp") == "" {
				t.Errorf("missing signed query in %s", r.URL.RawQuery)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v3/time":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"serverTime": 1782057000000,
			})
		case "/api/v3/account":
			if r.URL.Query().Get("omitZeroBalances") != "true" {
				t.Errorf("omitZeroBalances = %q", r.URL.Query().Get("omitZeroBalances"))
			}
			if r.URL.Query().Get("timestamp") != "1782057000000" {
				t.Errorf("timestamp = %q", r.URL.Query().Get("timestamp"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"accountType": "SPOT",
				"canTrade":    true,
				"updateTime":  1782057000000,
				"balances": []map[string]string{
					{"asset": "USDT", "free": "1000.5", "locked": "2.5"},
					{"asset": "BTC", "free": "0.00000000", "locked": "0.00000000"},
				},
			})
		case "/api/v3/openOrders":
			if r.URL.Query().Get("symbol") != "BTCUSDT" {
				t.Errorf("symbol = %q", r.URL.Query().Get("symbol"))
			}
			_ = json.NewEncoder(w).Encode([]map[string]string{{
				"symbol":              "BTCUSDT",
				"orderId":             "11",
				"clientOrderId":       "CCVAR1",
				"price":               "67000",
				"origQty":             "0.01",
				"executedQty":         "0.002",
				"cummulativeQuoteQty": "134",
				"status":              "NEW",
				"type":                "LIMIT",
				"side":                "BUY",
				"updateTime":          "1782057000000",
			}})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	snapshot, err := (BinanceClient{BaseURL: server.URL, Client: server.Client()}).Sync(context.Background(), ClientRequest{
		Environment: "testnet",
		Symbol:      "BTCUSDT",
		Now:         fixedNow(),
		Credential: vault.PlainCredential{
			APIKey: "BINANCEKEY123456",
			Secret: "BINANCESECRET789",
		},
	})
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if strings.Join(paths, ",") != "/api/v3/time,/api/v3/account,/api/v3/openOrders" {
		t.Fatalf("paths = %#v", paths)
	}
	if snapshot.AccountType != "SPOT" || !snapshot.CanTrade || len(snapshot.Balances) != 1 || snapshot.Balances[0].Total != 1003 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	if len(snapshot.OpenOrders) != 1 || snapshot.OpenOrders[0].ClientOrderID != "CCVAR1" {
		t.Fatalf("orders = %#v", snapshot.OpenOrders)
	}
}

func TestBinanceClientSyncReportsTimestampWindowClearly(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v3/time":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"serverTime": 1782057000000,
			})
		case "/api/v3/account":
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": -1021,
				"msg":  "Timestamp for this request is outside of the recvWindow.",
			})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	_, err := (BinanceClient{BaseURL: server.URL, Client: server.Client()}).Sync(context.Background(), ClientRequest{
		Environment: "testnet",
		Symbol:      "BTCUSDT",
		Now:         fixedNow(),
		Credential: vault.PlainCredential{
			APIKey: "BINANCEKEY123456",
			Secret: "BINANCESECRET789",
		},
	})
	if err == nil {
		t.Fatal("Sync() error = nil")
	}
	if err.Error() != "binance timestamp outside receive window" {
		t.Fatalf("error = %q", err.Error())
	}
}

func TestOKXClientSyncReportsNetworkUnavailableClearly(t *testing.T) {
	httpClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return nil, errors.New("dial tcp 169.254.0.2:443: connect: host is down")
		}),
	}
	_, err := (OKXClient{Client: httpClient}).Sync(context.Background(), ClientRequest{
		Environment: "demo",
		Symbol:      "BTCUSDT",
		Now:         fixedNow(),
		Credential: vault.PlainCredential{
			APIKey:        "OKXKEY123456",
			Secret:        "OKXSECRET789",
			APIPassphrase: "okx-api-passphrase",
		},
	})
	if err == nil {
		t.Fatal("Sync() error = nil")
	}
	if err.Error() != "okx network unavailable" {
		t.Fatalf("error = %q", err.Error())
	}
}

func TestOKXClientSyncUsesDemoHeaderAndParsesPayload(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
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
		switch r.URL.Path {
		case "/api/v5/account/balance":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": "0",
				"msg":  "",
				"data": []map[string]any{{
					"uTime": "1782057000000",
					"details": []map[string]string{{
						"ccy":      "USDT",
						"availBal": "2500.25",
						"cashBal":  "2600.25",
						"eqUsd":    "2600.25",
					}},
				}},
			})
		case "/api/v5/trade/orders-pending":
			if r.URL.Query().Get("instId") != "BTC-USDT" || r.URL.Query().Get("instType") != "SPOT" {
				t.Errorf("query = %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": "0",
				"msg":  "",
				"data": []map[string]string{{
					"instId":          "BTC-USDT",
					"ordId":           "okx-order-1",
					"clOrdId":         "CCVAR2",
					"side":            "buy",
					"ordType":         "limit",
					"state":           "live",
					"px":              "67000",
					"sz":              "0.02",
					"accFillSz":       "0.01",
					"fillNotionalUsd": "670",
					"uTime":           "1782057000000",
				}},
			})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	snapshot, err := (OKXClient{BaseURL: server.URL, Client: server.Client()}).Sync(context.Background(), ClientRequest{
		Environment: "demo",
		Symbol:      "BTCUSDT",
		Now:         fixedNow(),
		Credential: vault.PlainCredential{
			APIKey:        "OKXKEY123456",
			Secret:        "OKXSECRET789",
			APIPassphrase: "okx-api-passphrase",
		},
	})
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if strings.Join(paths, ",") != "/api/v5/account/balance,/api/v5/trade/orders-pending" {
		t.Fatalf("paths = %#v", paths)
	}
	if snapshot.AccountType != "UNIFIED" || len(snapshot.Balances) != 1 || snapshot.Balances[0].Locked != 100 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	if len(snapshot.OpenOrders) != 1 || snapshot.OpenOrders[0].ClientOrderID != "CCVAR2" {
		t.Fatalf("orders = %#v", snapshot.OpenOrders)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}
