// Package matches owns clocks, concurrency, authentication, and command
// idempotency. It exposes player-scoped operations, never the simulation World.
package matches

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"crowns/internal/game"

	"github.com/open-ships/statemachine"
)

var ErrNotFound = errors.New("match not found")
var ErrUnauthorized = errors.New("invalid match token")
var ErrCapacity = errors.New("server has reached its match capacity")
var ErrShuttingDown = errors.New("server is shutting down")

type Session struct {
	MatchID  string `json:"match_id"`
	Token    string `json:"token"`
	PlayerID int    `json:"player_id"`
	Name     string `json:"name"`
}
type Receipt struct {
	CommandID string `json:"command_id"`
	Tick      int    `json:"tick"`
	Accepted  bool   `json:"accepted"`
}
type Placement struct {
	Product  string   `json:"product"`
	Position game.Vec `json:"position"`
}
type PlacementResult struct {
	Valid  bool   `json:"valid"`
	Reason string `json:"reason,omitempty"`
}
type cachedCommand struct {
	hash    [32]byte
	receipt Receipt
	err     error
}
type Match struct {
	id              string
	db              *sql.DB
	savedAt         string
	savedCursor     int
	saveError       string
	lastSaveAttempt time.Time
	mu              sync.Mutex
	world           *game.World
	tokenHash       [32]byte
	commands        map[string]cachedCommand
	lastAccess      time.Time
	window          time.Time
	windowCommands  int
	lifecycle       *statemachine.Instance[leaseState, leaseEvent, *leaseContext]
	accumulator     float64
}
type Service struct {
	mu        sync.Mutex
	matches   map[string]*Match
	db        *sql.DB
	lifecycle *statemachine.Instance[serviceState, serviceEvent, *Service]
	stopping  chan struct{}
	closeErr  error
}

func NewService() *Service {
	return &Service{matches: map[string]*Match{}, stopping: make(chan struct{}), lifecycle: statemachine.NewInstance(serviceMachine, serviceServing)}
}

// Stopping broadcasts the start of shutdown to transports and long-lived
// streams. BeginShutdown also waits for any already executing mutation.
func (s *Service) Stopping() <-chan struct{} { return s.stopping }

func (s *Service) BeginShutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fireService(beginShutdown)
}

func tokenHash(token string) [32]byte { return sha256.Sum256([]byte(token)) }
func randomID(bytes int) (string, error) {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
func (s *Service) Create(cfg game.Config) (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lifecycle.State() != serviceServing {
		return Session{}, ErrShuttingDown
	}
	if len(s.matches) >= 16 {
		return Session{}, ErrCapacity
	}
	cfg.Name = strings.TrimSpace(cfg.Name)
	if len([]rune(cfg.Name)) > 80 {
		return Session{}, &game.RuleError{Code: "invalid_name", Message: "Use a game name of up to 80 characters."}
	}
	id, err := randomID(16)
	if err != nil {
		return Session{}, err
	}
	token, err := randomID(32)
	if err != nil {
		return Session{}, err
	}
	if cfg.Name == "" {
		cfg.Name = "Kingdom " + id[:8]
	}
	for _, m := range s.matches {
		if strings.EqualFold(m.world.Config.Name, cfg.Name) {
			return Session{}, ErrNameExists
		}
	}
	m := &Match{id: id, db: s.db, world: game.New(cfg), tokenHash: tokenHash(token), commands: map[string]cachedCommand{}, lastAccess: time.Now(), lifecycle: statemachine.NewInstance(leaseMachine, leaseOpen)}
	if err := m.save(time.Now()); err != nil {
		return Session{}, err
	}
	s.matches[id] = m
	return Session{MatchID: id, Token: token, PlayerID: 1, Name: cfg.Name}, nil
}
func (s *Service) Authorized(id, token string) (*Match, error) {
	return s.authorized(id, token, true)
}

// An existing stream must never reopen a deliberately unloaded session.
func (s *Service) AuthorizedStream(id, token string) (*Match, error) {
	return s.authorized(id, token, false)
}
func (s *Service) authorized(id, token string, load bool) (*Match, error) {
	s.mu.Lock()
	if s.lifecycle.State() != serviceServing {
		s.mu.Unlock()
		return nil, ErrShuttingDown
	}
	m := s.matches[id]
	var err error
	if load {
		m, err = s.load(id)
	} else if m == nil {
		err = ErrNotFound
	}
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	hash := tokenHash(token)
	if subtle.ConstantTimeCompare(hash[:], m.tokenHash[:]) != 1 {
		return nil, ErrUnauthorized
	}
	if err := m.available(); err != nil {
		return nil, err
	}
	m.lastAccess = time.Now()
	return m, nil
}
func (s *Service) Delete(id string, m *Match) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lifecycle.State() != serviceServing {
		return ErrShuttingDown
	}
	if s.matches[id] != m {
		return ErrNotFound
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if s.db != nil {
		if _, err := s.db.Exec("DELETE FROM sessions WHERE id=?", id); err != nil {
			return err
		}
	}
	m.fireLease(releaseLease, time.Now())
	delete(s.matches, id)
	return nil
}
func (m *Match) View() game.Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastAccess = time.Now()
	return m.world.View(1)
}
func (m *Match) Log(query game.LogQuery) (game.EventPage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.available(); err != nil {
		return game.EventPage{}, err
	}
	m.lastAccess = time.Now()
	return m.world.Log(1, query)
}
func (m *Match) Apply(c game.Command) (Receipt, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.available(); err != nil {
		return Receipt{}, err
	}
	if c.ID == "" || len(c.ID) > 64 {
		return Receipt{}, &game.RuleError{Code: "invalid_command_id", Message: "Use a unique command ID of 1–64 characters."}
	}
	data, err := json.Marshal(c)
	if err != nil {
		return Receipt{}, err
	}
	hash := sha256.Sum256(data)
	if old, ok := m.commands[c.ID]; ok {
		if old.hash != hash {
			return Receipt{}, &game.RuleError{Code: "idempotency_conflict", Message: "This command ID was already used for another request."}
		}
		return old.receipt, old.err
	}
	if len(m.commands) >= 50000 {
		return Receipt{}, &game.RuleError{Code: "command_limit", Message: "This match has reached its command limit."}
	}
	now := time.Now()
	if now.Sub(m.window) >= time.Second {
		m.window = now
		m.windowCommands = 0
	}
	if m.windowCommands >= 60 {
		return Receipt{}, &game.RuleError{Code: "rate_limited", Message: "Too many commands. Try again shortly."}
	}
	m.windowCommands++
	err = m.world.Apply(1, c)
	receipt := Receipt{CommandID: c.ID, Tick: m.world.Tick, Accepted: err == nil}
	m.commands[c.ID] = cachedCommand{hash, receipt, err}
	m.lastAccess = now
	return receipt, err
}
func (m *Match) Placement(p Placement) PlacementResult {
	m.mu.Lock()
	defer m.mu.Unlock()
	err := m.world.Placement(1, p.Product, p.Position)
	v := PlacementResult{Valid: err == nil}
	if err != nil {
		v.Reason = err.Error()
	}
	return v
}

// One scheduler serializes fixed steps for each match. A delayed scheduler slows
// wall-clock playback; it never changes Step or invents a larger physics delta.
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			s.mu.Lock()
			if ctx.Err() != nil || s.lifecycle.State() != serviceServing {
				s.mu.Unlock()
				return
			}
			for id, m := range s.matches {
				if ctx.Err() != nil {
					break
				}
				m.mu.Lock()
				// Save before closing a durable lease; failed writes keep the
				// only authoritative copy alive and are retried next interval.
				expiring := leaseExpired(nil, &leaseContext{m, now}) == nil
				if s.db != nil && (now.Sub(m.lastSaveAttempt) >= AutosaveInterval || expiring) {
					if err := m.saveContext(ctx, now); err != nil {
						if ctx.Err() != nil {
							m.mu.Unlock()
							break
						}
						slog.Error("checkpoint failed", "match", id, "error", err)
						m.lastAccess = now
						m.mu.Unlock()
						continue
					}
				}
				m.fireLease(leasePulse, now)
				if m.lifecycle.State() == leaseClosed {
					delete(s.matches, id)
					m.mu.Unlock()
					continue
				}
				m.accumulator += .05 * m.world.Speed
				for m.accumulator >= game.Step && ctx.Err() == nil {
					m.world.Update()
					m.accumulator -= game.Step
				}
				m.mu.Unlock()
			}
			s.mu.Unlock()
		}
	}
}
func (s *Service) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return fmt.Sprintf("%d matches", len(s.matches))
}
