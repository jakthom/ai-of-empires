package httpapi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"crowns/internal/game"
	"crowns/internal/matches"
)

func TestDatabaseDownloadIsPrivateAndRestoresWithoutHostCatalog(t *testing.T) {
	service, err := matches.OpenService(filepath.Join(t.TempDir(), "host.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	s := New(service, fstest.MapFS{})
	owner, err := service.CreateGame(matches.CreateGame{Config: game.Config{Name: "Portable realm", Settlements: 2}, Friends: 1, PlayerName: "Alice"}, "alice-browser")
	if err != nil {
		t.Fatal(err)
	}
	a, err := service.Access(owner.MatchID, owner.Token, "")
	if err != nil {
		t.Fatal(err)
	}
	info, err := a.Info()
	if err != nil {
		t.Fatal(err)
	}
	invite, err := a.Invite(info.Seats[1].ID, matches.InviteRequest{ID: "invite", Revision: info.Revision})
	if err != nil {
		t.Fatal(err)
	}
	friend, err := service.ClaimInvite(matches.ClaimInvite{ID: "claim", Secret: invite.Secret, Name: "Bob"}, "bob-browser")
	if err != nil {
		t.Fatal(err)
	}
	other, err := service.CreateGame(matches.CreateGame{Config: game.Config{Settlements: 1}, PlayerName: "Carol"}, "carol-browser")
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/games/" + owner.MatchID + "/database"
	for _, tc := range []struct {
		name, token string
		status      int
	}{{"anonymous", "", 401}, {"another game", other.Token, 401}, {"friend", friend.Token, 403}} {
		t.Run(tc.name, func(t *testing.T) {
			decodeResponse[ErrorBody](t, request(t, s, "POST", path, tc.token, nil), tc.status)
		})
	}
	response := request(t, s, "POST", path, owner.Token, nil)
	if response.Code != 200 || response.Header().Get("Content-Type") != "application/vnd.sqlite3" || !strings.HasPrefix(response.Body.String(), "SQLite format 3\x00") {
		t.Fatalf("invalid SQLite response: %d %v", response.Code, response.Header())
	}
	if !strings.Contains(response.Header().Get("Cache-Control"), "no-store") || !strings.Contains(response.Header().Get("Content-Disposition"), owner.MatchID+".sqlite") {
		t.Fatal("missing private download headers", response.Header())
	}
	newHost := filepath.Join(t.TempDir(), "new-host.sqlite")
	if err = os.MkdirAll(newHost+".games", 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(newHost+".games", "download.sqlite"), response.Body.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	restored, err := matches.OpenService(newHost)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	for _, member := range []matches.MemberSession{owner, friend} {
		if _, err = restored.Access(member.MatchID, member.Token, ""); err != nil {
			t.Fatal("download lost member access", err)
		}
	}
}
