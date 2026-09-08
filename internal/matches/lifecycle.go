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
	leaseOpen     leaseState = "open"
	leaseDraining leaseState = "draining"
	leaseClosed   leaseState = "closed"
	leasePulse    leaseEvent = "pulse"
	freezeLease   leaseEvent = "freeze"
	releaseLease  leaseEvent = "release"
)

type leaseContext struct {
	Match *Match
	Now   time.Time
}

var leaseMachine = statemachine.MustCompile([]statemachine.Transition[leaseState, leaseEvent, *leaseContext]{
	{From: leaseOpen, Event: freezeLease, To: leaseDraining},
	{From: leaseDraining, Event: releaseLease, To: leaseClosed, Do: clearLease},
	{From: leaseOpen, Event: releaseLease, To: leaseClosed, Do: clearLease},
	{From: leaseOpen, Event: leasePulse, To: leaseClosed, Guard: leaseExpired, Do: clearLease},
	{From: leaseOpen, Event: leasePulse, To: leaseOpen},
})

// The service owns admission and shutdown. Draining freezes each lease before
// checkpointing; it must preserve command receipts and the fractional clock.
type serviceState string
type serviceEvent string

const (
	serviceServing  serviceState = "serving"
	serviceDraining serviceState = "draining"
	serviceClosed   serviceState = "closed"
	beginShutdown   serviceEvent = "begin_shutdown"
	finishShutdown  serviceEvent = "finish_shutdown"
)

var serviceMachine = statemachine.MustCompile([]statemachine.Transition[serviceState, serviceEvent, *Service]{
	{From: serviceServing, Event: beginShutdown, To: serviceDraining, Do: freezeMatches},
	{From: serviceDraining, Event: beginShutdown, To: serviceDraining},
	{From: serviceClosed, Event: beginShutdown, To: serviceClosed},
	{From: serviceDraining, Event: finishShutdown, To: serviceClosed, Do: releaseMatches},
})

func freezeMatches(_ context.Context, s *Service) error {
	close(s.stopping)
	for _, m := range s.matches {
		m.mu.Lock()
		m.fireLease(freezeLease, time.Now())
		m.mu.Unlock()
	}
	return nil
}

func releaseMatches(_ context.Context, s *Service) error {
	for _, m := range s.matches {
		m.mu.Lock()
		m.fireLease(releaseLease, time.Now())
		m.mu.Unlock()
	}
	clear(s.matches)
	return nil
}

// Caller holds the service lock. Persistence runs between these transitions,
// outside the state machine, while all game mutations are frozen.
func (s *Service) fireService(event serviceEvent) {
	if _, err := s.lifecycle.Fire(context.Background(), event, s); err != nil {
		panic(err)
	}
}

// Caller holds the match lock. Handles already authorized before the shutdown
// barrier must refuse commands too; checking admission alone is insufficient.
func (m *Match) available() error {
	switch m.lifecycle.State() {
	case leaseDraining:
		return ErrShuttingDown
	case leaseClosed:
		return ErrNotFound
	default:
		return nil
	}
}

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
