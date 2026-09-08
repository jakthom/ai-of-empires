package game

import (
	"context"
	"testing"

	"github.com/open-ships/statemachine"
)

func monkFixture() (*World, *Entity, *Entity) {
	w := New(Config{Difficulty: "peaceful"})
	monk := w.spawn("monk", 1, Vec{20, 48})
	target := w.spawn("militia", 1, Vec{22, 48})
	w.refreshVisibility()
	return w, monk, target
}
func pulse(w *World, e *Entity, n int) {
	for i := 0; i < n; i++ {
		w.behave(e)
	}
}
func TestHealingLifecycle(t *testing.T) {
	w, e, target := monkFixture()
	target.HP -= 1
	w.setOrder(e, Order{Kind: "heal", Target: target.ID}, false)
	if e.behavior.State() != ApproachingHeal {
		t.Fatal("heal must approach first")
	}
	w.tickMonk(e)
	if e.behavior.State() != Healing {
		t.Fatal("in-range monk must enter healing")
	}
	pulse(w, e, 20)
	if target.HP != w.stats(target).HP || e.behavior.State() != Idle || e.Order.Kind != "idle" {
		t.Fatal("healing must finish without overhealing")
	}
}
func TestHealingLosesTargetWithoutApplyingEffect(t *testing.T) {
	w, e, target := monkFixture()
	target.HP = 10
	w.setOrder(e, Order{Kind: "heal", Target: target.ID}, false)
	w.tickMonk(e)
	target.Owner = 2
	w.tickMonk(e)
	if target.HP != 10 || e.behavior.State() != Idle {
		t.Fatal("changed ownership must invalidate healing before effect")
	}
}
func TestConversionWaitsForFaithAndResetsOutOfRange(t *testing.T) {
	w, e, target := monkFixture()
	target.Owner = 2
	e.Faith = 20
	w.setOrder(e, Order{Kind: "convert", Target: target.ID}, false)
	pulse(w, e, 2)
	if e.behavior.State() != RecoveringFaith || e.Work != 0 {
		t.Fatal("conversion cannot progress without faith")
	}
	e.Faith = 100
	w.tickMonk(e)
	pulse(w, e, 10)
	if e.behavior.State() != Converting || e.Work <= 0 {
		t.Fatal("faith enables conversion")
	}
	target.Position = Vec{29, 48}
	w.Players[1].Visible[int(target.Position.Y)*w.Width+int(target.Position.X)] = true
	w.tickMonk(e)
	if e.behavior.State() != ApproachingConvert || e.Work != 0 {
		t.Fatal("leaving range must reset conversion progress")
	}
}
func TestConversionEffectAndGuardsAreSeparate(t *testing.T) {
	w, e, target := monkFixture()
	target.Owner = 2
	w.setOrder(e, Order{Kind: "convert", Target: target.ID}, false)
	pulse(w, e, 2)
	e.Work = 12
	c := &unitContext{World: w, Actor: e, Target: target, Roll: 0}
	before := w.rng
	for range 3 {
		for range e.behavior.Permitted(context.Background(), c) {
		}
	}
	if target.Owner != 2 || w.rng != before || e.Faith != 100 {
		t.Fatal("affordance inspection must not execute effects or consume randomness")
	}
	if _, err := e.behavior.Fire(context.Background(), UnitPulse, c); err != nil {
		t.Fatal(err)
	}
	if target.Owner != 1 || e.Faith != 0 || e.behavior.State() != Idle {
		t.Fatal("successful transition must transfer ownership and consume faith")
	}
	w.behave(e)
	if target.Owner != 1 || e.Faith != 0 {
		t.Fatal("completed conversion effect cannot repeat")
	}
}
func TestRelicConservationAndInterruption(t *testing.T) {
	w, e, _ := monkFixture()
	relic := w.spawn("relic", 0, Vec{21, 48})
	monastery := w.spawn("monastery", 1, Vec{22, 48})
	w.refreshVisibility()
	w.setOrder(e, Order{Kind: "relic", Target: relic.ID}, false)
	pulse(w, e, 2)
	if !e.Relic || w.Entities[relic.ID] != nil {
		t.Fatal("pickup must transfer the relic exactly once")
	}
	w.setOrder(e, Order{Kind: "deposit_relic", Target: monastery.ID}, false)
	pulse(w, e, 2)
	if e.Relic || monastery.Amount != 1 {
		t.Fatal("deposit must transfer the carried relic exactly once")
	}
	w.remove(monastery.ID)
	count := 0
	for _, r := range w.entities(0, "relic") {
		if r.Position.Distance(monastery.Position) < 1 {
			count++
		}
	}
	if count != 1 {
		t.Fatal("destroyed monastery must release exactly one relic")
	}
	e.behavior = statemachine.NewInstance(unitMachine, Healing)
	w.setOrder(e, Order{Kind: "move", Position: &Vec{23, 48}}, false)
	if e.behavior.State() != Moving || e.Work != 0 {
		t.Fatal("new command must interrupt behavior")
	}
}
