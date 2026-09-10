package matches

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"crowns/internal/game"
)

type SaveSnapshot struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type SavedSnapshot struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Time      float64 `json:"time"`
	Tick      int     `json:"tick"`
	CreatedAt string  `json:"created_at"`
}
type SnapshotLibrary struct {
	Snapshots []SavedSnapshot `json:"snapshots"`
}
type ForkSnapshot struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type storedSnapshot struct {
	Info    SavedSnapshot
	Hash    [32]byte
	Payload []byte
}

func snapshotRequest(id, name string) error {
	if id == "" || len(id) > 64 {
		return ruleError("invalid_command_id", "Use a unique request ID of 1–64 characters.")
	}
	if strings.TrimSpace(name) == "" || len([]rune(name)) > 80 {
		return ruleError("invalid_name", "Name this snapshot or game using 1–80 characters.")
	}
	return nil
}
func (a *Access) snapshotOwner() error {
	if a.match.room == nil {
		return ErrForbidden
	}
	if _, err := a.valid(false); err != nil {
		return err
	}
	return a.owner()
}
func (m *Match) savedSnapshot(id string) (storedSnapshot, error) {
	if m.db == nil {
		v, ok := m.snapshots[id]
		if !ok {
			return v, ErrNotFound
		}
		return v, nil
	}
	var v storedSnapshot
	var metadata, hash []byte
	err := m.db.QueryRow("SELECT metadata,request_hash,payload FROM game_snapshots WHERE game_id=? AND snapshot_id=?", m.id, id).Scan(&metadata, &hash, &v.Payload)
	if errors.Is(err, sql.ErrNoRows) {
		return v, ErrNotFound
	}
	if err != nil {
		return v, err
	}
	if err = json.Unmarshal(metadata, &v.Info); err != nil {
		return v, err
	}
	copy(v.Hash[:], hash)
	return v, nil
}
func (a *Access) Snapshots() (SnapshotLibrary, error) {
	m := a.match
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := a.snapshotOwner(); err != nil {
		return SnapshotLibrary{}, err
	}
	return m.snapshotLibrary()
}
func (m *Match) snapshotLibrary() (SnapshotLibrary, error) {
	v := SnapshotLibrary{Snapshots: []SavedSnapshot{}}
	if m.db == nil {
		for _, s := range m.snapshots {
			v.Snapshots = append(v.Snapshots, s.Info)
		}
	} else {
		rows, err := m.db.Query("SELECT metadata FROM game_snapshots WHERE game_id=?", m.id)
		if err != nil {
			return v, err
		}
		defer rows.Close()
		for rows.Next() {
			var raw []byte
			var info SavedSnapshot
			if err = rows.Scan(&raw); err != nil {
				return v, err
			}
			if err = json.Unmarshal(raw, &info); err != nil {
				return v, err
			}
			v.Snapshots = append(v.Snapshots, info)
		}
		if err = rows.Err(); err != nil {
			return v, err
		}
	}
	slices.SortFunc(v.Snapshots, func(a, b SavedSnapshot) int {
		if a.CreatedAt != b.CreatedAt {
			return strings.Compare(b.CreatedAt, a.CreatedAt)
		}
		return strings.Compare(a.ID, b.ID)
	})
	return v, nil
}

// A named save is immutable persistence data, not another simulation state.
// Capture under the owner lock; compression runs on detached data so it cannot
// stall the simulation. The final commit rechecks access and duplicate IDs.
func (a *Access) SaveSnapshot(req SaveSnapshot) (SavedSnapshot, error) {
	if err := snapshotRequest(req.ID, req.Name); err != nil {
		return SavedSnapshot{}, err
	}
	m := a.match
	hash := sha256.Sum256([]byte(req.Name))
	m.mu.Lock()
	if err := a.snapshotOwner(); err != nil {
		m.mu.Unlock()
		return SavedSnapshot{}, err
	}
	if old, err := m.savedSnapshot(req.ID); err == nil {
		m.mu.Unlock()
		if old.Hash != hash {
			return SavedSnapshot{}, ruleError("idempotency_conflict", "This snapshot request ID already has a different name.")
		}
		return old.Info, nil
	} else if !errors.Is(err, ErrNotFound) {
		m.mu.Unlock()
		return SavedSnapshot{}, err
	}
	if m.world == nil || m.room.Runtime.State() != runtimeServing || m.room.Session.State() != sessionOpen || m.world.Status() == "finished" {
		m.mu.Unlock()
		return SavedSnapshot{}, ruleError("snapshot_unavailable", "Start or reopen an unfinished game before taking a snapshot.")
	}
	capture := m.world.CaptureCheckpoint()
	info := SavedSnapshot{ID: req.ID, Name: strings.TrimSpace(req.Name), Time: m.world.Time, Tick: m.world.Tick, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	payload := gameArchive{Manifest: archiveMeta{Format: archiveMagic, Rules: game.RulesVersion, GameID: m.id, Kind: "copy"}, Checkpoint: storedMatch{Room: m.room.stored()}, Journal: m.world.JournalSince(0)}
	m.mu.Unlock()
	var err error
	payload.Checkpoint.World, err = capture.Encode()
	if err != nil {
		return SavedSnapshot{}, err
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return SavedSnapshot{}, err
	}
	if len(raw) > maxExpandedArchive {
		return SavedSnapshot{}, ruleError("snapshot_too_large", "This snapshot exceeds the current save size limit.")
	}
	var compressed bytes.Buffer
	z := gzip.NewWriter(&compressed)
	if _, err = z.Write(raw); err != nil {
		return SavedSnapshot{}, err
	}
	if err = z.Close(); err != nil {
		return SavedSnapshot{}, err
	}
	saved := storedSnapshot{Info: info, Hash: hash, Payload: compressed.Bytes()}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err = a.snapshotOwner(); err != nil {
		return SavedSnapshot{}, err
	}
	if old, e := m.savedSnapshot(req.ID); e == nil {
		if old.Hash != hash {
			return SavedSnapshot{}, ruleError("idempotency_conflict", "This snapshot request ID already has a different name.")
		}
		return old.Info, nil
	} else if !errors.Is(e, ErrNotFound) {
		return SavedSnapshot{}, e
	}
	library, err := m.snapshotLibrary()
	if err != nil {
		return SavedSnapshot{}, err
	}
	if len(library.Snapshots) >= 100 {
		return SavedSnapshot{}, ruleError("snapshot_limit", "Remove an older snapshot before saving another. Each game can keep 100 snapshots.")
	}
	if m.db == nil {
		if m.snapshots == nil {
			m.snapshots = map[string]storedSnapshot{}
		}
		m.snapshots[req.ID] = saved
	} else {
		metadata, _ := json.Marshal(info)
		if _, err = m.db.Exec("INSERT INTO game_snapshots(game_id,snapshot_id,metadata,request_hash,payload) VALUES(?,?,?,?,?)", m.id, req.ID, metadata, hash[:], saved.Payload); err != nil {
			return SavedSnapshot{}, err
		}
	}
	return info, nil
}

func (a *Access) DeleteSnapshot(id string) error {
	m := a.match
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := a.snapshotOwner(); err != nil {
		return err
	}
	if m.db == nil {
		delete(m.snapshots, id)
		return nil
	}
	_, err := m.db.Exec("DELETE FROM game_snapshots WHERE game_id=? AND snapshot_id=?", m.id, id)
	return err
}

func (s *Service) ForkSnapshot(a *Access, id string, req ForkSnapshot, browser string) (ImportResult, error) {
	if err := snapshotRequest(req.ID, req.Name); err != nil {
		return ImportResult{}, err
	}
	m := a.match
	m.mu.Lock()
	if err := a.snapshotOwner(); err != nil {
		m.mu.Unlock()
		return ImportResult{}, err
	}
	saved, err := m.savedSnapshot(id)
	key := fmt.Sprintf("snapshot:%s:%s:%s", m.id, a.memberID, req.ID)
	m.mu.Unlock()
	if err != nil {
		return ImportResult{}, err
	}
	z, err := gzip.NewReader(bytes.NewReader(saved.Payload))
	if err != nil {
		return ImportResult{}, err
	}
	defer z.Close()
	raw, err := io.ReadAll(io.LimitReader(z, maxExpandedArchive+1))
	if err != nil {
		return ImportResult{}, err
	}
	if len(raw) > maxExpandedArchive {
		return ImportResult{}, ruleError("invalid_snapshot", "The snapshot exceeds the supported size.")
	}
	var payload gameArchive
	if err = json.Unmarshal(raw, &payload); err != nil {
		return ImportResult{}, err
	}
	if payload.Manifest.Format != archiveMagic || payload.Manifest.Rules != game.RulesVersion || payload.Manifest.GameID != m.id || payload.Checkpoint.Room == nil {
		return ImportResult{}, ruleError("invalid_snapshot", "This snapshot is incompatible with this game.")
	}
	hash := sha256.Sum256([]byte(id + "\x00" + req.Name))
	return s.importArchive(payload, ImportRequest{Copy: true, Name: strings.TrimSpace(req.Name)}, browser, &importedTransfer{ID: key, Receipt: fmt.Sprintf("%x", hash)})
}
