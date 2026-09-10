package game

import (
	"bytes"
	"encoding/json"
	"math"
	"reflect"
	"testing"
)

func localMerchantFixture(t *testing.T) (*World, *Entity, *Entity, *Entity) {
	w, home, _, cart := marketplaceFixture(t)
	neutral := w.spawn("market", 0, Vec{32, 20})
	w.refreshVisibility()
	return w, home, neutral, cart
}
func merchantTotal(w *World) Resources {
	sum := Resources{}
	for _, p := range w.Players {
		sum.Add(p.Resources)
		sum.Food += p.Economy.Consumption["food_upkeep"].Food
	}
	for _, r := range w.Marketplace.Regions {
		sum.Add(r.Stock)
		sum.Add(r.Cargo)
	}
	for _, e := range w.Entities {
		sum.Deposit(e.CargoType, e.Cargo)
	}
	for _, s := range w.Marketplace.Shipments {
		if s.Merchant != 0 && s.lifecycle.State() == shipmentOutbound {
			sum.Deposit(s.Terms.GiveResource, float64(s.Terms.GiveAmount))
		}
	}
	sum.Food = math.Round(sum.Food*1e8) / 1e8
	return sum
}
func TestRegionalPricesAreLocalAndRebuildingCannotResetSupply(t *testing.T) {
	w, home, neutral, cart := localMerchantFixture(t)
	r := w.Marketplace.Regions[neutral.ID]
	r.Stock.Wood = 250
	local, _ := w.quoteExchange(w.Players[1], home, "market_sell", "wood")
	remote, err := w.merchantTradeQuote(1, neutral.ID, "wood", "sell", cart)
	if err != nil || remote.Gain.Gold <= local.Gain.Gold {
		t.Fatal("shortage did not create valuable export", err)
	}
	before := *r
	if err := w.Apply(1, Command{Kind: "market_buy", EntityIDs: []int{home.ID}, Product: "wood"}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, *r) {
		t.Fatal("home purchase changed remote prices")
	}
	stock := w.Marketplace.Regions[-1].Stock
	replacement := w.spawn("market", 1, Vec{12, 28})
	w.remove(home.ID)
	if w.Marketplace.Regions[-1].Stock != stock {
		t.Fatal("new building reset merchant stock")
	}
	q, _ := w.quoteExchange(w.Players[1], replacement, "market_buy", "wood")
	if q.Cost.Gold != 141 {
		t.Fatal("new market bypassed the home quote", q)
	}
	// A rich till must still be able to sell goods; only warehouses are capped.
	w.Marketplace.Regions[-1].Stock.Gold = 10000
	if err := w.Apply(1, Command{Kind: "market_buy", EntityIDs: []int{replacement.ID}, Product: "wood"}); err != nil {
		t.Fatal("full till froze commerce", err)
	}
}

func TestMerchantProductionUsesSelectedAndMixedBiomes(t *testing.T) {
	for _, biome := range []string{"tropical", "desert", "alpine", "mixed"} {
		w, err := NewWorld(Config{Seed: 4817, Settlements: 2, Difficulty: "peaceful", World: WorldOptions{Type: "plains", Biome: biome}})
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range w.Marketplace.Regions {
			found := false
			for _, economy := range biomeEconomies() {
				if r.Biome == economy.Biome {
					found = true
					if r.Output.Wood != 60*economy.Deposits.Wood || r.Output.Food != 60*economy.Deposits.Food || r.Stock.Stone != 1000*economy.Deposits.Stone {
						t.Fatal("merchant ignored regional production", biome, r)
					}
				}
			}
			if !found || biome != "mixed" && r.Biome != biome {
				t.Fatal("incorrect merchant biome", biome, r.Biome)
			}
		}
	}
}

func TestSupplyRespectsSpaceReservedForIncomingGoodsAndRefunds(t *testing.T) {
	w, _, neutral, cart := localMerchantFixture(t)
	r := w.Marketplace.Regions[neutral.ID]
	r.Stock.Wood = merchantCapacity
	if err := w.Apply(1, Command{Kind: "trade", EntityIDs: []int{cart.ID}, TargetID: neutral.ID, Product: "wood", TradeMode: "buy"}); err != nil {
		t.Fatal(err)
	}
	s := w.Marketplace.Shipments[w.Marketplace.NextShipment-1]
	if err := w.Apply(1, Command{Kind: "stop", EntityIDs: []int{cart.ID}}); err != nil {
		t.Fatal(err)
	}
	r.NextSupply = 0
	r.Demand = Resources{}
	untilTrade(t, w, func() bool { return r.Deliveries == 1 })
	if r.Stock.Wood != merchantCapacity-100 {
		t.Fatal("supply consumed refund space")
	}
	if err := w.Apply(1, Command{Kind: "market_recall", ShipmentID: s.ID}); err != nil {
		t.Fatal(err)
	}
	untilTrade(t, w, func() bool { return s.terminal() })
	if r.Stock.Wood != merchantCapacity || w.Players[1].Resources.Gold != 1000 {
		t.Fatal("refund overflowed stock or charged twice")
	}
	// Nor can supply overwrite the space reserved for an arriving export.
	r.Stock.Wood = merchantCapacity - 100
	if err := w.Apply(1, Command{Kind: "trade", EntityIDs: []int{cart.ID}, TargetID: neutral.ID, Product: "wood"}); err != nil {
		t.Fatal(err)
	}
	s = w.Marketplace.Shipments[w.Marketplace.NextShipment-1]
	_ = w.Apply(1, Command{Kind: "stop", EntityIDs: []int{cart.ID}})
	r.NextSupply = 0
	untilTrade(t, w, func() bool { return r.Deliveries == 2 })
	if r.Stock.Wood != merchantCapacity-100 {
		t.Fatal("supply occupied incoming cargo space")
	}
	_ = w.Apply(1, Command{Kind: "market_resume", ShipmentID: s.ID})
	untilTrade(t, w, func() bool { return s.terminal() })
	if r.Stock.Wood != merchantCapacity {
		t.Fatal("incoming shipment exceeded warehouse capacity")
	}
}

func TestMerchantFreightSettlesBothDirectionsAndSurvivesCheckpoint(t *testing.T) {
	for _, mode := range []string{"buy", "sell"} {
		t.Run(mode, func(t *testing.T) {
			w, _, neutral, cart := localMerchantFixture(t)
			before := merchantTotal(w)
			command := Command{Kind: "trade", EntityIDs: []int{cart.ID}, TargetID: neutral.ID, Product: "wood", TradeMode: mode}
			if err := w.Apply(1, command); err != nil {
				t.Fatal(err)
			}
			s := w.Marketplace.Shipments[w.Marketplace.NextShipment-1]
			if merchantTotal(w) != before {
				t.Fatal("dispatch created or destroyed goods")
			}
			if err := w.Apply(1, command); err == nil || merchantTotal(w) != before {
				t.Fatal("cart loaded twice")
			}
			untilTrade(t, w, func() bool { return s.lifecycle.State() == shipmentReturning })
			if merchantTotal(w) != before {
				t.Fatal("collection created or destroyed goods")
			}
			capture := w.CaptureCheckpoint()
			data, _ := capture.Encode()
			restored, err := Restore(data, w.JournalSince(0))
			if err != nil {
				t.Fatal(err)
			}
			for range 250 {
				w.Update()
				restored.Update()
			}
			a, _ := w.Checkpoint()
			b, _ := restored.Checkpoint()
			if !bytes.Equal(a, b) {
				t.Fatal("merchant freight diverged after restore")
			}
			if s.lifecycle.State() != shipmentDelivered || merchantTotal(w) != before {
				t.Fatal("delivery settled incorrectly")
			}
			if w.Players[1].Production.view().Rates.Gold != 0 {
				t.Fatal("commodity exchange counted as new gold production")
			}
		})
	}
}

func TestMerchantRepeatHonorsPriceLimitsAndDistanceDoesNotMintGold(t *testing.T) {
	w, _, neutral, cart := localMerchantFixture(t)
	other := w.spawn("market", 0, Vec{52, 30})
	w.refreshVisibility()
	a, _ := w.merchantTradeQuote(1, neutral.ID, "wood", "sell", cart)
	b, _ := w.merchantTradeQuote(1, other.ID, "wood", "sell", cart)
	if a.Gain != b.Gain {
		t.Fatal("distance changed the price of identical goods")
	}
	limit := int(a.Gain.Gold)
	if err := w.Apply(1, Command{Kind: "trade", EntityIDs: []int{cart.ID}, TargetID: neutral.ID, Product: "wood", Repeat: true, TradeLimit: &limit}); err != nil {
		t.Fatal(err)
	}
	s := w.Marketplace.Shipments[w.Marketplace.NextShipment-1]
	untilTrade(t, w, func() bool { return s.terminal() })
	if len(w.Marketplace.Shipments) != 1 || cart.behavior.State() != Idle {
		t.Fatal("repeat ignored worsened sale price")
	}
	for _, bad := range []Command{
		{Kind: "trade", EntityIDs: []int{cart.ID}, TargetID: neutral.ID, Product: "gold"},
		{Kind: "trade", EntityIDs: []int{cart.ID}, TargetID: neutral.ID, Product: "wood", TradeMode: "steal"},
		{Kind: "trade", EntityIDs: []int{cart.ID}, TargetID: neutral.ID, Product: "wood", TradeLimit: new(0)},
	} {
		before, _ := w.Checkpoint()
		err := w.apply(1, bad)
		after, _ := w.Checkpoint()
		if err == nil || !bytes.Equal(before, after) {
			t.Fatal("invalid intention mutated state")
		}
	}
}

func TestMerchantRecallAndLossReleaseOnlyReservedPayment(t *testing.T) {
	for _, recall := range []bool{false, true} {
		t.Run(map[bool]string{true: "recall", false: "loss"}[recall], func(t *testing.T) {
			w, _, neutral, cart := localMerchantFixture(t)
			r := w.Marketplace.Regions[neutral.ID]
			before := r.Stock
			if err := w.Apply(1, Command{Kind: "trade", EntityIDs: []int{cart.ID}, TargetID: neutral.ID, Product: "wood"}); err != nil {
				t.Fatal(err)
			}
			s := w.Marketplace.Shipments[w.Marketplace.NextShipment-1]
			untilTrade(t, w, func() bool { return cart.Position.X > 20 })
			if recall {
				if err := w.Apply(1, Command{Kind: "market_recall", ShipmentID: s.ID}); err != nil {
					t.Fatal(err)
				}
			} else {
				w.remove(cart.ID)
			}
			untilTrade(t, w, func() bool { return s.terminal() })
			if r.Stock != before || w.Players[1].Resources.Gold != 1000 {
				t.Fatal("refund created a sale or lost reserved merchant gold")
			}
			wood := 900.
			if recall {
				wood = 1000
			}
			if w.Players[1].Resources.Wood != wood {
				t.Fatal("cargo loss or recall settlement incorrect")
			}
		})
	}
}

func TestRegionalSupplyPhysicallyArrivesExactlyOnce(t *testing.T) {
	w, _, neutral, _ := localMerchantFixture(t)
	r := w.Marketplace.Regions[neutral.ID]
	r.NextSupply = 0
	before := r.Stock
	untilTrade(t, w, func() bool { return r.Cart != 0 })
	if r.Stock != before || r.Cargo.Wood == 0 {
		t.Fatal("dispatch teleported supplies")
	}
	capture := w.CaptureCheckpoint()
	data, _ := capture.Encode()
	restored, err := Restore(data, w.JournalSince(0))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 600 && r.Deliveries == 0; i++ {
		w.Update()
		restored.Update()
	}
	if r.Deliveries != 1 || r.Cart != 0 || r.Cargo != (Resources{}) || r.Stock.Gold <= before.Gold {
		t.Fatal("delivery failed to supply stock and satisfy demand")
	}
	a, _ := w.Checkpoint()
	b, _ := restored.Checkpoint()
	if !bytes.Equal(a, b) {
		t.Fatal("supply replayed or diverged on restore")
	}
	frozen, _ := capture.Encode()
	if !bytes.Equal(data, frozen) {
		t.Fatal("frozen supply capture mutated")
	}
	after := r.Stock
	for range 20 {
		w.Update()
	}
	if r.Stock != after || r.Deliveries != 1 {
		t.Fatal("arrival executed twice")
	}
}

func TestSupplyRaidingAndBlockedPathsDelayReplenishment(t *testing.T) {
	w, _, neutral, _ := localMerchantFixture(t)
	r := w.Marketplace.Regions[neutral.ID]
	r.NextSupply = 0
	untilTrade(t, w, func() bool { return r.Cart != 0 })
	stock := r.Stock
	cart := w.Entities[r.Cart]
	// Enclose the cart with impassable terrain after departure.
	original := append([]Tile(nil), w.Tiles...)
	x, y := int(cart.Position.X), int(cart.Position.Y)
	for dy := -2; dy <= 2; dy++ {
		for dx := -2; dx <= 2; dx++ {
			if dx*dx == 4 || dy*dy == 4 {
				w.Tiles[(y+dy)*w.Width+x+dx] = Tile{Terrain: "cliff"}
			}
		}
	}
	w.rebuildRegions()
	for range 2100 {
		w.Update()
	}
	if r.Stock != stock || r.Deliveries != 0 || r.lifecycle.State() != supplyTravelling {
		t.Fatal("blocked route replenished stock")
	}
	w.Tiles = original
	w.rebuildRegions()
	// Neutral supply carts are valid explicit attack targets through normal combat.
	cart.HP = 1 // A wounded caravan still takes damage through normal combat.
	attacker := w.spawn("knight", 1, Vec{cart.Position.X + .8, cart.Position.Y})
	w.refreshVisibility()
	if err := w.Apply(1, Command{Kind: "attack", EntityIDs: []int{attacker.ID}, TargetID: cart.ID}); err != nil {
		t.Fatal(err)
	}
	untilTrade(t, w, func() bool { return r.Losses == 1 })
	if r.Stock != stock || r.Cargo != (Resources{}) || r.NextSupply <= w.Time {
		t.Fatal("destroyed cargo reached the Market")
	}
	w.remove(attacker.ID)
	untilTrade(t, w, func() bool { return r.Deliveries == 1 })
	if r.Losses != 1 {
		t.Fatal("loss was recorded more than once")
	}
}

func TestRegionalEconomyContinuesAfterDepletionWithoutUnboundedStock(t *testing.T) {
	w, _, neutral, _ := localMerchantFixture(t)
	r := w.Marketplace.Regions[neutral.ID]
	r.Stock = Resources{}
	r.Output = Resources{Food: 108, Wood: 132, Stone: 7} // Tropical production / imports.
	r.NextSupply = 0
	for range 36000 {
		w.Update()
	}
	if r.Deliveries < 15 || r.Stock.Gold <= 0 || r.Stock.Wood < 100 {
		t.Fatal("depleted economy did not recover", r.Deliveries, r.Stock)
	}
	for _, res := range []string{"food", "wood", "stone"} {
		if n := r.Stock.Amount(res); n < 0 || n > merchantCapacity || math.IsNaN(n) {
			t.Fatal("stock escaped capacity", r.Stock)
		}
	}
	if r.Stock.Gold > 2000+float64(r.Deliveries)*merchantBudget {
		t.Fatal("consumer budget exceeded")
	}
}

func TestMerchantInformationAndGuardsRespectVisibility(t *testing.T) {
	w, _, neutral, cart := localMerchantFixture(t)
	w.Config.World.Reveal = "hidden"
	w.refreshVisibility()
	if len(w.View(1).Marketplace.Markets) != 0 {
		t.Fatal("hidden regional prices leaked")
	}
	if _, err := w.merchantTradeQuote(1, neutral.ID, "wood", "sell", cart); err == nil {
		t.Fatal("unexplored market accepted")
	}
	w.spawn("scout", 1, Vec{29, 20})
	w.refreshVisibility()
	before, _ := w.Checkpoint()
	for range 5 {
		w.View(1)
		w.View(2)
		w.merchantTradeQuote(1, neutral.ID, "wood", "sell", cart)
	}
	after, _ := w.Checkpoint()
	if !bytes.Equal(before, after) {
		t.Fatal("read or guard mutated inventory, RNG or lifecycle")
	}
	if len(w.View(1).Marketplace.Markets) != 1 || len(w.View(2).Marketplace.Markets) != 1 { // player 2's fixture Market also sees this site
		t.Fatal("observed neutral market was absent")
	}
	stock2 := w.View(2).Marketplace.Merchants.Stock
	w.Marketplace.Regions[-1].Stock.Wood = 123
	if w.View(2).Marketplace.Merchants.Stock != stock2 {
		t.Fatal("foreign home merchant inventory leaked")
	}
}

func TestLegacyMerchantInventoryIsDistributedWithoutDuplication(t *testing.T) {
	w, _, _, _ := localMerchantFixture(t)
	c := w.checkpointState()
	c.Version = 5
	c.Supplies = nil
	c.World.Marketplace.Regions = nil
	c.World.Marketplace.LegacyStock = Resources{Food: 321, Wood: 456, Gold: 789, Stone: 234}
	data, _ := json.Marshal(c)
	restored, err := Restore(data, w.JournalSince(0))
	if err != nil {
		t.Fatal(err)
	}
	total := Resources{}
	for _, r := range restored.Marketplace.Regions {
		total.Add(r.Stock)
	}
	if total != c.World.Marketplace.LegacyStock {
		t.Fatal("migration duplicated or lost merchant funds", total)
	}
	if restored.Marketplace.LegacyStock != (Resources{}) {
		t.Fatal("legacy stock remained as a second authority")
	}
}
