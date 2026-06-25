package livesync

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

func (client BinanceClient) Sync(ctx context.Context, request ClientRequest) (AccountSnapshot, error) {
	if request.Environment != "testnet" && request.Environment != "demo" {
		return AccountSnapshot{}, fmt.Errorf("binance account sync supports only testnet or demo")
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
	timestamp := request.Now
	if serverTime, err := binanceServerTime(ctx, httpClient, baseURL); err == nil {
		timestamp = serverTime
	}
	accountReq, err := binance.NewSignedRequest(ctx, http.MethodGet, binance.SignedRequestConfig{
		BaseURL: baseURL,
		APIKey:  request.Credential.APIKey,
		Secret:  request.Credential.Secret,
		Path:    "/api/v3/account",
		Params: []binance.OrderedParam{
			{Key: "omitZeroBalances", Value: "true"},
		},
		Timestamp: timestamp,
	})
	if err != nil {
		return AccountSnapshot{}, err
	}
	accountRaw, err := doJSON(httpClient, accountReq)
	if err != nil {
		return AccountSnapshot{}, err
	}
	account, err := parseBinanceAccount(accountRaw)
	if err != nil {
		return AccountSnapshot{}, err
	}

	openOrdersReq, err := binance.NewSignedRequest(ctx, http.MethodGet, binance.SignedRequestConfig{
		BaseURL: baseURL,
		APIKey:  request.Credential.APIKey,
		Secret:  request.Credential.Secret,
		Path:    "/api/v3/openOrders",
		Params: []binance.OrderedParam{
			{Key: "symbol", Value: binanceSymbol(request.Symbol)},
		},
		Timestamp: timestamp,
	})
	if err != nil {
		return AccountSnapshot{}, err
	}
	openOrdersRaw, err := doJSON(httpClient, openOrdersReq)
	if err != nil {
		return AccountSnapshot{}, err
	}
	openOrders, err := parseBinanceOpenOrders(openOrdersRaw)
	if err != nil {
		return AccountSnapshot{}, err
	}
	account.Exchange = "Binance"
	account.Environment = request.Environment
	account.OpenOrders = openOrders
	account.SyncedAt = request.Now.UTC().Format(time.RFC3339)
	return account, nil
}

func (client OKXClient) Sync(ctx context.Context, request ClientRequest) (AccountSnapshot, error) {
	if request.Environment != "demo" {
		return AccountSnapshot{}, fmt.Errorf("okx account sync supports demo environment only")
	}
	httpClient := client.Client
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	baseURL := strings.TrimRight(client.BaseURL, "/")
	if baseURL == "" {
		baseURL = okx.DemoBaseURL
	}
	credentials := okx.AuthCredentials{
		APIKey:        request.Credential.APIKey,
		Secret:        request.Credential.Secret,
		APIPassphrase: request.Credential.APIPassphrase,
	}
	balanceReq, err := okx.NewAuthenticatedRequest(ctx, okx.AuthRequestConfig{
		BaseURL:     baseURL,
		Credentials: credentials,
		Method:      http.MethodGet,
		RequestPath: "/api/v5/account/balance",
		Timestamp:   request.Now,
		Demo:        true,
	})
	if err != nil {
		return AccountSnapshot{}, err
	}
	balanceRaw, err := doJSON(httpClient, balanceReq)
	if err != nil {
		return AccountSnapshot{}, err
	}
	account, err := parseOKXBalance(balanceRaw)
	if err != nil {
		return AccountSnapshot{}, err
	}

	requestPath := "/api/v5/trade/orders-pending?instType=SPOT"
	if symbol := okxInstID(request.Symbol); symbol != "" {
		values := url.Values{}
		values.Set("instType", "SPOT")
		values.Set("instId", symbol)
		requestPath = "/api/v5/trade/orders-pending?" + values.Encode()
	}
	ordersReq, err := okx.NewAuthenticatedRequest(ctx, okx.AuthRequestConfig{
		BaseURL:     baseURL,
		Credentials: credentials,
		Method:      http.MethodGet,
		RequestPath: requestPath,
		Timestamp:   request.Now,
		Demo:        true,
	})
	if err != nil {
		return AccountSnapshot{}, err
	}
	ordersRaw, err := doJSON(httpClient, ordersReq)
	if err != nil {
		return AccountSnapshot{}, err
	}
	orders, err := parseOKXOpenOrders(ordersRaw)
	if err != nil {
		return AccountSnapshot{}, err
	}
	account.Exchange = "OKX"
	account.Environment = request.Environment
	account.OpenOrders = orders
	account.SyncedAt = request.Now.UTC().Format(time.RFC3339)
	return account, nil
}

func doJSON(client *http.Client, req *http.Request) (any, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, exchangeHTTPError(req.URL.Host, resp.Status, body)
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return map[string]any{}, nil
	}
	var raw any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func binanceServerTime(ctx context.Context, client *http.Client, baseURL string) (time.Time, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/api/v3/time", nil)
	if err != nil {
		return time.Time{}, err
	}
	raw, err := doJSON(client, req)
	if err != nil {
		return time.Time{}, err
	}
	payload, ok := raw.(map[string]any)
	if !ok {
		return time.Time{}, fmt.Errorf("unexpected binance time payload")
	}
	serverTime, ok := parseAnyMilli(payload["serverTime"])
	if !ok {
		return time.Time{}, fmt.Errorf("binance server time missing")
	}
	return serverTime, nil
}

func exchangeHTTPError(host, status string, body []byte) error {
	payload := map[string]any{}
	bodyText := strings.TrimSpace(string(body))
	code := ""
	message := ""
	if err := json.Unmarshal(body, &payload); err == nil {
		code = stringValue(payload["code"])
		message = strings.TrimSpace(firstString(payload["msg"], payload["message"], payload["error"]))
	}
	if message == "" {
		message = bodyText
	}
	exchange := exchangeNameFromHost(host)
	if code == "-1021" || strings.Contains(strings.ToLower(message), "recvwindow") {
		return fmt.Errorf("binance timestamp outside receive window")
	}
	if exchange != "" && code != "" {
		return fmt.Errorf("%s returned code %s: %s", exchange, code, message)
	}
	if exchange != "" && message != "" {
		return fmt.Errorf("%s returned %s: %s", exchange, status, message)
	}
	return fmt.Errorf("%s returned %s: %s", host, status, bodyText)
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

func parseBinanceAccount(raw any) (AccountSnapshot, error) {
	payload, ok := raw.(map[string]any)
	if !ok {
		return AccountSnapshot{}, fmt.Errorf("unexpected binance account payload")
	}
	balances := []Balance{}
	if rows, ok := payload["balances"].([]any); ok {
		for _, item := range rows {
			row, ok := item.(map[string]any)
			if !ok {
				continue
			}
			free := parseAnyFloat(row["free"])
			locked := parseAnyFloat(row["locked"])
			total := free + locked
			if total <= 0 {
				continue
			}
			balances = append(balances, Balance{
				Asset:  stringValue(row["asset"]),
				Free:   free,
				Locked: locked,
				Total:  total,
			})
		}
	}
	return AccountSnapshot{
		AccountType:  stringValue(payload["accountType"]),
		CanTrade:     boolValue(payload["canTrade"]),
		Balances:     balances,
		RawUpdatedAt: milliTimeString(payload["updateTime"]),
	}, nil
}

func parseBinanceOpenOrders(raw any) ([]OpenOrder, error) {
	rows, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("unexpected binance open orders payload")
	}
	orders := make([]OpenOrder, 0, len(rows))
	for _, item := range rows {
		row, ok := item.(map[string]any)
		if !ok {
			continue
		}
		orders = append(orders, OpenOrder{
			Symbol:        stringValue(row["symbol"]),
			OrderID:       stringValue(row["orderId"]),
			ClientOrderID: stringValue(row["clientOrderId"]),
			Side:          stringValue(row["side"]),
			Type:          stringValue(row["type"]),
			Status:        stringValue(row["status"]),
			Price:         parseAnyFloat(row["price"]),
			OrigQty:       parseAnyFloat(row["origQty"]),
			ExecutedQty:   parseAnyFloat(row["executedQty"]),
			QuoteQty:      parseAnyFloat(row["cummulativeQuoteQty"]),
			UpdatedAt:     milliTimeString(row["updateTime"]),
		})
	}
	return orders, nil
}

func parseOKXBalance(raw any) (AccountSnapshot, error) {
	payload, ok := raw.(map[string]any)
	if !ok {
		return AccountSnapshot{}, fmt.Errorf("unexpected okx balance payload")
	}
	if code := stringValue(payload["code"]); code != "" && code != "0" {
		return AccountSnapshot{}, fmt.Errorf("okx balance returned code %s: %s", code, stringValue(payload["msg"]))
	}
	var balances []Balance
	var updatedAt string
	if data, ok := payload["data"].([]any); ok && len(data) > 0 {
		if account, ok := data[0].(map[string]any); ok {
			updatedAt = milliTimeString(account["uTime"])
			if details, ok := account["details"].([]any); ok {
				for _, item := range details {
					row, ok := item.(map[string]any)
					if !ok {
						continue
					}
					free := parseAnyFloat(firstNonEmpty(row["availBal"], row["availEq"]))
					total := parseAnyFloat(firstNonEmpty(row["cashBal"], row["eq"]))
					locked := total - free
					if locked < 0 {
						locked = 0
					}
					if total <= 0 && free <= 0 {
						continue
					}
					balances = append(balances, Balance{
						Asset:  stringValue(row["ccy"]),
						Free:   free,
						Locked: locked,
						Total:  total,
						USD:    parseAnyFloat(row["eqUsd"]),
					})
				}
			}
		}
	}
	return AccountSnapshot{
		AccountType:  "UNIFIED",
		CanTrade:     true,
		Balances:     balances,
		RawUpdatedAt: updatedAt,
	}, nil
}

func parseOKXOpenOrders(raw any) ([]OpenOrder, error) {
	payload, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unexpected okx orders payload")
	}
	if code := stringValue(payload["code"]); code != "" && code != "0" {
		return nil, fmt.Errorf("okx orders returned code %s: %s", code, stringValue(payload["msg"]))
	}
	orders := []OpenOrder{}
	if rows, ok := payload["data"].([]any); ok {
		for _, item := range rows {
			row, ok := item.(map[string]any)
			if !ok {
				continue
			}
			orders = append(orders, OpenOrder{
				Symbol:        stringValue(row["instId"]),
				OrderID:       stringValue(row["ordId"]),
				ClientOrderID: stringValue(row["clOrdId"]),
				Side:          stringValue(row["side"]),
				Type:          stringValue(row["ordType"]),
				Status:        stringValue(row["state"]),
				Price:         parseAnyFloat(row["px"]),
				OrigQty:       parseAnyFloat(row["sz"]),
				ExecutedQty:   parseAnyFloat(row["accFillSz"]),
				QuoteQty:      parseAnyFloat(row["fillNotionalUsd"]),
				UpdatedAt:     milliTimeString(row["uTime"]),
			})
		}
	}
	return orders, nil
}

func firstNonEmpty(values ...any) any {
	for _, value := range values {
		if strings.TrimSpace(stringValue(value)) != "" {
			return value
		}
	}
	return ""
}

func boolValue(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(typed, "true")
	default:
		return false
	}
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

func parseAnyMilli(value any) (time.Time, bool) {
	raw := stringValue(value)
	if raw == "" {
		return time.Time{}, false
	}
	millis, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || millis <= 0 {
		return time.Time{}, false
	}
	return time.UnixMilli(millis), true
}

func firstString(values ...any) string {
	for _, value := range values {
		if result := stringValue(value); strings.TrimSpace(result) != "" {
			return result
		}
	}
	return ""
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

func milliTimeString(value any) string {
	text := stringValue(value)
	if text == "" {
		return ""
	}
	ms, err := strconv.ParseInt(text, 10, 64)
	if err != nil || ms <= 0 {
		return text
	}
	return time.UnixMilli(ms).UTC().Format(time.RFC3339)
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
