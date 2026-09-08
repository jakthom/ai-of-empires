package game

import (
	"math"
	"slices"
)

// Update advances one fixed simulation step. Lifecycles consume events; this
// scheduler neither assigns their states nor implements transition effects.
func (w *World) Update() {
	if w.match.State() != MatchRunning {
		return
	}
	w.Tick++
	w.Time += Step
	if w.treatyInForce() {
		mustFire(w.peacePeriod, treatyPulse, w)
	}
	w.visibleClock += Step
	if w.visibleClock >= .2 {
		w.refreshVisibility()
		w.visibleClock = 0
	}
	w.aiClock += Step
	if w.aiClock >= 2 {
		w.pulseRelations()
		w.thinkAI()
		w.aiClock = 0
	}
	for _, id := range append([]int{}, w.IDs...) {
		e := w.Entities[id]
		if e == nil || e.life.State() != Active {
			continue
		}
		if e.Type == "monastery" && e.Owner > 0 {
			w.Players[e.Owner].Resources.Gold += e.Amount * .5 * Step
		}
		if e.Type == "wonder" && e.Owner > 0 {
			e.Work += Step
		}
		if e.Type == "monk" {
			e.Faith = math.Min(100, e.Faith+Step*2)
		}
		if e.Type == "berserk" {
			e.HP = math.Min(w.stats(e).HP, e.HP+Step*.3)
		}
		if e.Type == "trebuchet" {
			mustFire(e.siege, SiegePulse, &entityContext{World: w, Actor: e})
		}
		if len(e.Tasks) > 0 {
			mustFire(e.production, ProductionPulse, &entityContext{World: w, Actor: e})
		}
		if definitions[e.Type].Kind != "resource" {
			w.behave(e)
		}
	}
	w.updateProjectiles()
	w.checkVictory()
	if w.Tick%200 == 0 {
		w.IDs = slices.DeleteFunc(w.IDs, func(id int) bool { return w.Entities[id] == nil })
	}
}
func (w *World) nearest(pos Vec, pred func(*Entity) bool) *Entity {
	var best *Entity
	dist := math.Inf(1)
	for _, id := range w.IDs {
		e := w.Entities[id]
		if e == nil || e.Container != 0 || !pred(e) {
			continue
		}
		d := e.Position.Distance(pos)
		if d < dist {
			best = e
			dist = d
		}
	}
	return best
}
