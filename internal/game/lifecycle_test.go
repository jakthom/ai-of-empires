package game

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"testing"

	"github.com/open-ships/statemachine"
)

func stepWorld(w *World, n int) {
	for range n {
		w.Update()
	}
}
func TestEconomyDepositsAndExhaustion(t *testing.T) {
	w := New(Config{Difficulty: "peaceful"})
	e := w.entities(1, "villager")[0]
	source := w.spawn("berries", 0, Vec{e.Position.X + 1, e.Position.Y})
	source.Resource = "food"
	source.Amount = .2
	start := w.Players[1].Resources.Food
	if err := w.Apply(1, Command{Kind: "gather", EntityIDs: []int{e.ID}, TargetID: source.ID}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100 && w.Entities[source.ID] != nil; i++ {
		w.behave(e)
	}
	if w.Players[1].Resources.Food != start || e.Cargo != .2 || w.Entities[source.ID] != nil {
		t.Fatal("gathering must create cargo, deplete the resource, and not deposit remotely")
	}
	stepWorld(w, 1000)
	if w.Players[1].Resources.Food < start+.2-1e-6 {
		t.Fatal("worker must return and deposit cargo")
	}
}
func TestProductionPopulationBlockAndCancellation(t *testing.T) {
	w := New(Config{Difficulty: "peaceful"})
	tc := w.entities(1, "town_center")[0]
	w.spawn("villager", 1, Vec{23, 46}) // full five-population housing
	before := w.Players[1].Resources.Food
	if err := w.Apply(1, Command{Kind: "train", EntityIDs: []int{tc.ID}, Product: "villager"}); err != nil {
		t.Fatal(err)
	}
	stepWorld(w, 3)
	if tc.production.State() != ProductionBlocked || tc.Tasks[0].Remaining != 25 {
		t.Fatal("housing must block production without consuming its time")
	}
	if err := w.Apply(1, Command{Kind: "cancel", EntityIDs: []int{tc.ID}}); err != nil {
		t.Fatal(err)
	}
	if len(tc.Tasks) != 0 || tc.production.State() != ProductionIdle || w.Players[1].Resources.Food != before {
		t.Fatal("cancellation must refund exactly once")
	}
	if err := w.Apply(1, Command{Kind: "cancel", EntityIDs: []int{tc.ID}}); err == nil {
		t.Fatal("cannot cancel the same task twice")
	}
}
func TestProductionResumesAndCompletesOnce(t *testing.T) {
	w := New(Config{Difficulty: "peaceful"})
	tc := w.entities(1, "town_center")[0]
	w.spawn("house", 1, Vec{14, 41})
	if err := w.Apply(1, Command{Kind: "train", EntityIDs: []int{tc.ID}, Product: "villager"}); err != nil {
		t.Fatal(err)
	}
	stepWorld(w, 550)
	if len(w.entities(1, "villager")) != 4 || len(tc.Tasks) != 0 || tc.production.State() != ProductionIdle {
		t.Fatal("training should create one unit and return to idle")
	}
}
func TestConstructionAndDeathAreTerminal(t *testing.T) {
	w := New(Config{Difficulty: "peaceful"})
	e := w.spawnWithLife("house", 1, Vec{14, 40}, Foundation)
	ctx := &entityContext{World: w, Actor: e, Amount: .4}
	mustFire(e.life, BuildWork, ctx)
	if e.life.State() != Foundation {
		t.Fatal("partial work completed construction")
	}
	ctx.Amount = .6
	mustFire(e.life, BuildWork, ctx)
	if e.life.State() != Active || e.Progress != 1 {
		t.Fatal("completion must commit active state")
	}
	before := e.HP
	if err := fire(e.life, BuildWork, ctx); !errors.Is(err, statemachine.ErrNotPermitted) || e.HP != before {
		t.Fatal("completed building must refuse further construction effects")
	}
	w.hit(e, 2, 10000)
	if e.life.State() != Destroyed || w.Entities[e.ID] != nil {
		t.Fatal("lethal damage must destroy the entity")
	}
	if err := fire(e.life, DeleteEntity, ctx); !errors.Is(err, statemachine.ErrNotPermitted) {
		t.Fatal("terminal entity must reject repeated deletion")
	}
}
func TestCombatFlightAndCooldown(t *testing.T) {
	w := New(Config{Difficulty: "peaceful"})
	e := w.spawn("archer", 1, Vec{20, 48})
	target := w.spawn("militia", 2, Vec{23, 48})
	target.Stance = "passive"
	w.refreshVisibility()
	if err := w.Apply(1, Command{Kind: "attack", EntityIDs: []int{e.ID}, TargetID: target.ID}); err != nil {
		t.Fatal(err)
	}
	if e.behavior.State() != Chasing {
		t.Fatal("attack order should enter chase lifecycle")
	}
	stepWorld(w, 10)
	if e.behavior.State() != AttackCooldown || len(w.Projectiles) != 1 || target.HP != w.stats(target).HP {
		t.Fatal("release must create a travelling projectile before impact")
	}
	stepWorld(w, 15)
	if target.HP >= w.stats(target).HP || len(w.Projectiles) != 0 {
		t.Fatal("projectile impact must damage target and finish flight")
	}
}
func TestGarrisonDestructionCleansAllPassengers(t *testing.T) {
	w := New(Config{Difficulty: "peaceful"})
	tc := w.entities(1, "town_center")[0]
	for i, e := range w.entities(1, "villager") {
		e.Position = Vec{tc.Position.X + 2.5, tc.Position.Y + float64(i)*.1}
		w.setOrder(e, Order{Kind: "garrison", Target: tc.ID}, false)
		w.behave(e)
		if e.behavior.State() != Garrisoned {
			t.Fatal("passenger must enter garrisoned state")
		}
	}
	if len(tc.Passengers) != 3 {
		t.Fatal("missing passengers")
	}
	w.remove(tc.ID)
	if len(w.entities(1, "villager")) != 0 {
		t.Fatal("container destruction must clean every passenger, not skip shifted slice entries")
	}
}
func TestPauseAndFinishedMatchRejectInvalidTransitions(t *testing.T) {
	w := New(Config{Difficulty: "peaceful"})
	if err := w.Apply(1, Command{Kind: "pause"}); err != nil {
		t.Fatal(err)
	}
	stepWorld(w, 100)
	if w.Tick != 0 || w.match.State() != MatchPaused {
		t.Fatal("paused match advanced")
	}
	if err := w.Apply(1, Command{Kind: "resign"}); err != nil {
		t.Fatal(err)
	}
	if w.match.State() != MatchFinished || w.Winner != 2 {
		t.Fatal("resignation should complete paused match")
	}
	if err := fire(w.match, ResumeMatch, &matchContext{World: w}); !errors.Is(err, statemachine.ErrNotPermitted) {
		t.Fatal("finished match cannot resume")
	}
}
func TestEveryBehaviorHandlesPulse(t *testing.T) {
	states := map[UnitState]bool{Garrisoned: true}
	for _, row := range unitRows {
		states[row.From] = true
		states[row.To] = true
	}
	for state := range states {
		hasPulse := false
		for _, row := range unitRows {
			if row.From == state && row.Event == UnitPulse {
				hasPulse = true
			}
		}
		if !hasPulse {
			t.Errorf("%s has no pulse transition", state)
		}
	}
}

func TestQueuedBuildRefusalIsAtomic(t *testing.T) {
	w := New(Config{Difficulty: "peaceful"})
	e := w.entities(1, "villager")[0]
	e.Orders = make([]Order, 64)
	before, count := w.Players[1].Resources, len(w.Entities)
	err := w.Apply(1, Command{Kind: "build", EntityIDs: []int{e.ID}, Product: "house", Position: &Vec{14, 40}, Queue: true})
	if err == nil || w.Players[1].Resources != before || len(w.Entities) != count {
		t.Fatal("rejected queued build spent resources or created a foundation")
	}
}

func TestSiegeDeploymentGatesMovement(t *testing.T) {
	w := New(Config{Difficulty: "peaceful"})
	e := w.spawn("trebuchet", 1, Vec{24, 46})
	deploy := Command{Kind: "deploy", EntityIDs: []int{e.ID}}
	if err := w.Apply(1, deploy); err != nil {
		t.Fatal(err)
	}
	if e.siege.State() != SiegeDeploying {
		t.Fatal("deployment skipped its timed state")
	}
	if err := w.Apply(1, deploy); !errors.Is(err, statemachine.ErrNotPermitted) {
		t.Fatal("deployment restarted while in progress")
	}
	move := Command{Kind: "move", EntityIDs: []int{e.ID}, Position: &Vec{26, 46}}
	if err := w.Apply(1, move); err == nil {
		t.Fatal("moving during deployment")
	}
	stepWorld(w, 61)
	if e.siege.State() != SiegeDeployed {
		t.Fatal("deployment did not finish")
	}
	if err := w.Apply(1, deploy); err != nil {
		t.Fatal(err)
	}
	stepWorld(w, 61)
	if e.siege.State() != SiegePacked {
		t.Fatal("packing did not finish")
	}
	if err := w.Apply(1, move); err != nil {
		t.Fatal(err)
	}
}

func TestSkirmishLifecycleSoak(t *testing.T) {
	if testing.Short() {
		t.Skip("skirmish soak")
	}
	w := New(Config{Civilization: "britons", Difficulty: "hard", Mode: "sandbox", Seed: 4817})
	stepWorld(w, 12000)
	for _, e := range w.Entities {
		if !e.Position.Finite() || math.IsNaN(e.HP) || e.HP <= 0 || e.life.State() == Destroyed {
			t.Fatalf("invalid live entity: %s/%d", e.Type, e.ID)
		}
	}
	for _, p := range w.Players {
		if !p.Resources.CanPay(Resources{}) {
			t.Fatal("negative stockpile")
		}
	}
}
func TestReadModelDeterminismAndFog(t *testing.T) {
	a, b := New(Config{Seed: 83, Difficulty: "peaceful"}), New(Config{Seed: 83, Difficulty: "peaceful"})
	stepWorld(a, 40)
	stepWorld(b, 40)
	x, _ := json.Marshal(a.View(1))
	y, _ := json.Marshal(b.View(1))
	if string(x) != string(y) {
		t.Fatal("same seed and inputs produced different views")
	}
	for _, e := range a.View(1).Entities {
		if e.Owner == 2 {
			t.Fatal("unscouted enemy leaked")
		}
	}
	before := a.rng
	e := a.entities(1, "villager")[0]
	for range e.behavior.Permitted(context.Background(), &unitContext{Actor: e}) {
	}
	if a.rng != before {
		t.Fatal("reading actions changed simulation randomness")
	}
}

func TestProjectileViewsDoNotRevealHiddenDestinations(t *testing.T) {
	w := New(Config{Difficulty: "peaceful"})
	visible := w.entities(1, "town_center")[0].Position
	hidden := w.entities(2, "town_center")[0].Position
	w.Projectiles = []Projectile{{ID: 10001, Owner: 2, Position: visible, Destination: hidden, Target: 999, Damage: 50, Kind: "arrow"}, {ID: 10002, Owner: 2, Position: hidden, Kind: "arrow"}}
	v := w.View(1)
	if len(v.Projectiles) != 1 {
		t.Fatal("projectile outside sight leaked")
	}
	data, err := json.Marshal(v.Projectiles[0])
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	for _, hiddenField := range []string{"destination", "target", "source", "damage", "flight", "speed"} {
		if _, ok := fields[hiddenField]; ok {
			t.Fatalf("projectile exposes %s", hiddenField)
		}
	}
	w.Projectiles[0].Position = hidden
	if v.Projectiles[0].Position != visible {
		t.Fatal("published snapshot changed with the world")
	}
}
