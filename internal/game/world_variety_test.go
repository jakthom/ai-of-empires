package game

import (
	"reflect"
	"testing"
)

func TestMixedGiantWorldIsDiverseFairAndCheckpointed(t *testing.T) {
	cfg := Config{Seed: 4817, Settlements: 6, Difficulty: "peaceful", World: WorldOptions{Type: "mountain_lakes", Size: "giant", Biome: "mixed", Reveal: "all"}}
	w, err := NewWorld(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if w.Width != 288 || w.Height != 288 {
		t.Fatal("giant size missing")
	}
	biomes := map[string]int{}
	terrains := map[string]int{}
	for _, tile := range w.Tiles {
		biomes[tile.Biome]++
		terrains[tile.Terrain]++
	}
	if len(biomes) != 6 || terrains["water"] == 0 || terrains["cliff"] == 0 {
		t.Fatalf("missing geographic variety: %v %v", biomes, terrains)
	}
	for id := 2; id <= 6; id++ {
		if w.Players[id].Resources != w.Players[1].Resources {
			t.Fatal("region changed starting supplies")
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
	if !reflect.DeepEqual(w.Tiles, restored.Tiles) || !reflect.DeepEqual(w.View(1), restored.View(1)) {
		t.Fatal("regional terrain changed after restore")
	}
	other, err := NewWorld(cfg)
	if err != nil || !reflect.DeepEqual(w.Tiles, other.Tiles) {
		t.Fatal("regional generation was not deterministic", err)
	}
}

func TestRegionalBiomesDoNotRevealUnknownTerrain(t *testing.T) {
	w, err := NewWorld(Config{Settlements: 2, World: WorldOptions{Biome: "mixed"}})
	if err != nil {
		t.Fatal(err)
	}
	view := w.View(1)
	unknown := 0
	for i, tile := range view.Map.Tiles {
		if view.Map.Fog[i] == 0 {
			unknown++
			if tile.Biome != "" {
				t.Fatal("unknown region leaked its scenery")
			}
		}
	}
	if unknown == 0 {
		t.Fatal("fixture did not contain fog")
	}
}

func TestBuildingAppearanceUsesObservedOwnerAge(t *testing.T) {
	w, err := NewWorld(Config{Settlements: 2, Difficulty: "peaceful"})
	if err != nil {
		t.Fatal(err)
	}
	building := w.entities(2, "town_center")[0]
	for age := 0; age < 4; age++ {
		w.Players[2].Age = age
		if v := w.entityView(building, 1); v.AppearanceAge != age {
			t.Fatal("observed building used viewer age")
		}
	}
	w.Players[2].Age = 1
	remembered := w.entityView(building, 1)
	remembered.Visible = false
	w.Players[1].Memory[building.ID] = remembered
	w.Players[2].Age = 3
	for _, entity := range w.View(1).Entities {
		if entity.ID == building.ID && entity.AppearanceAge != 1 {
			t.Fatal("hidden age advance changed remembered building")
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
	if restored.Players[1].Memory[building.ID].AppearanceAge != 1 {
		t.Fatal("checkpoint lost observed building age")
	}
}
