package game

import (
	"context"
	"math"
	"slices"

	"github.com/open-ships/statemachine"
)

type voyageState string
type voyageEvent string

const (
	voyageIdle      voyageState = "idle"
	voyageBoarding  voyageState = "boarding"
	voyageSailing   voyageState = "sailing"
	voyageLanding   voyageState = "landing"
	voyageReturning voyageState = "returning"
	voyageAssess    voyageEvent = "assess"
)

var voyageStates = []voyageState{voyageIdle, voyageBoarding, voyageSailing, voyageLanding, voyageReturning}

type navalPlan struct {
	ShipID                         int
	Crew                           []int
	HomeWater, GoalWater, GoalLand Vec
	TargetOwner                    int
	Deadline, NextAt               float64
}
type voyageContext struct {
	AI        *aiContext
	Ship      *Entity
	Candidate *navalPlan
	Previous  voyageState
}
type voyageRow = statemachine.Transition[voyageState, voyageEvent, *voyageContext]

var voyageMachine = makeVoyageMachine()

func makeVoyageMachine() *statemachine.Machine[voyageState, voyageEvent, *voyageContext] {
	rows := []voyageRow{}
	for _, state := range voyageStates[1:] {
		rows = append(rows, voyageRow{From: state, Event: voyageAssess, To: voyageIdle, Guard: voyageShipLost, Do: finishVoyage})
	}
	for _, state := range []voyageState{voyageBoarding, voyageSailing} {
		rows = append(rows, voyageRow{From: state, Event: voyageAssess, To: voyageReturning, Guard: voyageAbandoned, Do: returnVoyage})
	}
	rows = append(rows,
		voyageRow{From: voyageIdle, Event: voyageAssess, To: voyageBoarding, Guard: voyageAvailable, Do: beginVoyage},
		voyageRow{From: voyageIdle, Event: voyageAssess, To: voyageIdle},
		voyageRow{From: voyageBoarding, Event: voyageAssess, To: voyageSailing, Guard: voyageLoaded, Do: sailVoyage},
		voyageRow{From: voyageBoarding, Event: voyageAssess, To: voyageBoarding, Do: boardVoyage},
		voyageRow{From: voyageSailing, Event: voyageAssess, To: voyageLanding, Guard: voyageArrived, Do: landVoyage},
		voyageRow{From: voyageSailing, Event: voyageAssess, To: voyageSailing, Do: sailVoyage},
		voyageRow{From: voyageLanding, Event: voyageAssess, To: voyageReturning, Guard: voyageUnloaded, Do: dispatchLanding},
		voyageRow{From: voyageLanding, Event: voyageAssess, To: voyageReturning, Guard: voyageTimedOut, Do: returnVoyage},
		voyageRow{From: voyageLanding, Event: voyageAssess, To: voyageLanding, Do: landVoyage},
		voyageRow{From: voyageReturning, Event: voyageAssess, To: voyageIdle, Guard: voyageHome, Do: finishVoyage},
		voyageRow{From: voyageReturning, Event: voyageAssess, To: voyageReturning, Do: returnVoyage},
	)
	return statemachine.MustCompile(rows)
}
func voyageShipLost(_ context.Context, c *voyageContext) error {
	return applicable(c.Ship == nil || c.Ship.Owner != c.AI.Player.ID)
}
func voyageAvailable(_ context.Context, c *voyageContext) error {
	return applicable(c.Candidate != nil)
}
func voyageTimedOut(_ context.Context, c *voyageContext) error {
	return applicable(c.AI.World.Time >= c.AI.Player.NavalPlan.Deadline)
}
func voyageAbandoned(_ context.Context, c *voyageContext) error {
	p := c.AI.Player.NavalPlan
	return applicable(c.AI.World.Time >= p.Deadline || c.AI.Threat != nil || p.TargetOwner > 0 && !c.AI.World.aiWantsConflict(c.AI.Player, p.TargetOwner))
}
func voyageLoaded(_ context.Context, c *voyageContext) error {
	return applicable(len(c.Ship.Passengers) >= len(c.AI.Player.NavalPlan.Crew) && len(c.Ship.Passengers) > 0)
}
func voyageArrived(_ context.Context, c *voyageContext) error {
	return applicable(c.Ship.Position.Distance(c.AI.Player.NavalPlan.GoalWater) < 1)
}
func voyageUnloaded(_ context.Context, c *voyageContext) error {
	return applicable(len(c.Ship.Passengers) == 0)
}
func voyageHome(_ context.Context, c *voyageContext) error {
	return applicable(c.Ship.Position.Distance(c.AI.Player.NavalPlan.HomeWater) < 1 && len(c.Ship.Passengers) == 0)
}

func beginVoyage(ctx context.Context, c *voyageContext) error {
	p := c.AI.Player
	p.NavalPlan = *c.Candidate
	c.Ship = c.AI.World.Entities[p.NavalPlan.ShipID]
	p.NavalPlan.Deadline = c.AI.World.Time + 120
	c.AI.World.event(p.ID, "A transport expedition is gathering at the shore.")
	return boardVoyage(ctx, c)
}
func voyageMove(c *voyageContext, goal Vec) {
	if c.Ship.behavior.State() == Idle && c.Ship.Position.Distance(goal) > .5 || c.Ship.Order.Position == nil || c.Ship.Order.Position.Distance(goal) > 1 {
		_ = c.AI.World.Apply(c.AI.Player.ID, Command{Kind: "move", EntityIDs: []int{c.Ship.ID}, Position: &goal})
	}
}
func boardVoyage(_ context.Context, c *voyageContext) error {
	w, p := c.AI.World, c.AI.Player
	voyageMove(c, p.NavalPlan.HomeWater)
	if c.Ship.Position.Distance(p.NavalPlan.HomeWater) > 1 {
		return nil
	}
	for _, id := range p.NavalPlan.Crew {
		e := w.Entities[id]
		if e != nil && e.Owner == p.ID && e.Container == 0 && (e.Order.Kind != "garrison" || e.Order.Target != c.Ship.ID) {
			_ = w.Apply(p.ID, Command{Kind: "garrison", EntityIDs: []int{id}, TargetID: c.Ship.ID})
		}
	}
	return nil
}
func sailVoyage(_ context.Context, c *voyageContext) error {
	p := &c.AI.Player.NavalPlan
	if c.Previous == voyageBoarding {
		p.Deadline = c.AI.World.Time + math.Max(120, c.Ship.Position.Distance(p.GoalWater)*3)
	}
	voyageMove(c, p.GoalWater)
	return nil
}
func landVoyage(_ context.Context, c *voyageContext) error {
	_ = c.AI.World.Apply(c.AI.Player.ID, Command{Kind: "unload", EntityIDs: []int{c.Ship.ID}})
	return nil
}
func dispatchLanding(ctx context.Context, c *voyageContext) error {
	w, p := c.AI.World, c.AI.Player
	crew := []*Entity{}
	for _, id := range p.NavalPlan.Crew {
		if e := w.Entities[id]; e != nil && e.Owner == p.ID && e.Container == 0 {
			crew = append(crew, e)
		}
	}
	if p.NavalPlan.TargetOwner > 0 {
		if w.aiWantsConflict(p, p.NavalPlan.TargetOwner) {
			_ = w.Apply(p.ID, Command{Kind: "attack_move", EntityIDs: append([]int{}, p.NavalPlan.Crew...), Position: &p.NavalPlan.GoalLand, TargetPlayer: p.NavalPlan.TargetOwner})
		}
	} else {
		w.aiBuild(p.ID, "town_center", p.NavalPlan.GoalLand, crew)
	}
	w.event(p.ID, "The transport expedition has landed.")
	// Landed soldiers stay on that island, with return-fire orders after their
	// bounded raid. The normal land planner must not recall them across water.
	p.AIPlan.NextRaidAt = math.Max(p.AIPlan.NextRaidAt, w.Time+c.AI.Policy.RaidInterval)
	return returnVoyage(ctx, c)
}
func returnVoyage(_ context.Context, c *voyageContext) error {
	p := c.AI.Player
	for _, id := range p.NavalPlan.Crew {
		if e := c.AI.World.Entities[id]; e != nil && e.Container == 0 && e.Order.Kind == "garrison" {
			_ = c.AI.World.Apply(p.ID, Command{Kind: "stop", EntityIDs: []int{id}})
		}
	}
	if c.Ship.Position.Distance(p.NavalPlan.HomeWater) < 1 {
		if len(c.Ship.Passengers) > 0 {
			_ = c.AI.World.Apply(p.ID, Command{Kind: "unload", EntityIDs: []int{c.Ship.ID}})
		}
	} else {
		voyageMove(c, p.NavalPlan.HomeWater)
	}
	return nil
}
func finishVoyage(_ context.Context, c *voyageContext) error {
	p := c.AI.Player
	for _, id := range p.NavalPlan.Crew {
		if e := c.AI.World.Entities[id]; e != nil && e.Owner == p.ID && e.Container == 0 && e.Order.Kind == "garrison" {
			_ = c.AI.World.Apply(p.ID, Command{Kind: "stop", EntityIDs: []int{id}})
		}
	}
	p.NavalPlan = navalPlan{NextAt: c.AI.World.Time + math.Max(90, c.AI.Policy.RaidInterval)}
	return nil
}

func (w *World) initializeVoyages() {
	for _, p := range w.Players {
		p.voyage = statemachine.NewInstance(voyageMachine, voyageIdle)
	}
}
func (w *World) aiNavy(c *aiContext) {
	p := c.Player
	// Docks and vessels are paid for through the public command boundary.
	if len(c.Workers) >= 5 && !w.hasOrBuilding(p.ID, "dock") && p.Resources.Wood >= 200 {
		w.aiBuild(p.ID, "dock", c.Home, c.Workers)
	}
	for _, dock := range w.entities(p.ID, "dock") {
		if dock.life.State() != Active || len(dock.Tasks) > 0 {
			continue
		}
		product := ""
		if len(w.entities(p.ID, "fishing_ship")) < 2 {
			product = "fishing_ship"
		} else if p.Age >= 1 && len(w.entities(p.ID, "transport")) == 0 && w.islandWorld() {
			product = "transport"
		}
		if product != "" && w.canTrain(p, dock, definitions[product]) == nil {
			_ = w.Apply(p.ID, Command{Kind: "train", EntityIDs: []int{dock.ID}, Product: product})
		}
	}
	for i, ship := range w.entities(p.ID, "fishing_ship") {
		if ship.behavior.State() != Idle {
			continue
		}
		fish := w.nearest(ship.Position, func(e *Entity) bool {
			return e.Type == "fish" && e.Amount > 0 && w.visibleEntity(p.ID, e) && w.sameRegion(ship.Position, e.Position, true)
		})
		if fish != nil && (i > 0 || !w.islandWorld()) {
			_ = w.Apply(p.ID, Command{Kind: "gather", EntityIDs: []int{ship.ID}, TargetID: fish.ID})
			continue
		}
		// One fishing boat surveys connected water before settling to work.
		goal := w.navalScoutGoal(p, ship)
		if goal != nil {
			_ = w.Apply(p.ID, Command{Kind: "move", EntityIDs: []int{ship.ID}, Position: goal})
		} else if fish != nil {
			_ = w.Apply(p.ID, Command{Kind: "gather", EntityIDs: []int{ship.ID}, TargetID: fish.ID})
		}
	}
	vc := &voyageContext{AI: c, Ship: w.Entities[p.NavalPlan.ShipID], Previous: p.voyage.State()}
	if p.voyage.State() == voyageIdle && w.Time >= p.NavalPlan.NextAt {
		vc.Candidate = w.planVoyage(c)
	}
	mustFire(p.voyage, voyageAssess, vc)
}

func (w *World) navalScoutGoal(p *Player, ship *Entity) *Vec {
	var best *Vec
	score := math.Inf(1)
	for y := 4; y < w.Height-3; y += 5 {
		for x := 4; x < w.Width-3; x += 5 {
			goal := Vec{float64(x) + .5, float64(y) + .5}
			if p.Explored[y*w.Width+x] || !w.sameRegion(ship.Position, goal, true) {
				continue
			}
			if d := ship.Position.Distance(goal); d < score {
				score = d
				g := goal
				best = &g
			}
		}
	}
	return best
}

// Select only an explored shore on the requested land and sea components.
func (w *World) knownShore(p *Player, land, sea int, near Vec) (Vec, Vec, bool) {
	best := math.Inf(1)
	var water, ground Vec
	for i, t := range w.Tiles {
		if !p.Explored[i] || t.Terrain != "water" || w.waterRegions[i] != sea {
			continue
		}
		a := Vec{float64(i%w.Width) + .5, float64(i/w.Width) + .5}
		if a.Distance(near) >= best {
			continue
		}
		for _, d := range []Vec{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			b := Vec{a.X + d.X, a.Y + d.Y}
			if w.region(b, false) == land && w.inside(b) && p.Explored[int(b.Y)*w.Width+int(b.X)] && w.free(a, .5, 0, true) && w.free(b, .45, 0, false) {
				best = a.Distance(near)
				water, ground = a, b
				break
			}
		}
	}
	return water, ground, !math.IsInf(best, 1)
}
func (w *World) planVoyage(c *aiContext) *navalPlan {
	p := c.Player
	ships := w.entities(p.ID, "transport")
	if len(ships) == 0 || c.Threat != nil {
		return nil
	}
	ship := ships[0]
	home := w.region(c.Home, false)
	sea := w.region(ship.Position, true)
	homeWater, _, ok := w.knownShore(p, home, sea, c.Home)
	if !ok {
		return nil
	}
	plan := &navalPlan{ShipID: ship.ID, HomeWater: homeWater}
	// Expansionists still obey the same raid timing, size and risk limits.
	if w.Time >= c.Policy.RaidAfter && w.Time >= p.AIPlan.NextRaidAt && len(c.RaidArmy) >= c.Policy.RaidSize {
		for _, contact := range c.Contacts {
			region := w.region(contact.Position, false)
			if region == 0 || region == home || !w.aiWantsConflict(p, contact.Owner) || c.Power < math.Max(3, aiDanger(c.Contacts, contact.Position, 12))*c.Policy.RaidAdvantage {
				continue
			}
			water, _, found := w.knownShore(p, region, sea, contact.Position)
			if !found {
				continue
			}
			plan.GoalWater, plan.GoalLand, plan.TargetOwner = water, contact.Position, contact.Owner
			for _, e := range c.RaidArmy {
				if len(plan.Crew) >= min(c.Policy.RaidLimit, w.garrisonCapacity(ship)) {
					break
				}
				if w.sameRegion(e.Position, c.Home, false) {
					plan.Crew = append(plan.Crew, e.ID)
				}
			}
			if len(plan.Crew) >= c.Policy.RaidSize {
				return plan
			}
		}
	}
	if p.Age < 2 || len(c.Centers) >= c.Policy.TownCenters || len(c.Workers) < 8 || !p.Resources.CanPay(w.cost(p, definitions["town_center"])) {
		return nil
	}
	visited := map[int]bool{home: true}
	for i, t := range w.Tiles {
		region := w.landRegions[i]
		if !p.Explored[i] || t.Terrain != "grass" || region == 0 || visited[region] {
			continue
		}
		visited[region] = true
		near := Vec{float64(i%w.Width) + .5, float64(i/w.Width) + .5}
		water, land, found := w.knownShore(p, region, sea, near)
		if !found || aiDanger(c.Contacts, land, 22) > 0 {
			continue
		}
		occupied := false
		for _, contact := range c.Contacts {
			if w.region(contact.Position, false) == region {
				occupied = true
				break
			}
		}
		for _, center := range c.Centers {
			if w.region(center.Position, false) == region {
				occupied = true
			}
		}
		if occupied {
			continue
		}
		plan.GoalWater, plan.GoalLand = water, land
		for _, e := range c.Workers {
			if e.Container == 0 && e.behavior.State() != Constructing && w.sameRegion(e.Position, c.Home, false) {
				plan.Crew = append(plan.Crew, e.ID)
				if len(plan.Crew) == 2 {
					return plan
				}
			}
		}
	}
	return nil
}

func (p *Player) voyaging(id int) bool {
	return p.voyage != nil && p.voyage.State() != voyageIdle && slices.Contains(p.NavalPlan.Crew, id)
}
