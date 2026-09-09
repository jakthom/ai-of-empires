package matches

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"crowns/internal/game"
)

func TestIsolatedSQLiteSnapshotCarriesWorldUsersJournalAndReceipts(t *testing.T) {
	s := roomService(t)
	owner, a, friend, b := joinedGame(t, s)
	other, _ := createLobby(t, s, 0)
	if s.stores[owner.MatchID] == s.stores[other.MatchID] || s.paths[owner.MatchID] == s.paths[other.MatchID] {
		t.Fatal("games share a database")
	}
	if _, err := b.Database(context.Background()); err == nil {
		t.Fatal("friend downloaded administrative state")
	}
	var foreign int
	if err := a.match.db.QueryRow("SELECT count(*) FROM sessions WHERE id!=?", owner.MatchID).Scan(&foreign); err != nil || foreign != 0 {
		t.Fatal("file contains another game", err)
	}
	if err := s.db.QueryRow("SELECT count(*) FROM sessions").Scan(&foreign); err != nil || foreign != 0 {
		t.Fatal("host catalog still contains gameplay", err)
	}
	view, _ := a.View()
	worker := 0
	for _, e := range view.Entities {
		if e.Type == "villager" && e.Owner == view.Player.ID {
			worker = e.ID
			break
		}
	}
	command := game.Command{ID: "portable-command", Kind: "stop", EntityIDs: []int{worker}}
	receipt, err := a.Apply(command)
	if err != nil {
		t.Fatal(err)
	}
	// Include sampled and partially accumulated production in the real world.
	a.match.mu.Lock()
	for range 143 {
		a.match.world.Update()
	}
	a.match.mu.Unlock()
	if _, err = a.Pause(controlFor(t, a, "pause-export")); err != nil {
		t.Fatal(err)
	}
	control := controlFor(t, a, "archive-export")
	transfer, err := a.Transfer(TransferRequest{ID: control.ID, Revision: control.Revision, Kind: "copy"})
	if err != nil {
		t.Fatal(err)
	}
	archive, err := a.Archive(transfer.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	before, _ := a.View()
	logBefore, _ := a.Log(game.LogQuery{FromStart: true, Limit: 200})
	for _, e := range logBefore.Events {
		if e.UserID != owner.MembershipID {
			t.Fatalf("foreign event exposed: %+v", e)
		}
	}
	for _, table := range []string{"browser_members", "game_secrets", "events", "game_archives"} {
		var count int
		if err = a.match.db.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count == 0 {
			t.Fatalf("missing portable %s", table)
		}
	}
	export, err := a.Database(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(export)
	if err != nil {
		t.Fatal(err)
	}
	if err = export.Close(); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "SQLite format 3") {
		t.Fatal("download is not a SQLite file")
	}
	path := filepath.Join(t.TempDir(), "host.sqlite")
	if err = os.MkdirAll(path+".games", 0700); err != nil {
		t.Fatal(err)
	}
	// Discovery uses the stored game ID, so a downloaded filename may change.
	if err = os.WriteFile(filepath.Join(path+".games", "evening.sqlite"), data, 0600); err != nil {
		t.Fatal(err)
	}
	dest, err := OpenService(path)
	if err != nil {
		t.Fatal(err)
	}
	defer dest.Close()
	if len(dest.stores) != 1 {
		t.Fatal("copy depended on other games or a host index")
	}
	restored, err := dest.Access(owner.MatchID, owner.Token, "")
	if err != nil {
		t.Fatal(err)
	}
	after, err := restored.View()
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("standalone database changed saved world", err)
	}
	logAfter, _ := restored.Log(game.LogQuery{FromStart: true, Limit: 200})
	if !reflect.DeepEqual(logBefore, logAfter) {
		t.Fatal("standalone database lost journal")
	}
	if got, err := restored.Archive(transfer.ID, ""); err != nil || !reflect.DeepEqual(got, archive) {
		t.Fatal("standalone database lost saved transfer archive", err)
	}
	if got, err := restored.Apply(command); err != nil || got != receipt {
		t.Fatal("command receipt lost or replayed", err)
	}
	if _, err := dest.Access(other.MatchID, owner.Token, ""); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign game appeared in copied database", err)
	}
	if _, err := dest.Access(owner.MatchID, friend.Token, ""); err != nil {
		t.Fatal("friend credential lost", err)
	}
	library, err := dest.Games("browser-bob", "")
	if err != nil || len(library.Games) != 1 {
		t.Fatal("private browser library lost", err)
	}
	recovered, err := dest.Rejoin(RejoinRequest{Code: friend.RejoinCode}, "new-browser")
	if err != nil || recovered.MembershipID != friend.MembershipID {
		t.Fatal("rejoin identity lost", err)
	}
	private, err := dest.Access(owner.MatchID, recovered.Token, "")
	if err != nil {
		t.Fatal(err)
	}
	page, _ := private.Log(game.LogQuery{})
	for _, e := range page.Events {
		if e.UserID != friend.MembershipID {
			t.Fatal("foreign log leaked after rejoin")
		}
	}
}

func legacyLibraryFixture(t *testing.T) (string, MemberSession, game.EventPage) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "old.sqlite")
	db, err := openDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	// Build a genuine combined-library fixture with both memberships and events.
	legacy := NewService()
	legacy.db = db
	owner, a, _, _ := joinedGame(t, legacy)
	for _, id := range []string{owner.MatchID} {
		isolated := legacy.stores[id]
		// Its in-memory file is materialized, then attached to the old catalog.
		file := filepath.Join(t.TempDir(), "source.sqlite")
		if _, err = isolated.Exec("VACUUM INTO ?", file); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec("ATTACH DATABASE ? AS source", file); err != nil {
			t.Fatal(err)
		}
		for _, table := range []string{"sessions", "events", "browser_members", "game_secrets", "game_archives", "imported_transfers"} {
			if _, err = db.Exec("INSERT INTO main." + table + " SELECT * FROM source." + table); err != nil {
				t.Fatal(err)
			}
		}
		if _, err = db.Exec("DETACH DATABASE source"); err != nil {
			t.Fatal(err)
		}
	}
	before, _ := a.Log(game.LogQuery{FromStart: true, Limit: 200})
	if err = legacy.Close(); err != nil {
		t.Fatal(err)
	}
	return path, owner, before
}

func TestLegacyLibraryMigrationIsCompleteAndRestartable(t *testing.T) {
	path, owner, before := legacyLibraryFixture(t)
	for range 2 {
		s, err := OpenService(path)
		if err != nil {
			t.Fatal(err)
		}
		a, err := s.Access(owner.MatchID, owner.Token, "")
		if err != nil {
			t.Fatal(err)
		}
		after, _ := a.Log(game.LogQuery{FromStart: true, Limit: 200})
		if !reflect.DeepEqual(before, after) {
			t.Fatal("migration lost history")
		}
		library, err := s.Games("browser-alice", "")
		if err != nil || len(library.Games) != 1 {
			t.Fatal("migration lost membership binding")
		}
		var count int
		if err = s.db.QueryRow("SELECT count(*) FROM events").Scan(&count); err != nil || count != 0 {
			t.Fatal("migration left events in combined database")
		}
		if err = s.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLegacyMigrationRecoversAfterCopyAndDetectsConflictingHistory(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		t.Run(map[bool]string{false: "interrupted", true: "conflicting journal"}[conflict], func(t *testing.T) {
			path, owner, before := legacyLibraryFixture(t)
			catalog, err := openDatabase(path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = catalog.Exec("CREATE TRIGGER interrupt_migration BEFORE DELETE ON sessions BEGIN SELECT RAISE(FAIL,'migration interrupted'); END"); err != nil {
				t.Fatal(err)
			}
			catalog.Close()
			if s, err := OpenService(path); err == nil {
				s.Close()
				t.Fatal("failure did not interrupt migration after its isolated copy")
			}
			catalog, err = openDatabase(path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = catalog.Exec("DROP TRIGGER interrupt_migration"); err != nil {
				t.Fatal(err)
			}
			catalog.Close()
			if conflict {
				isolated, err := openDatabase(filepath.Join(path+".games", owner.MatchID+".sqlite"))
				if err != nil {
					t.Fatal(err)
				}
				if _, err = isolated.Exec("DELETE FROM events WHERE session_id=?", owner.MatchID); err != nil {
					t.Fatal(err)
				}
				isolated.Close()
			}
			restored, err := OpenService(path)
			if conflict {
				if err == nil {
					restored.Close()
					t.Fatal("migration accepted incomplete event history")
				}
				if !strings.Contains(err.Error(), "conflicting legacy events") {
					t.Fatal(err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer restored.Close()
			a, err := restored.Access(owner.MatchID, owner.Token, "")
			if err != nil {
				t.Fatal(err)
			}
			after, err := a.Log(game.LogQuery{FromStart: true, Limit: 200})
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatal("interrupted migration changed history", err)
			}
		})
	}
}

func TestIsolatedDatabaseKeepsMoveImportReceipt(t *testing.T) {
	source := roomService(t)
	owner, a, _, _ := joinedGame(t, source)
	control := controlFor(t, a, "move")
	transfer, err := a.Transfer(TransferRequest{ID: control.ID, Revision: control.Revision, Kind: "move"})
	if err != nil {
		t.Fatal(err)
	}
	archive, err := a.Archive(transfer.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	destination := roomService(t)
	req := ImportRequest{Archive: archive, RejoinCode: owner.RejoinCode}
	imported, err := destination.Import(req, "browser-importer")
	if err != nil {
		t.Fatal(err)
	}
	access, err := destination.Access(imported.Session.MatchID, imported.Session.Token, "")
	if err != nil {
		t.Fatal(err)
	}
	file, err := access.Database(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	path := filepath.Join(t.TempDir(), "host.sqlite")
	if err = os.MkdirAll(path+".games", 0700); err != nil {
		t.Fatal(err)
	}
	out, err := os.Create(filepath.Join(path+".games", imported.Session.MatchID+".sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	_, copyErr := io.Copy(out, file)
	if err = errors.Join(copyErr, out.Close()); err != nil {
		t.Fatal(err)
	}
	restored, err := OpenService(path)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	repeated, err := restored.Import(req, "browser-importer")
	if err != nil || !repeated.AlreadyImported || repeated.CompletionReceipt != imported.CompletionReceipt || repeated.Session.MatchID != imported.Session.MatchID {
		t.Fatal("portable database lost import receipt or duplicated game", err)
	}
}

func TestFailedGameCreationLeavesNoOrphanFile(t *testing.T) {
	s := roomService(t)
	// A path error tests production failure handling without a test-only hook.
	blocked := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocked, []byte("occupied"), 0600); err != nil {
		t.Fatal(err)
	}
	s.gamesDir = blocked
	_, err := s.CreateGame(CreateGame{Config: game.Config{Settlements: 1}, PlayerName: "Alice"}, "browser")
	if err == nil || len(s.stores) != 0 || len(s.matches) != 0 {
		t.Fatal("failed create left an admitted game", err)
	}
}

func TestShutdownContinuesAcrossIsolatedDatabaseFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "host.sqlite")
	s, err := OpenService(path)
	if err != nil {
		t.Fatal(err)
	}
	first, a := createLobby(t, s, 0)
	second, b := createLobby(t, s, 0)
	if _, err = a.match.db.Exec("CREATE TRIGGER fail_checkpoint BEFORE UPDATE ON sessions BEGIN SELECT RAISE(FAIL,'broken game'); END"); err != nil {
		t.Fatal(err)
	}
	b.match.mu.Lock()
	b.match.room.Config.Name = "Saved independently"
	b.match.mu.Unlock()
	if err = s.Close(); err == nil || !strings.Contains(err.Error(), first.MatchID) {
		t.Fatal("save failure missing", err)
	}
	probe, err := sql.Open("sqlite", filepath.Join(path+".games", second.MatchID+".sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer probe.Close()
	var metadata string
	if err = probe.QueryRow("SELECT metadata FROM sessions").Scan(&metadata); err != nil || !strings.Contains(metadata, "Saved independently") {
		t.Fatal("another game's failed save prevented checkpoint", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err = a.match.db.PingContext(ctx); err == nil {
		t.Fatal("failed game's database was not closed")
	}
}
