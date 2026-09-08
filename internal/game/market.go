package game

import "fmt"

type exchangeQuote struct {
	Cost, Gain  Resources
	Description string
}

func (w *World) tradeHome(cart *Entity) *Entity {
	return w.nearest(cart.Position, func(t *Entity) bool {
		return t.Type == "market" && t.Owner == cart.Owner && t.life.State() == Active && w.reachableFootprint(cart, t.Position, definitions[t.Type].Radius+.7)
	})
}

// Quote and execute at the same boundary so the UI never calculates prices.
func (w *World) quoteExchange(p *Player, market *Entity, kind, resource string) (exchangeQuote, error) {
	q := exchangeQuote{}
	if market.Type != "market" || market.Owner != p.ID || market.life.State() != Active {
		return q, rule("invalid_producer", "Select a completed market.")
	}
	if resource != "food" && resource != "wood" && resource != "stone" {
		return q, rule("invalid_resource", "Choose food, wood, or stone.")
	}
	switch kind {
	case "market_sell":
		q.Cost.Deposit(resource, 100)
		q.Gain.Gold = 70
		if p.Civilization == "saracens" {
			q.Gain.Gold = 84
		}
		q.Description = fmt.Sprintf("Exchange 100 %s for %.0f gold.", resource, q.Gain.Gold)
	case "market_buy":
		q.Cost.Gold = 130
		q.Gain.Deposit(resource, 100)
		q.Description = fmt.Sprintf("Exchange 130 gold for 100 %s.", resource)
	default:
		return q, rule("invalid_exchange", "Choose a market purchase or sale.")
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
	p.Resources.Add(q.Cost.Scale(-1))
	p.Resources.Add(q.Gain)
	w.record(Event{Kind: "exchange", Message: q.Description, Resource: resource}, market, 0)
	return nil
}
