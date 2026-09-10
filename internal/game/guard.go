package game

import (
	"context"
	"math"
)

// The unit machine owns both escorting and combat. A guard order keeps its
// protected target throughout an engagement; Threat is only the current foe.
// There is no second movement controller or suspended duplicate guard order.
func guardUnit(_ context.Context, c *unitContext) error {
	d := definitions[c.Actor.Type]
	if d.Kind != "unit" || d.Attack <= 0 || d.Speed <= 0 || d.Class == "worker" || c.Actor.Type == "trebuchet" || c.Actor.Container != 0 {
		return rule("invalid_guard", "Choose mobile soldiers or warships to guard this target.")
	}
	return nil
}

func (w *World) guardTarget(guard, target *Entity) error {
	if target == nil || target == guard || target.life.State() != Active || target.Container != 0 || !w.visibleEntity(guard.Owner, target) {
		return rule("invalid_guard_target", "Choose an observable friendly unit, Market, Dock, or supply caravan.")
	}
	d, td := definitions[guard.Type], definitions[target.Type]
	if target.Owner == 0 && target.Type != "supply_cart" || target.Owner != 0 && target.Owner != guard.Owner && w.relation(guard.Owner, target.Owner) != atPeace || td.Kind != "unit" && target.Type != "market" && target.Type != "dock" {
		return rule("invalid_guard_target", "Guards protect friendly units, Markets, Docks and neutral supply caravans.")
	}
	if td.Kind == "unit" && d.Naval != td.Naval || td.Kind == "building" && d.Naval && target.Type != "dock" {
		return rule("guard_terrain", "Soldiers escort on land; warships escort at sea.")
	}
	return nil
}

func guardLost(_ context.Context, c *unitContext) error {
	if c.Actor.Order.Kind != "guard" {
		return applicable(false)
	}
	t := c.World.Entities[c.Actor.Order.Target]
	return applicable(t == nil || t.Owner != c.Actor.Order.TargetPlayer || c.World.guardTarget(c.Actor, t) != nil)
}

func (w *World) guardMayFight(e, enemy *Entity) bool {
	ward := w.Entities[e.Order.Target]
	if ward == nil || enemy == nil || e.Stance == "passive" || w.treatyInForce() || !w.visibleEntity(e.Owner, enemy) || enemy.Owner == 0 || enemy.Owner == e.Owner || enemy.Owner == ward.Owner {
		return false
	}
	d := w.stats(e)
	if e.Position.Distance(ward.Position) > defenseLeash+definitions[ward.Type].Radius || enemy.Position.Distance(ward.Position) > defenseLeash+d.Range+definitions[enemy.Type].Radius {
		return false
	}
	incident, attacked := w.recentAggressor(e.Owner, enemy.ID, enemy.Owner)
	return w.relation(e.Owner, enemy.Owner) == inConflict || attacked && (incident.Victim == ward.ID || incident.Victim == e.ID)
}

func (w *World) guardThreat(e *Entity) *Entity {
	return w.nearest(e.Position, func(t *Entity) bool {
		return definitions[t.Type].Attack > 0 && e.Position.Distance(t.Position) <= w.stats(e).Sight+definitions[t.Type].Radius && w.guardMayFight(e, t)
	})
}

func guardCombatEnded(_ context.Context, c *unitContext) error {
	return applicable(c.Actor.Order.Kind == "guard" && !c.World.guardMayFight(c.Actor, c.Target))
}
func guardCannotPursue(ctx context.Context, c *unitContext) error {
	return applicable(c.Actor.Order.Kind == "guard" && cannotPursue(ctx, c) == nil)
}
func endGuardCombat(_ context.Context, c *unitContext) error {
	c.Actor.Order.Threat = 0
	c.Actor.Path, c.Actor.Work = nil, 0
	return nil
}
func engageGuardThreat(_ context.Context, c *unitContext) error {
	c.Actor.Order.Threat = c.Candidate.ID
	c.Actor.Path, c.Actor.Work = nil, 0
	return nil
}
func followGuardTarget(_ context.Context, c *unitContext) error {
	e, w := c.Actor, c.World
	t := w.Entities[e.Order.Target]
	radius := definitions[t.Type].Radius + 1.4
	angle := float64(e.ID%8) * math.Pi / 4
	goal := Vec{t.Position.X + math.Cos(angle)*radius, t.Position.Y + math.Sin(angle)*radius}
	if !w.inside(goal) || definitions[e.Type].Naval && !w.water(goal) || !definitions[e.Type].Naval && !w.land(goal) {
		w.move(e, t.Position, radius, Step)
	} else {
		w.move(e, goal, .6, Step)
	}
	return nil
}
func guardTransitions() []unitRow {
	return []unitRow{
		{From: Guarding, Event: UnitPulse, To: Idle, Guard: guardLost, Do: finishUnitOrder},
		{From: Guarding, Event: UnitPulse, To: Chasing, Guard: func(_ context.Context, c *unitContext) error { return applicable(c.Candidate != nil) }, Do: engageGuardThreat},
		{From: Guarding, Event: UnitPulse, To: Guarding, Do: followGuardTarget},
	}
}

// Protecting a neutral caravan or a peaceful partner grants knowledge only of
// attacks actually witnessed by the escort, not access to foreign orders/logs.
func (w *World) observeGuardAttack(source, owner int, target *Entity) {
	attacker := w.Entities[source]
	if attacker == nil || target == nil || owner == 0 {
		return
	}
	for _, id := range w.IDs {
		guard := w.Entities[id]
		if guard == nil || guard.Owner == owner || guard.Order.Kind != "guard" || guard.Order.Target != target.ID || !w.visibleEntity(guard.Owner, attacker) || !w.visibleEntity(guard.Owner, target) || guard.Position.Distance(target.Position) > defenseLeash+definitions[target.Type].Radius {
			continue
		}
		if w.Incidents[guard.Owner] == nil {
			w.Incidents[guard.Owner] = map[int]aggression{}
		}
		w.Incidents[guard.Owner][source] = aggression{Owner: owner, Victim: target.ID, Position: target.Position, Time: w.Time}
	}
}
