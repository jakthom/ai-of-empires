package httpapi

import (
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"crowns/internal/game"
	"crowns/internal/matches"
)

func TestJournalAPIAuthenticationPagingAndExactlyOnceCommands(t *testing.T) {
	s := testServer()
	session := createSession(t, s)
	base := "/api/v1/matches/" + session.MatchID
	view := decodeResponse[game.Snapshot](t, request(t, s, "GET", base, session.Token, nil), 200)
	worker := 0
	for _, e := range view.Entities {
		if e.Type == "villager" && e.Owner == 1 {
			worker = e.ID
			break
		}
	}
	history := fmt.Sprintf("%s/entities/%d/history", base, worker)
	for _, path := range []string{base + "/log", history} {
		decodeResponse[ErrorBody](t, request(t, s, "GET", path, "", nil), 401)
		decodeResponse[ErrorBody](t, request(t, s, "GET", path, "wrong-token", nil), 401)
	}
	command := game.Command{ID: "record-once", Kind: "move", EntityIDs: []int{worker}, Position: &game.Vec{X: 18, Y: 48}}
	decodeResponse[matches.Receipt](t, request(t, s, "POST", base+"/commands", session.Token, command), 200)
	page := decodeResponse[game.EventPage](t, request(t, s, "GET", history, session.Token, nil), 200)
	decodeResponse[matches.Receipt](t, request(t, s, "POST", base+"/commands", session.Token, command), 200)
	replayed := decodeResponse[game.EventPage](t, request(t, s, "GET", history, session.Token, nil), 200)
	if !reflect.DeepEqual(page, replayed) {
		t.Fatal("command replay appended duplicate events")
	}
	first := decodeResponse[game.EventPage](t, request(t, s, "GET", history+"?after=0&limit=1", session.Token, nil), 200)
	if !first.HasNewer || len(first.Events) != 1 || first.Events[0].Kind != "created" {
		t.Fatal("after=0 must start at first event")
	}
	after := decodeResponse[game.EventPage](t, request(t, s, "GET", fmt.Sprintf("%s?after=%d", history, first.NewerCursor), session.Token, nil), 200)
	if len(after.Events) == 0 || after.Events[0].ID <= first.NewerCursor {
		t.Fatal("exclusive cursor failed")
	}
	for _, query := range []string{"after=-1", "after=", "after=0&before=3", "after=1&after=2", "before=0", "before=x", "limit=0", "limit=201", "unknown=1"} {
		decodeResponse[ErrorBody](t, request(t, s, "GET", base+"/log?"+query, session.Token, nil), 400)
	}
	for _, query := range []string{"q=a&q=b", "q=" + strings.Repeat("x", 201), "category=invalid", "category=orders&category=combat"} {
		decodeResponse[ErrorBody](t, request(t, s, "GET", base+"/log?"+query, session.Token, nil), 400)
	}
	filtered := decodeResponse[game.EventPage](t, request(t, s, "GET", base+"/log?q="+url.QueryEscape(fmt.Sprintf("Villager #%d", worker))+"&category=orders&limit=1", session.Token, nil), 200)
	if len(filtered.Events) != 1 || filtered.Events[0].EntityID != worker || filtered.Events[0].Kind != "order" {
		t.Fatal("HTTP did not combine identity and category filters")
	}
	empty := decodeResponse[game.EventPage](t, request(t, s, "GET", history+"?q=unmatched&category=combat", session.Token, nil), 200)
	if len(empty.Events) != 0 || empty.Entity == nil || !empty.Entity.Present {
		t.Fatal("filtered entity history lost its current status")
	}
	decodeResponse[ErrorBody](t, request(t, s, "GET", base+"/entities/999999/history", session.Token, nil), 404)
	request(t, s, "POST", base+"/commands", session.Token, game.Command{ID: "remove", Kind: "delete", EntityIDs: []int{worker}})
	removed := decodeResponse[game.EventPage](t, request(t, s, "GET", history, session.Token, nil), 200)
	if removed.Entity.Present || removed.Entity.Activity != "Destroyed" {
		t.Fatal("removed entity history unavailable")
	}
}
