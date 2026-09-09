package game

import (
	"fmt"
	"math"
	"slices"

	"github.com/open-ships/statemachine"
)

func (w *World) restoreMarketplace(c checkpoint) error {
	if c.Version < 5 {
		w.initializeMarketplace()
		return nil
	}
	m := &w.Marketplace
	if m.NextOffer < 1 || m.NextShipment < 1 || m.Revision < 0 || m.Offers == nil || m.Shipments == nil || len(c.Offers) != len(m.Offers) || len(c.Shipments) != len(m.Shipments) {
		return fmt.Errorf("invalid checkpoint marketplace")
	}
	for _, kind := range []string{"food", "wood", "gold", "stone"} {
		n := m.Merchants.Amount(kind)
		if n < 0 || math.IsNaN(n) || math.IsInf(n, 0) {
			return fmt.Errorf("invalid checkpoint merchant stock")
		}
	}
	for id, o := range m.Offers {
		state := c.Offers[id]
		if o == nil || o.ID != id || id < 1 || id >= m.NextOffer || w.Players[o.Owner] == nil || !validTradeTerms(o.Terms) || o.Terms.TargetPlayer < 0 || o.Terms.TargetPlayer > w.Config.Settlements || o.Terms.TargetPlayer == o.Owner || !slices.Contains([]offerState{offerOpen, offerFilled, offerCancelled}, state) || o.Remaining < 0 || o.Remaining > o.Terms.Lots || (state == offerOpen) != (o.Remaining > 0) {
			return fmt.Errorf("invalid checkpoint offer %d", id)
		}
		o.lifecycle = statemachine.NewInstance(offerMachine, state)
	}
	carts := map[int]bool{}
	for id, s := range m.Shipments {
		state := c.Shipments[id]
		if s == nil || s.ID != id || id < 1 || id >= m.NextShipment || s.Cart <= 0 || s.Home <= 0 || s.Market <= 0 || s.Buyer == s.Seller || w.Players[s.Buyer] == nil || w.Players[s.Seller] == nil || !validTradeTerms(s.Terms) || !slices.Contains([]shipmentState{shipmentOutbound, shipmentReturning, shipmentRefunding, shipmentDelivered, shipmentRecalled, shipmentLost}, state) {
			return fmt.Errorf("invalid checkpoint shipment %d", id)
		}
		s.lifecycle = statemachine.NewInstance(shipmentMachine, state)
		if !s.terminal() {
			if carts[s.Cart] {
				return fmt.Errorf("duplicate checkpoint caravan %d", s.Cart)
			}
			carts[s.Cart] = true
		}
	}
	return nil
}
