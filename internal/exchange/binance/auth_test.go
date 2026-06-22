package binance

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestSignPayloadMatchesBinanceOfficialExample(t *testing.T) {
	payload := "symbol=LTCBTC&side=BUY&type=LIMIT&timeInForce=GTC&quantity=1&price=0.1&recvWindow=5000&timestamp=1499827319559"
	secret := "NhqPtmdSJYdKjVHjA7PZj4Mge3R5YNiP1e3UZjInClVN65XAbvqqM6A7H5fATj0j"
	want := "c8db56825ae71d6d79447849e617115f4a920fa2acdcab2b053c4b2838bd6b71"
	if got := SignPayload(secret, payload); got != want {
		t.Fatalf("SignPayload() = %s, want %s", got, want)
	}
}

func TestEncodeOrderedParamsPercentEncodesBeforeSigning(t *testing.T) {
	payload := EncodeOrderedParams([]OrderedParam{
		{Key: "symbol", Value: "１２３４５６"},
		{Key: "side", Value: "BUY"},
		{Key: "type", Value: "LIMIT"},
		{Key: "timeInForce", Value: "GTC"},
		{Key: "quantity", Value: "1"},
		{Key: "price", Value: "0.1"},
		{Key: "recvWindow", Value: "5000"},
		{Key: "timestamp", Value: "1499827319559"},
	})
	secret := "NhqPtmdSJYdKjVHjA7PZj4Mge3R5YNiP1e3UZjInClVN65XAbvqqM6A7H5fATj0j"
	want := "e1353ec6b14d888f1164ae9af8228a3dbd508bc82eb867db8ab6046442f33ef3"
	if got := SignPayload(secret, payload); got != want {
		t.Fatalf("SignPayload(non-ascii) = %s, want %s; payload %s", got, want, payload)
	}
}

func TestNewTestOrderRequestBuildsSignedTestnetRequest(t *testing.T) {
	req, err := NewTestOrderRequest(context.Background(), SignedRequestConfig{
		BaseURL: "https://testnet.binance.vision",
		APIKey:  "key",
		Secret:  "secret",
		Params: []OrderedParam{
			{Key: "symbol", Value: "BTCUSDT"},
			{Key: "side", Value: "BUY"},
			{Key: "type", Value: "MARKET"},
			{Key: "quoteOrderQty", Value: "10"},
		},
		Timestamp: time.UnixMilli(1499827319559),
	})
	if err != nil {
		t.Fatalf("NewTestOrderRequest() error = %v", err)
	}
	if req.Method != http.MethodPost {
		t.Fatalf("method = %s", req.Method)
	}
	if req.URL.Host != "testnet.binance.vision" || req.URL.Path != "/api/v3/order/test" {
		t.Fatalf("url = %s", req.URL.String())
	}
	if req.Header.Get("X-MBX-APIKEY") != "key" {
		t.Fatal("missing X-MBX-APIKEY header")
	}
	if req.URL.Query().Get("signature") == "" {
		t.Fatal("missing signature")
	}
}
