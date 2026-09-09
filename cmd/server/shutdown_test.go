package main

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"

	"crowns/internal/game"
	"crowns/internal/matches"
)

// Run the real main (including signal handling and exit codes) in an isolated,
// race-instrumented child. Port zero never touches the development server.
func TestServerProcess(t *testing.T) {
	if os.Getenv("AI_EMPIRES_SHUTDOWN_TEST") != "1" {
		return
	}
	os.Args = []string{"ai-of-empires", "-addr", "127.0.0.1:0", "-db", os.Getenv("AI_EMPIRES_SHUTDOWN_DB")}
	main()
}

type testProcess struct {
	cmd    *exec.Cmd
	url    string
	dbPath string
	logs   string
	done   chan struct{}
	err    error // read only after done closes
}

func startProcess(t *testing.T) *testProcess {
	t.Helper()
	dir := t.TempDir()
	p := &testProcess{dbPath: filepath.Join(dir, "sessions.sqlite"), logs: filepath.Join(dir, "server.log"), done: make(chan struct{})}
	output, err := os.Create(p.logs)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = output.Close() })
	p.cmd = exec.Command(os.Args[0], "-test.run=^TestServerProcess$")
	p.cmd.Env = append(os.Environ(), "AI_EMPIRES_SHUTDOWN_TEST=1", "AI_EMPIRES_SHUTDOWN_DB="+p.dbPath)
	p.cmd.Stdout, p.cmd.Stderr = output, output
	if err := p.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { p.err = p.cmd.Wait(); close(p.done) }()
	t.Cleanup(func() {
		select {
		case <-p.done:
		default:
			_ = p.cmd.Process.Kill()
			<-p.done
		}
	})
	readyURL := regexp.MustCompile(`url=(http://127\.0\.0\.1:\d+)`)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()
	for {
		select {
		case <-p.done:
			t.Fatalf("server exited before listening: %v\n%s", p.err, p.logText())
		case <-timeout.C:
			t.Fatalf("server did not listen\n%s", p.logText())
		case <-ticker.C:
			if found := readyURL.FindStringSubmatch(p.logText()); len(found) == 2 {
				p.url = found[1]
				return p
			}
		}
	}
}

func (p *testProcess) logText() string { data, _ := os.ReadFile(p.logs); return string(data) }

func (p *testProcess) stop(t *testing.T, signal os.Signal) error {
	t.Helper()
	if err := p.cmd.Process.Signal(signal); err != nil {
		t.Fatal(err)
	}
	select {
	case <-p.done:
		return p.err
	case <-time.After(5 * time.Second):
		t.Fatalf("signal did not stop the server promptly\n%s", p.logText())
		return nil
	}
}

func apiJSON[T any](t *testing.T, base, method, path, token string, body any, status int) T {
	t.Helper()
	var data []byte
	if body != nil {
		var err error
		data, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	r, err := http.NewRequest(method, base+path, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Content-Type", "application/json")
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != status {
		data, _ := io.ReadAll(response.Body)
		t.Fatalf("%s %s: status %d: %s", method, path, response.StatusCode, data)
	}
	var result T
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result
}

func sameJSON(t *testing.T, a, b any) bool {
	t.Helper()
	left, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	right, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	return bytes.Equal(left, right)
}

func TestSignalsCheckpointWithLiveSSEAndResumeCommandReceipts(t *testing.T) {
	for _, signal := range []os.Signal{os.Interrupt, syscall.SIGTERM} {
		t.Run(signal.String(), func(t *testing.T) {
			p := startProcess(t)
			seat := apiJSON[matches.Session](t, p.url, "POST", "/api/v1/matches", "", game.Config{Name: "Signal test", Civilization: "britons", Difficulty: "peaceful", Mode: "skirmish"}, 201)
			path := "/api/v1/matches/" + seat.MatchID
			cmd := game.Command{ID: "pause-once", Kind: "pause"}
			receipt := apiJSON[matches.Receipt](t, p.url, "POST", path+"/commands", seat.Token, cmd, 200)
			view := apiJSON[game.Snapshot](t, p.url, "GET", path, seat.Token, nil, 200)
			log := apiJSON[game.EventPage](t, p.url, "GET", path+"/log?after=0&limit=200", seat.Token, nil, 200)
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			r, _ := http.NewRequestWithContext(ctx, "GET", p.url+path+"/events", nil)
			r.Header.Set("Authorization", "Bearer "+seat.Token)
			response, err := http.DefaultClient.Do(r)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != 200 {
				t.Fatalf("live stream: %d", response.StatusCode)
			}
			ended := make(chan struct{})
			go func() { defer close(ended); _, _ = io.Copy(io.Discard, response.Body) }()
			if err := p.stop(t, signal); err != nil {
				t.Fatalf("graceful shutdown: %v\n%s", err, p.logText())
			}
			select {
			case <-ended:
			case <-time.After(time.Second):
				t.Fatal("shutdown left the stream open")
			}
			if !strings.Contains(p.logText(), "sessions checkpointed and database closed") {
				t.Fatalf("shutdown completion missing\n%s", p.logText())
			}
			restored, err := matches.OpenService(p.dbPath)
			if err != nil {
				t.Fatal(err)
			}
			defer restored.Close()
			m, err := restored.Authorized(seat.MatchID, seat.Token)
			if err != nil {
				t.Fatal(err)
			}
			history, err := m.Log(game.LogQuery{FromStart: true, Limit: 200})
			// Compare wire state: Event.Player is intentionally private and
			// absent from the HTTP response but present in the restored model.
			if err != nil || !sameJSON(t, view, m.View()) || !sameJSON(t, log, history) {
				t.Fatalf("signal lost checkpointed state or history: %v", err)
			}
			if retried, err := m.Apply(cmd); err != nil || retried != receipt || !m.View().Paused {
				t.Fatal("signal lost command idempotency")
			}
		})
	}
}

func TestSignalSaveFailureExitsNonzeroWithLastCheckpointIntact(t *testing.T) {
	p := startProcess(t)
	seat := apiJSON[matches.Session](t, p.url, "POST", "/api/v1/matches", "", game.Config{Civilization: "britons", Difficulty: "peaceful", Mode: "skirmish"}, 201)
	// Only this test's game database is modified; the app has no fault-injection API.
	gamePath := filepath.Join(p.dbPath+".games", seat.MatchID+".sqlite")
	db, err := sql.Open("sqlite", gamePath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TRIGGER reject_events BEFORE INSERT ON events BEGIN SELECT RAISE(ABORT,'disk unavailable'); END;"); err != nil {
		t.Fatal(err)
	}
	apiJSON[matches.Receipt](t, p.url, "POST", "/api/v1/matches/"+seat.MatchID+"/commands", seat.Token, game.Command{ID: "unsaved-pause", Kind: "pause"}, 200)
	err = p.stop(t, syscall.SIGTERM)
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("save failure exit: %v\n%s", err, p.logText())
	}
	if logs := p.logText(); !strings.Contains(logs, seat.MatchID) || !strings.Contains(logs, "disk unavailable") || strings.Contains(logs, "sessions checkpointed and database closed") {
		t.Fatalf("shutdown failure was misreported\n%s", logs)
	}
	if _, err := db.Exec("DROP TRIGGER reject_events"); err != nil {
		t.Fatal(err)
	}
	restored, err := matches.OpenService(p.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	m, err := restored.Authorized(seat.MatchID, seat.Token)
	if err != nil || m.View().Paused {
		t.Fatalf("failed save corrupted last committed game: %v", err)
	}
}

func TestDrainTimeoutForceClosesSlowRequestButStillSaves(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.sqlite")
	s, err := matches.OpenService(path)
	if err != nil {
		t.Fatal(err)
	}
	seat, err := s.Create(game.Config{Difficulty: "peaceful"})
	if err != nil {
		t.Fatal(err)
	}
	m, _ := s.Authorized(seat.MatchID, seat.Token)
	if _, err := m.Apply(game.Command{ID: "saved-pause", Kind: "pause"}); err != nil {
		t.Fatal(err)
	}
	before := m.View()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- serve(ctx, listener, s, nil, shutdownTimeouts{50 * time.Millisecond, 3 * time.Second}) }()
	conn, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	_, err = fmt.Fprintf(conn, "POST /api/v1/matches/%s/commands HTTP/1.1\r\nHost: %s\r\nAuthorization: Bearer %s\r\nContent-Type: application/json\r\nContent-Length: 32\r\nExpect: 100-continue\r\n\r\n", seat.MatchID, listener.Addr(), seat.Token)
	if err != nil {
		t.Fatal(err)
	}
	// net/http sends Continue when the authorized handler starts reading. Hold
	// back the body so graceful drain must time out and close this connection.
	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, nil)
	if err != nil || response.StatusCode != 100 {
		t.Fatalf("request did not reach body reading: %v", err)
	}
	_ = response.Body.Close()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "drain HTTP") || strings.Contains(err.Error(), "save sessions") {
			t.Fatalf("timeout or successful final save misreported: %v", err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("slow request blocked shutdown")
	}
	if _, err := reader.ReadByte(); err == nil {
		t.Fatal("forced close left the command connection open")
	} else if e, ok := err.(net.Error); ok && e.Timeout() {
		t.Fatal("connection only ended on the client's timeout")
	}
	restored, err := matches.OpenService(path)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	loaded, err := restored.Authorized(seat.MatchID, seat.Token)
	if err != nil || !reflect.DeepEqual(before, loaded.View()) {
		t.Fatalf("HTTP timeout prevented final checkpoint: %v", err)
	}
}
