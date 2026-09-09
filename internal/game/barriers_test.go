package game

import (
	"math"
	"testing"
)

func barrierWorld() *World {
	w := New(Config{Difficulty: "peaceful", Settlements: 2, Mode: "sandbox"})
	w.Entities = map[int]*Entity{}
	w.IDs = nil
	for i := range w.Tiles {
		w.Tiles[i] = Tile{Terrain: "grass"}
	}
	for _, p := range w.Players {
		p.Age = 1
		p.AI = false
		for i := range p.Explored {
			p.Explored[i] = true
			p.Visible[i] = true
		}
	}
	w.rebuildRegions()
	return w
}

func TestWallDragBuildsContinuousLineWithOnePriceAndQueuedWork(t *testing.T) {
	w := barrierWorld()
	worker := w.spawn("villager", 1, Vec{10.5, 18.5})
	w.spawn("villager", 2, Vec{60.5, 60.5})
	start, end := Vec{10.5, 20.5}, Vec{16.5, 20.5}
	sites, price, err := w.PlanBuilding(1, "wall", start, &end)
	if err != nil || len(sites) != 7 || price.Stone != 35 {
		t.Fatalf("wrong plan: %v %+v %v", sites, price, err)
	}
	before := w.Players[1].Resources
	if err = w.Apply(1, Command{Kind: "build", Product: "wall", Position: &start, EndPosition: &end, EntityIDs: []int{worker.ID}}); err != nil {
		t.Fatal(err)
	}
	if w.Players[1].Resources.Stone != before.Stone-35 || len(w.entities(1, "wall")) != 7 || len(worker.Orders) != 6 {
		t.Fatal("line not admitted and queued atomically")
	}
	for range 2800 {
		w.Update()
	}
	for _, e := range w.entities(1, "wall") {
		if e.life.State() != Active {
			t.Fatalf("builder stalled on wall #%d: progress=%f state=%s position=%+v", e.ID, e.Progress, worker.behavior.State(), worker.Position)
		}
	}
	for i := 1; i < len(sites); i++ {
		if sites[i-1].Distance(sites[i]) != 1 {
			t.Fatal("gap between wall segments")
		}
	}
	if links := w.entityView(w.barrierAt(Vec{13.5, 20.5}), 1).Connections; len(links) != 2 {
		t.Fatal("wall connections missing from snapshot")
	}
}

func TestWallDragRefusesWholeLineOnObstructionCostOrQueueLimit(t *testing.T) {
	for _, failure := range []string{"obstruction", "cost", "queue"} {
		t.Run(failure, func(t *testing.T) {
			w := barrierWorld()
			worker := w.spawn("villager", 1, Vec{10.5, 18.5})
			start, end := Vec{10.5, 20.5}, Vec{16.5, 20.5}
			if failure == "obstruction" {
				w.spawn("tree", 0, Vec{16.5, 20.5})
			}
			if failure == "cost" {
				w.Players[1].Resources.Stone = 10
			}
			if failure == "queue" {
				worker.Orders = make([]Order, 60)
			}
			before, id := w.Players[1].Resources, w.NextID
			if err := w.Apply(1, Command{Kind: "build", Product: "wall", Position: &start, EndPosition: &end, EntityIDs: []int{worker.ID}, Queue: true}); err == nil {
				t.Fatal("invalid wall accepted")
			}
			if w.Players[1].Resources != before || w.NextID != id || len(w.entities(1, "wall")) != 0 {
				t.Fatal("invalid wall changed economy or left partial foundations")
			}
		})
	}
	for _, end := range []Vec{{14.5, 24.5}, {6.5, 24.5}, {6.5, 16.5}, {14.5, 16.5}} {
		points, err := wallRoute(Vec{10.5, 20.5}, end)
		if err != nil {
			t.Fatal(err)
		}
		for i := 1; i < len(points); i++ {
			if points[i].Distance(points[i-1]) != 1 {
				t.Fatal("diagonal drag leaves a passable gap")
			}
		}
	}
}

func TestWallDragCannotBendThroughStandaloneGate(t *testing.T) {
	w := barrierWorld()
	worker := w.spawn("villager", 1, Vec{18.5, 18.5})
	w.spawn("gate", 1, Vec{20.5, 20.5})
	start, end := Vec{19.5, 20.5}, Vec{20.5, 21.5}
	before := w.Players[1].Resources
	if err := w.Apply(1, Command{Kind: "build", Product: "wall", Position: &start, EndPosition: &end, EntityIDs: []int{worker.ID}}); err == nil {
		t.Fatal("drag created incompatible gate connections")
	}
	if w.Players[1].Resources != before || len(w.entities(1, "wall")) != 0 {
		t.Fatal("rejected gate corner created or charged for foundations")
	}
	end = Vec{21.5, 20.5}
	if err := w.Apply(1, Command{Kind: "build", Product: "wall", Position: &start, EndPosition: &end, EntityIDs: []int{worker.ID}}); err != nil {
		t.Fatal("drag could not join opposite sides of a standalone gate", err)
	}
}

func TestGateReplacesWallAndProtectsSettlementUntilDestroyed(t *testing.T) {
	w := barrierWorld()
	friendly := w.spawn("villager", 1, Vec{23.5, 18.5})
	enemy := w.spawn("villager", 2, Vec{24.5, 18.5})
	for x := 20.5; x <= 26.5; x++ {
		for y := 20.5; y <= 26.5; y++ {
			if x == 20.5 || x == 26.5 || y == 20.5 || y == 26.5 {
				w.spawn("wall", 1, Vec{x, y})
			}
		}
	}
	site, goal := Vec{23.5, 20.5}, Vec{23.5, 23.5}
	old := w.barrierAt(site)
	before := w.Players[1].Resources.Stone
	if err := w.Apply(1, Command{Kind: "build", Product: "gate", Position: &site, EntityIDs: []int{friendly.ID}}); err != nil {
		t.Fatal(err)
	}
	gate := w.barrierAt(site)
	if gate == nil || gate.Type != "gate" || gate.ID == old.ID || w.Entities[old.ID] != nil || before-w.Players[1].Resources.Stone != 30 {
		t.Fatal("gate did not replace wall exactly once")
	}
	if w.freeFor(friendly, site) {
		t.Fatal("unfinished gate lets units through")
	}
	mustFire(gate.life, BuildWork, &entityContext{World: w, Actor: gate, Amount: 1})
	if !w.freeFor(friendly, site) || w.freeFor(enemy, site) {
		t.Fatal("gate does not distinguish own units from enemies")
	}
	if len(w.findPath(enemy, goal, .4, false)) != 0 {
		t.Fatal("enemy path crossed closed perimeter")
	}
	if err := w.Apply(1, Command{Kind: "move", Position: &goal, EntityIDs: []int{friendly.ID}}); err != nil {
		t.Fatal(err)
	}
	for range 300 {
		w.Update()
	}
	if friendly.Position.Distance(goal) > .8 {
		t.Fatalf("friendly unit could not enter its gate: %+v", friendly.Position)
	}
	w.hit(gate, 2, 10000)
	if w.Entities[gate.ID] != nil || len(w.findPath(enemy, goal, .4, false)) == 0 {
		t.Fatal("destroyed gate did not open a breach")
	}
	if links := w.barrierLinks(w.barrierAt(Vec{22.5, 20.5}), 1); len(links) != 1 {
		t.Fatal("destroyed gate retained a wall connection")
	}
}

func TestStandaloneGateConnectsWallsAndRejectedReplacementKeepsWall(t *testing.T) {
	w := barrierWorld()
	worker := w.spawn("villager", 1, Vec{10.5, 18.5})
	gate := w.spawn("gate", 1, Vec{20.5, 20.5})
	for _, p := range []Vec{{19.5, 20.5}, {21.5, 20.5}} {
		if err := w.Placement(1, "wall", p); err != nil {
			t.Fatal("wall cannot join standalone gate", err)
		}
		w.spawn("wall", 1, p)
	}
	if len(w.barrierLinks(gate, 1)) != 2 {
		t.Fatal("standalone gate did not connect")
	}
	if err := w.Placement(1, "wall", Vec{20.5, 21.5}); err == nil {
		t.Fatal("perpendicular wall joined a straight gate")
	}
	w.spawn("wall", 1, Vec{40.5, 40.5})
	w.spawn("wall", 1, Vec{41.5, 40.5})
	w.spawn("wall", 1, Vec{40.5, 41.5})
	if err := w.Placement(1, "gate", Vec{40.5, 40.5}); err == nil {
		t.Fatal("gate allowed at a corner without a straight opening")
	}
	wall := w.spawn("wall", 1, Vec{30.5, 30.5})
	w.spawn("tree", 0, Vec{30.5, 31.2})
	before := w.Players[1].Resources
	if err := w.Apply(1, Command{Kind: "build", Product: "gate", Position: &wall.Position, EntityIDs: []int{worker.ID}}); err == nil {
		t.Fatal("obstructed replacement accepted")
	}
	if w.Entities[wall.ID] != wall || w.Players[1].Resources != before {
		t.Fatal("rejected replacement removed wall or spent resources")
	}
	if _, _, err := w.PlanBuilding(1, "wall", Vec{math.NaN(), 1}, nil); err == nil {
		t.Fatal("non-finite placement accepted")
	}
}

func TestWallDragEndpointLimitsThroughCommands(t *testing.T) {
	for _, end := range []Vec{{10.5, 20.5}, {70.5, 30.5}, {math.Inf(1), 20}, {math.NaN(), 20}} {
		w := barrierWorld()
		worker := w.spawn("villager", 1, Vec{10.5, 18.5})
		start := Vec{10.5, 20.5}
		err := w.Apply(1, Command{Kind: "build", Product: "wall", Position: &start, EndPosition: &end, EntityIDs: []int{worker.ID}})
		if end == start {
			if err != nil || len(w.entities(1, "wall")) != 1 {
				t.Fatal("zero-length drag should place one wall", err)
			}
		} else if err == nil || len(w.entities(1, "wall")) != 0 {
			t.Fatal("invalid wall endpoint admitted", err)
		}
	}
}
