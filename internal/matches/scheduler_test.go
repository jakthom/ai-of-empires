package matches

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"crowns/internal/game"
)

func TestClockUsesElapsedTimeWithBoundedCatchUpAndPause(t *testing.T) {
	s := NewService()
	seat, err := s.Create(game.Config{Difficulty: "peaceful"})
	if err != nil {
		t.Fatal(err)
	}
	m, _ := s.Authorized(seat.MatchID, seat.Token)
	m.world.Speed = 32
	now := time.Now()
	m.lastStepAt = now
	// Small irregular wakeups still account for all elapsed time at 32x.
	for _, delay := range []time.Duration{3, 7, 4, 6, 8, 2, 5, 5, 5, 5} {
		now = now.Add(delay * time.Millisecond)
		m.step(context.Background(), now)
	}
	if math.Abs(m.world.Time+m.accumulator-1.6) > 1e-8 {
		t.Fatal("clock lost elapsed wall time")
	}
	before := m.world.Tick
	m.step(context.Background(), now.Add(time.Hour))
	if m.world.Tick-before > 8 || m.accumulator > maxClockDebt.Seconds()*32 {
		t.Fatal("unbounded catch-up after a stall")
	}
	if err := m.world.SetPaused(true); err != nil {
		t.Fatal(err)
	}
	before = m.world.Tick
	m.step(context.Background(), now.Add(2*time.Hour))
	if m.world.Tick != before || m.accumulator != 0 {
		t.Fatal("pause accumulated future simulation work")
	}
	if err := m.world.SetPaused(false); err != nil {
		t.Fatal(err)
	}
	m.step(context.Background(), now.Add(2*time.Hour+5*time.Millisecond))
	if m.world.Tick-before > 4 {
		t.Fatal("resume replayed time spent paused")
	}
}

func TestBlockedGameDoesNotBlockAnotherGameClock(t *testing.T) {
	s := NewService()
	a, _ := s.Create(game.Config{Difficulty: "peaceful"})
	b, _ := s.Create(game.Config{Difficulty: "peaceful"})
	m, _ := s.Authorized(a.MatchID, a.Token)
	n, _ := s.Authorized(b.MatchID, b.Token)
	m.mu.Lock()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	defer func() { m.mu.Unlock(); cancel(); <-done; _ = s.Close() }()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		n.mu.Lock()
		tick := n.world.Tick
		n.mu.Unlock()
		if tick > 1 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("one busy game stopped the other game's clock")
}

func TestAutosaveDoesNotHoldGameLockAndNewerSaveWins(t *testing.T) {
	s, err := OpenService(filepath.Join(t.TempDir(), "sessions.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	seat, err := s.Create(game.Config{Difficulty: "peaceful"})
	if err != nil {
		t.Fatal(err)
	}
	m, _ := s.Authorized(seat.MatchID, seat.Token)
	// Occupy this game's only SQL connection, deterministically stalling disk
	// work while normal authenticated commands and snapshots remain available.
	conn, err := m.db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	m.mu.Lock()
	m.startAutosave(context.Background(), time.Now())
	job := m.autosave
	m.mu.Unlock()
	if job == nil {
		t.Fatal("autosave did not start")
	}
	changed := make(chan error, 1)
	go func() {
		_, err := m.Apply(game.Command{ID: "during-save", Kind: "pause"})
		_ = m.View()
		changed <- err
	}()
	select {
	case err := <-changed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("disk write held the game lock")
	}
	select {
	case <-job.done:
		t.Fatal("test did not stall the writer")
	default:
	}
	_ = conn.Close()
	if _, err := s.Save(seat.MatchID, m, false); err != nil {
		t.Fatal(err)
	}
	var data []byte
	if err := m.db.QueryRow("SELECT checkpoint FROM sessions WHERE id=?", m.id).Scan(&data); err != nil {
		t.Fatal(err)
	}
	var stored storedMatch
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	journal := m.world.JournalSince(0)
	cursor := m.savedCursor
	m.mu.Unlock()
	restored, err := game.Restore(stored.World, journal)
	if err != nil {
		t.Fatal(err)
	}
	if !restored.View(1).Paused || !stored.Commands["during-save"].Receipt.Accepted || cursor != restored.NextEvent {
		t.Fatal("older autosave overwrote a newer command or journal")
	}
}

func TestShutdownCancelsAnAutosaveWaitingOnSQLite(t *testing.T) {
	s, err := OpenService(filepath.Join(t.TempDir(), "sessions.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	seat, err := s.Create(game.Config{Difficulty: "peaceful"})
	if err != nil {
		t.Fatal(err)
	}
	m, _ := s.Authorized(seat.MatchID, seat.Token)
	other, err := openDatabase(s.paths[m.id])
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if _, err := other.Exec("BEGIN EXCLUSIVE"); err != nil {
		t.Fatal(err)
	}
	defer other.Exec("ROLLBACK")
	m.mu.Lock()
	m.startAutosave(context.Background(), time.Now())
	job := m.autosave
	m.mu.Unlock()
	time.Sleep(50 * time.Millisecond)
	select {
	case <-job.done:
		t.Fatal("test did not block SQLite")
	default:
	}
	s.BeginShutdown()
	select {
	case <-job.done:
	case <-time.After(500 * time.Millisecond):
		t.Error("SQLite busy waiting delayed the shutdown barrier")
	}
	_, _ = other.Exec("ROLLBACK")
	<-job.done
	if job.err == nil {
		t.Fatal("canceled autosave reported a durable commit")
	}
}

func TestResumeDuringLeaseRemovalRestoresCheckpointAndFencesOldHandle(t *testing.T) {
	s := roomService(t)
	member, old := createLobby(t, s, 0)
	if _, err := old.Start(controlFor(t, old, "start")); err != nil {
		t.Fatal(err)
	}
	if _, err := old.Pause(controlFor(t, old, "pause")); err != nil {
		t.Fatal(err)
	}
	want, err := old.View()
	if err != nil {
		t.Fatal(err)
	}
	// The clock has saved and closed the lease, but is waiting for the
	// registry lock to remove it. A new browser request can arrive here.
	m := old.match
	m.mu.Lock()
	m.fireLease(releaseLease, time.Now())
	m.mu.Unlock()
	fresh, err := s.RequestAccess(member.MatchID, member.Token, "")
	if err != nil {
		t.Fatalf("resume raced a finished lease: %v", err)
	}
	defer fresh.Release()
	if fresh.match == m || s.matches[member.MatchID] != fresh.match {
		t.Fatal("resume retained the closed runtime")
	}
	got, err := fresh.View()
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("resume changed checkpoint: %v", err)
	}
	if _, err := old.Apply(game.Command{ID: "stale", Kind: "stop"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("stale handle reopened: %v", err)
	}
}
