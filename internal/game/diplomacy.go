package game

import (
	"context"
	"fmt"

	"github.com/open-ships/statemachine"
)

type relationState string
type relationEvent string

const (
	atPeace            relationState = "peaceful"
	inConflict         relationState = "hostile"
	aggressionObserved relationEvent = "aggression"
	relationPulse      relationEvent = "pulse"
	peaceAfter                       = 300.0
	retaliationMemory                = 15.0
	defenseLeash                     = 6.0
)

type relationship struct {
	A, B       int
	QuietUntil float64
	lifecycle  *statemachine.Instance[relationState, relationEvent, *relationContext]
}

type relationContext struct {
	World    *World
	Relation *relationship
}

// Incidents are recent observed facts, like fog-of-war memory. A record names
// the actual aggressor and the friendly position it attacked; it is not a
// blanket permission to attack every member of that kingdom.
type aggression struct {
	Owner, Victim int
	Position      Vec
	Time          float64
}

var relationMachine = statemachine.MustCompile([]statemachine.Transition[relationState, relationEvent, *relationContext]{
	{From: atPeace, Event: aggressionObserved, To: inConflict, Do: beginConflict},
	{From: inConflict, Event: aggressionObserved, To: inConflict, Do: extendConflict},
	{From: inConflict, Event: relationPulse, To: atPeace, Guard: conflictQuiet, Do: restorePeace},
	{From: inConflict, Event: relationPulse, To: inConflict},
	{From: atPeace, Event: relationPulse, To: atPeace},
})

func relationKey(a, b int) string {
	if a > b {
		a, b = b, a
	}
	return fmt.Sprintf("%d:%d", a, b)
}

func (w *World) initializeRelations() {
	w.Relations = map[string]*relationship{}
	w.Incidents = map[int]map[int]aggression{}
	for a := 1; a <= w.Config.Settlements; a++ {
		for b := a + 1; b <= w.Config.Settlements; b++ {
			w.Relations[relationKey(a, b)] = &relationship{A: a, B: b, lifecycle: statemachine.NewInstance(relationMachine, atPeace)}
		}
	}
}

func (w *World) relation(a, b int) relationState {
	if r := w.Relations[relationKey(a, b)]; r != nil {
		return r.lifecycle.State()
	}
	return atPeace
}

func conflictQuiet(_ context.Context, c *relationContext) error {
	return applicable(c.World.Time >= c.Relation.QuietUntil)
}
func extendConflict(_ context.Context, c *relationContext) error {
	c.Relation.QuietUntil = c.World.Time + peaceAfter
	return nil
}
func beginConflict(ctx context.Context, c *relationContext) error {
	_ = extendConflict(ctx, c)
	w, r := c.World, c.Relation
	w.event(r.A, "Conflict has begun with "+w.Players[r.B].Name+".")
	w.event(r.B, "Conflict has begun with "+w.Players[r.A].Name+".")
	return nil
}
func restorePeace(_ context.Context, c *relationContext) error {
	w, r := c.World, c.Relation
	w.event(r.A, "Peace has returned with "+w.Players[r.B].Name+" after five game minutes without attacks.")
	w.event(r.B, "Peace has returned with "+w.Players[r.A].Name+" after five game minutes without attacks.")
	return nil
}

func (w *World) noteAggression(source, owner int, target *Entity) {
	if target == nil || owner == 0 || target.Owner == 0 || owner == target.Owner {
		return
	}
	if r := w.Relations[relationKey(owner, target.Owner)]; r != nil {
		mustFire(r.lifecycle, aggressionObserved, &relationContext{World: w, Relation: r})
	}
	if source != 0 {
		if w.Incidents[target.Owner] == nil {
			w.Incidents[target.Owner] = map[int]aggression{}
		}
		w.Incidents[target.Owner][source] = aggression{Owner: owner, Victim: target.ID, Position: target.Position, Time: w.Time}
	}
}

func (w *World) recentAggressor(player, source, owner int) (aggression, bool) {
	incident, ok := w.Incidents[player][source]
	return incident, ok && incident.Owner == owner && w.Time-incident.Time <= retaliationMemory
}

func (w *World) pulseRelations() {
	for a := 1; a <= w.Config.Settlements; a++ {
		for b := a + 1; b <= w.Config.Settlements; b++ {
			r := w.Relations[relationKey(a, b)]
			mustFire(r.lifecycle, relationPulse, &relationContext{World: w, Relation: r})
		}
	}
	for _, incidents := range w.Incidents {
		for id, incident := range incidents {
			if w.Time-incident.Time > retaliationMemory {
				delete(incidents, id)
			}
		}
	}
}
