package matches

import (
	"context"
	"crowns/internal/game"
	"github.com/open-ships/statemachine"
	"time"
)

const ConnectionTimeout = 12 * time.Second
const DisconnectGrace = 15 * time.Second

func (r *room) connected(member string) bool {
	for _, c := range r.Connections {
		if c.MemberID == member && c.State.State() == connectionConnected {
			return true
		}
	}
	return false
}
func (r *room) disconnectAll() {
	for _, c := range r.Connections {
		_ = fireRoom(c.State, disconnectBrowser, c)
	}
}
func (a *Access) Connect() (ConnectionInfo, error) {
	m := a.match
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := a.valid(false); err != nil {
		return ConnectionInfo{}, err
	}
	r := m.room
	if (r.Session.State() != sessionLobby && r.Session.State() != sessionOpen) || r.Runtime.State() != runtimeServing {
		return ConnectionInfo{}, ruleError("game_unavailable", "This game is closed or moving.")
	}
	// Prune dead tabs and cap live tabs so connection IDs cannot grow forever.
	for id, c := range r.Connections {
		if c.State.State() == connectionDisconnected {
			delete(r.Connections, id)
		}
	}
	if len(r.Connections) >= 48 {
		return ConnectionInfo{}, ruleError("connection_limit", "Too many open game tabs. Close an unused tab.")
	}
	id, err := randomID(16)
	if err != nil {
		return ConnectionInfo{}, err
	}
	c := &connection{ID: id, MemberID: a.memberID, Epoch: a.epoch, LastSeen: time.Now(), State: statemachine.NewInstance(connectionMachine, connectionDisconnected)}
	_ = fireRoom(c.State, connectBrowser, c)
	r.Connections[id] = c
	delete(r.AbsentSince, a.memberID)
	delete(r.AbsenceAcknowledged, a.memberID)
	return ConnectionInfo{ID: id, Epoch: a.epoch}, nil
}
func (a *Access) Heartbeat(id string) error {
	m := a.match
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := a.valid(false); err != nil {
		return err
	}
	r := m.room
	if r.Runtime.State() != runtimeServing || (r.Session.State() != sessionLobby && r.Session.State() != sessionOpen) {
		return ruleError("game_unavailable", "This game is closed or moving.")
	}
	c := r.Connections[id]
	if c == nil || c.MemberID != a.memberID || c.Epoch != a.epoch {
		return ErrUnauthorized
	}
	if c.State.State() == connectionDisconnected {
		_ = fireRoom(c.State, connectBrowser, c)
	}
	c.LastSeen = time.Now()
	delete(r.AbsentSince, a.memberID)
	delete(r.AbsenceAcknowledged, a.memberID)
	return nil
}
func (a *Access) Disconnect(id string) error {
	m := a.match
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := a.valid(false); err != nil {
		return err
	}
	c := m.room.Connections[id]
	if c == nil {
		return nil
	}
	if c.MemberID != a.memberID || c.Epoch != a.epoch {
		return ErrUnauthorized
	}
	_ = fireRoom(c.State, disconnectBrowser, c)
	if !m.room.connected(a.memberID) {
		m.room.AbsentSince[a.memberID] = time.Now()
		delete(m.room.AbsenceAcknowledged, a.memberID)
	}
	// A departure is a hint. The scheduler applies the same policy for expired
	// heartbeats and checkpoints under the game's mutation lock.
	return nil
}
func (m *Match) pauseForPresence(reason string) {
	if m.world == nil || m.world.Status() != "running" {
		return
	}
	_ = m.world.SetPaused(true)
	m.room.Revision++
	m.room.audit("server", "paused", reason)
}
func (m *Match) pulseRoom(ctx context.Context, now time.Time) bool {
	r := m.room
	if m.lifecycle.State() != leaseOpen {
		return false
	}
	for _, i := range r.Invites {
		if i.State.State() == inviteIssued && !now.Before(i.ExpiresAt) {
			_ = fireRoom(i.State, expireInvitation, i)
		}
	}
	for _, c := range r.Connections {
		if c.State.State() == connectionConnected && now.Sub(c.LastSeen) > ConnectionTimeout {
			_ = fireRoom(c.State, disconnectBrowser, c)
		}
	}
	count := 0
	for _, p := range r.Seats {
		if p.Controller != "human" || p.State.State() != seatClaimed {
			continue
		}
		if r.connected(p.MemberID) {
			count++
			delete(r.AbsentSince, p.MemberID)
			continue
		}
		since := r.AbsentSince[p.MemberID]
		if since.IsZero() {
			since = now
			r.AbsentSince[p.MemberID] = since
		}
		if !r.AbsenceAcknowledged[p.MemberID] && now.Sub(since) >= DisconnectGrace {
			m.pauseForPresence(p.Name + " disconnected. The owner can resume when ready.")
		}
	}
	noPlayers := count == 0 && (len(r.Connections) > 0 || now.Sub(r.OpenedAt) >= DisconnectGrace)
	if noPlayers {
		m.pauseForPresence("All players left. The game is saved and paused.")
	}
	unload := noPlayers || r.Session.State() == sessionClosed || r.Runtime.State() == runtimeFrozen || r.Runtime.State() == runtimeReleased
	if r.Session.State() == sessionDeleting || r.Session.State() == sessionDeleted {
		return false
	}
	if m.db != nil && (now.Sub(m.lastSaveAttempt) >= AutosaveInterval || (unload && m.activeRequests == 0)) {
		if err := m.saveContext(ctx, now); err != nil {
			return false
		}
		if unload && m.activeRequests == 0 {
			m.fireLease(releaseLease, now)
			return true
		}
	}
	if !unload && r.Runtime.State() == runtimeServing && r.Session.State() == sessionOpen && m.world != nil && m.world.Status() == "running" {
		m.accumulator += .05 * m.world.Speed
		for m.accumulator >= game.Step && ctx.Err() == nil {
			m.world.Update()
			m.accumulator -= game.Step
		}
	}
	return false
}
