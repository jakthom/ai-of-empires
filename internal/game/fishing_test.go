package game

import (
	"math"
	"reflect"
	"testing"
)

func TestMaritimeWaterwaysHaveFiniteFishInEveryBiome(t *testing.T) {
	for _, typ := range []string{"rivers", "lakes", "coast", "islands", "mountain_lakes", "wetlands", "fjords", "archipelago"} {
		for _, biome := range worldCatalog().Biomes {
			for _, seed := range []int64{7, 4817} {
				w, err := NewWorld(Config{Settlements: 6, Seed: seed, World: WorldOptions{Type: typ, Biome: biome.ID, Resources: "scarce"}})
				if err != nil {
					t.Fatal(err)
				}
				populated := map[int]bool{}
				for _, fish := range w.entities(0, "fish") {
					if w.tile(fish.Position).Terrain != "water" || fish.Resource != "food" || fish.Amount != 280 {
						t.Fatalf("%s/%s: invalid fish %+v", typ, biome.ID, fish)
					}
					populated[w.region(fish.Position, true)] = true
				}
				for i, tile := range w.Tiles {
					if tile.Terrain == "water" && !populated[w.waterRegions[i]] {
						t.Fatalf("%s/%s seed%d has an empty waterway", typ, biome.ID, seed)
					}
				}
				for _, p := range w.Players {
					closest := math.Inf(1)
					for i, tile := range w.Tiles {
						if tile.Terrain == "water" {
							closest = math.Min(closest, p.Start.Distance(Vec{float64(i%w.Width) + .5, float64(i/w.Width) + .5}))
						}
					}
					n := 0
					for _, fish := range w.entities(0, "fish") {
						if fish.Position.Distance(p.Start) < closest+8 {
							n++
						}
					}
					if n < 3 {
						t.Fatalf("%s/%s seed%d: player%d has only %d nearby shoals; closest water %.2f", typ, biome.ID, seed, p.ID, n, closest)
					}
				}
			}
		}
	}
}

func TestFishingDepletesShoalsDeliversFoodAndResumes(t *testing.T) {
	w, err := NewWorld(Config{Settlements: 1, Difficulty: "peaceful", World: WorldOptions{Type: "islands", Reveal: "all"}})
	if err != nil {
		t.Fatal(err)
	}
	p := w.Players[1]
	fish := w.entities(0, "fish")[0]
	water, land, ok := w.knownShore(p, w.region(p.Start, false), w.region(fish.Position, true), p.Start)
	if !ok {
		t.Fatal("no fishing shore")
	}
	dock := w.spawn("dock", 1, land)
	ship := w.spawn("fishing_ship", 1, water)
	fish.Amount = .3
	before := p.Resources.Food
	if err := w.Apply(1, Command{Kind: "gather", EntityIDs: []int{w.entities(1, "villager")[0].ID}, TargetID: fish.ID}); err == nil {
		t.Fatal("land worker accepted offshore fishing")
	}
	if err := w.Apply(1, Command{Kind: "gather", EntityIDs: []int{ship.ID}, TargetID: fish.ID}); err != nil {
		t.Fatal(err)
	}
	fished := false
	for i := 0; i < 2000 && w.Entities[fish.ID] != nil; i++ {
		w.Update()
		fished = fished || w.activity(ship) == "Fishing"
	}
	if !fished || w.Entities[fish.ID] != nil || math.Abs(ship.Cargo-.3) > 1e-8 || p.Resources.Food != before {
		t.Fatalf("fish was not gathered as undelivered cargo: %+v", ship)
	}
	data, err := w.Checkpoint()
	if err != nil {
		t.Fatal(err)
	}
	r, err := Restore(data, w.JournalSince(0))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2000 && p.Resources.Food == before; i++ {
		w.Update()
		r.Update()
	}
	if math.Abs(p.Resources.Food-before-.3) > 1e-8 || !reflect.DeepEqual(w.View(1), r.View(1)) {
		t.Fatal("saved fishing trip lost or duplicated food")
	}
	found := false
	for _, event := range w.journal.records {
		if event.EntityID == ship.ID && event.Kind == "delivery" && event.TargetID == dock.ID && event.Resource == "food" {
			found = true
		}
	}
	if !found {
		t.Fatal("fishing delivery missing from immutable history")
	}
}
