package game

import (
	"math"
	"reflect"
	"testing"
)

func TestAIAndHumanPayAndDevelopAtTheSameRates(t *testing.T) {
	for _, civilization := range []string{"britons", "goths", "persians"} {
		for _, difficulty := range GetCatalog().Difficulties {
			t.Run(civilization+"/"+difficulty.ID, func(t *testing.T) {
				w := New(Config{Civilization: civilization, Difficulty: difficulty.ID})
				w.Players[2].Civilization = civilization
				producers := [2][]*Entity{}
				workers := [2]*Entity{}
				for i := range 2 {
					player := i + 1
					tc := w.entities(player, "town_center")[0]
					w.spawn("house", player, Vec{tc.Position.X, tc.Position.Y + 8})
					barracks := w.spawn("barracks", player, Vec{tc.Position.X + 12, tc.Position.Y + 8})
					producers[i] = []*Entity{tc, barracks}
					for j, product := range []string{"villager", "militia"} {
						if err := w.Apply(player, Command{Kind: "train", EntityIDs: []int{producers[i][j].ID}, Product: product}); err != nil {
							t.Fatal(err)
						}
					}
					workers[i] = w.entities(player, "villager")[0]
					source := w.spawn("tree", 0, Vec{workers[i].Position.X + 1, workers[i].Position.Y})
					source.Resource, source.Amount = "wood", 100
					if err := w.Apply(player, Command{Kind: "gather", EntityIDs: []int{workers[i].ID}, TargetID: source.ID}); err != nil {
						t.Fatal(err)
					}
				}
				if w.Players[1].Resources != w.Players[2].Resources {
					t.Fatal("AI paid different training costs")
				}
				for range 610 {
					for _, pair := range producers {
						for _, e := range pair {
							mustFire(e.production, ProductionPulse, &entityContext{World: w, Actor: e})
						}
					}
					for j := range 2 {
						if !reflect.DeepEqual(producers[0][j].Tasks, producers[1][j].Tasks) {
							t.Fatal("AI production ran on a different clock")
						}
					}
				}
				for range 100 {
					w.behave(workers[0])
					w.behave(workers[1])
				}
				if workers[0].Cargo <= 0 || math.Abs(workers[0].Cargo-workers[1].Cargo) > 1e-8 {
					t.Fatal("AI gathered resources faster than the same human civilization")
				}
			})
		}
	}
}

func TestEasyRaidsHaveSmallFixedRostersAndLongBreaks(t *testing.T) {
	w := strategyWorld()
	w.Config.Difficulty = "easy"
	p := w.Players[2]
	revealStrategyWorld(w, 2)
	w.Time = 599
	w.thinkPlayer(2)
	if p.strategy.State() == aiRaiding {
		t.Fatal("Easy raided before ten game minutes")
	}
	w.Time = 600
	w.thinkPlayer(2)
	if p.strategy.State() != aiRaiding || len(p.AIPlan.Army) != 4 || p.AIPlan.NextRaidAt != 840 {
		t.Fatalf("unexpected Easy raid: state %s, army %v, next %.1f", p.strategy.State(), p.AIPlan.Army, p.AIPlan.NextRaidAt)
	}
	roster := append([]int{}, p.AIPlan.Army...)
	for i := range 3 {
		w.spawn("militia", 2, Vec{40 + float64(i), 47})
	}
	w.thinkPlayer(2)
	if !reflect.DeepEqual(roster, p.AIPlan.Army) {
		t.Fatal("fresh soldiers enlarged the raid instead of remaining in reserve")
	}
	for _, id := range roster {
		w.remove(id)
	}
	w.Time = 601
	w.thinkPlayer(2)
	w.Time = 647
	w.thinkPlayer(2)
	if p.strategy.State() == aiRaiding {
		t.Fatal("Easy sent another raid before the four-minute break")
	}
	w.Time = 840
	w.thinkPlayer(2)
	if p.strategy.State() != aiRaiding || len(p.AIPlan.Army) > 4 {
		t.Fatal("Easy did not resume bounded raids when the break ended")
	}
}

func TestEasyMilitaryBudgetIncludesAllProducersAndQueuedUnits(t *testing.T) {
	w := strategyWorld()
	w.Config.Difficulty = "easy"
	p := w.Players[2]
	p.Resources = Resources{Food: 1000, Wood: 1000, Gold: 1000}
	for _, e := range w.entities(2, "militia")[:2] {
		w.remove(e.ID)
	}
	for i := range 4 {
		w.spawn("barracks", 2, Vec{50 + float64(i)*5, 55})
	}
	w.aiTrainMilitary(2, 100)
	queued := 0
	for _, producer := range w.entities(2, "barracks") {
		queued += len(producer.Tasks)
	}
	if len(w.entities(2, "militia"))+queued != 8 {
		t.Fatal("Easy's eight-soldier budget was not shared across production queues")
	}
	before := p.Resources
	for range 10 {
		w.aiTrainMilitary(2, 100)
	}
	if p.Resources != before {
		t.Fatal("Easy kept purchasing soldiers beyond its budget")
	}
}

func TestEasyRefundsOversizedOldQueuesWithoutDeletingExistingUnits(t *testing.T) {
	w := strategyWorld()
	w.Config.Difficulty = "easy"
	p := w.Players[2]
	p.Resources = Resources{Food: 1000, Gold: 1000}
	before := p.Resources
	for i := range 3 {
		producer := w.spawn("barracks", 2, Vec{50 + float64(i)*5, 55})
		if err := w.Apply(2, Command{Kind: "train", EntityIDs: []int{producer.ID}, Product: "militia"}); err != nil {
			t.Fatal(err)
		}
	}
	w.aiTrainMilitary(2, 100)
	if p.Resources != before || len(w.entities(2, "militia")) != 8 {
		t.Fatal("old queues were not refunded or existing units were removed")
	}
	cursor := w.NextEvent
	w.aiTrainMilitary(2, 100)
	if p.Resources != before || w.NextEvent != cursor {
		t.Fatal("excess training was refunded more than once")
	}
}

func TestEasyPausesBetweenVillagerOrdersWithoutChangingTrainingSpeed(t *testing.T) {
	w := strategyWorld()
	w.Config.Difficulty = "easy"
	p := w.Players[2]
	p.Resources = Resources{Food: 1000}
	for i := range 3 {
		w.spawn("house", 2, Vec{35 + float64(i)*3, 50})
	}
	w.thinkPlayer(2)
	tc := w.entities(2, "town_center")[0]
	if len(tc.Tasks) != 1 || tc.Tasks[0].Duration != 25 || p.AIPlan.NextWorkerAt != 340 {
		t.Fatal("Easy must use normal 25-second training and space its orders 40 seconds apart")
	}
	for range 505 {
		mustFire(tc.production, ProductionPulse, &entityContext{World: w, Actor: tc})
	}
	w.Time = 326
	w.thinkPlayer(2)
	if len(tc.Tasks) != 0 {
		t.Fatal("Easy immediately queued another villager instead of leaving breathing room")
	}
	w.Time = 340
	w.thinkPlayer(2)
	if len(tc.Tasks) != 1 || p.Resources.Food != 900 {
		t.Fatal("Easy did not resume paid villager production after its pause")
	}
}

func TestEasyDoesNotRaiseItsEconomyOrArmyBudgetWithAge(t *testing.T) {
	w := strategyWorld()
	w.Config.Difficulty = "easy"
	p := w.Players[2]
	p.Age, p.Resources = 3, Resources{Food: 1000, Wood: 1000, Gold: 1000, Stone: 1000}
	for i := 1; i < 12; i++ {
		w.spawn("villager", 2, Vec{40 + float64(i%4), 50 + float64(i/4)})
	}
	w.spawn("town_center", 2, Vec{40, 68}) // A pre-update save may contain more centers.
	revealStrategyWorld(w, 2)
	w.thinkPlayer(2)
	for _, center := range w.entities(2, "town_center") {
		for _, task := range center.Tasks {
			if task.Type == "train" && task.Product == "villager" {
				t.Fatal("Easy exceeded its twelve-villager goal after an age advancement or old expansion")
			}
		}
	}
	for _, producer := range w.entities(2, "") {
		for _, task := range producer.Tasks {
			if task.Type == "train" && definitions[task.Product].Class != "worker" {
				t.Fatal("Easy exceeded its eight-soldier budget in Imperial Age")
			}
		}
	}
}
