package matches

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"crowns/internal/game"
)

type checkpointWrite struct {
	id            string
	db            *sql.DB
	now           time.Time
	info          SavedGame
	state         storedMatch
	world         *game.CheckpointData
	records       []game.JournalRecord
	cursor        int
	archive       []byte
	transferID    string
	importReceipt *importedTransfer
	browser       *browserWrite
}

type checkpointJob struct {
	packet *checkpointWrite
	done   chan struct{}
	cancel context.CancelFunc
	err    error // published by closing done; the writer never takes the game lock
}

// At most one background write and one captured checkpoint exist per game.
// Commands, explicit saves and lifecycle changes still use the mutation lock.
func (m *Match) startAutosave(ctx context.Context, now time.Time) {
	if m.autosave != nil || m.available() != nil || m.room != nil && (m.room.Session.State() == sessionDeleting || m.room.Session.State() == sessionDeleted) {
		return
	}
	p, err := m.prepareCheckpoint(ctx, now, nil)
	if err != nil {
		m.completeCheckpoint(p, err)
		return
	}
	ctx, cancel := context.WithTimeout(ctx, ShutdownSaveTimeout)
	j := &checkpointJob{packet: p, done: make(chan struct{}), cancel: cancel}
	m.autosave = j
	go func() {
		defer cancel()
		j.err = p.write(ctx)
		close(j.done)
	}()
}

// Caller holds m.mu. Only this owner applies save metadata and journal cursors;
// a background writer cannot roll them back after a newer explicit save.
func (m *Match) collectAutosave(wait bool) {
	j := m.autosave
	if j == nil {
		return
	}
	if wait {
		<-j.done
	} else {
		select {
		case <-j.done:
		default:
			return
		}
	}
	m.autosave = nil
	m.completeCheckpoint(j.packet, j.err)
	if j.err != nil && !(errors.Is(j.err, context.Canceled) && m.available() != nil) {
		slog.Error("autosave failed", "match", m.id, "error", j.err)
	}
}

func (m *Match) completeCheckpoint(p *checkpointWrite, err error) {
	if err != nil {
		m.saveError = "Autosave failed. Your game is still in memory; try saving again."
		return
	}
	if p.transferID != "" {
		if m.db == nil {
			if m.archives == nil {
				m.archives = map[string][]byte{}
			}
			m.archives[p.transferID] = p.archive
		}
		m.archiveCapture = nil
	}
	m.savedAt, m.saveError, m.savedCursor = p.info.SavedAt, "", p.cursor
}
