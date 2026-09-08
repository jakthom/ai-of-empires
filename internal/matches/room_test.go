package matches

import (
	"context"
	"crowns/internal/game"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func roomService(t *testing.T) *Service {
	t.Helper()
	s, err := OpenService(filepath.Join(t.TempDir(), "games.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}
func createLobby(t *testing.T, s *Service, friends int) (MemberSession, *Access) {
	t.Helper()
	v, err := s.CreateGame(CreateGame{Config: game.Config{Name: "Friends' realm", Settlements: friends + 1, Civilization: "britons", Difficulty: "peaceful", Mode: "skirmish", Seed: 4817}, Friends: friends, PlayerName: "Alice"}, "browser-alice")
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.Access(v.MatchID, v.Token, "")
	if err != nil {
		t.Fatal(err)
	}
	return v, a
}
func controlFor(t *testing.T, a *Access, id string) GameControl {
	t.Helper()
	v, err := a.Info()
	if err != nil {
		t.Fatal(err)
	}
	return GameControl{ID: id, Revision: v.Revision}
}
func readySeat(t *testing.T, a *Access) {
	t.Helper()
	v, err := a.Info()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range v.Seats {
		if p.Yours {
			_, err = a.Ready(p.ID, ReadyRequest{ID: "ready-" + p.ID, Revision: v.Revision, Ready: true})
			if err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatal("missing own seat")
}
func joinedGame(t *testing.T, s *Service) (MemberSession, *Access, MemberSession, *Access) {
	t.Helper()
	owner, a := createLobby(t, s, 1)
	v, _ := a.Info()
	invite, err := a.Invite(v.Seats[1].ID, InviteRequest{ID: "invite-bob", Revision: v.Revision})
	if err != nil {
		t.Fatal(err)
	}
	friend, err := s.ClaimInvite(ClaimInvite{ID: "claim-bob", Secret: invite.Secret, Name: "Bob"}, "browser-bob")
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Access(friend.MatchID, friend.Token, "")
	if err != nil {
		t.Fatal(err)
	}
	readySeat(t, a)
	readySeat(t, b)
	if _, err = a.Start(controlFor(t, a, "start")); err != nil {
		t.Fatal(err)
	}
	return owner, a, friend, b
}
func TestMultiplayerLobbyClaimsAndIsolation(t *testing.T) {
	s := roomService(t)
	owner, a := createLobby(t, s, 1)
	if a.match.world != nil {
		t.Fatal("lobby started simulation")
	}
	if info, err := a.Info(); err != nil || !info.CanStart {
		t.Fatal("unclaimed seats blocked the owner from starting", err)
	}
	info, _ := a.Info()
	i, err := a.Invite(info.Seats[1].ID, InviteRequest{ID: "invite", Revision: info.Revision})
	if err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 2; n++ {
		if _, err = s.InspectInvite(i.Secret); err != nil {
			t.Fatal("preview consumed invite", err)
		}
	}
	var mu sync.Mutex
	winners := []MemberSession{}
	var wg sync.WaitGroup
	for n := 0; n < 8; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, e := s.ClaimInvite(ClaimInvite{ID: "claim", Secret: i.Secret, Name: "Bob"}, "")
			if e == nil {
				mu.Lock()
				winners = append(winners, v)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if len(winners) != 1 {
		t.Fatalf("claim winners %d", len(winners))
	}
	friend := winners[0]
	b, err := s.Access(friend.MatchID, friend.Token, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Authorized(owner.MatchID, friend.Token); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("legacy player-1 access: %v", err)
	}
	readySeat(t, a)
	readySeat(t, b)
	if _, err = a.Start(controlFor(t, a, "start")); err != nil {
		t.Fatal(err)
	}
	av, _ := a.View()
	bv, _ := b.View()
	if av.Player.ID != 1 || bv.Player.ID != 2 {
		t.Fatal("shared acting player")
	}
	if a.match.world.Players[2].AI {
		t.Fatal("human seat runs AI")
	}
	findTC := func(v game.Snapshot) int {
		for _, e := range v.Entities {
			if e.Type == "town_center" && e.Owner == v.Player.ID {
				return e.ID
			}
		}
		return 0
	}
	for _, pair := range []struct {
		a *Access
		v game.Snapshot
	}{{a, av}, {b, bv}} {
		if _, err = pair.a.Apply(game.Command{ID: "same-id", Kind: "train", EntityIDs: []int{findTC(pair.v)}, Product: "villager"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = b.Apply(game.Command{ID: "steal", Kind: "train", EntityIDs: []int{findTC(av)}, Product: "villager"}); err == nil {
		t.Fatal("friend controlled owner building")
	}
	if _, err = b.Speed(controlFor(t, b, "speed")); !errors.Is(err, ErrForbidden) {
		t.Fatalf("friend changed speed %v", err)
	}
	if _, err = s.Access(owner.MatchID, "wrong-token", ""); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("bad token authenticated")
	}
	library, err := s.Games("unrelated-browser", "")
	if err != nil || len(library.Games) != 0 {
		t.Fatal("private library leaked")
	}
	library, err = s.Games("browser-alice", "")
	if err != nil || len(library.Games) != 1 {
		t.Fatal("owner library missing", err)
	}
}
func TestMultiplayerPauseCloseRestartAndReceipts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "games.sqlite")
	s, err := OpenService(path)
	if err != nil {
		t.Fatal(err)
	}
	owner, a, friend, b := joinedGame(t, s)
	stale := controlFor(t, a, "stale-resume")
	pause := controlFor(t, b, "pause")
	if _, err = b.Pause(pause); err != nil {
		t.Fatal(err)
	}
	if _, err = b.Pause(pause); err != nil {
		t.Fatal(err)
	}
	if _, err = a.ResumeGame(stale); err == nil {
		t.Fatal("stale resume undid pause")
	}
	if v, _ := a.View(); !v.Paused {
		t.Fatal("pause retry resumed")
	}
	if _, err = a.ResumeGame(controlFor(t, a, "resume")); err != nil {
		t.Fatal(err)
	}
	if _, err = a.CloseGame(controlFor(t, a, "close")); err != nil {
		t.Fatal(err)
	}
	if _, err = b.View(); err == nil {
		t.Fatal("closed game visible to active stream")
	}
	if _, err = b.Reopen(controlFor(t, b, "reopen-friend")); !errors.Is(err, ErrForbidden) {
		t.Fatal("friend reopened")
	}
	if _, err = a.Reopen(controlFor(t, a, "reopen")); err != nil {
		t.Fatal(err)
	}
	if v, _ := a.View(); !v.Paused {
		t.Fatal("reopen resumed")
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenService(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a, err = s.Access(owner.MatchID, owner.Token, "")
	if err != nil {
		t.Fatal(err)
	}
	b, err = s.Access(friend.MatchID, friend.Token, "")
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := b.View(); v.Player.ID != 2 || !v.Paused {
		t.Fatal("restart changed seat or clock")
	}
	if _, err = b.Pause(pause); err != nil {
		t.Fatal("durable control receipt lost", err)
	}
	recovered, err := s.Rejoin(RejoinRequest{Code: friend.RejoinCode}, "new-browser")
	if err != nil {
		t.Fatal(err)
	}
	if recovered.MembershipID != friend.MembershipID || recovered.PlayerID != 2 {
		t.Fatal("rejoin created another kingdom")
	}
	if _, err = b.View(); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("old authorized handle survived credential rotation")
	}
	if _, err = a.View(); err != nil {
		t.Fatal("friend rotation invalidated owner")
	}
}
func TestPresenceTracksTabsAndPausesAfterGrace(t *testing.T) {
	s := roomService(t)
	_, a, _, b := joinedGame(t, s)
	ac, _ := a.Connect()
	bc, _ := b.Connect()
	bc2, _ := b.Connect()
	if err := b.Disconnect(bc.ID); err != nil {
		t.Fatal(err)
	}
	m := a.match
	now := time.Now()
	m.mu.Lock()
	m.pulseRoom(context.Background(), now)
	if m.world.Status() != "running" {
		t.Fatal("one tab departing paused another tab")
	}
	m.mu.Unlock()
	_ = b.Disconnect(bc2.ID)
	m.mu.Lock()
	future := now.Add(DisconnectGrace + time.Second)
	m.room.Connections[ac.ID].LastSeen = future
	m.pulseRoom(context.Background(), future)
	status := m.world.Status()
	m.mu.Unlock()
	if status != "paused" {
		t.Fatal("absent human did not pause")
	}
	if _, err := a.ResumeGame(controlFor(t, a, "continue-absent")); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	m.room.Connections[ac.ID].LastSeen = future.Add(time.Second)
	m.pulseRoom(context.Background(), future.Add(time.Second))
	status = m.world.Status()
	m.mu.Unlock()
	if status != "running" {
		t.Fatal("explicit continue ignored")
	}
	_ = a.Disconnect(ac.ID)
	m.mu.Lock()
	unload := m.pulseRoom(context.Background(), time.Now())
	status = m.world.Status()
	m.mu.Unlock()
	if unload {
		s.mu.Lock()
		delete(s.matches, m.id)
		s.mu.Unlock()
	}
	if !unload || status != "paused" {
		t.Fatal("last player did not pause/save/unload")
	}
}
func TestPortableMoveEncryptedImportAndDeletionFence(t *testing.T) {
	source, target := roomService(t), roomService(t)
	owner, a, friend, _ := joinedGame(t, source)
	a.match.world.Update()
	tick := a.match.world.Tick
	req := TransferRequest{ID: "move", Revision: controlFor(t, a, "unused").Revision, Kind: "move"}
	moved, err := a.Transfer(req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.ResumeGame(controlFor(t, a, "invalid-resume")); err == nil {
		t.Fatal("source resumed during move")
	}
	archive, err := a.Archive(moved.ID, "correct horse castle")
	if err != nil {
		t.Fatal(err)
	}
	bad := ImportRequest{Archive: archive, Passphrase: "wrong", RejoinCode: owner.RejoinCode}
	if _, err = target.Import(bad, "target-owner"); err == nil {
		t.Fatal("wrong passphrase accepted")
	}
	good := bad
	good.Passphrase = "correct horse castle"
	imported, err := target.Import(good, "target-owner")
	if err != nil {
		t.Fatal(err)
	}
	if imported.Session.MatchID != owner.MatchID || imported.Session.MembershipID != owner.MembershipID {
		t.Fatal("move lost identity")
	}
	newAccess, err := target.Access(imported.Session.MatchID, imported.Session.Token, "")
	if err != nil {
		t.Fatal(err)
	}
	v, err := newAccess.View()
	if err != nil || !v.Paused || v.Tick != tick {
		t.Fatal("import advanced time", err)
	}
	if _, err = target.Access(owner.MatchID, owner.Token, ""); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("source token accepted on target")
	}
	rejoin, err := target.Rejoin(RejoinRequest{Code: friend.RejoinCode}, "target-friend")
	if err != nil || rejoin.PlayerID != 2 {
		t.Fatal("friend cannot recover moved seat", err)
	}
	again, err := target.Import(good, "target-owner")
	if err != nil || !again.AlreadyImported {
		t.Fatal("duplicate import not idempotent", err)
	}
	newAccess, err = target.Access(again.Session.MatchID, again.Session.Token, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.CompleteTransfer(CompleteTransfer{ID: "complete", Revision: controlFor(t, a, "unused").Revision, Receipt: imported.CompletionReceipt}); err != nil {
		t.Fatal(err)
	}
	del := controlFor(t, newAccess, "delete")
	del.Confirm = true
	if err = target.DeleteGame(newAccess, del); err != nil {
		t.Fatal(err)
	}
	newAccess.match.mu.Lock()
	err = newAccess.match.save(time.Now())
	newAccess.match.mu.Unlock()
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("late save resurrected deletion: %v", err)
	}
	if _, err = target.Import(good, "target-owner"); err == nil {
		t.Fatal("deleted game restored without explicit copy")
	}
	good.Copy = true
	good.Name = "A new chapter"
	copy, err := target.Import(good, "target-owner")
	if err != nil {
		t.Fatal(err)
	}
	if copy.Session.MatchID == owner.MatchID || copy.Session.MembershipID == owner.MembershipID {
		t.Fatal("copy reused credentials or identity")
	}
}

func TestLegacyAdoptionRequiresTokenAndPreservesKingdom(t *testing.T) {
	s := roomService(t)
	legacy, err := s.Create(game.Config{Civilization: "britons", Difficulty: "peaceful", Mode: "skirmish", Seed: 4817, Settlements: 2})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AdoptLegacy(legacy.MatchID, "incorrect", "new-browser"); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("adopted without original token")
	}
	m, err := s.Authorized(legacy.MatchID, legacy.Token)
	if err != nil {
		t.Fatal(err)
	}
	before := m.View()
	upgraded, err := s.AdoptLegacy(legacy.MatchID, legacy.Token, "new-browser")
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.Access(upgraded.MatchID, upgraded.Token, "")
	if err != nil {
		t.Fatal(err)
	}
	after, err := a.View()
	if err != nil {
		t.Fatal(err)
	}
	if before.Tick != after.Tick || len(before.Entities) != len(after.Entities) || !after.Paused || upgraded.MatchID != legacy.MatchID {
		t.Fatal("adoption changed the world")
	}
	if _, err = s.Authorized(legacy.MatchID, legacy.Token); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("legacy token bypasses memberships")
	}
}

func TestReadinessAllowsPeersButInvalidatesChangedRules(t *testing.T) {
	s := roomService(t)
	_, a := createLobby(t, s, 1)
	g, _ := a.Info()
	i, err := a.Invite(g.Seats[1].ID, InviteRequest{ID: "invite", Revision: g.Revision})
	if err != nil {
		t.Fatal(err)
	}
	member, err := s.ClaimInvite(ClaimInvite{ID: "claim", Secret: i.Secret, Name: "Bob"}, "bob")
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Access(member.MatchID, member.Token, "")
	if err != nil {
		t.Fatal(err)
	}
	g, _ = a.Info()
	revision := g.Revision
	if _, err = a.Ready(g.Seats[0].ID, ReadyRequest{ID: "alice-ready", Revision: revision, Ready: true}); err != nil {
		t.Fatal(err)
	}
	if _, err = b.Ready(g.Seats[1].ID, ReadyRequest{ID: "bob-ready", Revision: revision, Ready: true}); err != nil {
		t.Fatal("peer readiness made request stale", err)
	}
	info, _ := a.Info()
	cfg := info.Config
	cfg.Seed++
	if _, err = a.ChangeRules(RulesChange{ID: "new-map", Revision: info.Revision, Config: cfg}); err != nil {
		t.Fatal(err)
	}
	if _, err = b.Ready(g.Seats[1].ID, ReadyRequest{ID: "stale-ready", Revision: revision, Ready: true}); err == nil {
		t.Fatal("readied unseen world settings")
	}
}

func TestCloseFailureCanRetryAndOldCloseCannotCloseAgain(t *testing.T) {
	s := roomService(t)
	_, a, _, _ := joinedGame(t, s)
	if _, err := s.db.Exec(`CREATE TRIGGER reject_save BEFORE INSERT ON sessions BEGIN SELECT RAISE(ABORT,'test save failure'); END;`); err != nil {
		t.Fatal(err)
	}
	closeRequest := controlFor(t, a, "close-once")
	if _, err := a.CloseGame(closeRequest); err == nil {
		t.Fatal("close reported durable success after a failed save")
	}
	info, err := a.Info()
	if err != nil || info.Status != "closing" || a.match.world.Status() != "paused" {
		t.Fatal("failed close did not remain frozen", err)
	}
	if _, err = s.db.Exec("DROP TRIGGER reject_save"); err != nil {
		t.Fatal(err)
	}
	if _, err = a.CloseGame(closeRequest); err != nil {
		t.Fatal("retry close failed", err)
	}
	if _, err = a.Reopen(controlFor(t, a, "reopen-after-close")); err != nil {
		t.Fatal(err)
	}
	info, err = a.CloseGame(closeRequest)
	if err != nil || info.Status != "open" {
		t.Fatal("old close closed a reopened game", err)
	}
}

func TestFailedDeleteRemainsRecoverableAndRetries(t *testing.T) {
	s := roomService(t)
	session, a := createLobby(t, s, 0)
	if _, err := s.db.Exec(`CREATE TRIGGER reject_delete BEFORE DELETE ON sessions BEGIN SELECT RAISE(ABORT,'test delete failure'); END;`); err != nil {
		t.Fatal(err)
	}
	req := controlFor(t, a, "delete-once")
	req.Confirm = true
	if err := s.DeleteGame(a, req); err == nil {
		t.Fatal("delete succeeded with failed transaction")
	}
	a, err := s.Access(session.MatchID, session.Token, "")
	if err != nil {
		t.Fatal("failed deletion became inaccessible", err)
	}
	if _, err = s.db.Exec("DROP TRIGGER reject_delete"); err != nil {
		t.Fatal(err)
	}
	if err = s.DeleteGame(a, req); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = s.db.QueryRow("SELECT count(*) FROM sessions WHERE id=?", session.MatchID).Scan(&count); err != nil || count != 0 {
		t.Fatal("deleted game remained", err)
	}
}

func TestUnstartedLobbySurvivesCloseRestartAndTransfer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "games.sqlite")
	s, err := OpenService(path)
	if err != nil {
		t.Fatal(err)
	}
	owner, a := createLobby(t, s, 1)
	if _, err = a.CloseGame(controlFor(t, a, "close-lobby")); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenService(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a, err = s.Access(owner.MatchID, owner.Token, "")
	if err != nil {
		t.Fatal("closed lobby did not restore", err)
	}
	if _, err = a.Reopen(controlFor(t, a, "reopen-lobby")); err != nil {
		t.Fatal(err)
	}
	g, err := a.Info()
	if err != nil || g.Status != "lobby" || a.match.world != nil {
		t.Fatal("reopen created a simulation", err)
	}
	transfer, err := a.Transfer(TransferRequest{ID: "move-lobby", Revision: g.Revision, Kind: "move"})
	if err != nil {
		t.Fatal(err)
	}
	data, err := a.Archive(transfer.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	target := roomService(t)
	imported, err := target.Import(ImportRequest{Archive: data, RejoinCode: owner.RejoinCode}, "importing-browser")
	if err != nil {
		t.Fatal("lobby import failed", err)
	}
	moved, err := target.Access(imported.Session.MatchID, imported.Session.Token, "")
	if err != nil {
		t.Fatal(err)
	}
	g, err = moved.Info()
	if err != nil || g.Status != "lobby" || moved.match.world != nil {
		t.Fatal("import started the lobby", err)
	}
}

func TestActiveRequestRetainsClosedGameUntilReopenCommits(t *testing.T) {
	s := roomService(t)
	owner, a, _, _ := joinedGame(t, s)
	if _, err := a.CloseGame(controlFor(t, a, "close")); err != nil {
		t.Fatal(err)
	}
	request, err := s.RequestAccess(owner.MatchID, owner.Token, "")
	if err != nil {
		t.Fatal(err)
	}
	defer request.Release()
	m := request.match
	m.mu.Lock()
	unloaded := m.pulseRoom(context.Background(), time.Now())
	m.mu.Unlock()
	if unloaded {
		t.Fatal("scheduler unloaded an authorized request")
	}
	if _, err = request.Reopen(controlFor(t, request, "reopen")); err != nil {
		t.Fatal("request lost its runtime before committing", err)
	}
}

func TestFailedSharedControlsNeverAdvanceUncommittedWorld(t *testing.T) {
	s := roomService(t)
	_, a, _, _ := joinedGame(t, s)
	if _, err := a.Pause(controlFor(t, a, "initial-pause")); err != nil {
		t.Fatal(err)
	}
	resume := controlFor(t, a, "durable-resume")
	if _, err := s.db.Exec(`CREATE TRIGGER reject_control BEFORE INSERT ON sessions BEGIN SELECT RAISE(ABORT,'test save failure'); END;`); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ResumeGame(resume); err == nil || a.match.world.Status() != "paused" {
		t.Fatal("failed resume left world running", err)
	}
	if _, err := s.db.Exec("DROP TRIGGER reject_control"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ResumeGame(resume); err != nil {
		t.Fatal("resume retry failed", err)
	}
	oldSpeed := a.match.world.Speed
	speed := controlFor(t, a, "durable-speed")
	speed.Value = 32
	staleResume := controlFor(t, a, "stale-resume")
	if _, err := s.db.Exec(`CREATE TRIGGER reject_control BEFORE INSERT ON sessions BEGIN SELECT RAISE(ABORT,'test save failure'); END;`); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Speed(speed); err == nil || a.match.world.Speed != oldSpeed || a.match.world.Status() != "paused" {
		t.Fatal("failed speed change advanced shared controls", err)
	}
	if _, err := s.db.Exec("DROP TRIGGER reject_control"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ResumeGame(staleResume); err == nil {
		t.Fatal("stale resume undid a save-failure pause")
	}
	if _, err := a.ResumeGame(controlFor(t, a, "fresh-resume")); err != nil {
		t.Fatal(err)
	}
}

func TestOwnerStartsWithoutWaitingAndFriendsJoinExistingKingdoms(t *testing.T) {
	for _, state := range []string{"vacant", "reserved", "claimed"} {
		t.Run(state, func(t *testing.T) {
			s := roomService(t)
			_, a := createLobby(t, s, 1)
			info, _ := a.Info()
			seatID := info.Seats[1].ID
			var invite Invitation
			var err error
			if state != "vacant" {
				invite, err = a.Invite(seatID, InviteRequest{ID: "invite", Revision: info.Revision})
				if err != nil {
					t.Fatal(err)
				}
			}
			if state == "claimed" {
				if _, err = s.ClaimInvite(ClaimInvite{ID: "claim", Secret: invite.Secret, Name: "Bob"}, "browser-bob"); err != nil {
					t.Fatal(err)
				}
			}
			connection, err := a.Connect()
			if err != nil {
				t.Fatal(err)
			}
			if _, err = a.Start(controlFor(t, a, "start-without-ready")); err != nil {
				t.Fatal(err)
			}
			m := a.match
			ids := map[int]bool{}
			for id, entity := range m.world.Entities {
				if entity.Owner == 2 {
					ids[id] = true
				}
			}
			if len(ids) != 5 || m.world.Players[2].AI {
				t.Fatal("friend kingdom did not keep its human starting roster")
			}
			// Unclaimed seats and players absent when the owner pressed Start
			// must not stop the game as soon as the disconnect grace expires.
			for n := 0; n < 3; n++ {
				now := time.Now().Add(time.Duration(n) * (DisconnectGrace + time.Second))
				m.mu.Lock()
				m.room.Connections[connection.ID].LastSeen = now
				m.pulseRoom(context.Background(), now)
				status := m.world.Status()
				m.mu.Unlock()
				if status != "running" {
					t.Fatal("waiting seat paused the game")
				}
			}
			if state == "claimed" {
				return
			}
			if state == "vacant" {
				info, _ = a.Info()
				invite, err = a.Invite(seatID, InviteRequest{ID: "late-invite", Revision: info.Revision})
				if err != nil || m.world.Status() != "running" {
					t.Fatal("late invitation interrupted play", err)
				}
			}
			friend, err := s.ClaimInvite(ClaimInvite{ID: "late-claim", Secret: invite.Secret, Name: "Bob"}, "browser-bob")
			if err != nil {
				t.Fatal(err)
			}
			b, err := s.Access(friend.MatchID, friend.Token, "")
			if err != nil {
				t.Fatal(err)
			}
			view, err := b.View()
			if err != nil || view.Player.ID != 2 || view.Paused || view.Player.Name != "Bob" {
				t.Fatal("late join changed clock or kingdom", err)
			}
			for id := range ids {
				if m.world.Entities[id] == nil || m.world.Entities[id].Owner != 2 {
					t.Fatal("late join regenerated a kingdom")
				}
			}
		})
	}
}

func TestMembershipBindingFailureDoesNotConsumeInviteOrCreateOrphanGame(t *testing.T) {
	s := roomService(t)
	_, a := createLobby(t, s, 1)
	info, _ := a.Info()
	invite, err := a.Invite(info.Seats[1].ID, InviteRequest{ID: "invite", Revision: info.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`CREATE TRIGGER reject_binding BEFORE INSERT ON browser_members BEGIN SELECT RAISE(ABORT,'test binding failure'); END;`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ClaimInvite(ClaimInvite{ID: "claim", Secret: invite.Secret}, "browser-bob"); err == nil {
		t.Fatal("claim succeeded without its recovery binding")
	}
	if _, err = s.InspectInvite(invite.Secret); err != nil {
		t.Fatal("failed binding consumed invite", err)
	}
	if _, err = s.CreateGame(CreateGame{Config: game.Config{Name: "Orphan", Settlements: 1}}, "new-owner"); err == nil {
		t.Fatal("creation succeeded without binding")
	}
	var count int
	if err = s.db.QueryRow("SELECT count(*) FROM sessions").Scan(&count); err != nil || count != 1 || len(s.matches) != 1 {
		t.Fatal("binding failure left an orphan game", err)
	}
	if _, err = s.db.Exec("DROP TRIGGER reject_binding"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ClaimInvite(ClaimInvite{ID: "claim", Secret: invite.Secret}, "browser-bob"); err != nil {
		t.Fatal("claim retry failed", err)
	}
}

func TestStartIgnoresReadinessUpdatesButRequiresCurrentWorldSettings(t *testing.T) {
	s := roomService(t)
	_, a := createLobby(t, s, 0)
	start := controlFor(t, a, "start")
	readySeat(t, a)
	if _, err := a.Start(start); err != nil {
		t.Fatal("optional readiness invalidated Start", err)
	}
	_, b := createLobby(t, s, 0)
	stale := controlFor(t, b, "stale-start")
	info, _ := b.Info()
	config := info.Config
	config.Seed++
	if _, err := b.ChangeRules(RulesChange{ID: "changed-map", Revision: info.Revision, Config: config}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Start(stale); err == nil {
		t.Fatal("started with unseen world settings")
	}
	if _, err := b.Start(controlFor(t, b, "reviewed-start")); err != nil {
		t.Fatal(err)
	}
}
