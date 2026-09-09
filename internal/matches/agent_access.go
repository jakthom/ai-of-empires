package matches

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// AgentCredential grants access only through the player MCP boundary. It is
// derived from the existing membership credential owner, not another mutable
// credential lifecycle. Rejoin, replacement and hosting-epoch changes revoke it.
type AgentCredential struct {
	GameID       string `json:"game_id"`
	MembershipID string `json:"membership_id"`
	PlayerID     int    `json:"player_id"`
	Token        string `json:"token"`
}

func agentToken(gameID, epoch, memberID string, version int, key [32]byte) string {
	mac := hmac.New(sha256.New, key[:])
	_, _ = fmt.Fprintf(mac, "ai-of-empires/mcp/v1\x00%s\x00%s\x00%s\x00%d", gameID, epoch, memberID, version)
	return "mcp_" + hex.EncodeToString(mac.Sum(nil))
}

func (a *Access) AgentCredential() (AgentCredential, error) {
	m := a.match
	m.mu.Lock()
	defer m.mu.Unlock()
	p, err := a.valid(false)
	if err != nil {
		return AgentCredential{}, err
	}
	if p == nil {
		return AgentCredential{}, ErrUnauthorized
	}
	return AgentCredential{GameID: m.id, MembershipID: p.MemberID, PlayerID: p.PlayerID, Token: agentToken(m.id, m.room.Epoch, p.MemberID, p.CredentialVersion, p.TokenHash)}, nil
}

// RequestAgentAccess authenticates both URL identity and the restricted token.
// The ordinary REST Access/RequestAccess paths never accept this credential.
func (s *Service) RequestAgentAccess(id, memberID, token string) (*Access, error) {
	if memberID == "" || token == "" {
		return nil, ErrUnauthorized
	}
	return s.access(id, token, "", true, memberID)
}
