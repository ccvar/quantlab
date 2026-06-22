package binance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"ccvar.com/web3quant/internal/core"
	"ccvar.com/web3quant/internal/exchange"
)

const defaultBaseURL = "https://data-api.binance.vision"

type Adapter struct {
	BaseURL string
	Client  *http.Client
}

func New() Adapter {
	return Adapter{
		BaseURL: defaultBaseURL,
		Client:  &http.Client{Timeout: 7 * time.Second},
	}
}

func (adapter Adapter) Name() string {
	return "Binance"
}

func (adapter Adapter) FetchSnapshot(ctx context.Context, symbol string) (core.MarketSnapshot, error) {
	symbol = normalizeSymbol(symbol)
	var book struct {
		Symbol   string `json:"symbol"`
		BidPrice string `json:"bidPrice"`
		AskPrice string `json:"askPrice"`
		BidQty   string `json:"bidQty"`
		AskQty   string `json:"askQty"`
	}
	if err := adapter.get(ctx, "/api/v3/ticker/bookTicker", map[string]string{"symbol": symbol}, &book); err != nil {
		return core.MarketSnapshot{}, err
	}
	var ticker struct {
		LastPrice   string `json:"lastPrice"`
		QuoteVolume string `json:"quoteVolume"`
		CloseTime   int64  `json:"closeTime"`
	}
	if err := adapter.get(ctx, "/api/v3/ticker/24hr", map[string]string{"symbol": symbol}, &ticker); err != nil {
		return core.MarketSnapshot{}, err
	}

	bid, err := parseFloat(book.BidPrice)
	if err != nil {
		return core.MarketSnapshot{}, fmt.Errorf("parse bid: %w", err)
	}
	ask, err := parseFloat(book.AskPrice)
	if err != nil {
		return core.MarketSnapshot{}, fmt.Errorf("parse ask: %w", err)
	}
	last, err := parseFloat(ticker.LastPrice)
	if err != nil || last == 0 {
		last = (bid + ask) / 2
	}
	liquidity, _ := parseFloat(ticker.QuoteVolume)
	observedAt := time.Now().UTC()
	if ticker.CloseTime > 0 {
		observedAt = time.UnixMilli(ticker.CloseTime).UTC()
	}
	return core.MarketSnapshot{
		Exchange:      adapter.Name(),
		Symbol:        symbol,
		BestBid:       bid,
		BestAsk:       ask,
		Last:          last,
		SpreadPct:     spreadPct(bid, ask),
		LiquidityUSDT: liquidity,
		ObservedAt:    observedAt,
	}, nil
}

func (adapter Adapter) FetchCandles(ctx context.Context, symbol, interval string, limit int) ([]core.Candle, error) {
	symbol = normalizeSymbol(symbol)
	if interval == "" {
		interval = "15m"
	}
	if limit <= 0 || limit > 1000 {
		limit = 96
	}
	var payload [][]any
	err := adapter.get(ctx, "/api/v3/klines", map[string]string{
		"symbol":   symbol,
		"interval": interval,
		"limit":    strconv.Itoa(limit),
	}, &payload)
	if err != nil {
		return nil, err
	}
	candles := make([]core.Candle, 0, len(payload))
	for _, row := range payload {
		if len(row) < 6 {
			continue
		}
		ts, err := numberAt(row, 0)
		if err != nil {
			return nil, err
		}
		open, err := floatAt(row, 1)
		if err != nil {
			return nil, err
		}
		high, err := floatAt(row, 2)
		if err != nil {
			return nil, err
		}
		low, err := floatAt(row, 3)
		if err != nil {
			return nil, err
		}
		closePrice, err := floatAt(row, 4)
		if err != nil {
			return nil, err
		}
		volume, err := floatAt(row, 5)
		if err != nil {
			return nil, err
		}
		candles = append(candles, core.Candle{
			Time:   int64(ts / 1000),
			Open:   open,
			High:   high,
			Low:    low,
			Close:  closePrice,
			Volume: volume,
		})
	}
	return candles, nil
}

func (adapter Adapter) PlaceOrder(context.Context, core.OrderRequest) (core.Fill, error) {
	return core.Fill{}, exchange.ErrTradingDisabled
}

func (adapter Adapter) CancelOrder(context.Context, string) error {
	return exchange.ErrTradingDisabled
}

func (adapter Adapter) get(ctx context.Context, path string, query map[string]string, target any) error {
	client := adapter.Client
	if client == nil {
		client = &http.Client{Timeout: 7 * time.Second}
	}
	base := adapter.BaseURL
	if base == "" {
		base = defaultBaseURL
	}
	endpoint, err := url.Parse(strings.TrimRight(base, "/") + path)
	if err != nil {
		return err
	}
	values := endpoint.Query()
	for key, value := range query {
		values.Set(key, value)
	}
	endpoint.RawQuery = values.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("binance returned %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(target)
}

func normalizeSymbol(symbol string) string {
	return strings.ToUpper(strings.ReplaceAll(symbol, "-", ""))
}

func spreadPct(bid, ask float64) float64 {
	mid := (bid + ask) / 2
	if mid <= 0 {
		return 0
	}
	return (ask - bid) / mid * 100
}

func parseFloat(value string) (float64, error) {
	return strconv.ParseFloat(value, 64)
}

func floatAt(row []any, index int) (float64, error) {
	switch value := row[index].(type) {
	case string:
		return strconv.ParseFloat(value, 64)
	case float64:
		return value, nil
	default:
		return 0, fmt.Errorf("unexpected value at %d: %T", index, value)
	}
}

func numberAt(row []any, index int) (float64, error) {
	switch value := row[index].(type) {
	case float64:
		return value, nil
	case string:
		return strconv.ParseFloat(value, 64)
	default:
		return 0, fmt.Errorf("unexpected number at %d: %T", index, value)
	}
}
