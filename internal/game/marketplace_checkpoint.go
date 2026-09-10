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
		w.initializeMerchantRegions()
		return nil
	}
	m := &w.Marketplace
	if m.NextOffer < 1 || m.NextShipment < 1 || m.LegacyRevision < 0 || m.Offers == nil || m.Shipments == nil || len(c.Offers) != len(m.Offers) || len(c.Shipments) != len(m.Shipments) {
		return fmt.Errorf("invalid checkpoint marketplace")
	}
	for _, kind := range []string{"food", "wood", "gold", "stone"} {
		n := m.LegacyStock.Amount(kind)
		if n < 0 || math.IsNaN(n) || math.IsInf(n, 0) {
			return fmt.Errorf("invalid checkpoint merchant stock")
		}
	}
	if c.Version < 6 {
		m.Regions = map[int]*merchantRegion{}
		w.initializeMerchantRegions()
		// Preserve the old shared inventory exactly; distribute it, never copy
		// it into every new region. Future supplies establish regional surpluses.
		for _, r := range m.Regions {
			r.Stock = m.LegacyStock.Scale(1 / float64(len(m.Regions)))
		}
		m.LegacyStock, m.LegacyRevision = Resources{}, 0
	} else if err := w.restoreMerchantRegions(c); err != nil {
		return err
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
		if s == nil || s.ID != id || id < 1 || id >= m.NextShipment || s.Cart <= 0 || s.Home <= 0 || s.Market <= 0 || s.Buyer == s.Seller || w.Players[s.Buyer] == nil || (s.Merchant == 0 && w.Players[s.Seller] == nil) || s.Merchant != 0 && (s.Seller != 0 || s.Merchant != s.Market || m.Regions[s.Merchant] == nil || !slices.Contains([]string{"buy", "sell"}, s.TradeMode) || s.Limit < 1 || s.Limit > tradeCapacity || !slices.Contains([]string{"food", "wood", "stone"}, s.Product)) || !validTradeTerms(s.Terms) || !slices.Contains([]shipmentState{shipmentOutbound, shipmentReturning, shipmentRefunding, shipmentDelivered, shipmentRecalled, shipmentLost}, state) {
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

func (w *World) restoreMerchantRegions(c checkpoint) error {
	if len(c.Supplies) != len(w.Marketplace.Regions) || len(c.Supplies) < w.Config.Settlements {
		return fmt.Errorf("invalid checkpoint merchant regions")
	}
	carts := map[int]bool{}
	for id, r := range w.Marketplace.Regions {
		state := c.Supplies[id]
		if r == nil || r.ID != id || id == 0 || id < 0 && w.Players[-id] == nil || !slices.Contains([]supplyState{supplyWaiting, supplyTravelling}, state) || r.Revision < 0 || r.Deliveries < 0 || r.Losses < 0 || math.IsNaN(r.NextSupply) || math.IsInf(r.NextSupply, 0) || r.NextSupply < 0 || r.Cart < 0 || (state == supplyTravelling) != (r.Cart > 0) {
			return fmt.Errorf("invalid checkpoint supply %d", id)
		}
		for _, stock := range []Resources{r.Stock, r.Cargo, r.Output, r.Demand} {
			for _, kind := range []string{"food", "wood", "gold", "stone"} {
				n := stock.Amount(kind)
				if n < 0 || math.IsNaN(n) || math.IsInf(n, 0) {
					return fmt.Errorf("invalid checkpoint local inventory %d", id)
				}
			}
		}
		if state == supplyWaiting && r.Cargo != (Resources{}) || r.Cart > 0 && carts[r.Cart] {
			return fmt.Errorf("invalid checkpoint supply cargo %d", id)
		}
		if r.Cart > 0 {
			carts[r.Cart] = true
		}
		r.lifecycle = statemachine.NewInstance(supplyMachine, state)
	}
	for id := 1; id <= w.Config.Settlements; id++ {
		if w.Marketplace.Regions[-id] == nil {
			return fmt.Errorf("missing home merchants")
		}
	}
	return nil
}
