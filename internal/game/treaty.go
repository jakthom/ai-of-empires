package game

import (
	"context"
	"math"

	"github.com/open-ships/statemachine"
)

type treatyState string
type treatyEvent string

const (
	treatyActive  treatyState = "active"
	treatyExpired treatyState = "expired"
	treatyPulse   treatyEvent = "pulse"
)

var treatyMachine = statemachine.MustCompile([]statemachine.Transition[treatyState, treatyEvent, *World]{
	{From: treatyActive, Event: treatyPulse, To: treatyExpired, Guard: treatyDue, Do: announceTreatyEnd},
	{From: treatyActive, Event: treatyPulse, To: treatyActive},
	{From: treatyExpired, Event: treatyPulse, To: treatyExpired},
})

func treatyDue(_ context.Context, w *World) error {
	return applicable(w.Time >= float64(w.Config.World.TreatyMinutes)*60)
}
func announceTreatyEnd(_ context.Context, w *World) error {
	for id := 1; id <= w.Config.Settlements; id++ {
		w.event(id, "The initial peace period has ended. Kingdoms may now choose to attack; existing peace continues until aggression occurs.")
	}
	return nil
}
func (w *World) initializeTreaty() {
	state := treatyExpired
	if w.Config.World.TreatyMinutes > 0 {
		state = treatyActive
	}
	w.peacePeriod = statemachine.NewInstance(treatyMachine, state)
}
func (w *World) treatyInForce() bool {
	return w.peacePeriod != nil && w.peacePeriod.State() == treatyActive
}
func (w *World) TreatyRemaining() float64 {
	if !w.treatyInForce() {
		return 0
	}
	return math.Max(0, float64(w.Config.World.TreatyMinutes)*60-w.Time)
}
