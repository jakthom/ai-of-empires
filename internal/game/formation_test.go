package game

import (
	"math"
	"reflect"
	"testing"
)

func TestFollowingMovingDestinationDoesNotWaitForBlockedPathRetry(t *testing.T) {
	w := trafficWorld()
	e := w.spawn("militia", 1, Vec{10, 10})
	longest, stopped := 0, 0
	for i := range 400 {
		before := e.Position
		goal := Vec{10.9 + float64(i)*.035, 10}
		w.move(e, goal, .16, Step)
		if e.Position == before {
			stopped++
			longest = max(longest, stopped)
		} else {
			stopped = 0
		}
	}
	if longest > 2 || e.Position.X < 23 {
		t.Fatalf("visible stop/start motion: %d stationary ticks, at %v", longest, e.Position)
	}
}

func TestFormationMarchesInRanksAndArrivesWithoutPilingUp(t *testing.T) {
	w := trafficWorld()
	ids := []int{}
	for i := range 12 {
		typ := "militia"
		if i%3 == 0 {
			typ = "scout"
		}
		e := w.spawn(typ, 1, Vec{12 + float64(i%4), 20 + float64(i/4)})
		ids = append(ids, e.ID)
	}
	goal := Vec{45, 35}
	if err := w.Apply(1, Command{Kind: "move", EntityIDs: ids, Position: &goal}); err != nil {
		t.Fatal(err)
	}
	for range 400 {
		w.Update()
	}
	minProgress, maxProgress := math.Inf(1), math.Inf(-1)
	for _, id := range ids {
		e := w.Entities[id]
		m := e.Order.March
		if m == nil {
			t.Fatal("march finished prematurely")
		}
		p := (e.Position.X-m.Origin.X-m.Offset.X)*m.Forward.X + (e.Position.Y-m.Origin.Y-m.Offset.Y)*m.Forward.Y
		minProgress = math.Min(minProgress, p)
		maxProgress = math.Max(maxProgress, p)
	}
	if minProgress < 5 || maxProgress-minProgress > 1.6 {
		t.Fatalf("ranks separated in transit: %.2f..%.2f", minProgress, maxProgress)
	}
	for range 1800 {
		w.Update()
	}
	for i, id := range ids {
		e := w.Entities[id]
		if e.behavior.State() != Idle || e.Position.Distance(goal) > 3 {
			t.Fatalf("unit %d failed to arrive: %v %s", id, e.Position, e.behavior.State())
		}
		for _, other := range ids[i+1:] {
			if e.Position.Distance(w.Entities[other].Position) < .65 {
				t.Fatal("formation piled up")
			}
		}
	}
}

func TestFormationSurvivesInterruptionAndCheckpoint(t *testing.T) {
	w := trafficWorld()
	ids := []int{}
	for i := range 6 {
		ids = append(ids, w.spawn("militia", 1, Vec{10 + float64(i%3), 20 + float64(i/3)}).ID)
	}
	goal, next := Vec{35, 20}, Vec{35, 40}
	for _, q := range []Command{{Kind: "move", EntityIDs: ids, Position: &goal}, {Kind: "move", EntityIDs: ids, Position: &next, Queue: true}} {
		if err := w.Apply(1, q); err != nil {
			t.Fatal(err)
		}
	}
	for range 100 {
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
	if !reflect.DeepEqual(w.Entities[ids[1]].Order, restored.Entities[ids[1]].Order) {
		t.Fatal("lost formation intent")
	}
	for _, world := range []*World{w, restored} {
		if err := world.Apply(1, Command{Kind: "stop", EntityIDs: ids[:1]}); err != nil {
			t.Fatal(err)
		}
		world.hit(world.Entities[ids[1]], 0, 10000)
		for range 2400 {
			world.Update()
		}
		for _, id := range ids[2:] {
			if world.Entities[id].Position.Distance(next) > 2.5 {
				t.Fatalf("queued formation stuck after loss: %v", world.Entities[id].Position)
			}
		}
	}
	for _, id := range ids[2:] {
		if w.Entities[id].Position != restored.Entities[id].Position {
			t.Fatal("restore diverged")
		}
	}
}

func TestFormationUsesGateAndRejectsInvalidSelectionAtomically(t *testing.T) {
	w := trafficWorld()
	ids := []int{}
	for i := range 6 {
		ids = append(ids, w.spawn("militia", 1, Vec{14 + float64(i%3), 30 + float64(i/3)}).ID)
	}
	for y := range w.Height {
		if y == 31 {
			w.spawn("gate", 1, Vec{25.5, float64(y) + .5})
		} else {
			w.spawn("wall", 1, Vec{25.5, float64(y) + .5})
		}
	}
	goal := Vec{38, 32}
	if err := w.Apply(1, Command{Kind: "move", EntityIDs: append(append([]int{}, ids...), w.spawn("house", 1, Vec{12, 12}).ID), Position: &goal}); err == nil {
		t.Fatal("accepted immobile selection")
	}
	for _, id := range ids {
		if w.Entities[id].Order.March != nil || w.Entities[id].behavior.State() != Idle {
			t.Fatal("partial order")
		}
	}
	if err := w.Apply(1, Command{Kind: "move", EntityIDs: ids, Position: &goal}); err != nil {
		t.Fatal(err)
	}
	for range 2400 {
		w.Update()
		for _, id := range ids {
			if !w.freeFor(w.Entities[id], w.Entities[id].Position) {
				t.Fatal("crossed wall")
			}
		}
	}
	for _, id := range ids {
		if w.Entities[id].Position.Distance(goal) > 2.5 {
			t.Fatalf("gate passage stuck: %v", w.Entities[id].Position)
		}
	}
}

func TestFormationFindsDistinctSlotsBesideMapEdge(t *testing.T) {
	w := trafficWorld()
	ids := []int{}
	for i := 0; i < 12; i++ {
		ids = append(ids, w.spawn("militia", 1, Vec{12 + float64(i%4), 28 + float64(i/4)}).ID)
	}
	goal := Vec{1.2, 30}
	if err := w.Apply(1, Command{Kind: "move", EntityIDs: ids, Position: &goal}); err != nil {
		t.Fatal(err)
	}
	for i, id := range ids {
		for _, other := range ids[:i] {
			if w.Entities[id].Order.Position.Distance(*w.Entities[other].Order.Position) < .65 {
				t.Fatal("edge slots overlap")
			}
		}
	}
	stepWorld(w, 1600)
	for _, id := range ids {
		if w.Entities[id].behavior.State() != Idle {
			t.Fatal("edge formation did not finish", w.Entities[id].Position)
		}
	}
}

func TestFormationAssemblesScatteredRanksWithoutBlockingLateArrivals(t *testing.T) {
	w := trafficWorld()
	w.spawn("barracks", 1, Vec{22, 40})
	// A trained army around its barracks, with workers behind it and a scout
	// returning from a different direction. Positions reproduce a UI march.
	positions := []Vec{
		{21.654, 42.356},
		{19.824, 40.933},
		{22.236, 42.353},
		{22.500, 51.500},
		{24.300, 40.000},
		{24.197, 40.680},
		{23.898, 41.299},
		{23.430, 41.802},
		{22.833, 42.144},
		{20.839, 41.985},
		{20.304, 41.554},
		{19.723, 40.325},
		{19.729, 39.637},
		{19.937, 38.982},
		{20.330, 38.418},
		{20.872, 37.995},
		{21.515, 37.752},
		{22.201, 37.709},
		{22.869, 37.871},
		{23.460, 38.223},
		{23.920, 38.733},
		{24.208, 39.357},
		{24.800, 40.000},
		{24.311, 41.581},
		{23.015, 42.610},
		{19.228, 40.395},
		{19.235, 39.558},
		{19.489, 38.761},
	}
	ids := []int{}
	for i, position := range positions {
		kind := "militia"
		if i < 3 {
			kind = "villager"
		} else if i == 3 {
			kind = "scout"
		}
		ids = append(ids, w.spawn(kind, 1, position).ID)
	}
	goal := Vec{33.904, 39.281}
	if err := w.Apply(1, Command{Kind: "move", EntityIDs: ids, Position: &goal}); err != nil {
		t.Fatal(err)
	}
	stepWorld(w, 2400)
	for _, id := range ids {
		e := w.Entities[id]
		if e.behavior.State() != Idle || e.Position.Distance(goal) > 5 {
			t.Fatalf("late rank trapped at %v: %s", e.Position, e.behavior.State())
		}
	}
}
