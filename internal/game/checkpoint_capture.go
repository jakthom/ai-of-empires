package game

import (
	"maps"
	"slices"
)

func checkpointPointer[T any](v *T) *T {
	if v == nil {
		return nil
	}
	copy := *v
	return &copy
}

func checkpointOrder(o Order) Order {
	o.Position, o.DefendFrom = checkpointPointer(o.Position), checkpointPointer(o.DefendFrom)
	return o
}

// Copy the exported persistence data, not runtime caches or state machines.
// Lifecycle states have already been captured by checkpointState. This keeps
// large JSON encoding and SQLite I/O outside the simulation's mutation lock.
func (w *World) freezeCheckpointWorld() *World {
	c := *w
	// These are not serialized. Do not keep old journals, spatial caches or
	// live lifecycle instances reachable through a slow background save.
	c.journal = eventJournal{}
	c.match, c.peacePeriod = nil, nil
	c.landRegions, c.waterRegions, c.barriers, c.viewMaps = nil, nil, nil, nil
	c.Tiles, c.IDs = slices.Clone(w.Tiles), slices.Clone(w.IDs)
	c.Entities = make(map[int]*Entity, len(w.Entities))
	for id, e := range w.Entities {
		v := *e
		v.behavior, v.life, v.production, v.siege = nil, nil, nil, nil
		v.Order = checkpointOrder(e.Order)
		v.Orders = slices.Clone(e.Orders)
		for i := range v.Orders {
			v.Orders[i] = checkpointOrder(v.Orders[i])
		}
		v.Tasks, v.Path, v.Passengers = slices.Clone(e.Tasks), slices.Clone(e.Path), slices.Clone(e.Passengers)
		v.Rally = checkpointPointer(e.Rally)
		c.Entities[id] = &v
	}
	c.Players = make(map[int]*Player, len(w.Players))
	for id, p := range w.Players {
		v := *p
		v.voyage, v.strategy, v.lifecycle = nil, nil, nil
		v.UserAliases = slices.Clone(p.UserAliases)
		v.Production.Buckets, v.Production.History = slices.Clone(p.Production.Buckets), slices.Clone(p.Production.History)
		v.Technologies = maps.Clone(p.Technologies)
		v.AIPlan.Army, v.AIPlan.ScoutGoal, v.AIPlan.Surveyed = slices.Clone(p.AIPlan.Army), checkpointPointer(p.AIPlan.ScoutGoal), maps.Clone(p.AIPlan.Surveyed)
		v.NavalPlan.Crew = slices.Clone(p.NavalPlan.Crew)
		v.Explored, v.Visible = slices.Clone(p.Explored), slices.Clone(p.Visible)
		v.Memory = maps.Clone(p.Memory)
		for key, e := range v.Memory {
			e.Actions, e.Tasks, e.Passengers, e.Connections = slices.Clone(e.Actions), slices.Clone(e.Tasks), slices.Clone(e.Passengers), slices.Clone(e.Connections)
			e.Rally = checkpointPointer(e.Rally)
			for i := range e.Actions {
				e.Actions[i].Gain = checkpointPointer(e.Actions[i].Gain)
			}
			v.Memory[key] = e
		}
		c.Players[id] = &v
	}
	c.Relations = make(map[string]*relationship, len(w.Relations))
	for id, r := range w.Relations {
		c.Relations[id] = checkpointPointer(r)
		c.Relations[id].lifecycle = nil
	}
	c.Incidents = maps.Clone(w.Incidents)
	for id, incidents := range w.Incidents {
		c.Incidents[id] = maps.Clone(incidents)
	}
	c.Projectiles = slices.Clone(w.Projectiles)
	for i := range c.Projectiles {
		c.Projectiles[i].flight = nil
	}
	c.Marketplace.Offers = make(map[int]*tradeOffer, len(w.Marketplace.Offers))
	c.Marketplace.Regions = make(map[int]*merchantRegion, len(w.Marketplace.Regions))
	for id, r := range w.Marketplace.Regions {
		copy := *r
		copy.lifecycle = nil
		c.Marketplace.Regions[id] = &copy
	}
	for id, o := range w.Marketplace.Offers {
		copy := *o
		copy.lifecycle = nil
		c.Marketplace.Offers[id] = &copy
	}
	c.Marketplace.Shipments = make(map[int]*tradeShipment, len(w.Marketplace.Shipments))
	for id, s := range w.Marketplace.Shipments {
		copy := *s
		copy.lifecycle = nil
		c.Marketplace.Shipments[id] = &copy
	}
	return &c
}
