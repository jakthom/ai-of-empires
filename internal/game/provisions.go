package game

import (
	"context"
	"math"

	"github.com/open-ships/statemachine"
)

const foodPerPersonMinute = 2.0

type foodState string
type foodEvent string

const (
	foodFed      foodState = "fed"
	foodShortage foodState = "shortage"
	foodFamine   foodState = "famine"
	foodPulse    foodEvent = "pulse"
)

type foodContext struct {
	World    *World
	Player   *Player
	Required float64
}
type FoodView struct {
	State           string  `json:"state"`
	PerPersonMinute float64 `json:"per_person_minute"`
	DemandPerMinute float64 `json:"demand_per_minute"`
	Consumed        float64 `json:"consumed"`
	Unmet           float64 `json:"unmet"`
	ShortageSeconds float64 `json:"shortage_seconds"`
	WorkMultiplier  float64 `json:"work_multiplier"`
}

var foodMachine = statemachine.MustCompile([]statemachine.Transition[foodState, foodEvent, *foodContext]{
	{From: foodFed, Event: foodPulse, To: foodFed, Guard: canFeedPopulation, Do: feedPopulation},
	{From: foodFed, Event: foodPulse, To: foodShortage, Do: beginFoodShortage},
	{From: foodShortage, Event: foodPulse, To: foodFed, Guard: canFeedPopulation, Do: restoreFoodSupply},
	{From: foodShortage, Event: foodPulse, To: foodFamine, Guard: prolongedFoodShortage, Do: beginFamine},
	{From: foodShortage, Event: foodPulse, To: foodShortage, Do: feedPopulation},
	{From: foodFamine, Event: foodPulse, To: foodFed, Guard: canFeedPopulation, Do: restoreFoodSupply},
	{From: foodFamine, Event: foodPulse, To: foodFamine, Do: feedPopulation},
})

func (w *World) initializeProvisions() {
	for _, p := range w.Players {
		p.provisions = statemachine.NewInstance(foodMachine, foodFed)
	}
}
func canFeedPopulation(_ context.Context, c *foodContext) error {
	return applicable(c.Player.Resources.Food+1e-8 >= c.Required)
}
func prolongedFoodShortage(_ context.Context, c *foodContext) error {
	return applicable(c.World.Time-c.Player.ShortageSince >= 60)
}
func feedPopulation(_ context.Context, c *foodContext) error {
	paid := math.Min(c.Required, math.Max(0, c.Player.Resources.Food))
	c.World.consume(c.Player.ID, Resources{Food: paid}, "food_upkeep")
	c.Player.Economy.FoodUnmet += c.Required - paid
	if paid+1e-8 < c.Required {
		c.Player.Economy.HungrySeconds += Step
	}
	return nil
}
func beginFoodShortage(ctx context.Context, c *foodContext) error {
	c.Player.ShortageSince = c.World.Time
	c.World.event(c.Player.ID, "Food shortage: your population needs more food. Prolonged hunger slows work and production.")
	return feedPopulation(ctx, c)
}
func beginFamine(ctx context.Context, c *foodContext) error {
	c.World.event(c.Player.ID, "Famine: gathering, construction and production operate at half speed until food returns.")
	return feedPopulation(ctx, c)
}
func restoreFoodSupply(ctx context.Context, c *foodContext) error {
	c.World.event(c.Player.ID, "Food supplies have recovered. Work proceeds at full speed.")
	c.Player.ShortageSince = 0
	return feedPopulation(ctx, c)
}
func (p *Player) workMultiplier() float64 {
	if p.provisions != nil && p.provisions.State() == foodFamine {
		return .5
	}
	return 1
}
func (w *World) pulseProvisions() {
	population := map[int]int{}
	for _, id := range w.IDs {
		if e := w.Entities[id]; e != nil {
			population[e.Owner] += definitions[e.Type].Population
		}
	}
	for id := 1; id <= w.Config.Settlements; id++ {
		p := w.Players[id]
		if p.lifecycle.State() != PlayerDefeated {
			mustFire(p.provisions, foodPulse, &foodContext{World: w, Player: p, Required: float64(population[id]) * foodPerPersonMinute * Step / 60})
		}
	}
}
func (w *World) foodView(p *Player) FoodView {
	n, _ := w.population(p.ID)
	state := foodFed
	if p.provisions != nil {
		state = p.provisions.State()
	}
	shortage := 0.
	if state != foodFed {
		shortage = w.Time - p.ShortageSince
	}
	return FoodView{string(state), foodPerPersonMinute, float64(n) * foodPerPersonMinute, p.Economy.Consumption["food_upkeep"].Food, p.Economy.FoodUnmet, shortage, p.workMultiplier()}
}
