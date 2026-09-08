package matches

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"crowns/internal/game"
)

func TestSQLiteRestartResumesByTokenNameAndID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.sqlite")
	s, err := OpenService(path)
	if err != nil {
		t.Fatal(err)
	}
	seat, err := s.Create(game.Config{Name: "Evening kingdom", Difficulty: "peaceful", Settlements: 3, World: game.WorldOptions{Type: "islands", Biome: "alpine", Size: "medium", TreatyMinutes: 10}})
	if err != nil {
		t.Fatal(err)
	}
	m, _ := s.Authorized(seat.MatchID, seat.Token)
	cmd := game.Command{ID: "pause-once", Kind: "pause"}
	receipt, err := m.Apply(cmd)
	if err != nil {
		t.Fatal(err)
	}
	view := m.View()
	rejected := game.Command{ID: "rejected-once", Kind: "train", EntityIDs: []int{view.Entities[0].ID}, Product: "villager"}
	_, rejection := m.Apply(rejected)
	if rejection == nil {
		t.Fatal("paused game accepted training")
	}
	view = m.View()
	log, err := m.Log(game.LogQuery{FromStart: true, Limit: 200})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenService(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	m, err = s.Authorized(seat.MatchID, seat.Token)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(view, m.View()) {
		t.Fatal("restart changed snapshot")
	}
	gotLog, _ := m.Log(game.LogQuery{FromStart: true, Limit: 200})
	if !reflect.DeepEqual(log, gotLog) {
		t.Fatal("restart changed immutable history")
	}
	if _, err = m.Apply(rejected); err == nil || err.Error() != rejection.Error() {
		t.Fatal("restart changed a rejected command result")
	}
	got, err := m.Apply(cmd)
	if err != nil || got != receipt || !m.View().Paused {
		t.Fatal("retried command ran twice after restart")
	}
	if _, err = s.Create(game.Config{Name: "EVENING KINGDOM"}); !errors.Is(err, ErrNameExists) {
		t.Fatalf("duplicate name: %v", err)
	}
	if _, err = s.Save(seat.MatchID, m, true); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Apply(game.Command{ID: "stale", Kind: "pause"}); !errors.Is(err, ErrNotFound) {
		t.Fatal("unloaded match accepted a command")
	}
	list, err := s.List("evening")
	if err != nil || len(list.Games) != 1 || list.Games[0].Active {
		t.Fatal("saved game missing from name search")
	}
	resumed, err := s.Resume("evening kingdom")
	if err != nil || resumed.MatchID != seat.MatchID {
		t.Fatalf("resume name: %v", err)
	}
	if _, err = s.Authorized(seat.MatchID, seat.Token); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("resume did not rotate token")
	}
	resumed, err = s.Resume(seat.MatchID)
	if err != nil {
		t.Fatal(err)
	}
	m, err = s.Authorized(resumed.MatchID, resumed.Token)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Delete(seat.MatchID, m); err != nil {
		t.Fatal(err)
	}
	list, err = s.List("")
	if err != nil || len(list.Games) != 0 {
		t.Fatal("explicit deletion retained session")
	}
	var events int
	if err = s.db.QueryRow("SELECT count(*) FROM events").Scan(&events); err != nil || events != 0 {
		t.Fatal("deleted session retained events")
	}
}

func TestAutosaveAndAtomicFailure(t *testing.T) {
	s, err := OpenService(filepath.Join(t.TempDir(), "sessions.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	seat, err := s.Create(game.Config{Name: "Autosave", Difficulty: "peaceful"})
	if err != nil {
		t.Fatal(err)
	}
	m, _ := s.Authorized(seat.MatchID, seat.Token)
	m.mu.Lock()
	m.lastSaveAttempt = time.Now().Add(-AutosaveInterval)
	m.world.Update()
	m.mu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	time.Sleep(150 * time.Millisecond)
	cancel()
	<-done
	m.mu.Lock()
	saved := m.savedCursor
	stamp := m.savedAt
	if saved != m.world.NextEvent || m.world.Tick == 0 {
		t.Fatal("periodic save did not run")
	}
	_, err = s.db.Exec("CREATE TRIGGER reject_events BEFORE INSERT ON events BEGIN SELECT RAISE(ABORT,'disk unavailable'); END;")
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.world.Checkpoint()
	if err != nil {
		t.Fatal(err)
	}
	m.mu.Unlock()
	if _, err = m.Apply(game.Command{ID: "pause", Kind: "pause"}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Save(seat.MatchID, m, true); err == nil {
		t.Fatal("failed save reported success")
	}
	m.mu.Lock()
	if m.savedAt != stamp || m.savedCursor != saved || m.saveError == "" || m.lifecycle.State() != leaseOpen {
		t.Fatal("failed save lost unsaved state or acknowledged a partial save")
	}
	m.mu.Unlock()
	var metadata []byte
	if err = s.db.QueryRow("SELECT metadata FROM sessions WHERE id=?", seat.MatchID).Scan(&metadata); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec("DROP TRIGGER reject_events"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Save(seat.MatchID, m, false); err != nil {
		t.Fatal(err)
	}
	if m.Info().SaveError != "" {
		t.Fatal("successful retry retained failure")
	}
}
