package okx

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestSignRESTMatchesKnownHMACBase64Vector(t *testing.T) {
	got := SignREST(
		"2020-12-08T09:08:57.715Z",
		http.MethodGet,
		"/api/v5/account/balance?ccy=BTC",
		"",
		"22582BD0CFF14C41EDBF1AB98506286D",
	)
	want := "HiZhvSfMtWJA3uUIVXV3a/bSXNPCWvYFXoGCVS8V4zY="
	if got != want {
		t.Fatalf("SignREST() = %s, want %s", got, want)
	}
}

func TestNewAuthenticatedRequestBuildsDemoHeaders(t *testing.T) {
	req, err := NewAuthenticatedRequest(context.Background(), AuthRequestConfig{
		BaseURL:     DemoBaseURL,
		Method:      http.MethodPost,
		RequestPath: "/api/v5/trade/order",
		Body:        `{"instId":"BTC-USDT","tdMode":"cash","side":"buy","ordType":"limit","px":"1000","sz":"0.01"}`,
		Timestamp:   time.Date(2020, 3, 28, 12, 21, 41, 274000000, time.UTC),
		Demo:        true,
		Credentials: AuthCredentials{
			APIKey:        "key",
			Secret:        "secret",
			APIPassphrase: "pass",
		},
	})
	if err != nil {
		t.Fatalf("NewAuthenticatedRequest() error = %v", err)
	}
	if req.Method != http.MethodPost || req.URL.String() != "https://www.okx.com/api/v5/trade/order" {
		t.Fatalf("request = %s %s", req.Method, req.URL.String())
	}
	if req.Header.Get("OK-ACCESS-KEY") != "key" ||
		req.Header.Get("OK-ACCESS-PASSPHRASE") != "pass" ||
		req.Header.Get("OK-ACCESS-SIGN") == "" ||
		req.Header.Get("OK-ACCESS-TIMESTAMP") != "2020-03-28T12:21:41.274Z" {
		t.Fatalf("headers = %#v", req.Header)
	}
	if req.Header.Get("x-simulated-trading") != "1" {
		t.Fatal("missing simulated trading header")
	}
}
