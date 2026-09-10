package game

import "fmt"

// Destruction transfers only physically carried resources. Listings, warehouse
// stock, uncollected reservations and a kingdom's treasury are never loot.
// This runs once in the entity's lethal-damage transition, before removal.
func (w *World) captureSpoils(c *entityContext) {
	if w.Players[c.SourceOwner] == nil || c.SourceOwner == c.Actor.Owner {
		return
	}
	if c.Actor.Type == "supply_cart" {
		for _, id := range sortedTradeIDs(w.Marketplace.Regions) {
			r := w.Marketplace.Regions[id]
			if r.Cart == c.Actor.ID && r.lifecycle.State() == supplyTravelling {
				mustFire(r.lifecycle, supplyRaided, &supplyContext{World: w, Region: r, Raider: c.SourceOwner, Source: c.SourceID, Victim: c.Actor})
				return
			}
		}
	}
	loot := resourceAmount(c.Actor.CargoType, c.Actor.Cargo)
	c.Actor.Cargo, c.Actor.CargoType = 0, ""
	for _, id := range c.Actor.Passengers {
		if passenger := w.Entities[id]; passenger != nil {
			loot.Add(resourceAmount(passenger.CargoType, passenger.Cargo))
			passenger.Cargo, passenger.CargoType = 0, ""
		}
	}
	w.awardSpoils(c.SourceOwner, c.SourceID, c.Actor, loot)
}
func (w *World) awardSpoils(player, source int, victim *Entity, loot Resources) {
	p := w.Players[player]
	if p == nil || loot == (Resources{}) {
		return
	}
	p.Resources.Add(loot)
	for _, resource := range []string{"food", "wood", "gold", "stone"} {
		amount := loot.Amount(resource)
		if amount <= 0 {
			continue
		}
		w.record(Event{Kind: "spoils", Player: player, Message: fmt.Sprintf("Captured %.0f %s from %s #%d.", amount, resource, definitions[victim.Type].Name, victim.ID), TargetID: victim.ID, Resource: resource, Amount: amount}, w.Entities[source], player)
	}
}
