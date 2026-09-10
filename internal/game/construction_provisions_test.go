package game

import (
	"math"
	"reflect"
	"testing"
)

func TestFarmGridSharesEdgesAndReplacementBuilderCompletesBatch(t *testing.T) {
	w := barrierWorld()
	w.spawn("villager", 2, Vec{60, 60})
	a := w.spawn("villager", 1, Vec{10, 19})
	b := w.spawn("villager", 1, Vec{10, 20})
	for i := 0; i < 3; i++ {
		point := Vec{12 + float64(i)*2, 22}
		if err := w.Apply(1, Command{Kind: "build", Product: "farm", Position: &point, EntityIDs: []int{a.ID}, Queue: i > 0}); err != nil {
			t.Fatal(err)
		}
	}
	farms := w.entities(1, "farm")
	for i, f := range farms {
		if f.Position != (Vec{12 + float64(i)*2, 22}) || f.BuildGroup != farms[0].BuildGroup {
			t.Fatal("grid or batch mismatch", f.Position)
		}
	}
	if err := w.Apply(1, Command{Kind: "stop", EntityIDs: []int{a.ID}}); err != nil {
		t.Fatal(err)
	}
	if err := w.Apply(1, Command{Kind: "interact", EntityIDs: []int{b.ID}, TargetID: farms[0].ID}); err != nil {
		t.Fatal(err)
	}
	for range 1800 {
		w.Update()
	}
	for _, f := range farms {
		if f.life.State() != Active {
			t.Fatalf("replacement worker abandoned batch: %d %.3f %s", f.ID, f.Progress, b.behavior.State())
		}
	}
	if a.behavior.State() != Idle {
		t.Fatal("stopped builder was silently reassigned")
	}
	if w.Players[1].Economy.Consumption["construction"].Wood != 180 {
		t.Fatal("continuation paid twice")
	}
}

func TestBridgeConstructionCrossingCollapseAndRestore(t *testing.T) {
	w := barrierWorld()
	w.spawn("villager", 2, Vec{60, 60})
	for y := 0; y < w.Height; y++ {
		for x := 20; x <= 23; x++ {
			w.Tiles[y*w.Width+x] = Tile{Terrain: "water", Elevation: -.2}
		}
	}
	w.rebuildRegions()
	a := w.spawn("villager", 1, Vec{18.5, 20.5})
	start, end := Vec{19.5, 20.5}, Vec{24.5, 20.5}
	if w.sameRegion(start, end, false) {
		t.Fatal("river was already crossable")
	}
	before := w.Players[1].Resources
	if err := w.Apply(1, Command{Kind: "build", Product: "bridge", Position: &start, EndPosition: &end, EntityIDs: []int{a.ID}}); err != nil {
		t.Fatal(err)
	}
	if w.Players[1].Resources.Wood != before.Wood-80 || w.Players[1].Resources.Stone != before.Stone-20 {
		t.Fatal("wrong bridge price")
	}
	for range 1600 {
		w.Update()
	}
	for _, bridge := range w.entities(1, "bridge") {
		if bridge.life.State() != Active {
			t.Fatalf("unfinished span: %v %.3f; worker %v %s", bridge.Position, bridge.Progress, a.Position, a.behavior.State())
		}
	}
	if !w.sameRegion(start, end, false) {
		t.Fatal("completed bridge did not connect riverbanks")
	}
	goal := Vec{27.5, 20.5}
	if err := w.Apply(1, Command{Kind: "move", EntityIDs: []int{a.ID}, Position: &goal}); err != nil {
		t.Fatal(err)
	}
	for range 400 {
		w.Update()
	}
	if a.Position.X < 24 {
		t.Fatal("worker did not cross bridge", a.Position)
	}
	data, _ := w.Checkpoint()
	restored, err := Restore(data, w.JournalSince(0))
	if err != nil {
		t.Fatal(err)
	}
	if !restored.sameRegion(start, end, false) {
		t.Fatal("restore lost crossing")
	}
	bridge := w.entities(1, "bridge")[1]
	soldier := w.spawn("militia", 1, bridge.Position)
	ship := w.spawn("galley", 2, bridge.Position)
	mustFire(bridge.life, DamageEntity, &entityContext{World: w, Actor: bridge, Amount: bridge.HP, SourceOwner: 2})
	if w.Entities[soldier.ID] != nil || w.Entities[ship.ID] == nil || w.sameRegion(start, end, false) {
		t.Fatal("collapse did not remove crossing, or harmed a ship")
	}
	if err := fire(bridge.life, DamageEntity, &entityContext{World: w, Actor: bridge, Amount: 400, SourceOwner: 2}); err == nil {
		t.Fatal("destroyed bridge accepted repeated destruction")
	}
}

func TestPopulationConsumesFoodAndFamineRecoversWithoutReplay(t *testing.T) {
	w := barrierWorld()
	w.spawn("villager", 2, Vec{60, 60})
	w.spawn("villager", 1, Vec{10, 10})
	w.spawn("militia", 1, Vec{12, 10})
	p := w.Players[1]
	p.Resources.Food = 4
	for range 1200 {
		w.Update()
	}
	if math.Abs(p.Resources.Food) > 1e-7 || math.Abs(p.Economy.Consumption["food_upkeep"].Food-4) > 1e-7 {
		t.Fatal("food upkeep not proportional to population and game time", p.Resources.Food)
	}
	if view := p.Production.view(); math.Abs(view.ConsumptionRates.Food-4) > 1e-7 || view.Rates.Food != 0 {
		t.Fatal("food upkeep missing from consumption chart")
	}
	for range 1250 {
		w.Update()
	}
	if p.provisions.State() != foodFamine || p.workMultiplier() != .5 {
		t.Fatal("prolonged hunger had no effect", p.provisions.State())
	}
	data, _ := w.Checkpoint()
	restored, err := Restore(data, w.JournalSince(0))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p.Economy, restored.Players[1].Economy) || restored.Players[1].provisions.State() != foodFamine {
		t.Fatal("restore replayed consumption or lost famine")
	}
	p.Resources.Food = 10
	w.Update()
	if p.provisions.State() != foodFed || p.workMultiplier() != 1 {
		t.Fatal("food delivery did not end famine")
	}
	before := p.Resources.Food
	_ = w.SetPaused(true)
	for range 100 {
		w.Update()
	}
	if p.Resources.Food != before {
		t.Fatal("paused population consumed food")
	}
	view := p.Production.view()
	if view.Rates.Food != 0 {
		t.Fatal("consumption was misreported as production")
	}
}
