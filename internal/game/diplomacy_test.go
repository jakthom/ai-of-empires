package game

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/open-ships/statemachine"
)

func peacefulWorld() *World {
	w := New(Config{Difficulty: "peaceful", Settlements: 3})
	w.Entities, w.IDs = map[int]*Entity{}, nil
	for i := range w.Tiles {
		w.Tiles[i] = Tile{Terrain: "grass"}
	}
	for id, pos := range []Vec{{10, 10}, {60, 10}, {60, 60}} {
		w.spawn("town_center", id+1, pos)
		w.Players[id+1].Memory = map[int]EntityView{}
	}
	return w
}

func order(t *testing.T, w *World, player int, c Command) {
	t.Helper()
	if err := w.Apply(player, c); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultMilitaryAndBuildingsLeavePassingKingdomsAtPeace(t *testing.T) {
	for _, typ := range []string{"militia", "archer", "tower", "castle", "town_center"} {
		t.Run(typ, func(t *testing.T) {
			w := peacefulWorld()
			e := w.spawn(typ, 1, Vec{20, 30})
			if typ == "town_center" {
				v := w.spawn("villager", 1, Vec{22, 30})
				w.refreshVisibility()
				order(t, w, 1, Command{Kind: "garrison", EntityIDs: []int{v.ID}, TargetID: e.ID})
				stepWorld(w, 5)
				if len(e.Passengers) != 1 {
					t.Fatal("fixture did not garrison the Town Center")
				}
			}
			other := w.spawn("militia", 2, Vec{20, 34})
			w.refreshVisibility()
			stepWorld(w, 240)
			if e.Stance != "defensive" || e.HP != w.stats(e).HP || other.HP != w.stats(other).HP || e.behavior.State() != Idle || other.behavior.State() != Idle || w.relation(1, 2) != atPeace {
				t.Fatal("proximity alone started combat")
			}
		})
	}
}

func TestReturnFireProtectsNearbyFriendsAgainstTheActualAttacker(t *testing.T) {
	w := peacefulWorld()
	worker := w.spawn("villager", 1, Vec{20, 30})
	worker.Stance = "passive"
	guard := w.spawn("archer", 1, Vec{20, 31})
	attacker := w.spawn("archer", 2, Vec{23, 30})
	bystander := w.spawn("militia", 2, Vec{21, 31})
	neutral := w.spawn("militia", 3, Vec{20, 32})
	w.refreshVisibility()
	order(t, w, 2, Command{Kind: "attack", EntityIDs: []int{attacker.ID}, TargetID: worker.ID})
	stepWorld(w, 20)
	if guard.Order.Target != attacker.ID || guard.Order.Initiated || guard.Order.DefendFrom == nil {
		t.Fatal("guard did not retaliate against the actual attacker")
	}
	if bystander.HP != w.stats(bystander).HP || neutral.HP != w.stats(neutral).HP || w.relation(1, 3) != atPeace || w.relation(2, 3) != atPeace {
		t.Fatal("retaliation pulled innocent bystanders into the conflict")
	}
	if w.relation(1, 2) != inConflict || w.relation(2, 1) != inConflict {
		t.Fatal("actual attack did not start a mutual, pairwise conflict")
	}
}

func TestRetaliationStopsWhenAttackerLeavesAndDoesNotRenewItsPursuit(t *testing.T) {
	w := peacefulWorld()
	guard := w.spawn("militia", 1, Vec{20, 30})
	attacker := w.spawn("militia", 2, Vec{21, 30})
	w.refreshVisibility()
	w.hitFrom(guard, 2, attacker.ID, 1)
	w.behave(guard)
	if guard.Order.Target != attacker.ID {
		t.Fatal("unit did not defend itself")
	}
	guard.Position = Vec{26.2, 30}
	attacker.Position = Vec{29, 30}
	w.refreshVisibility()
	for range 10 {
		w.behave(guard)
	}
	if guard.behavior.State() != Idle || guard.Position != (Vec{26.2, 30}) {
		t.Fatal("automatic retaliation chased beyond the attacked area")
	}
	attacker.Position = Vec{27, 30}
	w.Time += retaliationMemory + 1
	w.behave(guard)
	if guard.behavior.State() != Idle {
		t.Fatal("stale aggression restarted a fight")
	}
}

func TestStancesAndExplicitOrdersHaveDistinctConsequences(t *testing.T) {
	for _, stance := range []string{"defensive", "stand_ground", "passive", "aggressive"} {
		t.Run(stance, func(t *testing.T) {
			w := peacefulWorld()
			e := w.spawn("militia", 1, Vec{20, 30})
			target := w.spawn("militia", 2, Vec{23, 30})
			target.Stance = "passive"
			w.refreshVisibility()
			order(t, w, 1, Command{Kind: "stance", EntityIDs: []int{e.ID}, Product: stance})
			stepWorld(w, 5)
			if stance == "aggressive" {
				if e.Order.Target != target.ID {
					t.Fatal("opted-in aggression did not engage")
				}
				order(t, w, 1, Command{Kind: "stance", EntityIDs: []int{e.ID}, Product: "defensive"})
				w.behave(e)
				if e.behavior.State() != Idle {
					t.Fatal("return fire did not interrupt unsolicited aggression")
				}
			} else if e.behavior.State() != Idle {
				t.Fatal("nonaggressive stance engaged without an attack")
			}
			w.hitFrom(e, 2, target.ID, 1)
			stepWorld(w, 2)
			if (stance == "passive" || stance == "stand_ground") && e.Position != (Vec{20, 30}) {
				t.Fatal("stationary or passive unit pursued an attacker")
			}
			order(t, w, 1, Command{Kind: "stance", EntityIDs: []int{e.ID}, Product: "passive"})
			w.behave(e)
			if e.behavior.State() != Idle {
				t.Fatal("hold fire failed to stop an automatic response")
			}
			order(t, w, 1, Command{Kind: "attack", EntityIDs: []int{e.ID}, TargetID: target.ID})
			stepWorld(w, 160)
			if target.HP >= w.stats(target).HP {
				t.Fatal("explicit attack did not override automatic firing stance")
			}
		})
	}
}

func TestScopedAttackMoveDoesNotAttackUninvolvedKingdoms(t *testing.T) {
	w := peacefulWorld()
	e := w.spawn("archer", 1, Vec{20, 30})
	bystander := w.spawn("militia", 3, Vec{21, 30})
	target := w.spawn("militia", 2, Vec{23, 30})
	w.refreshVisibility()
	for _, player := range []int{-1, 1, 4} {
		if err := w.Apply(1, Command{Kind: "attack_move", EntityIDs: []int{e.ID}, Position: &Vec{30, 30}, TargetPlayer: player}); err == nil || e.behavior.State() != Idle {
			t.Fatal("invalid kingdom changed an order")
		}
	}
	order(t, w, 1, Command{Kind: "attack_move", EntityIDs: []int{e.ID}, Position: &Vec{30, 30}, TargetPlayer: 2})
	stepWorld(w, 30)
	if e.Order.Target != target.ID || e.Order.TargetPlayer != 2 || bystander.HP != w.stats(bystander).HP || w.relation(1, 3) != atPeace {
		t.Fatal("scoped march attacked an uninvolved kingdom")
	}
}

func TestRelationshipLifecycleHasPureGuardsAndOnceOnlyNotices(t *testing.T) {
	w := peacefulWorld()
	a, b := w.spawn("militia", 1, Vec{20, 30}), w.spawn("militia", 2, Vec{21, 30})
	r := w.Relations[relationKey(1, 2)]
	c := &relationContext{World: w, Relation: r}
	before := w.NextEvent
	if err := fire(r.lifecycle, relationEvent("invalid"), c); !errors.Is(err, statemachine.ErrNotPermitted) || w.NextEvent != before {
		t.Fatal("invalid relation event had effects")
	}
	w.noteAggression(a.ID, 1, b)
	after := w.NextEvent
	if after != before+2 {
		t.Fatal("conflict should notify each affected kingdom once")
	}
	w.Time = 100
	w.noteAggression(a.ID, 1, b)
	if w.NextEvent != after || r.QuietUntil != 400 {
		t.Fatal("continued aggression duplicated notices or failed to renew conflict")
	}
	w.Time = 401
	if _, err := relationMachine.Next(context.Background(), r.lifecycle.State(), relationPulse, c); err != nil || r.lifecycle.State() != inConflict || w.NextEvent != after {
		t.Fatal("guard query mutated the relationship")
	}
	w.pulseRelations()
	if w.relation(1, 2) != atPeace || w.NextEvent != after+2 || len(w.Incidents[2]) != 0 {
		t.Fatal("quiet period did not restore peace and expire incidents")
	}
	w.pulseRelations()
	if w.NextEvent != after+2 {
		t.Fatal("peace notice repeated")
	}
	w.noteAggression(a.ID, 1, b)
	if w.relation(1, 2) != inConflict {
		t.Fatal("new attacks must be able to break the peace")
	}
}

func TestScopedMarchCanDefendItselfAndResumeWithoutChangingItsOpponent(t *testing.T) {
	w := peacefulWorld()
	e := w.spawn("archer", 1, Vec{20, 30})
	attacker := w.spawn("militia", 3, Vec{22, 30})
	w.refreshVisibility()
	order(t, w, 1, Command{Kind: "attack_move", EntityIDs: []int{e.ID}, Position: &Vec{35, 30}, TargetPlayer: 2})
	w.hitFrom(e, 3, attacker.ID, 1)
	w.behave(e)
	if e.Order.Target != attacker.ID || e.Order.Initiated || e.Order.DefendFrom == nil {
		t.Fatal("scoped march could not defend itself against a third kingdom")
	}
	w.remove(attacker.ID)
	w.behave(e)
	if e.Order.Kind != "attack_move" || e.Order.TargetPlayer != 2 {
		t.Fatal("retaliation discarded or broadened the original march")
	}
	target := w.spawn("militia", 2, Vec{23, 30})
	w.refreshVisibility()
	w.behave(e)
	if e.Order.Target != target.ID {
		t.Fatal("did not resume the intended opponent")
	}
	target.Owner = 3 // A conversion changes which kingdom owns the target.
	w.behave(e)
	if e.Order.Kind != "attack_move" || e.Order.TargetPlayer != 2 {
		t.Fatal("scoped attack followed a converted entity into another kingdom")
	}
}

func TestConversionAttemptsBreakPeaceBeforeOwnershipChanges(t *testing.T) {
	w := peacefulWorld()
	monk := w.spawn("monk", 1, Vec{20, 30})
	target := w.spawn("militia", 2, Vec{23, 30})
	w.refreshVisibility()
	order(t, w, 1, Command{Kind: "convert", EntityIDs: []int{monk.ID}, TargetID: target.ID})
	w.behave(monk)
	w.behave(monk)
	if monk.behavior.State() != Converting || target.Owner != 2 || w.relation(1, 2) != inConflict {
		t.Fatal("conversion did not provoke its target kingdom when the attempt began")
	}
	w.behave(target)
	if target.Order.Target != monk.ID {
		t.Fatal("target could not defend itself against a conversion attempt")
	}
}

func TestCheckpointPreservesRelationshipsRetaliationAndPreferences(t *testing.T) {
	w := peacefulWorld()
	a, b := w.spawn("archer", 1, Vec{20, 30}), w.spawn("militia", 2, Vec{23, 30})
	w.refreshVisibility()
	w.hitFrom(a, 2, b.ID, 1)
	w.behave(a)
	w.Players[2].Temperament = aiGuarded
	data, _ := w.Checkpoint()
	r, err := Restore(data, w.JournalSince(0))
	if err != nil {
		t.Fatal(err)
	}
	roundtrip, _ := r.Checkpoint()
	if !bytes.Equal(data, roundtrip) || !reflect.DeepEqual(w.JournalSince(0), r.JournalSince(0)) {
		t.Fatal("restoration lost diplomacy data or replayed effects")
	}
	for range 200 {
		w.Update()
		r.Update()
	}
	x, _ := w.Checkpoint()
	y, _ := r.Checkpoint()
	if !bytes.Equal(x, y) {
		t.Fatal("saved retaliation diverged on continuation")
	}
}

func TestOlderSavesStartPeacefullyWithoutDiscardingEconomyOrHistory(t *testing.T) {
	for _, version := range []int{1, 2} {
		w := strategyWorld()
		w.Config.Difficulty = "normal"
		revealStrategyWorld(w, 2)
		w.thinkPlayer(2)
		human := w.entities(1, "town_center")[0]
		human.Stance = "aggressive"
		data, _ := w.Checkpoint()
		var old map[string]any
		if err := json.Unmarshal(data, &old); err != nil {
			t.Fatal(err)
		}
		old["Version"] = version
		delete(old, "Relations")
		world := old["World"].(map[string]any)
		delete(world, "Relations")
		delete(world, "Incidents")
		for _, p := range world["Players"].(map[string]any) {
			delete(p.(map[string]any), "Temperament")
		}
		data, _ = json.Marshal(old)
		r, err := Restore(data, w.JournalSince(0))
		if err != nil {
			t.Fatal(err)
		}
		if r.relation(1, 2) != atPeace || r.Entities[human.ID].Stance != "defensive" || r.Players[2].strategy.State() != aiDeveloping {
			t.Fatal("old unsolicited aggression survived migration")
		}
		if r.Players[2].Resources != w.Players[2].Resources || !reflect.DeepEqual(r.JournalSince(0), w.JournalSince(0)) {
			t.Fatal("migration changed resources or immutable history")
		}
		for _, e := range r.entities(2, "") {
			if e.Order.Kind == "attack" || e.Order.Kind == "attack_move" {
				t.Fatal("old AI raid was not recalled")
			}
		}
		stepWorld(r, 100)
	}
}
