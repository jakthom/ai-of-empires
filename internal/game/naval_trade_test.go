package game

import (
	"bytes"
	"testing"
)

func seaTradeFixture(t *testing.T) (*World, *Entity, *Entity, *Entity) {
	w, _, _, _ := marketplaceFixture(t)
	for y := 5; y < 35; y++ {
		for x := 5; x < 50; x++ {
			w.Tiles[y*w.Width+x] = Tile{Terrain: "water", Elevation: -.2}
		}
	}
	w.rebuildRegions()
	w.Players[1].Age, w.Players[2].Age = 1, 1
	home, other := w.spawn("dock", 1, Vec{12, 35.5}), w.spawn("dock", 2, Vec{36, 35.5})
	ship := w.spawn("trade_ship", 1, Vec{12, 33})
	w.refreshVisibility()
	return w, home, other, ship
}
func TestNavalTradeDeliveryEscortAndCheckpoint(t *testing.T) {
	w, _, dock, ship := seaTradeFixture(t)
	o := postTestOffer(t, w, dock, 1)
	s := acceptTestOffer(t, w, o, ship, false)
	guard := w.spawn("galley", 1, Vec{14, 32})
	w.refreshVisibility()
	if err := w.Apply(1, Command{Kind: "guard", EntityIDs: []int{guard.ID}, TargetID: ship.ID}); err != nil {
		t.Fatal(err)
	}
	stepWorld(w, 150)
	if !w.water(ship.Position) || guard.Position.Distance(ship.Position) > 5 {
		t.Fatal("ship and escort did not sail together")
	}
	data, _ := w.Checkpoint()
	copy, err := Restore(data, w.JournalSince(0))
	if err != nil {
		t.Fatal(err)
	}
	untilTrade(t, w, func() bool { return s.lifecycle.State() == shipmentDelivered })
	for copy.Tick < w.Tick {
		copy.Update()
	}
	a, _ := w.Checkpoint()
	b, _ := copy.Checkpoint()
	if !bytes.Equal(a, b) {
		t.Fatal("sea delivery diverged after checkpoint")
	}
	if w.Players[1].Resources.Wood != 1200 || w.Players[1].Resources.Stone != 900 || w.Players[2].Resources.Wood != 800 || w.Players[2].Resources.Stone != 1100 {
		t.Fatal("sea trade failed conservation")
	}
}
func TestNavalMerchantAndSunkCargo(t *testing.T) {
	w, _, _, ship := seaTradeFixture(t)
	neutral := w.spawn("dock", 0, Vec{36, 35.5})
	w.refreshVisibility()
	if err := w.Apply(1, Command{Kind: "trade", EntityIDs: []int{ship.ID}, TargetID: neutral.ID, Product: "wood", TradeMode: "buy"}); err != nil {
		t.Fatal(err)
	}
	s := w.Marketplace.Shipments[w.Marketplace.NextShipment-1]
	untilTrade(t, w, func() bool { return s.lifecycle.State() == shipmentReturning })
	before := w.Players[3].Resources.Wood
	w.hit(ship, 3, 10000)
	w.pulseMarketplace()
	if s.lifecycle.State() != shipmentLost || w.Players[3].Resources.Wood != before+100 {
		t.Fatal("sinking did not transfer only the returning goods")
	}
}
func TestSeaRoutesRejectLandCarriersAndUnconnectedDocks(t *testing.T) {
	w, _, dock, ship := seaTradeFixture(t)
	o := postTestOffer(t, w, dock, 1)
	cart := w.entities(1, "trade_cart")[0]
	before := w.Players[1].Resources
	if err := w.Apply(1, Command{Kind: "market_accept", EntityIDs: []int{cart.ID}, OfferID: o.ID}); err == nil {
		t.Fatal("land cart accepted sea offer")
	}
	for y := 5; y < 35; y++ {
		w.Tiles[y*w.Width+25] = Tile{Terrain: "grass"}
	}
	w.rebuildRegions()
	if err := w.Apply(1, Command{Kind: "market_accept", EntityIDs: []int{ship.ID}, OfferID: o.ID}); err == nil {
		t.Fatal("disconnected sea route accepted")
	}
	if w.Players[1].Resources != before || o.Remaining != 1 {
		t.Fatal("rejected sea route spent resources")
	}
}
func TestNewMaritimeMapsProvideObservedTradingDocks(t *testing.T) {
	w, err := NewWorld(Config{Settlements: 2, Difficulty: "peaceful", World: WorldOptions{Type: "islands", Reveal: "all"}})
	if err != nil {
		t.Fatal(err)
	}
	docks := w.entities(0, "dock")
	if len(docks) == 0 {
		t.Fatal("maritime map has no neutral trading dock")
	}
	for _, dock := range docks {
		if w.Marketplace.Regions[dock.ID] == nil {
			t.Fatal("dock has no regional inventory")
		}
	}
}
