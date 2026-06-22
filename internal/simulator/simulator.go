package simulator

import (
	"fmt"
	"time"

	"ccvar.com/web3quant/internal/core"
	"ccvar.com/web3quant/internal/risk"
)

type FillModel struct {
	FeeRate     float64
	SlippagePct float64
	Now         func() time.Time
}

func (model FillModel) Fill(intent core.TradeIntent, market core.MarketSnapshot, decision risk.Decision) (core.Fill, error) {
	if !decision.Approved {
		return core.Fill{}, fmt.Errorf("risk rejected intent: %s", decision.ReasonText())
	}
	if intent.Side == core.SideHold {
		return core.Fill{}, fmt.Errorf("hold intent is not fillable")
	}

	price := intent.Price
	if intent.OrderType == core.OrderMarket || price == 0 {
		price = market.Last
	}
	if intent.Side == core.SideBuy {
		price = price * (1 + model.SlippagePct)
	} else {
		price = price * (1 - model.SlippagePct)
	}

	now := model.Now
	if now == nil {
		now = time.Now
	}
	return core.Fill{
		OrderID:      intent.ID,
		Symbol:       intent.Symbol,
		Side:         intent.Side,
		Price:        price,
		SizeUSDT:     intent.SizeUSDT,
		FeeUSDT:      intent.SizeUSDT * model.FeeRate,
		SlippageUSDT: intent.SizeUSDT * model.SlippagePct,
		FilledAt:     now(),
	}, nil
}
