package game

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestOriginalCheckpointKeepsItsTerrainAndHasNoTreaty(t *testing.T) {
	w := New(Config{Settlements: 3, Seed: 523})
	data, err := w.Checkpoint()
	if err != nil {
		t.Fatal(err)
	}
	var old map[string]any
	if err = json.Unmarshal(data, &old); err != nil {
		t.Fatal(err)
	}
	old["Version"] = 3
	delete(old, "Treaty")
	delete(old, "Voyages")
	oldWorld := old["World"].(map[string]any)
	delete(oldWorld, "Generation")
	delete(oldWorld["Config"].(map[string]any), "world")
	data, err = json.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	r, err := Restore(data, w.JournalSince(0))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(w.Tiles, r.Tiles) || !reflect.DeepEqual(w.View(1), r.View(1)) || r.treatyInForce() {
		t.Fatal("old save changed")
	}
}

func TestWorldSizeAndSeparationCombinations(t *testing.T) {
	for _, preset := range worldCatalog().Types {
		for _, size := range worldCatalog().Sizes {
			for _, distance := range worldCatalog().Separations {
				w, err := NewWorld(Config{Seed: 93, Settlements: 6, World: WorldOptions{Type: preset.ID, Size: size.ID, Separation: distance.ID}})
				if err != nil {
					t.Fatalf("%s/%s/%s: %v", preset.ID, size.ID, distance.ID, err)
				}
				if w.Width != size.Tiles || w.Height != size.Tiles {
					t.Fatal("size depended on population")
				}
			}
		}
	}
}

func TestSharedWaterRemainsNavigable(t *testing.T) {
	for _, typ := range []string{"rivers", "coast", "islands"} {
		for _, seed := range []int64{1, 4817, 91723} {
			w, err := NewWorld(Config{Settlements: 6, Seed: seed, World: WorldOptions{Type: typ}})
			if err != nil {
				t.Fatal(err)
			}
			regions := map[int]bool{}
			for i, region := range w.waterRegions {
				if w.Tiles[i].Terrain == "water" {
					regions[region] = true
				}
			}
			if len(regions) != 1 {
				t.Fatalf("%s seed%d has %d disconnected waterways", typ, seed, len(regions))
			}
		}
	}
}

func TestWorldPresetsCreateFairReachableHomes(t *testing.T) {
	for _, preset := range worldCatalog().Types {
		for _, count := range []int{1, 2, 6} {
			for _, seed := range []int64{1, 4817, 91723} {
				w, err := NewWorld(Config{Settlements: count, Seed: seed, World: WorldOptions{Type: preset.ID}})
				if err != nil {
					t.Fatalf("%s/%d/%d: %v", preset.ID, count, seed, err)
				}
				for id := 1; id <= count; id++ {
					p := w.Players[id]
					if n, cap := w.population(id); n != 4 || cap != 5 {
						t.Fatalf("%s: unequal population %d/%d", preset.ID, n, cap)
					}
					found := map[string]int{}
					for _, e := range w.Entities {
						if e.Owner == 0 && e.Position.Distance(p.Start) < 12 {
							if e.Type != "fish" && !w.sameRegion(p.Start, e.Position, false) {
								t.Fatalf("%s: unreachable starter %s", preset.ID, e.Type)
							}
							found[e.Type]++
						}
					}
					for typ, want := range map[string]int{"berries": 5, "sheep": 4, "gold": 6, "stone": 4, "tree": 24} {
						if found[typ] != want {
							t.Fatalf("%s/%d/%d player%d %s=%d want%d", preset.ID, count, seed, id, typ, found[typ], want)
						}
					}
				}
			}
		}
	}
}

func TestWorldOptionsDeterminismAbundanceRevealAndRestore(t *testing.T) {
	cfg := Config{Seed: 4829, Settlements: 3, World: WorldOptions{Type: "islands", Size: "medium", Reveal: "explored", TreatyMinutes: 10}}
	a, err := NewWorld(cfg)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := NewWorld(cfg)
	if !reflect.DeepEqual(a.View(1), b.View(1)) {
		t.Fatal("same options and seed differ")
	}
	cfg.World.Biome = "desert"
	cfg.World.Resources = "abundant"
	cfg.World.Reveal = "all"
	c, _ := NewWorld(cfg)
	if !reflect.DeepEqual(a.Tiles, c.Tiles) {
		t.Fatal("biome/abundance/reveal changed topology")
	}
	for id, e := range a.Entities {
		if e.Resource != "" && e.Owner == 0 && c.Entities[id].Amount != e.Amount*1.75 {
			t.Fatal("abundance did not scale natural deposit", id)
		}
	}
	if a.Players[1].Resources != c.Players[1].Resources {
		t.Fatal("abundance changed stockpiles")
	}
	for _, tile := range a.View(1).Map.Tiles {
		if tile.Terrain == "unknown" {
			t.Fatal("terrain not revealed")
		}
	}
	if len(a.View(1).Entities) >= len(c.View(1).Entities) {
		t.Fatal("explored terrain leaked entities")
	}
	for _, visible := range c.Players[1].Visible {
		if !visible {
			t.Fatal("full reveal missing tiles")
		}
	}
	data, err := a.Checkpoint()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := Restore(data, a.JournalSince(0))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a.View(1), restored.View(1)) {
		t.Fatal("restored world changed")
	}
	cfg.Seed++
	different, _ := NewWorld(cfg)
	if reflect.DeepEqual(c.Tiles, different.Tiles) {
		t.Fatal("seed did not affect terrain")
	}
}

func TestTreatyBlocksAllAggressionAndExpiresOnce(t *testing.T) {
	w, err := NewWorld(Config{World: WorldOptions{Reveal: "all", TreatyMinutes: 5}})
	if err != nil {
		t.Fatal(err)
	}
	a := w.entities(1, "scout")[0]
	b := w.entities(2, "villager")[0]
	if err := w.Apply(1, Command{Kind: "attack", EntityIDs: []int{a.ID}, TargetID: b.ID}); err == nil {
		t.Fatal("attack accepted during treaty")
	}
	a.Stance = "aggressive"
	a.Position = b.Position
	if w.acquire(a) != nil {
		t.Fatal("automatic attack during treaty")
	}
	hp := b.HP
	w.hitFrom(b, 1, a.ID, 10)
	if b.HP != hp || w.relation(1, 2) != atPeace {
		t.Fatal("projectile damaged target during treaty")
	}
	m := w.spawn("monk", 1, a.Position)
	for _, action := range w.entityView(m, 1).Actions {
		if action.Kind == "convert" && (action.Enabled || action.Reason == "") {
			t.Fatal("conversion advertised during treaty")
		}
	}
	if err := w.Apply(1, Command{Kind: "convert", EntityIDs: []int{m.ID}, TargetID: b.ID}); err == nil {
		t.Fatal("conversion accepted")
	}
	w.Time = 300 - Step/2
	w.Update()
	if w.treatyInForce() || w.TreatyRemaining() != 0 {
		t.Fatal("treaty did not end")
	}
	events := w.NextEvent
	mustFire(w.peacePeriod, treatyPulse, w)
	if events != w.NextEvent {
		t.Fatal("expiry repeated")
	}
	if w.relation(1, 2) != atPeace {
		t.Fatal("expiry declared war")
	}
	if err := w.Apply(1, Command{Kind: "attack", EntityIDs: []int{a.ID}, TargetID: b.ID}); err != nil {
		t.Fatal(err)
	}
}

func TestInvalidWorldSettingsAreRejected(t *testing.T) {
	for _, options := range []WorldOptions{{Type: "ocean"}, {Biome: "space"}, {Size: "huge"}, {Resources: "infinite"}, {Separation: "overlap"}, {Reveal: "cheat"}, {TreatyMinutes: -1}, {TreatyMinutes: 6}} {
		if _, err := NewWorld(Config{World: options}); err == nil {
			t.Fatalf("accepted %+v", options)
		}
	}
}
