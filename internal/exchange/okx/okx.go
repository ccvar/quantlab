package okx

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"ccvar.com/web3quant/internal/core"
	"ccvar.com/web3quant/internal/exchange"
)

const defaultBaseURL = "https://www.okx.com"

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
	return "OKX"
}

func (adapter Adapter) FetchSnapshot(ctx context.Context, symbol string) (core.MarketSnapshot, error) {
	instID := normalizeInstID(symbol)
	var payload struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data []struct {
			InstID    string `json:"instId"`
			Last      string `json:"last"`
			AskPx     string `json:"askPx"`
			BidPx     string `json:"bidPx"`
			VolCcy24h string `json:"volCcy24h"`
			TS        string `json:"ts"`
		} `json:"data"`
	}
	if err := adapter.get(ctx, "/api/v5/market/ticker", map[string]string{"instId": instID}, &payload); err != nil {
		return core.MarketSnapshot{}, err
	}
	if payload.Code != "0" {
		return core.MarketSnapshot{}, fmt.Errorf("okx returned code %s: %s", payload.Code, payload.Msg)
	}
	if len(payload.Data) == 0 {
		return core.MarketSnapshot{}, fmt.Errorf("okx returned no ticker data")
	}
	row := payload.Data[0]
	bid, err := parseFloat(row.BidPx)
	if err != nil {
		return core.MarketSnapshot{}, fmt.Errorf("parse bid: %w", err)
	}
	ask, err := parseFloat(row.AskPx)
	if err != nil {
		return core.MarketSnapshot{}, fmt.Errorf("parse ask: %w", err)
	}
	last, err := parseFloat(row.Last)
	if err != nil || last == 0 {
		last = (bid + ask) / 2
	}
	liquidity, _ := parseFloat(row.VolCcy24h)
	observedAt := time.Now().UTC()
	if ms, err := strconv.ParseInt(row.TS, 10, 64); err == nil && ms > 0 {
		observedAt = time.UnixMilli(ms).UTC()
	}
	return core.MarketSnapshot{
		Exchange:      adapter.Name(),
		Symbol:        instID,
		BestBid:       bid,
		BestAsk:       ask,
		Last:          last,
		SpreadPct:     spreadPct(bid, ask),
		LiquidityUSDT: liquidity,
		ObservedAt:    observedAt,
	}, nil
}

func (adapter Adapter) FetchCandles(ctx context.Context, symbol, interval string, limit int) ([]core.Candle, error) {
	instID := normalizeInstID(symbol)
	if interval == "" {
		interval = "15m"
	}
	if limit <= 0 || limit > 1440 {
		limit = 96
	}
	var payload struct {
		Code string     `json:"code"`
		Msg  string     `json:"msg"`
		Data [][]string `json:"data"`
	}
	if err := adapter.get(ctx, "/api/v5/market/candles", map[string]string{
		"instId": instID,
		"bar":    interval,
		"limit":  strconv.Itoa(limit),
	}, &payload); err != nil {
		return nil, err
	}
	if payload.Code != "0" {
		return nil, fmt.Errorf("okx returned code %s: %s", payload.Code, payload.Msg)
	}
	candles := make([]core.Candle, 0, len(payload.Data))
	for _, row := range payload.Data {
		if len(row) < 6 {
			continue
		}
		ts, err := strconv.ParseInt(row[0], 10, 64)
		if err != nil {
			return nil, err
		}
		open, err := parseFloat(row[1])
		if err != nil {
			return nil, err
		}
		high, err := parseFloat(row[2])
		if err != nil {
			return nil, err
		}
		low, err := parseFloat(row[3])
		if err != nil {
			return nil, err
		}
		closePrice, err := parseFloat(row[4])
		if err != nil {
			return nil, err
		}
		volume, err := parseFloat(row[5])
		if err != nil {
			return nil, err
		}
		candles = append(candles, core.Candle{
			Time:   ts / 1000,
			Open:   open,
			High:   high,
			Low:    low,
			Close:  closePrice,
			Volume: volume,
		})
	}
	sort.Slice(candles, func(i, j int) bool {
		return candles[i].Time < candles[j].Time
	})
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
		return fmt.Errorf("okx returned %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(target)
}

func normalizeInstID(symbol string) string {
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
