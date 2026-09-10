package game

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func marketplaceFixture(t *testing.T) (*World, *Entity, *Entity, *Entity) {
	t.Helper()
	w, err := NewWorld(Config{Settlements: 3, Difficulty: "peaceful", World: WorldOptions{Type: "plains", Reveal: "all"}})
	if err != nil {
		t.Fatal(err)
	}
	w.Entities, w.IDs = map[int]*Entity{}, nil
	for i := range w.Tiles {
		w.Tiles[i] = Tile{Terrain: "grass"}
	}
	w.rebuildRegions()
	for id := 1; id <= 3; id++ {
		w.Players[id].AI = false
		w.Players[id].Resources = Resources{1000, 1000, 1000, 1000}
		w.spawn("town_center", id, Vec{float64(id * 20), 40})
	}
	home, partner := w.spawn("market", 1, Vec{12, 20}), w.spawn("market", 2, Vec{32, 20})
	cart := w.spawn("trade_cart", 1, Vec{14.5, 20})
	w.Marketplace.Regions = map[int]*merchantRegion{}
	w.initializeMerchantRegions()
	w.refreshVisibility()
	return w, home, partner, cart
}
func postTestOffer(t *testing.T, w *World, market *Entity, lots int) *tradeOffer {
	t.Helper()
	if err := w.Apply(market.Owner, Command{Kind: "market_post", EntityIDs: []int{market.ID}, Offer: &TradeOfferIntent{GiveResource: "wood", GiveAmount: 200, WantResource: "stone", WantAmount: 100, Lots: lots}}); err != nil {
		t.Fatal(err)
	}
	return w.Marketplace.Offers[w.Marketplace.NextOffer-1]
}
func acceptTestOffer(t *testing.T, w *World, o *tradeOffer, cart *Entity, repeat bool) *tradeShipment {
	t.Helper()
	if err := w.Apply(cart.Owner, Command{Kind: "market_accept", OfferID: o.ID, EntityIDs: []int{cart.ID}, Repeat: repeat}); err != nil {
		t.Fatal(err)
	}
	return w.Marketplace.Shipments[w.Marketplace.NextShipment-1]
}
func untilTrade(t *testing.T, w *World, done func() bool) {
	t.Helper()
	for i := 0; i < 3000 && !done(); i++ {
		w.Update()
	}
	if !done() {
		t.Fatal("caravan failed to reach its destination")
	}
}

func TestMarketplaceEscrowPrivacyAndCancellation(t *testing.T) {
	w, _, market, cart := marketplaceFixture(t)
	terms := TradeOfferIntent{GiveResource: "wood", GiveAmount: 200, WantResource: "stone", WantAmount: 100, Lots: 3, TargetPlayer: 1}
	if err := w.Apply(2, Command{Kind: "market_post", EntityIDs: []int{market.ID}, Offer: &terms}); err != nil {
		t.Fatal(err)
	}
	o := w.Marketplace.Offers[1]
	if w.Players[2].Resources.Wood != 400 || w.View(2).Marketplace.Reserved.Wood != 600 {
		t.Fatal("advertised goods were not reserved once")
	}
	if len(w.View(1).Marketplace.Offers) != 1 || len(w.View(3).Marketplace.Offers) != 0 {
		t.Fatal("private offer leaked or disappeared")
	}
	for _, kind := range []string{"market_accept", "market_cancel"} {
		if err := w.Apply(3, Command{Kind: kind, OfferID: o.ID, EntityIDs: []int{cart.ID}}); err == nil {
			t.Fatal("third party accessed private offer")
		}
	}
	if err := w.Apply(2, Command{Kind: "market_cancel", OfferID: o.ID}); err != nil {
		t.Fatal(err)
	}
	if w.Players[2].Resources.Wood != 1000 || o.Remaining != 0 {
		t.Fatal("refund incorrect")
	}
	if err := w.Apply(2, Command{Kind: "market_cancel", OfferID: o.ID}); err == nil || w.Players[2].Resources.Wood != 1000 {
		t.Fatal("cancel refunded twice")
	}
	log, _ := w.Log(3, LogQuery{Category: "economy"})
	for _, event := range log.Events {
		if strings.Contains(event.Message, "offer #1") {
			t.Fatal("private offer leaked through journal")
		}
	}
}

func TestMarketplaceRejectsUnfundedInvalidAndUnexploredOffers(t *testing.T) {
	w, _, market, cart := marketplaceFixture(t)
	before := w.Players[2].Resources
	for _, terms := range []TradeOfferIntent{
		{GiveResource: "wood", GiveAmount: 501, WantResource: "gold", WantAmount: 100, Lots: 1},
		{GiveResource: "wood", GiveAmount: 100, WantResource: "wood", WantAmount: 100, Lots: 1},
		{GiveResource: "wood", GiveAmount: -1, WantResource: "gold", WantAmount: 100, Lots: 1},
		{GiveResource: "wood", GiveAmount: 500, WantResource: "gold", WantAmount: 100, Lots: 20},
	} {
		if err := w.Apply(2, Command{Kind: "market_post", EntityIDs: []int{market.ID}, Offer: &terms}); err == nil {
			t.Fatal("invalid offer posted")
		}
		if w.Players[2].Resources != before || len(w.Marketplace.Offers) != 0 {
			t.Fatal("failed offer changed stock or book")
		}
	}
	o := postTestOffer(t, w, market, 1)
	w.Config.World.Reveal = "hidden"
	clear(w.Players[1].Visible)
	clear(w.Players[1].Memory)
	v := w.View(1).Marketplace.Offers[0]
	if v.CanAccept || v.MarketID != 0 || v.Position != nil {
		t.Fatal("unexplored market disclosed")
	}
	if err := w.Apply(1, Command{Kind: "market_accept", OfferID: o.ID, EntityIDs: []int{cart.ID}}); err == nil {
		t.Fatal("unexplored trade accepted")
	}
	if w.Players[1].Resources.Stone != 1000 || o.Remaining != 1 {
		t.Fatal("refused acceptance changed funds")
	}
}

func TestMarketplacePhysicalDeliveryAndCheckpointExactlyOnce(t *testing.T) {
	w, _, market, cart := marketplaceFixture(t)
	o := postTestOffer(t, w, market, 1)
	s := acceptTestOffer(t, w, o, cart, false)
	if w.Players[1].Resources.Stone != 900 || w.Players[2].Resources.Stone != 1000 || cart.Cargo != 100 {
		t.Fatal("payment did not load onto cart")
	}
	untilTrade(t, w, func() bool { return s.lifecycle.State() == shipmentReturning })
	if w.Players[2].Resources.Stone != 1100 || w.Players[1].Resources.Wood != 1000 || cart.Cargo != 200 {
		t.Fatal("pickup transferred resources at wrong time")
	}
	data, err := w.CaptureCheckpoint().Encode()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := Restore(data, w.JournalSince(0))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 1500; i++ {
		w.Update()
		restored.Update()
	}
	if s.lifecycle.State() != shipmentDelivered || w.Players[1].Resources.Wood != 1200 || w.Players[2].Resources.Wood != 800 || w.Players[2].Resources.Stone != 1100 {
		t.Fatal("delivery settlement incorrect")
	}
	a, _ := w.Checkpoint()
	b, _ := restored.Checkpoint()
	if !bytes.Equal(a, b) {
		t.Fatal("restored delivery diverged or replayed effects")
	}
	if len(w.View(3).Marketplace.Shipments) != 0 {
		t.Fatal("third party sees shipment details")
	}
}

func TestMarketplaceStopResumeAndRepeatingLots(t *testing.T) {
	w, _, market, cart := marketplaceFixture(t)
	o := postTestOffer(t, w, market, 2)
	s := acceptTestOffer(t, w, o, cart, true)
	if err := w.Apply(1, Command{Kind: "stop", EntityIDs: []int{cart.ID}}); err != nil {
		t.Fatal(err)
	}
	pos := cart.Position
	for range 50 {
		w.Update()
	}
	if cart.Position != pos || w.View(1).Marketplace.Shipments[0].CanResume != true {
		t.Fatal("stopped caravan moved or lost resume")
	}
	if err := w.Apply(1, Command{Kind: "trade", EntityIDs: []int{cart.ID}, TargetID: market.ID}); err == nil {
		t.Fatal("active cargo reused in another trade")
	}
	if err := w.Apply(1, Command{Kind: "market_resume", ShipmentID: s.ID}); err != nil {
		t.Fatal(err)
	}
	untilTrade(t, w, func() bool { return len(w.Marketplace.Shipments) == 2 && w.Marketplace.Shipments[2].terminal() })
	if w.Players[1].Resources.Wood != 1400 || w.Players[1].Resources.Stone != 800 || w.Players[2].Resources.Wood != 600 || w.Players[2].Resources.Stone != 1200 || o.Remaining != 0 {
		t.Fatal("repeating lots did not settle exactly twice")
	}
}

func TestMarketplaceCaravanLossAndRecall(t *testing.T) {
	for _, returning := range []bool{false, true} {
		t.Run(map[bool]string{false: "payment_lost", true: "goods_lost"}[returning], func(t *testing.T) {
			w, _, market, cart := marketplaceFixture(t)
			o := postTestOffer(t, w, market, 1)
			s := acceptTestOffer(t, w, o, cart, false)
			if returning {
				untilTrade(t, w, func() bool { return s.lifecycle.State() == shipmentReturning })
			}
			if err := w.Apply(1, Command{Kind: "delete", EntityIDs: []int{cart.ID}}); err != nil {
				t.Fatal(err)
			}
			for range 100 {
				w.Update()
			}
			if s.lifecycle.State() != shipmentLost || w.Players[1].Resources.Wood != 1000 || w.Players[1].Resources.Stone != 900 {
				t.Fatal("lost cargo was redeemed")
			}
			want := Resources{1000, 1000, 1000, 1000}
			if returning {
				want.Wood = 800
				want.Stone = 1100
			}
			if w.Players[2].Resources != want {
				t.Fatal("seller settlement/refund wrong", w.Players[2].Resources)
			}
		})
	}
	w, _, market, cart := marketplaceFixture(t)
	o := postTestOffer(t, w, market, 1)
	s := acceptTestOffer(t, w, o, cart, false)
	untilTrade(t, w, func() bool { return cart.Position.X > 20 })
	if err := w.Apply(1, Command{Kind: "market_recall", ShipmentID: s.ID}); err != nil {
		t.Fatal(err)
	}
	if w.Players[1].Resources.Stone != 900 || w.Players[2].Resources.Wood != 1000 {
		t.Fatal("recall teleported payment or withheld seller goods")
	}
	if err := w.Apply(1, Command{Kind: "market_recall", ShipmentID: s.ID}); err == nil {
		t.Fatal("recall accepted twice")
	}
	untilTrade(t, w, func() bool { return s.terminal() })
	if s.lifecycle.State() != shipmentRecalled || w.Players[1].Resources.Stone != 1000 {
		t.Fatal("recalled payment was not delivered")
	}
}

func TestMarketplaceMarketLossAndCompetingClaims(t *testing.T) {
	w, home, market, cart := marketplaceFixture(t)
	o := postTestOffer(t, w, market, 2)
	s := acceptTestOffer(t, w, o, cart, false)
	if err := w.Apply(1, Command{Kind: "market_accept", OfferID: o.ID, EntityIDs: []int{cart.ID}}); err == nil || o.Remaining != 1 {
		t.Fatal("cart accepted two shipments")
	}
	if err := w.Apply(2, Command{Kind: "delete", EntityIDs: []int{market.ID}}); err != nil {
		t.Fatal(err)
	}
	w.Update()
	if o.lifecycle.State() != offerCancelled || w.Players[2].Resources.Wood != 1000 {
		t.Fatal("market loss did not release remaining and uncollected stock")
	}
	untilTrade(t, w, func() bool { return s.terminal() })
	if w.Players[1].Resources.Stone != 1000 {
		t.Fatal("market loss payment not returned")
	}
	_ = home
}

func TestMerchantFiniteStockPricesAndStaleQuotes(t *testing.T) {
	w, home, _, _ := marketplaceFixture(t)
	before := w.Players[1].Resources
	stock := w.Marketplace.Regions[-1].Stock
	quote, _ := w.quoteExchange(w.Players[1], home, "market_buy", "wood")
	revision := w.Marketplace.Regions[-1].Revision
	if err := w.Apply(1, Command{Kind: "market_buy", EntityIDs: []int{home.ID}, Product: "wood", MarketRevision: &revision}); err != nil {
		t.Fatal(err)
	}
	next, _ := w.quoteExchange(w.Players[1], home, "market_buy", "wood")
	if next.Cost.Gold <= quote.Cost.Gold {
		t.Fatal("buying did not increase price")
	}
	total := w.Players[1].Resources
	total.Add(w.Marketplace.Regions[-1].Stock)
	expected := before
	expected.Add(stock)
	if total != expected {
		t.Fatal("merchant exchange created resources")
	}
	after := w.Players[1].Resources
	if err := w.Apply(1, Command{Kind: "market_buy", EntityIDs: []int{home.ID}, Product: "wood", MarketRevision: &revision}); err == nil || w.Players[1].Resources != after {
		t.Fatal("stale quote executed")
	}
	w.Marketplace.Regions[-1].Stock.Wood = 0
	if err := w.Apply(1, Command{Kind: "market_buy", EntityIDs: []int{home.ID}, Product: "wood"}); err == nil {
		t.Fatal("merchants created missing wood")
	}
	w.Marketplace.Regions[-1].Stock.Gold = 0
	if err := w.Apply(1, Command{Kind: "market_sell", EntityIDs: []int{home.ID}, Product: "stone"}); err == nil {
		t.Fatal("merchants created missing gold")
	}
}

func TestMarketplaceFrozenCaptureAndOldCheckpointUpgrade(t *testing.T) {
	w, _, market, cart := marketplaceFixture(t)
	o := postTestOffer(t, w, market, 3)
	acceptTestOffer(t, w, o, cart, false)
	capture := w.CaptureCheckpoint()
	a, _ := capture.Encode()
	_ = w.Apply(2, Command{Kind: "market_cancel", OfferID: o.ID})
	for range 30 {
		w.Update()
	}
	b, _ := capture.Encode()
	if !bytes.Equal(a, b) {
		t.Fatal("frozen capture retained mutable trade records")
	}
	c := w.checkpointState()
	c.Version = 4
	c.Offers = nil
	c.Shipments = nil
	c.World.Marketplace = marketplace{}
	// Simulate an older save without any marketplace or caravan records.
	delete(c.Entities, cart.ID)
	delete(w.Entities, cart.ID)
	data, _ := json.Marshal(c)
	restored, err := Restore(data, w.JournalSince(0))
	if err != nil {
		t.Fatal(err)
	}
	if restored.Marketplace.Regions[-1].Stock.Wood != 1000 || len(restored.Marketplace.Offers) != 0 {
		t.Fatal("legacy marketplace initialization failed")
	}
}

func TestMarketplaceSnapshotDeltaCarriesBook(t *testing.T) {
	w, _, market, _ := marketplaceFixture(t)
	var stream SnapshotStream
	stream.Next(w.View(1), 0)
	postTestOffer(t, w, market, 1)
	f := stream.Next(w.View(1), 50)
	if f.Delta == nil || f.Delta.Marketplace == nil || !reflect.DeepEqual(*f.Delta.Marketplace, w.View(1).Marketplace) {
		t.Fatal("SSE delta lost marketplace")
	}
}

func TestMarketplacePassageAndPrivateCaravanOrders(t *testing.T) {
	w, _, market, cart := marketplaceFixture(t)
	gate := w.spawn("gate", 2, Vec{26, 20})
	worker := w.spawn("villager", 1, Vec{16, 25})
	if w.freeFor(cart, gate.Position) {
		t.Fatal("cart passed foreign gate before a trade agreement")
	}
	o := postTestOffer(t, w, market, 1)
	s := acceptTestOffer(t, w, o, cart, false)
	if !w.freeFor(cart, gate.Position) || w.freeFor(worker, gate.Position) {
		t.Fatal("trade passage was missing or granted military/worker access")
	}
	w.Config.World.Reveal = "hidden"
	w.refreshVisibility()
	if w.visibleEntity(2, cart) {
		t.Fatal("privacy fixture must place the cart outside the seller's sight")
	}
	if err := w.Apply(1, Command{Kind: "stop", EntityIDs: []int{cart.ID}}); err != nil {
		t.Fatal(err)
	}
	view := w.View(2).Marketplace.Shipments[0]
	if view.Position != nil || view.CartID != 0 || view.Status != string(shipmentOutbound) || view.CanResume {
		t.Fatal("seller saw hidden cart position or orders", view)
	}
	w.noteAggression(worker.ID, 1, market)
	w.Update()
	if s.lifecycle.State() != shipmentRefunding || w.canUseGate(cart, gate) {
		t.Fatal("conflict did not recall trade and revoke passage")
	}
}

func TestMarketplaceQueuedMoveFollowsDelivery(t *testing.T) {
	w, _, market, cart := marketplaceFixture(t)
	o := postTestOffer(t, w, market, 2)
	s := acceptTestOffer(t, w, o, cart, true)
	goal := Vec{12, 32}
	if err := w.Apply(1, Command{Kind: "move", EntityIDs: []int{cart.ID}, Position: &goal, Queue: true}); err != nil {
		t.Fatal(err)
	}
	untilTrade(t, w, func() bool { return s.terminal() })
	if cart.behavior.State() != Moving || cart.Order.Position == nil || *cart.Order.Position != goal || len(w.Marketplace.Shipments) != 1 || o.Remaining != 1 {
		t.Fatal("delivery discarded queued movement or repeated over it")
	}
}

func TestMarketplaceGuardQueriesDoNotConsumeStockOrRNG(t *testing.T) {
	w, _, market, cart := marketplaceFixture(t)
	o := postTestOffer(t, w, market, 2)
	before, _ := w.Checkpoint()
	for range 10 {
		if err := w.canAcceptOffer(1, o, cart); err != nil {
			t.Fatal(err)
		}
		w.View(1)
		w.View(2)
	}
	after, _ := w.Checkpoint()
	if !bytes.Equal(before, after) {
		t.Fatal("read or guard mutated authoritative state")
	}
}

func TestNeutralTradeRequiresFundedGoodsAndMerchantPayment(t *testing.T) {
	w, _, _, cart := marketplaceFixture(t)
	neutral := w.spawn("market", 0, Vec{22, 20})
	w.refreshVisibility()
	region := w.Marketplace.Regions[neutral.ID]
	region.Stock.Gold = 5
	command := Command{Kind: "trade", EntityIDs: []int{cart.ID}, TargetID: neutral.ID, Product: "wood"}
	before := w.Players[1].Resources
	if err := w.Apply(1, command); err == nil || w.Players[1].Resources != before {
		t.Fatal("unfunded merchant payment was accepted")
	}
	region.Stock.Gold = 1000
	w.Players[1].Resources.Wood = 0
	if err := w.Apply(1, command); err == nil || region.Stock.Gold != 1000 {
		t.Fatal("empty cart harvested merchant money")
	}
	w.Players[1].Resources.Wood = 1000
	if err := w.Apply(1, command); err != nil {
		t.Fatal(err)
	}
	shipment := w.Marketplace.Shipments[w.Marketplace.NextShipment-1]
	if cart.CargoType != "wood" || cart.Cargo != 100 || w.Players[1].Resources.Wood != 900 {
		t.Fatal("exports did not load physically")
	}
	untilTrade(t, w, func() bool { return shipment.terminal() })
	if shipment.lifecycle.State() != shipmentDelivered || w.Players[1].Resources.Gold != 1070 || region.Stock.Wood != 1100 || region.Stock.Gold != 930 {
		t.Fatal("funded merchant settlement failed", region.Stock)
	}
}

func TestAIMarketPublishesItsOwnSurplusThroughCommands(t *testing.T) {
	w, _, _, _ := marketplaceFixture(t)
	p := w.Players[1]
	p.Resources = Resources{Food: 1000, Wood: 500, Gold: 0, Stone: 200}
	w.aiCommerce(w.aiObserve(1))
	book := w.View(1).Marketplace
	if len(book.Offers) != 1 || book.Offers[0].Owner != 1 || book.Offers[0].Terms.GiveResource != "food" || book.Offers[0].Terms.WantResource != "gold" || p.Resources.Food != 800 {
		t.Fatal("AI failed to fund a surplus offer", book)
	}
	for _, e := range w.journal.records {
		if e.Player == 1 && e.Kind == "order" && strings.Contains(e.Message, "market post") {
			return
		}
	}
	t.Fatal("AI bypassed the gameplay command boundary")
}
