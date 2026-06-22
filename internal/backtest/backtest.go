package backtest

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"ccvar.com/web3quant/internal/core"
)

const DefaultStartingCapitalUSDT = 100000.0

var ErrNotEnoughCandles = errors.New("backtest requires at least slow window plus two candles")

type Config struct {
	StrategyName        string    `json:"strategyName"`
	Exchange            string    `json:"exchange"`
	Symbol              string    `json:"symbol"`
	Side                core.Side `json:"side"`
	Interval            string    `json:"interval"`
	OrderSizeUSDT       float64   `json:"orderSizeUsdt"`
	StartingCapitalUSDT float64   `json:"startingCapitalUsdt"`
	FastWindow          int       `json:"fastWindow"`
	SlowWindow          int       `json:"slowWindow"`
	FeeRate             float64   `json:"feeRate"`
	SlippagePct         float64   `json:"slippagePct"`
}

type Result struct {
	RunID   int64         `json:"runId,omitempty"`
	Summary Summary       `json:"summary"`
	Equity  []EquityPoint `json:"equity"`
	Trades  []Trade       `json:"trades"`
}

type Summary struct {
	StrategyName        string  `json:"strategyName"`
	Exchange            string  `json:"exchange"`
	Symbol              string  `json:"symbol"`
	Interval            string  `json:"interval"`
	CandleCount         int     `json:"candleCount"`
	StartingCapitalUSDT float64 `json:"startingCapitalUsdt"`
	EndingEquityUSDT    float64 `json:"endingEquityUsdt"`
	TotalPnLUSDT        float64 `json:"totalPnlUsdt"`
	ReturnPct           float64 `json:"returnPct"`
	BenchmarkReturnPct  float64 `json:"benchmarkReturnPct"`
	MaxDrawdownPct      float64 `json:"maxDrawdownPct"`
	FeesUSDT            float64 `json:"feesUsdt"`
	TradeCount          int     `json:"tradeCount"`
	WinCount            int     `json:"winCount"`
	LossCount           int     `json:"lossCount"`
	WinRatePct          float64 `json:"winRatePct"`
	ExposureTimePct     float64 `json:"exposureTimePct"`
	FastWindow          int     `json:"fastWindow"`
	SlowWindow          int     `json:"slowWindow"`
	MarketDataSource    string  `json:"marketDataSource"`
	Warning             string  `json:"warning,omitempty"`
	GeneratedAt         string  `json:"generatedAt"`
}

type EquityPoint struct {
	Time      int64   `json:"time"`
	Equity    float64 `json:"equity"`
	Benchmark float64 `json:"benchmark"`
}

type Trade struct {
	ID           string  `json:"id"`
	Side         string  `json:"side"`
	OpenedAt     string  `json:"openedAt"`
	ClosedAt     string  `json:"closedAt"`
	EntryPrice   float64 `json:"entryPrice"`
	ExitPrice    float64 `json:"exitPrice"`
	Quantity     float64 `json:"quantity"`
	NotionalUSDT float64 `json:"notionalUsdt"`
	PnLUSDT      float64 `json:"pnlUsdt"`
	ReturnPct    float64 `json:"returnPct"`
	FeesUSDT     float64 `json:"feesUsdt"`
	BarsHeld     int     `json:"barsHeld"`
	Status       string  `json:"status"`
}

type openPosition struct {
	id         string
	side       core.Side
	openedAt   int64
	entryPrice float64
	quantity   float64
	notional   float64
	entryFee   float64
	entryIndex int
}

func Run(config Config, candles []core.Candle, generatedAt time.Time) (Result, error) {
	config = normalizeConfig(config)
	ordered := normalizeCandles(candles)
	if len(ordered) < config.SlowWindow+2 {
		return Result{}, ErrNotEnoughCandles
	}
	for _, candle := range ordered {
		if candle.Close <= 0 {
			return Result{}, fmt.Errorf("invalid candle close %.8f", candle.Close)
		}
	}
	if generatedAt.IsZero() {
		generatedAt = time.Now().UTC()
	}

	cash := config.StartingCapitalUSDT
	fees := 0.0
	peak := config.StartingCapitalUSDT
	maxDrawdown := 0.0
	exposureBars := 0
	var position *openPosition
	trades := []Trade{}
	equity := make([]EquityPoint, 0, len(ordered))
	firstClose := ordered[0].Close

	for index, candle := range ordered {
		if index >= config.SlowWindow-1 {
			fast := averageClose(ordered, index, config.FastWindow)
			slow := averageClose(ordered, index, config.SlowWindow)
			if position == nil && shouldEnter(config.Side, fast, slow) {
				opened, fee := openTrade(config, candle, index, len(trades)+1)
				position = &opened
				fees += fee
				cash = applyOpenCash(cash, opened)
			} else if position != nil && shouldExit(position.side, fast, slow) {
				closed, exitFee := closeTrade(*position, config, candle, index, "closed")
				fees += exitFee
				cash = applyCloseCash(cash, *position, closed.ExitPrice, exitFee)
				trades = append(trades, closed)
				position = nil
			}
		}
		if position != nil {
			exposureBars++
		}
		currentEquity := markEquity(cash, position, candle.Close)
		benchmark := config.StartingCapitalUSDT * candle.Close / firstClose
		equity = append(equity, EquityPoint{Time: candle.Time, Equity: currentEquity, Benchmark: benchmark})
		if currentEquity > peak {
			peak = currentEquity
		}
		if peak > 0 {
			drawdown := (peak - currentEquity) / peak * 100
			if drawdown > maxDrawdown {
				maxDrawdown = drawdown
			}
		}
	}

	last := ordered[len(ordered)-1]
	if position != nil {
		closed, exitFee := closeTrade(*position, config, last, len(ordered)-1, "closed_end")
		fees += exitFee
		cash = applyCloseCash(cash, *position, closed.ExitPrice, exitFee)
		trades = append(trades, closed)
		position = nil
		equity[len(equity)-1].Equity = cash
	}

	wins := 0
	losses := 0
	for _, trade := range trades {
		if trade.PnLUSDT > 0 {
			wins++
		} else if trade.PnLUSDT < 0 {
			losses++
		}
	}
	winRate := 0.0
	if len(trades) > 0 {
		winRate = float64(wins) / float64(len(trades)) * 100
	}
	ending := equity[len(equity)-1].Equity
	benchmarkReturn := (last.Close/firstClose - 1) * 100
	return Result{
		Summary: Summary{
			StrategyName:        config.StrategyName,
			Exchange:            config.Exchange,
			Symbol:              config.Symbol,
			Interval:            config.Interval,
			CandleCount:         len(ordered),
			StartingCapitalUSDT: config.StartingCapitalUSDT,
			EndingEquityUSDT:    ending,
			TotalPnLUSDT:        ending - config.StartingCapitalUSDT,
			ReturnPct:           (ending/config.StartingCapitalUSDT - 1) * 100,
			BenchmarkReturnPct:  benchmarkReturn,
			MaxDrawdownPct:      maxDrawdown,
			FeesUSDT:            fees,
			TradeCount:          len(trades),
			WinCount:            wins,
			LossCount:           losses,
			WinRatePct:          winRate,
			ExposureTimePct:     float64(exposureBars) / float64(len(ordered)) * 100,
			FastWindow:          config.FastWindow,
			SlowWindow:          config.SlowWindow,
			GeneratedAt:         generatedAt.UTC().Format(time.RFC3339),
		},
		Equity: equity,
		Trades: trades,
	}, nil
}

func normalizeConfig(config Config) Config {
	config.StrategyName = defaultString(config.StrategyName, "AI Momentum Backtest")
	config.Exchange = defaultString(config.Exchange, "Binance")
	config.Symbol = strings.ToUpper(defaultString(config.Symbol, "BTCUSDT"))
	config.Interval = defaultString(config.Interval, "15m")
	if config.Side != core.SideSell {
		config.Side = core.SideBuy
	}
	if config.OrderSizeUSDT <= 0 {
		config.OrderSizeUSDT = 500
	}
	if config.StartingCapitalUSDT <= 0 {
		config.StartingCapitalUSDT = DefaultStartingCapitalUSDT
	}
	if config.FastWindow <= 0 {
		config.FastWindow = 6
	}
	if config.SlowWindow <= config.FastWindow {
		config.SlowWindow = config.FastWindow * 3
	}
	if config.FeeRate < 0 {
		config.FeeRate = 0
	}
	if config.FeeRate == 0 {
		config.FeeRate = 0.0005
	}
	if config.SlippagePct < 0 {
		config.SlippagePct = 0
	}
	if config.SlippagePct == 0 {
		config.SlippagePct = 0.0002
	}
	return config
}

func normalizeCandles(candles []core.Candle) []core.Candle {
	ordered := append([]core.Candle(nil), candles...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].Time < ordered[j].Time
	})
	return ordered
}

func averageClose(candles []core.Candle, index, window int) float64 {
	start := index - window + 1
	sum := 0.0
	for i := start; i <= index; i++ {
		sum += candles[i].Close
	}
	return sum / float64(window)
}

func shouldEnter(side core.Side, fast, slow float64) bool {
	if side == core.SideSell {
		return fast < slow
	}
	return fast > slow
}

func shouldExit(side core.Side, fast, slow float64) bool {
	if side == core.SideSell {
		return fast >= slow
	}
	return fast <= slow
}

func openTrade(config Config, candle core.Candle, index, number int) (openPosition, float64) {
	price := candle.Close
	if config.Side == core.SideSell {
		price *= 1 - config.SlippagePct
	} else {
		price *= 1 + config.SlippagePct
	}
	quantity := config.OrderSizeUSDT / price
	fee := config.OrderSizeUSDT * config.FeeRate
	return openPosition{
		id:         fmt.Sprintf("bt-%03d", number),
		side:       config.Side,
		openedAt:   candle.Time,
		entryPrice: price,
		quantity:   quantity,
		notional:   config.OrderSizeUSDT,
		entryFee:   fee,
		entryIndex: index,
	}, fee
}

func closeTrade(position openPosition, config Config, candle core.Candle, index int, status string) (Trade, float64) {
	exitPrice := candle.Close
	if position.side == core.SideSell {
		exitPrice *= 1 + config.SlippagePct
	} else {
		exitPrice *= 1 - config.SlippagePct
	}
	exitNotional := position.quantity * exitPrice
	exitFee := exitNotional * config.FeeRate
	gross := (exitPrice - position.entryPrice) * position.quantity
	if position.side == core.SideSell {
		gross = (position.entryPrice - exitPrice) * position.quantity
	}
	totalFees := position.entryFee + exitFee
	pnl := gross - totalFees
	return Trade{
		ID:           position.id,
		Side:         string(position.side),
		OpenedAt:     formatUnix(position.openedAt),
		ClosedAt:     formatUnix(candle.Time),
		EntryPrice:   position.entryPrice,
		ExitPrice:    exitPrice,
		Quantity:     position.quantity,
		NotionalUSDT: position.notional,
		PnLUSDT:      pnl,
		ReturnPct:    pnl / position.notional * 100,
		FeesUSDT:     totalFees,
		BarsHeld:     index - position.entryIndex + 1,
		Status:       status,
	}, exitFee
}

func applyOpenCash(cash float64, position openPosition) float64 {
	if position.side == core.SideSell {
		return cash + position.notional - position.entryFee
	}
	return cash - position.notional - position.entryFee
}

func applyCloseCash(cash float64, position openPosition, exitPrice, exitFee float64) float64 {
	exitNotional := position.quantity * exitPrice
	if position.side == core.SideSell {
		return cash - exitNotional - exitFee
	}
	return cash + exitNotional - exitFee
}

func markEquity(cash float64, position *openPosition, markPrice float64) float64 {
	if position == nil {
		return cash
	}
	value := position.quantity * markPrice
	if position.side == core.SideSell {
		return cash - value
	}
	return cash + value
}

func formatUnix(value int64) string {
	if value <= 0 {
		return ""
	}
	return time.Unix(value, 0).UTC().Format(time.RFC3339)
}

func defaultString(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func Round(value float64) float64 {
	return math.Round(value*1e8) / 1e8
}
