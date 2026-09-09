package httpapi

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"crowns/internal/game"
	"crowns/internal/matches"
)

// WireModels is the single source for OpenAPI schemas and browser DTO types.
// Simulation aggregates and state-machine instances must never be registered.
func WireModels() []reflect.Type {
	return []reflect.Type{reflect.TypeFor[AgentEndpoint](), reflect.TypeFor[AgentGame](), reflect.TypeFor[ObserveRequest](), reflect.TypeFor[Observation](), reflect.TypeFor[MapRegionRequest](), reflect.TypeFor[MapRegion](), reflect.TypeFor[AgentLogRequest](), reflect.TypeFor[AgentConnectionRequest](), reflect.TypeFor[AgentConnectionResult](), reflect.TypeFor[game.Config](), reflect.TypeFor[game.Command](), reflect.TypeFor[game.Snapshot](), reflect.TypeFor[game.SnapshotFrame](), reflect.TypeFor[game.EventPage](), reflect.TypeFor[game.Catalog](), reflect.TypeFor[matches.Session](), reflect.TypeFor[matches.SavedGames](), reflect.TypeFor[matches.ResumeRequest](), reflect.TypeFor[matches.Receipt](), reflect.TypeFor[matches.Placement](), reflect.TypeFor[matches.PlacementResult](), reflect.TypeFor[ErrorBody](), reflect.TypeFor[Health](), reflect.TypeFor[matches.CreateGame](), reflect.TypeFor[matches.MemberSession](), reflect.TypeFor[matches.GameLibrary](), reflect.TypeFor[matches.GameControl](), reflect.TypeFor[matches.SeatChange](), reflect.TypeFor[matches.ReadyRequest](), reflect.TypeFor[matches.RulesChange](), reflect.TypeFor[matches.InviteRequest](), reflect.TypeFor[matches.InviteSecret](), reflect.TypeFor[matches.ClaimInvite](), reflect.TypeFor[matches.RejoinRequest](), reflect.TypeFor[matches.Invitation](), reflect.TypeFor[matches.ConnectionInfo](), reflect.TypeFor[matches.AuditPage](), reflect.TypeFor[matches.TransferRequest](), reflect.TypeFor[matches.TransferInfo](), reflect.TypeFor[matches.ArchivePassword](), reflect.TypeFor[matches.ImportResult](), reflect.TypeFor[matches.CompleteTransfer]()}
}
func schemaTypes() map[string]reflect.Type {
	types := map[string]reflect.Type{}
	var visit func(reflect.Type)
	visit = func(t reflect.Type) {
		if t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice {
			visit(t.Elem())
			return
		}
		if t.Kind() != reflect.Struct {
			return
		}
		if _, ok := types[t.Name()]; ok {
			return
		}
		types[t.Name()] = t
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.IsExported() && f.Tag.Get("json") != "-" {
				visit(f.Type)
			}
		}
	}
	for _, t := range WireModels() {
		visit(t)
	}
	return types
}
func fieldName(f reflect.StructField) (string, bool) {
	tag := f.Tag.Get("json")
	if tag == "" {
		return f.Name, false
	}
	parts := strings.Split(tag, ",")
	return parts[0], len(parts) > 1 && parts[1] == "omitempty"
}
func jsonType(t reflect.Type) map[string]any {
	switch t.Kind() {
	case reflect.Pointer:
		return jsonType(t.Elem())
	case reflect.Struct:
		return map[string]any{"$ref": "#/components/schemas/" + t.Name()}
	case reflect.Slice:
		return map[string]any{"type": "array", "items": jsonType(t.Elem())}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Int, reflect.Int64:
		return map[string]any{"type": "integer"}
	default:
		return map[string]any{"type": "number"}
	}
}
func OpenAPI() map[string]any {
	schemas := map[string]any{}
	for name, t := range schemaTypes() {
		properties := map[string]any{}
		required := []string{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() || f.Tag.Get("json") == "-" {
				continue
			}
			key, optional := fieldName(f)
			properties[key] = jsonType(f.Type)
			if !optional {
				required = append(required, key)
			}
		}
		schemas[name] = map[string]any{"type": "object", "additionalProperties": false, "properties": properties, "required": required}
	}
	paths := map[string]any{}
	add := func(method, path, summary, input, output string, status int, auth bool) {
		operation := map[string]any{"summary": summary, "responses": map[string]any{fmt.Sprint(status): map[string]any{"description": "Success", "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/" + output}}}}, "default": map[string]any{"description": "Structured error", "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/ErrorBody"}}}}}}
		operation["responses"].(map[string]any)["503"] = map[string]any{
			"description": "Server unavailable: server_shutting_down during shutdown, or server_full at capacity. Reconnect after restart; retry uncertain commands with their original IDs.",
			"headers": map[string]any{"Retry-After": map[string]any{
				"description": "Suggested retry delay in seconds; included during shutdown.",
				"schema":      map[string]any{"type": "string", "example": "1"},
			}},
			"content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/ErrorBody"}}},
		}
		if auth {
			operation["security"] = []any{map[string]any{"matchToken": []string{}}}
			operation["parameters"] = []any{map[string]any{"name": "id", "in": "path", "required": true, "schema": map[string]any{"type": "string"}}}
		}
		if input != "" {
			operation["requestBody"] = map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/" + input}}}}
		}
		if output == "" {
			operation["responses"].(map[string]any)[fmt.Sprint(status)] = map[string]any{"description": "Success"}
		}
		if _, ok := paths[path]; !ok {
			paths[path] = map[string]any{}
		}
		paths[path].(map[string]any)[method] = operation
	}

	add("post", "/games", "Create a saved private lobby and owner membership", "CreateGame", "MemberSession", 201, false)
	add("get", "/games", "List only games bound to this browser's memberships", "", "GameLibrary", 200, false)
	add("get", "/games/{id}", "Read the lobby, roster, controls and save status", "", "GameInfo", 200, true)
	add("get", "/games/{id}/session", "Recover this browser's bound member session", "", "MemberSession", 200, true)
	add("get", "/games/{id}/agent", "Discover only your membership's MCP endpoint", "", "AgentEndpoint", 200, true)
	add("post", "/games/{id}/agent", "Get your membership's restricted MCP credential; cannot authorize REST or host administration", "", "AgentEndpoint", 200, true)
	add("get", "/games/{id}/snapshot", "Read this member's authorized world view", "", "Snapshot", 200, true)
	add("patch", "/games/{id}/rules", "Update lobby settings and clear readiness", "RulesChange", "GameInfo", 200, true)
	add("post", "/games/{id}/seats", "Add a human or AI seat in the lobby", "SeatChange", "GameInfo", 200, true)
	add("patch", "/games/{id}/seats/{seat}", "Configure a lobby seat", "SeatChange", "GameInfo", 200, true)
	add("put", "/games/{id}/seats/{seat}/ready", "Set your readiness for the current roster", "ReadyRequest", "GameInfo", 200, true)
	add("post", "/games/{id}/seats/{seat}/invites", "Issue a private, single-use seat invitation", "InviteRequest", "Invitation", 200, true)
	add("delete", "/games/{id}/invites/{invite}", "Revoke a pending invitation", "GameControl", "GameInfo", 200, true)
	add("post", "/invites/inspect", "Preview an invitation without claiming it", "InviteSecret", "Invitation", 200, false)
	add("post", "/invites/claim", "Atomically claim a human seat", "ClaimInvite", "MemberSession", 200, false)
	add("post", "/memberships/rejoin", "Recover a membership with its private rejoin code", "RejoinRequest", "MemberSession", 200, false)
	for _, action := range []string{"start", "pause", "resume", "speed", "close", "reopen", "cancel-close", "cancel-transfer"} {
		add("post", "/games/{id}/"+action, "Explicit shared game control: "+action, "GameControl", "GameInfo", 200, true)
	}
	add("post", "/games/{id}/save", "Commit a checkpoint, journal and receipts", "", "GameInfo", 200, true)
	add("delete", "/games/{id}", "Owner-confirmed permanent deletion with an autosave fence", "GameControl", "", 204, true)
	add("post", "/games/{id}/commands", "Submit an intention as your authenticated player", "Command", "Receipt", 200, true)
	add("post", "/games/{id}/placement", "Validate placement as your authenticated player", "Placement", "PlacementResult", 200, true)
	add("get", "/games/{id}/events", "Stream your fog-filtered world; requires a live connection", "", "Snapshot", 200, true)
	add("get", "/games/{id}/log", "Read the journal visible to your kingdom", "", "EventPage", 200, true)
	add("get", "/games/{id}/entities/{entity}/history", "Read an entity's observable history", "", "EventPage", 200, true)
	add("get", "/games/{id}/audit", "Read shared session lifecycle events after an audit ID", "", "AuditPage", 200, true)
	add("post", "/games/{id}/connections", "Open a browser connection without resuming play", "", "ConnectionInfo", 201, true)
	for _, action := range []string{"heartbeat", "leave"} {
		add("post", "/games/{id}/connections/{connection}/"+action, "Update this member's browser connection: "+action, "", "", 204, true)
	}
	add("post", "/games/{id}/transfers", "Freeze and save a game-scoped portable archive", "TransferRequest", "TransferInfo", 200, true)
	add("post", "/games/{id}/database", "Owner-only standalone SQLite snapshot including private game state and recovery credentials", "", "", 200, true)
	paths["/games/{id}/database"].(map[string]any)["post"].(map[string]any)["responses"].(map[string]any)["200"] = map[string]any{"description": "Self-contained SQLite game database", "content": map[string]any{"application/vnd.sqlite3": map[string]any{"schema": map[string]any{"type": "string", "format": "binary"}}}}
	add("post", "/games/{id}/transfers/complete", "Retire the source using a destination completion receipt", "CompleteTransfer", "GameInfo", 200, true)
	for _, method := range []string{"get", "post"} {
		input := ""
		if method == "post" {
			input = "ArchivePassword"
		}
		add(method, "/games/{id}/transfers/{transfer}/archive", "Download a saved archive; POST supports passphrase protection", input, "", 200, true)
		paths["/games/{id}/transfers/{transfer}/archive"].(map[string]any)[method].(map[string]any)["responses"].(map[string]any)["200"] = map[string]any{"description": "Portable .aoegame archive", "content": map[string]any{"application/octet-stream": map[string]any{"schema": map[string]any{"type": "string", "format": "binary"}}}}
	}
	add("post", "/game-imports", "Import a move or copy after proving archive ownership", "", "ImportResult", 201, false)
	paths["/game-imports"].(map[string]any)["post"].(map[string]any)["requestBody"] = map[string]any{"required": true, "content": map[string]any{"multipart/form-data": map[string]any{"schema": map[string]any{"type": "object", "required": []string{"archive", "rejoin_code"}, "properties": map[string]any{"archive": map[string]any{"type": "string", "format": "binary"}, "passphrase": map[string]any{"type": "string", "maxLength": 256}, "rejoin_code": map[string]any{"type": "string"}, "copy": map[string]any{"type": "boolean"}, "name": map[string]any{"type": "string", "maxLength": 80}}}}}}
	add("get", "/health", "Check server health", "", "Health", 200, false)
	add("get", "/catalog", "Read display metadata and ruleset catalog", "", "Catalog", 200, false)
	add("post", "/matches/{id}/adopt", "Upgrade an existing game using its original bearer token", "", "MemberSession", 200, true)
	add("post", "/matches", "Create a single-player match against server AI", "Config", "Session", 201, false)
	add("get", "/sessions", "Retired: use the authenticated /games library", "", "SavedGames", 200, false)
	paths["/sessions"].(map[string]any)["get"].(map[string]any)["parameters"] = []any{map[string]any{"name": "q", "in": "query", "description": "Case-insensitive substring of a game name or session ID", "schema": map[string]any{"type": "string", "maxLength": 200}}}
	add("post", "/sessions/resume", "Retired: use a private membership rejoin code", "ResumeRequest", "Session", 200, false)
	add("get", "/matches/{id}/session", "Read game name and last successful autosave", "", "SavedGame", 200, true)
	add("post", "/matches/{id}/save", "Atomically checkpoint the simulation, receipts and history", "", "SavedGame", 200, true)
	add("post", "/matches/{id}/leave", "Checkpoint and unload a session until it is resumed", "", "SavedGame", 200, true)
	add("get", "/matches/{id}", "Read your fog-filtered snapshot", "", "Snapshot", 200, true)
	add("delete", "/matches/{id}", "Permanently delete a game and its saved history", "", "", 204, true)
	add("post", "/matches/{id}/commands", "Submit an idempotent intention; the server validates all rules", "Command", "Receipt", 200, true)
	add("post", "/matches/{id}/placement", "Check a proposed building site on the server", "Placement", "PlacementResult", 200, true)
	add("get", "/matches/{id}/events", "Stream complete fog-filtered snapshots; reconnect replaces state", "", "Snapshot", 200, true)
	for _, path := range []string{"/matches/{id}/log", "/matches/{id}/entities/{entity}/history"} {
		add("get", path, "Read immutable events visible when they occurred; ascending IDs, newest page by default", "", "EventPage", 200, true)
		operation := paths[path].(map[string]any)["get"].(map[string]any)
		parameters := operation["parameters"].([]any)
		if strings.Contains(path, "{entity}") {
			parameters = append(parameters, map[string]any{"name": "entity", "in": "path", "required": true, "schema": map[string]any{"type": "integer", "minimum": 1}})
		}
		for _, name := range []string{"after", "before", "limit"} {
			minimum := 1
			if name == "after" {
				minimum = 0
			}
			schema := map[string]any{"type": "integer", "minimum": minimum}
			if name == "limit" {
				schema["maximum"], schema["default"] = 200, 100
			}
			parameters = append(parameters, map[string]any{"name": name, "in": "query", "description": "after and before are exclusive cursors and mutually exclusive; after=0 starts at the beginning", "schema": schema})
		}
		categories := []string{}
		for _, filter := range game.GetCatalog().LogFilters {
			categories = append(categories, filter.ID)
		}
		parameters = append(parameters,
			map[string]any{"name": "q", "in": "query", "description": "Case-insensitive search of all retained authorized events; all words must match. Identity forms Villager 3, #3, or a lone 3 match the exact entity ID. Other numbers search event text.", "schema": map[string]any{"type": "string", "maxLength": 200}},
			map[string]any{"name": "category", "in": "query", "description": "Combine a catalog event category with search and pagination. Cursors and availability refer to matching events.", "schema": map[string]any{"type": "string", "enum": categories}},
		)
		operation["parameters"] = parameters
	}
	op := paths["/matches/{id}/events"].(map[string]any)["get"].(map[string]any)
	op["responses"].(map[string]any)["200"] = map[string]any{"description": "SSE event `snapshot`, id = simulation tick; JSON data conforms to Snapshot. Bearer auth through fetch streaming.", "content": map[string]any{"text/event-stream": map[string]any{"schema": map[string]any{"type": "string"}}}}

	for path, raw := range paths {
		if !strings.HasPrefix(path, "/games") {
			continue
		}
		for method, operation := range raw.(map[string]any) {
			op := operation.(map[string]any)
			if _, ok := op["security"]; ok || (path == "/games" && method == "get") {
				op["security"] = []any{map[string]any{"matchToken": []string{}}, map[string]any{"browserMembership": []string{}}}
			}
			parameters, _ := op["parameters"].([]any)
			if path == "/games/{id}/log" || strings.HasSuffix(path, "/history") {
				for _, raw := range paths["/matches/{id}/log"].(map[string]any)["get"].(map[string]any)["parameters"].([]any) {
					p := raw.(map[string]any)
					if p["in"] == "query" {
						parameters = append(parameters, p)
					}
				}
			}
			if path == "/games/{id}/audit" {
				parameters = append(parameters, map[string]any{"name": "after", "in": "query", "schema": map[string]any{"type": "integer", "minimum": 0}})
			}
			if path == "/games" && method == "get" {
				parameters = append(parameters, map[string]any{"name": "q", "in": "query", "schema": map[string]any{"type": "string", "maxLength": 200}})
			}
			for _, name := range []string{"seat", "invite", "connection", "transfer", "entity"} {
				if strings.Contains(path, "{"+name+"}") {
					parameters = append(parameters, map[string]any{"name": name, "in": "path", "required": true, "schema": map[string]any{"type": "string"}})
				}
			}
			if path == "/games/{id}/events" {
				parameters = append(parameters, map[string]any{"name": "connection", "in": "query", "required": true, "schema": map[string]any{"type": "string"}})
				op["responses"].(map[string]any)["200"] = map[string]any{"description": "SSE snapshot frames; fresh authenticated state on reconnect", "content": map[string]any{"text/event-stream": map[string]any{"schema": map[string]any{"type": "string"}}}}
			}
			if len(parameters) > 0 {
				op["parameters"] = parameters
			} else {
				delete(op, "parameters")
			}
		}
	}
	for _, path := range []string{"/matches/{id}/events", "/games/{id}/events"} {
		op := paths[path].(map[string]any)["get"].(map[string]any)
		parameters, _ := op["parameters"].([]any)
		op["parameters"] = append(parameters, map[string]any{"name": "format", "in": "query", "description": "Omit for 10 Hz complete snapshots. delta-v1 sends 20 Hz SnapshotFrame observations: a full snapshot first, then entity replacements/removals and changed map cells. base must equal the last sequence; reconnect on a gap. sample_ms is monotonic wall time within this connection, independent of game speed. No historical replay or unobserved positions.", "schema": map[string]any{"type": "string", "enum": []string{"delta-v1"}}})
		op["responses"].(map[string]any)["200"] = map[string]any{"description": "SSE event snapshot (Snapshot, id=tick) by default; event frame (SnapshotFrame, id=sequence) for format=delta-v1. Every reconnect starts with a fresh, authenticated full snapshot. Authorization and fog filtering apply before differencing.", "content": map[string]any{"text/event-stream": map[string]any{"schema": map[string]any{"type": "string"}}}}
	}
	paths["/games/{id}/memberships/{member}/mcp"] = playerMCPContract()
	return map[string]any{"openapi": "3.1.0", "info": map[string]any{"title": "AI of Empires API", "version": "1.0.0", "description": "Go-authoritative RTS. Commands are intent; snapshots are authorized presentation state."}, "servers": []any{map[string]any{"url": "/api/v1"}}, "paths": paths, "components": map[string]any{"schemas": schemas, "securitySchemes": map[string]any{"browserMembership": map[string]any{"type": "apiKey", "in": "cookie", "name": "aoe_browser"}, "matchToken": map[string]any{"type": "http", "scheme": "bearer"}, "mcpPlayerToken": map[string]any{"type": "http", "scheme": "bearer", "description": "Restricted player credential from POST /games/{id}/agent. Valid only at that membership MCP endpoint; cannot authorize REST."}}}}
}
func tsType(t reflect.Type) string {
	switch t.Kind() {
	case reflect.Pointer:
		return tsType(t.Elem())
	case reflect.Struct:
		return t.Name()
	case reflect.Slice:
		return tsType(t.Elem()) + "[]"
	case reflect.Bool:
		return "boolean"
	case reflect.String:
		return "string"
	default:
		return "number"
	}
}
func TypeScript() string {
	var b strings.Builder
	b.WriteString("// Code generated by go run ./cmd/contracts. DO NOT EDIT.\n\n")
	types := schemaTypes()
	names := []string{}
	for name := range types {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		t := types[name]
		fmt.Fprintf(&b, "export interface %s {\n", name)
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() || f.Tag.Get("json") == "-" {
				continue
			}
			key, optional := fieldName(f)
			suffix := ""
			if optional {
				suffix = "?"
			}
			fmt.Fprintf(&b, "  %s%s: %s;\n", key, suffix, tsType(f.Type))
		}
		b.WriteString("}\n\n")
	}
	return b.String()
}
