package game

import (
	"math"
	"reflect"
	"testing"
)

func TestPopulationPeakIncludesUnitsLostBetweenChartSamples(t *testing.T) {
	w := trafficWorld()
	w.Players[1].Economy.PeakPopulation = 0
	unit := w.spawn("militia", 1, Vec{20, 20})
	w.hit(unit, 0, 10000)
	if got := w.kingdomStatistics(w.Players[1]); got.Population != 0 || got.PeakPopulation != 1 {
		t.Fatalf("short-lived population missing: current %d, peak %d", got.Population, got.PeakPopulation)
	}
}

func TestAttackIntentDeclaresWarAndNeutralTradeProtectsParticipants(t *testing.T) {
	w, _, partner, _ := marketplaceFixture(t)
	soldier := w.spawn("militia", 1, Vec{20, 20})
	for _, typ := range []string{"villager", "trade_cart", "market"} {
		target := w.spawn(typ, 2, Vec{25, 20})
		w.refreshVisibility()
		order(t, w, 1, Command{Kind: "attack", EntityIDs: []int{soldier.ID}, TargetID: target.ID})
		if w.relation(1, 2) != inConflict || target.HP != w.stats(target).HP {
			t.Fatal("attack order must declare war before damage")
		}
		if w.relation(1, 3) != atPeace {
			t.Fatal("unrelated kingdom pulled into conflict")
		}
	}
	neutral := w.spawn("market", 0, Vec{22, 28})
	guard := w.spawn("militia", 3, Vec{24, 28})
	guard.Stance = "passive"
	w.refreshVisibility()
	order(t, w, 3, Command{Kind: "guard", EntityIDs: []int{guard.ID}, TargetID: neutral.ID})
	order(t, w, 1, Command{Kind: "interact", EntityIDs: []int{soldier.ID}, TargetID: neutral.ID})
	if w.relation(1, 3) != inConflict {
		t.Fatal("neutral market's protector did not enter conflict")
	}
	stepWorld(w, 400)
	if neutral.HP >= w.stats(neutral).HP {
		t.Fatal("neutral market attack did not execute", soldier.Position, soldier.behavior.State(), soldier.Order, soldier.HP)
	}
	before := w.Players[1].Resources.Gold
	if err := w.Apply(1, Command{Kind: "peace_offer", TargetPlayer: 2, Value: 50}); err != nil {
		t.Fatal(err)
	}
	id := w.NextID - 1
	if err := w.Apply(3, Command{Kind: "peace_accept", OfferID: id}); err == nil {
		t.Fatal("third party accepted private offer")
	}
	if w.Players[1].Resources.Gold != before-50 || partner.HP != w.stats(partner).HP {
		t.Fatal("invalid acceptance changed accounting")
	}
	order(t, w, 1, Command{Kind: "peace_offer", TargetPlayer: 3, Value: 50})
	peace := w.NextID - 1
	order(t, w, 3, Command{Kind: "peace_accept", OfferID: peace})
	if soldier.Order.Kind != "idle" || guard.Order.Kind != "guard" {
		t.Fatal("peace did not recall neutral-site attack or retain its escort")
	}
	stepWorld(w, 20)
	if w.relation(1, 3) != atPeace {
		t.Fatal("ongoing neutral attack broke accepted peace")
	}
}

func TestPeacePaymentRestoresWithoutReplayAndRecallsAttacks(t *testing.T) {
	w, _, partner, _ := marketplaceFixture(t)
	a := w.spawn("archer", 1, Vec{25, 20})
	w.refreshVisibility()
	order(t, w, 1, Command{Kind: "attack", EntityIDs: []int{a.ID}, TargetID: partner.ID})
	// Release a real projectile, then restore a checkpoint with an escrowed offer.
	for i := 0; i < 400 && len(w.Projectiles) == 0; i++ {
		w.Update()
	}
	if len(w.Projectiles) == 0 {
		t.Fatal("archer did not release projectile")
	}
	order(t, w, 1, Command{Kind: "peace_offer", TargetPlayer: 2, Value: 100})
	id := w.NextID - 1
	before := w.Players[1].Resources.Gold
	data, err := w.Checkpoint()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := Restore(data, w.JournalSince(0))
	if err != nil {
		t.Fatal(err)
	}
	if restored.Players[1].Resources.Gold != before || restored.Reparations[id].lifecycle.State() != peaceOffered {
		t.Fatal("restore replayed escrow")
	}
	w = restored
	recipient := w.Players[2].Resources.Gold
	order(t, w, 2, Command{Kind: "peace_accept", OfferID: id})
	if w.relation(1, 2) != atPeace || w.Players[2].Resources.Gold != recipient+100 || w.Players[1].Economy.Consumption["reparations"].Gold != 100 {
		t.Fatal("accepted payment was not committed once")
	}
	if w.Entities[a.ID].Order.Kind != "idle" || w.Projectiles[0].flight.State() != Impacted {
		t.Fatal("peace left an attack in flight")
	}
	if err := w.Apply(2, Command{Kind: "peace_accept", OfferID: id}); err == nil {
		t.Fatal("accepted offer accepted twice")
	}
	if w.Players[2].Resources.Gold != recipient+100 {
		t.Fatal("duplicate payment")
	}
	stepWorld(w, 1)
	if w.relation(1, 2) != atPeace {
		t.Fatal("cancelled projectile broke peace")
	}
	order(t, w, 1, Command{Kind: "attack", EntityIDs: []int{a.ID}, TargetID: partner.ID})
	if w.relation(1, 2) != inConflict || w.Relations[relationKey(1, 2)].PurchasedUntil != 0 {
		t.Fatal("new attack did not break paid peace")
	}
}

func TestPeaceOfferRefundsExpiryDeclineWithdrawalAndAIThreshold(t *testing.T) {
	for _, action := range []string{"peace_decline", "peace_withdraw", "expire", "ai_decline", "ai_accept"} {
		t.Run(action, func(t *testing.T) {
			w, _, target, _ := marketplaceFixture(t)
			w.declareAttack(1, target)
			w.hit(target, 1, 100)
			price := w.peacePrice(1, 2)
			amount := price
			if action == "ai_decline" {
				amount = 1
			}
			before := w.Players[1].Resources.Gold
			other := w.Players[2].Resources.Gold
			order(t, w, 1, Command{Kind: "peace_offer", TargetPlayer: 2, Value: amount})
			id := w.NextID - 1
			switch action {
			case "expire":
				w.Time += 121
				w.pulseReparations()
			case "ai_accept", "ai_decline":
				w.Players[2].AI = true
				w.pulseReparations()
			case "peace_withdraw":
				order(t, w, 1, Command{Kind: action, OfferID: id})
			default:
				order(t, w, 2, Command{Kind: action, OfferID: id})
			}
			w.pulseReparations()
			if action == "ai_accept" {
				if w.Players[1].Resources.Gold != before-amount || w.Players[2].Resources.Gold != other+amount || w.relation(1, 2) != atPeace {
					t.Fatal("funded AI peace failed")
				}
			} else if w.Players[1].Resources.Gold != before || w.Players[2].Resources.Gold != other || w.relation(1, 2) != inConflict {
				t.Fatal("refund changed peace or duplicated money")
			}
		})
	}
}

func TestGlobalOrderBookExcludesPrivateDemandAndUpdatesReservation(t *testing.T) {
	w, home, partner, cart := marketplaceFixture(t)
	post := func(m *Entity, give string, ga int, want string, wa, lots, private int) {
		order(t, w, m.Owner, Command{Kind: "market_post", EntityIDs: []int{m.ID}, Offer: &TradeOfferIntent{GiveResource: give, GiveAmount: ga, WantResource: want, WantAmount: wa, Lots: lots, TargetPlayer: private}})
	}
	post(partner, "wood", 100, "gold", 80, 3, 0)
	post(home, "gold", 70, "wood", 100, 2, 0)
	post(partner, "gold", 100, "wood", 50, 4, 1)
	find := func() GlobalCommodityView {
		for _, g := range w.View(3).Marketplace.Global {
			if g.Resource == "wood" {
				return g
			}
		}
		t.Fatal("wood missing")
		return GlobalCommodityView{}
	}
	g := find()
	if g.Demand != 200 || g.Supply != 300 || g.BestBid != 70 || g.BestAsk != 80 {
		t.Fatal("private offer leaked or global quantities wrong", g)
	}
	order(t, w, 1, Command{Kind: "market_accept", OfferID: 1, EntityIDs: []int{cart.ID}})
	if find().Supply != 200 {
		t.Fatal("reserved lot remained available globally")
	}
	order(t, w, 2, Command{Kind: "market_cancel", OfferID: 1})
	if find().Supply != 0 {
		t.Fatal("cancelled supply remained listed")
	}
}

func TestStatisticsAccountingAndObserverDoNotChangePlayerKnowledge(t *testing.T) {
	w, _, _, _ := marketplaceFixture(t)
	w.Config.World.Reveal = "hidden"
	for _, p := range w.Players {
		for i := range p.Explored {
			p.Explored[i] = false
		}
		p.Memory = map[int]EntityView{}
	}
	w.refreshVisibility()
	before := w.View(1)
	explored := append([]bool(nil), w.Players[1].Explored...)
	observer := w.ObserverView(1)
	if !observer.GodMode || len(observer.Entities) <= len(before.Entities) {
		t.Fatal("observer did not reveal world")
	}
	if !reflect.DeepEqual(explored, w.Players[1].Explored) || !reflect.DeepEqual(before, w.View(1)) {
		t.Fatal("observer mutated player knowledge")
	}
	w.receiveProduction(1, "wood", 100)
	w.receiveProduction(1, "stone", 10)
	w.consume(1, Resources{Wood: 25}, "construction")
	w.refund(1, Resources{Wood: 5})
	for range 1200 {
		w.Update()
	}
	r := w.Statistics(1, false)
	if len(r.Kingdoms) != 1 || len(r.Awards) != 0 || r.Kingdoms[0].GDP != 113 || r.Kingdoms[0].Consumed.Wood != 25 || r.Kingdoms[0].Refunded.Wood != 5 {
		t.Fatal("accounting confused production, spending and refund", r.Kingdoms[0])
	}
	if len(r.Kingdoms[0].History) != 1 || len(r.Kingdoms[0].Production.History) != 12 {
		t.Fatal("game-time histories missing")
	}
	all := w.Statistics(1, true)
	if len(all.Awards) != 7 || len(all.Kingdoms) != 3 || !reflect.DeepEqual(all.Awards[0].Winners, []int{1}) {
		t.Fatal("world awards missing")
	}
	data, _ := w.Checkpoint()
	restored, err := Restore(data, w.JournalSince(0))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(all, restored.Statistics(1, true)) {
		t.Fatal("checkpoint changed statistical facts")
	}
	if math.Abs(r.Kingdoms[0].Food.DemandPerMinute-r.Kingdoms[0].Food.PerPersonMinute*float64(r.Kingdoms[0].Population)) > 1e-8 {
		t.Fatal("food demand not explained by population")
	}
}
