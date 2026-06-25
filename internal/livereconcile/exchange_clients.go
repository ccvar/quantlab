package livereconcile

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"ccvar.com/web3quant/internal/exchange/binance"
	"ccvar.com/web3quant/internal/exchange/okx"
)

type BinanceClient struct {
	Client  *http.Client
	BaseURL string
}

type OKXClient struct {
	Client  *http.Client
	BaseURL string
}

func (client BinanceClient) Reconcile(ctx context.Context, request ClientRequest) (Report, error) {
	if request.Environment != "testnet" && request.Environment != "demo" {
		return Report{}, fmt.Errorf("binance order reconciliation supports only testnet or demo")
	}
	httpClient := client.Client
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	baseURL := strings.TrimRight(client.BaseURL, "/")
	if baseURL == "" {
		if request.Environment == "demo" {
			baseURL = binance.SpotDemoBaseURL
		} else {
			baseURL = binance.SpotTestnetBaseURL
		}
	}
	params := []binance.OrderedParam{
		{Key: "symbol", Value: binanceSymbol(request.Symbol)},
	}
	if strings.TrimSpace(request.ExchangeOrderID) != "" {
		params = append(params, binance.OrderedParam{Key: "orderId", Value: request.ExchangeOrderID})
	} else {
		params = append(params, binance.OrderedParam{Key: "origClientOrderId", Value: request.ClientOrderID})
	}
	req, err := binance.NewSignedRequest(ctx, http.MethodGet, binance.SignedRequestConfig{
		BaseURL:   baseURL,
		APIKey:    request.Credential.APIKey,
		Secret:    request.Credential.Secret,
		Path:      "/api/v3/order",
		Params:    params,
		Timestamp: request.Now,
	})
	if err != nil {
		return Report{}, err
	}
	raw, err := doJSONMap(httpClient, req)
	if err != nil {
		return Report{}, err
	}
	status := strings.ToLower(defaultString(stringValue(raw["status"]), "unknown"))
	clientOrderID := defaultString(stringValue(raw["clientOrderId"]), request.ClientOrderID)
	exchangeOrderID := defaultString(stringValue(raw["orderId"]), request.ExchangeOrderID)
	filledUSDT := parseAnyFloat(raw["cummulativeQuoteQty"])
	if filledUSDT <= 0 {
		filledUSDT = parseAnyFloat(raw["executedQty"]) * parseAnyFloat(raw["price"])
	}
	return Report{
		Exchange:        "Binance",
		Environment:     request.Environment,
		Endpoint:        "/api/v3/order",
		ClientOrderID:   clientOrderID,
		ExchangeOrderID: exchangeOrderID,
		Status:          status,
		Message:         "Binance order status: " + strings.ToUpper(status),
		FilledUSDT:      filledUSDT,
		CheckedAt:       request.Now.UTC().Format(time.RFC3339),
		Raw:             raw,
	}, nil
}

func (client OKXClient) Reconcile(ctx context.Context, request ClientRequest) (Report, error) {
	if request.Environment != "demo" {
		return Report{}, fmt.Errorf("okx order reconciliation supports demo environment only")
	}
	httpClient := client.Client
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	baseURL := strings.TrimRight(client.BaseURL, "/")
	if baseURL == "" {
		baseURL = okx.DemoBaseURL
	}
	values := url.Values{}
	values.Set("instId", okxInstID(request.Symbol))
	if strings.TrimSpace(request.ExchangeOrderID) != "" {
		values.Set("ordId", request.ExchangeOrderID)
	} else {
		values.Set("clOrdId", request.ClientOrderID)
	}
	requestPath := "/api/v5/trade/order?" + values.Encode()
	req, err := okx.NewAuthenticatedRequest(ctx, okx.AuthRequestConfig{
		BaseURL: baseURL,
		Credentials: okx.AuthCredentials{
			APIKey:        request.Credential.APIKey,
			Secret:        request.Credential.Secret,
			APIPassphrase: request.Credential.APIPassphrase,
		},
		Method:      http.MethodGet,
		RequestPath: requestPath,
		Timestamp:   request.Now,
		Demo:        true,
	})
	if err != nil {
		return Report{}, err
	}
	raw, err := doJSONMap(httpClient, req)
	if err != nil {
		return Report{}, err
	}
	if code := stringValue(raw["code"]); code != "" && code != "0" {
		return Report{}, fmt.Errorf("okx order returned code %s: %s", code, stringValue(raw["msg"]))
	}
	row := firstOKXDataRow(raw)
	status := strings.ToLower(defaultString(stringValue(row["state"]), "unknown"))
	filledUSDT := parseAnyFloat(row["fillNotionalUsd"])
	if filledUSDT <= 0 {
		price := parseAnyFloat(firstNonEmpty(row["avgPx"], row["px"]))
		filledUSDT = parseAnyFloat(row["accFillSz"]) * price
	}
	clientOrderID := defaultString(stringValue(row["clOrdId"]), request.ClientOrderID)
	exchangeOrderID := defaultString(stringValue(row["ordId"]), request.ExchangeOrderID)
	return Report{
		Exchange:        "OKX",
		Environment:     request.Environment,
		Endpoint:        "/api/v5/trade/order",
		ClientOrderID:   clientOrderID,
		ExchangeOrderID: exchangeOrderID,
		Status:          status,
		Message:         "OKX order state: " + status,
		FilledUSDT:      filledUSDT,
		CheckedAt:       request.Now.UTC().Format(time.RFC3339),
		Raw:             raw,
	}, nil
}

func doJSONMap(client *http.Client, req *http.Request) (map[string]any, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, exchangeNetworkError(req.URL.Host)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s returned %s: %s", req.URL.Host, resp.Status, strings.TrimSpace(string(body)))
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return map[string]any{}, nil
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func exchangeNetworkError(host string) error {
	exchange := exchangeNameFromHost(host)
	if exchange != "" {
		return fmt.Errorf("%s network unavailable", exchange)
	}
	return fmt.Errorf("exchange network unavailable")
}

func exchangeNameFromHost(host string) string {
	normalized := strings.ToLower(host)
	switch {
	case strings.Contains(normalized, "binance"):
		return "binance"
	case strings.Contains(normalized, "okx"):
		return "okx"
	default:
		return ""
	}
}

func firstOKXDataRow(raw map[string]any) map[string]any {
	rows, ok := raw["data"].([]any)
	if !ok || len(rows) == 0 {
		return map[string]any{}
	}
	row, ok := rows[0].(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return row
}

func firstNonEmpty(values ...any) any {
	for _, value := range values {
		if strings.TrimSpace(stringValue(value)) != "" {
			return value
		}
	}
	return ""
}

func parseAnyFloat(value any) float64 {
	switch typed := value.(type) {
	case string:
		parsed, _ := strconv.ParseFloat(typed, 64)
		return parsed
	case float64:
		return typed
	case json.Number:
		parsed, _ := typed.Float64()
		return parsed
	default:
		return 0
	}
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case float64:
		if typed == float64(int64(typed)) {
			return strconv.FormatInt(int64(typed), 10)
		}
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case json.Number:
		return typed.String()
	default:
		return ""
	}
}

func binanceSymbol(symbol string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(symbol), "-", ""))
}

func okxInstID(symbol string) string {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if strings.Contains(symbol, "-") {
		return symbol
	}
	for _, quote := range []string{"USDT", "USDC", "USD", "BTC", "ETH"} {
		if strings.HasSuffix(symbol, quote) && len(symbol) > len(quote) {
			return symbol[:len(symbol)-len(quote)] + "-" + quote
		}
	}
	return symbol
}
