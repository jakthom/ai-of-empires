package game

// Commerce uses the same published book and command boundary as human/MCP
// players. A kingdom advertises a surplus against its own shortage; it never
// looks at another kingdom's stockpile or private offers.
func (w *World) aiCommerce(c *aiContext) {
	p := c.Player
	if w.Time < p.AIPlan.NextTradeAt {
		return
	}
	p.AIPlan.NextTradeAt = w.Time + 20
	markets := w.entities(p.ID, "market")
	var market *Entity
	for _, m := range markets {
		if m.life.State() == Active {
			market = m
			break
		}
	}
	if market == nil {
		return
	}
	floor := Resources{Food: 400, Wood: 400, Gold: 250, Stone: 200}
	book := w.marketplaceView(p.ID)
	for _, offer := range book.Offers {
		t := offer.Terms
		if offer.Owner == p.ID {
			if offer.State == string(offerOpen) && p.Resources.Amount(t.WantResource) > floor.Amount(t.WantResource)*2 {
				_ = w.Apply(p.ID, Command{Kind: "market_cancel", OfferID: offer.ID})
			}
			continue
		}
		if p.Resources.Amount(t.GiveResource) >= floor.Amount(t.GiveResource) || p.Resources.Amount(t.WantResource)-float64(t.WantAmount) < floor.Amount(t.WantResource) || t.WantAmount > t.GiveAmount*2 {
			continue
		}
		if offer.CanAccept {
			_ = w.Apply(p.ID, Command{Kind: "market_accept", OfferID: offer.ID, EntityIDs: []int{offer.CartID}})
			return
		}
		if offer.MarketID == 0 {
			continue
		}
		carts := w.entities(p.ID, "trade_cart")
		if len(carts) == 0 && len(market.Tasks) == 0 {
			_ = w.Apply(p.ID, Command{Kind: "train", EntityIDs: []int{market.ID}, Product: "trade_cart"})
		} else {
			for _, cart := range carts {
				if cart.behavior.State() == Idle && w.cartShipment(cart.ID) == nil && cart.Position.Distance(market.Position) > definitions["market"].Radius+1 {
					pos := market.Position
					_ = w.Apply(p.ID, Command{Kind: "move", EntityIDs: []int{cart.ID}, Position: &pos})
					break
				}
			}
		}
	}
	for _, offer := range book.Offers {
		if offer.Owner == p.ID && offer.State == string(offerOpen) {
			return
		}
	}
	want, give := "", ""
	low, high := 1., 1.
	for _, resource := range []string{"food", "wood", "gold", "stone"} {
		ratio := p.Resources.Amount(resource) / floor.Amount(resource)
		if ratio < low {
			low, want = ratio, resource
		}
		if ratio > high {
			high, give = ratio, resource
		}
	}
	if want == "" || give == "" || p.Resources.Amount(give)-floor.Amount(give) < 200 {
		return
	}
	_ = w.Apply(p.ID, Command{Kind: "market_post", EntityIDs: []int{market.ID}, Offer: &TradeOfferIntent{GiveResource: give, GiveAmount: 100, WantResource: want, WantAmount: 100, Lots: 2}})
}
