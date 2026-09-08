package matches

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"crowns/internal/game"
	"github.com/open-ships/statemachine"
	_ "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

const AutosaveInterval = 10 * time.Second
const ShutdownSaveTimeout = 15 * time.Second

var ErrNameExists = errors.New("a game with that name already exists")

type SavedGame struct {
	MatchID         string  `json:"match_id"`
	Name            string  `json:"name"`
	SavedAt         string  `json:"saved_at"`
	Time            float64 `json:"time"`
	Difficulty      string  `json:"difficulty"`
	Settlements     int     `json:"settlements"`
	Status          string  `json:"status"`
	Active          bool    `json:"active"`
	AutosaveSeconds int     `json:"autosave_seconds"`
	SaveError       string  `json:"save_error,omitempty"`
}
type SavedGames struct {
	Games []SavedGame `json:"games"`
}
type ResumeRequest struct {
	Identifier string `json:"identifier"`
}
type storedCommand struct {
	Hash               [32]byte
	Receipt            Receipt
	Error              *game.RuleError
	RejectedTransition bool
	OtherError         string
}
type storedMatch struct {
	World       json.RawMessage
	TokenHash   [32]byte
	Accumulator float64
	Commands    map[string]storedCommand
}

// OpenService opens a local session library. Only loaded sessions consume
// simulation time; opening a checkpoint never advances offline wall time.
func OpenService(path string) (*Service, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return nil, err
		}
		file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			return nil, err
		}
		_ = file.Close()
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	fail := func(err error) (*Service, error) { _ = db.Close(); return nil, err }
	if _, err = db.Exec("PRAGMA busy_timeout=5000; PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON; PRAGMA synchronous=FULL;"); err != nil {
		return fail(err)
	}
	var version int
	if err = db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return fail(err)
	}
	if version > 1 {
		return fail(fmt.Errorf("unsupported session database version %d", version))
	}
	if _, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS sessions (
		 id TEXT PRIMARY KEY, name_key TEXT NOT NULL UNIQUE,
		 metadata BLOB NOT NULL, checkpoint BLOB NOT NULL, saved_at INTEGER NOT NULL
		);
		CREATE TABLE IF NOT EXISTS events (
		 session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
		 event_id INTEGER NOT NULL, record BLOB NOT NULL,
		 PRIMARY KEY(session_id,event_id)
		);
		PRAGMA user_version=1;
	`); err != nil {
		return fail(err)
	}
	s := NewService()
	s.db = db
	return s, nil
}

func (m *Match) info() SavedGame {
	return SavedGame{MatchID: m.id, Name: m.world.Config.Name, SavedAt: m.savedAt, Time: m.world.Time, Difficulty: m.world.Config.Difficulty, Settlements: m.world.Config.Settlements, Status: m.world.Status(), Active: m.lifecycle.State() == leaseOpen, AutosaveSeconds: int(AutosaveInterval / time.Second), SaveError: m.saveError}
}
func (m *Match) Info() SavedGame { m.mu.Lock(); defer m.mu.Unlock(); return m.info() }

// The caller holds the match lock. The checkpoint, authentication, command
// receipts, and new journal records commit together or do not commit at all.
func (m *Match) save(now time.Time) error {
	return m.saveContext(context.Background(), now)
}

func (m *Match) saveContext(ctx context.Context, now time.Time) (result error) {
	m.lastSaveAttempt = now
	defer func() {
		if result != nil {
			m.saveError = "Autosave failed. Your game is still in memory; try saving again."
		}
	}()
	if err := ctx.Err(); err != nil {
		return err
	}
	if m.db == nil {
		m.savedAt = now.UTC().Format(time.RFC3339Nano)
		m.saveError = ""
		return nil
	}
	world, err := m.world.Checkpoint()
	if err != nil {
		return err
	}
	c := storedMatch{World: world, TokenHash: m.tokenHash, Accumulator: m.accumulator, Commands: map[string]storedCommand{}}
	for id, cmd := range m.commands {
		if err := ctx.Err(); err != nil {
			return err
		}
		stored := storedCommand{Hash: cmd.hash, Receipt: cmd.receipt}
		if cmd.err != nil && !errors.As(cmd.err, &stored.Error) {
			if errors.Is(cmd.err, statemachine.ErrNotPermitted) {
				stored.RejectedTransition = true
			} else {
				stored.OtherError = cmd.err.Error()
			}
		}
		c.Commands[id] = stored
	}
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	info := m.info()
	info.SavedAt = now.UTC().Format(time.RFC3339Nano)
	info.Active = false
	info.SaveError = ""
	metadata, err := json.Marshal(info)
	if err != nil {
		return err
	}
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO sessions(id,name_key,metadata,checkpoint,saved_at) VALUES(?,?,?,?,?)
	 ON CONFLICT(id) DO UPDATE SET name_key=excluded.name_key, metadata=excluded.metadata, checkpoint=excluded.checkpoint, saved_at=excluded.saved_at`, m.id, strings.ToLower(m.world.Config.Name), metadata, data, now.UnixNano())
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed: sessions.name_key") {
			return ErrNameExists
		}
		return err
	}
	stmt, err := tx.PrepareContext(ctx, "INSERT INTO events(session_id,event_id,record) VALUES(?,?,?)")
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, record := range m.world.JournalSince(m.savedCursor) {
		if err := ctx.Err(); err != nil {
			return err
		}
		encoded, err := json.Marshal(record)
		if err != nil {
			return err
		}
		if _, err = stmt.ExecContext(ctx, m.id, record.Event.ID, encoded); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	m.savedAt, m.savedCursor, m.saveError = info.SavedAt, m.world.NextEvent, ""
	return nil
}

// The service lock protects loading and unloading; the match lock protects
// simulation, token rotation and checkpoint commits.
func (s *Service) load(id string) (*Match, error) {
	if s.lifecycle.State() != serviceServing {
		return nil, ErrShuttingDown
	}
	if m := s.matches[id]; m != nil {
		return m, nil
	}
	if s.db == nil {
		return nil, ErrNotFound
	}
	if len(s.matches) >= 16 {
		return nil, ErrCapacity
	}
	var data, metadata []byte
	if err := s.db.QueryRow("SELECT checkpoint,metadata FROM sessions WHERE id=?", id).Scan(&data, &metadata); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var c storedMatch
	var info SavedGame
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(metadata, &info); err != nil {
		return nil, err
	}
	rows, err := s.db.Query("SELECT record FROM events WHERE session_id=? ORDER BY event_id", id)
	if err != nil {
		return nil, err
	}
	records := []game.JournalRecord{}
	for rows.Next() {
		var encoded []byte
		var record game.JournalRecord
		if err = rows.Scan(&encoded); err != nil {
			break
		}
		if err = json.Unmarshal(encoded, &record); err != nil {
			break
		}
		records = append(records, record)
	}
	rowErr := rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	if rowErr != nil {
		return nil, rowErr
	}
	world, err := game.Restore(c.World, records)
	if err != nil {
		return nil, err
	}
	m := &Match{id: id, db: s.db, world: world, tokenHash: c.TokenHash, commands: map[string]cachedCommand{}, lastAccess: time.Now(), lifecycle: statemachine.NewInstance(leaseMachine, leaseOpen), accumulator: c.Accumulator, savedAt: info.SavedAt, savedCursor: world.NextEvent}
	for id, cmd := range c.Commands {
		var err error
		if cmd.Error != nil {
			err = cmd.Error
		} else if cmd.RejectedTransition {
			err = statemachine.ErrNotPermitted
		} else if cmd.OtherError != "" {
			err = errors.New(cmd.OtherError)
		}
		m.commands[id] = cachedCommand{cmd.Hash, cmd.Receipt, err}
	}
	s.matches[id] = m
	return m, nil
}

func (s *Service) List(query string) (SavedGames, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lifecycle.State() != serviceServing {
		return SavedGames{}, ErrShuttingDown
	}
	result := SavedGames{Games: []SavedGame{}}
	query = strings.ToLower(strings.TrimSpace(query))
	if s.db == nil {
		for _, m := range s.matches {
			m.mu.Lock()
			info := m.info()
			m.mu.Unlock()
			if strings.Contains(strings.ToLower(info.Name), query) || strings.Contains(info.MatchID, query) {
				result.Games = append(result.Games, info)
			}
		}
		return result, nil
	}
	rows, err := s.db.Query("SELECT metadata FROM sessions WHERE instr(name_key,?)>0 OR instr(id,?)>0 ORDER BY saved_at DESC LIMIT 100", query, query)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var data []byte
		var info SavedGame
		if err := rows.Scan(&data); err != nil {
			return result, err
		}
		if err := json.Unmarshal(data, &info); err != nil {
			return result, err
		}
		// Do not acquire a match lock while holding SQLite's only connection:
		// an autosave can own that lock while waiting for the connection.
		result.Games = append(result.Games, info)
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	_ = rows.Close()
	for i, info := range result.Games {
		if m := s.matches[info.MatchID]; m != nil {
			m.mu.Lock()
			result.Games[i] = m.info()
			m.mu.Unlock()
		}
	}
	return result, nil
}

func (s *Service) Resume(identifier string) (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lifecycle.State() != serviceServing {
		return Session{}, ErrShuttingDown
	}
	identifier = strings.TrimSpace(identifier)
	id := ""
	if s.db != nil {
		err := s.db.QueryRow("SELECT id FROM sessions WHERE id=? OR name_key=? ORDER BY CASE WHEN id=? THEN 0 ELSE 1 END LIMIT 1", identifier, strings.ToLower(identifier), identifier).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return Session{}, ErrNotFound
		}
		if err != nil {
			return Session{}, err
		}
	} else {
		for key, m := range s.matches {
			if key == identifier || strings.EqualFold(m.world.Config.Name, identifier) {
				id = key
				break
			}
		}
	}
	m, err := s.load(id)
	if err != nil {
		return Session{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	token, err := randomID(32)
	if err != nil {
		return Session{}, err
	}
	previous := m.tokenHash
	m.tokenHash = tokenHash(token)
	if err = m.save(time.Now()); err != nil {
		m.tokenHash = previous
		return Session{}, err
	}
	m.lastAccess = time.Now()
	return Session{MatchID: m.id, Token: token, PlayerID: 1, Name: m.world.Config.Name}, nil
}

func (s *Service) Save(id string, m *Match, leave bool) (SavedGame, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lifecycle.State() != serviceServing {
		return SavedGame{}, ErrShuttingDown
	}
	if s.matches[id] != m {
		return SavedGame{}, ErrNotFound
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.save(time.Now()); err != nil {
		return SavedGame{}, err
	}
	info := m.info()
	if leave && s.db != nil {
		m.fireLease(releaseLease, time.Now())
		delete(s.matches, id)
		info.Active = false
	}
	return info, nil
}

// Close freezes games, checkpoints them with a fresh shutdown budget, and
// closes SQLite. Repeated or concurrent calls return the first close result.
func (s *Service) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), ShutdownSaveTimeout)
	defer cancel()
	return s.CloseContext(ctx)
}

// CloseContext's context bounds final checkpoint I/O, independently of the
// canceled simulation/request contexts. BeginShutdown is a mutation barrier:
// even a stalled handler retaining a Match cannot change a saved world later.
func (s *Service) CloseContext(ctx context.Context) error {
	s.BeginShutdown()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lifecycle.State() == serviceClosed {
		return s.closeErr
	}
	var result error
	if s.db != nil && len(s.matches) > 0 {
		// SQLite's busy handler can sleep before observing an interrupt. Use
		// short waits during final saves; our retries obey the shared context.
		if _, err := s.db.ExecContext(ctx, "PRAGMA busy_timeout=100"); err != nil {
			result = errors.Join(result, fmt.Errorf("configure shutdown checkpoints: %w", err))
		}
	}
	for id, m := range s.matches {
		m.mu.Lock()
		err := m.saveOnShutdown(ctx)
		m.mu.Unlock()
		if err != nil {
			result = errors.Join(result, fmt.Errorf("checkpoint session %s: %w", id, err))
		}
	}
	if s.db != nil {
		if err := s.db.Close(); err != nil {
			result = errors.Join(result, fmt.Errorf("close session database: %w", err))
		}
	}
	s.closeErr = result
	s.fireService(finishShutdown)
	return result
}

// Retry only transient lock contention. Other storage errors are reported
// immediately, retaining the last committed checkpoint. Caller holds m.mu.
func (m *Match) saveOnShutdown(ctx context.Context) error {
	for {
		err := m.saveContext(ctx, time.Now())
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return errors.Join(err, ctx.Err())
		}
		var sqliteErr interface{ Code() int }
		if !errors.As(err, &sqliteErr) || (sqliteErr.Code()&0xff != sqlite3.SQLITE_BUSY && sqliteErr.Code()&0xff != sqlite3.SQLITE_LOCKED) {
			return err
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return errors.Join(err, ctx.Err())
		case <-timer.C:
		}
	}
}
