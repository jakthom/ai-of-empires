package game

type MarketplaceView struct {
	Offers    []TradeOfferView    `json:"offers"`
	Shipments []TradeShipmentView `json:"shipments"`
	Merchants MerchantView        `json:"merchants"`
	Reserved  Resources           `json:"reserved"`
	Capacity  int                 `json:"capacity"`
	MaxLots   int                 `json:"max_lots"`
	MaxOffers int                 `json:"max_offers"`
}
type MerchantView struct {
	Stock    Resources `json:"stock"`
	Revision int       `json:"revision"`
	MarketID int       `json:"market_id,omitempty"`
	Actions  []Action  `json:"actions"`
}
type TradeOfferView struct {
	ID        int              `json:"id"`
	Owner     int              `json:"owner"`
	Terms     TradeOfferIntent `json:"terms"`
	Remaining int              `json:"remaining"`
	State     string           `json:"state"`
	MarketID  int              `json:"market_id,omitempty"`
	Position  *Vec             `json:"position,omitempty"`
	CanAccept bool             `json:"can_accept"`
	Reason    string           `json:"reason,omitempty"`
	CartID    int              `json:"cart_id,omitempty"`
}
type TradeShipmentView struct {
	ID        int              `json:"id"`
	OfferID   int              `json:"offer_id"`
	Seller    int              `json:"seller"`
	Buyer     int              `json:"buyer"`
	Terms     TradeOfferIntent `json:"terms"`
	State     string           `json:"state"`
	Status    string           `json:"status"`
	CartID    int              `json:"cart_id,omitempty"`
	Position  *Vec             `json:"position,omitempty"`
	CanResume bool             `json:"can_resume"`
	CanRecall bool             `json:"can_recall"`
	Repeat    bool             `json:"repeat"`
}

// Listing terms are deliberately published. Foreign inventories, unobserved
// Market locations, unrelated private offers and third-party deliveries never
// enter this read model, including disabled-action reasons.
func (w *World) marketplaceView(player int) MarketplaceView {
	v := MarketplaceView{Offers: []TradeOfferView{}, Shipments: []TradeShipmentView{}, Capacity: tradeCapacity, MaxLots: maxOfferLots, MaxOffers: maxOpenOffers}
	var market *Entity
	for _, e := range w.entities(player, "market") {
		if e.life.State() == Active {
			market = e
			break
		}
	}
	v.Merchants = MerchantView{Stock: w.Marketplace.Merchants, Revision: w.Marketplace.Revision, Actions: []Action{}}
	if market != nil {
		v.Merchants.MarketID = market.ID
	}
	for _, resource := range []string{"food", "wood", "stone"} {
		for _, kind := range []string{"market_buy", "market_sell"} {
			q, err := w.quoteExchange(w.Players[player], market, kind, resource)
			a := action(kind, resource, map[string]string{"market_buy": "Buy ", "market_sell": "Sell "}[kind]+resource, q.Description, q.Cost, 0, err)
			a.Gain = &q.Gain
			v.Merchants.Actions = append(v.Merchants.Actions, a)
		}
	}
	carts := w.entities(player, "trade_cart")
	for _, id := range sortedTradeIDs(w.Marketplace.Offers) {
		o := w.Marketplace.Offers[id]
		if !o.addressedTo(player) || o.Owner != player && o.lifecycle.State() != offerOpen {
			continue
		}
		view := TradeOfferView{ID: o.ID, Owner: o.Owner, Terms: o.Terms, Remaining: o.Remaining, State: string(o.lifecycle.State())}
		if w.knowsTradingMarket(player, o) {
			view.MarketID = o.Market
			if e := w.Entities[o.Market]; e != nil && (o.Owner == player || w.visibleEntity(player, e)) {
				pos := e.Position
				view.Position = &pos
			} else if m, ok := w.Players[player].Memory[o.Market]; ok {
				pos := m.Position
				view.Position = &pos
			}
		}
		err := w.canAcceptOffer(player, o, nil)
		for _, cart := range carts {
			candidateErr := w.canAcceptOffer(player, o, cart)
			if candidateErr == nil {
				view.CanAccept, view.CartID = true, cart.ID
				err = nil
				break
			}
			// Prefer the refusal for an otherwise idle cart over busy-cart errors.
			if cart.behavior.State() == Idle && w.cartShipment(cart.ID) == nil {
				err = candidateErr
			}
		}
		if err != nil {
			view.Reason = err.Error()
		}
		if w.match.State() != MatchRunning || w.Players[player].lifecycle.State() == PlayerDefeated {
			view.CanAccept = false
			view.Reason = "Trade orders require a running game and an active kingdom."
		}
		v.Offers = append(v.Offers, view)
		if o.Owner == player {
			v.Reserved.Deposit(o.Terms.GiveResource, float64(o.Terms.GiveAmount*o.Remaining))
		}
	}
	for _, id := range sortedTradeIDs(w.Marketplace.Shipments) {
		s := w.Marketplace.Shipments[id]
		if s.Buyer != player && s.Seller != player {
			continue
		}
		view := TradeShipmentView{ID: s.ID, OfferID: s.Offer, Seller: s.Seller, Buyer: s.Buyer, Terms: s.Terms, State: string(s.lifecycle.State()), Status: string(s.lifecycle.State()), Repeat: s.Repeat}
		cart := w.liveTrader(s)
		if cart != nil && (s.Buyer == player || w.visibleEntity(player, cart)) {
			view.CartID = cart.ID
			pos := cart.Position
			view.Position = &pos
		}
		if !s.terminal() && cart != nil && s.Buyer == player {
			if cart.behavior.State() != Caravanning || cart.Order.Shipment != s.ID {
				view.Status = "Paused — resume the caravan to continue"
				view.CanResume = s.Buyer == player
			} else if s.lifecycle.State() != shipmentOutbound && w.tradeHome(cart) == nil {
				view.Status = "Waiting for a reachable home Market"
			} else if len(cart.Path) == 0 && cart.Repath > 0 && cart.Position.Distance(cart.PathGoal) > definitions["market"].Radius+.8 {
				view.Status = "Route blocked — waiting for a clear path"
			}
			view.CanRecall = s.Buyer == player && s.lifecycle.State() == shipmentOutbound
		}
		if w.match.State() != MatchRunning || w.Players[player].lifecycle.State() == PlayerDefeated {
			view.CanRecall, view.CanResume = false, false
		}
		v.Shipments = append(v.Shipments, view)
		if s.Seller == player && s.lifecycle.State() == shipmentOutbound {
			v.Reserved.Deposit(s.Terms.GiveResource, float64(s.Terms.GiveAmount))
		}
	}
	return v
}
