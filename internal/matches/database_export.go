package matches

import (
	"context"
	"errors"
	"os"
	"time"
)

type DatabaseSnapshot struct {
	*os.File
	directory string
}

func (d *DatabaseSnapshot) Close() error {
	return errors.Join(d.File.Close(), os.RemoveAll(d.directory))
}

// Database exports one committed, standalone snapshot. Holding the match lock
// covers the checkpoint and SQLite copy. No WAL sidecars, host catalog, other
// games, or browser-specific local storage are needed to restore the game.
// Like archive export, this grants the owner a private administrative backup.
func (a *Access) Database(ctx context.Context) (*DatabaseSnapshot, error) {
	m := a.match
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := a.valid(false); err != nil {
		return nil, err
	}
	if err := a.owner(); err != nil {
		return nil, err
	}
	if m.db == nil {
		return nil, ruleError("storage_required", "Database downloads require SQLite storage.")
	}
	if err := m.saveContext(ctx, time.Now()); err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "ai-empires-export-")
	if err != nil {
		return nil, err
	}
	path := dir + "/game.sqlite"
	if _, err = m.db.ExecContext(ctx, "VACUUM INTO ?", path); err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	return &DatabaseSnapshot{File: file, directory: dir}, nil
}
