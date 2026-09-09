package httpapi

import (
	"crowns/internal/game"
	"crowns/internal/matches"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const browserCookie = "aoe_browser"

func browserID(r *http.Request) string {
	c, err := r.Cookie(browserCookie)
	if err != nil {
		return ""
	}
	return c.Value
}
func ensureBrowser(w http.ResponseWriter, r *http.Request) string {
	if id := browserID(r); len(id) >= 26 && len(id) <= 128 {
		return id
	}
	id := rand.Text()
	http.SetCookie(w, &http.Cookie{Name: browserCookie, Value: id, Path: "/", HttpOnly: true, Secure: r.TLS != nil || strings.HasPrefix(r.Header.Get("Origin"), "https://"), SameSite: http.SameSiteStrictMode, MaxAge: 365 * 24 * 60 * 60})
	return id
}
func bearer(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(auth, "Bearer ")
}
func (s *Server) gameAccess(w http.ResponseWriter, r *http.Request) *matches.Access {
	a, err := s.matches.RequestAccess(r.PathValue("id"), bearer(r), browserID(r))
	if err != nil {
		domainError(w, err)
		return nil
	}
	return a
}
func gameRequest[T any](s *Server, w http.ResponseWriter, r *http.Request, fn func(*matches.Access, T) (any, error)) {
	a := s.gameAccess(w, r)
	if a == nil {
		return
	}
	defer a.Release()
	var req T
	if !decode(w, r, &req) {
		return
	}
	result, err := fn(a, req)
	if err != nil {
		domainError(w, err)
		return
	}
	respond(w, 200, result)
}
func (s *Server) gameRoutes() {
	s.mux.HandleFunc("POST /api/v1/matches/{id}/adopt", func(w http.ResponseWriter, r *http.Request) {
		v, err := s.matches.AdoptLegacy(r.PathValue("id"), bearer(r), ensureBrowser(w, r))
		if err != nil {
			domainError(w, err)
			return
		}
		respond(w, 200, v)
	})
	s.mux.HandleFunc("POST /api/v1/games", func(w http.ResponseWriter, r *http.Request) {
		var req matches.CreateGame
		if !decode(w, r, &req) {
			return
		}
		v, err := s.matches.CreateGame(req, ensureBrowser(w, r))
		if err != nil {
			domainError(w, err)
			return
		}
		w.Header().Set("Location", "/api/v1/games/"+v.MatchID)
		respond(w, 201, v)
	})
	s.mux.HandleFunc("GET /api/v1/games", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if len(q) > 800 {
			writeError(w, 422, "invalid_query", "Search with up to 200 characters.")
			return
		}
		v, err := s.matches.Games(browserID(r), q)
		if err != nil {
			domainError(w, err)
			return
		}
		respond(w, 200, v)
	})
	s.mux.HandleFunc("GET /api/v1/games/{id}", func(w http.ResponseWriter, r *http.Request) {
		if a := s.gameAccess(w, r); a != nil {
			defer a.Release()
			v, err := a.Info()
			if err != nil {
				domainError(w, err)
				return
			}
			respond(w, 200, v)
		}
	})
	s.mux.HandleFunc("GET /api/v1/games/{id}/session", func(w http.ResponseWriter, r *http.Request) {
		if a := s.gameAccess(w, r); a != nil {
			defer a.Release()
			v, err := a.Session()
			if err != nil {
				domainError(w, err)
				return
			}
			respond(w, 200, v)
		}
	})
	s.mux.HandleFunc("GET /api/v1/games/{id}/snapshot", func(w http.ResponseWriter, r *http.Request) {
		if a := s.gameAccess(w, r); a != nil {
			defer a.Release()
			v, err := a.View()
			if err != nil {
				domainError(w, err)
				return
			}
			respond(w, 200, v)
		}
	})
	s.mux.HandleFunc("PATCH /api/v1/games/{id}/rules", func(w http.ResponseWriter, r *http.Request) {
		gameRequest(s, w, r, func(a *matches.Access, q matches.RulesChange) (any, error) { return a.ChangeRules(q) })
	})
	s.mux.HandleFunc("POST /api/v1/games/{id}/seats", func(w http.ResponseWriter, r *http.Request) {
		gameRequest(s, w, r, func(a *matches.Access, q matches.SeatChange) (any, error) { return a.AddSeat(q) })
	})
	s.mux.HandleFunc("PATCH /api/v1/games/{id}/seats/{seat}", func(w http.ResponseWriter, r *http.Request) {
		gameRequest(s, w, r, func(a *matches.Access, q matches.SeatChange) (any, error) {
			return a.ChangeSeat(r.PathValue("seat"), q)
		})
	})
	s.mux.HandleFunc("PUT /api/v1/games/{id}/seats/{seat}/ready", func(w http.ResponseWriter, r *http.Request) {
		gameRequest(s, w, r, func(a *matches.Access, q matches.ReadyRequest) (any, error) { return a.Ready(r.PathValue("seat"), q) })
	})
	s.mux.HandleFunc("POST /api/v1/games/{id}/seats/{seat}/invites", func(w http.ResponseWriter, r *http.Request) {
		gameRequest(s, w, r, func(a *matches.Access, q matches.InviteRequest) (any, error) { return a.Invite(r.PathValue("seat"), q) })
	})
	s.mux.HandleFunc("DELETE /api/v1/games/{id}/invites/{invite}", func(w http.ResponseWriter, r *http.Request) {
		gameRequest(s, w, r, func(a *matches.Access, q matches.GameControl) (any, error) {
			return a.RevokeInvite(r.PathValue("invite"), q)
		})
	})
	s.mux.HandleFunc("POST /api/v1/invites/inspect", func(w http.ResponseWriter, r *http.Request) {
		var q matches.InviteSecret
		if !decode(w, r, &q) {
			return
		}
		v, err := s.matches.InspectInvite(q.Secret)
		if err != nil {
			domainError(w, err)
			return
		}
		respond(w, 200, v)
	})
	s.mux.HandleFunc("POST /api/v1/invites/claim", func(w http.ResponseWriter, r *http.Request) {
		var q matches.ClaimInvite
		if !decode(w, r, &q) {
			return
		}
		v, err := s.matches.ClaimInvite(q, ensureBrowser(w, r))
		if err != nil {
			domainError(w, err)
			return
		}
		respond(w, 200, v)
	})
	s.mux.HandleFunc("POST /api/v1/memberships/rejoin", func(w http.ResponseWriter, r *http.Request) {
		var q matches.RejoinRequest
		if !decode(w, r, &q) {
			return
		}
		v, err := s.matches.Rejoin(q, ensureBrowser(w, r))
		if err != nil {
			domainError(w, err)
			return
		}
		respond(w, 200, v)
	})
	controls := map[string]func(*matches.Access, matches.GameControl) (matches.GameInfo, error){"start": (*matches.Access).Start, "pause": (*matches.Access).Pause, "resume": (*matches.Access).ResumeGame, "speed": (*matches.Access).Speed, "close": (*matches.Access).CloseGame, "reopen": (*matches.Access).Reopen, "cancel-close": (*matches.Access).CancelClose, "cancel-transfer": (*matches.Access).CancelTransfer}
	for path, fn := range controls {
		s.mux.HandleFunc("POST /api/v1/games/{id}/"+path, func(w http.ResponseWriter, r *http.Request) {
			gameRequest(s, w, r, func(a *matches.Access, q matches.GameControl) (any, error) { return fn(a, q) })
		})
	}
	s.mux.HandleFunc("POST /api/v1/games/{id}/save", func(w http.ResponseWriter, r *http.Request) {
		if a := s.gameAccess(w, r); a != nil {
			defer a.Release()
			v, err := a.SaveGame()
			if err != nil {
				domainError(w, err)
				return
			}
			respond(w, 200, v)
		}
	})
	s.mux.HandleFunc("DELETE /api/v1/games/{id}", func(w http.ResponseWriter, r *http.Request) {
		a := s.gameAccess(w, r)
		if a == nil {
			return
		}
		defer a.Release()
		var q matches.GameControl
		if !decode(w, r, &q) {
			return
		}
		if err := s.matches.DeleteGame(a, q); err != nil {
			domainError(w, err)
			return
		}
		w.WriteHeader(204)
	})
	s.mux.HandleFunc("POST /api/v1/games/{id}/commands", func(w http.ResponseWriter, r *http.Request) {
		gameRequest(s, w, r, func(a *matches.Access, q game.Command) (any, error) { return a.Apply(q) })
	})
	s.mux.HandleFunc("GET /api/v1/games/{id}/marketplace", func(w http.ResponseWriter, r *http.Request) {
		if a := s.gameAccess(w, r); a != nil {
			defer a.Release()
			v, err := a.View()
			if err != nil {
				domainError(w, err)
				return
			}
			respond(w, 200, v.Marketplace)
		}
	})
	s.mux.HandleFunc("POST /api/v1/games/{id}/placement", func(w http.ResponseWriter, r *http.Request) {
		gameRequest(s, w, r, func(a *matches.Access, q matches.Placement) (any, error) { return a.Placement(q) })
	})
	s.mux.HandleFunc("GET /api/v1/games/{id}/log", s.journal)
	s.mux.HandleFunc("GET /api/v1/games/{id}/entities/{entity}/history", s.journal)
	s.mux.HandleFunc("GET /api/v1/games/{id}/audit", func(w http.ResponseWriter, r *http.Request) {
		if a := s.gameAccess(w, r); a != nil {
			defer a.Release()
			after, _ := strconv.Atoi(r.URL.Query().Get("after"))
			v, err := a.Audit(after)
			if err != nil {
				domainError(w, err)
				return
			}
			respond(w, 200, v)
		}
	})
	s.mux.HandleFunc("POST /api/v1/games/{id}/connections", func(w http.ResponseWriter, r *http.Request) {
		if a := s.gameAccess(w, r); a != nil {
			defer a.Release()
			v, err := a.Connect()
			if err != nil {
				domainError(w, err)
				return
			}
			respond(w, 201, v)
		}
	})
	s.mux.HandleFunc("POST /api/v1/games/{id}/connections/{connection}/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		if a := s.gameAccess(w, r); a != nil {
			defer a.Release()
			if err := a.Heartbeat(r.PathValue("connection")); err != nil {
				domainError(w, err)
				return
			}
			w.WriteHeader(204)
		}
	})
	s.mux.HandleFunc("POST /api/v1/games/{id}/connections/{connection}/leave", func(w http.ResponseWriter, r *http.Request) {
		if a := s.gameAccess(w, r); a != nil {
			defer a.Release()
			if err := a.Disconnect(r.PathValue("connection")); err != nil {
				domainError(w, err)
				return
			}
			w.WriteHeader(204)
		}
	})
	s.mux.HandleFunc("GET /api/v1/games/{id}/events", s.gameEvents)
	s.mux.HandleFunc("POST /api/v1/games/{id}/transfers", func(w http.ResponseWriter, r *http.Request) {
		gameRequest(s, w, r, func(a *matches.Access, q matches.TransferRequest) (any, error) { return a.Transfer(q) })
	})
	s.mux.HandleFunc("POST /api/v1/games/{id}/transfers/complete", func(w http.ResponseWriter, r *http.Request) {
		gameRequest(s, w, r, func(a *matches.Access, q matches.CompleteTransfer) (any, error) { return a.CompleteTransfer(q) })
	})
	s.mux.HandleFunc("GET /api/v1/games/{id}/transfers/{transfer}/archive", s.downloadArchive)
	s.mux.HandleFunc("POST /api/v1/games/{id}/database", s.downloadDatabase)
	s.mux.HandleFunc("POST /api/v1/games/{id}/transfers/{transfer}/archive", s.downloadArchive)
	s.mux.HandleFunc("POST /api/v1/game-imports", s.importGame)
}
func (s *Server) gameEvents(w http.ResponseWriter, r *http.Request) {
	a := s.gameAccess(w, r)
	if a == nil {
		return
	}
	defer a.Release()
	id := r.URL.Query().Get("connection")
	if err := a.Heartbeat(id); err != nil {
		domainError(w, err)
		return
	}
	if _, err := a.View(); err != nil {
		domainError(w, err)
		return
	}
	defer a.Disconnect(id)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(200)
	controller := http.NewResponseController(w)
	finished, interrupted := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(interrupted)
		select {
		case <-s.matches.Stopping():
		case <-r.Context().Done():
		case <-finished:
			return
		}
		_ = controller.SetWriteDeadline(time.Now())
	}()
	defer func() { close(finished); <-interrupted }()
	delta := r.URL.Query().Get("format") == "delta-v1"
	interval := 100 * time.Millisecond
	if delta {
		interval = 50 * time.Millisecond
	}
	stream, started := game.SnapshotStream{}, time.Now()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		_ = controller.SetWriteDeadline(time.Now().Add(10 * time.Second))
		select {
		case <-s.matches.Stopping():
			return
		case <-r.Context().Done():
			return
		default:
		}
		if err := a.Heartbeat(id); err != nil {
			return
		}
		v, err := a.View()
		if err != nil {
			return
		}
		var payload any = v
		event, sequence := "snapshot", v.Tick
		if delta {
			frame := stream.Next(v, float64(time.Since(started))/float64(time.Millisecond))
			payload, event, sequence = frame, "frame", frame.Sequence
		}
		data, err := json.Marshal(payload)
		if err != nil {
			return
		}
		if _, err = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", sequence, event, data); err != nil {
			return
		}
		if err = controller.Flush(); err != nil {
			return
		}
		select {
		case <-s.matches.Stopping():
			return
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}
func (s *Server) downloadArchive(w http.ResponseWriter, r *http.Request) {
	a := s.gameAccess(w, r)
	if a == nil {
		return
	}
	defer a.Release()
	var req matches.ArchivePassword
	if r.Method == "POST" && !decode(w, r, &req) {
		return
	}
	data, err := a.Archive(r.PathValue("transfer"), req.Passphrase)
	if err != nil {
		domainError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="`+r.PathValue("id")+`.aoegame"`)
	w.WriteHeader(200)
	_, _ = w.Write(data)
}
func (s *Server) downloadDatabase(w http.ResponseWriter, r *http.Request) {
	a := s.gameAccess(w, r)
	if a == nil {
		return
	}
	defer a.Release()
	data, err := a.Database(r.Context())
	if err != nil {
		domainError(w, err)
		return
	}
	defer data.Close()
	w.Header().Set("Content-Type", "application/vnd.sqlite3")
	w.Header().Set("Content-Disposition", `attachment; filename="`+r.PathValue("id")+`.sqlite"`)
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, data)
}
func (s *Server) importGame(w http.ResponseWriter, r *http.Request) {
	// Multipart carries binary archives without JSON/base64 expansion. Form
	// fields are bounded independently; uploads never become filesystem paths.
	r.Body = http.MaxBytesReader(w, r.Body, matches.MaxArchiveBytes+(1<<20))
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		writeError(w, 400, "invalid_archive", "Choose an .aoegame file up to 64 MB.")
		return
	}
	defer r.MultipartForm.RemoveAll()
	file, _, err := r.FormFile("archive")
	if err != nil {
		writeError(w, 400, "invalid_archive", "Choose an .aoegame file.")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, matches.MaxArchiveBytes+1))
	if err != nil {
		writeError(w, 400, "invalid_archive", "The archive could not be read.")
		return
	}
	v, err := s.matches.Import(matches.ImportRequest{Archive: data, Passphrase: r.FormValue("passphrase"), RejoinCode: r.FormValue("rejoin_code"), Copy: r.FormValue("copy") == "true", Name: r.FormValue("name")}, ensureBrowser(w, r))
	if err != nil {
		domainError(w, err)
		return
	}
	respond(w, 201, v)
}
