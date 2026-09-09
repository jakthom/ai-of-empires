package matches

import (
	"context"
	"crowns/internal/game"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/open-ships/statemachine"
	"time"
)

type storedSeat struct {
	ID, MemberID, Name, Civilization, Controller string
	PlayerID, CredentialVersion                  int
	TokenHash, RejoinHash                        [32]byte
	State                                        seatState
	Readiness                                    readyState
}
type storedInvite struct {
	ID, SeatID string
	Hash       [32]byte
	ExpiresAt  time.Time
	State      inviteState
}
type storedRoom struct {
	RosterRevision int
	Config         game.Config
	Epoch, OwnerID string
	Revision       int
	Seats          []storedSeat
	Invites        []storedInvite
	Controls       map[string]controlReceipt
	Audit          []AuditEvent
	Transfer       *transferRecord
	Session        sessionState
	Runtime        runtimeState
}

func (r *room) stored() *storedRoom {
	c := &storedRoom{RosterRevision: r.RosterRevision, Config: r.Config, Epoch: r.Epoch, OwnerID: r.OwnerID, Revision: r.Revision, Controls: map[string]controlReceipt{}, Audit: append([]AuditEvent(nil), r.Audit...), Session: r.Session.State(), Runtime: r.Runtime.State()}
	if r.Transfer != nil {
		t := *r.Transfer
		c.Transfer = &t
	}
	for k, v := range r.Controls {
		c.Controls[k] = v
	}
	for _, s := range r.Seats {
		c.Seats = append(c.Seats, storedSeat{s.ID, s.MemberID, s.Name, s.Civilization, s.Controller, s.PlayerID, s.CredentialVersion, s.TokenHash, s.RejoinHash, s.State.State(), s.Readiness.State()})
	}
	for _, i := range r.Invites {
		c.Invites = append(c.Invites, storedInvite{i.ID, i.SeatID, i.Hash, i.ExpiresAt, i.State.State()})
	}
	return c
}
func restoreRoom(c *storedRoom) (*room, error) {
	if c == nil {
		return nil, nil
	}
	valid := func(v string, values ...string) bool {
		for _, x := range values {
			if x == v {
				return true
			}
		}
		return false
	}
	if len(c.Seats) < 1 || len(c.Seats) > 6 || len(c.Seats) != c.Config.Settlements || c.Epoch == "" || !valid(string(c.Session), "lobby", "open", "closing", "closed", "deleting") || !valid(string(c.Runtime), "serving", "quiescing", "frozen", "released") {
		return nil, errors.New("invalid saved game lifecycle")
	}
	r := &room{RosterRevision: c.RosterRevision, Config: c.Config, Epoch: c.Epoch, OwnerID: c.OwnerID, Revision: c.Revision, Controls: c.Controls, Audit: c.Audit, Transfer: c.Transfer, Invites: map[string]*invitation{}, Session: statemachine.NewInstance(sessionMachine, c.Session), Runtime: statemachine.NewInstance(runtimeMachine, c.Runtime), Connections: map[string]*connection{}, AbsentSince: map[string]time.Time{}, AbsenceAcknowledged: map[string]bool{}, OpenedAt: time.Now()}
	if r.Controls == nil {
		r.Controls = map[string]controlReceipt{}
	}
	ids, members := map[string]bool{}, map[string]bool{}
	ownerFound := false
	for i, s := range c.Seats {
		if s.ID == "" || ids[s.ID] || s.PlayerID != i+1 || !game.ValidCivilization(s.Civilization) || !valid(s.Controller, "human", "ai") || !valid(string(s.State), "vacant", "reserved", "claimed", "retired") || !valid(string(s.Readiness), "not_ready", "ready") {
			return nil, errors.New("invalid saved seat")
		}
		ids[s.ID] = true
		if s.State == seatClaimed {
			if s.MemberID == "" || members[s.MemberID] || s.Controller != "human" {
				return nil, errors.New("invalid membership")
			}
			members[s.MemberID] = true
			ownerFound = ownerFound || s.MemberID == r.OwnerID
		}
		r.Seats = append(r.Seats, &seat{ID: s.ID, MemberID: s.MemberID, Name: s.Name, Civilization: s.Civilization, Controller: s.Controller, PlayerID: s.PlayerID, CredentialVersion: s.CredentialVersion, TokenHash: s.TokenHash, RejoinHash: s.RejoinHash, State: statemachine.NewInstance(seatMachine, s.State), Readiness: statemachine.NewInstance(readyMachine, s.Readiness)})
	}
	if !ownerFound {
		return nil, errors.New("missing game owner")
	}
	for _, i := range c.Invites {
		if !ids[i.SeatID] || !valid(string(i.State), "issued", "claimed", "expired", "revoked") {
			return nil, errors.New("invalid saved invitation")
		}
		r.Invites[i.ID] = &invitation{ID: i.ID, SeatID: i.SeatID, Hash: i.Hash, ExpiresAt: i.ExpiresAt, State: statemachine.NewInstance(inviteMachine, i.State)}
	}
	return r, nil
}

type browserWrite struct {
	Hash    string
	Binding browserBinding
}

// Membership credentials and the browser's recovery binding commit together.
// A failed binding must not consume an invitation or strand a new owner.
func (s *Service) saveMembership(browser string, m *Match, member *seat) error {
	if browser == "" {
		return m.save(time.Now())
	}
	hash := fmt.Sprintf("%x", tokenHash(browser))
	b := browserBinding{m.id, member.MemberID, member.CredentialVersion}
	if err := m.checkpoint(context.Background(), time.Now(), &browserWrite{hash, b}); err != nil {
		return err
	}
	if s.db != nil {
		return nil
	}
	if s.browsers[hash] == nil {
		s.browsers[hash] = map[string]browserBinding{}
	}
	s.browsers[hash][m.id] = b
	return nil
}
func (s *Service) binding(browser, id string) (browserBinding, error) {
	var b browserBinding
	if browser == "" {
		return b, ErrUnauthorized
	}
	hash := fmt.Sprintf("%x", tokenHash(browser))
	if s.db != nil {
		db, err := s.gameDatabase(id)
		if err != nil {
			return b, ErrUnauthorized
		}
		err = db.QueryRow("SELECT game_id,member_id,version FROM browser_members WHERE browser_hash=? AND game_id=?", hash, id).Scan(&b.GameID, &b.MemberID, &b.Version)
		if err != nil {
			return b, ErrUnauthorized
		}
		return b, nil
	}
	b, ok := s.browsers[hash][id]
	if !ok {
		return b, ErrUnauthorized
	}
	return b, nil
}

// Authenticate metadata before restoring an expensive simulation. Caller holds
// the service lock; active matches also require their own lock.
func (s *Service) roomRecord(id string) (*storedRoom, error) {
	if m := s.matches[id]; m != nil {
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.room == nil {
			return nil, ErrUnauthorized
		}
		return m.room.stored(), nil
	}
	if s.db == nil {
		return nil, ErrNotFound
	}
	db, err := s.gameDatabase(id)
	if err != nil {
		return nil, err
	}
	var data []byte
	if err := db.QueryRow("SELECT room FROM sessions WHERE id=?", id).Scan(&data); err != nil {
		return nil, ErrNotFound
	}
	if len(data) == 0 {
		return nil, ErrUnauthorized
	}
	var c storedRoom
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func storedUsers(r *storedRoom) map[int]game.RestoreUser {
	if r == nil {
		return nil
	}
	users := map[int]game.RestoreUser{}
	for _, p := range r.Seats {
		if p.State == seatClaimed {
			user := game.RestoreUser{ID: p.MemberID}
			// The owner cannot be replaced. Older friend seats have no
			// event-to-membership attribution across late joins/replacements.
			if p.MemberID == r.OwnerID {
				user.LegacyID = p.MemberID
			}
			users[p.PlayerID] = user
		}
	}
	return users
}
