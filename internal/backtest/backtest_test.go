package backtest

import (
	"errors"
	"math"
	"testing"
	"time"

	"ccvar.com/web3quant/internal/core"
)

func TestRunLongMomentumBacktest(t *testing.T) {
	result, err := Run(Config{
		StrategyName:  "QA Momentum",
		Exchange:      "Binance",
		Symbol:        "BTCUSDT",
		Side:          core.SideBuy,
		OrderSizeUSDT: 1000,
		FastWindow:    2,
		SlowWindow:    3,
		FeeRate:       0.001,
		SlippagePct:   0.001,
	}, candlesFromCloses([]float64{100, 101, 102, 104, 106, 105, 103, 101}), time.Unix(10, 0))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Summary.TradeCount != 1 || len(result.Trades) != 1 {
		t.Fatalf("trades = %#v summary=%#v", result.Trades, result.Summary)
	}
	if result.Trades[0].Side != "buy" || result.Trades[0].Status != "closed" {
		t.Fatalf("trade = %#v", result.Trades[0])
	}
	if result.Summary.EndingEquityUSDT <= result.Summary.StartingCapitalUSDT {
		t.Fatalf("ending equity %.8f <= starting %.8f", result.Summary.EndingEquityUSDT, result.Summary.StartingCapitalUSDT)
	}
	if result.Summary.WinCount != 1 || result.Summary.LossCount != 0 {
		t.Fatalf("win/loss = %d/%d", result.Summary.WinCount, result.Summary.LossCount)
	}
	if len(result.Equity) != 8 || result.Equity[0].Benchmark != 100000 {
		t.Fatalf("equity = %#v", result.Equity)
	}
	if result.Summary.GeneratedAt != "1970-01-01T00:00:10Z" {
		t.Fatalf("generatedAt = %q", result.Summary.GeneratedAt)
	}
}

func TestRunShortMomentumBacktest(t *testing.T) {
	result, err := Run(Config{
		Exchange:      "OKX",
		Symbol:        "ETH-USDT",
		Side:          core.SideSell,
		OrderSizeUSDT: 1000,
		FastWindow:    2,
		SlowWindow:    3,
		FeeRate:       0.0005,
		SlippagePct:   0.0002,
	}, candlesFromCloses([]float64{110, 109, 108, 106, 104, 105, 107, 109}), time.Unix(20, 0))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Summary.TradeCount != 1 || result.Trades[0].Side != "sell" {
		t.Fatalf("result = %#v", result)
	}
	if result.Trades[0].PnLUSDT <= 0 {
		t.Fatalf("short pnl = %.8f, want positive", result.Trades[0].PnLUSDT)
	}
	if result.Summary.Exchange != "OKX" || result.Summary.Symbol != "ETH-USDT" {
		t.Fatalf("summary = %#v", result.Summary)
	}
}

func TestRunRejectsTooFewCandles(t *testing.T) {
	_, err := Run(Config{FastWindow: 2, SlowWindow: 5}, candlesFromCloses([]float64{100, 101, 102}), time.Time{})
	if !errors.Is(err, ErrNotEnoughCandles) {
		t.Fatalf("Run() err = %v, want ErrNotEnoughCandles", err)
	}
}

func candlesFromCloses(closes []float64) []core.Candle {
	candles := make([]core.Candle, 0, len(closes))
	for index, closePrice := range closes {
		candles = append(candles, core.Candle{
			Time:   int64(1700000000 + index*900),
			Open:   closePrice,
			High:   closePrice + 1,
			Low:    math.Max(closePrice-1, 0.01),
			Close:  closePrice,
			Volume: 100,
		})
	}
	return candles
}
