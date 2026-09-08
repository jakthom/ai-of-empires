package game

import (
	"reflect"
	"testing"

	"github.com/open-ships/statemachine"
)

func TestIslandAIBuildsItsOwnFishingEconomy(t *testing.T) {
	w, err := NewWorld(Config{Seed: 11, Difficulty: "expert", World: WorldOptions{Type: "islands"}})
	if err != nil {
		t.Fatal(err)
	}
	for w.Time < 700 && len(w.entities(2, "fishing_ship")) == 0 {
		w.Update()
	}
	if !w.hasBuilding(2, "dock") || len(w.entities(2, "fishing_ship")) == 0 {
		t.Fatalf("AI never developed fishing after %.0fs: age=%d resources=%+v workers=%d", w.Time, w.Players[2].Age, w.Players[2].Resources, len(w.entities(2, "villager")))
	}
}

func TestIslandVoyageBoardsSailsLandsAndResumes(t *testing.T) {
	w, err := NewWorld(Config{Seed: 7, Difficulty: "easy", World: WorldOptions{Type: "islands", Reveal: "all"}})
	if err != nil {
		t.Fatal(err)
	}
	p := w.Players[2]
	p.Age = 1
	p.Temperament = aiExpansionist
	p.Resources = Resources{}
	w.Time = 700
	// Use the actual generated shores and ordinary movement/boarding effects.
	var water Vec
	for i, tile := range w.Tiles {
		if tile.Terrain == "water" {
			water = Vec{float64(i%w.Width) + .5, float64(i/w.Width) + .5}
			break
		}
	}
	home, ground, ok := w.knownShore(p, w.region(p.Start, false), w.region(water, true), p.Start)
	if !ok {
		t.Fatal("no home shore")
	}
	ship := w.spawn("transport", 2, home)
	for i := 0; i < 4; i++ {
		w.spawn("militia", 2, Vec{ground.X + float64(i%2)*.6, ground.Y + float64(i/2)*.6})
	}
	w.refreshVisibility()
	w.aiNavy(w.aiObserve(2))
	if p.voyage.State() != voyageBoarding || p.NavalPlan.ShipID != ship.ID {
		t.Fatalf("no voyage planned: %s %+v", p.voyage.State(), p.NavalPlan)
	}
	if len(p.NavalPlan.Crew) > 4 {
		t.Fatal("easy raid limit exceeded")
	}
	var restored *World
	landed := false
	for i := 0; i < 7000; i++ {
		w.Update()
		if restored != nil {
			restored.Update()
		}
		if p.voyage.State() == voyageSailing && restored == nil {
			data, err := w.Checkpoint()
			if err != nil {
				t.Fatal(err)
			}
			restored, err = Restore(data, w.JournalSince(0))
			if err != nil {
				t.Fatal(err)
			}
		}
		for _, record := range w.journal.records {
			if record.Message == "The transport expedition has landed." {
				landed = true
				break
			}
		}
		if landed {
			break
		}
	}
	if !landed {
		t.Fatalf("voyage stalled %s ship=%+v goal=%+v passengers=%v", p.voyage.State(), ship.Position, p.NavalPlan.GoalWater, ship.Passengers)
	}
	if restored == nil || !reflect.DeepEqual(w.View(2), restored.View(2)) {
		t.Fatal("voyage restore diverged")
	}
}

func TestVoyageLossAndInterruptionReleaseCrew(t *testing.T) {
	w, _ := NewWorld(Config{World: WorldOptions{Type: "islands", Reveal: "all"}})
	p := w.Players[2]
	crew := w.entities(2, "villager")[0]
	p.NavalPlan = navalPlan{ShipID: 999999, Crew: []int{crew.ID}}
	p.voyage = statemachine.NewInstance(voyageMachine, voyageBoarding)
	w.aiNavy(w.aiObserve(2))
	if p.voyage.State() != voyageIdle || p.voyaging(crew.ID) {
		t.Fatal("lost transport retained crew reservation")
	}
	if p.NavalPlan.NextAt <= w.Time {
		t.Fatal("retry was not delayed")
	}
}

func TestNavalEconomyUsesPaidProductionAndKnownFish(t *testing.T) {
	w, _ := NewWorld(Config{World: WorldOptions{Type: "islands", Reveal: "all"}, Difficulty: "easy"})
	p := w.Players[2]
	p.Resources = Resources{Wood: 500}
	p.Age = 1
	var water Vec
	for i, tile := range w.Tiles {
		if tile.Terrain == "water" {
			water = Vec{float64(i%w.Width) + .5, float64(i/w.Width) + .5}
			break
		}
	}
	home, land, ok := w.knownShore(p, w.region(p.Start, false), w.region(water, true), p.Start)
	if !ok {
		t.Fatal("no shore")
	}
	dock := w.spawn("dock", 2, land)
	w.aiNavy(w.aiObserve(2))
	if len(dock.Tasks) != 1 || dock.Tasks[0].Product != "fishing_ship" || p.Resources.Wood != 425 {
		t.Fatal("navy bypassed production/payment")
	}
	ship := w.spawn("fishing_ship", 2, home)
	w.resource("fish", Vec{home.X + 2, home.Y})
	w.refreshVisibility()
	w.aiNavy(w.aiObserve(2))
	if ship.Order.Kind != "gather" {
		t.Fatalf("fishing boat did not fish: %+v", ship.Order)
	}
	for i := range p.Visible {
		p.Visible[i] = false
	}
	w.setOrder(ship, Order{Kind: "idle"}, false)
	w.aiNavy(w.aiObserve(2))
	if ship.Order.Kind == "gather" {
		t.Fatal("boat targeted hidden fish")
	}
}
