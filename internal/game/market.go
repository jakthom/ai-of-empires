package game

import (
	"fmt"
	"math"
)

type exchangeQuote struct {
	Cost, Gain  Resources
	Description string
}

func (w *World) tradeHome(cart *Entity) *Entity {
	if cart == nil {
		return nil
	}
	return w.nearest(cart.Position, func(t *Entity) bool {
		return t.Type == tradePostType(cart) && t.Owner == cart.Owner && t.life.State() == Active && w.reachableFootprint(cart, t.Position, definitions[t.Type].Radius+.7)
	})
}

// Quote and execute at the same boundary so the UI never calculates prices.
func (w *World) quoteExchange(p *Player, market *Entity, kind, resource string) (exchangeQuote, error) {
	q := exchangeQuote{}
	if resource != "food" && resource != "wood" && resource != "stone" {
		return q, rule("invalid_resource", "Choose food, wood, or stone.")
	}
	if market != nil && market.Owner != p.ID {
		return q, rule("invalid_producer", "Select your completed Market.")
	}
	return w.quoteMerchant(p, w.Marketplace.Regions[-p.ID], market, kind, resource)
}

func (w *World) quoteMerchant(p *Player, region *merchantRegion, market *Entity, kind, resource string) (exchangeQuote, error) {
	q := exchangeQuote{}
	if region == nil || (resource != "food" && resource != "wood" && resource != "stone") {
		return q, rule("invalid_resource", "Choose food, wood, or stone at an available Market.")
	}
	price := merchantPrice(region, resource)
	switch kind {
	case "market_sell":
		q.Cost.Deposit(resource, 100)
		q.Gain.Gold = math.Floor(price * .7)
		if p.Civilization == "saracens" {
			q.Gain.Gold = math.Floor(price * .84)
		}
		q.Description = fmt.Sprintf("Exchange 100 %s for %.0f gold.", resource, q.Gain.Gold)
	case "market_buy":
		q.Cost.Gold = math.Ceil(price * 1.3)
		q.Gain.Deposit(resource, 100)
		q.Description = fmt.Sprintf("Exchange %.0f gold for 100 %s.", q.Cost.Gold, resource)
	default:
		return q, rule("invalid_exchange", "Choose a market purchase or sale.")
	}
	if !tradingPost(market) || (market.Owner != p.ID && market.Owner != 0) || market.life.State() != Active {
		return q, rule("invalid_producer", "Build or select a completed Market.")
	}
	if market.Type == "dock" && p.Age < 1 {
		return q, rule("age_required", "Dock trade requires the Feudal Age.")
	}
	if !region.Stock.CanPay(q.Gain) {
		return q, rule("merchant_stock", "This Market cannot fund the lot. Try another partner or wait for supplies.")
	}
	for _, res := range []string{"food", "wood", "gold", "stone"} {
		if q.Cost.Amount(res) > w.merchantRoom(region, res) {
			return q, rule("merchant_full", "This Market has enough of that resource. Try another partner or wait for demand.")
		}
	}
	if !p.Resources.CanPay(q.Cost) {
		return q, rule("insufficient_resources", "Not enough resources. "+q.Description)
	}
	return q, nil
}

func (w *World) exchange(p *Player, market *Entity, kind, resource string) error {
	q, err := w.quoteExchange(p, market, kind, resource)
	if err != nil {
		return err
	}
	w.consume(p.ID, q.Cost, "trade")
	p.Economy.TradeSold.Add(q.Cost)
	p.Economy.TradeBought.Add(q.Gain)
	p.Resources.Add(q.Gain)
	r := w.Marketplace.Regions[-p.ID]
	r.Stock.Add(q.Cost)
	r.Stock.Add(q.Gain.Scale(-1))
	r.Revision++
	w.record(Event{Kind: "exchange", Message: q.Description, Resource: resource}, market, 0)
	return nil
}
