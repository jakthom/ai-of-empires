package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"unicode/utf8"

	"crowns/internal/game"
	"crowns/internal/matches"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type playerAccessKey struct{}

func playerMCPContract() map[string]any {
	envelope := map[string]any{"type": "object", "additionalProperties": true, "description": "MCP JSON-RPC envelope. Discover typed tool input/output schemas with tools/list."}
	return map[string]any{"post": map[string]any{
		"summary":     "Player-specific MCP Streamable HTTP endpoint",
		"description": "Every request requires a restricted MCP bearer token matching both the game and membership in this URL. Obtain it with POST /games/{id}/agent. Ordinary session tokens, browser cookies and MCP session IDs do not grant access. Stateless JSON responses; tool schemas derive from Go wire types. No host administration, foreign history or hidden-world access.",
		"security":    []any{map[string]any{"mcpPlayerToken": []string{}}},
		"parameters": []any{
			map[string]any{"name": "id", "in": "path", "required": true, "schema": map[string]any{"type": "string"}},
			map[string]any{"name": "member", "in": "path", "required": true, "schema": map[string]any{"type": "string"}},
			map[string]any{"name": "Accept", "in": "header", "required": true, "description": "Must accept both application/json and text/event-stream", "schema": map[string]any{"type": "string", "example": "application/json, text/event-stream"}},
		},
		"requestBody": map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": envelope}}},
		"responses": map[string]any{
			"200":     map[string]any{"description": "MCP result or protocol error. Gameplay refusals use isError with the existing JSON RuleError in text content.", "content": map[string]any{"application/json": map[string]any{"schema": envelope}}},
			"202":     map[string]any{"description": "MCP notification accepted"},
			"default": map[string]any{"description": "Authentication, origin, size, protocol or shutdown refusal"},
		},
	}}
}

// AgentEndpoint is discovered through the ordinary authenticated REST API.
// URLs identify memberships; only the matching bearer credential grants access.
type AgentEndpoint struct {
	GameID       string `json:"game_id"`
	MembershipID string `json:"membership_id"`
	PlayerID     int    `json:"player_id"`
	MCPURL       string `json:"mcp_url"`
	Token        string `json:"token,omitempty"`
}

func playerMCPPath(gameID, memberID string) string {
	return "/api/v1/games/" + gameID + "/memberships/" + memberID + "/mcp"
}

func (s *Server) mcpRoutes() {
	server := newPlayerMCP()
	transport := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{
		Stateless: true, JSONResponse: true, MaxRequestBodyBytes: 32 << 10,
	})
	s.mux.HandleFunc("GET /api/v1/games/{id}/agent", func(w http.ResponseWriter, r *http.Request) {
		if a := s.gameAccess(w, r); a != nil {
			defer a.Release()
			member, err := a.Session()
			if err != nil {
				domainError(w, err)
				return
			}
			respond(w, 200, AgentEndpoint{GameID: member.MatchID, MembershipID: member.MembershipID, PlayerID: member.PlayerID, MCPURL: playerMCPPath(member.MatchID, member.MembershipID)})
		}
	})
	s.mux.HandleFunc("POST /api/v1/games/{id}/agent", func(w http.ResponseWriter, r *http.Request) {
		if a := s.gameAccess(w, r); a != nil {
			defer a.Release()
			credential, err := a.AgentCredential()
			if err != nil {
				domainError(w, err)
				return
			}
			respond(w, 200, AgentEndpoint{GameID: credential.GameID, MembershipID: credential.MembershipID, PlayerID: credential.PlayerID, MCPURL: playerMCPPath(credential.GameID, credential.MembershipID), Token: credential.Token})
		}
	})
	s.mux.HandleFunc("/api/v1/games/{id}/memberships/{member}/mcp", func(w http.ResponseWriter, r *http.Request) {
		// Cookies, URL secrets, MCP session IDs and tool arguments are never
		// credentials here. Authenticate even discovery/initialization requests.
		token := bearer(r)
		if token == "" {
			w.Header().Set("WWW-Authenticate", `Bearer realm="game-player"`)
			domainError(w, matches.ErrUnauthorized)
			return
		}
		a, err := s.matches.RequestAgentAccess(r.PathValue("id"), r.PathValue("member"), token)
		if err != nil {
			domainError(w, err)
			return
		}
		defer a.Release()
		member, err := a.Session()
		if err != nil {
			domainError(w, err)
			return
		}
		if member.MembershipID != r.PathValue("member") {
			domainError(w, matches.ErrForbidden)
			return
		}
		// This access exists only for this HTTP request. There is no cached
		// player, shared snapshot baseline or cross-player MCP session state.
		transport.ServeHTTP(privateMCPResponse{w}, r.WithContext(context.WithValue(r.Context(), playerAccessKey{}, a)))
	})
}

// The SDK sets its own cache policy; keep private payloads out of caches even
// for protocol discovery and errors. Unwrap preserves ResponseController use.
type privateMCPResponse struct{ http.ResponseWriter }

func (w privateMCPResponse) WriteHeader(status int) {
	w.Header().Set("Cache-Control", "no-store")
	w.ResponseWriter.WriteHeader(status)
}

func (w privateMCPResponse) Write(p []byte) (int, error) {
	w.Header().Set("Cache-Control", "no-store")
	return w.ResponseWriter.Write(p)
}

func (w privateMCPResponse) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// AgentGame deliberately omits the world seed, other membership IDs and host
// administration details. Visible opponents come from the authorized snapshot.
type AgentGame struct {
	GameID      string  `json:"game_id"`
	Name        string  `json:"name"`
	PlayerID    int     `json:"player_id"`
	SeatID      string  `json:"seat_id"`
	Owner       bool    `json:"owner"`
	Status      string  `json:"status"`
	MatchStatus string  `json:"match_status"`
	Revision    int     `json:"revision"`
	Time        float64 `json:"time"`
}

func agentGame(v matches.GameInfo) AgentGame {
	out := AgentGame{GameID: v.GameID, Name: v.Name, PlayerID: v.PlayerID, Owner: v.Owner, Status: v.Status, MatchStatus: v.MatchStatus, Revision: v.Revision, Time: v.Time}
	for _, seat := range v.Seats {
		if seat.Yours {
			out.SeatID = seat.ID
		}
	}
	return out
}

type ObserveRequest struct {
	Owner         *int  `json:"owner,omitempty" jsonschema:"Optional entity owner filter: 0 for neutral resources, your player_id for your kingdom. Filtering never grants additional visibility."`
	EntityIDs     []int `json:"entity_ids,omitempty" jsonschema:"Optional subset of at most 200 entity IDs from observations."`
	AfterEntityID int   `json:"after_entity_id,omitempty" jsonschema:"Return entity IDs greater than this cursor. Each page is a fresh observation."`
	Limit         int   `json:"limit,omitempty" jsonschema:"Entities per page: 1 to 200, default 100."`
	IncludeMap    bool  `json:"include_map,omitempty" jsonschema:"Include the complete fog-filtered tile arrays. Default false; use map_region for bounded terrain reads."`
}

type Observation struct {
	Snapshot     game.Snapshot `json:"snapshot"`
	MapIncluded  bool          `json:"map_included"`
	HasMore      bool          `json:"has_more"`
	NextEntityID int           `json:"next_entity_id"`
}

func observe(a *matches.Access, q ObserveRequest) (Observation, error) {
	if q.Limit < 0 || q.Limit > 200 || q.AfterEntityID < 0 || len(q.EntityIDs) > 200 || q.Owner != nil && *q.Owner < 0 {
		return Observation{}, agentInputError("Use a nonnegative entity cursor, up to 200 entity IDs and a limit of 1–200.")
	}
	for _, id := range q.EntityIDs {
		if id <= 0 {
			return Observation{}, agentInputError("Entity IDs must be positive.")
		}
	}
	if q.Limit == 0 {
		q.Limit = 100
	}
	v, err := a.View()
	if err != nil {
		return Observation{}, err
	}
	v.Entities = slices.DeleteFunc(v.Entities, func(e game.EntityView) bool {
		return e.ID <= q.AfterEntityID || q.Owner != nil && e.Owner != *q.Owner || len(q.EntityIDs) > 0 && !slices.Contains(q.EntityIDs, e.ID)
	})
	slices.SortFunc(v.Entities, func(a, b game.EntityView) int { return a.ID - b.ID })
	out := Observation{Snapshot: v, MapIncluded: q.IncludeMap, HasMore: len(v.Entities) > q.Limit}
	if out.HasMore {
		out.Snapshot.Entities = v.Entities[:q.Limit]
		out.NextEntityID = out.Snapshot.Entities[q.Limit-1].ID
	}
	if !q.IncludeMap {
		out.Snapshot.Map.Tiles, out.Snapshot.Map.Fog = []game.Tile{}, []int{}
	}
	return out, nil
}

type MapRegionRequest struct {
	X      int `json:"x" jsonschema:"Left map tile coordinate, starting at 0."`
	Y      int `json:"y" jsonschema:"Top map tile coordinate, starting at 0."`
	Width  int `json:"width" jsonschema:"Number of columns, 1–32."`
	Height int `json:"height" jsonschema:"Number of rows, 1–32."`
}

type MapRegion struct {
	Tick      int         `json:"tick"`
	MapWidth  int         `json:"map_width"`
	MapHeight int         `json:"map_height"`
	X         int         `json:"x"`
	Y         int         `json:"y"`
	Width     int         `json:"width"`
	Height    int         `json:"height"`
	Tiles     []game.Tile `json:"tiles"`
	Fog       []int       `json:"fog"`
}

func mapRegion(a *matches.Access, q MapRegionRequest) (MapRegion, error) {
	if q.X < 0 || q.Y < 0 || q.Width < 1 || q.Width > 32 || q.Height < 1 || q.Height > 32 {
		return MapRegion{}, agentInputError("Choose nonnegative tile coordinates and a region from 1×1 to 32×32.")
	}
	v, err := a.View()
	if err != nil {
		return MapRegion{}, err
	}
	if q.X >= v.Map.Width || q.Y >= v.Map.Height || q.Width > v.Map.Width-q.X || q.Height > v.Map.Height-q.Y {
		return MapRegion{}, agentInputError("Choose a region entirely inside the map.")
	}
	out := MapRegion{Tick: v.Tick, MapWidth: v.Map.Width, MapHeight: v.Map.Height, X: q.X, Y: q.Y, Width: q.Width, Height: q.Height, Tiles: []game.Tile{}, Fog: []int{}}
	for y := q.Y; y < q.Y+q.Height; y++ {
		start := y*v.Map.Width + q.X
		out.Tiles = append(out.Tiles, v.Map.Tiles[start:start+q.Width]...)
		out.Fog = append(out.Fog, v.Map.Fog[start:start+q.Width]...)
	}
	return out, nil
}

type AgentLogRequest struct {
	After    *int   `json:"after,omitempty" jsonschema:"Exclusive event cursor; 0 starts at the beginning. Mutually exclusive with before."`
	Before   *int   `json:"before,omitempty" jsonschema:"Exclusive older-page cursor, at least 1."`
	Limit    int    `json:"limit,omitempty" jsonschema:"Page size from 1–200, default 100."`
	EntityID int    `json:"entity_id,omitempty" jsonschema:"Optional positive entity ID for your personal history of that entity."`
	Search   string `json:"q,omitempty" jsonschema:"Search up to 200 characters in your retained history."`
	Category string `json:"category,omitempty" jsonschema:"Category from catalog.log_filters; empty means all."`
}

func agentLog(a *matches.Access, q AgentLogRequest) (game.EventPage, error) {
	if q.After != nil && q.Before != nil || q.After != nil && *q.After < 0 || q.Before != nil && *q.Before < 1 || q.Limit < 0 || q.Limit > 200 || q.EntityID < 0 {
		return game.EventPage{}, agentInputError("Use either after (0 or greater) or before (1 or greater), a positive entity ID and a limit of 1–200.")
	}
	if !utf8.ValidString(q.Search) || utf8.RuneCountInString(q.Search) > 200 {
		return game.EventPage{}, agentInputError("Search must be at most 200 characters.")
	}
	query := game.LogQuery{Limit: q.Limit, EntityID: q.EntityID, Search: q.Search, Category: q.Category}
	if q.After != nil {
		query.After, query.FromStart = *q.After, true
	}
	if q.Before != nil {
		query.Before = *q.Before
	}
	return a.Log(query)
}

type AgentConnectionRequest struct {
	ConnectionID string `json:"connection_id" jsonschema:"Your connection ID from connect_player. This ID grants no authority by itself."`
}

type AgentConnectionResult struct {
	OK bool `json:"ok"`
}

func agentInputError(message string) error {
	return &game.RuleError{Code: "invalid_arguments", Message: message}
}

func playerTool[In, Out any](s *mcp.Server, t *mcp.Tool, fn func(*matches.Access, In) (Out, error)) {
	mcp.AddTool(s, t, func(ctx context.Context, _ *mcp.CallToolRequest, q In) (*mcp.CallToolResult, Out, error) {
		var zero Out
		a, ok := ctx.Value(playerAccessKey{}).(*matches.Access)
		if !ok {
			return nil, zero, matches.ErrUnauthorized
		}
		out, err := fn(a, q)
		if err != nil {
			_, body := domainFailure(err)
			data, _ := json.Marshal(body)
			return nil, zero, errors.New(string(data))
		}
		return nil, out, nil
	})
}

func newPlayerMCP() *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "ai-of-empires-player", Version: "1.0.0"}, &mcp.ServerOptions{
		Instructions: "You control only the player bound to this endpoint and bearer credential. Read game_status, catalog and observe before acting. Use map-space X/Y and entity IDs from your observations. Fog hides enemy units; remembered buildings have visible=false and may be stale. Only your own actions, queues and journal are private knowledge. Read enabled actions, costs and refusal reasons from Go. command IDs must be unique per intention (1–64 characters); retry ambiguous requests with the identical ID and payload. Receipts acknowledge acceptance, not completion. Pause/resume/speed are explicit shared controls using the observed revision. Simulation time advances independently of tool calls. Keep a human browser connected, or have your client maintain connect_player/heartbeat independently of model reasoning (every 4 seconds). Never infer hidden state from missing IDs. Chat, prompts and agent reasoning are not shared by this server.",
		Capabilities: &mcp.ServerCapabilities{Tools: &mcp.ToolCapabilities{}},
	})
	tool := func(name, description string, readOnly, idempotent bool) *mcp.Tool {
		closedWorld := false
		return &mcp.Tool{Name: name, Description: description, Annotations: &mcp.ToolAnnotations{ReadOnlyHint: readOnly, IdempotentHint: idempotent, OpenWorldHint: &closedWorld}}
	}
	playerTool(s, tool("game_status", "Read your player identity, own seat, lobby/match status and shared control revision. Does not reveal the world seed or other memberships.", true, true), func(a *matches.Access, _ struct{}) (AgentGame, error) {
		v, err := a.Info()
		return agentGame(v), err
	})
	playerTool(s, tool("catalog", "Discover all supported command kinds and their fields, units, buildings, technologies, civilizations, world options and log filters. Current prices and availability come from observations.", true, true), func(_ *matches.Access, _ struct{}) (game.Catalog, error) { return game.GetCatalog(), nil })
	playerTool(s, tool("observe", "Read your authorized snapshot with paged entities (default 100). Use owner=your player_id to find your units. Keep filters when paging with next_entity_id. Map dimensions are always included; tile/fog arrays are empty unless include_map=true. Read-only; never ticks the simulation.", true, true), observe)
	playerTool(s, tool("marketplace", "Read the ongoing buy/sell/barter book, finite merchant stock and quotes, and your caravan deliveries. Private offers appear only for their participants. Use catalog's market_post/accept/cancel/resume/recall commands to trade. Scout the offering Market; payment and goods travel with a vulnerable cart. Prices and availability come from Go.", true, true), func(a *matches.Access, _ struct{}) (game.MarketplaceView, error) {
		v, err := a.View()
		return v.Marketplace, err
	})
	playerTool(s, tool("map_region", "Read a bounded rectangle from your fog-filtered map. Row-major tiles/fog; fog 0=unknown, 1=explored, 2=visible. Unknown cells contain no hidden terrain. Placement must still be checked by Go.", true, true), mapRegion)
	commandTool := tool("command", "Issue any supported gameplay intention as your authenticated player. Consult catalog.commands for fields and observe for available actions. Includes construction, economy, combat, production, monks, naval transport and resign. A unique ID makes identical retries apply at most once; reuse with different content is rejected. No player/owner override is accepted.", false, true)
	schema, err := jsonschema.For[game.Command](nil)
	if err != nil {
		panic(err)
	}
	for _, c := range game.PlayerCommands() {
		schema.Properties["kind"].Enum = append(schema.Properties["kind"].Enum, c.Kind)
	}
	commandTool.InputSchema = schema
	playerTool(s, commandTool, (*matches.Access).Apply)
	playerTool(s, tool("check_placement", "Ask Go to validate and price a building or wall route on explored terrain. This does not charge resources or place a building. command revalidates before committing.", true, true), (*matches.Access).Placement)
	playerTool(s, tool("read_log", "Read/search your private event log or entity history, including command outcomes. Seeing an enemy never grants access to their private history. Follow has_newer with newer_cursor to catch up.", true, true), agentLog)
	for _, control := range []struct {
		name, description string
		apply             func(*matches.Access, matches.GameControl) (matches.GameInfo, error)
	}{
		{"start_game", "Owner only: start the configured lobby using id and the observed revision.", (*matches.Access).Start},
		{"pause_game", "Pause the shared clock. Any member can pause; use id and the observed revision.", (*matches.Access).Pause},
		{"resume_game", "Owner only: resume using id and the current revision. A stale revision cannot undo a newer pause.", (*matches.Access).ResumeGame},
		{"set_speed", "Owner only: change shared speed using id, revision and value from catalog.speeds.", (*matches.Access).Speed},
	} {
		playerTool(s, tool(control.name, control.description, false, true), func(a *matches.Access, q matches.GameControl) (AgentGame, error) {
			v, err := control.apply(a, q)
			return agentGame(v), err
		})
	}
	playerTool(s, tool("save_game", "Checkpoint the game through your membership without changing the clock.", false, true), func(a *matches.Access, _ struct{}) (AgentGame, error) {
		v, err := a.SaveGame()
		return agentGame(v), err
	})
	playerTool(s, tool("connect_player", "Open a presence connection without starting or resuming play. Your client must heartbeat every 4 seconds independently of reasoning; expires after 12 seconds. Reuse the returned connection_id.", false, false), func(a *matches.Access, _ struct{}) (matches.ConnectionInfo, error) { return a.Connect() })
	playerTool(s, tool("heartbeat", "Refresh only your presence connection. Expired/revoked membership and foreign connection IDs are rejected.", false, false), func(a *matches.Access, q AgentConnectionRequest) (AgentConnectionResult, error) {
		err := a.Heartbeat(q.ConnectionID)
		return AgentConnectionResult{err == nil}, err
	})
	playerTool(s, tool("leave_player", "Disconnect only your presence connection. Existing shared disconnect/pause policy applies; other connections remain active.", false, true), func(a *matches.Access, q AgentConnectionRequest) (AgentConnectionResult, error) {
		err := a.Disconnect(q.ConnectionID)
		return AgentConnectionResult{err == nil}, err
	})
	return s
}
