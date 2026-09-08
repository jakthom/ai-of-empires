package game

import (
	"fmt"
	"testing"
)

func TestOpeningWorkerGathersAndDepositsResources(t *testing.T) {
	opening := New(Config{Difficulty: "peaceful", Seed: 4817})
	for _, source := range opening.entities(0, "") {
		if (source.Type != "stone" && source.Type != "gold" && source.Type != "tree") || !opening.visibleEntity(1, source) {
			continue
		}
		t.Run(fmt.Sprintf("%s_%d", source.Type, source.ID), func(t *testing.T) {
			w := New(Config{Difficulty: "peaceful", Seed: 4817})
			worker := w.entities(1, "villager")[0]
			target := w.Entities[source.ID]
			before := w.Players[1].Resources
			if err := w.Apply(1, Command{Kind: "interact", EntityIDs: []int{worker.ID}, TargetID: target.ID}); err != nil {
				t.Fatal(err)
			}
			for range 1600 {
				w.Update()
				if w.Players[1].Resources != before {
					if target.Amount >= source.Amount {
						t.Fatal("deposit must come from the commanded resource")
					}
					return
				}
			}
			t.Fatalf("worker never deposited %s: state=%s position=%+v target=%+v cargo=%f path=%+v", target.Resource, worker.behavior.State(), worker.Position, target.Position, worker.Cargo, worker.Path)
		})
	}
}

func TestWorkerResumesUnfinishedFarmWithInteract(t *testing.T) {
	w := New(Config{Difficulty: "peaceful", Seed: 4817})
	worker := w.entities(1, "villager")[0]
	if err := w.Apply(1, Command{Kind: "build", EntityIDs: []int{worker.ID}, Product: "farm", Position: &Vec{22.5, 46.5}}); err != nil {
		t.Fatal(err)
	}
	farm := w.entities(1, "farm")[0]
	if err := w.Apply(1, Command{Kind: "stop", EntityIDs: []int{worker.ID}}); err != nil {
		t.Fatal(err)
	}
	if err := w.Apply(1, Command{Kind: "interact", EntityIDs: []int{worker.ID}, TargetID: farm.ID}); err != nil {
		t.Fatalf("clicking the unfinished farm should resume construction: %v", err)
	}
	for range 1200 {
		w.Update()
		if farm.Progress == 1 && farm.Amount < 175 {
			return
		}
	}
	t.Fatalf("farm never completed and produced food: state=%s position=%+v progress=%f amount=%f", worker.behavior.State(), worker.Position, farm.Progress, farm.Amount)
}

func TestWorkerCanReseedAnAbandonedFarm(t *testing.T) {
	w := New(Config{Difficulty: "peaceful"})
	workers := w.entities(1, "villager")
	farm := w.spawn("farm", 1, Vec{22.5, 46.5})
	farm.Amount = 0
	mustFire(farm.life, ExhaustResource, &entityContext{World: w, Actor: farm})
	w.Players[1].Resources.Wood = 60
	command := Command{Kind: "interact", EntityIDs: []int{workers[0].ID, workers[1].ID}, TargetID: farm.ID}
	if err := w.Apply(1, command); err != nil {
		t.Fatalf("empty owned farm should accept a work order: %v", err)
	}
	if farm.life.State() != Exhausted || w.Players[1].Resources.Wood != 60 {
		t.Fatal("command routing bypassed the reseeding lifecycle")
	}
	for range 1200 {
		w.Update()
		if farm.life.State() == Active && farm.Amount < 175 {
			if w.Players[1].Resources.Wood != 0 {
				t.Fatal("reseed cost must be paid exactly once for a group order")
			}
			page, _ := w.Log(1, LogQuery{EntityID: farm.ID})
			reseeds := 0
			for _, event := range page.Events {
				if event.Lifecycle == "life" && event.PreviousState == string(Exhausted) && event.State == string(Foundation) {
					reseeds++
				}
			}
			if reseeds != 1 {
				t.Fatalf("want one immutable reseed event; got %d", reseeds)
			}
			return
		}
	}
	t.Fatal("villager failed to reseed, rebuild and resume farming")
}

func TestManualReseedRequiresWoodAndRejectsOtherTargets(t *testing.T) {
	w := New(Config{Difficulty: "peaceful"})
	worker := w.entities(1, "villager")[0]
	farm := w.spawn("farm", 1, Vec{22.5, 46.5})
	farm.Amount = 0
	mustFire(farm.life, ExhaustResource, &entityContext{World: w, Actor: farm})
	w.Players[1].Resources.Wood = 59
	command := Command{Kind: "interact", EntityIDs: []int{worker.ID}, TargetID: farm.ID}
	err := w.Apply(1, command)
	if err == nil || err.Error() != "Reseeding a farm requires 60 wood." {
		t.Fatalf("missing actionable wood requirement: %v", err)
	}
	if farm.life.State() != Exhausted || worker.behavior.State() != Idle || w.Players[1].Resources.Wood != 59 {
		t.Fatal("refused reseed changed the game")
	}
	w.Players[1].Resources.Wood = 60
	if err := w.Apply(1, command); err != nil {
		t.Fatalf("reseed should be retryable after gathering wood: %v", err)
	}
	for _, typ := range []string{"tree", "stone"} {
		source := w.spawn(typ, 0, Vec{21, 46})
		source.Resource, source.Amount = "wood", 0
		if err := w.Apply(1, Command{Kind: "gather", EntityIDs: []int{worker.ID}, TargetID: source.ID}); err == nil {
			t.Fatal("empty natural resources cannot be reseeded")
		}
	}
	foreign := w.spawn("farm", 2, Vec{23, 46})
	foreign.Amount = 0
	mustFire(foreign.life, ExhaustResource, &entityContext{World: w, Actor: foreign})
	if err := w.Apply(1, Command{Kind: "gather", EntityIDs: []int{worker.ID}, TargetID: foreign.ID}); err == nil {
		t.Fatal("opponent's farm cannot be reseeded")
	}
}
