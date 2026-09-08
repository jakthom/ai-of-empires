package game

import (
	"reflect"
	"testing"
)

func depletedTestFarm(w *World) *Entity {
	farm := w.spawn("farm", 1, Vec{22.5, 46.5})
	farm.Amount = 0
	mustFire(farm.life, ExhaustResource, &entityContext{World: w, Actor: farm})
	return farm
}

func TestExplicitFarmReseedPaysOnceAndSurvivesInterruptionAndSave(t *testing.T) {
	w := New(Config{Difficulty: "peaceful"})
	farm := depletedTestFarm(w)
	p := w.Players[1]
	p.Resources.Wood = 60
	p.Technologies["horse_collar"] = true
	worker := w.entities(1, "villager")[0]
	w.setOrder(worker, Order{Kind: "gather", Target: farm.ID}, false)
	command := Command{Kind: "reseed_farm", EntityIDs: []int{farm.ID}}
	if err := w.Apply(1, command); err != nil {
		t.Fatal(err)
	}
	if farm.life.State() != Foundation || farm.Amount != 250 || p.Resources.Wood != 0 || worker.Order.Kind != "build" || worker.Order.Target != farm.ID {
		t.Fatal("explicit reseed did not use the lifecycle and assigned farmer")
	}
	if err := w.Apply(1, command); err == nil || p.Resources.Wood != 0 {
		t.Fatal("duplicate reseed paid twice")
	}
	if err := w.Apply(1, Command{Kind: "stop", EntityIDs: []int{worker.ID}}); err != nil {
		t.Fatal(err)
	}
	stepWorld(w, 10)
	if farm.Progress != 0 {
		t.Fatal("stopped worker continued reseeding")
	}
	if err := w.Apply(1, Command{Kind: "interact", EntityIDs: []int{worker.ID}, TargetID: farm.ID}); err != nil {
		t.Fatal(err)
	}
	data, err := w.Checkpoint()
	if err != nil {
		t.Fatal(err)
	}
	r, err := Restore(data, w.JournalSince(0))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 1600 && (farm.life.State() != Active || farm.Amount == 250); i++ {
		w.Update()
		r.Update()
	}
	if farm.life.State() != Active || farm.Amount >= 250 || !reflect.DeepEqual(w.View(1), r.View(1)) {
		t.Fatal("saved reseeding did not resume farming")
	}
	count := 0
	for _, event := range w.journal.records {
		if event.EntityID == farm.ID && event.PreviousState == string(Exhausted) && event.State == string(Foundation) {
			count++
		}
	}
	if count != 1 || p.Resources.Wood != 0 {
		t.Fatal("reseed effects were not exactly once")
	}
}

func TestExplicitReseedRefusalsLeaveFarmAndStockpileUntouched(t *testing.T) {
	for _, scenario := range []string{"wood", "busy", "foreign", "active", "multiple", "not_farm"} {
		t.Run(scenario, func(t *testing.T) {
			w := New(Config{Difficulty: "peaceful"})
			farm := depletedTestFarm(w)
			w.Players[1].Resources.Wood = 60
			command := Command{Kind: "reseed_farm", EntityIDs: []int{farm.ID}}
			switch scenario {
			case "wood":
				w.Players[1].Resources.Wood = 59
			case "busy":
				for _, worker := range w.entities(1, "villager") {
					w.setOrder(worker, Order{Kind: "move", Position: &Vec{30, 45}}, false)
				}
			case "foreign":
				farm.Owner = 2
			case "active":
				farm = w.spawn("farm", 1, Vec{25.5, 46.5})
				command.EntityIDs = []int{farm.ID}
			case "multiple":
				command.EntityIDs = append(command.EntityIDs, w.entities(1, "villager")[0].ID)
			case "not_farm":
				command.EntityIDs = []int{w.entities(1, "town_center")[0].ID}
			}
			before, state, amount := w.Players[1].Resources, farm.life.State(), farm.Amount
			if err := w.Apply(1, command); err == nil {
				t.Fatal("invalid reseed accepted")
			}
			if before != w.Players[1].Resources || state != farm.life.State() || amount != farm.Amount {
				t.Fatal("refused reseed changed state")
			}
		})
	}
}
