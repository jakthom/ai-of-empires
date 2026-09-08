package httpapi

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"crowns/internal/matches"
)

func TestShutdownRefusesRequestsAndClosesLiveStream(t *testing.T) {
	s := testServer()
	seat := createSession(t, s)
	server := httptest.NewServer(s)
	defer server.Close()
	defer s.matches.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	r, _ := http.NewRequestWithContext(ctx, "GET", server.URL+"/api/v1/matches/"+seat.MatchID+"/events", nil)
	r.Header.Set("Authorization", "Bearer "+seat.Token)
	response, err := server.Client().Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatalf("stream status: %d", response.StatusCode)
	}
	ended := make(chan struct{})
	go func() { defer close(ended); _, _ = io.Copy(io.Discard, response.Body) }()
	s.matches.BeginShutdown()
	for _, path := range []string{"/api/v1/health", "/api/v1/sessions", "/api/v1/matches/" + seat.MatchID, "/"} {
		w := request(t, s, "GET", path, seat.Token, nil)
		body := decodeResponse[ErrorBody](t, w, 503)
		if body.Error.Code != "server_shutting_down" || w.Header().Get("Retry-After") != "1" {
			t.Fatalf("shutdown response: %+v, %v", body, w.Header())
		}
	}
	select {
	case <-ended:
	case <-time.After(time.Second):
		t.Fatal("live stream kept shutdown open")
	}
	drain, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	if err := server.Config.Shutdown(drain); err != nil {
		t.Fatalf("HTTP failed to drain after ending SSE: %v", err)
	}
}

type delayedBody struct {
	io.Reader
	entered chan struct{}
	resume  chan struct{}
	once    sync.Once
}

func (b *delayedBody) Read(p []byte) (int, error) {
	b.once.Do(func() { close(b.entered); <-b.resume })
	return b.Reader.Read(p)
}
func (*delayedBody) Close() error { return nil }

func TestAuthorizedCommandBodyArrivingDuringShutdownCannotMutate(t *testing.T) {
	for _, closed := range []bool{false, true} {
		t.Run(map[bool]string{false: "draining", true: "closed"}[closed], func(t *testing.T) {
			s := testServer()
			seat := createSession(t, s)
			defer s.matches.Close()
			m, _ := s.matches.Authorized(seat.MatchID, seat.Token)
			before := m.View()
			body := &delayedBody{Reader: strings.NewReader(`{"id":"late","kind":"pause"}`), entered: make(chan struct{}), resume: make(chan struct{})}
			r := httptest.NewRequest("POST", "/api/v1/matches/"+seat.MatchID+"/commands", body)
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Authorization", "Bearer "+seat.Token)
			w := httptest.NewRecorder()
			done := make(chan struct{})
			go func() { defer close(done); s.ServeHTTP(w, r) }()
			// Body reading starts after authorization has returned a Match.
			select {
			case <-body.entered:
			case <-time.After(time.Second):
				close(body.resume)
				t.Fatal("handler never read the command body")
			}
			s.matches.BeginShutdown()
			if closed {
				if err := s.matches.Close(); err != nil {
					t.Fatal(err)
				}
			}
			close(body.resume)
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("late command handler did not finish")
			}
			status := http.StatusServiceUnavailable
			if closed {
				status = http.StatusNotFound
			}
			decodeResponse[ErrorBody](t, w, status)
			if !reflect.DeepEqual(before, m.View()) {
				t.Fatal("late command mutated a frozen world")
			}
		})
	}
}

// A peer that never reads eventually blocks Write. This writer makes that
// boundary deterministic, without relying on a platform's TCP buffer size.
type blockedStreamWriter struct {
	header      http.Header
	writing     chan struct{}
	interrupted chan struct{}
	writeOnce   sync.Once
	stopOnce    sync.Once
}

func (w *blockedStreamWriter) Header() http.Header { return w.header }
func (*blockedStreamWriter) WriteHeader(int)       {}
func (*blockedStreamWriter) Flush()                {}
func (w *blockedStreamWriter) Write([]byte) (int, error) {
	w.writeOnce.Do(func() { close(w.writing) })
	<-w.interrupted
	return 0, net.ErrClosed
}
func (w *blockedStreamWriter) SetWriteDeadline(deadline time.Time) error {
	if !deadline.After(time.Now()) {
		w.stopOnce.Do(func() { close(w.interrupted) })
	}
	return nil
}

func TestShutdownInterruptsBlockedStreamWrite(t *testing.T) {
	s := testServer()
	seat := createSession(t, s)
	defer s.matches.Close()
	r := httptest.NewRequest("GET", "/api/v1/matches/"+seat.MatchID+"/events", nil)
	r.Header.Set("Authorization", "Bearer "+seat.Token)
	w := &blockedStreamWriter{header: make(http.Header), writing: make(chan struct{}), interrupted: make(chan struct{})}
	defer w.SetWriteDeadline(time.Now())
	done := make(chan struct{})
	go func() { defer close(done); s.ServeHTTP(w, r) }()
	select {
	case <-w.writing:
	case <-time.After(time.Second):
		t.Fatal("stream did not reach its first write")
	}
	s.matches.BeginShutdown()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("shutdown left a stream blocked in Write")
	}
	if _, err := s.matches.List(""); err != matches.ErrShuttingDown {
		t.Fatalf("stream shutdown did not share service admission: %v", err)
	}
}
