package game

import (
	"fmt"
	"maps"
	"math"
	"slices"
)

// These are accumulated facts, not lifecycle state. They persist with their
// kingdom and only change when the corresponding gameplay effect commits.
type EconomicRecord struct {
	Since                                                      float64
	Refunded                                                   Resources
	DamageDealt, DamageTaken                                   float64
	Consumption                                                map[string]Resources
	FoodUnmet, HungrySeconds                                   float64
	BuildingsCompleted, UnitsTrained, UnitsLost, BuildingsLost int
	Construction                                               Resources
	TradeSold, TradeBought                                     Resources
	TradeDeliveries                                            int
	PeakPopulation                                             int
	History                                                    []EconomicSample
}

type KingdomStatistics struct {
	ID                   int                  `json:"id"`
	Name                 string               `json:"name"`
	Civilization         string               `json:"civilization"`
	Age                  string               `json:"age"`
	AgeIndex             int                  `json:"age_index"`
	Defeated             bool                 `json:"defeated"`
	AccountingSince      float64              `json:"accounting_since"`
	Population           int                  `json:"population"`
	PeakPopulation       int                  `json:"peak_population"`
	Workers              int                  `json:"workers"`
	Military             int                  `json:"military"`
	IdleWorkers          int                  `json:"idle_workers"`
	WorkerTasks          map[string]int       `json:"worker_tasks"`
	Buildings            map[string]int       `json:"buildings"`
	Foundations          int                  `json:"foundations"`
	Units                map[string]int       `json:"units"`
	Technologies         []string             `json:"technologies"`
	Stock                Resources            `json:"stock"`
	Produced             Resources            `json:"produced"`
	Consumed             Resources            `json:"consumed"`
	Refunded             Resources            `json:"refunded"`
	ConsumptionByPurpose map[string]Resources `json:"consumption_by_purpose"`
	Production           ProductionView       `json:"production"`
	Food                 FoodView             `json:"food"`
	GDP                  float64              `json:"gdp"`
	GDPPerMinute         float64              `json:"gdp_per_minute"`
	GDPPerCapita         float64              `json:"gdp_per_capita"`
	StockValue           float64              `json:"stock_value"`
	ConstructionValue    float64              `json:"construction_value"`
	DevelopmentPerMinute float64              `json:"development_per_minute"`
	BuildingsCompleted   int                  `json:"buildings_completed"`
	UnitsTrained         int                  `json:"units_trained"`
	UnitsLost            int                  `json:"units_lost"`
	BuildingsLost        int                  `json:"buildings_lost"`
	Kills                int                  `json:"kills"`
	DamageDealt          float64              `json:"damage_dealt"`
	DamageTaken          float64              `json:"damage_taken"`
	ExploredPercent      float64              `json:"explored_percent"`
	TradeSold            Resources            `json:"trade_sold"`
	TradeBought          Resources            `json:"trade_bought"`
	TradeVolume          float64              `json:"trade_volume"`
	TradeDeliveries      int                  `json:"trade_deliveries"`
	History              []EconomicSample     `json:"history"`
}
type StatisticAward struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Rule    string  `json:"rule"`
	Winners []int   `json:"winners"`
	Value   float64 `json:"value"`
}
type StatisticsReport struct {
	Time           float64             `json:"time"`
	Status         string              `json:"status"`
	Scope          string              `json:"scope"`
	OfficialWinner int                 `json:"official_winner"`
	VictoryReason  string              `json:"victory_reason"`
	Valuation      Resources           `json:"valuation"`
	Accounting     string              `json:"accounting"`
	Kingdoms       []KingdomStatistics `json:"kingdoms"`
	Awards         []StatisticAward    `json:"awards"`
	Summary        []string            `json:"summary"`
}

var statisticalPrices = Resources{Food: 1, Wood: 1, Gold: 1, Stone: 1.3}

func resourceValue(r Resources) float64 { return r.Food + r.Wood + r.Gold + r.Stone*1.3 }

func (w *World) recordPopulationPeak(owner int) {
	if p := w.Players[owner]; p != nil {
		n, _ := w.population(owner)
		p.Economy.PeakPopulation = max(p.Economy.PeakPopulation, n)
	}
}

func (w *World) kingdomStatistics(p *Player) KingdomStatistics {
	e := p.Economy
	n, _ := w.population(p.ID)
	production := p.Production.view()
	v := KingdomStatistics{ID: p.ID, Name: p.Name, Civilization: p.Civilization, Age: Ages[p.Age], AgeIndex: p.Age, Defeated: p.lifecycle.State() == PlayerDefeated, AccountingSince: e.Since, Population: n, PeakPopulation: max(n, e.PeakPopulation), WorkerTasks: map[string]int{}, Buildings: map[string]int{}, Units: map[string]int{}, Technologies: sortedKeys(p.Technologies), Stock: p.Resources, Produced: p.Production.Total, Consumed: p.Production.Consumed, Refunded: e.Refunded, ConsumptionByPurpose: maps.Clone(e.Consumption), Production: production, Food: w.foodView(p), GDP: resourceValue(p.Production.Total), GDPPerMinute: resourceValue(production.Rates), StockValue: resourceValue(p.Resources), ConstructionValue: resourceValue(e.Construction), BuildingsCompleted: e.BuildingsCompleted, UnitsTrained: e.UnitsTrained, UnitsLost: e.UnitsLost, BuildingsLost: e.BuildingsLost, Kills: p.Kills, DamageDealt: e.DamageDealt, DamageTaken: e.DamageTaken, TradeSold: e.TradeSold, TradeBought: e.TradeBought, TradeVolume: resourceValue(e.TradeSold) + resourceValue(e.TradeBought), TradeDeliveries: e.TradeDeliveries, History: slices.Clone(e.History)}
	if v.ConsumptionByPurpose == nil {
		v.ConsumptionByPurpose = map[string]Resources{}
	}
	if v.History == nil {
		v.History = []EconomicSample{}
	}
	v.GDPPerCapita = v.GDP / float64(max(1, n))
	v.DevelopmentPerMinute = float64(e.BuildingsCompleted) * 60 / math.Max(60, w.Time-e.Since)
	for _, id := range w.IDs {
		entity := w.Entities[id]
		if entity == nil || entity.Owner != p.ID {
			continue
		}
		d := definitions[entity.Type]
		if d.Kind == "building" {
			if entity.life.State() == Foundation {
				v.Foundations++
			} else {
				v.Buildings[entity.Type]++
			}
		}
		if d.Kind == "unit" {
			v.Units[entity.Type]++
			if d.Class == "worker" {
				v.Workers++
				v.WorkerTasks[string(entity.behavior.State())]++
				if entity.behavior.State() == Idle {
					v.IdleWorkers++
				}
			} else if d.Attack > 0 {
				v.Military++
			}
		}
	}
	for _, known := range p.Explored {
		if known {
			v.ExploredPercent++
		}
	}
	v.ExploredPercent *= 100 / float64(len(p.Explored))
	return v
}

// all is an authorization decision made by the match boundary. During live
// gameplay ordinary players and MCP agents receive only their own accounting.
func (w *World) Statistics(player int, all bool) StatisticsReport {
	r := StatisticsReport{Time: w.Time, Status: string(w.match.State()), Scope: "kingdom", OfficialWinner: w.Winner, VictoryReason: w.VictoryReason, Valuation: statisticalPrices, Accounting: "GDP is a fixed-price resource-production proxy: food, wood and gold = 1; stone = 1.3. Trade, gifts, refunds and captured stock are excluded from production. Consumption records gross spending and food upkeep; refunds are separate. Construction value measures completed investment and is not added to GDP. Rates use game minutes; resource charts show a rolling minute over five minutes, campaign charts sample every minute.", Kingdoms: []KingdomStatistics{}, Awards: []StatisticAward{}, Summary: []string{}}
	if all {
		r.Scope = "world"
	}
	for id := 1; id <= w.Config.Settlements; id++ {
		if all || id == player {
			v := w.kingdomStatistics(w.Players[id])
			r.Kingdoms = append(r.Kingdoms, v)
			r.Summary = append(r.Summary, fmt.Sprintf("%s reached %s, produced %.0f GDP, completed %d buildings and trained %d units. Its population peaked at %d; %d units and %d buildings were lost.", v.Name, v.Age, v.GDP, v.BuildingsCompleted, v.UnitsTrained, v.PeakPopulation, v.UnitsLost, v.BuildingsLost))
		}
	}
	if !all {
		return r
	}
	for _, category := range []struct {
		id, name, rule string
		score          func(KingdomStatistics) float64
	}{
		{"economy", "Economic leader", "Highest cumulative GDP proxy from production.", func(v KingdomStatistics) float64 { return v.GDP }},
		{"development", "Development leader", "Highest resource value of completed construction, including replacement fields.", func(v KingdomStatistics) float64 { return v.ConstructionValue }},
		{"commerce", "Trade leader", "Highest combined value of completed incoming and outgoing trade.", func(v KingdomStatistics) float64 { return v.TradeVolume }},
		{"science", "Technology leader", "100 points per age advanced plus 1 per technology.", func(v KingdomStatistics) float64 { return float64(v.AgeIndex*100 + len(v.Technologies)) }},
		{"military", "Military leader", "Most enemy units and buildings destroyed.", func(v KingdomStatistics) float64 { return float64(v.Kills) }},
		{"population", "Population leader", "Largest peak population.", func(v KingdomStatistics) float64 { return float64(v.PeakPopulation) }},
		{"exploration", "Exploration leader", "Largest percentage of the map explored; reveal-all games can tie.", func(v KingdomStatistics) float64 { return v.ExploredPercent }},
	} {
		a := StatisticAward{ID: category.id, Name: category.name, Rule: category.rule, Winners: []int{}, Value: -1}
		for _, v := range r.Kingdoms {
			score := category.score(v)
			if score > a.Value+1e-8 {
				a.Value = score
				a.Winners = []int{v.ID}
			} else if math.Abs(score-a.Value) < 1e-8 {
				a.Winners = append(a.Winners, v.ID)
			}
		}
		r.Awards = append(r.Awards, a)
	}
	return r
}

func (w *World) sampleEconomy() {
	if w.Tick%productionSampleTicks != 0 {
		return
	}
	for id := 1; id <= w.Config.Settlements; id++ {
		p := w.Players[id]
		n, _ := w.population(id)
		p.Economy.PeakPopulation = max(p.Economy.PeakPopulation, n)
		if w.Tick%1200 != 0 {
			continue
		}
		buildings := 0
		for _, e := range w.entities(id, "") {
			if definitions[e.Type].Kind == "building" && e.life.State() == Active {
				buildings++
			}
		}
		p.Economy.History = append(p.Economy.History, EconomicSample{Time: w.Time, Population: n, Buildings: buildings, GDP: resourceValue(p.Production.Total), Produced: p.Production.Total, Consumed: p.Production.Consumed})
		if len(p.Economy.History) > 4320 {
			p.Economy.History = slices.Clone(p.Economy.History[len(p.Economy.History)-4320:])
		}
	}
}
func (w *World) refund(player int, amount Resources) {
	p := w.Players[player]
	p.Resources.Add(amount)
	p.Economy.Refunded.Add(amount)
}

type EconomicSample struct {
	Time       float64   `json:"time"`
	Population int       `json:"population"`
	Buildings  int       `json:"buildings"`
	GDP        float64   `json:"gdp"`
	Produced   Resources `json:"produced"`
	Consumed   Resources `json:"consumed"`
}
