package game

import (
	"strings"
	"testing"
)

func TestObservedProductionAndDiscoveryKeepOrdersPrivate(t *testing.T) {
	w := New(Config{Difficulty: "peaceful", Mode: "sandbox"})
	enemy := w.entities(2, "town_center")[0]
	if err := w.Apply(2, Command{Kind: "research", EntityIDs: []int{enemy.ID}, Product: "loom"}); err != nil {
		t.Fatal(err)
	}
	if v := w.entityView(enemy, 2); v.Activity != "Researching Loom" || len(v.Tasks) != 1 {
		t.Fatal("owner lost their production details", v)
	}
	for _, viewer := range []int{0, 1} {
		v := w.entityView(enemy, viewer)
		if v.Activity != "Observed" || v.State != "observed" || len(v.Tasks) > 0 || len(v.Actions) > 0 {
			t.Fatal("observer learned private research", v)
		}
	}
	w.record(Event{Kind: "discovered", Message: "Town Center discovered"}, enemy, 1)
	page, err := w.Log(1, LogQuery{EntityID: enemy.ID})
	if err != nil || len(page.Events) != 1 || strings.Contains(page.Entity.Activity, "Loom") || page.Events[0].State != "observed" {
		t.Fatal("discovery leaked production", page, err)
	}
}

func TestOldObservationsAreSanitizedWithoutRewritingHistory(t *testing.T) {
	w := New(Config{Difficulty: "peaceful"})
	enemy := w.entities(2, "town_center")[0]
	old := w.entityView(enemy, 2)
	old.Visible, old.Activity = false, "Researching Loom"
	old.Tasks = []Task{{Type: "research", Product: "loom"}}
	old.Rally = &Vec{40, 40}
	w.Players[1].Memory[enemy.ID] = old
	event := Event{ID: w.NextEvent + 1, Kind: "discovered", Player: 1, EntityID: enemy.ID, EntityType: enemy.Type, Activity: "Researching Loom", State: "seeking_resource", Resource: "gold"}
	w.journal.append(event, []int{1})
	v := w.View(1)
	for _, e := range v.Entities {
		if e.ID == enemy.ID && (e.Visible || e.Activity != "Observed" || len(e.Tasks) > 0 || e.Rally != nil) {
			t.Fatal("old checkpoint memory leaked private state", e)
		}
	}
	page, err := w.Log(1, LogQuery{EntityID: enemy.ID})
	if err != nil || page.Entity.Activity != "Last seen: Observed" || page.Events[0].Resource != "" || page.Events[0].State != "observed" {
		t.Fatal("old discovery leaked private state", page, err)
	}
	for _, query := range []string{"Loom", "researching", "seeking resource", "gold"} {
		for range 2 { // Check both a new search and the cached index.
			page, err := w.Log(1, LogQuery{EntityID: enemy.ID, Search: query})
			if err != nil || len(page.Events) != 0 || page.LatestCursor != 0 || page.HasNewer || page.HasOlder {
				t.Fatal("search disclosed redacted discovery details", query, page, err)
			}
		}
	}
	if w.journal.records[len(w.journal.records)-1] != event || w.Players[1].Memory[enemy.ID].Activity != old.Activity {
		t.Fatal("observing rewrote checkpoint memory or immutable history")
	}
}
