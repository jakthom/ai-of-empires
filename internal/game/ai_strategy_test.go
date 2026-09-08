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

func strategyWorld() *World {
	w := New(Config{Difficulty: "aggressive", Settlements: 3})
	w.Entities, w.IDs = map[int]*Entity{}, nil
	w.Time = 300
	for i := range w.Tiles {
		w.Tiles[i] = Tile{Terrain: "grass"}
	}
	for _, p := range w.Players {
		p.Civilization = "britons"
		p.Resources = Resources{}
		p.Memory = map[int]EntityView{}
		clear(p.Visible)
		clear(p.Explored)
	}
	w.spawn("town_center", 1, Vec{15, 40})
	w.spawn("town_center", 2, Vec{40, 40})
	w.spawn("town_center", 3, Vec{74, 40})
	w.spawn("villager", 2, Vec{40, 44})
	for i := range 8 {
		w.spawn("militia", 2, Vec{40 + float64(i)*.6, 46})
	}
	w.refreshVisibility()
	return w
}

func revealStrategyWorld(w *World, player int) {
	for i := range w.Tiles {
		w.Players[player].Explored[i], w.Players[player].Visible[i] = true, true
	}
}

func TestAIStrategyChoosesAdvantageRegardlessOfController(t *testing.T) {
	for _, weak := range []int{1, 3} {
		w := strategyWorld()
		strong := 4 - weak
		center := w.entities(strong, "town_center")[0].Position
		w.spawn("castle", strong, Vec{center.X + 4, center.Y})
		for i := range 6 {
			w.spawn("knight", strong, Vec{center.X, center.Y + 3 + float64(i)})
		}
		revealStrategyWorld(w, 2)
		w.thinkPlayer(2)
		p := w.Players[2]
		if p.strategy.State() != aiRaiding || p.AIPlan.TargetOwner != weak {
			t.Fatalf("weak kingdom %d should be chosen equally whether human or AI: state %s, target %d", weak, p.strategy.State(), p.AIPlan.TargetOwner)
		}
		// A player seat's controller is not part of the opportunity calculation.
		before := w.aiObserve(2).Opportunity
		w.Players[1].AI, w.Players[3].AI = true, false
		after := w.aiObserve(2).Opportunity
		if !reflect.DeepEqual(before, after) {
			t.Fatal("changing human/AI control changed the strategic value of a target")
		}
	}
}

func TestAICivilizationsFightEachOtherInsteadOfTargetingTheHuman(t *testing.T) {
	w := strategyWorld()
	human := w.entities(1, "town_center")[0]
	opponent := w.entities(3, "town_center")[0]
	w.spawn("castle", 1, Vec{19, 40})
	for i := range 6 {
		w.spawn("knight", 1, Vec{15, 43 + float64(i)}).Stance = "stand_ground"
	}
	revealStrategyWorld(w, 2)
	w.thinkPlayer(2)
	for range 1400 {
		w.Update()
		if opponent.HP < w.stats(opponent).HP {
			if human.HP != w.stats(human).HP {
				t.Fatal("the human was attacked despite the better AI target")
			}
			return
		}
	}
	t.Fatalf("AI army never damaged its chosen AI opponent: strategy %s, target %d", w.Players[2].strategy.State(), w.Players[2].AIPlan.TargetOwner)
}

func TestSixSettlementOpeningProducesConflictBetweenAIKingdoms(t *testing.T) {
	w := New(Config{Difficulty: "aggressive", Settlements: 6, Seed: 4817})
	cursor := w.NextEvent
	for range 12000 {
		w.Update()
		for _, record := range w.JournalSince(cursor) {
			event := record.Event
			source, target := w.Entities[event.EntityID], w.Entities[event.TargetID]
			if event.Kind == "attack" && source != nil && source.Owner > 1 {
				if target != nil && target.Owner > 1 && target.Owner != source.Owner {
					t.Logf("Kingdom %d attacked kingdom %d after %.1f game seconds", source.Owner, target.Owner, w.Time)
					return
				}
			}
		}
		cursor = w.NextEvent
	}
	t.Fatal("AI kingdoms never fought each other in the unmodified six-settlement opening")
}

func TestAIProtectsHomeAndWorkersBeforeContinuingARaid(t *testing.T) {
	w := strategyWorld()
	revealStrategyWorld(w, 2)
	w.thinkPlayer(2)
	if w.Players[2].strategy.State() != aiRaiding {
		t.Fatal("expected an initial profitable expedition")
	}
	worker := w.entities(2, "villager")[0]
	invader := w.spawn("knight", 3, Vec{worker.Position.X + 3, worker.Position.Y})
	w.thinkPlayer(2)
	if w.Players[2].strategy.State() == aiDefending {
		t.Fatal("passing troops must not be treated as an attack")
	}
	w.hitFrom(worker, invader.Owner, invader.ID, 1)
	w.thinkPlayer(2)
	p := w.Players[2]
	if p.strategy.State() != aiDefending || p.AIPlan.TargetOwner != 0 {
		t.Fatal("a threat to the economy did not interrupt the raid")
	}
	for _, e := range w.entities(2, "militia") {
		if e.Order.Kind != "attack" || e.Order.Target != invader.ID {
			t.Fatal("army was not recalled to the local attacker")
		}
	}
	if worker.Order.Kind != "move" || worker.Order.Position.Distance(invader.Position) <= worker.Position.Distance(invader.Position) {
		t.Fatal("endangered worker did not retreat away from the attacker")
	}
	w.remove(invader.ID)
	w.thinkPlayer(2)
	if p.strategy.State() != aiRecovering || p.AIPlan.RecoverUntil <= w.Time {
		t.Fatal("settlement should recover after the defense before another expedition")
	}
}

func TestAIWithdrawsFromLossesAndHonorsRecoveryWithoutRepeatingOrders(t *testing.T) {
	w := strategyWorld()
	revealStrategyWorld(w, 2)
	w.thinkPlayer(2)
	army := w.entities(2, "militia")
	for _, e := range army[:5] {
		w.remove(e.ID)
	}
	for _, e := range army[5:] {
		e.Position = Vec{25, 42}
	}
	w.thinkPlayer(2)
	p := w.Players[2]
	if p.strategy.State() != aiRecovering {
		t.Fatal("losing the majority of an army should end its raid")
	}
	for _, e := range army[5:] {
		if e.Order.Kind != "move" || e.Stance != "passive" || e.Order.Position.Distance(Vec{40, 40}) > 7 {
			t.Fatal("survivors must withdraw to their own settlement")
		}
	}
	deadline, cursor := p.AIPlan.RecoverUntil, w.NextEvent
	w.thinkPlayer(2)
	if p.strategy.State() != aiRecovering || p.AIPlan.RecoverUntil != deadline || w.NextEvent != cursor {
		t.Fatal("repeated assessments extended recovery or reissued identical orders")
	}
	w.Time = deadline + 1
	w.thinkPlayer(2)
	if p.strategy.State() == aiRecovering {
		t.Fatal("recovery should finish once the cooldown has elapsed")
	}
}

func TestAIWithdrawsWhenScoutingRevealsOverwhelmingDefenders(t *testing.T) {
	w := strategyWorld()
	revealStrategyWorld(w, 2)
	w.thinkPlayer(2)
	plan := w.Players[2].AIPlan
	for i, e := range w.entities(2, "militia") {
		e.Position = Vec{plan.Goal.X, plan.Goal.Y + float64(i)*.6}
	}
	for i := range 12 {
		w.spawn("knight", plan.TargetOwner, Vec{plan.Goal.X + 3, plan.Goal.Y + float64(i)*.5})
	}
	w.thinkPlayer(2)
	if w.Players[2].strategy.State() != aiRecovering {
		t.Fatal("newly observed superior defenders should end an expedition before its army is destroyed")
	}
}

func TestAIRejectsUnfavorableBattlesAndInvestsInEconomy(t *testing.T) {
	w := strategyWorld()
	p := w.Players[2]
	p.Resources = Resources{Food: 1000, Wood: 1000, Gold: 1000, Stone: 1000}
	for _, owner := range []int{1, 3} {
		center := w.entities(owner, "town_center")[0].Position
		for i := range 6 {
			w.spawn("castle", owner, Vec{center.X + float64(i)*2, center.Y + 4})
		}
	}
	revealStrategyWorld(w, 2)
	w.thinkPlayer(2)
	if p.strategy.State() != aiDeveloping || p.AIPlan.TargetOwner != 0 {
		t.Fatal("AI committed its army to an overwhelmingly stronger kingdom")
	}
	tc := w.entities(2, "town_center")[0]
	if len(tc.Tasks) != 1 || tc.Tasks[0].Product != "villager" || p.Resources.Food != 950 {
		t.Fatal("an unprofitable war should leave the economy developing through paid commands")
	}
}

func TestAIStrategyCannotSeeHiddenArmiesOrStartingLocations(t *testing.T) {
	w := strategyWorld()
	p := w.Players[2]
	before := w.aiObserve(2)
	if len(before.Contacts) != 0 || before.Opportunity != nil {
		t.Fatal("unscouted opponents should not be known")
	}
	w.Players[1].Start = Vec{30, 30}
	w.Players[1].Resources = Resources{Food: 999999, Wood: 999999}
	for i := range 30 {
		w.spawn("knight", 1, Vec{15 + float64(i%5), 35 + float64(i/5)})
	}
	after := w.aiObserve(2)
	if !reflect.DeepEqual(before.Contacts, after.Contacts) || !reflect.DeepEqual(before.Opportunity, after.Opportunity) || after.Threat != nil {
		t.Fatal("hidden changes leaked into the strategic assessment")
	}
	// Remember a previously seen building, then destroy it outside vision.
	target := w.entities(3, "town_center")[0]
	p.Memory[target.ID] = w.entityView(target, 2)
	known := w.aiObserve(2).Opportunity
	w.remove(target.ID)
	if known == nil || !reflect.DeepEqual(known, w.aiObserve(2).Opportunity) {
		t.Fatal("AI learned about a hidden destruction before scouting the site")
	}
}

func TestAIStrategyGuardsAndInvalidEventsHaveNoEffects(t *testing.T) {
	w := strategyWorld()
	revealStrategyWorld(w, 2)
	c := w.aiObserve(2)
	before, _ := w.Checkpoint()
	for _, guard := range []func(context.Context, *aiContext) error{aiHomeThreatened, aiCampaignUnprofitable, aiRecoveryPending, aiProfitableRaid} {
		_ = guard(context.Background(), c)
	}
	if err := fire(c.Player.strategy, aiEvent("invalid"), c); !errors.Is(err, statemachine.ErrNotPermitted) {
		t.Fatal("strategy accepted an invalid event")
	}
	after, _ := w.Checkpoint()
	if !bytes.Equal(before, after) {
		t.Fatal("guard evaluation or rejected event mutated the world")
	}
}

func TestAISafeExpansionUsesPaidConstruction(t *testing.T) {
	w := strategyWorld()
	p := w.Players[2]
	p.Age, p.Resources = 2, Resources{Wood: 1000, Stone: 1000}
	for i := 1; i < difficultyPolicy(w.Config.Difficulty).Workers; i++ {
		w.spawn("villager", 2, Vec{40 + float64(i%5), 47 + float64(i/5)})
	}
	w.resource("gold", Vec{40, 68})
	revealStrategyWorld(w, 2)
	w.aiExpand(w.aiObserve(2))
	centers := w.entities(2, "town_center")
	if len(centers) != 2 || centers[1].life.State() != Foundation || centers[1].Position.Distance(centers[0].Position) < 20 {
		t.Fatal("a developed economy did not expand to a known safe deposit")
	}
	if p.Resources.Wood != 725 || p.Resources.Stone != 900 {
		t.Fatal("expansion did not pay the normal Town Center cost")
	}
	w.aiExpand(w.aiObserve(2))
	if len(w.entities(2, "town_center")) != 2 || p.Resources.Wood != 725 {
		t.Fatal("an expansion under construction was duplicated")
	}
}

func TestCheckpointPreservesRaidAndRecoveryStrategies(t *testing.T) {
	w := strategyWorld()
	revealStrategyWorld(w, 2)
	w.thinkPlayer(2)
	for _, state := range []aiState{aiRaiding, aiRecovering} {
		if state == aiRecovering {
			for _, e := range w.entities(2, "militia")[:5] {
				w.remove(e.ID)
			}
			w.thinkPlayer(2)
		}
		data, _ := w.Checkpoint()
		restored, err := Restore(data, w.JournalSince(0))
		if err != nil {
			t.Fatal(err)
		}
		if restored.Players[2].strategy.State() != state || !reflect.DeepEqual(restored.Players[2].AIPlan, w.Players[2].AIPlan) {
			t.Fatal("checkpoint lost the active strategy, army, target or deadline")
		}
		for range 80 {
			w.Update()
			restored.Update()
		}
		a, _ := w.Checkpoint()
		b, _ := restored.Checkpoint()
		if !bytes.Equal(a, b) || !reflect.DeepEqual(w.JournalSince(0), restored.JournalSince(0)) {
			t.Fatalf("%s strategy diverged after resume", state)
		}
	}
}

func TestCheckpointMigratesEarlierGamesToIndependentStrategies(t *testing.T) {
	w := New(Config{Difficulty: "expert"})
	data, _ := w.Checkpoint()
	var old map[string]any
	if err := json.Unmarshal(data, &old); err != nil {
		t.Fatal(err)
	}
	old["Version"] = 1
	delete(old, "Strategies")
	for _, p := range old["World"].(map[string]any)["Players"].(map[string]any) {
		delete(p.(map[string]any), "AIPlan")
	}
	data, _ = json.Marshal(old)
	restored, err := Restore(data, w.JournalSince(0))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(w.View(1), restored.View(1)) || restored.Players[2].strategy.State() != aiDeveloping {
		t.Fatal("migration changed gameplay state or failed to initialize strategy")
	}
	stepWorld(restored, 100)
}
