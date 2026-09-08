package game

import (
	"math"
	"testing"
)

func trafficWorld() *World {
	w := New(Config{Settlements: 1, Difficulty: "peaceful", Mode: "sandbox"})
	w.Entities = map[int]*Entity{}
	w.IDs = nil
	for i := range w.Tiles {
		w.Tiles[i] = Tile{Terrain: "grass"}
	}
	return w
}
func TestTrainedUnitsLeaveBesideTheirOwnProducerWithoutStacking(t *testing.T) {
	w := trafficWorld()
	tc := w.spawn("town_center", 1, Vec{20, 20})
	other := w.spawn("town_center", 1, Vec{40, 40})
	for _, b := range []*Entity{tc, other} {
		for range 3 {
			if err := w.Apply(1, Command{Kind: "train", EntityIDs: []int{b.ID}, Product: "villager"}); err != nil {
				t.Fatal(err)
			}
		}
	}
	for range 1800 {
		w.Update()
	}
	units := w.entities(1, "villager")
	if len(units) != 6 {
		t.Fatalf("trained %d villagers", len(units))
	}
	for i, e := range units {
		if math.Min(e.Position.Distance(tc.Position), e.Position.Distance(other.Position)) > 4 {
			t.Fatal("unit appeared away from producer")
		}
		for _, o := range units[i+1:] {
			if e.Position.Distance(o.Position) < definitions[e.Type].Radius+definitions[o.Type].Radius {
				t.Fatal("produced units overlap")
			}
		}
	}
}
func TestTrafficPassesStationaryWorkersAndOpposingUnits(t *testing.T) {
	for _, opposing := range []bool{false, true} {
		w := trafficWorld()
		// Walls leave a three-tile corridor and open areas at either end.
		for x := 12.; x <= 28; x += 1 {
			w.spawn("wall", 1, Vec{x, 18})
			w.spawn("wall", 1, Vec{x, 22})
		}
		a := w.spawn("villager", 1, Vec{10, 20})
		b := w.spawn("villager", 1, Vec{20, 20})
		goalA, goalB := Vec{30, 20}, Vec{9, 20}
		if opposing {
			b.Position = Vec{29, 20}
			if err := w.Apply(1, Command{Kind: "move", EntityIDs: []int{b.ID}, Position: &goalB}); err != nil {
				t.Fatal(err)
			}
		}
		if err := w.Apply(1, Command{Kind: "move", EntityIDs: []int{a.ID}, Position: &goalA}); err != nil {
			t.Fatal(err)
		}
		for range 1200 {
			w.Update()
			if !w.freeFor(a, a.Position) || !w.freeFor(b, b.Position) {
				t.Fatal("traffic crossed a wall")
			}
		}
		if a.Position.Distance(goalA) > 1 || opposing && b.Position.Distance(goalB) > 1 {
			t.Fatalf("traffic stuck (opposing=%v): %v and %v", opposing, a.Position, b.Position)
		}
	}
}

func TestOpposingTrafficMakesRoomInASingleTilePassage(t *testing.T) {
	w := trafficWorld()
	for y := range w.Height {
		for x := range w.Width {
			if x > 12 && x < 28 && y != 20 {
				w.Tiles[y*w.Width+x].Terrain = "cliff"
			}
		}
	}
	a := w.spawn("villager", 1, Vec{10.5, 20.5})
	b := w.spawn("villager", 1, Vec{30.5, 20.5})
	goalA, goalB := b.Position, a.Position
	if err := w.Apply(1, Command{Kind: "move", EntityIDs: []int{a.ID}, Position: &goalA}); err != nil {
		t.Fatal(err)
	}
	if err := w.Apply(1, Command{Kind: "move", EntityIDs: []int{b.ID}, Position: &goalB}); err != nil {
		t.Fatal(err)
	}
	for range 1600 {
		w.Update()
		if !w.land(a.Position) || !w.land(b.Position) {
			t.Fatal("traffic crossed impassable terrain")
		}
	}
	if a.Position.Distance(goalA) > .8 || b.Position.Distance(goalB) > .8 {
		t.Fatalf("opposing traffic deadlocked: %v and %v", a.Position, b.Position)
	}
}
func TestUnreachablePathWaitsBeforeRetryAndNewOrdersRetryImmediately(t *testing.T) {
	w := New(Config{Difficulty: "peaceful"})
	e := w.entities(1, "villager")[0]
	goal := Vec{14, 12}
	w.move(e, goal, .8, Step)
	timer := e.Repath
	w.move(e, goal, .8, Step)
	if e.Repath != timer-Step {
		t.Fatal("unreachable path was searched again before its retry timer")
	}
	next := Vec{20, 47}
	if err := w.Apply(1, Command{Kind: "move", EntityIDs: []int{e.ID}, Position: &next}); err != nil {
		t.Fatal(err)
	}
	before := e.Position
	w.Update()
	if e.Position == before {
		t.Fatal("new movement order inherited failed-path delay")
	}
}
