package httpapi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"crowns/internal/game"
	"crowns/internal/matches"
)

func testServer() *Server {
	return New(matches.NewService(), fstest.MapFS{
		"index.html": {Data: []byte("<!doctype html><title>AI of Empires</title>")},
		"docs.html":  {Data: []byte("<!doctype html><title>AI of Empires API Reference</title>")},
	})
}

func request(t *testing.T, s http.Handler, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	r := httptest.NewRequest(method, path, bytes.NewReader(data))
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

func decodeResponse[T any](t *testing.T, r *httptest.ResponseRecorder, status int) T {
	t.Helper()
	if r.Code != status {
		t.Fatalf("status %d, wanted %d: %s", r.Code, status, r.Body)
	}
	var value T
	if err := json.Unmarshal(r.Body.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func createSession(t *testing.T, s *Server) matches.Session {
	t.Helper()
	return decodeResponse[matches.Session](t, request(t, s, "POST", "/api/v1/matches", "", game.Config{Civilization: "britons", Difficulty: "peaceful", Mode: "skirmish", Seed: 83}), 201)
}

func TestServerServesUIAndVersionedContract(t *testing.T) {
	s := testServer()
	ui := request(t, s, "GET", "/", "", nil)
	if ui.Code != 200 || !strings.Contains(ui.Body.String(), "AI of Empires") {
		t.Fatalf("UI unavailable: %s", ui.Body)
	}
	if ui.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("missing CSP")
	}
	docs := request(t, s, "GET", "/api/docs", "", nil)
	if docs.Code != 200 || !strings.Contains(docs.Body.String(), "API Reference") {
		t.Fatalf("API reference unavailable: %s", docs.Body)
	}
	decodeResponse[Health](t, request(t, s, "GET", "/api/v1/health", "", nil), 200)
	contract := decodeResponse[map[string]any](t, request(t, s, "GET", "/api/v1/openapi.json", "", nil), 200)
	if contract["openapi"] != "3.1.0" {
		t.Fatal("missing versioned contract")
	}
	decodeResponse[ErrorBody](t, request(t, s, "GET", "/api/v1/missing", "", nil), 404)
}

func TestMatchAuthorityAndStrictInputs(t *testing.T) {
	s := testServer()
	a, b := createSession(t, s), createSession(t, s)
	path := "/api/v1/matches/" + a.MatchID
	decodeResponse[ErrorBody](t, request(t, s, "GET", path, "", nil), 401)
	decodeResponse[ErrorBody](t, request(t, s, "GET", path, b.Token, nil), 401)
	v := decodeResponse[game.Snapshot](t, request(t, s, "GET", path, a.Token, nil), 200)
	var tc int
	for _, e := range v.Entities {
		if e.Owner == 2 {
			t.Fatal("unscouted opponent leaked into snapshot")
		}
		if e.Type == "town_center" && e.Owner == 1 {
			tc = e.ID
		}
	}
	if tc == 0 {
		t.Fatal("missing own Town Center")
	}
	for _, property := range []string{"hp", "resources", "owner", "damage"} {
		body := map[string]any{"id": property, "kind": "stop", "entity_ids": []int{tc}, property: 9999}
		decodeResponse[ErrorBody](t, request(t, s, "POST", path+"/commands", a.Token, body), 400)
	}
	invalid := game.Command{ID: "foreign-unit", Kind: "stop", EntityIDs: []int{999999}}
	decodeResponse[ErrorBody](t, request(t, s, "POST", path+"/commands", a.Token, invalid), 422)
	r := httptest.NewRequest("POST", path+"/commands", strings.NewReader(`{"id":"a","kind":"pause"} {}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+a.Token)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	decodeResponse[ErrorBody](t, w, 400)
	r = httptest.NewRequest("GET", path, nil)
	r.Header.Set("Authorization", "Bearer "+a.Token)
	r.Header.Set("Origin", "https://unrelated.example")
	w = httptest.NewRecorder()
	s.ServeHTTP(w, r)
	decodeResponse[ErrorBody](t, w, 403)
}

func TestCommandRetriesCannotDoubleSpendOrRefund(t *testing.T) {
	s := testServer()
	session := createSession(t, s)
	path := "/api/v1/matches/" + session.MatchID
	view := func() game.Snapshot {
		return decodeResponse[game.Snapshot](t, request(t, s, "GET", path, session.Token, nil), 200)
	}
	before := view()
	var tc int
	for _, e := range before.Entities {
		if e.Type == "town_center" && e.Owner == 1 {
			tc = e.ID
		}
	}
	train := game.Command{ID: "train-1", Kind: "train", Product: "villager", EntityIDs: []int{tc}}
	first := decodeResponse[matches.Receipt](t, request(t, s, "POST", path+"/commands", session.Token, train), 200)
	again := decodeResponse[matches.Receipt](t, request(t, s, "POST", path+"/commands", session.Token, train), 200)
	if first != again || view().Player.Resources.Food != before.Player.Resources.Food-50 {
		t.Fatal("retry changed receipt or spent twice")
	}
	train.Kind = "cancel"
	decodeResponse[ErrorBody](t, request(t, s, "POST", path+"/commands", session.Token, train), 409)
	cancel := game.Command{ID: "cancel-1", Kind: "cancel", EntityIDs: []int{tc}}
	for range 2 {
		decodeResponse[matches.Receipt](t, request(t, s, "POST", path+"/commands", session.Token, cancel), 200)
	}
	if view().Player.Resources.Food != before.Player.Resources.Food {
		t.Fatal("refund was not exactly once")
	}
	pause := game.Command{ID: "pause-1", Kind: "pause"}
	for range 2 {
		decodeResponse[matches.Receipt](t, request(t, s, "POST", path+"/commands", session.Token, pause), 200)
	}
	if !view().Paused {
		t.Fatal("retry toggled pause twice")
	}
	if got := request(t, s, "DELETE", path, session.Token, nil); got.Code != 204 {
		t.Fatal(got.Code)
	}
	decodeResponse[ErrorBody](t, request(t, s, "GET", path, session.Token, nil), 404)
}

func TestSnapshotStreamStartsWithAuthorizedFullState(t *testing.T) {
	s := testServer()
	session := createSession(t, s)
	server := httptest.NewServer(s)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	r, err := http.NewRequestWithContext(ctx, "GET", server.URL+"/api/v1/matches/"+session.MatchID+"/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Authorization", "Bearer "+session.Token)
	response, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 || response.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatal(response.Status)
	}
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 4096), 2<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var snapshot game.Snapshot
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &snapshot); err != nil {
			t.Fatal(err)
		}
		if snapshot.Player.ID != session.PlayerID || len(snapshot.Entities) == 0 {
			t.Fatal("invalid stream state")
		}
		return
	}
	t.Fatalf("stream ended without snapshot: %v", scanner.Err())
}

func TestSchedulerAndHTTPSerializeConcurrentCommands(t *testing.T) {
	service := matches.NewService()
	s := New(service, fstest.MapFS{})
	session := createSession(t, s)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); service.Run(ctx) }()
	server := httptest.NewServer(s)
	defer server.Close()
	var readers sync.WaitGroup
	for worker := range 4 {
		readers.Add(1)
		go func() {
			defer readers.Done()
			t.Run(fmt.Sprintf("worker-%d", worker), func(t *testing.T) {
				for i := range 8 {
					r, _ := http.NewRequest("GET", server.URL+"/api/v1/matches/"+session.MatchID, nil)
					r.Header.Set("Authorization", "Bearer "+session.Token)
					response, err := http.DefaultClient.Do(r)
					if err != nil {
						t.Fatal(err)
					}
					_, err = io.Copy(io.Discard, response.Body)
					response.Body.Close()
					if err != nil {
						t.Fatal(err)
					}
					decodeResponse[matches.Receipt](t, request(t, s, "POST", "/api/v1/matches/"+session.MatchID+"/commands", session.Token, game.Command{ID: fmt.Sprintf("%d-%d", worker, i), Kind: "speed", Value: 1.7}), 200)
				}
			})
		}()
	}
	readers.Wait()
	ticks := time.NewTicker(10 * time.Millisecond)
	defer ticks.Stop()
	deadline := time.After(3 * time.Second)
waiting:
	for {
		select {
		case <-deadline:
			t.Fatal("scheduler did not advance alongside HTTP access")
		case <-ticks.C:
			v := decodeResponse[game.Snapshot](t, request(t, s, "GET", "/api/v1/matches/"+session.MatchID, session.Token, nil), 200)
			if v.Tick >= 3 {
				break waiting
			}
		}
	}
	cancel()
	<-done
}
