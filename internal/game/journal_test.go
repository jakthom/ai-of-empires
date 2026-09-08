package game

import (
	"fmt"
	"math"
	"reflect"
	"testing"
)

func TestJournalIsAppendOnlyPagedAndRetainsRemovedEntities(t *testing.T) {
	w := New(Config{Difficulty: "peaceful"})
	worker := w.entities(1, "villager")[0]
	first, err := w.Log(1, LogQuery{EntityID: worker.ID, FromStart: true})
	if err != nil || len(first.Events) != 1 || first.Events[0].Kind != "created" {
		t.Fatalf("missing creation: %+v %v", first, err)
	}
	original := first.Events[0]
	first.Events[0].Message = "tampered"
	first.Entity.Activity = "tampered"
	for i := range 321 {
		if err := w.Apply(1, Command{ID: fmt.Sprint(i), Kind: "stop", EntityIDs: []int{worker.ID}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Apply(1, Command{Kind: "delete", EntityIDs: []int{worker.ID}}); err != nil {
		t.Fatal(err)
	}
	all := []Event{}
	cursor := 0
	for {
		page, err := w.Log(1, LogQuery{EntityID: worker.ID, FromStart: true, After: cursor, Limit: 37})
		if err != nil {
			t.Fatal(err)
		}
		if page.Entity.Present || page.Entity.Activity != "Destroyed" {
			t.Fatalf("removed entity status: %+v", page.Entity)
		}
		for _, event := range page.Events {
			if event.ID <= cursor {
				t.Fatal("cursor repeated or moved backwards")
			}
			cursor = event.ID
			all = append(all, event)
		}
		if !page.HasNewer {
			break
		}
	}
	if len(all) < 324 || all[0] != original {
		t.Fatalf("history truncated or mutated: %d entries, first %+v", len(all), all[0])
	}
	last, _ := w.Log(1, LogQuery{EntityID: worker.ID, Limit: 23})
	older, _ := w.Log(1, LogQuery{EntityID: worker.ID, Before: last.OlderCursor, Limit: 23})
	if !last.HasOlder || !older.HasNewer || older.NewerCursor >= last.OlderCursor {
		t.Fatal("backward pages must not overlap")
	}
	snapshot := w.View(1)
	if len(snapshot.Events) != 40 || snapshot.EventCursor != snapshot.Events[39].ID {
		t.Fatal("snapshots must carry a bounded tail and catch-up cursor")
	}
}

func TestJournalVisibilityIsCapturedWhenTheEventHappens(t *testing.T) {
	w := New(Config{Difficulty: "peaceful"})
	enemy := w.entities(2, "villager")[0]
	w.entityEvent(enemy, "private_test", "A hidden order", 0)
	if _, err := w.Log(1, LogQuery{EntityID: enemy.ID}); err == nil {
		t.Fatal("hidden entity history exposed")
	}
	enemy.Position = Vec{20, 45}
	w.refreshVisibility()
	page, err := w.Log(1, LogQuery{EntityID: enemy.ID})
	if err != nil || len(page.Events) != 1 || page.Events[0].Kind != "discovered" {
		t.Fatalf("exploration revealed earlier hidden history: %+v %v", page, err)
	}
	w.entityEvent(enemy, "observed_test", "A visible activity", 0)
	enemy.Position = Vec{57, 22}
	w.refreshVisibility()
	w.entityEvent(enemy, "private_test", "Another hidden order", 0)
	w.hit(enemy, 1, 10000)
	page, _ = w.Log(1, LogQuery{EntityID: enemy.ID})
	if page.Entity.Present || page.Entity.Activity == "Destroyed" {
		t.Fatal("hidden death leaked")
	}
	if len(page.Events) != 2 || page.Events[1].Kind != "observed_test" {
		t.Fatal("history visibility changed after losing sight")
	}
	for _, event := range w.View(1).Events {
		if event.Kind == "private_test" {
			t.Fatal("hidden events leaked through SSE read model")
		}
	}
}

func TestJournalExplainsResourceWorkAndDelivery(t *testing.T) {
	for _, test := range []struct{ typ, resource, activity string }{{"tree", "wood", "Logging"}, {"stone", "stone", "Mining stone"}, {"farm", "food", "Farming"}} {
		t.Run(test.typ, func(t *testing.T) {
			w := New(Config{Difficulty: "peaceful"})
			worker := w.entities(1, "villager")[0]
			owner := 0
			if test.typ == "farm" {
				owner = 1
			}
			source := w.spawn(test.typ, owner, Vec{worker.Position.X + 1, worker.Position.Y})
			source.Resource, source.Amount = test.resource, 1
			if err := w.Apply(1, Command{ID: "work", Kind: "interact", EntityIDs: []int{worker.ID}, TargetID: source.ID}); err != nil {
				t.Fatal(err)
			}
			for range 2 {
				w.Update()
			}
			if got := w.entityView(worker, 1).Activity; got != test.activity {
				t.Fatalf("activity: %s", got)
			}
			journalLength, rng := len(w.journal.records), w.rng
			for range 5 {
				_ = w.View(1)
				_, _ = w.Log(1, LogQuery{EntityID: worker.ID})
			}
			if journalLength != len(w.journal.records) || rng != w.rng {
				t.Fatal("read models changed the simulation")
			}
			for range 800 {
				w.Update()
				page, _ := w.Log(1, LogQuery{EntityID: worker.ID})
				for _, event := range page.Events {
					if event.Kind == "delivery" {
						if event.Resource != test.resource || math.Abs(event.Amount-1) > 1e-6 {
							t.Fatalf("delivery event: %+v", event)
						}
						return
					}
				}
			}
			t.Fatal("no completed delivery in worker history")
		})
	}
}

func TestSupportedSpeedsPreserveFixedPhysicsStep(t *testing.T) {
	w := New(Config{Difficulty: "peaceful"})
	before := GetCatalog().Speeds
	GetCatalog().Speeds[0] = 900 // Read-model clients cannot change supported rules.
	if !reflect.DeepEqual(before, GetCatalog().Speeds) {
		t.Fatal("catalog aliases the rules")
	}
	for _, speed := range before {
		if err := w.Apply(1, Command{Kind: "speed", Value: speed}); err != nil {
			t.Fatal(err)
		}
		time := w.Time
		w.Update()
		if math.Abs(w.Time-time-Step) > 1e-8 || w.Speed != speed {
			t.Fatal("speed changed physics dt")
		}
	}
	for _, speed := range []float64{0, -1, 3.5, 64, math.NaN(), math.Inf(1)} {
		if w.Apply(1, Command{Kind: "speed", Value: speed}) == nil || w.Speed != 32 {
			t.Fatal("unsupported speed changed world")
		}
	}
}

func TestSharedSpeedChangeIsVisibleToEveryKingdomAndCheckpointed(t *testing.T) {
	w := New(Config{Settlements: 2, Difficulty: "peaceful"})
	for range 2 {
		if err := w.SetSpeed(32); err != nil {
			t.Fatal(err)
		}
	}
	data, err := w.Checkpoint()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := Restore(data, w.JournalSince(0))
	if err != nil {
		t.Fatal(err)
	}
	for _, world := range []*World{w, restored} {
		for player := 1; player <= 2; player++ {
			page, err := world.Log(player, LogQuery{Limit: 200})
			if err != nil {
				t.Fatal(err)
			}
			count := 0
			for _, event := range page.Events {
				if event.Message == "Game speed set to 32×" {
					count++
				}
			}
			if count != 1 {
				t.Fatalf("player%d observed %d speed-change events", player, count)
			}
		}
	}
}
