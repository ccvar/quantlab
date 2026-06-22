package paperaccount

import (
	"sort"
	"strings"
	"time"

	"ccvar.com/web3quant/internal/storage"
)

const StartingCapitalUSDT = 100000.0

type Snapshot struct {
	StartingCapitalUSDT float64    `json:"startingCapitalUsdt"`
	CashUSDT            float64    `json:"cashUsdt"`
	EquityUSDT          float64    `json:"equityUsdt"`
	RealizedPnLUSDT     float64    `json:"realizedPnlUsdt"`
	UnrealizedPnLUSDT   float64    `json:"unrealizedPnlUsdt"`
	TotalPnLUSDT        float64    `json:"totalPnlUsdt"`
	ReturnPct           float64    `json:"returnPct"`
	OpenNotionalUSDT    float64    `json:"openNotionalUsdt"`
	FeesUSDT            float64    `json:"feesUsdt"`
	FilledCount         int        `json:"filledCount"`
	RejectedCount       int        `json:"rejectedCount"`
	WinCount            int        `json:"winCount"`
	LossCount           int        `json:"lossCount"`
	Positions           []Position `json:"positions"`
	UpdatedAt           string     `json:"updatedAt"`
}

type Position struct {
	Symbol            string  `json:"symbol"`
	Side              string  `json:"side"`
	Quantity          float64 `json:"quantity"`
	AveragePrice      float64 `json:"averagePrice"`
	MarkPrice         float64 `json:"markPrice"`
	NotionalUSDT      float64 `json:"notionalUsdt"`
	UnrealizedPnLUSDT float64 `json:"unrealizedPnlUsdt"`
	PnLPct            float64 `json:"pnlPct"`
}

type positionState struct {
	symbol   string
	quantity float64
	avgPrice float64
	mark     float64
}

func Build(records []storage.PaperExecutionRecord) Snapshot {
	ordered := append([]storage.PaperExecutionRecord(nil), records...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].ID < ordered[j].ID
	})

	cash := StartingCapitalUSDT
	realized := 0.0
	fees := 0.0
	filled := 0
	rejected := 0
	wins := 0
	losses := 0
	updatedAt := ""
	positions := map[string]*positionState{}

	for _, record := range ordered {
		if record.CreatedAt != "" {
			updatedAt = record.CreatedAt
		}
		if strings.EqualFold(record.RiskStatus, "rejected") {
			rejected++
			continue
		}
		if !strings.EqualFold(record.FillStatus, "filled") || record.FillPrice <= 0 || record.SizeUSDT <= 0 {
			continue
		}
		filled++
		fees += record.FeeUSDT
		symbol := strings.ToUpper(strings.TrimSpace(record.Symbol))
		if symbol == "" {
			symbol = "UNKNOWN"
		}
		position := positions[symbol]
		if position == nil {
			position = &positionState{symbol: symbol}
			positions[symbol] = position
		}
		price := record.FillPrice
		quantity := record.SizeUSDT / price
		position.mark = price

		if strings.EqualFold(record.Side, "sell") {
			cash += record.SizeUSDT - record.FeeUSDT
			realizedPnL := applySell(position, quantity, price)
			realized += realizedPnL
			if realizedPnL > 0 {
				wins++
			} else if realizedPnL < 0 {
				losses++
			}
			continue
		}

		cash -= record.SizeUSDT + record.FeeUSDT
		realizedPnL := applyBuy(position, quantity, price)
		realized += realizedPnL
		if realizedPnL > 0 {
			wins++
		} else if realizedPnL < 0 {
			losses++
		}
	}

	positionViews := make([]Position, 0, len(positions))
	unrealized := 0.0
	openNotional := 0.0
	for _, position := range positions {
		if abs(position.quantity) < 1e-12 {
			continue
		}
		mark := position.mark
		if mark <= 0 {
			mark = position.avgPrice
		}
		pnl := position.quantity * (mark - position.avgPrice)
		notional := abs(position.quantity) * mark
		unrealized += pnl
		openNotional += notional
		side := "long"
		if position.quantity < 0 {
			side = "short"
		}
		pnlPct := 0.0
		if position.avgPrice > 0 {
			pnlPct = (mark - position.avgPrice) / position.avgPrice * 100
			if side == "short" {
				pnlPct = -pnlPct
			}
		}
		positionViews = append(positionViews, Position{
			Symbol:            position.symbol,
			Side:              side,
			Quantity:          abs(position.quantity),
			AveragePrice:      position.avgPrice,
			MarkPrice:         mark,
			NotionalUSDT:      notional,
			UnrealizedPnLUSDT: pnl,
			PnLPct:            pnlPct,
		})
	}
	sort.Slice(positionViews, func(i, j int) bool {
		return positionViews[i].NotionalUSDT > positionViews[j].NotionalUSDT
	})
	if updatedAt == "" {
		updatedAt = time.Now().UTC().Format(time.RFC3339)
	}

	equity := cash
	for _, position := range positions {
		mark := position.mark
		if mark <= 0 {
			mark = position.avgPrice
		}
		equity += position.quantity * mark
	}
	totalPnL := equity - StartingCapitalUSDT
	return Snapshot{
		StartingCapitalUSDT: StartingCapitalUSDT,
		CashUSDT:            cash,
		EquityUSDT:          equity,
		RealizedPnLUSDT:     realized,
		UnrealizedPnLUSDT:   unrealized,
		TotalPnLUSDT:        totalPnL,
		ReturnPct:           totalPnL / StartingCapitalUSDT * 100,
		OpenNotionalUSDT:    openNotional,
		FeesUSDT:            fees,
		FilledCount:         filled,
		RejectedCount:       rejected,
		WinCount:            wins,
		LossCount:           losses,
		Positions:           positionViews,
		UpdatedAt:           updatedAt,
	}
}

func applyBuy(position *positionState, quantity, price float64) float64 {
	if position.quantity >= 0 {
		position.avgPrice = weightedAverage(position.quantity, position.avgPrice, quantity, price)
		position.quantity += quantity
		return 0
	}
	closing := min(quantity, abs(position.quantity))
	realized := (position.avgPrice - price) * closing
	position.quantity += closing
	remaining := quantity - closing
	if remaining > 0 {
		position.quantity = remaining
		position.avgPrice = price
	} else if abs(position.quantity) < 1e-12 {
		position.quantity = 0
		position.avgPrice = 0
	}
	return realized
}

func applySell(position *positionState, quantity, price float64) float64 {
	if position.quantity <= 0 {
		position.avgPrice = weightedAverage(abs(position.quantity), position.avgPrice, quantity, price)
		position.quantity -= quantity
		return 0
	}
	closing := min(quantity, position.quantity)
	realized := (price - position.avgPrice) * closing
	position.quantity -= closing
	remaining := quantity - closing
	if remaining > 0 {
		position.quantity = -remaining
		position.avgPrice = price
	} else if abs(position.quantity) < 1e-12 {
		position.quantity = 0
		position.avgPrice = 0
	}
	return realized
}

func weightedAverage(existingQty, existingPrice, addQty, addPrice float64) float64 {
	total := existingQty + addQty
	if total <= 0 {
		return addPrice
	}
	return (existingQty*existingPrice + addQty*addPrice) / total
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func abs(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}
