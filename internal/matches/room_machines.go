package matches

import (
	"context"
	"crowns/internal/game"
	"errors"
	"github.com/open-ships/statemachine"
	"time"
)

type sessionState string
type sessionEvent string

const (
	sessionLobby    sessionState = "lobby"
	sessionOpen     sessionState = "open"
	sessionClosing  sessionState = "closing"
	sessionClosed   sessionState = "closed"
	sessionDeleting sessionState = "deleting"
	sessionDeleted  sessionState = "deleted"
	startGame       sessionEvent = "start"
	beginClose      sessionEvent = "close"
	completeClose   sessionEvent = "saved"
	cancelClose     sessionEvent = "cancel_close"
	reopenGame      sessionEvent = "reopen"
	beginDelete     sessionEvent = "delete"
	completeDelete  sessionEvent = "deleted"
)

type runtimeState string
type runtimeEvent string

const (
	runtimeServing   runtimeState = "serving"
	runtimeQuiescing runtimeState = "quiescing"
	runtimeFrozen    runtimeState = "frozen"
	runtimeReleased  runtimeState = "released"
	quiesceRuntime   runtimeEvent = "quiesce"
	freezeRuntime    runtimeEvent = "freeze"
	releaseRuntime   runtimeEvent = "release"
	restoreRuntime   runtimeEvent = "restore"
)

type roomContext struct {
	Match *Match
	World *game.World
	Actor string
}

var sessionMachine = statemachine.MustCompile([]statemachine.Transition[sessionState, sessionEvent, *roomContext]{
	{From: sessionLobby, Event: startGame, To: sessionOpen, Guard: startableRoster, Do: installWorld},
	{From: sessionLobby, Event: beginClose, To: sessionClosing, Do: pauseRoom}, {From: sessionOpen, Event: beginClose, To: sessionClosing, Do: pauseRoom},
	{From: sessionClosing, Event: completeClose, To: sessionClosed},
	{From: sessionClosed, Event: reopenGame, To: sessionLobby, Guard: hasNoWorld}, {From: sessionClosed, Event: reopenGame, To: sessionOpen, Do: pauseRoom},
	{From: sessionClosing, Event: cancelClose, To: sessionLobby, Guard: hasNoWorld}, {From: sessionClosing, Event: cancelClose, To: sessionOpen, Do: pauseRoom},
	{From: sessionLobby, Event: beginDelete, To: sessionDeleting, Do: pauseRoom}, {From: sessionOpen, Event: beginDelete, To: sessionDeleting, Do: pauseRoom},
	{From: sessionClosed, Event: beginDelete, To: sessionDeleting}, {From: sessionClosing, Event: beginDelete, To: sessionDeleting},
	{From: sessionDeleting, Event: completeDelete, To: sessionDeleted},
})
var runtimeMachine = statemachine.MustCompile([]statemachine.Transition[runtimeState, runtimeEvent, *roomContext]{
	{From: runtimeServing, Event: quiesceRuntime, To: runtimeQuiescing, Do: pauseRoom},
	{From: runtimeQuiescing, Event: freezeRuntime, To: runtimeFrozen}, {From: runtimeFrozen, Event: releaseRuntime, To: runtimeReleased},
	{From: runtimeServing, Event: releaseRuntime, To: runtimeReleased, Do: pauseRoom},
	{From: runtimeFrozen, Event: restoreRuntime, To: runtimeServing}, {From: runtimeQuiescing, Event: restoreRuntime, To: runtimeServing}, {From: runtimeReleased, Event: restoreRuntime, To: runtimeServing},
})

func pauseRoom(_ context.Context, c *roomContext) error {
	if c.Match.world != nil {
		return c.Match.world.SetPaused(true)
	}
	return nil
}
func installWorld(_ context.Context, c *roomContext) error { c.Match.world = c.World; return nil }
func hasNoWorld(_ context.Context, c *roomContext) error {
	if c.Match.world != nil {
		return errors.New("game has started")
	}
	return nil
}
func startableRoster(_ context.Context, c *roomContext) error { return c.Match.room.canStart() }
func (r *room) canStart() error {
	if r.Runtime.State() != runtimeServing {
		return ruleError("game_unavailable", "Finish or cancel the move before starting.")
	}
	for _, s := range r.Seats {
		if s.MemberID == r.OwnerID && s.Controller == "human" && s.State.State() == seatClaimed {
			return nil
		}
	}
	return ruleError("owner_required", "The game needs its owner before starting.")
}

type seatState string
type seatEvent string

const (
	seatVacant   seatState = "vacant"
	seatReserved seatState = "reserved"
	seatClaimed  seatState = "claimed"
	seatRetired  seatState = "retired"
	reserveSeat  seatEvent = "reserve"
	claimSeat    seatEvent = "claim"
	retireSeat   seatEvent = "retire"
	resetSeat    seatEvent = "reset"
)

var seatMachine = statemachine.MustCompile([]statemachine.Transition[seatState, seatEvent, *seat]{
	{From: seatVacant, Event: reserveSeat, To: seatReserved}, {From: seatReserved, Event: reserveSeat, To: seatReserved},
	{From: seatReserved, Event: claimSeat, To: seatClaimed}, {From: seatClaimed, Event: retireSeat, To: seatRetired, Do: revokeSeat},
	{From: seatRetired, Event: resetSeat, To: seatVacant}, {From: seatReserved, Event: resetSeat, To: seatVacant}, {From: seatVacant, Event: resetSeat, To: seatVacant},
})

func revokeSeat(_ context.Context, s *seat) error {
	s.TokenHash = [32]byte{}
	s.RejoinHash = [32]byte{}
	s.CredentialVersion++
	s.MemberID = ""
	return nil
}

type readyState string
type readyEvent string

const (
	readyNo    readyState = "not_ready"
	readyYes   readyState = "ready"
	markReady  readyEvent = "ready"
	clearReady readyEvent = "clear"
)

var readyMachine = statemachine.MustCompile([]statemachine.Transition[readyState, readyEvent, *seat]{
	{From: readyNo, Event: markReady, To: readyYes}, {From: readyYes, Event: markReady, To: readyYes}, {From: readyNo, Event: clearReady, To: readyNo}, {From: readyYes, Event: clearReady, To: readyNo},
})

type inviteState string
type inviteEvent string

const (
	inviteIssued     inviteState = "issued"
	inviteClaimed    inviteState = "claimed"
	inviteExpired    inviteState = "expired"
	inviteRevoked    inviteState = "revoked"
	claimInvitation  inviteEvent = "claim"
	expireInvitation inviteEvent = "expire"
	revokeInvitation inviteEvent = "revoke"
)

var inviteMachine = statemachine.MustCompile([]statemachine.Transition[inviteState, inviteEvent, *invitation]{
	{From: inviteIssued, Event: claimInvitation, To: inviteClaimed}, {From: inviteIssued, Event: expireInvitation, To: inviteExpired}, {From: inviteIssued, Event: revokeInvitation, To: inviteRevoked},
})

type connectionState string
type connectionEvent string

const (
	connectionConnected    connectionState = "connected"
	connectionDisconnected connectionState = "disconnected"
	connectBrowser         connectionEvent = "connect"
	disconnectBrowser      connectionEvent = "disconnect"
)

var connectionMachine = statemachine.MustCompile([]statemachine.Transition[connectionState, connectionEvent, *connection]{
	{From: connectionDisconnected, Event: connectBrowser, To: connectionConnected}, {From: connectionConnected, Event: disconnectBrowser, To: connectionDisconnected}, {From: connectionDisconnected, Event: disconnectBrowser, To: connectionDisconnected},
})

func fireRoom[S comparable, E comparable, C any](i *statemachine.Instance[S, E, C], e E, c C) error {
	_, err := i.Fire(context.Background(), e, c)
	return err
}
func newSeat(id string, p int, name, civ, controller string) *seat {
	return &seat{ID: id, PlayerID: p, Name: name, Civilization: civ, Controller: controller, State: statemachine.NewInstance(seatMachine, seatVacant), Readiness: statemachine.NewInstance(readyMachine, readyNo)}
}
func (r *room) invalidateReady() {
	r.RosterRevision = r.Revision + 1
	for _, s := range r.Seats {
		_ = fireRoom(s.Readiness, clearReady, s)
	}
}
func (r *room) audit(actor, action, message string) {
	name := actor
	for _, s := range r.Seats {
		if s.MemberID == actor {
			name = s.Name
			break
		}
	}
	r.Audit = append(r.Audit, AuditEvent{Audience: "members", ActorName: name, ID: len(r.Audit) + 1, At: time.Now().UTC().Format(time.RFC3339Nano), Actor: actor, Action: action, Message: message})
}
func ruleError(code, message string) error { return &game.RuleError{Code: code, Message: message} }
