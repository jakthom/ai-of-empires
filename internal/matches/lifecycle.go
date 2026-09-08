package matches

import (
	"context"
	"errors"
	"time"

	"github.com/open-ships/statemachine"
)

// The server lease has its own lifetime, independent of whether the game is
// running, paused, or finished. Read activity keeps the lease alive.
type leaseState string
type leaseEvent string

const (
	leaseOpen    leaseState = "open"
	leaseClosed  leaseState = "closed"
	leasePulse   leaseEvent = "pulse"
	releaseLease leaseEvent = "release"
)

type leaseContext struct {
	Match *Match
	Now   time.Time
}

var leaseMachine = statemachine.MustCompile([]statemachine.Transition[leaseState, leaseEvent, *leaseContext]{
	{From: leaseOpen, Event: releaseLease, To: leaseClosed, Do: clearLease},
	{From: leaseOpen, Event: leasePulse, To: leaseClosed, Guard: leaseExpired, Do: clearLease},
	{From: leaseOpen, Event: leasePulse, To: leaseOpen},
})

func leaseExpired(_ context.Context, c *leaseContext) error {
	idleLimit := 30 * time.Minute
	if c.Match.db != nil {
		idleLimit = 30 * time.Second
	}
	if c.Now.Sub(c.Match.lastAccess) <= idleLimit {
		return errors.New("lease still active")
	}
	return nil
}

func clearLease(_ context.Context, c *leaseContext) error {
	clear(c.Match.commands)
	c.Match.accumulator = 0
	return nil
}

// Caller holds the match lock. Only scheduler ticks and a validated release
// reach this helper; invalid internal events are programming errors.
func (m *Match) fireLease(event leaseEvent, now time.Time) {
	if _, err := m.lifecycle.Fire(context.Background(), event, &leaseContext{m, now}); err != nil {
		panic(err)
	}
}
