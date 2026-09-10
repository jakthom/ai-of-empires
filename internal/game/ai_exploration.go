package game

import "math"

// One mobile unit surveys unexplored areas while the settlement develops.
// Goals use map bounds and this player's fog, never another player's Start.
// Failed destinations expire so an impassable patch cannot monopolize a scout.
func (w *World) aiScout(c *aiContext) {
	plan := &c.Player.AIPlan
	var scout *Entity
	for _, e := range c.Army {
		if e.Order.Kind == "guard" || c.Player.voyaging(e.ID) || !w.sameRegion(e.Position, c.Home, false) {
			continue
		}
		if e.ID == plan.ScoutID {
			scout = e
			break
		}
		if scout == nil || definitions[e.Type].Speed > definitions[scout.Type].Speed {
			scout = e
		}
	}
	if scout == nil {
		plan.ScoutID, plan.ScoutGoal = 0, nil
		return
	}
	if plan.ScoutID != scout.ID {
		plan.ScoutID, plan.ScoutGoal = scout.ID, nil
	}
	if plan.ScoutGoal != nil && w.Time < plan.ScoutDeadline && scout.Position.Distance(*plan.ScoutGoal) > 3 {
		aiMarch(c, []*Entity{scout}, *plan.ScoutGoal, false, false)
		return
	}
	plan.ScoutGoal = nil
	center := Vec{float64(w.Width) / 2, float64(w.Height) / 2}
	best := math.Inf(1)
	for y := 7; y < w.Height-5; y += 12 {
		for x := 7; x < w.Width-5; x += 12 {
			i := y*w.Width + x
			goal := Vec{float64(x) + .5, float64(y) + .5}
			if !w.sameRegion(scout.Position, goal, false) || plan.Surveyed[i] > w.Time || c.Player.Visible[i] || c.Player.Explored[i] && !w.land(goal) {
				continue
			}
			cost := scout.Position.Distance(goal) + center.Distance(goal)*.4
			if c.Player.Explored[i] {
				cost += 45 // Once explored, periodically revisit to update memory.
			}
			cost += aiDanger(c.Contacts, goal, 10) * 2
			if cost < best {
				best, plan.ScoutGoal = cost, &goal
			}
		}
	}
	if plan.ScoutGoal != nil {
		if plan.Surveyed == nil {
			plan.Surveyed = map[int]float64{}
		}
		goal := *plan.ScoutGoal
		plan.Surveyed[int(goal.Y)*w.Width+int(goal.X)] = w.Time + 180
		plan.ScoutDeadline = w.Time + math.Max(20, scout.Position.Distance(goal)/definitions[scout.Type].Speed*2+10)
		aiMarch(c, []*Entity{scout}, goal, false, true)
	}
}

// Safe, explored deposits can support additional Town Centers in Castle Age.
// Expansion spends the same resources and uses the same placement/builder
// lifecycles as a human. It does not claim unseen resources across the map.
func (w *World) aiExpand(c *aiContext) {
	p := c.Player
	if p.Age < 2 || len(c.Centers) >= c.Policy.TownCenters || len(c.Workers) < c.Policy.Workers || w.canBuild(p, definitions["town_center"]) != nil {
		return
	}
	var best *Entity
	bestScore := 0.
	for _, id := range w.IDs {
		e := w.Entities[id]
		if e == nil || e.Owner != 0 || e.Amount <= 100 || e.Resource == "" || e.Type == "fish" || !w.sameRegion(c.Home, e.Position, false) || !w.visibleEntity(p.ID, e) {
			continue
		}
		distance := math.Inf(1)
		for _, center := range c.Centers {
			distance = math.Min(distance, center.Position.Distance(e.Position))
		}
		if distance < 20 || distance > 42 || aiDanger(c.Contacts, e.Position, 18) > 3 {
			continue
		}
		score := math.Min(e.Amount, 800) / distance
		if score > bestScore {
			best, bestScore = e, score
		}
	}
	if best != nil {
		w.aiBuild(p.ID, "town_center", best.Position, c.Workers)
	}
}
