package core

import "time"

type TradingMode string

const (
	ModeShadow TradingMode = "shadow"
	ModePaper  TradingMode = "paper"
	ModeLive   TradingMode = "live"
)

type Side string

const (
	SideBuy  Side = "buy"
	SideSell Side = "sell"
	SideHold Side = "hold"
)

type OrderType string

const (
	OrderMarket OrderType = "market"
	OrderLimit  OrderType = "limit"
)

type TradeIntent struct {
	ID          string        `json:"id"`
	Mode        TradingMode   `json:"mode"`
	Exchange    string        `json:"exchange"`
	Symbol      string        `json:"symbol"`
	Side        Side          `json:"side"`
	OrderType   OrderType     `json:"orderType"`
	Price       float64       `json:"price"`
	SizeUSDT    float64       `json:"sizeUsdt"`
	Confidence  float64       `json:"confidence"`
	MaxSlippage float64       `json:"maxSlippage"`
	Reason      string        `json:"reason"`
	TTL         time.Duration `json:"ttl"`
	GeneratedAt time.Time     `json:"generatedAt"`
}

type AccountState struct {
	EquityUSDT          float64            `json:"equityUsdt"`
	AvailableUSDT       float64            `json:"availableUsdt"`
	DailyPnLUSDT        float64            `json:"dailyPnlUsdt"`
	DailyDrawdownPct    float64            `json:"dailyDrawdownPct"`
	OpenNotionalUSDT    float64            `json:"openNotionalUsdt"`
	SymbolExposureUSDT  map[string]float64 `json:"symbolExposureUsdt"`
	ConsecutiveLosses   int                `json:"consecutiveLosses"`
	LiveTradingUnlocked bool               `json:"liveTradingUnlocked"`
}

type MarketSnapshot struct {
	Exchange       string    `json:"exchange"`
	Symbol         string    `json:"symbol"`
	BestBid        float64   `json:"bestBid"`
	BestAsk        float64   `json:"bestAsk"`
	Last           float64   `json:"last"`
	SpreadPct      float64   `json:"spreadPct"`
	LiquidityUSDT  float64   `json:"liquidityUsdt"`
	FundingRatePct float64   `json:"fundingRatePct"`
	ObservedAt     time.Time `json:"observedAt"`
}

type Candle struct {
	Time   int64   `json:"time"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume float64 `json:"volume"`
}

type OrderRequest struct {
	ClientOrderID string    `json:"clientOrderId"`
	Exchange      string    `json:"exchange"`
	Symbol        string    `json:"symbol"`
	Side          Side      `json:"side"`
	OrderType     OrderType `json:"orderType"`
	Price         float64   `json:"price"`
	SizeUSDT      float64   `json:"sizeUsdt"`
}

type Fill struct {
	OrderID      string    `json:"orderId"`
	Symbol       string    `json:"symbol"`
	Side         Side      `json:"side"`
	Price        float64   `json:"price"`
	SizeUSDT     float64   `json:"sizeUsdt"`
	FeeUSDT      float64   `json:"feeUsdt"`
	SlippageUSDT float64   `json:"slippageUsdt"`
	FilledAt     time.Time `json:"filledAt"`
}
