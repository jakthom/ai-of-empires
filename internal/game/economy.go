package game

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/open-ships/statemachine"
)

func economyTransitions() []unitRow {
	rows := []unitRow{}
	for _, state := range []UnitState{SeekingResource, Gathering} {
		rows = append(rows,
			unitRow{From: state, Event: UnitPulse, To: Returning, Guard: needsDropOff, Do: clearRoute},
			unitRow{From: state, Event: UnitPulse, To: Constructing, Guard: canReseed, Do: reseedTarget},
			unitRow{From: state, Event: UnitPulse, To: SeekingResource, Guard: replacementAvailable, Do: retargetResource},
			unitRow{From: state, Event: UnitPulse, To: Idle, Guard: resourceLost, Do: finishUnitOrder},
		)
	}
	rows = append(rows,
		unitRow{From: SeekingResource, Event: UnitPulse, To: SeekingResource, Guard: farmOccupied},
		unitRow{From: SeekingResource, Event: UnitPulse, To: Gathering, Guard: resourceReached, Do: beginGathering},
		unitRow{From: SeekingResource, Event: UnitPulse, To: SeekingResource, Do: approachResource},
		unitRow{From: Gathering, Event: UnitPulse, To: Gathering, Do: gatherResource},
		unitRow{From: Returning, Event: UnitPulse, To: Returning, Guard: func(_ context.Context, c *unitContext) error { return applicable(c.DropOff == nil) }},
		unitRow{From: Returning, Event: UnitPulse, To: SeekingResource, Guard: dropOffReached, Do: depositCargo},
		unitRow{From: Returning, Event: UnitPulse, To: Returning, Do: approachDropOff},
		unitRow{From: Constructing, Event: UnitPulse, To: Idle, Guard: workTargetLost, Do: finishUnitOrder},
		unitRow{From: Constructing, Event: UnitPulse, To: SeekingResource, Guard: finishedFarm, Do: retaskToFarm},
		unitRow{From: Constructing, Event: UnitPulse, To: Idle, Guard: constructionFinished, Do: finishUnitOrder},
		unitRow{From: Constructing, Event: UnitPulse, To: Constructing, Guard: workTargetReached, Do: contributeConstruction},
		unitRow{From: Constructing, Event: UnitPulse, To: Constructing, Do: approachWorkTarget},
		unitRow{From: Repairing, Event: UnitPulse, To: Idle, Guard: workTargetLost, Do: finishUnitOrder},
		unitRow{From: Repairing, Event: UnitPulse, To: Idle, Guard: repairFinished, Do: finishUnitOrder},
		unitRow{From: Repairing, Event: UnitPulse, To: Repairing, Guard: workTargetReached, Do: contributeRepair},
		unitRow{From: Repairing, Event: UnitPulse, To: Repairing, Do: approachWorkTarget},
	)
	for _, state := range []UnitState{Trading, ReturningTrade} {
		rows = append(rows, unitRow{From: state, Event: UnitPulse, To: Idle, Guard: tradeRouteLost, Do: finishUnitOrder})
	}
	return append(rows,
		unitRow{From: Trading, Event: UnitPulse, To: ReturningTrade, Guard: tradeDestinationReached, Do: collectTradeCargo},
		unitRow{From: Trading, Event: UnitPulse, To: Trading, Do: approachTradeDestination},
		unitRow{From: ReturningTrade, Event: UnitPulse, To: Trading, Guard: dropOffReached, Do: deliverTradeCargo},
		unitRow{From: ReturningTrade, Event: UnitPulse, To: ReturningTrade, Do: approachDropOff},
	)
}
func validResource(e, t *Entity) bool {
	return t != nil && t.Amount > 0 && t.Resource != "" && t.life.State() == Active && (t.Owner == 0 || t.Owner == e.Owner) && (definitions[t.Type].Kind == "resource" || t.Type == "farm") && definitions[e.Type].Naval == (t.Type == "fish")
}
func cargoCapacity(c *unitContext) float64 {
	if c.Actor.Type == "fishing_ship" || c.World.Players[c.Actor.Owner].Technologies["wheelbarrow"] {
		return 15
	}
	return 10
}
func needsDropOff(_ context.Context, c *unitContext) error {
	return applicable(c.Actor.Cargo > 0 && (c.Actor.Cargo >= cargoCapacity(c)-1e-6 || !validResource(c.Actor, c.Target) || c.Actor.CargoType != c.Target.Resource))
}
func clearRoute(_ context.Context, c *unitContext) error { c.Actor.Path = nil; return nil }
func canReseed(_ context.Context, c *unitContext) error {
	return applicable(c.Target != nil && c.Target.Type == "farm" && c.Target.Owner == c.Actor.Owner && c.Target.Amount <= 0 && c.Target.life.State() == Exhausted && c.World.Players[c.Actor.Owner].Resources.Wood >= 60)
}
func reseedTarget(_ context.Context, c *unitContext) error {
	mustFire(c.Target.life, ReseedFarm, &entityContext{World: c.World, Actor: c.Target})
	c.Actor.Order = Order{Kind: "build", Target: c.Target.ID}
	c.Actor.Path = nil
	return nil
}
func replacementAvailable(_ context.Context, c *unitContext) error {
	return applicable(!validResource(c.Actor, c.Target) && c.Candidate != nil)
}
func retargetResource(_ context.Context, c *unitContext) error {
	c.Actor.Order.Target = c.Candidate.ID
	c.Actor.Path = nil
	c.World.record(Event{Kind: "retargeted", Message: "Continuing work at another " + definitions[c.Candidate.Type].Name, TargetID: c.Candidate.ID}, c.Actor, 0)
	return nil
}
func resourceLost(_ context.Context, c *unitContext) error {
	return applicable(!validResource(c.Actor, c.Target))
}
func farmOccupied(_ context.Context, c *unitContext) error {
	if c.Target.Type != "farm" {
		return applicable(false)
	}
	for _, e := range c.World.entities(c.Actor.Owner, "villager") {
		if e.ID < c.Actor.ID && e.Order.Target == c.Target.ID && (e.behavior.State() == Gathering || e.behavior.State() == SeekingResource) {
			return nil
		}
	}
	return applicable(false)
}
func resourceReached(_ context.Context, c *unitContext) error {
	return applicable(c.Actor.Position.Distance(c.Target.Position) <= definitions[c.Target.Type].Radius+.7)
}
func beginGathering(_ context.Context, c *unitContext) error {
	c.Actor.CargoType = c.Target.Resource
	c.World.record(Event{Kind: "harvest", Message: fmt.Sprintf("%s #%d started gathering %s", definitions[c.Actor.Type].Name, c.Actor.ID, c.Target.Resource), Resource: c.Target.Resource, TargetID: c.Actor.ID}, c.Target, 0)
	return nil
}
func approachResource(_ context.Context, c *unitContext) error {
	c.World.move(c.Actor, c.Target.Position, definitions[c.Target.Type].Radius+.65, Step)
	return nil
}
func gatheringRate(c *unitContext) float64 {
	p, source := c.World.Players[c.Actor.Owner], c.Target
	rate := .4
	if source.Type == "sheep" {
		rate = .45
	}
	if source.Type == "berries" {
		rate = .32
	}
	if source.Type == "fish" {
		rate = .6
	}
	if p.Civilization == "celts" && source.Resource == "wood" {
		rate *= 1.15
	}
	if p.Civilization == "mongols" && source.Resource == "food" {
		rate *= 1.2
	}
	if p.Civilization == "turks" && source.Resource == "gold" {
		rate *= 1.2
	}
	if p.Technologies["double_bit_axe"] && source.Resource == "wood" {
		rate *= 1.2
	}
	if p.Technologies["gold_mining"] && source.Resource == "gold" {
		rate *= 1.15
	}
	return rate
}
func gatherResource(_ context.Context, c *unitContext) error {
	amount := math.Min(gatheringRate(c)*Step, math.Min(cargoCapacity(c)-c.Actor.Cargo, c.Target.Amount))
	c.Actor.Cargo += amount
	c.Target.Amount -= amount
	if c.Target.Amount <= 1e-8 {
		c.Target.Amount = 0
		mustFire(c.Target.life, ExhaustResource, &entityContext{World: c.World, Actor: c.Target})
	}
	return nil
}
func (w *World) dropOff(e *Entity) *Entity {
	return w.nearest(e.Position, func(b *Entity) bool {
		return b.Owner == e.Owner && b.life.State() == Active && slices.Contains(definitions[b.Type].DropOff, e.CargoType) && (e.Type != "fishing_ship" || b.Type == "dock") && w.reachableFootprint(e, b.Position, definitions[b.Type].Radius+.7)
	})
}
func dropOffReached(_ context.Context, c *unitContext) error {
	return applicable(c.DropOff != nil && c.Actor.Position.Distance(c.DropOff.Position) <= definitions[c.DropOff.Type].Radius+.7)
}
func depositCargo(_ context.Context, c *unitContext) error {
	p, e := c.World.Players[c.Actor.Owner], c.Actor
	p.Resources.Deposit(e.CargoType, e.Cargo)
	p.Gathered.Deposit(e.CargoType, e.Cargo)
	message := fmt.Sprintf("Delivered %.2f %s to %s #%d", e.Cargo, e.CargoType, definitions[c.DropOff.Type].Name, c.DropOff.ID)
	c.World.record(Event{Kind: "delivery", Message: message, Resource: e.CargoType, Amount: e.Cargo, TargetID: c.DropOff.ID}, e, 0)
	c.World.record(Event{Kind: "delivery", Message: fmt.Sprintf("Received %.2f %s from %s #%d", e.Cargo, e.CargoType, definitions[e.Type].Name, e.ID), Resource: e.CargoType, Amount: e.Cargo, TargetID: e.ID}, c.DropOff, 0)
	e.Cargo = 0
	e.Path = nil
	return nil
}
func approachDropOff(_ context.Context, c *unitContext) error {
	c.World.move(c.Actor, c.DropOff.Position, definitions[c.DropOff.Type].Radius+.65, Step)
	return nil
}
func workTargetLost(_ context.Context, c *unitContext) error {
	return applicable(c.Target == nil || c.Target.Owner != c.Actor.Owner || definitions[c.Target.Type].Kind != "building")
}
func finishedFarm(_ context.Context, c *unitContext) error {
	return applicable(c.Target.Type == "farm" && c.Target.life.State() == Active)
}
func constructionFinished(_ context.Context, c *unitContext) error {
	return applicable(c.Target.life.State() == Active)
}
func retaskToFarm(_ context.Context, c *unitContext) error {
	c.Actor.Order = Order{Kind: "gather", Target: c.Target.ID}
	c.Actor.Path = nil
	return nil
}
func workTargetReached(_ context.Context, c *unitContext) error {
	return applicable(c.Actor.Position.Distance(c.Target.Position) <= definitions[c.Target.Type].Radius+.8)
}
func approachWorkTarget(_ context.Context, c *unitContext) error {
	c.World.move(c.Actor, c.Target.Position, definitions[c.Target.Type].Radius+.7, Step)
	return nil
}
func contributeConstruction(_ context.Context, c *unitContext) error {
	builders := 0
	for _, e := range c.World.entities(c.Actor.Owner, "villager") {
		if e.behavior.State() == Constructing && e.Order.Target == c.Target.ID && e.Position.Distance(c.Target.Position) <= definitions[c.Target.Type].Radius+.8 {
			builders++
		}
	}
	work := Step / definitions[c.Target.Type].Time * (.4 + .6/float64(max(1, builders)))
	mustFire(c.Target.life, BuildWork, &entityContext{World: c.World, Actor: c.Target, Amount: work})
	return nil
}
func repairFinished(_ context.Context, c *unitContext) error {
	return applicable(c.Target.HP >= c.World.stats(c.Target).HP)
}
func contributeRepair(_ context.Context, c *unitContext) error {
	err := fire(c.Target.life, RepairWork, &entityContext{World: c.World, Actor: c.Target, Amount: 8 * Step})
	// A resource guard refusal is an expected stalled repair, not an effect failure.
	if errors.Is(err, statemachine.ErrNotPermitted) {
		return nil
	}
	return err
}
func tradeRouteLost(_ context.Context, c *unitContext) error {
	return applicable(c.Target == nil || c.Target.Type != "market" || c.Target.Owner != 0 || c.DropOff == nil)
}
func tradeDestinationReached(_ context.Context, c *unitContext) error {
	return applicable(c.Actor.Position.Distance(c.Target.Position) <= definitions[c.Target.Type].Radius+.8)
}
func collectTradeCargo(_ context.Context, c *unitContext) error {
	c.Actor.Cargo = math.Max(1, c.DropOff.Position.Distance(c.Target.Position)*.7)
	c.Actor.CargoType = "gold"
	c.Actor.Path = nil
	return nil
}
func deliverTradeCargo(_ context.Context, c *unitContext) error {
	c.World.Players[c.Actor.Owner].Resources.Gold += c.Actor.Cargo
	c.World.entityEvent(c.Actor, "trade", fmt.Sprintf("Delivered %.2f trade gold", c.Actor.Cargo), c.Actor.Cargo)
	c.Actor.Cargo = 0
	c.Actor.Path = nil
	return nil
}
func approachTradeDestination(_ context.Context, c *unitContext) error {
	c.World.move(c.Actor, c.Target.Position, definitions[c.Target.Type].Radius+.7, Step)
	return nil
}
