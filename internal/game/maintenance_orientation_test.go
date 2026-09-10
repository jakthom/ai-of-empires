package game

import (
	"context"
	"testing"
)

func TestGateOrientationPreviewRotationAndRestore(t *testing.T) {
	w := barrierWorld()
	pos := Vec{20.5, 20.5}
	worker := w.spawn("villager", 1, Vec{17.5, 18.5})
	w.spawn("wall", 1, Vec{20.5, 19.5})
	w.spawn("wall", 1, Vec{20.5, 21.5})
	_, _, axis, err := w.PlanOrientedBuilding(1, "gate", pos, nil, "auto")
	if err != nil || axis != "north_south" {
		t.Fatal("preview did not align", axis, err)
	}
	before := w.Players[1].Resources
	if err = w.Apply(1, Command{Kind: "build", Product: "gate", EntityIDs: []int{worker.ID}, Position: &pos, Orientation: "east_west"}); err == nil || w.Players[1].Resources != before {
		t.Fatal("mismatched alignment charged/placed")
	}
	if err = w.Apply(1, Command{Kind: "build", Product: "gate", EntityIDs: []int{worker.ID}, Position: &pos, Orientation: "auto"}); err != nil {
		t.Fatal(err)
	}
	gate := w.barrierAt(pos)
	if v := w.entityView(gate, 1); v.Orientation != axis {
		t.Fatal("gate differs from preview")
	}
	c := &entityContext{World: w, Actor: gate, Orientation: "east_west"}
	if err = canRotateGate(context.Background(), c); err == nil || gate.Orientation != "auto" {
		t.Fatal("rotation guard mutated gate or allowed broken wall")
	}
	if err = w.Apply(1, Command{Kind: "rotate_gate", EntityIDs: []int{gate.ID}}); err == nil {
		t.Fatal("rotated away from wall")
	}
	standalone := w.spawn("gate", 1, Vec{35.5, 35.5})
	if err = w.Apply(1, Command{Kind: "rotate_gate", EntityIDs: []int{standalone.ID}}); err != nil {
		t.Fatal(err)
	}
	if w.entityView(standalone, 1).Orientation != "north_south" {
		t.Fatal("standalone rotation unavailable")
	}
	if err = w.Placement(1, "wall", Vec{36.5, 35.5}); err == nil {
		t.Fatal("perpendicular wall ignored manual rotation")
	}
	data, _ := w.Checkpoint()
	restored, err := Restore(data, w.JournalSince(0))
	if err != nil {
		t.Fatal(err)
	}
	if restored.entityView(restored.Entities[standalone.ID], 1).Orientation != "north_south" {
		t.Fatal("rotation lost in save")
	}
	w.hit(standalone, 0, 10000)
	if err = fire(standalone.life, RotateGate, &entityContext{World: w, Actor: standalone, Orientation: "east_west"}); err == nil {
		t.Fatal("destroyed gate rotated")
	}
}

func TestBuildingMaintenanceAssignsOnlyIdleWorkersAndChargesForActualRepair(t *testing.T) {
	w := trafficWorld()
	w.Players[1].Resources = Resources{Wood: 1000, Food: 1000, Gold: 1000, Stone: 1000}
	h := w.spawn("house", 1, Vec{20, 20})
	worker := w.spawn("villager", 1, Vec{22, 20})
	busy := w.spawn("villager", 1, Vec{20, 21})
	goal := Vec{40, 40}
	w.setOrder(busy, Order{Kind: "move", Position: &goal}, false)
	h.HP -= 100
	before := w.Players[1].Resources
	if err := w.Apply(1, Command{Kind: "repair_building", EntityIDs: []int{h.ID}}); err != nil {
		t.Fatal(err)
	}
	if worker.Order.Kind != "repair" || busy.Order.Kind != "move" || w.Players[1].Resources != before {
		t.Fatal("assignment changed economy or interrupted busy worker")
	}
	if err := w.Apply(1, Command{Kind: "repair_building", EntityIDs: []int{h.ID}}); err == nil {
		t.Fatal("duplicate assignment accepted")
	}
	for range 100 {
		w.Update()
	}
	if h.HP <= 450 {
		t.Fatal("building not repaired")
	}
	if err := w.Apply(1, Command{Kind: "stop", EntityIDs: []int{worker.ID}}); err != nil {
		t.Fatal(err)
	}
	hp := h.HP
	for range 50 {
		w.Update()
	}
	if h.HP != hp {
		t.Fatal("repair continued after Stop")
	}
	if err := w.Apply(1, Command{Kind: "repair_building", EntityIDs: []int{h.ID}}); err != nil {
		t.Fatal(err)
	}
	for range 700 {
		w.Update()
	}
	if h.HP != w.stats(h).HP {
		t.Fatal("repair failed to finish")
	}
	expected := definitions[h.Type].Cost.Scale(100 / w.stats(h).HP * .5)
	if diff := before.Wood - w.Players[1].Resources.Wood - expected.Wood; diff > 1e-7 || diff < -1e-7 {
		t.Fatal("repair charged beyond actual restored health")
	}
	farm := w.spawn("farm", 1, Vec{23, 23})
	if err := w.Apply(1, Command{Kind: "work_farm", EntityIDs: []int{farm.ID}}); err != nil {
		t.Fatal(err)
	}
	if worker.Order.Kind != "gather" || worker.Order.Target != farm.ID {
		t.Fatal("farm did not receive farmer")
	}
	for range 100 {
		w.Update()
	}
	if farm.Amount >= 175 {
		t.Fatal("farm did not produce")
	}
}

func TestEveryBuildingExplainsPurposeAndProductionCapabilities(t *testing.T) {
	for _, d := range GetCatalog().Definitions {
		if d.Kind == "building" && (d.Importance == "" || d.Description == "Construct "+d.Name+" with villagers.") {
			t.Fatalf("missing guide for %s", d.ID)
		}
	}
}
