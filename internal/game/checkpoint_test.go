package game

import (
	"bytes"
	"reflect"
	"testing"
)

func TestCheckpointRestoresSimulationWithoutReplayingEffects(t *testing.T) {
	w := New(Config{Difficulty: "expert", Mode: "sandbox", Settlements: 4})
	worker := w.entities(1, "villager")[0]
	source := w.entities(0, "tree")[0]
	if err := w.Apply(1, Command{Kind: "gather", EntityIDs: []int{worker.ID}, TargetID: source.ID}); err != nil {
		t.Fatal(err)
	}
	tc := w.entities(1, "town_center")[0]
	if err := w.Apply(1, Command{Kind: "train", EntityIDs: []int{tc.ID}, Product: "villager"}); err != nil {
		t.Fatal(err)
	}
	for range 301 {
		w.Update()
	}
	data, err := w.Checkpoint()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := Restore(data, w.JournalSince(0))
	if err != nil {
		t.Fatal(err)
	}
	roundtrip, _ := restored.Checkpoint()
	if !bytes.Equal(data, roundtrip) || !reflect.DeepEqual(w.JournalSince(0), restored.JournalSince(0)) {
		t.Fatal("restore changed state or replayed history")
	}
	for range 601 {
		w.Update()
		restored.Update()
	}
	a, _ := w.Checkpoint()
	b, _ := restored.Checkpoint()
	if !bytes.Equal(a, b) || !reflect.DeepEqual(w.JournalSince(0), restored.JournalSince(0)) {
		t.Fatal("restored clocks, paths, queues, RNG or state machines diverged")
	}
	if _, err = Restore(data, nil); err == nil {
		t.Fatal("accepted checkpoint with missing history")
	}
}

func TestSettlementCountsHaveEqualOpeningsAndAllOpponents(t *testing.T) {
	for _, n := range []int{1, 2, 3, 6} {
		w := New(Config{Settlements: n, Difficulty: "peaceful"})
		if len(w.Players) != n || len(w.View(1).Opponents) != n-1 {
			t.Fatalf("settlements %d not represented", n)
		}
		for id := 1; id <= n; id++ {
			if len(w.entities(id, "villager")) != 3 || len(w.entities(id, "scout")) != 1 || len(w.entities(id, "town_center")) != 1 {
				t.Fatalf("unequal opening for %d/%d", id, n)
			}
			if w.Players[id].Resources != w.Players[1].Resources {
				t.Fatal("unequal starting resources")
			}
			for _, e := range w.entities(id, "") {
				if !w.land(e.Position) {
					t.Fatalf("settlement %d spawned on impassable terrain", id)
				}
			}
		}
		w.Update()
		if w.Status() != "running" {
			t.Fatal("solo game finished without playing")
		}
		if n > 2 {
			if err := w.Apply(2, Command{Kind: "resign"}); err != nil {
				t.Fatal(err)
			}
			w.Update()
			if w.Status() != "running" {
				t.Fatal("game ended while multiple settlements remain")
			}
		}
	}
}
