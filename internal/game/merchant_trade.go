package game

import (
	"context"
	"fmt"
	"math"
	"slices"

	"github.com/open-ships/statemachine"
)

func (w *World) knowsNeutralMarket(player, id int) bool {
	e := w.Entities[id]
	if e != nil && e.Type == "market" && e.Owner == 0 && w.visibleEntity(player, e) {
		return true
	}
	m, ok := w.Players[player].Memory[id]
	return ok && m.Type == "market" && m.Owner == 0
}

func (w *World) merchantTradeQuote(player, market int, product, mode string, cart *Entity) (exchangeQuote, error) {
	q := exchangeQuote{}
	if !w.knowsNeutralMarket(player, market) {
		return q, rule("market_unexplored", "Explore the neutral Market before arranging a delivery.")
	}
	if !slices.Contains([]string{"buy", "sell"}, mode) {
		return q, rule("invalid_trade_mode", "Choose buy or sell.")
	}
	r := w.Marketplace.Regions[market]
	target := w.Entities[market]
	var err error
	q, err = w.quoteMerchant(w.Players[player], r, target, "market_"+mode, product)
	if err != nil {
		return q, err
	}
	if target.Owner != 0 {
		return q, rule("invalid_target", "Choose a neutral Market.")
	}
	if cart == nil || cart.Owner != player || cart.Type != "trade_cart" || cart.life.State() != Active || cart.Container != 0 || cart.behavior.State() != Idle || len(cart.Orders) > 0 || cart.Cargo > 0 || w.cartShipment(cart.ID) != nil {
		return q, rule("cart_required", "Use an idle, empty Trade Cart without queued orders or an active delivery.")
	}
	home := w.tradeHome(cart)
	if home == nil {
		return q, rule("market_required", "Build a Market on this landmass to receive deliveries.")
	}
	if cart.Position.Distance(home.Position) > definitions["market"].Radius+1.1 {
		return q, rule("cart_at_market", "Move the Trade Cart beside your Market to load its payment or exports.")
	}
	if !w.reachableFootprint(cart, target.Position, definitions["market"].Radius+.7) {
		return q, rule("unreachable_market", "This trade needs Markets connected by land.")
	}
	return q, nil
}

func (w *World) startMerchantTrade(player int, c Command) error {
	if len(c.EntityIDs) != 1 || c.Queue {
		return rule("invalid_selection", "Select one Trade Cart; start this route beside your Market without queueing it.")
	}
	mode := c.TradeMode
	if mode == "" {
		mode = "sell"
	}
	product := c.Product
	if product == "" {
		// Existing clients may omit a commodity. Pick an owned surplus, never
		// withdraw a free distance-based payment or inspect another kingdom.
		amount := -1.0
		for _, resource := range []string{"food", "wood", "stone"} {
			if n := w.Players[player].Resources.Amount(resource); n > amount {
				product, amount = resource, n
			}
		}
	}
	cart := w.Entities[c.EntityIDs[0]]
	q, err := w.merchantTradeQuote(player, c.TargetID, product, mode, cart)
	if err != nil {
		return err
	}
	r := w.Marketplace.Regions[c.TargetID]
	if c.MarketRevision != nil && *c.MarketRevision != r.Revision {
		return rule("market_changed", "The local quote changed. Read the Market again.")
	}
	price := int(q.Gain.Gold)
	limit := 1
	terms := TradeOfferIntent{GiveResource: "gold", GiveAmount: price, WantResource: product, WantAmount: 100, Lots: 1}
	if mode == "buy" {
		price = int(q.Cost.Gold)
		limit = tradeCapacity
		terms = TradeOfferIntent{GiveResource: product, GiveAmount: 100, WantResource: "gold", WantAmount: price, Lots: 1}
	}
	if c.TradeLimit != nil {
		if *c.TradeLimit < 1 || *c.TradeLimit > tradeCapacity {
			return rule("invalid_trade_limit", "Choose a gold price limit between 1 and 500 per 100 goods.")
		}
		if mode == "sell" && price < *c.TradeLimit || mode == "buy" && price > *c.TradeLimit {
			return rule("trade_price_limit", "This Market's price is outside your route's limit.")
		}
		limit = *c.TradeLimit
	}
	s := &tradeShipment{ID: w.Marketplace.NextShipment, Buyer: player, Market: c.TargetID, Home: w.tradeHome(cart).ID, Cart: cart.ID, Merchant: c.TargetID, Product: product, TradeMode: mode, Limit: limit, Terms: terms, Repeat: c.Repeat, CreatedAt: w.Time, lifecycle: statemachine.NewInstance(shipmentMachine, shipmentReserved)}
	return fire(s.lifecycle, shipmentMerchantLaunch, &shipmentContext{World: w, Shipment: s})
}
func merchantShipmentAvailable(_ context.Context, c *shipmentContext) error {
	s := c.Shipment
	q, err := c.World.merchantTradeQuote(s.Buyer, s.Market, s.Product, s.TradeMode, c.cart())
	if err != nil {
		return err
	}
	if !validTradeTerms(s.Terms) || q.Cost.Amount(s.Terms.WantResource) != float64(s.Terms.WantAmount) || q.Gain.Amount(s.Terms.GiveResource) != float64(s.Terms.GiveAmount) {
		return rule("market_changed", "Read a fresh merchant quote.")
	}
	return nil
}
func launchMerchantShipment(ctx context.Context, c *shipmentContext) error {
	w, s := c.World, c.Shipment
	r := w.Marketplace.Regions[s.Merchant]
	r.Stock.Deposit(s.Terms.GiveResource, -float64(s.Terms.GiveAmount))
	r.Revision++
	w.Marketplace.Shipments[s.ID] = s
	w.Marketplace.NextShipment++
	return launchShipment(ctx, c)
}
func (w *World) creditShipmentSeller(s *tradeShipment, resource string, amount float64) {
	if s.Merchant != 0 {
		r := w.Marketplace.Regions[s.Merchant]
		r.Stock.Deposit(resource, amount)
		r.Revision++
		return
	}
	w.Players[s.Seller].Resources.Deposit(resource, amount)
}
func (w *World) merchantIncoming(id int) Resources {
	r := Resources{}
	for _, s := range w.Marketplace.Shipments {
		if s.Merchant == id && s.lifecycle.State() == shipmentOutbound {
			r.Deposit(s.Terms.WantResource, float64(s.Terms.WantAmount))
		}
	}
	return r
}
func (w *World) merchantReserved(id int) Resources {
	r := Resources{}
	for _, s := range w.Marketplace.Shipments {
		if s.Merchant == id && s.lifecycle.State() == shipmentOutbound {
			r.Deposit(s.Terms.GiveResource, float64(s.Terms.GiveAmount))
		}
	}
	return r
}
func (w *World) merchantRoom(r *merchantRegion, resource string) float64 {
	// Goods have warehouse capacity. Cash may circulate into a well-funded
	// Market; a full till must never make it impossible to purchase its goods.
	if resource == "gold" {
		return math.Inf(1)
	}
	// Leave room for both an accepted delivery and its potential refund.
	return float64(merchantCapacity) - r.Stock.Amount(resource) - w.merchantIncoming(r.ID).Amount(resource) - w.merchantReserved(r.ID).Amount(resource)
}
func (s *tradeShipment) label() string {
	if s.Merchant != 0 {
		return fmt.Sprintf("merchant trade at Market #%d", s.Market)
	}
	return fmt.Sprintf("offer #%d", s.Offer)
}
