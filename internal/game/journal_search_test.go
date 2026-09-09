package game

import (
	"fmt"
	"reflect"
	"testing"
)

func TestJournalSearchCombinesIdentityCategoryAndFullHistoryPaging(t *testing.T) {
	w := New(Config{Difficulty: "peaceful"})
	worker := w.entities(1, "villager")[1]
	expected := []Event{}
	for i := range 350 {
		kind := "order"
		if i%3 == 0 {
			kind = "delivery"
		}
		w.record(Event{Kind: kind, Message: fmt.Sprintf("Quarry work %d", i), Resource: "stone"}, worker, 0)
		if kind == "delivery" {
			expected = append(expected, w.journal.records[len(w.journal.records)-1])
		}
	}
	query := LogQuery{Search: fmt.Sprintf("vILlaGer #%d STONE", worker.ID), Category: "economy", FromStart: true, Limit: 17}
	got := []Event{}
	for {
		page, err := w.Log(1, query)
		if err != nil || len(page.Events) == 0 || page.LatestCursor != expected[len(expected)-1].ID {
			t.Fatalf("search failed: %+v %v", page, err)
		}
		got = append(got, page.Events...)
		query.After = page.NewerCursor
		if !page.HasNewer {
			break
		}
	}
	if !reflect.DeepEqual(got, expected) {
		t.Fatal("filtered forward pages omitted, duplicated, or altered old events")
	}
	query.FromStart, query.After, query.Before = false, 0, got[len(got)-1].ID
	older, _ := w.Log(1, query)
	if !older.HasOlder || !older.HasNewer || older.NewerCursor >= query.Before || len(older.Events) != 17 {
		t.Fatal("backward cursors must refer only to matching events")
	}
	query.FromStart, query.After, query.Before = true, got[len(got)-1].ID, 0
	page, _ := w.Log(1, query)
	if len(page.Events) != 0 || page.HasNewer {
		t.Fatal("unrelated new orders are not new search results")
	}
	w.record(Event{Kind: "delivery", Message: "Delivered stone", Resource: "stone"}, worker, 0)
	page, _ = w.Log(1, query)
	if len(page.Events) != 1 || page.NewerCursor <= query.After {
		t.Fatal("live search did not include a newly appended match")
	}
	page.Events[0].Message = "tampered"
	again, _ := w.Log(1, query)
	if again.Events[0].Message != "Delivered stone" {
		t.Fatal("search result aliases immutable history")
	}
}

func TestJournalIdentitySearchAndVisibility(t *testing.T) {
	w := New(Config{Difficulty: "peaceful"})
	worker := w.entities(1, "villager")[1]
	for _, q := range []string{fmt.Sprintf("Villager %d", worker.ID), fmt.Sprintf("villager%d", worker.ID), fmt.Sprintf("#%d", worker.ID)} {
		page, _ := w.Log(1, LogQuery{Search: q})
		if len(page.Events) != 1 || page.Events[0].EntityID != worker.ID {
			t.Fatalf("identity search %q: %+v", q, page)
		}
	}
	if matchesLog(Event{EntityID: 30, EntityName: "Villager", Message: "Helping Villager #3", TargetID: 3}, searchWords("villager 3"), "") {
		t.Fatal("identity query matched an ID prefix or mentioned target")
	}
	enemy := w.entities(2, "villager")[0]
	w.entityEvent(enemy, "order", "Hidden quarry orders", 0)
	page, _ := w.Log(1, LogQuery{Search: "quarry"})
	if len(page.Events) != 0 {
		t.Fatal("search exposed hidden enemy history")
	}
	private, _ := w.Log(2, LogQuery{Search: "quarry"})
	if len(private.Events) != 1 {
		t.Fatal("search cache mixed players")
	}
	enemy.Position = worker.Position
	w.refreshVisibility()
	w.entityEvent(enemy, "order", "Visible quarry orders", 0)
	page, _ = w.Log(1, LogQuery{Search: "quarry"})
	if len(page.Events) != 0 {
		t.Fatal("foreign activity became searchable after discovery")
	}
	empty, _ := w.Log(1, LogQuery{EntityID: worker.ID, Search: "no match"})
	if len(empty.Events) != 0 || empty.LatestCursor != 0 || empty.Entity == nil || !empty.Entity.Present {
		t.Fatal("an empty search must retain the entity's current status")
	}
	for i := range 12 {
		_, _ = w.Log(1, LogQuery{Search: fmt.Sprintf("absent%d", i)})
	}
	page, _ = w.Log(1, LogQuery{Search: "quarry"})
	if len(page.Events) != 0 {
		t.Fatal("switching searches exposed foreign activity")
	}
}

func TestJournalSearchAmountsRemainTextUnlessAnIdentityIsRequested(t *testing.T) {
	w := New(Config{Difficulty: "peaceful"})
	worker := w.entities(1, "villager")[1]
	w.record(Event{Kind: "delivery", Message: "Delivered 10.00 wood", Resource: "wood"}, worker, 0)
	for _, query := range []string{"delivered 10 wood", "villager delivered 10 wood", "Villager 3 delivered 10 wood", "#3 delivered 10 wood", "# 3 delivered 10 wood", "3"} {
		page, err := w.Log(1, LogQuery{Search: query, Category: "economy"})
		if err != nil || len(page.Events) != 1 || page.Events[0].EntityID != worker.ID {
			t.Fatalf("search %q lost a matching resource amount: %+v %v", query, page, err)
		}
	}
	for _, query := range []string{"delivered #10 wood", "villager 30", "delivered 20 wood"} {
		page, _ := w.Log(1, LogQuery{Search: query})
		if len(page.Events) != 0 {
			t.Fatalf("search %q confused an identity with a resource amount", query)
		}
	}
}
