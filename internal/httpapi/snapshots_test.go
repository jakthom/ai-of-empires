package httpapi

import (
	"testing"

	"crowns/internal/matches"
)

func TestSnapshotRoutesKeepAdministrationPrivate(t *testing.T) {
	f := newMCPGame(t, "explored")
	f.start(t)
	base := "/api/v1/games/" + f.owner.MatchID
	saved := decodeResponse[matches.SavedSnapshot](t, request(t, f.server, "POST", base+"/snapshots", f.owner.Token, matches.SaveSnapshot{ID: "before-war", Name: "Before war"}), 200)
	agent := decodeResponse[AgentEndpoint](t, request(t, f.server, "POST", base+"/agent", f.owner.Token, nil), 200)
	for _, credential := range []struct {
		name, token string
		status      int
	}{
		{"anonymous", "", 401}, {"opponent", f.friend.Token, 403}, {"owner's gameplay agent", agent.Token, 401},
	} {
		t.Run(credential.name, func(t *testing.T) {
			for _, route := range []struct {
				method, path string
				body         any
			}{
				{"GET", base + "/snapshots", nil},
				{"POST", base + "/snapshots", matches.SaveSnapshot{ID: "denied", Name: "No"}},
				{"POST", base + "/snapshots/" + saved.ID + "/fork", matches.ForkSnapshot{ID: "denied", Name: "No"}},
				{"DELETE", base + "/snapshots/" + saved.ID, nil},
			} {
				decodeResponse[ErrorBody](t, request(t, f.server, route.method, route.path, credential.token, route.body), credential.status)
			}
		})
	}
	library := decodeResponse[matches.SnapshotLibrary](t, request(t, f.server, "GET", base+"/snapshots", f.owner.Token, nil), 200)
	if len(library.Snapshots) != 1 || library.Snapshots[0] != saved {
		t.Fatal("refused requests changed saved points")
	}
	fork := decodeResponse[matches.ImportResult](t, request(t, f.server, "POST", base+"/snapshots/"+saved.ID+"/fork", f.owner.Token, matches.ForkSnapshot{ID: "fork", Name: "Another campaign"}), 200)
	if fork.Session.MatchID == f.owner.MatchID || !f.view(t, fork.Session).Paused {
		t.Fatal("snapshot did not create a separate paused game")
	}
	decodeResponse[ErrorBody](t, request(t, f.server, "GET", "/api/v1/games/"+fork.Session.MatchID+"/snapshot", f.friend.Token, nil), 401)
	if response := request(t, f.server, "DELETE", base+"/snapshots/"+saved.ID, f.owner.Token, nil); response.Code != 204 {
		t.Fatal(response.Code, response.Body)
	}
}
