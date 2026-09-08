package matches

import (
	"crowns/internal/game"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"github.com/open-ships/statemachine"
	"strings"
	"time"
)

func (s *Service) CreateGame(req CreateGame, browser string) (MemberSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lifecycle.State() != serviceServing {
		return MemberSession{}, ErrShuttingDown
	}
	if len(s.matches) >= 16 {
		return MemberSession{}, ErrCapacity
	}
	if err := validateRoomConfig(req.Config); err != nil {
		return MemberSession{}, err
	}
	cfg, err := game.NormalizeConfig(req.Config)
	if err != nil {
		return MemberSession{}, err
	}
	cfg.Name = strings.TrimSpace(cfg.Name)
	if len([]rune(cfg.Name)) > 80 {
		return MemberSession{}, ruleError("invalid_name", "Use a game name of up to 80 characters.")
	}
	if req.Friends < 0 || req.Friends >= cfg.Settlements {
		return MemberSession{}, ruleError("invalid_friends", "Friend seats must fit within the settlement count.")
	}
	name, err := playerName(req.PlayerName, "Your kingdom")
	if err != nil {
		return MemberSession{}, err
	}
	id, err := randomID(16)
	if err != nil {
		return MemberSession{}, err
	}
	epoch, err := randomID(16)
	if err != nil {
		return MemberSession{}, err
	}
	if cfg.Name == "" {
		cfg.Name = "Kingdom " + id[:8]
	}
	r := &room{Config: cfg, Epoch: epoch, Invites: map[string]*invitation{}, Controls: map[string]controlReceipt{}, Session: statemachine.NewInstance(sessionMachine, sessionLobby), Runtime: statemachine.NewInstance(runtimeMachine, runtimeServing), Connections: map[string]*connection{}, AbsentSince: map[string]time.Time{}, AbsenceAcknowledged: map[string]bool{}, OpenedAt: time.Now()}
	civs := game.GetCatalog().Civilizations
	for i := 0; i < cfg.Settlements; i++ {
		sid, err := randomID(12)
		if err != nil {
			return MemberSession{}, err
		}
		controller, civ, label := "ai", civs[i%len(civs)].ID, fmt.Sprintf("Kingdom %d", i+1)
		if i <= req.Friends {
			controller = "human"
			label = fmt.Sprintf("Friend %d", i)
		}
		if i == 0 {
			civ = cfg.Civilization
			label = name
		}
		r.Seats = append(r.Seats, newSeat(sid, i+1, label, civ, controller))
	}
	m := &Match{id: id, db: s.db, room: r, commands: map[string]cachedCommand{}, lastAccess: time.Now(), lifecycle: statemachine.NewInstance(leaseMachine, leaseOpen)}
	owner := r.Seats[0]
	_ = fireRoom(owner.State, reserveSeat, owner)
	_ = fireRoom(owner.State, claimSeat, owner)
	session, err := m.issueCredentials(owner, true)
	if err != nil {
		return MemberSession{}, err
	}
	r.OwnerID = owner.MemberID
	session.Owner = true
	r.audit(owner.MemberID, "created", "Created the private lobby.")
	if err = s.saveMembership(browser, m, owner); err != nil {
		return MemberSession{}, err
	}
	s.matches[id] = m
	return session, nil
}
func playerName(name, fallback string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = fallback
	}
	if len([]rune(name)) > 40 {
		return "", ruleError("invalid_name", "Use a player name of up to 40 characters.")
	}
	return name, nil
}
func (m *Match) issueCredentials(p *seat, newMember bool) (MemberSession, error) {
	token, err := randomID(32)
	if err != nil {
		return MemberSession{}, err
	}
	code := ""
	if newMember {
		p.MemberID, err = randomID(16)
		if err != nil {
			return MemberSession{}, err
		}
		code, err = randomID(32)
		if err != nil {
			return MemberSession{}, err
		}
		p.RejoinHash = tokenHash(code)
	}
	p.TokenHash = tokenHash(token)
	p.CredentialVersion++
	return MemberSession{MatchID: m.id, Token: token, PlayerID: p.PlayerID, Name: m.room.Config.Name, MembershipID: p.MemberID, RejoinCode: code, Epoch: m.room.Epoch, Owner: p.MemberID == m.room.OwnerID}, nil
}
func hashEqual(a, b [32]byte) bool { return subtle.ConstantTimeCompare(a[:], b[:]) == 1 }
func (s *Service) Access(id, token, browser string) (*Access, error) {
	return s.access(id, token, browser, false)
}

// RequestAccess retains residency until the HTTP handler releases it. Durable
// close and transfer still reject gameplay immediately, but their unloaded
// checkpoints cannot invalidate a newly authorized Reopen or archive download.
func (s *Service) RequestAccess(id, token, browser string) (*Access, error) {
	return s.access(id, token, browser, true)
}
func (s *Service) access(id, token, browser string, hold bool) (*Access, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lifecycle.State() != serviceServing {
		return nil, ErrShuttingDown
	}
	c, err := s.roomRecord(id)
	if err != nil {
		return nil, err
	}
	b, _ := s.binding(browser, id)
	found := ""
	version := 0
	for _, p := range c.Seats {
		if p.State != seatClaimed {
			continue
		}
		allowed := token != "" && hashEqual(p.TokenHash, tokenHash(token))
		if token == "" {
			allowed = b.MemberID == p.MemberID && b.Version == p.CredentialVersion
		}
		if allowed {
			found = p.MemberID
			version = p.CredentialVersion
			break
		}
	}
	if found == "" {
		return nil, ErrUnauthorized
	}
	m, err := s.load(id)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	a := &Access{match: m, memberID: found, epoch: c.Epoch, version: version}
	if _, err = a.valid(false); err != nil {
		return nil, err
	}
	m.lastAccess = time.Now()
	if hold {
		m.activeRequests++
		a.held = true
	}
	return a, nil
}

// AccessLegacy is only for the original /matches adapter. It cannot authorize
// multiplayer games, so a friend can never enter a player-1 compatibility path.
func (s *Service) AccessLegacy(id, token string) (*Access, error) {
	m, err := s.Authorized(id, token)
	if err != nil {
		return nil, err
	}
	return &Access{match: m}, nil
}
func (a *Access) valid(play bool) (*seat, error) {
	m := a.match
	if err := m.available(); err != nil {
		return nil, err
	}
	if m.room == nil {
		if a.memberID != "" {
			return nil, ErrUnauthorized
		}
		return nil, nil
	}
	r := m.room
	if a.epoch != r.Epoch {
		return nil, ErrUnauthorized
	}
	var p *seat
	for _, s := range r.Seats {
		if s.MemberID == a.memberID && s.CredentialVersion == a.version && s.State.State() == seatClaimed {
			p = s
			break
		}
	}
	if p == nil {
		return nil, ErrUnauthorized
	}
	if r.Session.State() == sessionDeleted {
		return nil, ErrNotFound
	}
	if play && (r.Session.State() != sessionOpen || r.Runtime.State() != runtimeServing || m.world == nil) {
		return nil, ruleError("game_unavailable", "Open and resume this game before playing.")
	}
	return p, nil
}
func (a *Access) owner() error {
	if a.match.room.OwnerID != a.memberID {
		return ErrForbidden
	}
	return nil
}

var ErrForbidden = fmt.Errorf("only the game owner can do that")

func (a *Access) memberSession() MemberSession {
	r := a.match.room
	for _, p := range r.Seats {
		if p.MemberID == a.memberID {
			return MemberSession{MatchID: a.match.id, Name: r.Config.Name, PlayerID: p.PlayerID, MembershipID: p.MemberID, Epoch: r.Epoch, Owner: p.MemberID == r.OwnerID}
		}
	}
	return MemberSession{}
}
func (a *Access) Session() (MemberSession, error) {
	a.match.mu.Lock()
	defer a.match.mu.Unlock()
	if _, err := a.valid(false); err != nil {
		return MemberSession{}, err
	}
	return a.memberSession(), nil
}
func (a *Access) info() GameInfo {
	m, r := a.match, a.match.room
	v := GameInfo{GameID: m.id, Name: r.Config.Name, Config: r.Config, Status: string(r.Session.State()), Runtime: string(r.Runtime.State()), Epoch: r.Epoch, Revision: r.Revision, Seats: []SeatView{}, Owner: r.OwnerID == a.memberID, SavedAt: m.savedAt, SaveError: m.saveError, MatchStatus: "not_started"}
	if m.world != nil {
		v.Time = m.world.Time
		v.MatchStatus = m.world.Status()
	}
	for _, p := range r.Seats {
		sv := SeatView{ID: p.ID, PlayerID: p.PlayerID, MembershipID: p.MemberID, Name: p.Name, Civilization: p.Civilization, Controller: p.Controller, Status: string(p.State.State()), Ready: p.Readiness.State() == readyYes, Owner: p.MemberID == r.OwnerID, Yours: p.MemberID == a.memberID}
		if sv.Yours {
			v.PlayerID = p.PlayerID
		}
		for _, c := range r.Connections {
			if c.MemberID == p.MemberID && c.State.State() == connectionConnected {
				sv.Connected = true
				break
			}
		}
		if v.Owner {
			for _, i := range r.Invites {
				if i.SeatID == p.ID && i.State.State() == inviteIssued && time.Now().Before(i.ExpiresAt) {
					sv.InviteID = i.ID
					sv.InviteExpiresAt = i.ExpiresAt.UTC().Format(time.RFC3339)
				}
			}
		}
		v.Seats = append(v.Seats, sv)
	}
	v.CanStart = v.Owner && r.Session.State() == sessionLobby && r.canStart() == nil
	if r.Session.State() == sessionLobby {
		if err := r.canStart(); err != nil {
			v.StartReason = err.Error()
		} else if !v.Owner {
			v.StartReason = "Waiting for the owner to start. Ready is optional."
		} else {
			v.StartReason = "Start whenever you like. Ready is optional; unclaimed kingdoms stay reserved for friends to join later."
		}
	}
	if r.Transfer != nil {
		v.TransferID = r.Transfer.ID
	}
	return v
}
func (a *Access) Info() (GameInfo, error) {
	m := a.match
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := a.valid(false); err != nil {
		return GameInfo{}, err
	}
	return a.info(), nil
}
func (a *Access) View() (game.Snapshot, error) {
	m := a.match
	m.mu.Lock()
	defer m.mu.Unlock()
	p, err := a.valid(true)
	if err != nil {
		return game.Snapshot{}, err
	}
	id := 1
	if p != nil {
		id = p.PlayerID
	}
	m.lastAccess = time.Now()
	v := m.world.View(id)
	if m.room != nil {
		v.ControlRevision = m.room.Revision
	}
	return v, nil
}
func (a *Access) Log(q game.LogQuery) (game.EventPage, error) {
	m := a.match
	m.mu.Lock()
	defer m.mu.Unlock()
	p, err := a.valid(true)
	if err != nil {
		return game.EventPage{}, err
	}
	id := 1
	if p != nil {
		id = p.PlayerID
	}
	return m.world.Log(id, q)
}
func (a *Access) Placement(req Placement) (PlacementResult, error) {
	m := a.match
	m.mu.Lock()
	defer m.mu.Unlock()
	p, err := a.valid(true)
	if err != nil {
		return PlacementResult{}, err
	}
	id := 1
	if p != nil {
		id = p.PlayerID
	}
	err = m.world.Placement(id, req.Product, req.Position)
	v := PlacementResult{Valid: err == nil}
	if err != nil {
		v.Reason = err.Error()
	}
	return v, nil
}
func (a *Access) Apply(c game.Command) (Receipt, error) {
	if a.memberID == "" {
		return a.match.Apply(c)
	}
	m := a.match
	m.mu.Lock()
	defer m.mu.Unlock()
	p, err := a.valid(true)
	if err != nil {
		return Receipt{}, err
	}
	if c.Kind == "pause" || c.Kind == "resume" || c.Kind == "speed" {
		return Receipt{}, ruleError("shared_control", "Use the explicit shared game controls.")
	}
	if c.ID == "" || len(c.ID) > 64 {
		return Receipt{}, ruleError("invalid_command_id", "Use a unique command ID of 1–64 characters.")
	}
	data, err := json.Marshal(c)
	if err != nil {
		return Receipt{}, err
	}
	hash := sha256.Sum256(data)
	key := p.MemberID + ":" + c.ID
	if old, ok := m.commands[key]; ok {
		if old.hash != hash {
			return Receipt{}, ruleError("idempotency_conflict", "This command ID was already used for another request.")
		}
		return old.receipt, old.err
	}
	if len(m.commands) >= 50000 {
		return Receipt{}, ruleError("command_limit", "This game has reached its command limit.")
	}
	now := time.Now()
	if now.Sub(m.window) >= time.Second {
		m.window = now
		m.windowCommands = 0
	}
	if now.Sub(p.window) >= time.Second {
		p.window = now
		p.commands = 0
	}
	if m.windowCommands >= 180 || p.commands >= 60 {
		return Receipt{}, ruleError("rate_limited", "Too many commands. Try again shortly.")
	}
	m.windowCommands++
	p.commands++
	err = m.world.Apply(p.PlayerID, c)
	v := Receipt{CommandID: c.ID, Tick: m.world.Tick, Accepted: err == nil}
	m.commands[key] = cachedCommand{hash, v, err}
	m.lastAccess = now
	return v, err
}
func (a *Access) Audit(after int) (AuditPage, error) {
	m := a.match
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := a.valid(false); err != nil {
		return AuditPage{}, err
	}
	v := AuditPage{Events: []AuditEvent{}}
	for _, e := range m.room.Audit {
		if e.ID > after {
			v.Events = append(v.Events, e)
			if len(v.Events) == 200 {
				break
			}
		}
	}
	return v, nil
}

func validateRoomConfig(cfg game.Config) error {
	if cfg.Civilization != "" && !game.ValidCivilization(cfg.Civilization) {
		return ruleError("invalid_civilization", "Choose a civilization from the catalog.")
	}
	if cfg.Difficulty != "" && !game.ValidDifficulty(cfg.Difficulty) {
		return ruleError("invalid_difficulty", "Choose a difficulty from the catalog.")
	}
	return nil
}

func (a *Access) Release() {
	m := a.match
	m.mu.Lock()
	defer m.mu.Unlock()
	if a.held {
		a.held = false
		m.activeRequests--
	}
}
