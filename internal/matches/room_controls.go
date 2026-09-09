package matches

import (
	"crowns/internal/game"
	"crypto/sha256"
	"encoding/json"
	"maps"
	"strings"
	"time"
)

func (a *Access) control(kind, id string, revision int, request any, owner, ignoreRevision bool, fn func() error) (GameInfo, error) {
	m := a.match
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := a.valid(false); err != nil {
		return GameInfo{}, err
	}
	if owner {
		if err := a.owner(); err != nil {
			return GameInfo{}, err
		}
	}
	if id == "" || len(id) > 64 {
		return GameInfo{}, ruleError("invalid_command_id", "Use a unique request ID of 1–64 characters.")
	}
	raw, err := json.Marshal(request)
	if err != nil {
		return GameInfo{}, err
	}
	hash := sha256.Sum256(append([]byte(kind+":"), raw...))
	key := a.memberID + ":" + id
	if old, ok := m.room.Controls[key]; ok {
		if old.Hash != hash || old.Kind != kind {
			return GameInfo{}, ruleError("idempotency_conflict", "This request ID was already used.")
		}
		return a.info(), nil
	}
	if !ignoreRevision && revision != m.room.Revision {
		return GameInfo{}, ruleError("stale_revision", "The game changed. Refresh its controls and try again.")
	}
	if len(m.room.Controls) >= 10000 {
		return GameInfo{}, ruleError("command_limit", "This game has reached its control request limit.")
	}
	before, oldRoom, oldWorld := m.room.stored(), m.room, m.world
	oldAbsence := maps.Clone(oldRoom.AbsenceAcknowledged)
	oldSpeed, wasRunning := 0., false
	if oldWorld != nil {
		oldSpeed, wasRunning = oldWorld.Speed, oldWorld.Status() == "running"
	}
	if err = fn(); err != nil {
		return GameInfo{}, err
	}
	m.room.Revision++
	m.room.Controls[key] = controlReceipt{hash, kind}
	if err = m.save(time.Now()); err != nil {
		restored, restoreErr := restoreRoom(before)
		if restoreErr != nil {
			panic(restoreErr)
		}
		restored.Connections = oldRoom.Connections
		restored.AbsentSince = oldRoom.AbsentSince
		restored.AbsenceAcknowledged = oldAbsence
		restored.OpenedAt = oldRoom.OpenedAt
		m.room = restored
		if oldWorld == nil {
			m.world = nil
		} else {
			// A failed durable control must never leave an uncommitted Resume
			// advancing the world. Retain the last speed and stop the clock.
			_ = m.world.SetSpeed(oldSpeed)
			_ = m.world.SetPaused(true)
			if wasRunning {
				m.room.Revision++
				m.room.audit("server", "paused", "Paused because the game could not be saved. Retry the save before resuming.")
			}
		}
		return GameInfo{}, err
	}
	return a.info(), nil
}
func (a *Access) requireLobby() error {
	r := a.match.room
	if r.Session.State() != sessionLobby || r.Runtime.State() != runtimeServing {
		return ruleError("invalid_state", "Seats and world settings can only change in an open lobby.")
	}
	return nil
}
func (a *Access) findSeat(id string) (*seat, error) {
	for _, s := range a.match.room.Seats {
		if s.ID == id {
			return s, nil
		}
	}
	return nil, ErrNotFound
}
func (a *Access) ChangeSeat(id string, req SeatChange) (GameInfo, error) {
	return a.control("seat:"+id, req.ID, req.Revision, req, false, false, func() error {
		if err := a.requireLobby(); err != nil {
			return err
		}
		s, err := a.findSeat(id)
		if err != nil {
			return err
		}
		own := s.MemberID == a.memberID
		owner := a.memberID == a.match.room.OwnerID
		if !own && !owner {
			return ErrForbidden
		}
		if req.Controller != "human" && req.Controller != "ai" {
			return ruleError("invalid_controller", "Choose a human or AI seat.")
		}
		if !game.ValidCivilization(req.Civilization) {
			return ruleError("invalid_civilization", "Choose a civilization from the catalog.")
		}
		name, err := playerName(req.Name, s.Name)
		if err != nil {
			return err
		}
		if req.Controller != s.Controller && (s.State.State() == seatClaimed || !owner) {
			return ruleError("seat_claimed", "A claimed human seat cannot be changed to AI.")
		}
		a.revokeInvites(s.ID)
		s.Name, s.Civilization, s.Controller = name, req.Civilization, req.Controller
		if s.State.State() == seatReserved {
			_ = fireRoom(s.State, resetSeat, s)
		}
		a.match.room.invalidateReady()
		a.match.room.audit(a.memberID, "seat_changed", "Updated "+s.Name+" in the lobby.")
		return nil
	})
}
func (a *Access) AddSeat(req SeatChange) (GameInfo, error) {
	return a.control("add_seat", req.ID, req.Revision, req, true, false, func() error {
		if err := a.requireLobby(); err != nil {
			return err
		}
		r := a.match.room
		if len(r.Seats) >= 6 {
			return ruleError("invalid_settlements", "A game supports up to six kingdoms.")
		}
		if req.Controller != "human" && req.Controller != "ai" {
			return ruleError("invalid_controller", "Choose a human or AI seat.")
		}
		if !game.ValidCivilization(req.Civilization) {
			return ruleError("invalid_civilization", "Choose a civilization from the catalog.")
		}
		name, err := playerName(req.Name, "New kingdom")
		if err != nil {
			return err
		}
		id, err := randomID(12)
		if err != nil {
			return err
		}
		r.Seats = append(r.Seats, newSeat(id, len(r.Seats)+1, name, req.Civilization, req.Controller))
		r.Config.Settlements = len(r.Seats)
		r.invalidateReady()
		r.audit(a.memberID, "seat_added", "Added "+name+" to the lobby.")
		return nil
	})
}
func (a *Access) ChangeRules(req RulesChange) (GameInfo, error) {
	return a.control("rules", req.ID, req.Revision, req, true, false, func() error {
		if err := a.requireLobby(); err != nil {
			return err
		}
		if err := validateRoomConfig(req.Config); err != nil {
			return err
		}
		cfg, err := game.NormalizeConfig(req.Config)
		if err != nil {
			return err
		}
		if cfg.Settlements != len(a.match.room.Seats) {
			return ruleError("invalid_settlements", "Configure the lobby seats separately.")
		}
		cfg.Name = strings.TrimSpace(cfg.Name)
		if len([]rune(cfg.Name)) > 80 || cfg.Name == "" {
			return ruleError("invalid_name", "Give this game a name of up to 80 characters.")
		}
		a.match.room.Config = cfg
		a.match.room.invalidateReady()
		a.match.room.audit(a.memberID, "rules_changed", "Updated the world settings and cleared optional readiness signals.")
		return nil
	})
}
func (a *Access) Ready(id string, req ReadyRequest) (GameInfo, error) {
	return a.control("ready:"+id, req.ID, req.Revision, req, false, true, func() error {
		if req.Revision < a.match.room.RosterRevision || req.Revision > a.match.room.Revision {
			return ruleError("stale_revision", "World settings or seats changed. Review the lobby and ready again.")
		}
		if err := a.requireLobby(); err != nil {
			return err
		}
		s, err := a.findSeat(id)
		if err != nil {
			return err
		}
		if s.MemberID != a.memberID {
			return ErrForbidden
		}
		event := clearReady
		if req.Ready {
			event = markReady
		}
		return fireRoom(s.Readiness, event, s)
	})
}
func (a *Access) Start(req GameControl) (GameInfo, error) {
	return a.control("start", req.ID, req.Revision, req, true, true, func() error {
		if err := a.requireLobby(); err != nil {
			return err
		}
		m, r := a.match, a.match.room
		if req.Revision < r.RosterRevision || req.Revision > r.Revision {
			return ruleError("stale_revision", "World settings or seats changed. Review the lobby before starting.")
		}
		if err := r.canStart(); err != nil {
			return err
		}
		roster := []game.Kingdom{}
		for _, s := range r.Seats {
			roster = append(roster, game.Kingdom{UserID: s.MemberID, Name: s.Name, Civilization: s.Civilization, Human: s.Controller == "human"})
		}
		w, err := game.NewWorldForRoster(r.Config, roster)
		if err != nil {
			return err
		}
		if err = fireRoom(r.Session, startGame, &roomContext{Match: m, World: w, Actor: a.memberID}); err != nil {
			return err
		}
		r.OpenedAt = time.Now()
		// Starting is the owner's explicit choice to play with whoever is here.
		// A missing claimed player only triggers disconnect policy after joining.
		for _, s := range r.Seats {
			if s.Controller == "human" && s.State.State() == seatClaimed && !r.connected(s.MemberID) {
				r.AbsenceAcknowledged[s.MemberID] = true
			}
		}
		r.audit(a.memberID, "started", "Started the game.")
		return nil
	})
}
func (a *Access) Pause(req GameControl) (GameInfo, error) {
	return a.control("pause", req.ID, req.Revision, req, false, true, func() error {
		if _, err := a.valid(true); err != nil {
			return err
		}
		if a.match.world.Status() == "finished" {
			return ruleError("game_finished", "This game has finished and can only be viewed.")
		}
		if err := a.match.world.SetPaused(true); err != nil {
			return err
		}
		a.match.room.audit(a.memberID, "paused", "Paused the shared game.")
		return nil
	})
}
func (a *Access) ResumeGame(req GameControl) (GameInfo, error) {
	return a.control("resume", req.ID, req.Revision, req, true, false, func() error {
		if _, err := a.valid(true); err != nil {
			return err
		}
		if a.match.world.Status() == "finished" {
			return ruleError("game_finished", "This game has finished and can only be viewed.")
		}
		if err := a.match.world.SetPaused(false); err != nil {
			return err
		}
		r := a.match.room
		for _, s := range r.Seats {
			if s.Controller == "human" && !r.connected(s.MemberID) {
				r.AbsenceAcknowledged[s.MemberID] = true
			}
		}
		r.audit(a.memberID, "resumed", "Resumed the shared game.")
		return nil
	})
}
func (a *Access) Speed(req GameControl) (GameInfo, error) {
	return a.control("speed", req.ID, req.Revision, req, true, false, func() error {
		_, err := a.valid(true)
		if err != nil {
			return err
		}
		if err = a.match.world.SetSpeed(req.Value); err != nil {
			return err
		}
		a.match.room.audit(a.memberID, "speed_changed", "Changed the shared game speed.")
		return nil
	})
}
func (a *Access) SaveGame() (GameInfo, error) {
	m := a.match
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := a.valid(false); err != nil {
		return GameInfo{}, err
	}
	if err := m.save(time.Now()); err != nil {
		return GameInfo{}, err
	}
	return a.info(), nil
}

// A failed close stays in Closing with an immutable clock. Retry and Cancel are
// explicit operations; SQLite never runs inside a state-machine transition.
func (a *Access) CloseGame(req GameControl) (GameInfo, error) {
	m := a.match
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := a.valid(false); err != nil {
		return GameInfo{}, err
	}
	if err := a.owner(); err != nil {
		return GameInfo{}, err
	}
	r := m.room
	if req.ID == "" || len(req.ID) > 64 {
		return GameInfo{}, ruleError("invalid_command_id", "Provide a close request ID.")
	}
	raw, _ := json.Marshal(req)
	hash := sha256.Sum256(append([]byte("close:"), raw...))
	key := a.memberID + ":" + req.ID
	if old, ok := r.Controls[key]; ok {
		if old.Hash != hash || old.Kind != "close" {
			return GameInfo{}, ruleError("idempotency_conflict", "This request ID was already used.")
		}
		if r.Session.State() != sessionClosing {
			return a.info(), nil
		}
	}
	if r.Session.State() == sessionClosed {
		return a.info(), nil
	}
	if r.Runtime.State() != runtimeServing {
		return GameInfo{}, ruleError("transfer_frozen", "Finish or cancel the move before closing this game.")
	}
	if r.Session.State() != sessionClosing {
		if req.Revision != r.Revision {
			return GameInfo{}, ruleError("stale_revision", "The game changed. Refresh and try again.")
		}
		if err := fireRoom(r.Session, beginClose, &roomContext{Match: m}); err != nil {
			return GameInfo{}, err
		}
		r.Revision++
		r.audit(a.memberID, "closing", "Closing the game for everyone.")
	}
	r.Controls[key] = controlReceipt{Hash: hash, Kind: "close"}
	if err := m.save(time.Now()); err != nil {
		return GameInfo{}, err
	}
	before := r.stored()
	if err := fireRoom(r.Session, completeClose, &roomContext{Match: m}); err != nil {
		return GameInfo{}, err
	}
	r.audit(a.memberID, "closed", "Closed and saved the game.")
	if err := m.save(time.Now()); err != nil {
		m.room, _ = restoreRoom(before)
		return GameInfo{}, err
	}
	r.disconnectAll()
	return a.info(), nil
}
func (a *Access) Reopen(req GameControl) (GameInfo, error) {
	return a.control("reopen", req.ID, req.Revision, req, true, false, func() error {
		r := a.match.room
		if r.Runtime.State() != runtimeServing {
			return ruleError("transfer_frozen", "Finish or cancel the move first.")
		}
		if err := fireRoom(r.Session, reopenGame, &roomContext{Match: a.match}); err != nil {
			return err
		}
		r.OpenedAt = time.Now()
		r.audit(a.memberID, "reopened", "Reopened the saved game, paused.")
		return nil
	})
}
func (a *Access) CancelClose(req GameControl) (GameInfo, error) {
	return a.control("cancel_close", req.ID, req.Revision, req, true, false, func() error {
		if err := fireRoom(a.match.room.Session, cancelClose, &roomContext{Match: a.match}); err != nil {
			return err
		}
		a.match.room.audit(a.memberID, "close_cancelled", "Cancelled closing. The game remains paused.")
		return nil
	})
}
func (s *Service) DeleteGame(a *Access, req GameControl) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lifecycle.State() != serviceServing {
		return ErrShuttingDown
	}
	m := a.match
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.room.Session.State() == sessionDeleted {
		return nil
	}
	if m.room.Session.State() != sessionDeleting {
		if _, err := a.valid(false); err != nil {
			return err
		}
	}
	if err := a.owner(); err != nil {
		return err
	}
	if !req.Confirm || req.ID == "" {
		return ruleError("confirmation_required", "Confirm deleting this game and all of its local saves and history.")
	}
	if m.room.Session.State() != sessionDeleting {
		if req.Revision != m.room.Revision {
			return ruleError("stale_revision", "The game changed. Refresh before deleting.")
		}
		if err := fireRoom(m.room.Session, beginDelete, &roomContext{Match: m}); err != nil {
			return err
		}
	}
	if s.db != nil {
		if err := s.deleteDatabase(m, m.room.Epoch); err != nil {
			return err
		}
	}
	s.deleted[m.id] = true
	for _, bindings := range s.browsers {
		delete(bindings, m.id)
	}
	_ = fireRoom(m.room.Session, completeDelete, &roomContext{Match: m})
	m.room.disconnectAll()
	m.fireLease(releaseLease, time.Now())
	delete(s.matches, m.id)
	return nil
}
