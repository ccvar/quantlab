package liveexec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ccvar.com/web3quant/internal/core"
	"ccvar.com/web3quant/internal/exchange/binance"
	"ccvar.com/web3quant/internal/exchange/okx"
)

type BinanceExecutor struct {
	Client  *http.Client
	BaseURL string
}

type OKXExecutor struct {
	Client  *http.Client
	BaseURL string
}

func (executor BinanceExecutor) Execute(ctx context.Context, request ExecuteRequest) (ExecutionReport, error) {
	if request.Environment != "testnet" && request.Environment != "demo" {
		return ExecutionReport{}, fmt.Errorf("binance live execution supports only testnet or demo")
	}
	client := executor.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	baseURL := strings.TrimRight(executor.BaseURL, "/")
	if baseURL == "" {
		if request.Environment == "demo" {
			baseURL = binance.SpotDemoBaseURL
		} else {
			baseURL = binance.SpotTestnetBaseURL
		}
	}
	path := "/api/v3/order"
	status := "submitted"
	message := "order submitted to Binance testnet/demo"
	if request.ValidationOnly {
		path = "/api/v3/order/test"
		status = "validated"
		message = "order validated by Binance test endpoint"
	}
	params := []binance.OrderedParam{
		{Key: "symbol", Value: binanceSymbol(request.Order.Symbol)},
		{Key: "side", Value: strings.ToUpper(string(request.Order.Side))},
		{Key: "type", Value: strings.ToUpper(string(request.Order.OrderType))},
		{Key: "quoteOrderQty", Value: formatDecimal(request.Order.SizeUSDT)},
		{Key: "newClientOrderId", Value: request.Order.ClientOrderID},
		{Key: "newOrderRespType", Value: "RESULT"},
	}
	signed, err := binance.NewSignedRequest(ctx, http.MethodPost, binance.SignedRequestConfig{
		BaseURL:   baseURL,
		APIKey:    request.Credential.APIKey,
		Secret:    request.Credential.Secret,
		Path:      path,
		Params:    params,
		Timestamp: request.Now,
	})
	if err != nil {
		return ExecutionReport{}, err
	}
	sentAt := time.Now().UTC()
	resp, err := client.Do(signed)
	if err != nil {
		return ExecutionReport{}, err
	}
	defer resp.Body.Close()
	raw, err := decodeResponse(resp.Body)
	if err != nil {
		return ExecutionReport{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ExecutionReport{}, fmt.Errorf("binance order returned %s: %s", resp.Status, rawMessage(raw))
	}
	report := ExecutionReport{
		Exchange:        "Binance",
		Environment:     request.Environment,
		Endpoint:        path,
		ValidationOnly:  request.ValidationOnly,
		ClientOrderID:   request.Order.ClientOrderID,
		ExchangeOrderID: stringValue(raw["orderId"]),
		Status:          status,
		Message:         message,
		SentAt:          sentAt.Format(time.RFC3339),
		ReceivedAt:      time.Now().UTC().Format(time.RFC3339),
		Raw:             raw,
	}
	if rawStatus := stringValue(raw["status"]); rawStatus != "" {
		report.Status = strings.ToLower(rawStatus)
		report.Message = "Binance order status: " + rawStatus
	}
	return report, nil
}

func (executor OKXExecutor) Execute(ctx context.Context, request ExecuteRequest) (ExecutionReport, error) {
	if request.Environment != "demo" {
		return ExecutionReport{}, fmt.Errorf("okx live execution supports demo environment only")
	}
	body, err := okxOrderBody(request)
	if err != nil {
		return ExecutionReport{}, err
	}
	if request.ValidationOnly {
		return ExecutionReport{
			Exchange:       "OKX",
			Environment:    request.Environment,
			Endpoint:       "/api/v5/trade/order",
			ValidationOnly: true,
			ClientOrderID:  request.Order.ClientOrderID,
			Status:         "signed-preflight",
			Message:        "OKX has no order/test endpoint; request signed locally without network submit",
			SentAt:         request.Now.UTC().Format(time.RFC3339),
			ReceivedAt:     request.Now.UTC().Format(time.RFC3339),
			Raw: map[string]any{
				"body": body,
			},
		}, nil
	}
	client := executor.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	baseURL := strings.TrimRight(executor.BaseURL, "/")
	if baseURL == "" {
		baseURL = okx.DemoBaseURL
	}
	req, err := okx.NewAuthenticatedRequest(ctx, okx.AuthRequestConfig{
		BaseURL: baseURL,
		Credentials: okx.AuthCredentials{
			APIKey:        request.Credential.APIKey,
			Secret:        request.Credential.Secret,
			APIPassphrase: request.Credential.APIPassphrase,
		},
		Method:      http.MethodPost,
		RequestPath: "/api/v5/trade/order",
		Body:        body,
		Timestamp:   request.Now,
		Demo:        true,
	})
	if err != nil {
		return ExecutionReport{}, err
	}
	sentAt := time.Now().UTC()
	resp, err := client.Do(req)
	if err != nil {
		return ExecutionReport{}, err
	}
	defer resp.Body.Close()
	raw, err := decodeResponse(resp.Body)
	if err != nil {
		return ExecutionReport{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ExecutionReport{}, fmt.Errorf("okx order returned %s: %s", resp.Status, rawMessage(raw))
	}
	code := stringValue(raw["code"])
	if code != "" && code != "0" {
		return ExecutionReport{}, fmt.Errorf("okx order rejected code %s: %s", code, rawMessage(raw))
	}
	orderID, clientOrderID, rowStatus, rowMessage := okxOrderResult(raw)
	if rowStatus != "" && rowStatus != "0" {
		return ExecutionReport{}, fmt.Errorf("okx order rejected code %s: %s", rowStatus, rowMessage)
	}
	if clientOrderID == "" {
		clientOrderID = request.Order.ClientOrderID
	}
	return ExecutionReport{
		Exchange:        "OKX",
		Environment:     request.Environment,
		Endpoint:        "/api/v5/trade/order",
		ValidationOnly:  false,
		ClientOrderID:   clientOrderID,
		ExchangeOrderID: orderID,
		Status:          "submitted",
		Message:         "OKX demo order accepted",
		SentAt:          sentAt.Format(time.RFC3339),
		ReceivedAt:      time.Now().UTC().Format(time.RFC3339),
		Raw:             raw,
	}, nil
}

type okxOrder struct {
	InstID  string `json:"instId"`
	TdMode  string `json:"tdMode"`
	ClOrdID string `json:"clOrdId"`
	Side    string `json:"side"`
	OrdType string `json:"ordType"`
	Sz      string `json:"sz"`
	TgtCcy  string `json:"tgtCcy,omitempty"`
	Tag     string `json:"tag,omitempty"`
}

func okxOrderBody(request ExecuteRequest) (string, error) {
	order := okxOrder{
		InstID:  okxInstID(request.Order.Symbol),
		TdMode:  "cash",
		ClOrdID: request.Order.ClientOrderID,
		Side:    strings.ToLower(string(request.Order.Side)),
		OrdType: strings.ToLower(string(request.Order.OrderType)),
		Tag:     "CCVAR",
	}
	switch request.Order.Side {
	case core.SideBuy:
		order.Sz = formatDecimal(request.Order.SizeUSDT)
		order.TgtCcy = "quote_ccy"
	case core.SideSell:
		if request.Market.Last <= 0 {
			return "", errors.New("market last price is required for OKX sell sizing")
		}
		order.Sz = formatDecimal(request.Order.SizeUSDT / request.Market.Last)
		order.TgtCcy = "base_ccy"
	default:
		return "", errors.New("unsupported OKX order side")
	}
	payload, err := json.Marshal(order)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func decodeResponse(reader io.Reader) (map[string]any, error) {
	body, err := io.ReadAll(io.LimitReader(reader, 1<<20))
	if err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return map[string]any{}, nil
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return map[string]any{"body": string(body)}, nil
	}
	return raw, nil
}

func okxOrderResult(raw map[string]any) (orderID string, clientOrderID string, statusCode string, statusMessage string) {
	rows, ok := raw["data"].([]any)
	if !ok || len(rows) == 0 {
		return "", "", "", ""
	}
	row, ok := rows[0].(map[string]any)
	if !ok {
		return "", "", "", ""
	}
	return stringValue(row["ordId"]), stringValue(row["clOrdId"]), stringValue(row["sCode"]), stringValue(row["sMsg"])
}

func rawMessage(raw map[string]any) string {
	if message := stringValue(raw["msg"]); message != "" {
		return message
	}
	if message := stringValue(raw["message"]); message != "" {
		return message
	}
	if body := stringValue(raw["body"]); body != "" {
		return body
	}
	return fmt.Sprint(raw)
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case float64:
		return strconv.FormatInt(int64(typed), 10)
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

func formatDecimal(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}
