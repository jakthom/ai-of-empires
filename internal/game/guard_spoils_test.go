package game

import (
	"bytes"
	"context"
	"testing"
)

func TestGuardFollowsDefendsAndReturnsWithoutLosingItsOrder(t *testing.T) {
	w, _, _, cart := marketplaceFixture(t)
	guard := w.spawn("knight", 1, Vec{17, 20})
	raider := w.spawn("militia", 3, Vec{15, 21})
	raider.HP = 6
	w.refreshVisibility()
	if err := w.Apply(1, Command{Kind: "guard", EntityIDs: []int{guard.ID}, TargetID: cart.ID}); err != nil {
		t.Fatal(err)
	}
	stepWorld(w, 20)
	if raider.HP != 6 || guard.behavior.State() != Guarding {
		t.Fatal("a peaceful passerby is not an escort target")
	}
	if err := w.Apply(3, Command{Kind: "attack", EntityIDs: []int{raider.ID}, TargetID: cart.ID}); err != nil {
		t.Fatal(err)
	}
	stepWorld(w, 160)
	if w.Entities[raider.ID] != nil || w.Entities[cart.ID] == nil || guard.behavior.State() != Guarding || guard.Order.Target != cart.ID || guard.Order.Threat != 0 || len(guard.Orders) != 0 {
		t.Fatalf("escort did not protect and return: %s %+v", guard.behavior.State(), guard.Order)
	}
	goal := Vec{32, 25}
	if err := w.Apply(1, Command{Kind: "move", EntityIDs: []int{cart.ID}, Position: &goal}); err != nil {
		t.Fatal(err)
	}
	stepWorld(w, 400)
	if guard.Position.Distance(cart.Position) > 4 {
		t.Fatal("guard did not follow its moving charge")
	}
	if err := w.Apply(1, Command{Kind: "stop", EntityIDs: []int{guard.ID}}); err != nil {
		t.Fatal(err)
	}
	if guard.Order.Kind != "idle" || guard.behavior.State() != Idle {
		t.Fatal("stop did not interrupt the escort")
	}
}

func TestGuardLeashLossPrivacyAndCheckpoint(t *testing.T) {
	w, _, _, cart := marketplaceFixture(t)
	guard := w.spawn("knight", 1, Vec{16, 20})
	raider := w.spawn("knight", 3, Vec{17, 21})
	raider.Stance = "passive"
	w.refreshVisibility()
	if err := w.Apply(1, Command{Kind: "guard", EntityIDs: []int{guard.ID}, TargetID: cart.ID}); err != nil {
		t.Fatal(err)
	}
	w.noteAggression(raider.ID, 3, cart)
	stepWorld(w, 3)
	if guard.Order.Threat != raider.ID {
		t.Fatal("escort failed to engage observed attacker")
	}
	if w.entityView(guard, 1).GuardTarget != cart.ID || w.entityView(guard, 2).GuardTarget != 0 {
		t.Fatal("guard order privacy")
	}
	before, _ := w.Checkpoint()
	w.guardThreat(guard)
	_ = w.guardTarget(guard, cart)
	_ = guardCombatEnded(context.Background(), &unitContext{World: w, Actor: guard, Target: raider})
	after, _ := w.Checkpoint()
	if !bytes.Equal(before, after) {
		t.Fatal("guard evaluation mutated the world")
	}
	restored, err := Restore(before, w.JournalSince(0))
	if err != nil {
		t.Fatal(err)
	}
	stepWorld(w, 60)
	stepWorld(restored, 60)
	a, _ := w.Checkpoint()
	b, _ := restored.Checkpoint()
	if !bytes.Equal(a, b) {
		t.Fatal("active escort diverged after restore")
	}
	raider.Position = Vec{60, 60}
	stepWorld(w, 2)
	if guard.behavior.State() != Guarding || guard.Order.Threat != 0 {
		t.Fatal("guard chased outside its leash")
	}
	w.remove(cart.ID)
	stepWorld(w, 1)
	if guard.behavior.State() != Idle || guard.Order.Target != 0 {
		t.Fatal("lost charge left a dangling escort")
	}
}

func TestGuardInvalidSelectionsAndNeutralSupplyDefense(t *testing.T) {
	w, _, _, cart := marketplaceFixture(t)
	guard := w.spawn("knight", 1, Vec{16, 20})
	worker := w.spawn("villager", 1, Vec{17, 20})
	ship := w.spawn("galley", 1, Vec{18, 20})
	w.refreshVisibility()
	for _, cmd := range []Command{{Kind: "guard", EntityIDs: []int{worker.ID}, TargetID: cart.ID}, {Kind: "guard", EntityIDs: []int{guard.ID, worker.ID}, TargetID: cart.ID}, {Kind: "guard", EntityIDs: []int{ship.ID}, TargetID: cart.ID}, {Kind: "guard", EntityIDs: []int{guard.ID}, TargetID: guard.ID}} {
		if err := w.Apply(1, cmd); err == nil {
			t.Fatal("invalid escort accepted")
		}
		if guard.Order.Kind != "idle" {
			t.Fatal("mixed selection partially changed orders")
		}
	}
	supply := w.spawn("supply_cart", 0, Vec{17, 22})
	raider := w.spawn("militia", 3, Vec{18, 22})
	raider.HP = 6
	w.refreshVisibility()
	if err := w.Apply(1, Command{Kind: "guard", EntityIDs: []int{guard.ID}, TargetID: supply.ID}); err != nil {
		t.Fatal(err)
	}
	if err := w.Apply(3, Command{Kind: "attack", EntityIDs: []int{raider.ID}, TargetID: supply.ID}); err != nil {
		t.Fatal(err)
	}
	stepWorld(w, 180)
	if w.Entities[raider.ID] != nil || w.Entities[supply.ID] == nil {
		t.Fatal("guard did not protect neutral commerce")
	}
}

func TestSpoilsCaptureOnlyCarriedCargoExactlyOnce(t *testing.T) {
	for _, returning := range []bool{false, true} {
		t.Run(map[bool]string{false: "payment_outbound", true: "goods_returning"}[returning], func(t *testing.T) {
			w, _, partner, cart := marketplaceFixture(t)
			offer := postTestOffer(t, w, partner, 1)
			s := acceptTestOffer(t, w, offer, cart, false)
			if returning {
				untilTrade(t, w, func() bool { return s.lifecycle.State() == shipmentReturning })
			}
			before := w.Players[3].Resources
			cargo := resourceAmount(cart.CargoType, cart.Cargo)
			raider := w.spawn("knight", 3, Vec{16, 20})
			w.hitFrom(cart, 3, raider.ID, 10000)
			if w.Players[3].Resources != func() Resources { r := before; r.Add(cargo); return r }() {
				t.Fatal("attacker did not capture carried cargo")
			}
			w.hitFrom(cart, 3, raider.ID, 10000)
			stepWorld(w, 1)
			if s.lifecycle.State() != shipmentLost {
				t.Fatal("plundered shipment did not settle loss")
			}
			if w.Players[3].Production.Pending != (Resources{}) {
				t.Fatal("spoils counted as production")
			}
			if returning && w.Players[2].Resources.Stone != 1100 || !returning && w.Players[2].Resources.Wood != 1000 {
				t.Fatal("spoils stole or duplicated destination reserves")
			}
			data, _ := w.Checkpoint()
			copy, err := Restore(data, w.JournalSince(0))
			if err != nil {
				t.Fatal(err)
			}
			stepWorld(copy, 2)
			if copy.Players[3].Resources != w.Players[3].Resources {
				t.Fatal("restore duplicated spoils")
			}
		})
	}
}

func TestSpoilsDoNotLootTreasuriesOrVoluntaryDeletion(t *testing.T) {
	w, home, _, cart := marketplaceFixture(t)
	before := w.Players[3].Resources
	w.hit(home, 3, 10000)
	if w.Players[3].Resources != before {
		t.Fatal("destroying an empty building raided a treasury")
	}
	cart.CargoType, cart.Cargo = "gold", 70
	if err := w.Apply(1, Command{Kind: "delete", EntityIDs: []int{cart.ID}}); err != nil {
		t.Fatal(err)
	}
	if w.Players[3].Resources != before {
		t.Fatal("deletion awarded spoils")
	}
	worker := w.spawn("villager", 1, Vec{18, 20})
	worker.CargoType, worker.Cargo = "wood", 9
	w.hit(worker, 3, 10000)
	if w.Players[3].Resources.Wood != before.Wood+9 {
		t.Fatal("worker cargo was not captured")
	}
}

func TestSupplyPlunderCannotAlsoReplenishMarket(t *testing.T) {
	w, _, _, _ := marketplaceFixture(t)
	r := w.Marketplace.Regions[-1]
	r.NextSupply = 0
	w.pulseMerchantSupplies()
	if r.Cart == 0 {
		t.Fatal("supply did not launch")
	}
	stock, cargo, before := r.Stock, r.Cargo, w.Players[3].Resources
	cart := w.Entities[r.Cart]
	w.hit(cart, 3, 10000)
	w.pulseMerchantSupplies()
	got := before
	got.Add(cargo)
	if w.Players[3].Resources != got || r.Stock != stock || r.Cargo != (Resources{}) || r.Cart != 0 || r.Losses != 1 || r.lifecycle.State() != supplyWaiting {
		t.Fatal("supply plunder did not consume the single cargo authority")
	}
}

func TestDamageStagesAndObservedAftermathFollowGameClock(t *testing.T) {
	w, home, _, _ := marketplaceFixture(t)
	if w.entityView(home, 1).DamageStage != 0 {
		t.Fatal("healthy building damaged")
	}
	w.hit(home, 3, home.HP*.6)
	if w.entityView(home, 2).DamageStage != 2 {
		t.Fatal("observed building has no visible damage stage")
	}
	w.hit(home, 3, 10000)
	if len(w.View(1).Effects) != 1 {
		t.Fatal("authoritative destruction has no remains")
	}
	data, _ := w.Checkpoint()
	copy, err := Restore(data, w.JournalSince(0))
	if err != nil {
		t.Fatal(err)
	}
	if err := copy.Apply(1, Command{Kind: "pause"}); err != nil {
		t.Fatal(err)
	}
	stepWorld(copy, 300)
	if len(copy.Aftermath) != 1 {
		t.Fatal("paused remains expired")
	}
	copy.Apply(1, Command{Kind: "pause"})
	stepWorld(copy, 241)
	if len(copy.Aftermath) != 0 {
		t.Fatal("expired remains linger")
	}
	w.Config.World.Reveal = "normal"
	clear(w.Players[2].Visible)
	if len(w.View(2).Effects) != 0 {
		t.Fatal("hidden destruction leaked through effects")
	}
}

func TestAIEscortsStayAssignedAndVisibleDefendersDeterRaids(t *testing.T) {
	w, _, partner, cart := marketplaceFixture(t)
	acceptTestOffer(t, w, postTestOffer(t, w, partner, 1), cart, false)
	guards := []*Entity{w.spawn("knight", 1, Vec{15, 20}), w.spawn("knight", 1, Vec{16, 20}), w.spawn("knight", 1, Vec{17, 20})}
	for _, guard := range guards {
		guard.Stance = "passive" // Idle troops may have returned from a retreat.
	}
	w.refreshVisibility()
	c := w.aiObserve(1)
	w.aiTradeEscorts(c)
	// The same assessment frame still contains the newly assigned escorts.
	// Neither scouting nor forced rallying may immediately replace the order.
	w.aiScout(c)
	aiRally(c, true)
	for _, guard := range guards[:2] {
		if guard.Order.Kind != "guard" || guard.Order.Target != cart.ID || guard.Stance != "defensive" {
			t.Fatal("AI overwrote its escort assignment")
		}
	}
	if guards[2].Order.Kind == "guard" {
		t.Fatal("one delivery consumed more than two idle escorts")
	}
	for _, unit := range w.aiObserve(1).Army {
		if unit.Order.Kind == "guard" {
			t.Fatal("escort entered the raid roster")
		}
	}
	w.Players[3].Temperament = aiExpansionist
	raid := w.aiObserve(3)
	raid.Policy, raid.Power = difficultyPolicy("standard"), 20
	v := w.entityView(cart, 3)
	raid.Contacts = []aiContact{{v.ID, v.Owner, v.Type, v.Position, v.HP, v.MaxHP, v.Progress, true}}
	if target := aiChooseOpportunity(raid); target == nil || target.ID != cart.ID {
		t.Fatal("unprotected visible commerce was not considered")
	}
	for _, guard := range guards[:2] {
		v := w.entityView(guard, 3)
		raid.Contacts = append(raid.Contacts, aiContact{v.ID, v.Owner, v.Type, v.Position, v.HP, v.MaxHP, v.Progress, true})
	}
	if aiChooseOpportunity(raid) != nil {
		t.Fatal("outmatched raiders ignored the visible escort")
	}
}
