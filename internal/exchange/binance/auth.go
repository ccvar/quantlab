package binance

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	SpotTestnetBaseURL = "https://testnet.binance.vision"
	SpotDemoBaseURL    = "https://demo-api.binance.com"
)

type OrderedParam struct {
	Key   string
	Value string
}

type SignedRequestConfig struct {
	BaseURL    string
	APIKey     string
	Secret     string
	Path       string
	Params     []OrderedParam
	RecvWindow int64
	Timestamp  time.Time
}

func SignPayload(secret, payload string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

func EncodeOrderedParams(params []OrderedParam) string {
	values := make([]string, 0, len(params))
	for _, param := range params {
		values = append(values, url.QueryEscape(param.Key)+"="+url.QueryEscape(param.Value))
	}
	return strings.Join(values, "&")
}

func NewSignedRequest(ctx context.Context, method string, config SignedRequestConfig) (*http.Request, error) {
	if strings.TrimSpace(config.APIKey) == "" || strings.TrimSpace(config.Secret) == "" {
		return nil, fmt.Errorf("binance api key and secret are required")
	}
	baseURL := strings.TrimRight(config.BaseURL, "/")
	if baseURL == "" {
		baseURL = SpotTestnetBaseURL
	}
	path := config.Path
	if path == "" {
		path = "/api/v3/order/test"
	}
	timestamp := config.Timestamp
	if timestamp.IsZero() {
		timestamp = time.Now()
	}
	recvWindow := config.RecvWindow
	if recvWindow <= 0 {
		recvWindow = 5000
	}
	params := append([]OrderedParam{}, config.Params...)
	params = append(params,
		OrderedParam{Key: "recvWindow", Value: fmt.Sprintf("%d", recvWindow)},
		OrderedParam{Key: "timestamp", Value: fmt.Sprintf("%d", timestamp.UTC().UnixMilli())},
	)
	payload := EncodeOrderedParams(params)
	signature := SignPayload(config.Secret, payload)
	endpoint := baseURL + path + "?" + payload + "&signature=" + signature
	req, err := http.NewRequestWithContext(ctx, method, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-MBX-APIKEY", config.APIKey)
	return req, nil
}

func NewTestOrderRequest(ctx context.Context, config SignedRequestConfig) (*http.Request, error) {
	config.Path = "/api/v3/order/test"
	return NewSignedRequest(ctx, http.MethodPost, config)
}
