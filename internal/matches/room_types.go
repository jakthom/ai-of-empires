package matches

import (
	"crowns/internal/game"
	"github.com/open-ships/statemachine"
	"time"
)

// These DTOs are the multiplayer boundary. Private hashes and machine instances
// never appear in read models. A Session is one member, not one shared game key.
type MemberSession struct {
	MatchID      string `json:"match_id"`
	Token        string `json:"token"`
	PlayerID     int    `json:"player_id"`
	Name         string `json:"name"`
	MembershipID string `json:"membership_id"`
	RejoinCode   string `json:"rejoin_code,omitempty"`
	Epoch        string `json:"epoch"`
	Owner        bool   `json:"owner"`
}
type CreateGame struct {
	Config     game.Config `json:"config"`
	Friends    int         `json:"friends"`
	PlayerName string      `json:"player_name"`
}
type GameControl struct {
	ID       string  `json:"id"`
	Revision int     `json:"revision"`
	Value    float64 `json:"value,omitempty"`
	Confirm  bool    `json:"confirm,omitempty"`
}
type SeatChange struct {
	ID           string `json:"id"`
	Revision     int    `json:"revision"`
	Name         string `json:"name"`
	Civilization string `json:"civilization"`
	Controller   string `json:"controller"`
}
type ReadyRequest struct {
	ID       string `json:"id"`
	Revision int    `json:"revision"`
	Ready    bool   `json:"ready"`
}
type RulesChange struct {
	ID       string      `json:"id"`
	Revision int         `json:"revision"`
	Config   game.Config `json:"config"`
}
type InviteRequest struct {
	ID       string `json:"id"`
	Revision int    `json:"revision"`
	Replace  bool   `json:"replace"`
}
type InviteSecret struct {
	Secret string `json:"secret"`
}
type ClaimInvite struct {
	ID     string `json:"id"`
	Secret string `json:"secret"`
	Name   string `json:"name"`
}
type RejoinRequest struct {
	Code string `json:"code"`
}
type Invitation struct {
	ID        string `json:"id"`
	GameID    string `json:"game_id"`
	GameName  string `json:"game_name"`
	SeatID    string `json:"seat_id"`
	Name      string `json:"name"`
	ExpiresAt string `json:"expires_at"`
	Secret    string `json:"secret,omitempty"`
}
type SeatView struct {
	ID              string `json:"id"`
	PlayerID        int    `json:"player_id"`
	MembershipID    string `json:"membership_id,omitempty"`
	Name            string `json:"name"`
	Civilization    string `json:"civilization"`
	Controller      string `json:"controller"`
	Status          string `json:"status"`
	Ready           bool   `json:"ready"`
	Connected       bool   `json:"connected"`
	Owner           bool   `json:"owner"`
	Yours           bool   `json:"yours"`
	InviteID        string `json:"invite_id,omitempty"`
	InviteExpiresAt string `json:"invite_expires_at,omitempty"`
}
type GameInfo struct {
	GameID      string      `json:"game_id"`
	Name        string      `json:"name"`
	Config      game.Config `json:"config"`
	Status      string      `json:"status"`
	MatchStatus string      `json:"match_status"`
	Runtime     string      `json:"runtime"`
	Epoch       string      `json:"epoch"`
	Revision    int         `json:"revision"`
	Seats       []SeatView  `json:"seats"`
	Owner       bool        `json:"owner"`
	PlayerID    int         `json:"player_id"`
	Time        float64     `json:"time"`
	SavedAt     string      `json:"saved_at"`
	SaveError   string      `json:"save_error,omitempty"`
	CanStart    bool        `json:"can_start"`
	StartReason string      `json:"start_reason"`
	TransferID  string      `json:"transfer_id,omitempty"`
}
type GameLibrary struct {
	Games []GameInfo `json:"games"`
}
type AuditEvent struct {
	Audience  string `json:"audience"`
	ActorName string `json:"actor_name"`
	ID        int    `json:"id"`
	At        string `json:"at"`
	Actor     string `json:"actor"`
	Action    string `json:"action"`
	Message   string `json:"message"`
}
type AuditPage struct {
	Events []AuditEvent `json:"events"`
}
type ConnectionInfo struct {
	ID    string `json:"id"`
	Epoch string `json:"epoch"`
}

type room struct {
	RosterRevision      int
	Config              game.Config
	Epoch               string
	OwnerID             string
	Revision            int
	Seats               []*seat
	Invites             map[string]*invitation
	Controls            map[string]controlReceipt
	Audit               []AuditEvent
	Transfer            *transferRecord
	Session             *statemachine.Instance[sessionState, sessionEvent, *roomContext]
	Runtime             *statemachine.Instance[runtimeState, runtimeEvent, *roomContext]
	Connections         map[string]*connection
	AbsentSince         map[string]time.Time
	AbsenceAcknowledged map[string]bool
	OpenedAt            time.Time
}
type seat struct {
	ID, MemberID, Name, Civilization, Controller string
	PlayerID, CredentialVersion                  int
	TokenHash, RejoinHash                        [32]byte
	State                                        *statemachine.Instance[seatState, seatEvent, *seat]
	Readiness                                    *statemachine.Instance[readyState, readyEvent, *seat]
	window                                       time.Time
	commands                                     int
}
type invitation struct {
	ID, SeatID string
	Hash       [32]byte
	ExpiresAt  time.Time
	State      *statemachine.Instance[inviteState, inviteEvent, *invitation]
}
type connection struct {
	ID, MemberID, Epoch string
	LastSeen            time.Time
	State               *statemachine.Instance[connectionState, connectionEvent, *connection]
}
type controlReceipt struct {
	Hash [32]byte
	Kind string
}
type transferRecord struct {
	ID, Kind, ImportedFrom, ReceiptHash string
	CreatedAt                           time.Time
}
type Access struct {
	held            bool
	match           *Match
	memberID, epoch string
	version         int
}
type browserBinding struct {
	GameID, MemberID string
	Version          int
}
