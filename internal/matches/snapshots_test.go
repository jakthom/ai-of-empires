package matches

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"crowns/internal/game"
)

func TestNamedSnapshotForkIsPrivateDurableAndRepeatable(t *testing.T) {
	s := roomService(t)
	owner, a, friend, b := joinedGame(t, s)
	if _, err := b.SaveSnapshot(SaveSnapshot{ID: "forbidden", Name: "No"}); !errors.Is(err, ErrForbidden) {
		t.Fatal("friend saved administrative state", err)
	}
	if _, err := b.Snapshots(); !errors.Is(err, ErrForbidden) {
		t.Fatal("friend listed administrative state", err)
	}
	if _, err := a.SaveSnapshot(SaveSnapshot{ID: "empty", Name: " "}); err == nil {
		t.Fatal("accepted blank name")
	}
	if _, err := a.Pause(controlFor(t, a, "pause")); err != nil {
		t.Fatal(err)
	}
	before, err := a.View()
	if err != nil {
		t.Fatal(err)
	}
	req := SaveSnapshot{ID: "before-raid", Name: "Before the raid"}
	saved, err := a.SaveSnapshot(req)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Tick != before.Tick || saved.Time != before.Time {
		t.Fatal("snapshot point drifted")
	}
	if duplicate, err := a.SaveSnapshot(req); err != nil || duplicate != saved {
		t.Fatal("save was not exactly once", err)
	}
	if _, err = a.SaveSnapshot(SaveSnapshot{ID: req.ID, Name: "Changed"}); err == nil {
		t.Fatal("accepted conflicting retry")
	}
	if _, err = s.ForkSnapshot(b, req.ID, ForkSnapshot{ID: "denied", Name: "No"}, "browser-bob"); !errors.Is(err, ErrForbidden) {
		t.Fatal("friend forked private state", err)
	}
	// Mutating the source after capture must not alter any saved entity or order.
	if _, err = a.ResumeGame(controlFor(t, a, "resume")); err != nil {
		t.Fatal(err)
	}
	var worker game.EntityView
	for _, e := range before.Entities {
		if e.Type == "villager" && e.Owner == 1 {
			worker = e
			break
		}
	}
	goal := game.Vec{X: worker.Position.X + 5, Y: worker.Position.Y + 2}
	if _, err = a.Apply(game.Command{ID: "move", Kind: "move", EntityIDs: []int{worker.ID}, Position: &goal}); err != nil {
		t.Fatal(err)
	}
	a.match.mu.Lock()
	for range 100 {
		a.match.world.Update()
	}
	a.match.mu.Unlock()
	result, err := s.ForkSnapshot(a, req.ID, ForkSnapshot{ID: "start-once", Name: "Try another strategy"}, "browser-alice")
	if err != nil {
		t.Fatal(err)
	}
	if result.Session.MatchID == owner.MatchID || result.Session.Token == owner.Token || result.Session.RejoinCode == owner.RejoinCode || !result.Session.Owner {
		t.Fatal("fork did not issue fresh game ownership")
	}
	copyAccess, err := s.Access(result.Session.MatchID, result.Session.Token, "")
	if err != nil {
		t.Fatal(err)
	}
	v, err := copyAccess.View()
	if err != nil {
		t.Fatal(err)
	}
	if !v.Paused || v.Tick != before.Tick || v.Time != before.Time || !reflect.DeepEqual(v.Player.Resources, before.Player.Resources) {
		t.Fatal("fork lost snapshot economy/clock")
	}
	for _, e := range v.Entities {
		if e.ID == worker.ID && e.Position != worker.Position {
			t.Fatal("fork captured source after snapshot")
		}
	}
	if _, err = s.Access(result.Session.MatchID, friend.Token, ""); err == nil {
		t.Fatal("old friend credential entered copy")
	}
	info, _ := copyAccess.Info()
	if info.Seats[1].Status == "claimed" {
		t.Fatal("copied friend's private membership")
	}
	duplicate, err := s.ForkSnapshot(a, req.ID, ForkSnapshot{ID: "start-once", Name: "Try another strategy"}, "browser-alice")
	if err != nil || duplicate.Session.MatchID != result.Session.MatchID || !duplicate.AlreadyImported {
		t.Fatal("fork duplicated on retry", err)
	}
	if _, err = s.ForkSnapshot(a, req.ID, ForkSnapshot{ID: "start-once", Name: "Different"}, "browser-alice"); err == nil {
		t.Fatal("fork accepted conflicting retry")
	}
	// SQLite exports contain both the current world and named historical points.
	dbFile, err := a.Database(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer dbFile.Close()
	db, err := sql.Open("sqlite", dbFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err = db.QueryRow("SELECT count(*) FROM game_snapshots WHERE game_id=?", owner.MatchID).Scan(&count); err != nil || count != 1 {
		t.Fatal("export omitted snapshots", count, err)
	}
	var check string
	if err = db.QueryRow("PRAGMA integrity_check").Scan(&check); err != nil || check != "ok" {
		t.Fatal("incomplete SQLite export", check, err)
	}
	if err = b.DeleteSnapshot(req.ID); !errors.Is(err, ErrForbidden) {
		t.Fatal("friend deleted snapshot", err)
	}
	if err = a.DeleteSnapshot(req.ID); err != nil {
		t.Fatal(err)
	}
	if err = a.DeleteSnapshot(req.ID); err != nil {
		t.Fatal("delete retry failed", err)
	}
	list, err := a.Snapshots()
	if err != nil || len(list.Snapshots) != 0 {
		t.Fatal("snapshot not deleted", err)
	}
}

func TestNamedSnapshotsSurviveServerRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "games.sqlite")
	s, err := OpenService(path)
	if err != nil {
		t.Fatal(err)
	}
	owner, a := createLobby(t, s, 0)
	readySeat(t, a)
	if _, err = a.Start(controlFor(t, a, "start")); err != nil {
		t.Fatal(err)
	}
	point, err := a.SaveSnapshot(SaveSnapshot{ID: "first", Name: "First settlement"})
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
	a, err = s.Access(owner.MatchID, owner.Token, "")
	if err != nil {
		t.Fatal(err)
	}
	list, err := a.Snapshots()
	if err != nil || len(list.Snapshots) != 1 || list.Snapshots[0] != point {
		t.Fatal("snapshot lost on restart", err)
	}
	result, err := s.ForkSnapshot(a, point.ID, ForkSnapshot{ID: "restored", Name: "Restored chapter"}, "browser-alice")
	if err != nil {
		t.Fatal(err)
	}
	copy, err := s.Access(result.Session.MatchID, result.Session.Token, "")
	if err != nil {
		t.Fatal(err)
	}
	v, err := copy.View()
	if err != nil || v.Tick != point.Tick || !v.Paused {
		t.Fatal("restored snapshot could not start", err)
	}
}
