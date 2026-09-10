package game

import (
	"context"
	"fmt"
	"github.com/open-ships/statemachine"
	"math"
)

func combatTransitions() []unitRow {
	rows := []unitRow{}
	for _, state := range []UnitState{Chasing, Attacking, AttackCooldown} {
		rows = append(rows, unitRow{From: state, Event: UnitPulse, To: Idle, Guard: combatTargetLost, Do: finishUnitOrder})
	}
	return append(rows,
		unitRow{From: Chasing, Event: UnitPulse, To: Idle, Guard: cannotPursue, Do: finishUnitOrder},
		unitRow{From: Chasing, Event: UnitPulse, To: Attacking, Guard: attackInRange, Do: beginAttackWindup},
		unitRow{From: Chasing, Event: UnitPulse, To: Chasing, Do: pursueTarget},
		unitRow{From: Attacking, Event: UnitPulse, To: Chasing, Guard: invalid(attackInRange), Do: resetAttackWork},
		unitRow{From: Attacking, Event: UnitPulse, To: AttackCooldown, Guard: attackReady, Do: releaseAttack},
		unitRow{From: Attacking, Event: UnitPulse, To: Attacking, Do: advanceAttackWindup},
		unitRow{From: AttackCooldown, Event: UnitPulse, To: Chasing, Guard: cooldownReady},
		unitRow{From: AttackCooldown, Event: UnitPulse, To: AttackCooldown, Do: advanceCooldown},
	)
}
func combatTargetLost(_ context.Context, c *unitContext) error {
	return applicable(c.Target == nil || c.Target.Owner == 0 && c.Target.Type != "supply_cart" || c.Target.Owner == c.Actor.Owner || !c.World.visibleEntity(c.Actor.Owner, c.Target) || (c.Actor.Type == "town_center" && len(c.Actor.Passengers) == 0) || !c.World.mayContinueAttack(c.Actor, c.Target))
}

func (w *World) mayContinueAttack(e, target *Entity) bool {
	if w.treatyInForce() {
		return false
	}
	if target == nil || e.Order.TargetPlayer != 0 && e.Order.TargetPlayer != target.Owner {
		return false
	}
	if e.Order.Initiated {
		return true
	}
	if e.Stance == "passive" {
		return false
	}
	if e.Stance == "aggressive" {
		return true
	}
	anchor := e.Order.DefendFrom
	if anchor == nil || e.Position.Distance(*anchor) > defenseLeash {
		return false
	}
	_, attacked := w.recentAggressor(e.Owner, target.ID, target.Owner)
	return attacked && target.Position.Distance(*anchor) <= defenseLeash+w.stats(e).Range+definitions[target.Type].Radius
}
func attackInRange(_ context.Context, c *unitContext) error {
	d := c.World.stats(c.Actor)
	distance := c.Actor.Position.Distance(c.Target.Position) - definitions[c.Target.Type].Radius
	return applicable(distance <= d.Range+.1 && distance >= d.MinRange && (c.Actor.Type != "trebuchet" || c.Actor.siege.State() == SiegeDeployed))
}
func cannotPursue(ctx context.Context, c *unitContext) error {
	return applicable(attackInRange(ctx, c) != nil && (definitions[c.Actor.Type].Kind == "building" || c.Actor.Stance == "stand_ground" || c.Actor.Type == "trebuchet"))
}
func beginAttackWindup(_ context.Context, c *unitContext) error { c.Actor.Work = .25; return nil }
func resetAttackWork(_ context.Context, c *unitContext) error   { c.Actor.Work = 0; return nil }
func attackReady(_ context.Context, c *unitContext) error       { return applicable(c.Actor.Work <= Step) }
func cooldownReady(_ context.Context, c *unitContext) error {
	return applicable(c.Actor.Cooldown <= Step)
}
func advanceAttackWindup(_ context.Context, c *unitContext) error { c.Actor.Work -= Step; return nil }
func advanceCooldown(_ context.Context, c *unitContext) error {
	c.Actor.Cooldown = math.Max(0, c.Actor.Cooldown-Step)
	return nil
}
func pursueTarget(_ context.Context, c *unitContext) error {
	e, t, w := c.Actor, c.Target, c.World
	d := w.stats(e)
	if e.Position.Distance(t.Position)-definitions[t.Type].Radius < d.MinRange {
		dx, dy := e.Position.X-t.Position.X, e.Position.Y-t.Position.Y
		length := math.Max(.01, math.Hypot(dx, dy))
		goal := Vec{e.Position.X + dx/length*2, e.Position.Y + dy/length*2}
		if w.inside(goal) {
			w.move(e, goal, .6, Step)
		}
		return nil
	}
	w.move(e, t.Position, d.Range+definitions[t.Type].Radius, Step)
	return nil
}
func releaseAttack(_ context.Context, c *unitContext) error {
	w, e, t := c.World, c.Actor, c.Target
	d := w.stats(e)
	damage := w.damage(e, t)
	e.Cooldown = d.Reload
	if e.Type == "town_center" {
		damage *= math.Min(3, float64(len(e.Passengers)))
	}
	w.record(Event{Kind: "attack", Message: fmt.Sprintf("Attacked %s #%d", definitions[t.Type].Name, t.ID), TargetID: t.ID, Amount: damage}, e, 0)
	w.noteAggression(e.ID, e.Owner, t)
	if !d.Projectile {
		w.hitFrom(t, e.Owner, e.ID, damage)
		return nil
	}
	splash, kind, speed := 0., "arrow", 9.
	if d.Class == "siege" {
		splash, kind, speed = 1.4, "stone", 6
	}
	p := Projectile{ID: w.NextID, Owner: e.Owner, Position: e.Position, Destination: t.Position, Source: e.ID, Target: t.ID, Damage: damage, Splash: splash, Speed: speed, Kind: kind, flight: statemachine.NewInstance(flightMachine, Flying)}
	w.NextID++
	w.Projectiles = append(w.Projectiles, p)
	return nil
}
func (w *World) hit(target *Entity, owner int, damage float64) {
	w.hitFrom(target, owner, 0, damage)
}
func (w *World) hitFrom(target *Entity, owner, source int, damage float64) {
	if target != nil && !(w.treatyInForce() && owner > 0 && target.Owner > 0 && owner != target.Owner) {
		w.noteAggression(source, owner, target)
		mustFire(target.life, DamageEntity, &entityContext{World: w, Actor: target, Amount: damage, SourceOwner: owner})
	}
}

type flightContext struct {
	World       *World
	Projectile  *Projectile
	Destination Vec
}

var flightMachine = statemachine.MustCompile([]statemachine.Transition[FlightState, FlightEvent, *flightContext]{
	{From: Flying, Event: FlightPulse, To: Impacted, Guard: func(_ context.Context, c *flightContext) error {
		return applicable(c.Projectile.Position.Distance(c.Destination) <= c.Projectile.Speed*Step)
	}, Do: impactProjectile},
	{From: Flying, Event: FlightPulse, To: Flying, Do: advanceProjectile},
})

func advanceProjectile(_ context.Context, c *flightContext) error {
	p := c.Projectile
	p.Destination = c.Destination
	distance := p.Position.Distance(p.Destination)
	p.Position.X += (p.Destination.X - p.Position.X) / distance * p.Speed * Step
	p.Position.Y += (p.Destination.Y - p.Position.Y) / distance * p.Speed * Step
	return nil
}
func impactProjectile(_ context.Context, c *flightContext) error {
	w, p := c.World, c.Projectile
	p.Position = c.Destination
	p.Destination = c.Destination
	if p.Splash > 0 {
		for _, id := range append([]int{}, w.IDs...) {
			t := w.Entities[id]
			if t != nil && t.Container == 0 && t.ID != p.Source && (t.Owner > 0 || t.Type == "supply_cart") && t.Position.Distance(p.Destination) <= p.Splash+definitions[t.Type].Radius {
				w.hitFrom(t, p.Owner, p.Source, p.Damage)
			}
		}
		return nil
	}
	t := w.Entities[p.Target]
	if t != nil && t.Container == 0 && t.Position.Distance(p.Destination) <= definitions[t.Type].Radius+.5 {
		w.hitFrom(t, p.Owner, p.Source, p.Damage)
	}
	return nil
}
func (w *World) updateProjectiles() {
	remaining := w.Projectiles[:0]
	for i := range w.Projectiles {
		p := w.Projectiles[i]
		dest := p.Destination
		if w.Players[p.Owner].Technologies["ballistics"] {
			if t := w.Entities[p.Target]; t != nil && t.Container == 0 {
				dest = t.Position
			}
		}
		mustFire(p.flight, FlightPulse, &flightContext{World: w, Projectile: &p, Destination: dest})
		if p.flight.State() == Flying {
			remaining = append(remaining, p)
		}
	}
	w.Projectiles = remaining
}

func (w *World) acquire(e *Entity) *Entity {
	if w.treatyInForce() {
		return nil
	}
	d := w.stats(e)
	deliberate := e.Order.Kind == "attack_move"
	if d.Attack <= 0 || e.Owner == 0 || e.Stance == "passive" && !deliberate {
		return nil
	}
	if e.Type == "town_center" && len(e.Passengers) == 0 {
		return nil
	}
	r := d.Sight
	if e.Stance == "stand_ground" || d.Kind == "building" {
		r = d.Range
	}
	return w.nearest(e.Position, func(t *Entity) bool {
		if t.Owner == 0 || t.Owner == e.Owner || !w.visibleEntity(e.Owner, t) || e.Position.Distance(t.Position) > r+definitions[t.Type].Radius || definitions[t.Type].Kind == "resource" {
			return false
		}
		if deliberate && (e.Order.TargetPlayer == 0 || e.Order.TargetPlayer == t.Owner) {
			return true
		}
		if !deliberate && e.Stance == "aggressive" && d.Class != "worker" {
			return true
		}
		if e.Stance == "passive" {
			return false
		}
		incident, attacked := w.recentAggressor(e.Owner, t.ID, t.Owner)
		if !attacked || d.Class == "worker" && incident.Victim != e.ID {
			return false
		}
		if t.Position.Distance(incident.Position) > defenseLeash+d.Range+definitions[t.Type].Radius {
			return false
		}
		// Help nearby friends against their actual attacker. A foreign army
		// walking past, or an innocent neighbor of the attacker, is no target.
		return incident.Victim == e.ID || incident.Position.Distance(e.Position) <= defenseLeash
	})
}
func (w *World) damage(e, t *Entity) float64 {
	d, td := w.stats(e), w.stats(t)
	armor := td.Armor
	if d.Projectile {
		armor = td.PierceArmor
	}
	damage := math.Max(1, d.Attack-armor)
	if e.Type == "spearman" && (td.Class == "cavalry" || td.Class == "mounted_archer") {
		damage += 15
		if w.Players[e.Owner].Technologies["pikeman"] {
			damage += 10
		}
	}
	if e.Type == "camel" || e.Type == "mameluke" {
		if td.Class == "cavalry" {
			damage += 9
		}
	}
	if e.Type == "skirmisher" && (td.Class == "archer" || td.Class == "mounted_archer") {
		damage += 4
	}
	if e.Type == "ram" && td.Kind == "building" {
		damage += 125
	}
	if e.Type == "mangudai" && td.Class == "siege" {
		damage += 7
	}
	if e.Type == "cataphract" && td.Class == "infantry" {
		damage += 9
	}
	if d.Class == "siege" && td.Kind == "building" && w.Players[e.Owner].Technologies["siege_engineers"] {
		damage *= 1.2
	}
	if w.tile(e.Position).Elevation > w.tile(t.Position).Elevation+.1 {
		damage *= 1.25
	}
	return damage
}
