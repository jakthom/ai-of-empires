package game

import (
	"context"
	"math"
	"slices"
)

// Unit ticking is an event, not a switch that chooses a next state.
func (w *World) behave(e *Entity) {
	c := &unitContext{World: w, Actor: e, Target: w.Entities[e.Order.Target], Roll: 1}
	if e.behavior.State() == Converting && e.Work >= 4 {
		c.Roll = w.random()
	}
	if e.behavior.State() == Idle || e.behavior.State() == Moving {
		c.Candidate = w.acquire(e)
	}
	if e.behavior.State() == Returning {
		c.DropOff = w.dropOff(e)
	}
	if e.behavior.State() == SeekingResource || e.behavior.State() == Gathering {
		if c.Target == nil || c.Target.Amount <= 0 {
			c.Candidate = w.nearest(e.Position, func(t *Entity) bool {
				return validResource(e, t) && t.Resource == e.CargoType && w.visibleEntity(e.Owner, t) && w.sameRegion(e.Position, t.Position, definitions[e.Type].Naval)
			})
		}
	}
	if e.behavior.State() == Trading || e.behavior.State() == ReturningTrade {
		c.DropOff = w.tradeHome(e)
	}
	mustFire(e.behavior, UnitPulse, c)
	if e.behavior.State() == Idle && len(e.Orders) > 0 {
		w.advanceOrder(e)
	}
}
func (w *World) setOrder(e *Entity, o Order, queue bool) {
	c := &unitContext{World: w, Actor: e, Order: &o}
	event := orderEvent(o.Kind)
	if queue && e.behavior.State() != Idle {
		event = QueueOrder
	}
	mustFire(e.behavior, event, c)
}
func (w *World) advanceOrder(e *Entity) {
	if len(e.Orders) == 0 {
		o := Order{Kind: "idle"}
		mustFire(e.behavior, StopOrder, &unitContext{World: w, Actor: e, Order: &o})
		return
	}
	o := e.Orders[0]
	e.Orders = e.Orders[1:]
	mustFire(e.behavior, orderEvent(o.Kind), &unitContext{World: w, Actor: e, Order: &o, PreserveQueue: true})
}
func acceptOrder(_ context.Context, c *unitContext) error {
	if c.Order == nil {
		return rule("missing_order", "An order is required.")
	}
	e := c.Actor
	e.Order = *c.Order
	e.Path = nil
	e.Repath = 0
	e.Work = 0
	if !c.PreserveQueue {
		e.Orders = nil
	}
	return nil
}
func queueHasRoom(_ context.Context, c *unitContext) error {
	if len(c.Actor.Orders) >= 64 {
		return rule("queue_full", "The order queue is full.")
	}
	return nil
}
func appendOrder(_ context.Context, c *unitContext) error {
	if c.Order == nil {
		return rule("missing_order", "An order is required.")
	}
	c.Actor.Orders = append(c.Actor.Orders, *c.Order)
	return nil
}
func finishUnitOrder(_ context.Context, c *unitContext) error {
	c.Actor.Order = Order{Kind: "idle"}
	c.Actor.Path = nil
	c.Actor.Work = 0
	return nil
}
func applicable(ok bool) error {
	if ok {
		return nil
	}
	return rule("guard_declined", "Transition condition does not apply.")
}
func movementTransitions() []unitRow {
	return []unitRow{
		{From: Idle, Event: UnitPulse, To: Chasing, Guard: func(_ context.Context, c *unitContext) error {
			return applicable(c.Candidate != nil)
		}, Do: engageCandidate},
		{From: Idle, Event: UnitPulse, To: Idle},
		{From: Moving, Event: UnitPulse, To: Chasing, Guard: func(_ context.Context, c *unitContext) error {
			return applicable(c.Actor.Order.Kind == "attack_move" && c.Candidate != nil)
		}, Do: engageCandidate},
		{From: Moving, Event: UnitPulse, To: Idle, Guard: func(_ context.Context, c *unitContext) error {
			return applicable(c.Actor.Order.Position == nil || c.Actor.Position.Distance(*c.Actor.Order.Position) <= .8)
		}, Do: finishUnitOrder},
		{From: Moving, Event: UnitPulse, To: Moving, Do: func(_ context.Context, c *unitContext) error {
			c.World.move(c.Actor, *c.Actor.Order.Position, .8, Step)
			return nil
		}},
	}
}
func engageCandidate(_ context.Context, c *unitContext) error {
	order := Order{Kind: "attack", Target: c.Candidate.ID}
	if c.Actor.Order.Kind == "attack_move" {
		if c.Actor.Order.TargetPlayer == 0 || c.Actor.Order.TargetPlayer == c.Candidate.Owner {
			order.Initiated = true
			order.TargetPlayer = c.Actor.Order.TargetPlayer
		}
		c.Actor.Orders = append([]Order{c.Actor.Order}, c.Actor.Orders...)
	}
	if !order.Initiated && c.Actor.Stance != "aggressive" {
		anchor := c.Actor.Position
		order.DefendFrom = &anchor
	}
	c.Actor.Order = order
	c.Actor.Path = nil
	return nil
}
func transportTransitions() []unitRow {
	return []unitRow{
		{From: Embarking, Event: UnitPulse, To: Idle, Guard: func(_ context.Context, c *unitContext) error {
			return applicable(c.Target == nil || c.Target.Owner != c.Actor.Owner || c.Target.life.State() != Active || len(c.Target.Passengers) >= c.World.garrisonCapacity(c.Target))
		}, Do: finishUnitOrder},
		{From: Embarking, Event: UnitPulse, To: Garrisoned, Guard: func(_ context.Context, c *unitContext) error {
			return applicable(c.Actor.Position.Distance(c.Target.Position) <= definitions[c.Target.Type].Radius+.85)
		}, Do: embarkPassenger},
		{From: Embarking, Event: UnitPulse, To: Embarking, Do: func(_ context.Context, c *unitContext) error {
			c.World.move(c.Actor, c.Target.Position, definitions[c.Target.Type].Radius+.8, Step)
			return nil
		}},
		{From: Garrisoned, Event: UnitPulse, To: Garrisoned, Do: maintainGarrison},
		{From: Garrisoned, Event: Disembark, To: Idle, Guard: canDisembark, Do: disembarkPassenger},
	}
}
func embarkPassenger(_ context.Context, c *unitContext) error {
	c.Actor.Container = c.Target.ID
	c.Target.Passengers = append(c.Target.Passengers, c.Actor.ID)
	c.Actor.Path = nil
	return nil
}
func maintainGarrison(_ context.Context, c *unitContext) error {
	if b := c.World.Entities[c.Actor.Container]; b != nil {
		c.Actor.Position = b.Position
		c.Actor.HP = math.Min(c.World.stats(c.Actor).HP, c.Actor.HP+Step*.5)
	}
	return nil
}
func canDisembark(_ context.Context, c *unitContext) error {
	b := c.World.Entities[c.Actor.Container]
	if b == nil {
		return rule("invalid_container", "Garrison no longer exists.")
	}
	_, ok := c.World.exit(b, c.Actor)
	if !ok {
		return rule("blocked_exit", "There is no clear exit. Move the transport closer to land.")
	}
	return nil
}
func disembarkPassenger(ctx context.Context, c *unitContext) error {
	b := c.World.Entities[c.Actor.Container]
	pos, _ := c.World.exit(b, c.Actor)
	c.Actor.Container = 0
	c.Actor.Position = pos
	b.Passengers = slices.DeleteFunc(b.Passengers, func(id int) bool { return id == c.Actor.ID })
	return finishUnitOrder(ctx, c)
}
