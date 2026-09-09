package game

import (
	"math"
	"reflect"
	"testing"
)

func TestProductionMeasuresDeliveredIncomeInGameTime(t *testing.T) {
	w := New(Config{Difficulty: "peaceful"})
	for _, p := range w.Players {
		p.AI = false
	}
	worker := w.entities(1, "villager")[0]
	tree := w.spawn("tree", 0, Vec{worker.Position.X + 1, worker.Position.Y})
	tree.Resource, tree.Amount = "wood", 10000
	if err := w.Apply(1, Command{ID: "logging", Kind: "gather", EntityIDs: []int{worker.ID}, TargetID: tree.ID}); err != nil {
		t.Fatal(err)
	}
	monastery := w.spawn("monastery", 1, Vec{10, 40})
	monastery.Amount = 2
	w.Players[2].Resources.Gold = 98765 // foreign economy must not enter the series
	for range 2000 {
		w.Update()
	}
	v := w.View(1).Player.Production
	var wood float64
	for _, record := range w.JournalSince(0) {
		e := record.Event
		if e.Kind == "delivery" && e.EntityID == worker.ID && e.Time > w.Time-60+1e-6 {
			wood += e.Amount
		}
	}
	if wood <= 0 || math.Abs(v.Rates.Wood-wood) > 1e-6 || math.Abs(v.Rates.Gold-60) > 1e-6 {
		t.Fatalf("wrong gross rate: %+v, delivered wood %f", v.Rates, wood)
	}
	if v.Rates.Food != 0 || v.Rates.Stone != 0 {
		t.Fatal("starting supplies counted as production")
	}
	// Spending and a cancellation refund cannot change the delivery series.
	w.Players[1].Resources.Add(Resources{Food: -50, Wood: 100})
	if !reflect.DeepEqual(v, w.View(1).Player.Production) {
		t.Fatal("stockpile changes distorted production")
	}
	if err := w.SetPaused(true); err != nil {
		t.Fatal(err)
	}
	for range 200 {
		w.Update()
	}
	if !reflect.DeepEqual(v, w.View(1).Player.Production) {
		t.Fatal("paused game advanced its chart")
	}
	v.History[0].Rates.Wood = 999999
	if w.View(1).Player.Production.History[0].Rates.Wood == 999999 {
		t.Fatal("read model shared mutable history")
	}
}

func TestProductionCheckpointKeepsPartialBucketAndBoundsHistory(t *testing.T) {
	w := New(Config{Difficulty: "peaceful"})
	for _, p := range w.Players {
		p.AI = false
	}
	m := w.spawn("monastery", 1, Vec{10, 40})
	m.Amount = 1
	for range 123 {
		w.Update()
	}
	data, err := w.Checkpoint()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := Restore(data, w.JournalSince(0))
	if err != nil {
		t.Fatal(err)
	}
	for range 6400 {
		w.Update()
		restored.Update()
	}
	if !reflect.DeepEqual(w.Players[1].Production, restored.Players[1].Production) {
		t.Fatal("restart lost income or samples")
	}
	if len(restored.Players[1].Production.History) != productionHistory || len(restored.Players[1].Production.Buckets) != productionWindow {
		t.Fatal("rate history grew without a bound")
	}
	m.Amount = 0
	for range 1300 {
		w.Update()
	}
	if w.View(1).Player.Production.Rates != (Resources{}) {
		t.Fatal("income rate did not decay to zero after production stopped")
	}
}

func TestProductionSamplingWindowBoundaries(t *testing.T) {
	w := New(Config{Difficulty: "peaceful"})
	for _, p := range w.Players {
		p.AI = false
	}
	m := w.spawn("monastery", 1, Vec{10, 40})
	m.Amount = 2
	for range 99 {
		w.Update()
	}
	if len(w.View(1).Player.Production.History) != 0 {
		t.Fatal("sample emitted before five game seconds")
	}
	w.Update()
	first := w.View(1).Player.Production
	if len(first.History) != 1 || math.Abs(first.Rates.Gold-5) > 1e-8 || math.Abs(first.History[0].Time-5) > 1e-8 {
		t.Fatal("five-second bucket is incorrect", first)
	}
	m.Amount = 0
	for range 1100 {
		w.Update()
	}
	if math.Abs(w.View(1).Player.Production.Rates.Gold-5) > 1e-8 {
		t.Fatal("bucket expired before one minute")
	}
	for range 100 {
		w.Update()
	}
	if w.View(1).Player.Production.Rates.Gold != 0 {
		t.Fatal("bucket remained beyond one-minute window")
	}
}
