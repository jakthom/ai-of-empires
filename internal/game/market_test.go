package game

import "testing"

func TestMarketQuotesMatchPaidExchangeAndHistory(t *testing.T) {
	for _, civ := range []string{"britons", "saracens"} {
		for _, resource := range []string{"food", "wood", "stone"} {
			for _, kind := range []string{"market_sell", "market_buy"} {
				w := New(Config{Civilization: civ, Difficulty: "peaceful"})
				market := w.spawn("market", 1, Vec{22, 44})
				p := w.Players[1]
				p.Resources = Resources{500, 500, 500, 500}
				before := p.Resources
				var offer Action
				for _, e := range w.View(1).Entities {
					if e.ID == market.ID {
						for _, a := range e.Actions {
							if a.Kind == kind && a.Product == resource {
								offer = a
							}
						}
					}
				}
				if offer.Gain == nil || !offer.Enabled {
					t.Fatal("exchange quote was not projected")
				}
				expectedGain, expectedCost := Resources{}, Resources{}
				if kind == "market_buy" {
					expectedCost.Gold = 130
					expectedGain.Deposit(resource, 100)
				} else {
					expectedCost.Deposit(resource, 100)
					expectedGain.Gold = 70
					if civ == "saracens" {
						expectedGain.Gold = 84
					}
				}
				if offer.Cost != expectedCost || *offer.Gain != expectedGain {
					t.Fatal("incorrect server quote")
				}
				if err := w.Apply(1, Command{Kind: kind, EntityIDs: []int{market.ID}, Product: resource}); err != nil {
					t.Fatal(err)
				}
				before.Add(expectedCost.Scale(-1))
				before.Add(expectedGain)
				if p.Resources != before {
					t.Fatal("exchange diverged from quote")
				}
				count := 0
				for _, event := range w.journal.records {
					if event.EntityID == market.ID && event.Kind == "exchange" && event.Message == offer.Description {
						count++
					}
				}
				if count != 1 {
					t.Fatal("missing exact exchange receipt in market history")
				}
				log, err := w.Log(1, LogQuery{EntityID: market.ID, Category: "economy"})
				if err != nil || len(log.Events) != 1 || log.Events[0].Kind != "exchange" {
					t.Fatal("exchange missing from resource/trade filter")
				}
			}
		}
	}
}

func TestMarketRefusesUnaffordableInvalidAndUnfinishedExchanges(t *testing.T) {
	w := New(Config{Difficulty: "peaceful"})
	market := w.spawn("market", 1, Vec{22, 44})
	w.Players[1].Resources = Resources{}
	for _, command := range []Command{
		{Kind: "market_buy", EntityIDs: []int{market.ID}, Product: "wood"},
		{Kind: "market_sell", EntityIDs: []int{market.ID}, Product: "food"},
		{Kind: "market_sell", EntityIDs: []int{market.ID}, Product: "gold"},
		{Kind: "market_buy", EntityIDs: []int{w.entities(1, "town_center")[0].ID}, Product: "stone"},
	} {
		if err := w.Apply(1, command); err == nil || w.Players[1].Resources != (Resources{}) {
			t.Fatal("refused exchange was not atomic")
		}
	}
	unfinished := w.spawnWithLife("market", 1, Vec{25, 44}, Foundation)
	w.Players[1].Resources.Gold = 130
	if err := w.Apply(1, Command{Kind: "market_buy", EntityIDs: []int{unfinished.ID}, Product: "wood"}); err == nil || w.Players[1].Resources.Gold != 130 {
		t.Fatal("unfinished market traded")
	}
}

func TestTradeCartLoopsAndWaitsWhenItsHomeMarketIsLost(t *testing.T) {
	w, home, _, cart := marketplaceFixture(t)
	target := w.spawn("market", 0, Vec{32, 30})
	w.refreshVisibility()
	if err := w.Apply(1, Command{Kind: "trade", EntityIDs: []int{cart.ID}, TargetID: target.ID, Product: "wood", Repeat: true}); err != nil {
		t.Fatal(err)
	}
	untilTrade(t, w, func() bool { return len(w.Marketplace.Shipments) >= 3 })
	if w.Players[1].Resources.Wood != 700 || w.Players[1].Resources.Gold <= 1000 {
		t.Fatal("route did not fund repeat trips")
	}
	w.remove(home.ID)
	for range 800 {
		w.Update()
	}
	if cart.Cargo == 0 || cart.behavior.State() != Caravanning {
		t.Fatal("lost home should retain cargo awaiting a replacement")
	}
	if err := w.Apply(1, Command{Kind: "trade", EntityIDs: []int{cart.ID}, TargetID: target.ID}); err == nil {
		t.Fatal("busy cart accepted another route")
	}
}

func TestTradeCartRefusesDisconnectedIslandMarket(t *testing.T) {
	w, err := NewWorld(Config{Settlements: 2, World: WorldOptions{Type: "islands", Reveal: "all"}})
	if err != nil {
		t.Fatal(err)
	}
	p := w.Players[1]
	w.spawn("market", 1, Vec{p.Start.X + 5, p.Start.Y + 3})
	cart := w.spawn("trade_cart", 1, p.Start)
	neutral := w.entities(0, "market")[0]
	if err := w.Apply(1, Command{Kind: "trade", EntityIDs: []int{cart.ID}, TargetID: neutral.ID}); err == nil || cart.behavior.State() != Idle {
		t.Fatal("trade cart accepted an unreachable overseas route")
	}
}
