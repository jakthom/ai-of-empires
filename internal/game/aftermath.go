package game

import (
	"context"
	"fmt"
	"math"

	"github.com/open-ships/statemachine"
)

type aftermathState string
type aftermathEvent string

const (
	aftermathPresent aftermathState = "present"
	aftermathExpired aftermathState = "expired"
	aftermathPulse   aftermathEvent = "pulse"
)

// Only Go confirms destruction. A model disappearing into fog never implies
// death. These short-lived, non-colliding remains obey the game clock and fog.
type BattlefieldEffectView struct {
	ID            int     `json:"id"`
	Kind          string  `json:"kind"`
	Type          string  `json:"type"`
	Owner         int     `json:"owner"`
	Position      Vec     `json:"position"`
	Radius        float64 `json:"radius"`
	AppearanceAge int     `json:"appearance_age"`
	StartedAt     float64 `json:"started_at"`
	Duration      float64 `json:"duration"`
}
type aftermath struct {
	View      BattlefieldEffectView
	lifecycle *statemachine.Instance[aftermathState, aftermathEvent, *aftermathContext]
}
type aftermathContext struct {
	World  *World
	Effect *aftermath
}

var aftermathMachine = statemachine.MustCompile([]statemachine.Transition[aftermathState, aftermathEvent, *aftermathContext]{
	{From: aftermathPresent, Event: aftermathPulse, To: aftermathExpired, Guard: func(_ context.Context, c *aftermathContext) error {
		return applicable(c.World.Time >= c.Effect.View.StartedAt+c.Effect.View.Duration)
	}},
	{From: aftermathPresent, Event: aftermathPulse, To: aftermathPresent},
})

func (w *World) leaveAftermath(e *Entity) {
	d := definitions[e.Type]
	if e.Container != 0 || d.Kind != "unit" && d.Kind != "building" {
		return
	}
	duration, age := 6.0, 0
	if d.Kind == "building" {
		duration = 12
	}
	if p := w.Players[e.Owner]; p != nil {
		age = p.Age
	}
	w.Aftermath = append(w.Aftermath, aftermath{View: BattlefieldEffectView{ID: w.NextID, Kind: d.Kind, Type: e.Type, Owner: e.Owner, Position: e.Position, Radius: d.Radius, AppearanceAge: age, StartedAt: w.Time, Duration: duration}, lifecycle: statemachine.NewInstance(aftermathMachine, aftermathPresent)})
	w.NextID++
}
func (w *World) pulseAftermath() {
	alive := w.Aftermath[:0]
	for i := range w.Aftermath {
		e := w.Aftermath[i]
		mustFire(e.lifecycle, aftermathPulse, &aftermathContext{World: w, Effect: &e})
		if e.lifecycle.State() == aftermathPresent {
			alive = append(alive, e)
		}
	}
	w.Aftermath = alive
}
func (w *World) restoreAftermath(c checkpoint) error {
	if len(c.Aftermath) != len(w.Aftermath) {
		return fmt.Errorf("invalid checkpoint aftermath")
	}
	seen := map[int]bool{}
	for i := range w.Aftermath {
		e := &w.Aftermath[i]
		v := e.View
		d := definitions[v.Type]
		if c.Aftermath[i] != aftermathPresent || seen[v.ID] || w.Entities[v.ID] != nil || v.ID <= 0 || v.ID >= w.NextID || !v.Position.Finite() || !w.inside(v.Position) || d.ID == "" || v.Kind != d.Kind || v.Kind != "unit" && v.Kind != "building" || v.Owner < 0 || v.Owner > w.Config.Settlements || v.Radius != d.Radius || v.AppearanceAge < 0 || v.AppearanceAge > 3 || math.IsNaN(v.StartedAt) || math.IsInf(v.StartedAt, 0) || v.StartedAt < 0 || v.StartedAt > w.Time || v.Kind == "unit" && v.Duration != 6 || v.Kind == "building" && v.Duration != 12 {
			return fmt.Errorf("invalid checkpoint remains %d", v.ID)
		}
		seen[v.ID] = true
		e.lifecycle = statemachine.NewInstance(aftermathMachine, aftermathPresent)
	}
	return nil
}
func (w *World) damageStage(e *Entity) int {
	maximum := w.stats(e).HP
	if e.life.State() == Foundation {
		maximum = math.Max(1, maximum*e.Progress)
	}
	ratio := e.HP / maximum
	if ratio < .25 {
		return 3
	}
	if ratio < .5 {
		return 2
	}
	if ratio < .85 {
		return 1
	}
	return 0
}
