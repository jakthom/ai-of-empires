package matches

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"crowns/internal/game"
	"github.com/open-ships/statemachine"
)

func TestShutdownBarrierPreservesStateAndClosesExactlyOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.sqlite")
	s, err := OpenService(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	seat, err := s.Create(game.Config{Name: "Shutdown", Difficulty: "peaceful"})
	if err != nil {
		t.Fatal(err)
	}
	m, _ := s.Authorized(seat.MatchID, seat.Token)
	cmd := game.Command{ID: "pause-once", Kind: "pause"}
	receipt, err := m.Apply(cmd)
	if err != nil {
		t.Fatal(err)
	}
	m.accumulator = game.Step / 2
	view := m.View()
	if _, err := s.lifecycle.Fire(context.Background(), finishShutdown, s); !errors.Is(err, statemachine.ErrNotPermitted) {
		t.Fatalf("closed before the mutation barrier: %v", err)
	}
	if _, err := s.db.Exec(`CREATE TABLE close_saves (id TEXT);
	 CREATE TRIGGER count_close_saves AFTER UPDATE ON sessions BEGIN INSERT INTO close_saves VALUES(NEW.id); END;`); err != nil {
		t.Fatal(err)
	}

	for range 2 {
		s.BeginShutdown()
	}
	select {
	case <-s.Stopping():
	default:
		t.Fatal("shutdown did not notify the transport")
	}
	if s.lifecycle.State() != serviceDraining || m.lifecycle.State() != leaseDraining || len(m.commands) != 1 || m.accumulator != game.Step/2 {
		t.Fatal("draining discarded receipts or the fractional clock")
	}
	if _, err := m.lifecycle.Fire(context.Background(), leasePulse, &leaseContext{m, time.Now().Add(time.Hour)}); !errors.Is(err, statemachine.ErrNotPermitted) {
		t.Fatal("a frozen lease accepted a simulation tick")
	}
	checks := map[string]func() error{
		"create":  func() error { _, err := s.Create(game.Config{}); return err },
		"resume":  func() error { _, err := s.Resume(seat.MatchID); return err },
		"load":    func() error { _, err := s.Authorized(seat.MatchID, seat.Token); return err },
		"stream":  func() error { _, err := s.AuthorizedStream(seat.MatchID, seat.Token); return err },
		"save":    func() error { _, err := s.Save(seat.MatchID, m, false); return err },
		"leave":   func() error { _, err := s.Save(seat.MatchID, m, true); return err },
		"delete":  func() error { return s.Delete(seat.MatchID, m) },
		"list":    func() error { _, err := s.List(""); return err },
		"command": func() error { _, err := m.Apply(game.Command{ID: "late", Kind: "pause"}); return err },
		"history": func() error { _, err := m.Log(game.LogQuery{}); return err },
	}
	for name, call := range checks {
		if err := call(); !errors.Is(err, ErrShuttingDown) {
			t.Errorf("%s crossed the shutdown barrier: %v", name, err)
		}
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if err := s.Close(); err != nil {
				t.Errorf("concurrent close: %v", err)
			}
		})
	}
	wg.Wait()
	if s.lifecycle.State() != serviceClosed || m.lifecycle.State() != leaseClosed || len(s.matches) != 0 {
		t.Fatal("close did not release the service and leases")
	}
	if _, err := m.Apply(game.Command{ID: "after-close", Kind: "pause"}); !errors.Is(err, ErrNotFound) {
		t.Fatal("stale handle changed a checkpointed world")
	}

	restored, err := OpenService(path)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	var saves int
	if err := restored.db.QueryRow("SELECT count(*) FROM close_saves").Scan(&saves); err != nil || saves != 1 {
		t.Fatalf("final checkpoint ran %d times: %v", saves, err)
	}
	loaded, err := restored.Authorized(seat.MatchID, seat.Token)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(view, loaded.View()) || loaded.accumulator != game.Step/2 {
		t.Fatal("shutdown changed the persisted game or clock")
	}
	if got, err := loaded.Apply(cmd); err != nil || got != receipt || !loaded.View().Paused {
		t.Fatal("retry executed again after shutdown")
	}
}

func TestShutdownStopsActiveSchedulerWithoutCallerCancellation(t *testing.T) {
	s := NewService()
	seat, err := s.Create(game.Config{Difficulty: "peaceful"})
	if err != nil {
		t.Fatal(err)
	}
	m, _ := s.Authorized(seat.MatchID, seat.Token)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); s.Run(ctx) }()
	deadline := time.Now().Add(time.Second)
	for m.View().Tick == 0 {
		if time.Now().After(deadline) {
			t.Fatal("scheduler did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	s.BeginShutdown()
	view := m.View()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("scheduler ignored the shutdown barrier")
	}
	if !reflect.DeepEqual(view, m.View()) {
		t.Fatal("simulation advanced after the barrier")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestShutdownSaveFailureKeepsAtomicCheckpointAndReportsSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.sqlite")
	s, err := OpenService(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	seats := make([]Session, 2)
	for i := range seats {
		seats[i], err = s.Create(game.Config{Difficulty: "peaceful"})
		if err != nil {
			t.Fatal(err)
		}
	}
	failed, _ := s.Authorized(seats[0].MatchID, seats[0].Token)
	before := failed.View()
	beforeLog, _ := failed.Log(game.LogQuery{FromStart: true, Limit: 200})
	// Fail an event insert after the session row has been written. The whole
	// transaction must roll back, while the other game's final save proceeds.
	if _, err := s.db.Exec("CREATE TRIGGER reject_events BEFORE INSERT ON events WHEN NEW.session_id='" + seats[0].MatchID + "' BEGIN SELECT RAISE(ABORT,'disk unavailable'); END;"); err != nil {
		t.Fatal(err)
	}
	for _, seat := range seats {
		m, _ := s.Authorized(seat.MatchID, seat.Token)
		if _, err := m.Apply(game.Command{ID: "pause", Kind: "pause"}); err != nil {
			t.Fatal(err)
		}
	}
	closeErr := s.Close()
	if closeErr == nil || !strings.Contains(closeErr.Error(), seats[0].MatchID) || !strings.Contains(closeErr.Error(), "disk unavailable") {
		t.Fatalf("save failure not identified: %v", closeErr)
	}
	if err := s.Close(); err != closeErr {
		t.Fatalf("repeated close lost its failure: %v", err)
	}
	if err := s.db.Ping(); err == nil {
		t.Fatal("failed shutdown left SQLite open")
	}
	restored, err := OpenService(path)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if _, err := restored.db.Exec("DROP TRIGGER reject_events"); err != nil {
		t.Fatal(err)
	}
	m, _ := restored.Authorized(seats[0].MatchID, seats[0].Token)
	log, _ := m.Log(game.LogQuery{FromStart: true, Limit: 200})
	if !reflect.DeepEqual(before, m.View()) || !reflect.DeepEqual(beforeLog, log) {
		t.Fatal("failed save committed a partial checkpoint or journal")
	}
	m, _ = restored.Authorized(seats[1].MatchID, seats[1].Token)
	if !m.View().Paused {
		t.Fatal("one save failure prevented another game's checkpoint")
	}
}

func TestShutdownCheckpointDeadlineAndLockRetry(t *testing.T) {
	for _, release := range []bool{false, true} {
		t.Run(map[bool]string{false: "deadline", true: "retry"}[release], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "sessions.sqlite")
			s, err := OpenService(path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = s.Close() })
			seat, err := s.Create(game.Config{Difficulty: "peaceful"})
			if err != nil {
				t.Fatal(err)
			}
			m, _ := s.Authorized(seat.MatchID, seat.Token)
			if _, err := m.Apply(game.Command{ID: "pause", Kind: "pause"}); err != nil {
				t.Fatal(err)
			}
			locker, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			defer locker.Close()
			locker.SetMaxOpenConns(1)
			if _, err := locker.Exec("BEGIN IMMEDIATE"); err != nil {
				t.Fatal(err)
			}
			defer locker.Exec("ROLLBACK")
			budget := 400 * time.Millisecond
			if release {
				budget = 2 * time.Second
			}
			ctx, cancel := context.WithTimeout(context.Background(), budget)
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- s.CloseContext(ctx) }()
			if release {
				time.Sleep(150 * time.Millisecond)
				if _, err := locker.Exec("ROLLBACK"); err != nil {
					t.Fatal(err)
				}
			}
			select {
			case err := <-done:
				if release && err != nil || !release && !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("unexpected final checkpoint result: %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("checkpoint ignored its deadline")
			}
			if _, err := locker.Exec("ROLLBACK"); err != nil && !release {
				t.Fatal(err)
			}
			restored, err := OpenService(path)
			if err != nil {
				t.Fatal(err)
			}
			defer restored.Close()
			loaded, err := restored.Authorized(seat.MatchID, seat.Token)
			if err != nil || loaded.View().Paused != release {
				t.Fatalf("wrong checkpoint after contention: %v", err)
			}
		})
	}
}
