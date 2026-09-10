package game

import "slices"

// Commerce uses the same published book and command boundary as human/MCP
// players. A kingdom advertises a surplus against its own shortage; it never
// looks at another kingdom's stockpile or private offers.
func (w *World) aiCommerce(c *aiContext) {
	p := c.Player
	w.aiTradeEscorts(c)
	if w.Time < p.AIPlan.NextTradeAt {
		return
	}
	p.AIPlan.NextTradeAt = w.Time + 20
	markets := w.tradingPosts(p.ID)
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
	if w.aiMerchantCommerce(p, book, floor) {
		return
	}
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
		remote := w.Entities[offer.MarketID]
		if remote == nil {
			continue
		}
		market := w.nearest(p.Start, func(e *Entity) bool { return w.ownMarket(p.ID, e.ID) && e.Type == remote.Type })
		if market == nil {
			continue
		}
		carrierType := "trade_cart"
		if market.Type == "dock" {
			carrierType = "trade_ship"
		}
		carts := w.entities(p.ID, carrierType)
		if len(carts) == 0 && len(market.Tasks) == 0 {
			_ = w.Apply(p.ID, Command{Kind: "train", EntityIDs: []int{market.ID}, Product: carrierType})
		} else {
			for _, cart := range carts {
				if cart.behavior.State() == Idle && w.cartShipment(cart.ID) == nil && cart.Position.Distance(market.Position) > tradeRadius(market)+1 {
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

func (w *World) aiMerchantCommerce(p *Player, book MarketplaceView, floor Resources) bool {
	for _, region := range book.Markets {
		remote := w.Entities[region.Merchant.MarketID]
		if remote == nil {
			continue
		}
		market := w.nearest(p.Start, func(e *Entity) bool { return w.ownMarket(p.ID, e.ID) && e.Type == remote.Type })
		if market == nil {
			continue
		}
		for _, route := range region.Routes {
			useful := route.Mode == "sell" && p.Resources.Gold < floor.Gold && p.Resources.Amount(route.Product)-100 >= floor.Amount(route.Product)
			useful = useful || route.Mode == "buy" && p.Resources.Amount(route.Product) < floor.Amount(route.Product) && p.Resources.Gold-route.Cost.Gold >= floor.Gold
			if !useful || route.Cost == (Resources{}) || route.Gain == (Resources{}) {
				continue
			}
			// If home merchants can fill the need at least as well, save the
			// journey. All values come from this kingdom's authenticated view.
			for _, local := range book.Merchants.Actions {
				if local.Product != route.Product || local.Kind != "market_"+route.Mode || !local.Enabled || local.Gain == nil {
					continue
				}
				if route.Mode == "sell" && local.Gain.Gold >= route.Gain.Gold || route.Mode == "buy" && local.Cost.Gold <= route.Cost.Gold {
					_ = w.Apply(p.ID, Command{Kind: local.Kind, EntityIDs: []int{market.ID}, Product: route.Product, MarketRevision: &book.Merchants.Revision})
					return true
				}
			}
			if route.CanStart {
				limit := int(route.Gain.Gold)
				if route.Mode == "buy" {
					limit = int(route.Cost.Gold)
				}
				_ = w.Apply(p.ID, Command{Kind: "trade", EntityIDs: []int{route.CartID}, TargetID: region.Merchant.MarketID, Product: route.Product, TradeMode: route.Mode, TradeLimit: &limit, MarketRevision: &region.Merchant.Revision})
				return true
			}
			carrierType := "trade_cart"
			if market.Type == "dock" {
				carrierType = "trade_ship"
			}
			carts := w.entities(p.ID, carrierType)
			if len(carts) == 0 && len(market.Tasks) == 0 {
				_ = w.Apply(p.ID, Command{Kind: "train", EntityIDs: []int{market.ID}, Product: carrierType})
				return true
			}
			for _, cart := range carts {
				if cart.behavior.State() == Idle && w.cartShipment(cart.ID) == nil && cart.Position.Distance(market.Position) > tradeRadius(market)+1.1 {
					pos := market.Position
					_ = w.Apply(p.ID, Command{Kind: "move", EntityIDs: []int{cart.ID}, Position: &pos})
					return true
				}
			}
		}
	}
	return false
}

// Assign at most two nearby idle defenders to loaded carriers. Existing escort
// orders are excluded from raids and rally commands; no army or funds appear.
func (w *World) aiTradeEscorts(c *aiContext) {
	for _, carrier := range w.tradeCarriers(c.Player.ID) {
		if carrier.Cargo <= 0 || w.cartShipment(carrier.ID) == nil {
			continue
		}
		count := 0
		for _, unit := range w.entities(c.Player.ID, "") {
			if unit.Order.Kind == "guard" && unit.Order.Target == carrier.ID {
				count++
			}
		}
		for _, unit := range w.entities(c.Player.ID, "") {
			if count >= 2 {
				break
			}
			if unit.behavior.State() != Idle || unit.ID == c.Player.AIPlan.ScoutID || slices.Contains(c.Player.AIPlan.Army, unit.ID) || c.Player.voyaging(unit.ID) || unit.Position.Distance(carrier.Position) > 12 || guardUnit(nil, &unitContext{Actor: unit}) != nil || w.guardTarget(unit, carrier) != nil {
				continue
			}
			if w.Apply(c.Player.ID, Command{Kind: "guard", EntityIDs: []int{unit.ID}, TargetID: carrier.ID}) == nil {
				aiStance(c, unit, "defensive")
				count++
			}
		}
	}
}
