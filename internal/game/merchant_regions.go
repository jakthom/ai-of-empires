package game

import (
	"context"
	"fmt"
	"math"

	"github.com/open-ships/statemachine"
)

const merchantCycle = 90.0
const merchantBudget = 240.0

type supplyState string
type supplyEvent string

const (
	supplyWaiting    supplyState = "preparing"
	supplyTravelling supplyState = "travelling"
	supplyPulse      supplyEvent = "pulse"
	supplyRaided     supplyEvent = "raided"
)

// A region owns its inventory and one replenishment lifecycle. Cargo exists
// only here while its identified physical cart is alive, never in stock too.
// Negative IDs are a kingdom's home merchants; positive IDs are neutral Markets.
type merchantRegion struct {
	ID                 int
	Biome              string
	Stock              Resources
	Revision           int
	Output             Resources
	Demand             Resources
	NextSupply         float64
	Cart               int
	Cargo              Resources
	Deliveries, Losses int
	lifecycle          *statemachine.Instance[supplyState, supplyEvent, *supplyContext]
}
type supplyContext struct {
	Raider, Source int
	Victim         *Entity
	World          *World
	Region         *merchantRegion
	Market         *Entity
	Origin         *Vec
}

var supplyMachine *statemachine.Machine[supplyState, supplyEvent, *supplyContext]

func init() {
	supplyMachine = statemachine.MustCompile([]statemachine.Transition[supplyState, supplyEvent, *supplyContext]{
		{From: supplyTravelling, Event: supplyRaided, To: supplyWaiting, Do: raidSupply},
		{From: supplyWaiting, Event: supplyPulse, To: supplyTravelling, Guard: supplyReady, Do: launchSupply},
		{From: supplyWaiting, Event: supplyPulse, To: supplyWaiting},
		{From: supplyTravelling, Event: supplyPulse, To: supplyWaiting, Guard: supplyLost, Do: loseSupply},
		{From: supplyTravelling, Event: supplyPulse, To: supplyWaiting, Guard: supplyArrived, Do: deliverSupply},
		{From: supplyTravelling, Event: supplyPulse, To: supplyTravelling, Do: moveSupply},
	})
}

func (w *World) addMerchantRegion(id int, pos Vec) {
	if w.Marketplace.Regions == nil || w.Marketplace.Regions[id] != nil {
		return
	}
	biome := w.tile(pos).Biome
	if biome == "" && w.Config.World.Biome != "mixed" {
		biome = w.Config.World.Biome
	}
	factor := Resources{Food: 1, Wood: 1, Gold: 1, Stone: 1}
	for _, b := range biomeEconomies() {
		if b.Biome == biome {
			factor = b.Deposits
		}
	}
	if biome == "" {
		biome = "temperate"
	}
	w.Marketplace.Regions[id] = &merchantRegion{ID: id, Biome: biome,
		Stock:  Resources{Food: 1000 * factor.Food, Wood: 1000 * factor.Wood, Stone: 1000 * factor.Stone, Gold: 2000},
		Output: Resources{Food: 60 * factor.Food, Wood: 60 * factor.Wood, Stone: 20 * factor.Stone},
		Demand: Resources{Food: 80, Wood: 60, Stone: 30}, NextSupply: w.Time + merchantCycle,
		lifecycle: statemachine.NewInstance(supplyMachine, supplyWaiting)}
}
func (w *World) initializeMerchantRegions() {
	for id := 1; id <= w.Config.Settlements; id++ {
		w.addMerchantRegion(-id, w.Players[id].Start)
	}
	for _, e := range w.tradingPosts(0) {
		w.addMerchantRegion(e.ID, e.Position)
	}
}
func (w *World) merchantHost(r *merchantRegion) *Entity {
	if r.ID > 0 {
		e := w.Entities[r.ID]
		if tradingPost(e) && e.Owner == 0 && e.life.State() == Active {
			return e
		}
		return nil
	}
	p := w.Players[-r.ID]
	if p == nil || p.lifecycle.State() == PlayerDefeated {
		return nil
	}
	return w.nearest(p.Start, func(e *Entity) bool { return w.ownMarket(p.ID, e.ID) })
}
func (w *World) supplyOrigin(market *Entity) *Vec {
	if market == nil {
		return nil
	}
	for radius := 12.; radius <= 18; radius += 3 {
		for i := 0; i < 16; i++ {
			a := float64(i) * math.Pi / 8
			p := Vec{market.Position.X + radius*math.Cos(a), market.Position.Y + radius*math.Sin(a)}
			if w.free(p, .6, 0, false) && w.sameRegion(p, market.Position, false) {
				return &p
			}
		}
	}
	return nil
}
func supplyReady(_ context.Context, c *supplyContext) error {
	return applicable(c.World.Time >= c.Region.NextSupply && c.Market != nil && c.Origin != nil)
}
func launchSupply(_ context.Context, c *supplyContext) error {
	r, w := c.Region, c.World
	cart := w.spawn("supply_cart", 0, *c.Origin)
	r.Cart = cart.ID
	r.Cargo = r.Output
	// Consumer money accompanies a finite shipment. It can enter the merchant
	// till only in exchange for goods, on arrival; unspent money leaves again.
	r.Cargo.Gold = merchantBudget
	return nil
}
func supplyLost(_ context.Context, c *supplyContext) error {
	cart := c.World.Entities[c.Region.Cart]
	return applicable(c.Market == nil || cart == nil || cart.Type != "supply_cart" || cart.Owner != 0 || cart.life.State() != Active)
}
func supplyArrived(_ context.Context, c *supplyContext) error {
	return applicable(c.World.Entities[c.Region.Cart].Position.Distance(c.Market.Position) <= tradeRadius(c.Market)+.8)
}
func finishSupply(c *supplyContext) {
	c.World.remove(c.Region.Cart)
	c.Region.Cart = 0
	c.Region.Cargo = Resources{}
	c.Region.NextSupply = c.World.Time + merchantCycle
}
func loseSupply(_ context.Context, c *supplyContext) error {
	c.Region.Losses++
	finishSupply(c)
	return nil
}
func deliverSupply(_ context.Context, c *supplyContext) error {
	r := c.Region
	for _, resource := range []string{"food", "wood", "stone"} {
		r.Stock.Deposit(resource, math.Min(r.Cargo.Amount(resource), math.Max(0, c.World.merchantRoom(r, resource))))
	}
	budget := r.Cargo.Gold
	for _, resource := range []string{"food", "wood", "stone"} {
		// Households buy a bounded amount at the local wholesale valuation. Demand
		// consumes goods; it never pays for an empty inventory or merely a distance.
		price := merchantPrice(r, resource) / 100
		amount := math.Min(r.Stock.Amount(resource), math.Min(r.Demand.Amount(resource), budget/price))
		r.Stock.Deposit(resource, -amount)
		payment := amount * price
		r.Stock.Gold += payment
		budget = math.Max(0, budget-payment)
	}
	r.Revision++
	r.Deliveries++
	c.World.record(Event{Kind: "trade", Message: fmt.Sprintf("Regional supply arrived: %s.", r.Biome)}, c.Market, 0)
	finishSupply(c)
	return nil
}
func moveSupply(_ context.Context, c *supplyContext) error {
	c.World.move(c.World.Entities[c.Region.Cart], c.Market.Position, tradeRadius(c.Market)+.7, Step)
	return nil
}
func (w *World) pulseMerchantSupplies() {
	for _, id := range sortedTradeIDs(w.Marketplace.Regions) {
		r := w.Marketplace.Regions[id]
		c := &supplyContext{World: w, Region: r, Market: w.merchantHost(r)}
		if r.lifecycle.State() == supplyWaiting && w.Time >= r.NextSupply {
			// Retry a blocked departure once per second, without changing its due date.
			if w.Tick%20 != 0 {
				continue
			}
			c.Origin = w.supplyOrigin(c.Market)
		}
		mustFire(r.lifecycle, supplyPulse, c)
	}
}
func merchantPrice(r *merchantRegion, resource string) float64 {
	return math.Max(30, math.Min(200, 100+(1000-r.Stock.Amount(resource))*.08))
}

// The victim's lethal-damage effect removes the cart after this commit.
// Removing it here would synchronously re-enter its own entity lifecycle.
func raidSupply(_ context.Context, c *supplyContext) error {
	r := c.Region
	c.World.awardSpoils(c.Raider, c.Source, c.Victim, r.Cargo)
	r.Cart = 0
	r.Cargo = Resources{}
	r.Losses++
	r.NextSupply = c.World.Time + merchantCycle
	return nil
}
