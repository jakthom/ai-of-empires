package game

import (
	"math"
	"testing"
)

func TestBiomeScarcityCreatesSurplusesWithoutChangingStarts(t *testing.T) {
	worlds := map[string]*World{}
	totals := map[string]Resources{}
	for _, biome := range []string{"desert", "tropical", "alpine", "savanna"} {
		w, err := NewWorld(Config{Seed: 4817, Settlements: 2, World: WorldOptions{Type: "plains", Biome: biome, Reveal: "all"}})
		if err != nil {
			t.Fatal(err)
		}
		worlds[biome] = w
		total := Resources{}
		for _, e := range w.Entities {
			nearHome := false
			for _, p := range w.Players {
				nearHome = nearHome || p.Start.Distance(e.Position) < 15
			}
			if !nearHome && e.Resource != "" && e.Type != "fish" {
				total.Deposit(e.Resource, e.Amount)
			}
		}
		totals[biome] = total
	}
	if totals["tropical"].Wood < totals["desert"].Wood*5 || totals["desert"].Gold < totals["tropical"].Gold*3 || totals["alpine"].Stone < totals["tropical"].Stone*5 || totals["savanna"].Food < totals["alpine"].Food*4 {
		t.Fatal("regions lack meaningful resource strengths", totals)
	}
	a, b := worlds["desert"], worlds["tropical"]
	for id, p := range a.Players {
		if p.Resources != b.Players[id].Resources {
			t.Fatal("biome changed starting stockpile")
		}
		for eid, e := range a.Entities {
			if e.Resource != "" && p.Start.Distance(e.Position) < 15 && (b.Entities[eid] == nil || e.Amount != b.Entities[eid].Amount) {
				t.Fatal("biome changed viable home patch", eid)
			}
		}
	}
	for i, tile := range a.Tiles {
		if tile.Terrain != b.Tiles[i].Terrain || tile.Elevation != b.Tiles[i].Elevation {
			t.Fatal("resource biome changed topology")
		}
	}
}

func TestMixedBiomeAbundanceAndSaveRetainDeposits(t *testing.T) {
	cfg := Config{Seed: 11, Settlements: 2, World: WorldOptions{Type: "plains", Biome: "mixed", Reveal: "all"}}
	a, err := NewWorld(cfg)
	if err != nil {
		t.Fatal(err)
	}
	cfg.World.Resources = "abundant"
	b, err := NewWorld(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for id, e := range a.Entities {
		if e.Resource != "" && math.Abs(b.Entities[id].Amount-e.Amount*1.75) > 1e-8 {
			t.Fatal("global abundance lost regional scarcity")
		}
	}
	data, _ := a.Checkpoint()
	restored, err := Restore(data, a.JournalSince(0))
	if err != nil {
		t.Fatal(err)
	}
	for id, e := range a.Entities {
		if restored.Entities[id].Amount != e.Amount {
			t.Fatal("restore regenerated a deposit")
		}
	}
}
