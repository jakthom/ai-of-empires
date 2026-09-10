package game

import (
	"context"
	"fmt"

	"github.com/open-ships/statemachine"
)

type shipmentState string
type shipmentEvent string

const (
	shipmentReserved       shipmentState = "reserved"
	shipmentOutbound       shipmentState = "outbound"
	shipmentReturning      shipmentState = "returning"
	shipmentRefunding      shipmentState = "returning_payment"
	shipmentDelivered      shipmentState = "delivered"
	shipmentRecalled       shipmentState = "recalled"
	shipmentLost           shipmentState = "lost"
	shipmentLaunch         shipmentEvent = "launch"
	shipmentMerchantLaunch shipmentEvent = "merchant_launch"
	shipmentPulse          shipmentEvent = "pulse"
	shipmentRecall         shipmentEvent = "recall"
)

type tradeShipment struct {
	Merchant, Limit                              int
	Product, TradeMode                           string
	ID, Offer, Seller, Buyer, Market, Home, Cart int
	Terms                                        TradeOfferIntent
	Repeat                                       bool
	CreatedAt                                    float64
	lifecycle                                    *statemachine.Instance[shipmentState, shipmentEvent, *shipmentContext]
}
type shipmentContext struct {
	World    *World
	Shipment *tradeShipment
}

var shipmentMachine = statemachine.MustCompile(shipmentTransitions())

func shipmentTransitions() []statemachine.Transition[shipmentState, shipmentEvent, *shipmentContext] {
	type row = statemachine.Transition[shipmentState, shipmentEvent, *shipmentContext]
	rows := []row{
		{From: shipmentReserved, Event: shipmentMerchantLaunch, To: shipmentOutbound, Guard: merchantShipmentAvailable, Do: launchMerchantShipment},
		{From: shipmentReserved, Event: shipmentLaunch, To: shipmentOutbound, Do: launchShipment},
		{From: shipmentOutbound, Event: shipmentPulse, To: shipmentLost, Guard: shipmentCartLost, Do: loseUncollectedShipment},
	}
	for _, state := range []shipmentState{shipmentReturning, shipmentRefunding} {
		rows = append(rows, row{From: state, Event: shipmentPulse, To: shipmentLost, Guard: shipmentCartLost, Do: loseShipment})
	}
	rows = append(rows,
		row{From: shipmentOutbound, Event: shipmentRecall, To: shipmentRefunding, Do: recallShipment},
		row{From: shipmentOutbound, Event: shipmentPulse, To: shipmentRefunding, Guard: shipmentPartnerLost, Do: recallShipment},
		row{From: shipmentOutbound, Event: shipmentPulse, To: shipmentReturning, Guard: shipmentAtSeller, Do: collectShipment},
		row{From: shipmentOutbound, Event: shipmentPulse, To: shipmentOutbound, Do: moveShipmentToSeller},
		row{From: shipmentReturning, Event: shipmentPulse, To: shipmentDelivered, Guard: shipmentAtHome, Do: deliverShipment},
		row{From: shipmentReturning, Event: shipmentPulse, To: shipmentReturning, Do: moveShipmentHome},
		row{From: shipmentRefunding, Event: shipmentPulse, To: shipmentRecalled, Guard: shipmentAtHome, Do: returnShipmentPayment},
		row{From: shipmentRefunding, Event: shipmentPulse, To: shipmentRefunding, Do: moveShipmentHome},
	)
	return rows
}
func (s *tradeShipment) terminal() bool {
	return s.lifecycle.State() == shipmentDelivered || s.lifecycle.State() == shipmentRecalled || s.lifecycle.State() == shipmentLost
}
func (c *shipmentContext) cart() *Entity { return c.World.liveTrader(c.Shipment) }
func (c *shipmentContext) moving() bool {
	cart := c.cart()
	return cart != nil && cart.behavior.State() == Caravanning && cart.Order.Shipment == c.Shipment.ID
}
func launchShipment(_ context.Context, c *shipmentContext) error {
	w, s, cart := c.World, c.Shipment, c.cart()
	w.Players[s.Buyer].Resources.Deposit(s.Terms.WantResource, -float64(s.Terms.WantAmount))
	cart.CargoType, cart.Cargo = s.Terms.WantResource, float64(s.Terms.WantAmount)
	w.setOrder(cart, Order{Kind: "caravan", Shipment: s.ID}, false)
	for _, player := range []int{s.Buyer, s.Seller} {
		w.tradeNotice(player, fmt.Sprintf("Caravan #%d accepted %s: %s.", s.ID, s.label(), tradeTerms(s.Terms)))
	}
	return nil
}
func shipmentCartLost(_ context.Context, c *shipmentContext) error {
	return applicable(c.cart() == nil || c.World.Players[c.Shipment.Buyer].lifecycle.State() == PlayerDefeated)
}
func shipmentPartnerLost(_ context.Context, c *shipmentContext) error {
	s, w := c.Shipment, c.World
	if s.Merchant != 0 {
		return applicable(w.merchantHost(w.Marketplace.Regions[s.Merchant]) == nil)
	}
	return applicable(!w.ownMarket(s.Seller, s.Market) || w.Players[s.Seller].lifecycle.State() == PlayerDefeated || w.relation(s.Seller, s.Buyer) != atPeace)
}
func shipmentAtSeller(_ context.Context, c *shipmentContext) error {
	return applicable(c.moving() && c.cart().Position.Distance(c.World.Entities[c.Shipment.Market].Position) <= tradeRadius(c.World.Entities[c.Shipment.Market])+.8)
}
func collectShipment(_ context.Context, c *shipmentContext) error {
	s, cart := c.Shipment, c.cart()
	payment := resourceAmount(s.Terms.WantResource, float64(s.Terms.WantAmount))
	c.World.accountConsumption(s.Buyer, payment, "trade")
	c.World.Players[s.Buyer].Economy.TradeSold.Add(payment)
	if s.Seller > 0 {
		goods := resourceAmount(s.Terms.GiveResource, float64(s.Terms.GiveAmount))
		c.World.accountConsumption(s.Seller, goods, "trade")
		p := c.World.Players[s.Seller]
		p.Economy.TradeSold.Add(goods)
		p.Economy.TradeBought.Add(payment)
		p.Economy.TradeDeliveries++
	}
	c.World.creditShipmentSeller(s, s.Terms.WantResource, float64(s.Terms.WantAmount))
	cart.CargoType, cart.Cargo, cart.Path = s.Terms.GiveResource, float64(s.Terms.GiveAmount), nil
	for _, player := range []int{s.Buyer, s.Seller} {
		c.World.tradeNotice(player, fmt.Sprintf("Caravan #%d paid %d %s and collected %d %s.", s.ID, s.Terms.WantAmount, s.Terms.WantResource, s.Terms.GiveAmount, s.Terms.GiveResource))
	}
	return nil
}
func moveShipmentToSeller(_ context.Context, c *shipmentContext) error {
	if c.moving() {
		c.World.move(c.cart(), c.World.Entities[c.Shipment.Market].Position, tradeRadius(c.World.Entities[c.Shipment.Market])+.7, Step)
	}
	return nil
}
func (c *shipmentContext) home() *Entity {
	if c.World.ownMarket(c.Shipment.Buyer, c.Shipment.Home) && c.World.Entities[c.Shipment.Home].Type == tradePostType(c.cart()) {
		home := c.World.Entities[c.Shipment.Home]
		if c.World.reachableFootprint(c.cart(), home.Position, tradeRadius(home)+.7) {
			return home
		}
	}
	return c.World.tradeHome(c.cart())
}
func shipmentAtHome(_ context.Context, c *shipmentContext) error {
	if !c.moving() {
		return applicable(false)
	}
	home := c.home()
	return applicable(home != nil && c.cart().Position.Distance(home.Position) <= tradeRadius(home)+.8)
}
func moveShipmentHome(_ context.Context, c *shipmentContext) error {
	if c.moving() {
		if home := c.home(); home != nil {
			c.World.move(c.cart(), home.Position, tradeRadius(home)+.7, Step)
		}
	}
	return nil
}
func finishShipmentCart(c *shipmentContext) {
	if cart := c.cart(); cart != nil {
		cart.Cargo, cart.CargoType = 0, ""
		if cart.Order.Shipment == c.Shipment.ID {
			order := Order{Kind: "idle"}
			mustFire(cart.behavior, StopOrder, &unitContext{World: c.World, Actor: cart, Order: &order, PreserveQueue: true})
		}
	}
}
func deliverShipment(_ context.Context, c *shipmentContext) error {
	s := c.Shipment
	c.World.Players[s.Buyer].Resources.Deposit(s.Terms.GiveResource, float64(s.Terms.GiveAmount))
	p := c.World.Players[s.Buyer]
	p.Economy.TradeBought.Add(resourceAmount(s.Terms.GiveResource, float64(s.Terms.GiveAmount)))
	p.Economy.TradeDeliveries++
	finishShipmentCart(c)
	for _, player := range []int{s.Buyer, s.Seller} {
		c.World.tradeNotice(player, fmt.Sprintf("Caravan #%d delivered %d %s. Trade complete.", s.ID, s.Terms.GiveAmount, s.Terms.GiveResource))
	}
	return nil
}
func recallShipment(_ context.Context, c *shipmentContext) error {
	s := c.Shipment
	c.World.creditShipmentSeller(s, s.Terms.GiveResource, float64(s.Terms.GiveAmount))
	if cart := c.cart(); cart != nil {
		cart.Path = nil
	}
	for _, player := range []int{s.Buyer, s.Seller} {
		c.World.tradeNotice(player, fmt.Sprintf("Caravan #%d recalled before collection. Reserved goods released; payment returns with the cart.", s.ID))
	}
	return nil
}
func returnShipmentPayment(_ context.Context, c *shipmentContext) error {
	s := c.Shipment
	c.World.Players[s.Buyer].Resources.Deposit(s.Terms.WantResource, float64(s.Terms.WantAmount))
	finishShipmentCart(c)
	c.World.tradeNotice(s.Buyer, fmt.Sprintf("Caravan #%d returned your %d %s payment.", s.ID, s.Terms.WantAmount, s.Terms.WantResource))
	return nil
}
func loseUncollectedShipment(ctx context.Context, c *shipmentContext) error {
	s := c.Shipment
	c.World.creditShipmentSeller(s, s.Terms.GiveResource, float64(s.Terms.GiveAmount))
	return loseShipment(ctx, c)
}
func loseShipment(_ context.Context, c *shipmentContext) error {
	s := c.Shipment
	// Conversion and resignation lose the cargo as well as destruction. It
	// cannot later be redeemed by issuing an unrelated resource/trade order.
	if cart := c.World.Entities[s.Cart]; cart != nil {
		cart.Cargo, cart.CargoType = 0, ""
	}
	finishShipmentCart(c)
	for _, player := range []int{s.Buyer, s.Seller} {
		c.World.tradeNotice(player, fmt.Sprintf("Caravan #%d was lost. Undelivered cargo was lost; goods still at the seller were released.", s.ID))
	}
	return nil
}

// Publishing grants this caravan passage through the trading partner's gates,
// only while peaceful. It grants neither general military access nor vision.
func (w *World) canUseGate(e, gate *Entity) bool {
	if gate.life.State() != Active {
		return false
	}
	if gate.Owner == e.Owner {
		return true
	}
	if e.Type == "supply_cart" && e.Owner == 0 {
		r := w.Marketplace.Regions[-gate.Owner]
		return r != nil && r.Cart == e.ID && r.lifecycle.State() == supplyTravelling
	}
	if e.Type != "trade_cart" || e.Order.Kind != "caravan" {
		return false
	}
	s := w.Marketplace.Shipments[e.Order.Shipment]
	return s != nil && !s.terminal() && s.Cart == e.ID && s.Buyer == e.Owner && s.Seller == gate.Owner && w.relation(s.Buyer, s.Seller) == atPeace
}
