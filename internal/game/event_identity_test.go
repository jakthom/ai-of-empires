package game

import "testing"

func TestPrivateJournalSurvivesReclaimConversionAndRestore(t *testing.T) {
	w := New(Config{Difficulty: "peaceful"})
	w.BindUser(1, "alice")
	w.BindUser(2, "bob")
	a, b := w.entities(1, "villager")[0], w.entities(2, "villager")[0]
	b.Position = a.Position
	w.refreshVisibility()
	w.entityEvent(a, "personal", "Alice's work", 0)
	w.entityEvent(b, "personal", "Bob's work", 0)
	w.event(0, "World announcement")
	for id, user := range map[int]string{1: "alice", 2: "bob"} {
		page, err := w.Log(id, LogQuery{FromStart: true})
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range page.Events {
			if e.UserID != user || e.Player != id {
				t.Fatalf("foreign event: %+v", e)
			}
		}
	}
	// Acquiring another player's unit never transfers its earlier history.
	b.Owner = 1
	w.entityEvent(b, "personal", "Alice's new unit", 0)
	page, err := w.Log(1, LogQuery{EntityID: b.ID})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range page.Events {
		if e.Message == "Bob's work" {
			t.Fatal("conversion exposed former owner's log")
		}
	}
	old := w.JournalSince(0)
	w.BindUser(1, "charlie")
	if page, _ := w.Log(1, LogQuery{}); len(page.Events) != 0 {
		t.Fatal("seat reclaim exposed former member's history")
	}
	w.entityEvent(a, "personal", "Charlie's work", 0)
	data, err := w.Checkpoint()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := Restore(data, w.JournalSince(0))
	if err != nil {
		t.Fatal(err)
	}
	page, err = restored.Log(1, LogQuery{})
	if err != nil || len(page.Events) != 1 || page.Events[0].UserID != "charlie" {
		t.Fatalf("restored private index: %+v %v", page, err)
	}
	for i, record := range old {
		if restored.JournalSince(0)[i].Event != record.Event {
			t.Fatal("reclaim rewrote an immutable event")
		}
	}
}

func TestPrivateLegacyJournalDoesNotBecomeTheCurrentSeatHoldersHistory(t *testing.T) {
	w := New(Config{Difficulty: "peaceful"})
	journal := w.JournalSince(0)
	for i := range journal {
		journal[i].Event.UserID = ""
	}
	for _, p := range w.Players {
		p.UserID = ""
	}
	data, err := w.Checkpoint()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := RestoreForUsers(data, journal, map[int]RestoreUser{1: {ID: "alice", LegacyID: "alice"}, 2: {ID: "replacement-member"}})
	if err != nil {
		t.Fatal(err)
	}
	if page, _ := restored.Log(2, LogQuery{}); len(page.Events) != 0 {
		t.Fatal("old friend-seat events were assigned to its replacement")
	}
	if page, _ := restored.Log(1, LogQuery{}); len(page.Events) == 0 {
		t.Fatal("proven original owner lost personal history")
	}
	if len(restored.JournalSince(0)) != len(journal) {
		t.Fatal("unattributed legacy records were discarded")
	}
	restored.entityEvent(restored.entities(2, "villager")[0], "personal", "New member's work", 0)
	page, err := restored.Log(2, LogQuery{})
	if err != nil || len(page.Events) != 1 || page.Events[0].UserID != "replacement-member" {
		t.Fatal("current member did not receive their own new events", err)
	}
}
