package matches

import (
	"errors"
	"path/filepath"
	"testing"

	"crowns/internal/game"
)

func TestAgentCredentialsAreStableScopedAndSurviveRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "host.sqlite")
	s, err := OpenService(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	owner, a, friend, b := joinedGame(t, s)
	one, err := a.AgentCredential()
	if err != nil {
		t.Fatal(err)
	}
	again, err := a.AgentCredential()
	if err != nil || again != one {
		t.Fatal("credential retrieval changed membership state", err)
	}
	two, err := b.AgentCredential()
	if err != nil {
		t.Fatal(err)
	}
	if one.Token == two.Token || one.Token == owner.Token || two.Token == friend.Token {
		t.Fatal("credentials are not separated by scope and membership")
	}
	other, err := s.CreateGame(CreateGame{Config: game.Config{Settlements: 1}}, "browser-other-agent-test")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ game, member, token string }{
		{owner.MatchID, friend.MembershipID, one.Token},
		{owner.MatchID, owner.MembershipID, two.Token},
		{owner.MatchID, owner.MembershipID, owner.Token},
		{other.MatchID, owner.MembershipID, one.Token},
		{owner.MatchID, "", one.Token},
		{owner.MatchID, owner.MembershipID, ""},
	} {
		if access, err := s.RequestAgentAccess(tc.game, tc.member, tc.token); !errors.Is(err, ErrUnauthorized) {
			if access != nil {
				access.Release()
			}
			t.Fatal("accepted wrong game, member or credential scope", err)
		}
	}
	if _, err := s.Access(owner.MatchID, one.Token, "browser-alice"); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("restricted token bypassed REST authentication or fell back to cookies", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenService(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, credential := range []AgentCredential{one, two} {
		access, err := s.RequestAgentAccess(credential.GameID, credential.MembershipID, credential.Token)
		if err != nil {
			t.Fatal("restart lost derived credential", err)
		}
		view, err := access.View()
		access.Release()
		if err != nil || view.Player.ID != credential.PlayerID || !view.Paused {
			t.Fatal("restart changed identity or advanced offline", err)
		}
	}
}

func TestSeatReplacementRevokesAgentAccessAlreadyInFlight(t *testing.T) {
	s := roomService(t)
	owner, a, friend, b := joinedGame(t, s)
	credential, err := b.AgentCredential()
	if err != nil {
		t.Fatal(err)
	}
	old, err := s.RequestAgentAccess(owner.MatchID, friend.MembershipID, credential.Token)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Release()
	info, err := a.Info()
	if err != nil {
		t.Fatal(err)
	}
	invitation, err := a.Invite(info.Seats[1].ID, InviteRequest{ID: "replace-agent", Revision: info.Revision, Replace: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.View(); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("retired membership kept an already-authenticated view", err)
	}
	if _, err := old.Apply(game.Command{ID: "late-resign", Kind: "resign"}); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("retired membership kept command authority", err)
	}
	if _, err := s.RequestAgentAccess(owner.MatchID, friend.MembershipID, credential.Token); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("retired membership could reconnect", err)
	}
	replacement, err := s.ClaimInvite(ClaimInvite{ID: "claim-new-agent", Secret: invitation.Secret, Name: "New opponent"}, "browser-new-agent")
	if err != nil {
		t.Fatal(err)
	}
	if replacement.PlayerID != friend.PlayerID || replacement.MembershipID == friend.MembershipID {
		t.Fatal("replacement must retain kingdom but get a new endpoint identity")
	}
	if _, err := s.RequestAgentAccess(owner.MatchID, replacement.MembershipID, credential.Token); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("old credential authorized the new membership", err)
	}
}
