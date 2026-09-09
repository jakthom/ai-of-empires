package game

import (
	"fmt"
	"slices"

	"github.com/open-ships/statemachine"
)

const tradeCapacity = 500
const maxOfferLots = 20
const maxOpenOffers = 12
const merchantCapacity = 5000

// The book owns listings and deliveries. Reserved goods are represented once:
// open lots belong to the offer; a reserved lot belongs to its shipment until
// collection; transported goods belong to the cart. None are spendable stock.
type marketplace struct {
	Merchants               Resources
	Revision                int
	NextOffer, NextShipment int
	Offers                  map[int]*tradeOffer
	Shipments               map[int]*tradeShipment
}

type TradeOfferIntent struct {
	GiveResource string `json:"give_resource"`
	GiveAmount   int    `json:"give_amount"`
	WantResource string `json:"want_resource"`
	WantAmount   int    `json:"want_amount"`
	Lots         int    `json:"lots"`
	TargetPlayer int    `json:"target_player,omitempty"`
}

func resourceAmount(kind string, amount float64) Resources {
	r := Resources{}
	r.Deposit(kind, amount)
	return r
}

func (r Resources) Amount(kind string) float64 {
	switch kind {
	case "food":
		return r.Food
	case "wood":
		return r.Wood
	case "gold":
		return r.Gold
	case "stone":
		return r.Stone
	}
	return 0
}

func (w *World) initializeMarketplace() {
	w.Marketplace = marketplace{Merchants: Resources{Food: 1000, Wood: 1000, Gold: 2000, Stone: 1000}, NextOffer: 1, NextShipment: 1, Offers: map[int]*tradeOffer{}, Shipments: map[int]*tradeShipment{}}
}

func (w *World) ownMarket(player, id int) bool {
	e := w.Entities[id]
	return e != nil && e.Owner == player && e.Type == "market" && e.life.State() == Active
}

func (w *World) liveTrader(s *tradeShipment) *Entity {
	cart := w.Entities[s.Cart]
	if cart == nil || cart.Owner != s.Buyer || cart.life.State() != Active || cart.Type != "trade_cart" {
		return nil
	}
	return cart
}

func (w *World) cartShipment(id int) *tradeShipment {
	for _, key := range sortedTradeIDs(w.Marketplace.Shipments) {
		s := w.Marketplace.Shipments[key]
		if s.Cart == id && !s.terminal() {
			return s
		}
	}
	return nil
}

func sortedTradeIDs[T any](values map[int]T) []int {
	ids := make([]int, 0, len(values))
	for id := range values {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

func (w *World) marketplaceCommand(player int, c Command) error {
	switch c.Kind {
	case "market_post":
		if len(c.EntityIDs) != 1 || !w.ownMarket(player, c.EntityIDs[0]) {
			return rule("market_required", "Select one completed Market belonging to your kingdom.")
		}
		if c.Offer == nil {
			return rule("invalid_offer", "Specify the resources, quantities, and number of lots.")
		}
		o := &tradeOffer{ID: w.Marketplace.NextOffer, Owner: player, Market: c.EntityIDs[0], Terms: *c.Offer, CreatedAt: w.Time, lifecycle: statemachine.NewInstance(offerMachine, offerDraft)}
		return fire(o.lifecycle, offerPost, &offerContext{World: w, Offer: o, Player: player})
	case "market_cancel":
		o := w.Marketplace.Offers[c.OfferID]
		if o == nil || o.Owner != player {
			return rule("offer_unavailable", "Choose one of your offers.")
		}
		if o.lifecycle.State() != offerOpen {
			return rule("offer_closed", "This offer has already closed.")
		}
		return fire(o.lifecycle, offerCancel, &offerContext{World: w, Offer: o, Player: player})
	case "market_accept":
		o := w.Marketplace.Offers[c.OfferID]
		if o == nil || !o.addressedTo(player) {
			return rule("offer_unavailable", "This offer is unavailable.")
		}
		if len(c.EntityIDs) != 1 {
			return rule("invalid_selection", "Select one idle Trade Cart at your Market.")
		}
		cart := w.Entities[c.EntityIDs[0]]
		if err := w.canAcceptOffer(player, o, cart); err != nil {
			return err
		}
		ctx := &offerContext{World: w, Offer: o, Player: player, Cart: cart, Repeat: c.Repeat}
		if err := fire(o.lifecycle, offerFill, ctx); err != nil {
			return err
		}
		// Start the delivery after the listing transition has committed.
		mustFire(ctx.Shipment.lifecycle, shipmentLaunch, &shipmentContext{World: w, Shipment: ctx.Shipment})
		return nil
	case "market_resume", "market_recall":
		s := w.Marketplace.Shipments[c.ShipmentID]
		if s == nil || s.Buyer != player || s.terminal() || w.liveTrader(s) == nil {
			return rule("shipment_unavailable", "Choose one of your active deliveries.")
		}
		if c.Kind == "market_recall" {
			if s.lifecycle.State() != shipmentOutbound {
				return rule("already_collected", "The cart must deliver its current cargo home.")
			}
			mustFire(s.lifecycle, shipmentRecall, &shipmentContext{World: w, Shipment: s})
		}
		w.setOrder(w.liveTrader(s), Order{Kind: "caravan", Shipment: s.ID}, false)
		return nil
	}
	return rule("unknown_command", "Unknown marketplace command.")
}

func (w *World) tradeNotice(player int, message string) {
	w.record(Event{Player: player, Kind: "trade", Message: message}, nil, player)
}

func (w *World) pulseMarketplace() {
	for _, id := range sortedTradeIDs(w.Marketplace.Offers) {
		o := w.Marketplace.Offers[id]
		if o.lifecycle.State() == offerOpen {
			mustFire(o.lifecycle, offerPulse, &offerContext{World: w, Offer: o, Player: o.Owner})
		}
	}
	var followups []Command
	var buyers []int
	for _, id := range sortedTradeIDs(w.Marketplace.Shipments) {
		s := w.Marketplace.Shipments[id]
		if s.terminal() {
			continue
		}
		mustFire(s.lifecycle, shipmentPulse, &shipmentContext{World: w, Shipment: s})
		if s.terminal() {
			if cart := w.liveTrader(s); cart != nil && cart.behavior.State() == Idle && len(cart.Orders) > 0 {
				w.advanceOrder(cart)
				continue
			}
		}
		if s.lifecycle.State() == shipmentDelivered && s.Repeat {
			if offer := w.Marketplace.Offers[s.Offer]; offer == nil || offer.lifecycle.State() != offerOpen {
				w.tradeNotice(s.Buyer, fmt.Sprintf("Trade route complete: offer #%d has closed.", s.Offer))
				continue
			}
			followups = append(followups, Command{Kind: "market_accept", OfferID: s.Offer, EntityIDs: []int{s.Cart}, Repeat: true})
			buyers = append(buyers, s.Buyer)
		}
	}
	// No lifecycle fires itself recursively; continuation uses the same command
	// boundary as a human, including stock, capacity, permission and route checks.
	for i, c := range followups {
		if err := w.Apply(buyers[i], c); err != nil {
			w.tradeNotice(buyers[i], "Repeating trade stopped: "+err.Error())
		}
	}
	if w.Tick%200 == 0 {
		w.pruneMarketplace()
	}
}

func (w *World) pruneMarketplace() {
	ids := sortedTradeIDs(w.Marketplace.Shipments)
	for _, id := range ids[:max(0, len(ids)-200)] {
		if w.Marketplace.Shipments[id].terminal() {
			delete(w.Marketplace.Shipments, id)
		}
	}
	ids = sortedTradeIDs(w.Marketplace.Offers)
	for _, id := range ids[:max(0, len(ids)-100)] {
		if w.Marketplace.Offers[id].lifecycle.State() != offerOpen {
			delete(w.Marketplace.Offers, id)
		}
	}
}

func tradeTerms(t TradeOfferIntent) string {
	return fmt.Sprintf("%d %s for %d %s", t.GiveAmount, t.GiveResource, t.WantAmount, t.WantResource)
}
