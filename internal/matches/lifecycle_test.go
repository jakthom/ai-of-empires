package matches

import (
	"context"
	"errors"
	"testing"
	"time"

	"crowns/internal/game"
	"github.com/open-ships/statemachine"
)

func TestIdleLeaseExpiresAndRejectsStaleCommands(t *testing.T) {
	s := NewService()
	seat, err := s.Create(game.Config{Difficulty: "peaceful"})
	if err != nil {
		t.Fatal(err)
	}
	m, err := s.Authorized(seat.MatchID, seat.Token)
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.Apply(game.Command{ID: "speed", Kind: "speed", Value: 1})
	if err != nil {
		t.Fatal(err)
	}
	m.fireLease(leasePulse, m.lastAccess.Add(29*time.Minute))
	if m.lifecycle.State() != leaseOpen {
		t.Fatal("active lease expired")
	}
	m.fireLease(leasePulse, m.lastAccess.Add(31*time.Minute))
	if m.lifecycle.State() != leaseClosed || len(m.commands) != 0 {
		t.Fatal("expired lease retained its command cache")
	}
	if _, err = m.Apply(game.Command{ID: "stale", Kind: "pause"}); !errors.Is(err, ErrNotFound) {
		t.Fatal("closed match accepted a command")
	}
	_, err = m.lifecycle.Fire(context.Background(), releaseLease, &leaseContext{m, time.Now()})
	if !errors.Is(err, statemachine.ErrNotPermitted) {
		t.Fatal("closed lease reopened or repeated its cleanup")
	}
}
