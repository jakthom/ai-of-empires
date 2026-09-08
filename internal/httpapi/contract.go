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
	return []reflect.Type{reflect.TypeFor[game.Config](), reflect.TypeFor[game.Command](), reflect.TypeFor[game.Snapshot](), reflect.TypeFor[game.EventPage](), reflect.TypeFor[game.Catalog](), reflect.TypeFor[matches.Session](), reflect.TypeFor[matches.SavedGames](), reflect.TypeFor[matches.ResumeRequest](), reflect.TypeFor[matches.Receipt](), reflect.TypeFor[matches.Placement](), reflect.TypeFor[matches.PlacementResult](), reflect.TypeFor[ErrorBody](), reflect.TypeFor[Health]()}
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
	add("get", "/health", "Check server health", "", "Health", 200, false)
	add("get", "/catalog", "Read display metadata and ruleset catalog", "", "Catalog", 200, false)
	add("post", "/matches", "Create a single-player match against server AI", "Config", "Session", 201, false)
	add("get", "/sessions", "List up to 100 local saved games, newest save first", "", "SavedGames", 200, false)
	paths["/sessions"].(map[string]any)["get"].(map[string]any)["parameters"] = []any{map[string]any{"name": "q", "in": "query", "description": "Case-insensitive substring of a game name or session ID", "schema": map[string]any{"type": "string", "maxLength": 200}}}
	add("post", "/sessions/resume", "Resume a local game by exact name or ID; rotates its bearer token", "ResumeRequest", "Session", 200, false)
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
	return map[string]any{"openapi": "3.1.0", "info": map[string]any{"title": "AI of Empires API", "version": "1.0.0", "description": "Go-authoritative RTS. Commands are intent; snapshots are authorized presentation state."}, "servers": []any{map[string]any{"url": "/api/v1"}}, "paths": paths, "components": map[string]any{"schemas": schemas, "securitySchemes": map[string]any{"matchToken": map[string]any{"type": "http", "scheme": "bearer"}}}}
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
