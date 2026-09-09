package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"crowns/internal/game"
	"crowns/internal/matches"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type mcpGameFixture struct {
	service       *matches.Service
	server        *Server
	owner, friend matches.MemberSession
}

func newMCPGame(t *testing.T, reveal string) mcpGameFixture {
	t.Helper()
	svc := matches.NewService()
	t.Cleanup(func() { _ = svc.Close() })
	s := New(svc, fstest.MapFS{})
	owner := decodeResponse[matches.MemberSession](t, request(t, s, "POST", "/api/v1/games", "", matches.CreateGame{
		Config: game.Config{Settlements: 2, Civilization: "britons", Difficulty: "peaceful", Mode: "sandbox", World: game.WorldOptions{Reveal: reveal}}, Friends: 1, PlayerName: "Human",
	}), 201)
	base := "/api/v1/games/" + owner.MatchID
	info := decodeResponse[matches.GameInfo](t, request(t, s, "GET", base, owner.Token, nil), 200)
	invite := decodeResponse[matches.Invitation](t, request(t, s, "POST", base+"/seats/"+info.Seats[1].ID+"/invites", owner.Token, matches.InviteRequest{ID: "invite-agent", Revision: info.Revision}), 200)
	friend := decodeResponse[matches.MemberSession](t, request(t, s, "POST", "/api/v1/invites/claim", "", matches.ClaimInvite{ID: "claim-agent", Secret: invite.Secret, Name: "Agent opponent"}), 200)
	return mcpGameFixture{svc, s, owner, friend}
}

func (f mcpGameFixture) start(t *testing.T) {
	t.Helper()
	base := "/api/v1/games/" + f.owner.MatchID
	info := decodeResponse[matches.GameInfo](t, request(t, f.server, "GET", base, f.owner.Token, nil), 200)
	decodeResponse[matches.GameInfo](t, request(t, f.server, "POST", base+"/start", f.owner.Token, matches.GameControl{ID: "start", Revision: info.Revision}), 200)
}

func (f mcpGameFixture) view(t *testing.T, member matches.MemberSession) game.Snapshot {
	t.Helper()
	return decodeResponse[game.Snapshot](t, request(t, f.server, "GET", "/api/v1/games/"+member.MatchID+"/snapshot", member.Token, nil), 200)
}

type playerTransport struct {
	base  http.RoundTripper
	token string
}

func (t playerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(r)
}

func connectMCP(t *testing.T, host *httptest.Server, member matches.MemberSession) *mcp.ClientSession {
	t.Helper()
	credential := decodeResponse[AgentEndpoint](t, request(t, host.Config.Handler, "POST", "/api/v1/games/"+member.MatchID+"/agent", member.Token, nil), 200)
	client := mcp.NewClient(&mcp.Implementation{Name: "game-test", Version: "1"}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   host.URL + playerMCPPath(member.MatchID, member.MembershipID),
		HTTPClient: &http.Client{Transport: playerTransport{host.Client().Transport, credential.Token}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func callMCP(t *testing.T, session *mcp.ClientSession, name string, args any) *mcp.CallToolResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	v, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func mcpOutput[T any](t *testing.T, session *mcp.ClientSession, name string, args any) T {
	t.Helper()
	v := callMCP(t, session, name, args)
	if v.IsError {
		t.Fatalf("%s failed: %+v", name, v)
	}
	data, err := json.Marshal(v.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var out T
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func mcpRefusal(t *testing.T, session *mcp.ClientSession, name string, args any, code string) {
	t.Helper()
	v := callMCP(t, session, name, args)
	var data []byte
	if len(v.Content) > 0 {
		if content, ok := v.Content[0].(*mcp.TextContent); ok {
			data = []byte(content.Text)
		}
	}
	var body ErrorBody
	_ = json.Unmarshal(data, &body)
	if !v.IsError || body.Error.Code != code {
		t.Fatalf("%s: expected %s, got %+v %s", name, code, v, data)
	}
}

func ownEntity(t *testing.T, v game.Snapshot, typ string) game.EntityView {
	t.Helper()
	for _, e := range v.Entities {
		if e.Owner == v.Player.ID && e.Type == typ {
			return e
		}
	}
	t.Fatalf("no owned %s", typ)
	return game.EntityView{}
}

func rawMCP(t *testing.T, s http.Handler, method, path, token, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json, text/event-stream")
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

func TestMCPMembershipEndpointsRequireMatchingBearer(t *testing.T) {
	f := newMCPGame(t, "hidden")
	ownerPath, friendPath := "", ""
	for i, member := range []matches.MemberSession{f.owner, f.friend} {
		v := decodeResponse[AgentEndpoint](t, request(t, f.server, "GET", "/api/v1/games/"+member.MatchID+"/agent", member.Token, nil), 200)
		if v.MembershipID != member.MembershipID || v.PlayerID != member.PlayerID || v.MCPURL != playerMCPPath(member.MatchID, member.MembershipID) {
			t.Fatalf("incorrect endpoint discovery: %+v", v)
		}
		if i == 0 {
			ownerPath = v.MCPURL
		} else {
			friendPath = v.MCPURL
		}
	}
	if ownerPath == friendPath {
		t.Fatal("players shared an endpoint")
	}
	ownerCredential := decodeResponse[AgentEndpoint](t, request(t, f.server, "POST", "/api/v1/games/"+f.owner.MatchID+"/agent", f.owner.Token, nil), 200)
	friendCredential := decodeResponse[AgentEndpoint](t, request(t, f.server, "POST", "/api/v1/games/"+f.friend.MatchID+"/agent", f.friend.Token, nil), 200)
	if ownerCredential.Token == "" || ownerCredential.Token == f.owner.Token || friendCredential.Token == ownerCredential.Token {
		t.Fatal("agent credentials must be separate from sessions and other players")
	}
	for _, credential := range []AgentEndpoint{ownerCredential, friendCredential} {
		for _, path := range []string{"", "/snapshot", "/log", "/agent", "/session"} {
			decodeResponse[ErrorBody](t, request(t, f.server, "GET", "/api/v1/games/"+credential.GameID+path, credential.Token, nil), 401)
		}
		decodeResponse[ErrorBody](t, request(t, f.server, "POST", "/api/v1/games/"+credential.GameID+"/database", credential.Token, nil), 401)
		decodeResponse[ErrorBody](t, request(t, f.server, "POST", "/api/v1/games/"+credential.GameID+"/agent", credential.Token, nil), 401)
	}
	init := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"legacy-test","version":"1"}}}`
	for _, tc := range []struct {
		name, path, token string
		headers           map[string]string
		status            int
	}{
		{"anonymous", ownerPath, "", nil, 401},
		{"cookie only", ownerPath, "", map[string]string{"Cookie": "aoe_browser=some-browser"}, 401},
		{"query token", ownerPath + "?token=" + f.owner.Token, "", nil, 401},
		{"wrong token", ownerPath, "wrong", nil, 401},
		{"session token", ownerPath, f.owner.Token, nil, 401},
		{"other endpoint", friendPath, ownerCredential.Token, nil, 401},
		{"other membership", ownerPath, friendCredential.Token, nil, 401},
		{"unknown membership", playerMCPPath(f.owner.MatchID, "missing"), ownerCredential.Token, nil, 401},
		{"cross origin", ownerPath, ownerCredential.Token, map[string]string{"Origin": "https://foreign.example"}, 403},
		{"fake session", ownerPath, "", map[string]string{"Mcp-Session-Id": f.owner.MembershipID}, 401},
		{"owner legacy handshake", ownerPath, ownerCredential.Token, nil, 200},
		{"friend legacy handshake", friendPath, friendCredential.Token, nil, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := rawMCP(t, f.server, "POST", tc.path, tc.token, init, tc.headers)
			if v.Code != tc.status {
				t.Fatalf("got %d, want %d: %s", v.Code, tc.status, v.Body)
			}
			if v.Header().Get("Cache-Control") != "no-store" || v.Header().Get("Mcp-Session-Id") != "" {
				t.Fatal("MCP responses must not cache or create cross-request sessions")
			}
		})
	}
	for _, method := range []string{"GET", "DELETE"} {
		v := rawMCP(t, f.server, method, ownerPath, ownerCredential.Token, "", nil)
		if v.Code != 405 || v.Header().Get("Allow") != "POST" {
			t.Fatalf("stateless %s: %d %s", method, v.Code, v.Body)
		}
	}
	v := rawMCP(t, f.server, "POST", ownerPath, ownerCredential.Token, strings.Repeat(" ", 33<<10)+init, nil)
	if v.Code != 413 {
		t.Fatalf("oversized MCP body: %d %s", v.Code, v.Body)
	}
}

func TestMCPObservationAndCommandsUseTheAuthenticatedPlayer(t *testing.T) {
	f := newMCPGame(t, "hidden")
	f.start(t)
	host := httptest.NewServer(f.server)
	t.Cleanup(host.Close)
	owner, friend := connectMCP(t, host, f.owner), connectMCP(t, host, f.friend)
	one, two := f.view(t, f.owner), f.view(t, f.friend)
	for _, tc := range []struct {
		client *mcp.ClientSession
		view   game.Snapshot
	}{{owner, one}, {friend, two}} {
		v := mcpOutput[Observation](t, tc.client, "observe", ObserveRequest{IncludeMap: true, Limit: 200})
		want := tc.view
		slices.SortFunc(want.Entities, func(a, b game.EntityView) int { return a.ID - b.ID })
		if v.HasMore {
			want.Entities = want.Entities[:200]
		}
		if !reflect.DeepEqual(v.Snapshot, want) {
			t.Fatal("MCP observation differed from the same player's REST snapshot")
		}
		status := mcpOutput[AgentGame](t, tc.client, "game_status", struct{}{})
		if status.PlayerID != tc.view.Player.ID {
			t.Fatalf("wrong identity: %+v", status)
		}
	}
	foreign := ownEntity(t, two, "town_center")
	region := MapRegionRequest{X: int(foreign.Position.X), Y: int(foreign.Position.Y), Width: 1, Height: 1}
	unknown := mcpOutput[MapRegion](t, owner, "map_region", region)
	known := mcpOutput[MapRegion](t, friend, "map_region", region)
	if unknown.Fog[0] != 0 || unknown.Tiles[0].Terrain != "unknown" || known.Fog[0] != 2 || known.Tiles[0].Terrain == "unknown" {
		t.Fatal("map region leaked foreign exploration", unknown, known)
	}
	hidden := mcpOutput[Observation](t, owner, "observe", ObserveRequest{EntityIDs: []int{foreign.ID}})
	if len(hidden.Snapshot.Entities) != 0 {
		t.Fatal("ID filter revealed a hidden enemy")
	}
	worker := ownEntity(t, one, "villager")
	mcpRefusal(t, owner, "command", game.Command{ID: "foreign-command", Kind: "train", EntityIDs: []int{foreign.ID}, Product: "villager"}, "invalid_selection")
	mcpRefusal(t, owner, "command", game.Command{ID: "hidden-target", Kind: "attack", EntityIDs: []int{worker.ID}, TargetID: foreign.ID}, "invalid_target")
	mcpRefusal(t, owner, "read_log", AgentLogRequest{EntityID: foreign.ID}, "entity_not_found")

	// Both transports and concurrent agent calls share membership-scoped receipts.
	producer := ownEntity(t, one, "town_center")
	command := game.Command{ID: "shared-retry", Kind: "train", EntityIDs: []int{producer.ID}, Product: "villager"}
	first := mcpOutput[matches.Receipt](t, owner, "command", command)
	var wg sync.WaitGroup
	for range 5 {
		wg.Go(func() {
			v := mcpOutput[matches.Receipt](t, owner, "command", command)
			if v != first {
				t.Error("retry changed receipt")
			}
		})
	}
	wg.Wait()
	rest := decodeResponse[matches.Receipt](t, request(t, f.server, "POST", "/api/v1/games/"+f.owner.MatchID+"/commands", f.owner.Token, command), 200)
	if first != rest {
		t.Fatal("REST replay differed from MCP receipt")
	}
	after := f.view(t, f.owner)
	if len(ownEntity(t, after, "town_center").Tasks) != 1 || one.Player.Resources.Food-after.Player.Resources.Food != 50 {
		t.Fatal("retries spent resources or queued work more than once")
	}
	changed := command
	changed.Product = "scout"
	mcpRefusal(t, owner, "command", changed, "idempotency_conflict")
	command.EntityIDs = []int{foreign.ID}
	mcpOutput[matches.Receipt](t, friend, "command", command)
	if len(ownEntity(t, f.view(t, f.friend), "town_center").Tasks) != 1 {
		t.Fatal("another player's identical ID collided")
	}
	log := mcpOutput[game.EventPage](t, owner, "read_log", AgentLogRequest{Limit: 200})
	count := 0
	for _, e := range log.Events {
		if e.Player != f.owner.PlayerID || e.UserID != f.owner.MembershipID {
			t.Fatal("foreign private event exposed", e)
		}
		if e.CommandID == command.ID && e.Kind == "order" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly one command event, got %d", count)
	}
	// Cancellation is also exactly once and returns the original resources.
	cancel := game.Command{ID: "cancel-once", Kind: "cancel", EntityIDs: []int{producer.ID}}
	mcpOutput[matches.Receipt](t, owner, "command", cancel)
	mcpOutput[matches.Receipt](t, owner, "command", cancel)
	if got := f.view(t, f.owner); len(ownEntity(t, got, "town_center").Tasks) != 0 || got.Player.Resources != one.Player.Resources {
		t.Fatal("cancel failed to refund exactly once")
	}
}

func TestMCPVisibleOpponentsDoNotExposePrivatePlans(t *testing.T) {
	f := newMCPGame(t, "all")
	f.start(t)
	host := httptest.NewServer(f.server)
	t.Cleanup(host.Close)
	owner, friend := connectMCP(t, host, f.owner), connectMCP(t, host, f.friend)
	foreign := ownEntity(t, f.view(t, f.friend), "town_center")
	mcpOutput[matches.Receipt](t, friend, "command", game.Command{ID: "secret-production", Kind: "research", EntityIDs: []int{foreign.ID}, Product: "loom"})
	v := mcpOutput[Observation](t, owner, "observe", ObserveRequest{EntityIDs: []int{foreign.ID}})
	if len(v.Snapshot.Entities) != 1 {
		t.Fatal("visible enemy missing")
	}
	e := v.Snapshot.Entities[0]
	if !e.Visible || e.Activity != "Observed" || len(e.Tasks) != 0 || len(e.Actions) != 0 || len(e.Passengers) != 0 || e.Rally != nil {
		t.Fatal("visible enemy disclosed private details", e)
	}
	log := mcpOutput[game.EventPage](t, owner, "read_log", AgentLogRequest{EntityID: foreign.ID})
	for _, e := range log.Events {
		if e.Kind != "discovered" || e.CommandID != "" || e.UserID != f.owner.MembershipID {
			t.Fatal("visibility granted foreign history", e)
		}
	}
	status := callMCP(t, owner, "game_status", struct{}{})
	data, _ := json.Marshal(status.StructuredContent)
	for _, secret := range []string{"seed", f.friend.MembershipID, f.friend.Token, f.friend.RejoinCode} {
		if bytes.Contains(data, []byte(secret)) {
			t.Fatalf("game status disclosed %q", secret)
		}
	}
}

func TestMCPControlsPresenceAndRevocation(t *testing.T) {
	f := newMCPGame(t, "hidden")
	host := httptest.NewServer(f.server)
	t.Cleanup(host.Close)
	owner, friend := connectMCP(t, host, f.owner), connectMCP(t, host, f.friend)
	status := mcpOutput[AgentGame](t, owner, "game_status", struct{}{})
	mcpRefusal(t, friend, "start_game", matches.GameControl{ID: "foreign-start", Revision: status.Revision}, "forbidden")
	status = mcpOutput[AgentGame](t, owner, "start_game", matches.GameControl{ID: "mcp-start", Revision: status.Revision})
	if status.MatchStatus != "running" {
		t.Fatal("start did not create a running world")
	}
	one := mcpOutput[matches.ConnectionInfo](t, owner, "connect_player", struct{}{})
	two := mcpOutput[matches.ConnectionInfo](t, friend, "connect_player", struct{}{})
	mcpRefusal(t, owner, "heartbeat", AgentConnectionRequest{two.ID}, "unauthorized")
	mcpRefusal(t, owner, "leave_player", AgentConnectionRequest{two.ID}, "unauthorized")
	mcpOutput[AgentConnectionResult](t, owner, "heartbeat", AgentConnectionRequest{one.ID})
	mcpOutput[AgentConnectionResult](t, friend, "leave_player", AgentConnectionRequest{two.ID})
	base := "/api/v1/games/" + f.owner.MatchID
	info := decodeResponse[matches.GameInfo](t, request(t, f.server, "GET", base, f.owner.Token, nil), 200)
	if !info.Seats[0].Connected || info.Seats[1].Connected {
		t.Fatal("leaving another connection affected owner presence")
	}
	paused := mcpOutput[AgentGame](t, friend, "pause_game", matches.GameControl{ID: "agent-pause", Revision: status.Revision})
	mcpRefusal(t, owner, "resume_game", matches.GameControl{ID: "stale-resume", Revision: status.Revision}, "stale_revision")
	mcpRefusal(t, friend, "resume_game", matches.GameControl{ID: "foreign-resume", Revision: paused.Revision}, "forbidden")
	mcpRefusal(t, friend, "set_speed", matches.GameControl{ID: "foreign-speed", Revision: paused.Revision, Value: 32}, "forbidden")
	status = mcpOutput[AgentGame](t, owner, "resume_game", matches.GameControl{ID: "owner-resume", Revision: paused.Revision})
	if status.MatchStatus != "running" {
		t.Fatal("owner resume failed")
	}
	oldCredential := decodeResponse[AgentEndpoint](t, request(t, f.server, "POST", base+"/agent", f.friend.Token, nil), 200)
	newMember := decodeResponse[matches.MemberSession](t, request(t, f.server, "POST", "/api/v1/memberships/rejoin", "", matches.RejoinRequest{Code: f.friend.RejoinCode}), 200)
	path := playerMCPPath(f.friend.MatchID, f.friend.MembershipID)
	v := rawMCP(t, f.server, "POST", path, oldCredential.Token, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, nil)
	if v.Code != 401 {
		t.Fatalf("revoked token retained MCP access: %d", v.Code)
	}
	replacement := connectMCP(t, host, newMember)
	if got := mcpOutput[AgentGame](t, replacement, "game_status", struct{}{}); got.PlayerID != f.friend.PlayerID {
		t.Fatal("rejoin changed the endpoint's player")
	}
	decodeResponse[matches.GameInfo](t, request(t, f.server, "POST", base+"/close", f.owner.Token, matches.GameControl{ID: "close", Revision: status.Revision}), 200)
	mcpRefusal(t, owner, "observe", ObserveRequest{}, "game_unavailable")
	f.service.BeginShutdown()
	v = rawMCP(t, f.server, "POST", path, newMember.Token, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, nil)
	if v.Code != 503 || v.Header().Get("Retry-After") != "1" {
		t.Fatalf("shutdown failed to fence MCP: %d %s", v.Code, v.Body)
	}
}

func TestMCPDiscoveryStrictInputsAndBoundedReads(t *testing.T) {
	f := newMCPGame(t, "hidden")
	f.start(t)
	host := httptest.NewServer(f.server)
	t.Cleanup(host.Close)
	client := connectMCP(t, host, f.owner)
	listed, err := client.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	var commandSchema map[string]any
	for _, tool := range listed.Tools {
		names = append(names, tool.Name)
		if tool.Name == "command" {
			data, _ := json.Marshal(tool.InputSchema)
			_ = json.Unmarshal(data, &commandSchema)
		}
		if tool.OutputSchema == nil || tool.Annotations == nil {
			t.Fatal("tools need typed outputs and side-effect annotations", tool.Name)
		}
	}
	for _, name := range []string{"observe", "command", "check_placement", "map_region", "read_log", "catalog", "game_status", "start_game", "pause_game", "resume_game", "set_speed", "save_game", "connect_player", "heartbeat", "leave_player", "marketplace"} {
		if !slices.Contains(names, name) {
			t.Fatal("missing tool", name)
		}
	}
	if len(names) != 16 {
		t.Fatal("unexpected tools: audit their player authority", names)
	}
	catalog := mcpOutput[game.Catalog](t, client, "catalog", struct{}{})
	properties := commandSchema["properties"].(map[string]any)
	kinds := properties["kind"].(map[string]any)["enum"].([]any)
	if len(kinds) != len(catalog.Commands) || len(kinds) != 32 || commandSchema["additionalProperties"] != false {
		t.Fatal("command schema missing catalogue coverage or strict field validation")
	}
	for _, c := range catalog.Commands {
		if !slices.Contains(kinds, any(c.Kind)) {
			t.Fatal("command not exposed through MCP", c.Kind)
		}
	}
	before := f.view(t, f.owner)
	for _, args := range []map[string]any{
		{"id": "forged-player", "kind": "resign", "player_id": f.friend.PlayerID},
		{"id": "forged-owner", "kind": "resign", "owner": f.friend.PlayerID},
		{"id": "unknown-kind", "kind": "teleport"},
		{"id": "legacy-toggle", "kind": "pause"},
		{"id": "forged-position", "kind": "move", "entity_ids": []int{ownEntity(t, before, "villager").ID}, "position": map[string]any{"x": 1, "y": 1, "hp": 1000}},
	} {
		v, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: "command", Arguments: args})
		if err == nil && !v.IsError {
			t.Fatal("accepted forbidden argument", args)
		}
	}
	if !reflect.DeepEqual(before, f.view(t, f.owner)) {
		t.Fatal("invalid schema input changed gameplay")
	}
	mcpRefusal(t, client, "map_region", MapRegionRequest{Width: 33, Height: 1}, "invalid_arguments")
	mcpRefusal(t, client, "map_region", MapRegionRequest{X: before.Map.Width, Width: 1, Height: 1}, "invalid_arguments")
	mcpRefusal(t, client, "observe", ObserveRequest{Limit: 201}, "invalid_arguments")
	mcpRefusal(t, client, "read_log", map[string]any{"after": 0, "before": 1}, "invalid_arguments")
	mcpRefusal(t, client, "read_log", AgentLogRequest{Search: strings.Repeat("a", 201)}, "invalid_arguments")
	page := mcpOutput[Observation](t, client, "observe", ObserveRequest{Owner: &f.owner.PlayerID, Limit: 2})
	if !page.HasMore || len(page.Snapshot.Entities) != 2 || page.MapIncluded || len(page.Snapshot.Map.Tiles) != 0 {
		t.Fatal("observation not bounded", page)
	}
	next := mcpOutput[Observation](t, client, "observe", ObserveRequest{Owner: &f.owner.PlayerID, Limit: 2, AfterEntityID: page.NextEntityID})
	if len(next.Snapshot.Entities) == 0 || next.Snapshot.Entities[0].ID <= page.NextEntityID {
		t.Fatal("observation cursor overlapped")
	}
	position := ownEntity(t, before, "town_center").Position
	placement := matches.Placement{Product: "house", Position: position}
	preview := mcpOutput[matches.PlacementResult](t, client, "check_placement", placement)
	if preview.Valid || preview.Reason == "" || before.Player.Resources != f.view(t, f.owner).Player.Resources {
		t.Fatal("placement preview bypassed validation or charged resources")
	}
	// Discovery itself contains only the authenticated player's endpoint.
	document := OpenAPI()
	path := document["paths"].(map[string]any)["/games/{id}/memberships/{member}/mcp"].(map[string]any)["post"].(map[string]any)
	if fmt.Sprint(path["security"]) != "[map[mcpPlayerToken:[]]]" {
		t.Fatal("OpenAPI advertised cookie access to MCP", path["security"])
	}
}
