package matches

import (
	"encoding/json"
	"fmt"
	"github.com/open-ships/statemachine"
	"strings"
	"time"
)

func (a *Access) revokeInvites(seatID string) {
	for _, i := range a.match.room.Invites {
		if (seatID == "" || i.SeatID == seatID) && i.State.State() == inviteIssued {
			_ = fireRoom(i.State, revokeInvitation, i)
		}
	}
}
func (a *Access) Invite(seatID string, req InviteRequest) (Invitation, error) {
	var result Invitation
	_, err := a.control("invite:"+seatID, req.ID, req.Revision, req, true, false, func() error {
		r := a.match.room
		if (r.Session.State() != sessionLobby && r.Session.State() != sessionOpen) || r.Runtime.State() != runtimeServing {
			return ruleError("game_unavailable", "Reopen this game before inviting a player.")
		}
		s, err := a.findSeat(seatID)
		if err != nil {
			return err
		}
		if s.Controller != "human" || s.MemberID == r.OwnerID {
			return ruleError("invalid_seat", "Choose a friend seat.")
		}
		replacing := s.State.State() == seatClaimed
		if replacing && !req.Replace {
			return ruleError("replacement_required", "Confirm replacing this player's access. Their kingdom will be preserved.")
		}
		secret, err := randomID(32)
		if err != nil {
			return err
		}
		id, err := randomID(16)
		if err != nil {
			return err
		}
		a.revokeInvites(seatID)
		if s.State.State() == seatClaimed {
			old := s.MemberID
			_ = fireRoom(s.State, retireSeat, s)
			_ = fireRoom(s.State, resetSeat, s)
			for _, c := range r.Connections {
				if c.MemberID == old {
					_ = fireRoom(c.State, disconnectBrowser, c)
				}
			}
		}
		_ = fireRoom(s.State, reserveSeat, s)
		i := &invitation{ID: id, SeatID: seatID, Hash: tokenHash(secret), ExpiresAt: time.Now().Add(24 * time.Hour), State: statemachine.NewInstance(inviteMachine, inviteIssued)}
		r.Invites[id] = i
		if r.Session.State() == sessionLobby {
			r.invalidateReady()
		} else if replacing {
			_ = a.match.world.SetPaused(true)
		}
		r.audit(a.memberID, "invited", "Issued an invitation for "+s.Name+".")
		result = inviteView(a.match, i)
		result.Secret = secret
		return nil
	})
	if err == nil && result.ID == "" {
		return result, ruleError("invitation_already_issued", "This invitation was already issued. Create a replacement if you lost its link.")
	}
	return result, err
}
func inviteView(m *Match, i *invitation) Invitation {
	v := Invitation{ID: i.ID, GameID: m.id, GameName: m.room.Config.Name, SeatID: i.SeatID, ExpiresAt: i.ExpiresAt.UTC().Format(time.RFC3339)}
	for _, s := range m.room.Seats {
		if s.ID == i.SeatID {
			v.Name = s.Name
		}
	}
	return v
}
func (a *Access) RevokeInvite(id string, req GameControl) (GameInfo, error) {
	return a.control("revoke_invite:"+id, req.ID, req.Revision, req, true, false, func() error {
		i := a.match.room.Invites[id]
		if i == nil {
			return ErrNotFound
		}
		if i.State.State() == inviteIssued {
			_ = fireRoom(i.State, revokeInvitation, i)
			a.match.room.audit(a.memberID, "invite_revoked", "Revoked a pending invitation.")
		}
		return nil
	})
}

// Secret lookup uses an indexed hash in SQLite. Neither game names nor IDs can
// reclaim membership. The service lock serializes concurrent invite claims.
func (s *Service) secretGame(secret, kind string) (string, error) {
	if len(secret) != 64 {
		return "", ErrUnauthorized
	}
	hash := tokenHash(secret)
	if s.db != nil {
		var id string
		if err := s.db.QueryRow("SELECT game_id FROM game_secrets WHERE hash=? AND kind=?", fmt.Sprintf("%x", hash), kind).Scan(&id); err != nil {
			return "", ErrUnauthorized
		}
		return id, nil
	}
	for id, m := range s.matches {
		m.mu.Lock()
		found := false
		if m.room != nil {
			if kind == "invite" {
				for _, i := range m.room.Invites {
					if hashEqual(hash, i.Hash) && i.State.State() == inviteIssued {
						found = true
					}
				}
			} else {
				for _, p := range m.room.Seats {
					if p.State.State() == seatClaimed && hashEqual(hash, p.RejoinHash) {
						found = true
					}
				}
			}
		}
		m.mu.Unlock()
		if found {
			return id, nil
		}
	}
	return "", ErrUnauthorized
}
func (s *Service) InspectInvite(secret string) (Invitation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lifecycle.State() != serviceServing {
		return Invitation{}, ErrShuttingDown
	}
	id, err := s.secretGame(secret, "invite")
	if err != nil {
		return Invitation{}, err
	}
	m, err := s.load(id)
	if err != nil {
		return Invitation{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	i, err := m.validInvite(secret)
	if err != nil {
		return Invitation{}, err
	}
	return inviteView(m, i), nil
}
func (m *Match) validInvite(secret string) (*invitation, error) {
	if m.room == nil || m.room.Runtime.State() != runtimeServing || (m.room.Session.State() != sessionLobby && m.room.Session.State() != sessionOpen) {
		return nil, ruleError("game_unavailable", "This game is closed or moving. Ask the owner to reopen it.")
	}
	for _, i := range m.room.Invites {
		if hashEqual(i.Hash, tokenHash(secret)) && i.State.State() == inviteIssued {
			if !time.Now().Before(i.ExpiresAt) {
				return nil, ruleError("invite_expired", "This invitation expired. Ask the owner for another link.")
			}
			return i, nil
		}
	}
	return nil, ruleError("invite_unavailable", "This invitation was already claimed or revoked.")
}
func (s *Service) ClaimInvite(req ClaimInvite, browser string) (MemberSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lifecycle.State() != serviceServing {
		return MemberSession{}, ErrShuttingDown
	}
	if req.ID == "" || len(req.ID) > 64 {
		return MemberSession{}, ruleError("invalid_command_id", "Provide a claim request ID.")
	}
	id, err := s.secretGame(req.Secret, "invite")
	if err != nil {
		return MemberSession{}, err
	}
	m, err := s.load(id)
	if err != nil {
		return MemberSession{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	i, err := m.validInvite(req.Secret)
	if err != nil {
		return MemberSession{}, err
	}
	a := &Access{match: m}
	p, err := a.findSeat(i.SeatID)
	if err != nil {
		return MemberSession{}, err
	}
	if b, err := s.binding(browser, id); err == nil {
		for _, existing := range m.room.Seats {
			if existing.MemberID == b.MemberID && existing.State.State() == seatClaimed {
				return MemberSession{}, ruleError("already_member", "This browser already controls a kingdom in this game. Use a separate browser profile for another player.")
			}
		}
	}
	name, err := playerName(req.Name, p.Name)
	if err != nil {
		return MemberSession{}, err
	}
	before := m.room.stored()
	oldRoom := m.room
	oldName := p.Name
	result, err := m.issueCredentials(p, true)
	if err != nil {
		return MemberSession{}, err
	}
	p.Name = name
	if err = fireRoom(i.State, claimInvitation, i); err != nil {
		return MemberSession{}, err
	}
	if err = fireRoom(p.State, claimSeat, p); err != nil {
		return MemberSession{}, err
	}
	if m.world != nil {
		m.world.Players[p.PlayerID].Name = name
	} else {
		m.room.invalidateReady()
	}
	m.room.Revision++
	m.room.audit(p.MemberID, "joined", name+" claimed their kingdom.")
	if err = s.saveMembership(browser, m, p); err != nil {
		m.room, _ = restoreRoom(before)
		m.room.Connections = oldRoom.Connections
		m.room.AbsentSince = oldRoom.AbsentSince
		m.room.AbsenceAcknowledged = oldRoom.AbsenceAcknowledged
		m.room.OpenedAt = oldRoom.OpenedAt
		if m.world != nil {
			m.world.Players[p.PlayerID].Name = oldName
		}
		return MemberSession{}, err
	}
	return result, nil
}
func (s *Service) Rejoin(req RejoinRequest, browser string) (MemberSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lifecycle.State() != serviceServing {
		return MemberSession{}, ErrShuttingDown
	}
	code := strings.TrimSpace(req.Code)
	id, err := s.secretGame(code, "rejoin")
	if err != nil {
		return MemberSession{}, err
	}
	m, err := s.load(id)
	if err != nil {
		return MemberSession{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range m.room.Seats {
		if p.State.State() != seatClaimed || !hashEqual(p.RejoinHash, tokenHash(code)) {
			continue
		}
		oldHash, oldVersion := p.TokenHash, p.CredentialVersion
		result, err := m.issueCredentials(p, false)
		if err != nil {
			return MemberSession{}, err
		}
		m.room.audit(p.MemberID, "rejoined", p.Name+" recovered their membership.")
		if err = s.saveMembership(browser, m, p); err != nil {
			p.TokenHash = oldHash
			p.CredentialVersion = oldVersion
			m.room.Audit = m.room.Audit[:len(m.room.Audit)-1]
			return MemberSession{}, err
		}
		return result, nil
	}
	return MemberSession{}, ErrUnauthorized
}
func (s *Service) Games(browser, query string) (GameLibrary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lifecycle.State() != serviceServing {
		return GameLibrary{}, ErrShuttingDown
	}
	v := GameLibrary{Games: []GameInfo{}}
	if browser == "" {
		return v, nil
	}
	hash := fmt.Sprintf("%x", tokenHash(browser))
	bindings := []browserBinding{}
	if s.db != nil {
		rows, err := s.db.Query("SELECT game_id,member_id,version FROM browser_members WHERE browser_hash=? LIMIT 100", hash)
		if err != nil {
			return v, err
		}
		for rows.Next() {
			var b browserBinding
			if err = rows.Scan(&b.GameID, &b.MemberID, &b.Version); err != nil {
				rows.Close()
				return v, err
			}
			bindings = append(bindings, b)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return v, err
		}
	} else {
		for _, b := range s.browsers[hash] {
			bindings = append(bindings, b)
		}
	}
	query = strings.ToLower(strings.TrimSpace(query))
	for _, b := range bindings {
		c, err := s.roomRecord(b.GameID)
		if err != nil {
			continue
		}
		r, err := restoreRoom(c)
		if err != nil {
			return v, err
		}
		valid := false
		for _, p := range r.Seats {
			if p.MemberID == b.MemberID && p.CredentialVersion == b.Version && p.State.State() == seatClaimed {
				valid = true
			}
		}
		if !valid || (!strings.Contains(strings.ToLower(r.Config.Name), query) && !strings.Contains(b.GameID, query)) {
			continue
		}
		m := s.matches[b.GameID]
		if m != nil {
			m.mu.Lock()
			a := &Access{match: m, memberID: b.MemberID}
			v.Games = append(v.Games, a.info())
			m.mu.Unlock()
		} else {
			var info SavedGame
			var data []byte
			if s.db != nil {
				if err = s.db.QueryRow("SELECT metadata FROM sessions WHERE id=?", b.GameID).Scan(&data); err != nil {
					continue
				}
				if err = json.Unmarshal(data, &info); err != nil {
					return v, err
				}
			}
			temporary := &Match{id: b.GameID, room: r, savedAt: info.SavedAt}
			a := &Access{match: temporary, memberID: b.MemberID}
			g := a.info()
			g.Time = info.Time
			if r.Session.State() != sessionLobby {
				g.MatchStatus = "paused"
			}
			v.Games = append(v.Games, g)
		}
	}
	return v, nil
}
