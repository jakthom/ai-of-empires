// Package matches owns clocks, concurrency, authentication, and command
// idempotency. It exposes player-scoped operations, never the simulation World.
package matches

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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
	Orientation string    `json:"orientation,omitempty"`
	EndPosition *game.Vec `json:"end_position,omitempty"`
	Product     string    `json:"product"`
	Position    game.Vec  `json:"position"`
}
type PlacementResult struct {
	DeckElevation float64        `json:"deck_elevation,omitempty"`
	Orientation   string         `json:"orientation,omitempty"`
	Positions     []game.Vec     `json:"positions"`
	Cost          game.Resources `json:"cost"`
	Valid         bool           `json:"valid"`
	Reason        string         `json:"reason,omitempty"`
}
type cachedCommand struct {
	hash    [32]byte
	receipt Receipt
	err     error
}
type Match struct {
	snapshots       map[string]storedSnapshot
	activeRequests  int
	archiveCapture  *archiveMeta
	archives        map[string][]byte
	importReceipt   *importedTransfer
	room            *room
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
	lastStepAt      time.Time
	lastMaintenance time.Time
	autosave        *checkpointJob
	accumulator     float64
}
type Service struct {
	stores    map[string]*sql.DB
	paths     map[string]string
	gamesDir  string
	imported  map[string]importedTransfer
	browsers  map[string]map[string]browserBinding
	deleted   map[string]bool
	mu        sync.Mutex
	matches   map[string]*Match
	db        *sql.DB
	lifecycle *statemachine.Instance[serviceState, serviceEvent, *Service]
	stopping  chan struct{}
	closeErr  error
}

func NewService() *Service {
	return &Service{stores: map[string]*sql.DB{}, paths: map[string]string{}, imported: map[string]importedTransfer{}, browsers: map[string]map[string]browserBinding{}, deleted: map[string]bool{}, matches: map[string]*Match{}, stopping: make(chan struct{}), lifecycle: statemachine.NewInstance(serviceMachine, serviceServing)}
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
		if m.room == nil && strings.EqualFold(m.world.Config.Name, cfg.Name) {
			return Session{}, ErrNameExists
		}
	}
	for _, db := range s.stores {
		var count int
		if err := db.QueryRow("SELECT count(*) FROM sessions WHERE name_key=?", strings.ToLower(cfg.Name)).Scan(&count); err != nil {
			return Session{}, err
		}
		if count > 0 {
			return Session{}, ErrNameExists
		}
	}
	world, err := game.NewWorld(cfg)
	if err != nil {
		return Session{}, err
	}
	db, err := s.newGameDatabase(id)
	if err != nil {
		return Session{}, err
	}
	defer s.discardUncreated(id)
	m := &Match{id: id, db: db, world: world, tokenHash: tokenHash(token), commands: map[string]cachedCommand{}, lastAccess: time.Now(), lifecycle: statemachine.NewInstance(leaseMachine, leaseOpen)}
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
		m, err = s.loadLocked(id)
	} else if m == nil {
		err = ErrNotFound
	} else {
		m.mu.Lock()
	}
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	defer m.mu.Unlock()
	if m.room != nil {
		return nil, ErrUnauthorized
	}
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
		if err := s.deleteDatabase(m, "legacy"); err != nil {
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
	positions, cost, orientation, err := m.world.PlanOrientedBuilding(1, p.Product, p.Position, p.EndPosition, p.Orientation)
	v := PlacementResult{Valid: err == nil, Positions: positions, Cost: cost, Orientation: orientation}
	if p.Product == "bridge" {
		v.DeckElevation = m.world.BridgeElevation(p.Position, p.EndPosition)
	}
	if err != nil {
		v.Reason = err.Error()
	}
	return v
}

func (s *Service) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return fmt.Sprintf("%d matches", len(s.matches))
}
