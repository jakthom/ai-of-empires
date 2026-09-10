package httpapi

import (
	"net/http/httptest"
	"testing"

	"crowns/internal/game"
	"crowns/internal/matches"
)

func TestObserverAndWorldStatisticsRemainOwnerOnlyDuringPlay(t *testing.T) {
	f := newMCPGame(t, "hidden")
	f.start(t)
	base := "/api/v1/games/" + f.owner.MatchID
	before := f.view(t, f.owner)
	view := decodeResponse[game.Snapshot](t, request(t, f.server, "GET", base+"/observer", f.owner.Token, nil), 200)
	if !view.GodMode || len(view.Entities) <= len(before.Entities) {
		t.Fatal("owner observer missing world")
	}
	for _, fog := range view.Map.Fog {
		if fog != 2 {
			t.Fatal("observer still fogged")
		}
	}
	world := decodeResponse[game.StatisticsReport](t, request(t, f.server, "GET", base+"/statistics?scope=world", f.owner.Token, nil), 200)
	if len(world.Kingdoms) != 2 || len(world.Awards) != 7 {
		t.Fatal("owner report incomplete")
	}
	friend := decodeResponse[game.StatisticsReport](t, request(t, f.server, "GET", base+"/statistics", f.friend.Token, nil), 200)
	if len(friend.Kingdoms) != 1 || friend.Kingdoms[0].ID != f.friend.PlayerID || len(friend.Awards) != 0 {
		t.Fatal("player report includes opponents")
	}
	connection := decodeResponse[matches.ConnectionInfo](t, request(t, f.server, "POST", base+"/connections", f.friend.Token, nil), 201)
	for _, path := range []string{"/observer", "/observer/events?connection=" + connection.ID, "/statistics?scope=world"} {
		decodeResponse[ErrorBody](t, request(t, f.server, "GET", base+path, f.friend.Token, nil), 403)
	}
	host := httptest.NewServer(f.server)
	t.Cleanup(host.Close)
	agent := connectMCP(t, host, f.owner)
	report := mcpOutput[game.StatisticsReport](t, agent, "statistics", struct{}{})
	if report.Scope != "kingdom" || len(report.Kingdoms) != 1 {
		t.Fatal("owner gameplay agent obtained administration")
	}
	if r := callMCP(t, agent, "statistics", map[string]any{"scope": "world"}); !r.IsError {
		t.Fatal("statistics accepted a forged scope")
	}
	credential := decodeResponse[AgentEndpoint](t, request(t, f.server, "POST", base+"/agent", f.owner.Token, nil), 200)
	for _, path := range []string{"/observer", "/statistics?scope=world"} {
		decodeResponse[ErrorBody](t, request(t, f.server, "GET", base+path, credential.Token, nil), 401)
	}
	after := f.view(t, f.owner)
	if after.GodMode || len(after.Entities) >= len(view.Entities) {
		t.Fatal("owner observer changed gameplay view")
	}
}

func TestFinishedCampaignStatisticsAreAvailableToAllMembers(t *testing.T) {
	f := newMCPGame(t, "hidden")
	f.start(t)
	base := "/api/v1/games/" + f.owner.MatchID
	decodeResponse[matches.Receipt](t, request(t, f.server, "POST", base+"/commands", f.owner.Token, game.Command{ID: "resign-final-report", Kind: "resign"}), 200)
	r := decodeResponse[game.StatisticsReport](t, request(t, f.server, "GET", base+"/statistics?scope=world", f.friend.Token, nil), 200)
	if r.Status != "finished" || r.OfficialWinner != f.friend.PlayerID || len(r.Kingdoms) != 2 || len(r.Summary) != 2 || len(r.Awards) != 7 {
		t.Fatal("final report missing outcome", r)
	}
	decodeResponse[ErrorBody](t, request(t, f.server, "GET", base+"/observer", f.friend.Token, nil), 403)
}
