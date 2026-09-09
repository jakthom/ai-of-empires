package matches

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The host database retains only deletion markers. Every game's authoritative
// tables live together in one SQLite file, including recovery credentials,
// command receipts, archives, the checkpoint, and the append-only journal.
// DELETE journaling makes every committed, quiescent file self-contained.
func OpenService(path string) (*Service, error) {
	catalog, err := openDatabase(path)
	if err != nil {
		return nil, err
	}
	s := NewService()
	s.db = catalog
	fail := func(err error) (*Service, error) {
		for _, db := range s.stores {
			_ = db.Close()
		}
		_ = catalog.Close()
		return nil, err
	}
	if path != ":memory:" {
		s.gamesDir = path + ".games"
		if err = os.MkdirAll(s.gamesDir, 0700); err != nil {
			return fail(err)
		}
	}
	rows, err := catalog.Query("SELECT game_id FROM tombstones")
	if err != nil {
		return fail(err)
	}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			break
		}
		s.deleted[id] = true
	}
	rowErr := rows.Err()
	rows.Close()
	if err != nil {
		return fail(err)
	}
	if rowErr != nil {
		return fail(rowErr)
	}
	if s.gamesDir != "" {
		files, err := filepath.Glob(filepath.Join(s.gamesDir, "*.sqlite"))
		if err != nil {
			return fail(err)
		}
		for _, path := range files {
			db, err := openDatabase(path)
			if err != nil {
				return fail(fmt.Errorf("open game %s: %w", filepath.Base(path), err))
			}
			var id string
			var count int
			err = db.QueryRow("SELECT count(*), coalesce(min(id),'') FROM sessions").Scan(&count, &id)
			if err != nil || count > 1 {
				db.Close()
				return fail(fmt.Errorf("invalid isolated game database %s", filepath.Base(path)))
			}
			if count == 0 {
				db.Close()
				continue
			} // interrupted creation, safe to ignore
			if s.deleted[id] {
				db.Close()
				if err = os.Remove(path); err != nil {
					return fail(err)
				}
				continue
			}
			if s.stores[id] != nil {
				db.Close()
				return fail(fmt.Errorf("duplicate game %s in game directory", id))
			}
			s.stores[id], s.paths[id] = db, path
		}
	}
	if err = s.migrateLibrary(path); err != nil {
		return fail(err)
	}
	return s, nil
}

// Caller holds s.mu. Database handles outlive loaded simulations and close at
// deletion/shutdown; authentication never needs to restore an expensive world.
func (s *Service) gameDatabase(id string) (*sql.DB, error) {
	if s.deleted[id] || s.stores[id] == nil {
		return nil, ErrNotFound
	}
	return s.stores[id], nil
}

func (s *Service) newGameDatabase(id string) (*sql.DB, error) {
	if s.db == nil {
		return nil, nil
	}
	if s.deleted[id] || s.stores[id] != nil {
		return nil, errors.New("game already exists")
	}
	// IDs used as filenames are generated locally or validated archive IDs.
	if len(id) != 32 || strings.Trim(id, "0123456789abcdef") != "" {
		return nil, errors.New("invalid game ID")
	}
	path := ":memory:"
	if s.gamesDir != "" {
		path = filepath.Join(s.gamesDir, id+".sqlite")
	}
	db, err := openDatabase(path)
	if err != nil {
		return nil, err
	}
	s.stores[id] = db
	if path != ":memory:" {
		s.paths[id] = path
	}
	return db, nil
}

func (s *Service) discardUncreated(id string) {
	if s.matches[id] != nil {
		return
	}
	if db := s.stores[id]; db != nil {
		_ = db.Close()
		delete(s.stores, id)
	}
	if path := s.paths[id]; path != "" {
		_ = os.Remove(path)
		delete(s.paths, id)
	}
}

// Tombstone commits first: a crash or failed unlink cannot resurrect a deleted
// game on this host. A stale Match is also fenced by its own database marker.
func (s *Service) deleteDatabase(m *Match, epoch string) error {
	if !s.deleted[m.id] {
		tx, err := m.db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err = tx.Exec("INSERT INTO tombstones(game_id,epoch) VALUES(?,?) ON CONFLICT(game_id) DO NOTHING", m.id, epoch); err != nil {
			return err
		}
		if _, err = tx.Exec("DELETE FROM sessions WHERE id=?", m.id); err != nil {
			return err
		}
		if _, err = s.db.Exec("INSERT INTO tombstones(game_id,epoch) VALUES(?,?) ON CONFLICT(game_id) DO NOTHING", m.id, epoch); err != nil {
			return err
		}
		s.deleted[m.id] = true
		if err = tx.Commit(); err != nil {
			return err
		}
	}
	if m.db != nil {
		if err := m.db.Close(); err != nil {
			return err
		}
	}
	delete(s.stores, m.id)
	if path := s.paths[m.id]; path != "" {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		delete(s.paths, m.id)
	}
	return nil
}

// Legacy migration is restartable: copy each complete game in a transaction,
// then delete its old rows. A crash between those commits leaves two identical
// checkpoints, which the next run verifies before removing the legacy copy.
var gameTables = []struct{ name, key string }{{"sessions", "id"}, {"events", "session_id"}, {"browser_members", "game_id"}, {"game_secrets", "game_id"}, {"game_archives", "game_id"}, {"imported_transfers", "game_id"}}

func (s *Service) migrateLibrary(path string) error {
	rows, err := s.db.Query("SELECT id FROM sessions")
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			break
		}
		ids = append(ids, id)
	}
	rowErr := rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if rowErr != nil {
		return rowErr
	}
	for _, id := range ids {
		if s.deleted[id] {
			_, err = s.db.Exec("DELETE FROM sessions WHERE id=?", id)
			if err != nil {
				return err
			}
			continue
		}
		db := s.stores[id]
		if db == nil {
			db, err = s.newGameDatabase(id)
			if err != nil {
				return err
			}
			if _, err = db.Exec("ATTACH DATABASE ? AS legacy", path); err != nil {
				return err
			}
			tx, err := db.Begin()
			if err != nil {
				return err
			}
			for _, table := range gameTables {
				_, err = tx.Exec("INSERT INTO main."+table.name+" SELECT * FROM legacy."+table.name+" WHERE "+table.key+"=?", id)
				if err != nil {
					break
				}
			}
			if err != nil {
				tx.Rollback()
				return fmt.Errorf("migrate game %s: %w", id, err)
			}
			if err = tx.Commit(); err != nil {
				return err
			}
			if _, err = db.Exec("DETACH DATABASE legacy"); err != nil {
				return err
			}
		} else {
			if _, err = db.Exec("ATTACH DATABASE ? AS legacy", path); err != nil {
				return err
			}
			for _, table := range gameTables {
				var different int
				left := "SELECT * FROM main." + table.name + " WHERE " + table.key + "=?"
				right := "SELECT * FROM legacy." + table.name + " WHERE " + table.key + "=?"
				err = db.QueryRow("SELECT EXISTS("+left+" EXCEPT "+right+")+EXISTS("+right+" EXCEPT "+left+")", id, id, id, id).Scan(&different)
				if err != nil {
					return err
				}
				if different != 0 {
					return fmt.Errorf("conflicting legacy %s for game %s", table.name, id)
				}
			}
			if _, err = db.Exec("DETACH DATABASE legacy"); err != nil {
				return err
			}
		}
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		if _, err = tx.Exec("DELETE FROM sessions WHERE id=?", id); err != nil {
			tx.Rollback()
			return err
		}
		if _, err = tx.Exec("DELETE FROM imported_transfers WHERE game_id=?", id); err != nil {
			tx.Rollback()
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}

	}
	return nil
}
