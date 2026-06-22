package liveexec

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ccvar.com/web3quant/internal/core"
	"ccvar.com/web3quant/internal/vault"
)

func TestOKXExecutorValidationOnlyDoesNotSubmitNetworkOrder(t *testing.T) {
	report, err := (OKXExecutor{}).Execute(context.Background(), okxExecuteRequest(true))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if report.Status != "signed-preflight" || !report.ValidationOnly {
		t.Fatalf("report = %#v", report)
	}
	body, ok := report.Raw["body"].(string)
	if !ok || !strings.Contains(body, `"tgtCcy":"quote_ccy"`) {
		t.Fatalf("body = %#v", report.Raw["body"])
	}
}

func TestOKXExecutorSubmitsDemoOrderWithAuthHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/v5/trade/order" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.Header.Get("OK-ACCESS-KEY") != "OKXKEY123456" {
			t.Errorf("missing OKX key header")
		}
		if r.Header.Get("OK-ACCESS-SIGN") == "" || r.Header.Get("OK-ACCESS-TIMESTAMP") == "" {
			t.Errorf("missing OKX signature headers")
		}
		if r.Header.Get("OK-ACCESS-PASSPHRASE") != "okx-api-passphrase" {
			t.Errorf("missing OKX passphrase header")
		}
		if r.Header.Get("x-simulated-trading") != "1" {
			t.Errorf("missing demo trading header")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if body["instId"] != "BTC-USDT" || body["tdMode"] != "cash" || body["ordType"] != "market" || body["sz"] != "100" {
			t.Errorf("body = %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": "0",
			"msg":  "",
			"data": []map[string]string{{
				"ordId":   "okx-order-1",
				"clOrdId": "CCVARABC",
				"sCode":   "0",
				"sMsg":    "",
			}},
		})
	}))
	defer server.Close()

	request := okxExecuteRequest(false)
	request.Order.ClientOrderID = "CCVARABC"
	report, err := (OKXExecutor{BaseURL: server.URL, Client: server.Client()}).Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if report.Status != "submitted" || report.ExchangeOrderID != "okx-order-1" || report.ClientOrderID != "CCVARABC" {
		t.Fatalf("report = %#v", report)
	}
}

func okxExecuteRequest(validationOnly bool) ExecuteRequest {
	return ExecuteRequest{
		Environment:    "demo",
		ValidationOnly: validationOnly,
		Now:            fixedNow(),
		Market: core.MarketSnapshot{
			Exchange: "OKX",
			Symbol:   "BTC-USDT",
			Last:     67000,
		},
		Order: core.OrderRequest{
			ClientOrderID: "CCVARABC",
			Exchange:      "OKX",
			Symbol:        "BTC-USDT",
			Side:          core.SideBuy,
			OrderType:     core.OrderMarket,
			Price:         67000,
			SizeUSDT:      100,
		},
		Credential: vault.PlainCredential{
			APIKey:        "OKXKEY123456",
			Secret:        "OKXSECRET789",
			APIPassphrase: "okx-api-passphrase",
		},
	}
}
