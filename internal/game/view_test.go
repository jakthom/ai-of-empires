package game

import "testing"

func TestGatheringReadModelPreservesSimulationAndOffersCommands(t *testing.T) {
	w := New(Config{Difficulty: "peaceful"})
	e := w.entities(1, "villager")[0]
	source := w.spawn("berries", 0, Vec{e.Position.X + 1, e.Position.Y})
	source.Resource, source.Amount = "food", 10
	if err := w.Apply(1, Command{Kind: "interact", EntityIDs: []int{e.ID}, TargetID: source.ID}); err != nil {
		t.Fatal(err)
	}
	pulse(w, e, 2)
	if e.behavior.State() != Gathering || e.Cargo <= 0 {
		t.Fatal("fixture must be actively gathering with cargo")
	}
	cargo, remaining, rng, order := e.Cargo, source.Amount, w.rng, e.Order
	for range 3 {
		view := w.View(1) // Previously panicked in cargoCapacity via Permitted.
		found := false
		for _, entity := range view.Entities {
			if entity.ID != e.ID {
				continue
			}
			found = true
			if entity.State != string(Gathering) || entity.Cargo != cargo {
				t.Fatal("snapshot must report the authoritative gathering state")
			}
			for _, kind := range []string{"move", "attack_move", "stop"} {
				available := false
				for _, action := range entity.Actions {
					available = available || action.Kind == kind && action.Enabled
				}
				if !available {
					t.Fatalf("gathering worker is missing the %s command", kind)
				}
			}
		}
		if !found {
			t.Fatal("owned worker missing from snapshot")
		}
	}
	if e.Cargo != cargo || source.Amount != remaining || w.rng != rng || e.Order != order || e.behavior.State() != Gathering {
		t.Fatal("building the action menu changed the simulation")
	}
}

func TestReadModelRespectsCommandGuards(t *testing.T) {
	w := New(Config{Difficulty: "peaceful"})
	e := w.spawn("trebuchet", 1, Vec{20, 48})
	mustFire(e.siege, Deploy, &entityContext{World: w, Actor: e})
	for _, action := range w.entityView(e, 1).Actions {
		if action.Kind == "move" && action.Enabled {
			t.Fatal("a trebuchet cannot move while deploying")
		}
	}
}
