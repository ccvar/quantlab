package okx

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const DemoBaseURL = "https://www.okx.com"

type AuthCredentials struct {
	APIKey        string
	Secret        string
	APIPassphrase string
}

type AuthRequestConfig struct {
	BaseURL     string
	Credentials AuthCredentials
	Method      string
	RequestPath string
	Body        string
	Timestamp   time.Time
	Demo        bool
}

func SignREST(timestamp, method, requestPath, body, secret string) string {
	prehash := timestamp + strings.ToUpper(method) + requestPath + body
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(prehash))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func NewAuthenticatedRequest(ctx context.Context, config AuthRequestConfig) (*http.Request, error) {
	if strings.TrimSpace(config.Credentials.APIKey) == "" ||
		strings.TrimSpace(config.Credentials.Secret) == "" ||
		strings.TrimSpace(config.Credentials.APIPassphrase) == "" {
		return nil, fmt.Errorf("okx api key, secret, and api passphrase are required")
	}
	baseURL := strings.TrimRight(config.BaseURL, "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	method := strings.ToUpper(config.Method)
	if method == "" {
		method = http.MethodGet
	}
	timestamp := config.Timestamp
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}
	timestampText := timestamp.UTC().Format("2006-01-02T15:04:05.000Z")
	body := config.Body
	req, err := http.NewRequestWithContext(ctx, method, baseURL+config.RequestPath, bytes.NewBufferString(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("OK-ACCESS-KEY", config.Credentials.APIKey)
	req.Header.Set("OK-ACCESS-SIGN", SignREST(timestampText, method, config.RequestPath, body, config.Credentials.Secret))
	req.Header.Set("OK-ACCESS-TIMESTAMP", timestampText)
	req.Header.Set("OK-ACCESS-PASSPHRASE", config.Credentials.APIPassphrase)
	if config.Demo {
		req.Header.Set("x-simulated-trading", "1")
	}
	return req, nil
}
