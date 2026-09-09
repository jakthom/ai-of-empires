package game

import (
	"context"
	"fmt"
	"slices"

	"github.com/open-ships/statemachine"
)

type offerState string
type offerEvent string

const (
	offerDraft     offerState = "draft"
	offerOpen      offerState = "open"
	offerFilled    offerState = "filled"
	offerCancelled offerState = "cancelled"
	offerPost      offerEvent = "post"
	offerFill      offerEvent = "fill"
	offerCancel    offerEvent = "cancel"
	offerPulse     offerEvent = "pulse"
)

type tradeOffer struct {
	ID, Owner, Market int
	Terms             TradeOfferIntent
	Remaining         int
	CreatedAt         float64
	lifecycle         *statemachine.Instance[offerState, offerEvent, *offerContext]
}
type offerContext struct {
	World    *World
	Offer    *tradeOffer
	Player   int
	Cart     *Entity
	Repeat   bool
	Shipment *tradeShipment
}

var offerMachine = statemachine.MustCompile([]statemachine.Transition[offerState, offerEvent, *offerContext]{
	{From: offerDraft, Event: offerPost, To: offerOpen, Guard: canPostOffer, Do: postOffer},
	{From: offerOpen, Event: offerFill, To: offerFilled, Guard: lastOfferLot, Do: fillOffer},
	{From: offerOpen, Event: offerFill, To: offerOpen, Guard: availableOfferLot, Do: fillOffer},
	{From: offerOpen, Event: offerCancel, To: offerCancelled, Guard: ownsOffer, Do: cancelOffer},
	{From: offerOpen, Event: offerPulse, To: offerCancelled, Guard: offerMarketLost, Do: cancelOffer},
	{From: offerOpen, Event: offerPulse, To: offerOpen},
})

func validTradeTerms(t TradeOfferIntent) bool {
	resources := []string{"food", "wood", "gold", "stone"}
	return slices.Contains(resources, t.GiveResource) && slices.Contains(resources, t.WantResource) && t.GiveResource != t.WantResource && t.GiveAmount > 0 && t.GiveAmount <= tradeCapacity && t.WantAmount > 0 && t.WantAmount <= tradeCapacity && t.Lots > 0 && t.Lots <= maxOfferLots
}
func canPostOffer(_ context.Context, c *offerContext) error {
	w, o := c.World, c.Offer
	if !validTradeTerms(o.Terms) {
		return rule("invalid_offer", "Choose different resources, 1–500 of each per lot, and 1–20 lots.")
	}
	if o.Terms.TargetPlayer < 0 || o.Terms.TargetPlayer == o.Owner || o.Terms.TargetPlayer > w.Config.Settlements {
		return rule("invalid_recipient", "Choose another kingdom or publish to everyone.")
	}
	if o.Terms.TargetPlayer > 0 && w.Players[o.Terms.TargetPlayer].lifecycle.State() == PlayerDefeated {
		return rule("invalid_recipient", "Choose an active kingdom.")
	}
	if !w.ownMarket(o.Owner, o.Market) {
		return rule("market_required", "Build a Market before posting an offer.")
	}
	open := 0
	for _, existing := range w.Marketplace.Offers {
		if existing.Owner == o.Owner && existing.lifecycle.State() == offerOpen {
			open++
		}
	}
	if open >= maxOpenOffers {
		return rule("offer_limit", "Cancel an offer before posting more than 12 open offers.")
	}
	if !w.Players[o.Owner].Resources.CanPay(resourceAmount(o.Terms.GiveResource, float64(o.Terms.GiveAmount*o.Terms.Lots))) {
		return rule("insufficient_resources", "You need enough resources to reserve every advertised lot.")
	}
	return nil
}
func postOffer(_ context.Context, c *offerContext) error {
	w, o := c.World, c.Offer
	w.Players[o.Owner].Resources.Deposit(o.Terms.GiveResource, -float64(o.Terms.GiveAmount*o.Terms.Lots))
	o.Remaining = o.Terms.Lots
	w.Marketplace.Offers[o.ID] = o
	w.Marketplace.NextOffer++
	w.tradeNotice(o.Owner, fmt.Sprintf("Posted offer #%d: %s, %d lots reserved.", o.ID, tradeTerms(o.Terms), o.Remaining))
	return nil
}
func (o *tradeOffer) addressedTo(player int) bool {
	return o.Owner == player || o.Terms.TargetPlayer == 0 || o.Terms.TargetPlayer == player
}

func (w *World) knowsTradingMarket(player int, o *tradeOffer) bool {
	if o.Owner == player {
		return true
	}
	if e := w.Entities[o.Market]; e != nil && w.visibleEntity(player, e) {
		return true
	}
	m, ok := w.Players[player].Memory[o.Market]
	return ok && m.Owner == o.Owner && m.Type == "market" && m.Progress >= 1
}

func (w *World) canAcceptOffer(player int, o *tradeOffer, cart *Entity) error {
	if !o.addressedTo(player) || o.Owner == player || o.lifecycle.State() != offerOpen || o.Remaining < 1 {
		return rule("offer_unavailable", "Choose an open offer from another kingdom.")
	}
	if !w.knowsTradingMarket(player, o) {
		return rule("market_unexplored", "Explore the offering kingdom's Market before sending a caravan.")
	}
	if !w.ownMarket(o.Owner, o.Market) || w.Players[o.Owner].lifecycle.State() == PlayerDefeated {
		return rule("offer_unavailable", "This offer is unavailable.")
	}
	if w.relation(player, o.Owner) != atPeace {
		return rule("trade_conflict", "Trade requires peace between the two kingdoms.")
	}
	if cart == nil || cart.Owner != player || cart.Type != "trade_cart" || cart.life.State() != Active || cart.Container != 0 || cart.behavior.State() != Idle || len(cart.Orders) > 0 || cart.Cargo > 0 || w.cartShipment(cart.ID) != nil {
		return rule("cart_required", "Use an idle, empty Trade Cart without an active delivery.")
	}
	home := w.tradeHome(cart)
	if home == nil || cart.Position.Distance(home.Position) > definitions["market"].Radius+1.1 {
		return rule("cart_at_market", "Move the Trade Cart beside your completed Market to load payment.")
	}
	if !w.reachableFootprint(cart, w.Entities[o.Market].Position, definitions["market"].Radius+.7) {
		return rule("unreachable_market", "This trade needs Markets connected by land.")
	}
	if !w.Players[player].Resources.CanPay(resourceAmount(o.Terms.WantResource, float64(o.Terms.WantAmount))) {
		return rule("insufficient_resources", "You cannot afford this offer's requested payment.")
	}
	return nil
}
func availableOfferLot(_ context.Context, c *offerContext) error {
	return c.World.canAcceptOffer(c.Player, c.Offer, c.Cart)
}
func lastOfferLot(ctx context.Context, c *offerContext) error {
	if err := applicable(c.Offer.Remaining == 1); err != nil {
		return err
	}
	return availableOfferLot(ctx, c)
}
func fillOffer(_ context.Context, c *offerContext) error {
	w, o := c.World, c.Offer
	o.Remaining--
	s := &tradeShipment{ID: w.Marketplace.NextShipment, Offer: o.ID, Seller: o.Owner, Buyer: c.Player, Market: o.Market, Home: w.tradeHome(c.Cart).ID, Cart: c.Cart.ID, Terms: o.Terms, Repeat: c.Repeat, CreatedAt: w.Time, lifecycle: statemachine.NewInstance(shipmentMachine, shipmentReserved)}
	w.Marketplace.NextShipment++
	w.Marketplace.Shipments[s.ID] = s
	c.Shipment = s
	return nil
}
func ownsOffer(_ context.Context, c *offerContext) error {
	return applicable(c.Player == c.Offer.Owner)
}
func offerMarketLost(_ context.Context, c *offerContext) error {
	return applicable(!c.World.ownMarket(c.Offer.Owner, c.Offer.Market) || c.World.Players[c.Offer.Owner].lifecycle.State() == PlayerDefeated || c.World.match.State() == MatchFinished)
}
func cancelOffer(_ context.Context, c *offerContext) error {
	o := c.Offer
	c.World.Players[o.Owner].Resources.Deposit(o.Terms.GiveResource, float64(o.Terms.GiveAmount*o.Remaining))
	c.World.tradeNotice(o.Owner, fmt.Sprintf("Offer #%d closed; %d unclaimed lots returned to stock.", o.ID, o.Remaining))
	o.Remaining = 0
	return nil
}
