package game

import "testing"

func TestWorkerReachesDestinationWithinStartingTile(t *testing.T) {
	w := New(Config{Difficulty: "peaceful"})
	worker := w.entities(1, "villager")[0]
	worker.Position = Vec{33.01, 42.01}
	goal := Vec{33.9, 42.9}
	if err := w.Apply(1, Command{Kind: "move", EntityIDs: []int{worker.ID}, Position: &goal}); err != nil {
		t.Fatal(err)
	}
	stepWorld(w, 100)
	if worker.behavior.State() != Idle || worker.Position.Distance(goal) > .8 {
		t.Fatalf("worker stopped outside arrival range: state=%s position=%+v goal=%+v", worker.behavior.State(), worker.Position, goal)
	}
}

func TestWorkerRoutesAroundObstacleBetweenFreeTileCenters(t *testing.T) {
	w := New(Config{Difficulty: "peaceful"})
	worker := w.entities(1, "villager")[0]
	worker.Position = Vec{33.5, 42.5}
	w.resource("tree", Vec{34, 43})
	goal := Vec{36.5, 45.5}
	if err := w.Apply(1, Command{Kind: "move", EntityIDs: []int{worker.ID}, Position: &goal}); err != nil {
		t.Fatal(err)
	}
	for range 300 {
		w.Update()
		if !w.freeFor(worker, worker.Position) {
			t.Fatalf("worker crossed an obstacle: %+v", worker.Position)
		}
		if worker.behavior.State() == Idle {
			if worker.Position.Distance(goal) > .8 {
				t.Fatal("worker reported arrival outside destination range")
			}
			return
		}
	}
	t.Fatalf("worker became stuck between free tile centers: %+v", worker.Position)
}
