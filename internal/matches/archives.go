package matches

import (
	"bytes"
	"compress/gzip"
	"crowns/internal/game"
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/open-ships/statemachine"
	"io"
	"time"
)

const MaxArchiveBytes = 64 << 20
const maxExpandedArchive = 128 << 20
const archiveMagic = "AI-of-Empires/game/1"

type TransferRequest struct {
	ID       string `json:"id"`
	Revision int    `json:"revision"`
	Kind     string `json:"kind"`
}
type TransferInfo struct {
	ID           string `json:"id"`
	GameID       string `json:"game_id"`
	Kind         string `json:"kind"`
	Status       string `json:"status"`
	DownloadPath string `json:"download_path"`
}
type ArchivePassword struct {
	Passphrase string `json:"passphrase"`
}
type ImportRequest struct {
	Archive    []byte `json:"archive"`
	Passphrase string `json:"passphrase"`
	RejoinCode string `json:"rejoin_code"`
	Copy       bool   `json:"copy"`
	Name       string `json:"name,omitempty"`
}
type ImportResult struct {
	Session           MemberSession `json:"session"`
	CompletionReceipt string        `json:"completion_receipt,omitempty"`
	AlreadyImported   bool          `json:"already_imported"`
}
type CompleteTransfer struct {
	ID       string `json:"id"`
	Revision int    `json:"revision"`
	Receipt  string `json:"receipt"`
}
type archiveMeta struct {
	Format           string
	Rules            string
	GameID           string
	TransferID       string
	Kind             string
	CreatedAt        time.Time
	CompletionSecret string
}
type gameArchive struct {
	Manifest   archiveMeta
	Checkpoint storedMatch
	Journal    []game.JournalRecord
}
type archiveEnvelope struct {
	Format string
	Salt   []byte `json:",omitempty"`
	Nonce  []byte `json:",omitempty"`
	Data   []byte
}
type importedTransfer struct{ ID, GameID, Receipt string }

func (m *Match) capture(c storedMatch) ([]byte, error) {
	payload := gameArchive{Manifest: *m.archiveCapture, Checkpoint: c}
	if m.world != nil {
		payload.Journal = m.world.JournalSince(0)
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	if len(data) > maxExpandedArchive {
		return nil, ruleError("archive_too_large", "This game's archive exceeds the current size limit.")
	}
	var compressed bytes.Buffer
	z := gzip.NewWriter(&compressed)
	if _, err = z.Write(data); err != nil {
		return nil, err
	}
	if err = z.Close(); err != nil {
		return nil, err
	}
	return compressed.Bytes(), nil
}
func (a *Access) Transfer(req TransferRequest) (TransferInfo, error) {
	if req.Kind != "move" && req.Kind != "copy" {
		return TransferInfo{}, ruleError("invalid_transfer", "Choose Move game or Copy as new game.")
	}
	_, err := a.control("transfer", req.ID, req.Revision, req, true, false, func() error {
		m, r := a.match, a.match.room
		if r.Runtime.State() != runtimeServing || (r.Session.State() != sessionOpen && r.Session.State() != sessionLobby) {
			return ruleError("game_unavailable", "Open the game before moving or copying it.")
		}
		id, err := randomID(16)
		if err != nil {
			return err
		}
		secret, err := randomID(32)
		if err != nil {
			return err
		}
		if err = fireRoom(r.Runtime, quiesceRuntime, &roomContext{Match: m}); err != nil {
			return err
		}
		a.revokeInvites("")
		if err = fireRoom(r.Runtime, freezeRuntime, &roomContext{Match: m}); err != nil {
			return err
		}
		hash := tokenHash(secret)
		r.Transfer = &transferRecord{ID: id, Kind: req.Kind, ReceiptHash: hex.EncodeToString(hash[:]), CreatedAt: time.Now()}
		r.audit(a.memberID, "transfer_created", "Saved a portable "+req.Kind+" archive.")
		if req.Kind == "copy" {
			_ = fireRoom(r.Runtime, restoreRuntime, &roomContext{Match: m})
		}
		m.archiveCapture = &archiveMeta{Format: archiveMagic, Rules: game.RulesVersion, GameID: m.id, TransferID: id, Kind: req.Kind, CreatedAt: r.Transfer.CreatedAt, CompletionSecret: secret}
		return nil
	})
	m := a.match
	m.mu.Lock()
	defer m.mu.Unlock()
	m.archiveCapture = nil
	if err != nil {
		return TransferInfo{}, err
	}
	t := m.room.Transfer
	if t == nil {
		return TransferInfo{}, ErrNotFound
	}
	return TransferInfo{ID: t.ID, GameID: m.id, Kind: t.Kind, Status: "saved", DownloadPath: "/api/v1/games/" + m.id + "/transfers/" + t.ID + "/archive"}, nil
}
func (a *Access) Archive(id, passphrase string) ([]byte, error) {
	m := a.match
	m.mu.Lock()
	if _, err := a.valid(false); err != nil {
		m.mu.Unlock()
		return nil, err
	}
	if err := a.owner(); err != nil {
		m.mu.Unlock()
		return nil, err
	}
	var raw []byte
	var err error
	if m.db != nil {
		err = m.db.QueryRow("SELECT payload FROM game_archives WHERE game_id=? AND transfer_id=?", m.id, id).Scan(&raw)
	} else {
		raw = m.archives[id]
		if raw == nil {
			err = ErrNotFound
		}
	}
	m.mu.Unlock()
	if err != nil {
		return nil, ErrNotFound
	}
	envelope := archiveEnvelope{Format: archiveMagic, Data: raw}
	if passphrase != "" {
		if len(passphrase) < 8 || len(passphrase) > 256 {
			return nil, ruleError("invalid_passphrase", "Use a passphrase of 8–256 characters.")
		}
		envelope.Salt = make([]byte, 16)
		envelope.Nonce = make([]byte, 12)
		if _, err = rand.Read(envelope.Salt); err != nil {
			return nil, err
		}
		if _, err = rand.Read(envelope.Nonce); err != nil {
			return nil, err
		}
		gcm, err := archiveCipher(passphrase, envelope.Salt)
		if err != nil {
			return nil, err
		}
		envelope.Data = gcm.Seal(nil, envelope.Nonce, raw, []byte(archiveMagic))
	}
	return json.Marshal(envelope)
}
func archiveCipher(password string, salt []byte) (cipher.AEAD, error) {
	key, err := pbkdf2.Key(sha256.New, password, salt, 600000, 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
func readArchive(req ImportRequest) (result gameArchive, err error) {
	// Restore rejects invalid game states. Malformed private archives must never
	// crash the host even if they were hand-edited outside this application.
	defer func() {
		if recover() != nil {
			err = ruleError("invalid_archive", "The archive contains invalid game data.")
		}
	}()
	if len(req.Archive) == 0 || len(req.Archive) > MaxArchiveBytes {
		return result, ruleError("invalid_archive", "Choose an .aoegame archive up to 64 MB.")
	}
	if len(req.Passphrase) > 256 {
		return result, ruleError("invalid_passphrase", "The passphrase is too long.")
	}
	var envelope archiveEnvelope
	if err = json.Unmarshal(req.Archive, &envelope); err != nil || envelope.Format != archiveMagic {
		return result, ruleError("invalid_archive", "This is not a supported AI of Empires archive.")
	}
	raw := envelope.Data
	if len(envelope.Salt) > 0 || len(envelope.Nonce) > 0 {
		if len(envelope.Salt) != 16 || len(envelope.Nonce) != 12 {
			return result, ruleError("invalid_archive", "The encrypted archive is malformed.")
		}
		gcm, e := archiveCipher(req.Passphrase, envelope.Salt)
		if e != nil {
			return result, e
		}
		raw, e = gcm.Open(nil, envelope.Nonce, raw, []byte(archiveMagic))
		if e != nil {
			return result, ruleError("invalid_passphrase", "The passphrase is wrong or the archive is damaged.")
		}
	}
	z, e := gzip.NewReader(bytes.NewReader(raw))
	if e != nil {
		return result, ruleError("invalid_archive", "The archive is damaged.")
	}
	defer z.Close()
	data, e := io.ReadAll(io.LimitReader(z, maxExpandedArchive+1))
	if e != nil || len(data) > maxExpandedArchive {
		return result, ruleError("invalid_archive", "The archive is damaged or too large.")
	}
	if e = json.Unmarshal(data, &result); e != nil || result.Manifest.Format != archiveMagic || result.Manifest.Rules != game.RulesVersion || result.Checkpoint.Room == nil || len(result.Manifest.GameID) != 32 || len(result.Manifest.TransferID) != 32 {
		return result, ruleError("invalid_archive", "The archive version or game data is unsupported.")
	}
	_, e = restoreRoom(result.Checkpoint.Room)
	if e != nil {
		return result, ruleError("invalid_archive", e.Error())
	}
	owner := result.Checkpoint.Room.OwnerID
	authorized := false
	for _, p := range result.Checkpoint.Room.Seats {
		if p.MemberID == owner && hashEqual(p.RejoinHash, tokenHash(req.RejoinCode)) {
			authorized = true
		}
	}
	if !authorized {
		return result, ruleError("ownership_required", "Enter the game owner's private rejoin code to import this archive.")
	}
	if result.Manifest.Kind != "move" && !req.Copy {
		return result, ruleError("copy_required", "Import this backup as a new game.")
	}
	return result, nil
}
func (s *Service) Import(req ImportRequest, browser string) (result ImportResult, err error) {
	payload, err := readArchive(req)
	if err != nil {
		return result, err
	}
	// Validate and restore before admitting any durable rows. No archive paths,
	// SQL, or executable content are interpreted.
	var world *game.World
	if checkpointHasWorld(payload.Checkpoint.World) {
		func() {
			defer func() {
				if recover() != nil {
					err = errors.New("invalid checkpoint")
				}
			}()
			world, err = game.Restore(payload.Checkpoint.World, payload.Journal)
		}()
		if err != nil {
			return result, ruleError("invalid_archive", "The game's checkpoint cannot be restored.")
		}
		_ = world.SetPaused(true)
	}
	r, err := restoreRoom(payload.Checkpoint.Room)
	if err != nil {
		return result, err
	}
	if (world == nil && r.Session.State() != sessionLobby) || (world != nil && r.Session.State() != sessionOpen) {
		return result, ruleError("invalid_archive", "The checkpoint does not match its session lifecycle.")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lifecycle.State() != serviceServing {
		return result, ErrShuttingDown
	}
	if len(s.matches) >= 16 {
		return result, ErrCapacity
	}
	id := payload.Manifest.GameID
	if !req.Copy {
		var old importedTransfer
		if s.db != nil {
			_ = s.db.QueryRow("SELECT transfer_id,game_id,receipt FROM imported_transfers WHERE transfer_id=?", payload.Manifest.TransferID).Scan(&old.ID, &old.GameID, &old.Receipt)
		} else {
			old = s.imported[payload.Manifest.TransferID]
		}
		if old.ID != "" {
			m, e := s.load(old.GameID)
			if e != nil {
				return result, ruleError("game_deleted", "The imported game was deleted. Import a new copy instead.")
			}
			m.mu.Lock()
			defer m.mu.Unlock()
			for _, p := range m.room.Seats {
				if p.MemberID == m.room.OwnerID && hashEqual(p.RejoinHash, tokenHash(req.RejoinCode)) {
					oldHash, oldVersion := p.TokenHash, p.CredentialVersion
					session, e := m.issueCredentials(p, false)
					if e != nil {
						return result, e
					}
					if e = s.saveMembership(browser, m, p); e != nil {
						p.TokenHash, p.CredentialVersion = oldHash, oldVersion
						return result, e
					}
					return ImportResult{Session: session, CompletionReceipt: old.Receipt, AlreadyImported: true}, nil
				}
			}
			return result, ErrUnauthorized
		}
		var exists int
		if s.db != nil {
			_ = s.db.QueryRow("SELECT (SELECT count(*) FROM sessions WHERE id=?)+(SELECT count(*) FROM tombstones WHERE game_id=?)", id, id).Scan(&exists)
		}
		if exists > 0 || s.matches[id] != nil || s.deleted[id] {
			return result, ruleError("game_exists", "This host already has this game or its deletion marker. Import a new copy instead.")
		}
	} else {
		id, err = randomID(16)
		if err != nil {
			return result, err
		}
	}
	epoch, err := randomID(16)
	if err != nil {
		return result, err
	}
	r.Epoch = epoch
	r.Revision++
	r.Transfer = nil
	if r.Runtime.State() != runtimeServing {
		if err = fireRoom(r.Runtime, restoreRuntime, &roomContext{}); err != nil {
			return result, err
		}
	}
	m := &Match{id: id, db: s.db, room: r, world: world, commands: map[string]cachedCommand{}, accumulator: payload.Checkpoint.Accumulator, lastAccess: time.Now(), lifecycle: statemachine.NewInstance(leaseMachine, leaseOpen)}
	oldOwner := r.OwnerID
	var owner *seat
	for _, p := range r.Seats {
		p.TokenHash = [32]byte{}
		p.CredentialVersion++
		if p.MemberID == oldOwner {
			owner = p
		}
		if req.Copy {
			p.ID, err = randomID(12)
			if err != nil {
				return result, err
			}
			if p.MemberID != oldOwner && p.State.State() == seatClaimed {
				_ = fireRoom(p.State, retireSeat, p)
				_ = fireRoom(p.State, resetSeat, p)
			}
			if p.State.State() == seatReserved {
				_ = fireRoom(p.State, resetSeat, p)
			}
		}
	}
	for _, i := range r.Invites {
		if i.State.State() == inviteIssued {
			_ = fireRoom(i.State, revokeInvitation, i)
		}
	}
	if req.Copy {
		r.Invites = map[string]*invitation{}
		r.Controls = map[string]controlReceipt{}
		name := req.Name
		if name == "" {
			name = r.Config.Name + " (copy)"
		}
		if len([]rune(name)) > 80 {
			return result, ruleError("invalid_name", "Use a copy name of up to 80 characters.")
		}
		r.Config.Name = name
		if world != nil {
			world.Config.Name = name
		}
	} else {
		restoreCommands(m, payload.Checkpoint.Commands)
	}
	session, err := m.issueCredentials(owner, req.Copy)
	if err != nil {
		return result, err
	}
	r.OwnerID = owner.MemberID
	session.Owner = true
	r.audit(owner.MemberID, "imported", "Imported this game on a new host. Play starts paused.")
	receipt := ""
	if !req.Copy {
		receipt = payload.Manifest.TransferID + "." + payload.Manifest.CompletionSecret
		m.importReceipt = &importedTransfer{ID: payload.Manifest.TransferID, GameID: id, Receipt: receipt}
	}
	if err = s.saveMembership(browser, m, owner); err != nil {
		return result, err
	}
	s.matches[id] = m
	if !req.Copy {
		s.imported[payload.Manifest.TransferID] = *m.importReceipt
	}
	m.importReceipt = nil
	return ImportResult{Session: session, CompletionReceipt: receipt}, nil
}
func restoreCommands(m *Match, commands map[string]storedCommand) {
	for id, c := range commands {
		var err error
		if c.Error != nil {
			err = c.Error
		} else if c.RejectedTransition {
			err = statemachine.ErrNotPermitted
		} else if c.OtherError != "" {
			err = errors.New(c.OtherError)
		}
		m.commands[id] = cachedCommand{c.Hash, c.Receipt, err}
	}
}
func (a *Access) CancelTransfer(req GameControl) (GameInfo, error) {
	return a.control("cancel_transfer", req.ID, req.Revision, req, true, false, func() error {
		if !req.Confirm {
			return ruleError("confirmation_required", "Confirm that this game is not running on the destination host.")
		}
		r := a.match.room
		if r.Transfer == nil || r.Runtime.State() != runtimeFrozen {
			return ruleError("invalid_state", "There is no pending move to cancel.")
		}
		if err := fireRoom(r.Runtime, restoreRuntime, &roomContext{Match: a.match}); err != nil {
			return err
		}
		r.audit(a.memberID, "transfer_cancelled", "Cancelled the move. The game remains paused.")
		r.Transfer = nil
		return nil
	})
}
func (a *Access) CompleteTransfer(req CompleteTransfer) (GameInfo, error) {
	return a.control("complete_transfer", req.ID, req.Revision, req, true, false, func() error {
		r := a.match.room
		if r.Transfer == nil || r.Runtime.State() != runtimeFrozen {
			return ruleError("invalid_state", "There is no pending move.")
		}
		prefix := r.Transfer.ID + "."
		if len(req.Receipt) != len(prefix)+64 || req.Receipt[:len(prefix)] != prefix {
			return ruleError("invalid_receipt", "Use the completion receipt from the destination host.")
		}
		h := tokenHash(req.Receipt[len(prefix):])
		if hex.EncodeToString(h[:]) != r.Transfer.ReceiptHash {
			return ruleError("invalid_receipt", "The completion receipt does not match this move.")
		}
		if err := fireRoom(r.Runtime, releaseRuntime, &roomContext{Match: a.match}); err != nil {
			return err
		}
		r.audit(a.memberID, "transfer_completed", "The destination imported the game. This host is retired.")
		return nil
	})
}
