package game

import (
	"math"
	"slices"
	"sort"
)

// Contacts contain only the information available in a player read model.
// In particular they contain no enemy orders, upgrades, stockpile or hidden
// units. Remembered buildings retain their last observed position and health.
type aiContact struct {
	ID, Owner int
	Type      string
	Position  Vec
	HP, MaxHP float64
	Progress  float64
	Visible   bool
}

type aiOpportunity struct {
	ID, Owner int
	Position  Vec
	Goal      Vec
	Score     float64
}

func (w *World) aiObserve(player int) *aiContext {
	p := w.Players[player]
	c := &aiContext{World: w, Player: p, Policy: difficultyPolicy(w.Config.Difficulty), Previous: p.strategy.State(), Home: p.Start,
		Workers: w.entities(player, "villager"), Centers: w.entities(player, "town_center")}
	if len(c.Centers) > 0 {
		c.Home = c.Centers[0].Position
	} else if len(c.Workers) > 0 {
		c.Home = c.Workers[0].Position
	}
	seen := map[int]bool{}
	for _, id := range w.IDs {
		e := w.Entities[id]
		if e == nil {
			continue
		}
		d := definitions[e.Type]
		if e.Owner == player {
			if d.Kind == "unit" && d.Class != "worker" && d.Class != "trader" && !d.Naval && d.Attack > 0 && e.Container == 0 {
				c.Army = append(c.Army, e)
			}
			continue
		}
		if e.Owner == 0 || !w.visibleEntity(player, e) {
			continue
		}
		v := w.entityView(e, player)
		c.Contacts = append(c.Contacts, aiContact{v.ID, v.Owner, v.Type, v.Position, v.HP, v.MaxHP, v.Progress, true})
		seen[id] = true
	}
	// Evaluate an expedition using only the force this difficulty is willing
	// to send. Reserves and old saved armies cannot inflate a small raid's odds.
	c.RaidArmy = append([]*Entity{}, c.Army...)
	sort.SliceStable(c.RaidArmy, func(i, j int) bool {
		a, b := aiOwnPower(w, c.RaidArmy[i]), aiOwnPower(w, c.RaidArmy[j])
		if a != b {
			return a > b
		}
		return c.RaidArmy[i].Position.Distance(c.Home) < c.RaidArmy[j].Position.Distance(c.Home)
	})
	c.RaidArmy = c.RaidArmy[:min(len(c.RaidArmy), c.Policy.RaidLimit)]
	for _, e := range c.RaidArmy {
		c.Power += aiOwnPower(w, e)
	}
	// Map iteration must not decide a target or alter checkpoint determinism.
	ids := make([]int, 0, len(p.Memory))
	for id := range p.Memory {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		v := p.Memory[id]
		if !seen[id] && v.Owner > 0 && v.Owner != player && !w.visible(player, v.Position) {
			c.Contacts = append(c.Contacts, aiContact{v.ID, v.Owner, v.Type, v.Position, v.HP, v.MaxHP, v.Progress, false})
		}
	}
	for i := range c.Contacts {
		t := &c.Contacts[i]
		_, attacked := w.recentAggressor(player, t.ID, t.Owner)
		if !t.Visible || !attacked {
			continue
		}
		threatens := false
		for _, center := range c.Centers {
			threatens = threatens || center.Position.Distance(t.Position) < 17
		}
		for _, worker := range c.Workers {
			threatens = threatens || worker.Position.Distance(t.Position) < 8
		}
		if threatens && (c.Threat == nil || t.Position.Distance(c.Home) < c.Threat.Position.Distance(c.Home)) {
			c.Threat = t
		}
	}
	if c.Threat != nil {
		c.ThreatPower = aiDanger(c.Contacts, c.Threat.Position, 12)
		for _, e := range w.entities(player, "") {
			if e.Position.Distance(c.Threat.Position) < 18 && definitions[e.Type].Class != "worker" && e.Container == 0 {
				c.HomePower += aiOwnPower(w, e)
			}
		}
	}
	plan := p.AIPlan
	for _, e := range c.Army {
		if !slices.Contains(plan.Army, e.ID) {
			continue
		}
		c.CampaignPower += aiOwnPower(w, e)
		support := 0.
		for _, ally := range c.Army {
			if e.Position.Distance(ally.Position) < 12 {
				support += aiOwnPower(w, ally)
			}
		}
		c.CampaignRisk = math.Max(c.CampaignRisk, aiDanger(c.Contacts, e.Position, 9)/math.Max(1, support))
	}
	if plan.TargetID != 0 {
		// Defeat is public. Otherwise, losing sight does not reveal that a
		// building was destroyed; the army has to revisit its last known site.
		c.ObjectiveKnown = !w.visible(player, plan.TargetPosition)
		for _, t := range c.Contacts {
			c.ObjectiveKnown = c.ObjectiveKnown || t.ID == plan.TargetID
		}
		if other := w.Players[plan.TargetOwner]; other == nil || other.lifecycle.State() == PlayerDefeated {
			c.ObjectiveKnown = false
		}
	}
	c.Opportunity = aiChooseOpportunity(c)
	return c
}

func aiPower(d Definition, hp, maximum float64) float64 {
	if d.Attack <= 0 || maximum <= 0 {
		return 0
	}
	strength := math.Sqrt(math.Min(maximum, 160)*d.Attack/math.Max(1, d.Reload)) * (1 + d.Range*.06) * math.Min(1, hp/maximum)
	if d.Class == "worker" {
		strength *= .35
	}
	return strength
}

func aiOwnPower(w *World, e *Entity) float64 {
	if e.life.State() != Active || e.Type == "town_center" && len(e.Passengers) == 0 {
		return 0
	}
	d := w.stats(e)
	return aiPower(d, e.HP, d.HP)
}

func aiContactPower(t aiContact) float64 {
	if t.Progress < 1 {
		return 0
	}
	if t.Type == "town_center" {
		// Occupants are private. Budget a modest garrison risk instead of
		// inspecting hidden passengers or treating an empty TC as an army.
		return 6
	}
	return aiPower(definitions[t.Type], t.HP, t.MaxHP)
}

func aiDanger(contacts []aiContact, position Vec, radius float64) float64 {
	power := 0.
	for _, t := range contacts {
		if position.Distance(t.Position) < radius+definitions[t.Type].Radius {
			power += aiContactPower(t)
		}
	}
	return power
}

func aiChooseOpportunity(c *aiContext) *aiOpportunity {
	var best *aiOpportunity
	for _, t := range c.Contacts {
		if !c.World.aiWantsConflict(c.Player, t.Owner) {
			continue
		}
		if other := c.World.Players[t.Owner]; other == nil || other.lifecycle.State() == PlayerDefeated {
			continue
		}
		defense := aiDanger(c.Contacts, t.Position, 11)
		if !t.Visible {
			defense += 5 // Unobserved reinforcements are a risk, not known facts.
		}
		if c.Power < math.Max(3, defense)*c.Policy.RaidAdvantage {
			continue
		}
		d := definitions[t.Type]
		value := 25.
		if d.Kind == "building" {
			value = 30 + (d.Cost.Food+d.Cost.Wood+d.Cost.Gold+d.Cost.Stone)*.22
		}
		if d.Class == "worker" {
			value = 65 // Denying production is more useful than chasing soldiers.
		}
		if t.Type == "town_center" || t.Type == "wonder" {
			value *= 1.5
		}
		distance := c.Home.Distance(t.Position)
		expectedLoss := defense * math.Min(1, defense/math.Max(1, c.Power))
		score := value/(1+distance/30) - expectedLoss*1.5
		// Resolve exact ties by geography, never the player/creation order.
		if best == nil || score > best.Score || score == best.Score && (t.Position.X < best.Position.X || t.Position.X == best.Position.X && t.Position.Y < best.Position.Y) {
			best = &aiOpportunity{ID: t.ID, Owner: t.Owner, Position: t.Position, Goal: aiAway(c.World, t.Position, c.Home, -(d.Radius + 2)), Score: score}
		}
	}
	return best
}
