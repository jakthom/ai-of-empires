package matches

import (
	"github.com/open-ships/statemachine"
	"time"
)

// AdoptLegacy upgrades an existing local game only after proving possession of
// its original token. The world, game ID, entity identities and journals stay
// intact; no unauthenticated name lookup grants a membership.
func (s *Service) AdoptLegacy(id, token, browser string) (MemberSession, error) {
	old, err := s.Authorized(id, token)
	if err != nil {
		return MemberSession{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lifecycle.State() != serviceServing {
		return MemberSession{}, ErrShuttingDown
	}
	if s.matches[id] != old {
		return MemberSession{}, ErrNotFound
	}
	old.mu.Lock()
	defer old.mu.Unlock()
	if old.room != nil || !hashEqual(old.tokenHash, tokenHash(token)) {
		return MemberSession{}, ErrUnauthorized
	}
	epoch, err := randomID(16)
	if err != nil {
		return MemberSession{}, err
	}
	r := &room{Config: old.world.Config, Epoch: epoch, Invites: map[string]*invitation{}, Controls: map[string]controlReceipt{}, Session: statemachine.NewInstance(sessionMachine, sessionOpen), Runtime: statemachine.NewInstance(runtimeMachine, runtimeServing), Connections: map[string]*connection{}, AbsentSince: map[string]time.Time{}, AbsenceAcknowledged: map[string]bool{}, OpenedAt: time.Now()}
	for id := 1; id <= r.Config.Settlements; id++ {
		p := old.world.Players[id]
		sid, err := randomID(12)
		if err != nil {
			return MemberSession{}, err
		}
		controller := "ai"
		if !p.AI {
			controller = "human"
		}
		r.Seats = append(r.Seats, newSeat(sid, id, p.Name, p.Civilization, controller))
	}
	owner := r.Seats[0]
	_ = fireRoom(owner.State, reserveSeat, owner)
	_ = fireRoom(owner.State, claimSeat, owner)
	old.room = r
	session, err := old.issueCredentials(owner, true)
	if err != nil {
		old.room = nil
		return MemberSession{}, err
	}
	r.OwnerID = owner.MemberID
	session.Owner = true
	_ = old.world.SetPaused(true)
	r.audit(owner.MemberID, "upgraded", "Upgraded this saved game to a private membership. The game remains paused.")
	// Scope old command receipts to the new owner without replaying accepted work.
	commands := old.commands
	old.commands = map[string]cachedCommand{}
	for id, c := range commands {
		old.commands[owner.MemberID+":"+id] = c
	}
	if err = s.saveMembership(browser, old, owner); err != nil {
		old.room = nil
		old.commands = commands
		return MemberSession{}, err
	}
	return session, nil
}
