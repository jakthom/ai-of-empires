// Package httpapi is the transport boundary. It decodes intentions, authenticates
// match seats and returns domain read models; no gameplay rules live here.
package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"crowns/internal/game"
	"crowns/internal/matches"

	"github.com/open-ships/statemachine"
)

type ErrorBody struct {
	Error game.RuleError `json:"error"`
}
type Health struct {
	Status       string `json:"status"`
	RulesVersion string `json:"rules_version"`
}
type Server struct {
	matches *matches.Service
	mux     *http.ServeMux
	assets  fs.FS
}

func New(service *matches.Service, assets fs.FS) *Server {
	s := &Server{matches: service, mux: http.NewServeMux(), assets: assets}
	s.gameRoutes()
	s.mux.HandleFunc("GET /api/v1/health", func(w http.ResponseWriter, r *http.Request) { respond(w, 200, Health{"ok", game.RulesVersion}) })
	s.mux.HandleFunc("GET /api/v1/catalog", func(w http.ResponseWriter, r *http.Request) { respond(w, 200, game.GetCatalog()) })
	s.mux.HandleFunc("GET /api/v1/openapi.json", func(w http.ResponseWriter, r *http.Request) { respond(w, 200, OpenAPI()) })
	s.mux.HandleFunc("GET /api/docs", s.docs)
	s.mux.HandleFunc("POST /api/v1/matches", s.create)
	s.mux.HandleFunc("GET /api/v1/sessions", s.sessions)
	s.mux.HandleFunc("POST /api/v1/sessions/resume", s.resume)
	s.mux.HandleFunc("GET /api/v1/matches/{id}/session", s.sessionInfo)
	s.mux.HandleFunc("POST /api/v1/matches/{id}/save", s.save)
	s.mux.HandleFunc("POST /api/v1/matches/{id}/leave", s.leave)
	s.mux.HandleFunc("GET /api/v1/matches/{id}", s.snapshot)
	s.mux.HandleFunc("DELETE /api/v1/matches/{id}", s.close)
	s.mux.HandleFunc("POST /api/v1/matches/{id}/commands", s.command)
	s.mux.HandleFunc("POST /api/v1/matches/{id}/placement", s.placement)
	s.mux.HandleFunc("GET /api/v1/matches/{id}/events", s.events)
	s.mux.HandleFunc("GET /api/v1/matches/{id}/log", s.journal)
	s.mux.HandleFunc("GET /api/v1/matches/{id}/entities/{entity}/history", s.journal)
	s.mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, 404, "not_found", "API endpoint not found.")
	})
	s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			writeError(w, 405, "method_not_allowed", "Use GET to load the interface.")
			return
		}
		s.static(w, r)
	})
	return s
}
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "same-origin")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; worker-src 'self'; frame-ancestors 'none'")
	if strings.HasPrefix(r.URL.Path, "/api/") {
		w.Header().Set("Cache-Control", "no-store")
	}
	select {
	case <-s.matches.Stopping():
		domainError(w, matches.ErrShuttingDown)
		return
	default:
	}
	if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+r.Host && origin != "https://"+r.Host {
		writeError(w, 403, "origin_rejected", "Cross-origin requests are not allowed.")
		return
	}
	s.mux.ServeHTTP(w, r)
}
func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		writeError(w, 415, "unsupported_media_type", "Send application/json.")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		writeError(w, 400, "invalid_json", "Invalid JSON body or unknown field.")
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeError(w, 400, "invalid_json", "Send exactly one JSON object.")
		return false
	}
	return true
}
func respond(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Debug("response ended", "error", err)
	}
}
func writeError(w http.ResponseWriter, status int, code, message string) {
	respond(w, status, ErrorBody{game.RuleError{Code: code, Message: message}})
}
func domainError(w http.ResponseWriter, err error) {
	var rule *game.RuleError
	switch {
	case errors.As(err, &rule):
		status := 422
		if rule.Code == "idempotency_conflict" || rule.Code == "stale_revision" || rule.Code == "game_exists" {
			status = 409
		}
		if rule.Code == "rate_limited" {
			status = 429
		}
		if rule.Code == "entity_not_found" {
			status = 404
		}
		respond(w, status, ErrorBody{*rule})
	case errors.Is(err, matches.ErrNotFound):
		writeError(w, 404, "match_not_found", "No saved game has that name or session ID.")
	case errors.Is(err, matches.ErrNameExists):
		writeError(w, 409, "name_exists", "A game with that name already exists. Choose another name or resume it.")
	case errors.Is(err, matches.ErrForbidden):
		writeError(w, 403, "forbidden", "This action is not allowed for your membership.")
	case errors.Is(err, matches.ErrUnauthorized):
		writeError(w, 401, "unauthorized", "A valid match token is required.")
	case errors.Is(err, matches.ErrCapacity):
		writeError(w, 503, "server_full", "The server is full. Try again later.")
	case errors.Is(err, matches.ErrShuttingDown):
		w.Header().Set("Retry-After", "1")
		writeError(w, 503, "server_shutting_down", "The server is shutting down. Reconnect after it restarts.")
	case errors.Is(err, statemachine.ErrNotPermitted):
		writeError(w, 422, "invalid_state", "This action is unavailable in the current state.")
	default:
		slog.Error("request failed", "error", err)
		writeError(w, 500, "internal_error", "The request could not be completed.")
	}
}
func (s *Server) authorize(w http.ResponseWriter, r *http.Request) *matches.Match {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		writeError(w, 401, "unauthorized", "A match token is required.")
		return nil
	}
	m, err := s.matches.Authorized(r.PathValue("id"), strings.TrimPrefix(auth, "Bearer "))
	if err != nil {
		domainError(w, err)
		return nil
	}
	return m
}
func (s *Server) create(w http.ResponseWriter, r *http.Request) {
	var cfg game.Config
	if !decode(w, r, &cfg) {
		return
	}
	if !game.ValidCivilization(cfg.Civilization) {
		writeError(w, 422, "invalid_civilization", "Choose a civilization from the catalog.")
		return
	}
	if !game.ValidDifficulty(cfg.Difficulty) {
		writeError(w, 422, "invalid_difficulty", "Choose a difficulty from the catalog.")
		return
	}
	if cfg.Mode != "skirmish" && cfg.Mode != "sandbox" {
		writeError(w, 422, "invalid_mode", "Choose skirmish or sandbox.")
		return
	}
	if cfg.Settlements < 0 || cfg.Settlements > 6 {
		writeError(w, 422, "invalid_settlements", "Choose from 1 to 6 settlements, including yours.")
		return
	}
	session, err := s.matches.Create(cfg)
	if err != nil {
		domainError(w, err)
		return
	}
	w.Header().Set("Location", "/api/v1/matches/"+session.MatchID)
	respond(w, 201, session)
}
func (s *Server) snapshot(w http.ResponseWriter, r *http.Request) {
	if m := s.authorize(w, r); m != nil {
		respond(w, 200, m.View())
	}
}
func (s *Server) close(w http.ResponseWriter, r *http.Request) {
	if m := s.authorize(w, r); m != nil {
		if err := s.matches.Delete(r.PathValue("id"), m); err != nil {
			domainError(w, err)
			return
		}
		w.WriteHeader(204)
	}
}

func (s *Server) sessions(w http.ResponseWriter, r *http.Request) {
	writeError(w, 401, "membership_required", "Use your private game library or a rejoin code. Names and IDs do not grant access.")
}
func (s *Server) resume(w http.ResponseWriter, r *http.Request) {
	writeError(w, 401, "membership_required", "Use your private rejoin code to recover a game.")
}
func (s *Server) sessionInfo(w http.ResponseWriter, r *http.Request) {
	if m := s.authorize(w, r); m != nil {
		respond(w, 200, m.Info())
	}
}
func (s *Server) save(w http.ResponseWriter, r *http.Request)  { s.checkpoint(w, r, false) }
func (s *Server) leave(w http.ResponseWriter, r *http.Request) { s.checkpoint(w, r, true) }
func (s *Server) checkpoint(w http.ResponseWriter, r *http.Request, leave bool) {
	if m := s.authorize(w, r); m != nil {
		result, err := s.matches.Save(r.PathValue("id"), m, leave)
		if err != nil {
			domainError(w, err)
			return
		}
		respond(w, 200, result)
	}
}
func (s *Server) command(w http.ResponseWriter, r *http.Request) {
	m := s.authorize(w, r)
	if m == nil {
		return
	}
	var c game.Command
	if !decode(w, r, &c) {
		return
	}
	receipt, err := m.Apply(c)
	if err != nil {
		domainError(w, err)
		return
	}
	respond(w, 200, receipt)
}
func (s *Server) placement(w http.ResponseWriter, r *http.Request) {
	m := s.authorize(w, r)
	if m == nil {
		return
	}
	var p matches.Placement
	if !decode(w, r, &p) {
		return
	}
	respond(w, 200, m.Placement(p))
}
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	m := s.authorize(w, r)
	if m == nil {
		return
	}
	_, ok := w.(http.Flusher)
	if !ok {
		writeError(w, 500, "stream_unavailable", "Streaming is unavailable.")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(200)
	controller := http.NewResponseController(w)
	// Interrupt a blocked write too, not just the interval between snapshots.
	// Join this watcher before returning so it cannot set a deadline on a
	// connection after net/http has reused it for another request.
	finished := make(chan struct{})
	interrupted := make(chan struct{})
	go func() {
		defer close(interrupted)
		select {
		case <-s.matches.Stopping():
		case <-r.Context().Done():
		case <-finished:
			return
		}
		_ = controller.SetWriteDeadline(time.Now())
	}()
	defer func() { close(finished); <-interrupted }()
	delta := r.URL.Query().Get("format") == "delta-v1"
	interval := 100 * time.Millisecond
	if delta {
		interval = 50 * time.Millisecond
	}
	stream, started := game.SnapshotStream{}, time.Now()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		// Set the deadline before checking cancellation: a new frame must not
		// override the interrupting deadline and then block for another 10s.
		_ = controller.SetWriteDeadline(time.Now().Add(10 * time.Second))
		select {
		case <-s.matches.Stopping():
			return
		case <-r.Context().Done():
			return
		default:
		}
		// Re-authorize to terminate streams when a match is deleted. Coalescing
		// full read models means slow/reconnecting clients never require backlog.
		if current, err := s.matches.AuthorizedStream(r.PathValue("id"), strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")); err != nil || current != m {
			return
		}
		view := m.View()
		var payload any = view
		event, sequence := "snapshot", view.Tick
		if delta {
			frame := stream.Next(view, float64(time.Since(started))/float64(time.Millisecond))
			payload, event, sequence = frame, "frame", frame.Sequence
		}
		data, err := json.Marshal(payload)
		if err != nil {
			return
		}
		if _, err = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", sequence, event, data); err != nil {
			return
		}
		if err := controller.Flush(); err != nil {
			return
		}
		select {
		case <-s.matches.Stopping():
			return
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}
func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	if s.assets == nil {
		http.Error(w, "Build the UI with: npm --prefix web ci && npm --prefix web run build", 503)
		return
	}
	if r.URL.Path != "/" && r.URL.Path != "/join" && r.URL.Path != "/game" {
		http.FileServerFS(s.assets).ServeHTTP(w, r)
		return
	}
	data, err := fs.ReadFile(s.assets, "index.html")
	if err != nil {
		http.Error(w, "Build the UI with: npm --prefix web ci && npm --prefix web run build", 503)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

// docs serves the locally bundled, interactive API reference. Keeping it on the
// same origin lets callers authorize requests with their match bearer token.
func (s *Server) docs(w http.ResponseWriter, r *http.Request) {
	if s.assets == nil {
		http.Error(w, "Build the UI with: npm --prefix web ci && npm --prefix web run build", http.StatusServiceUnavailable)
		return
	}
	data, err := fs.ReadFile(s.assets, "docs.html")
	if err != nil {
		http.Error(w, "Build the UI with: npm --prefix web ci && npm --prefix web run build", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}
