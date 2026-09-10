package game

// Production is gross income actually received, measured in game time. Costs,
// refunds and market exchanges never enter these counters. Both the partial
// bucket and the bounded history travel with the checkpoint.
const productionSampleTicks = 100 // five game seconds
const productionWindow = 12       // one game minute
const productionHistory = 60      // five game minutes

type ProductionSample struct {
	Time        float64   `json:"time"`
	Rates       Resources `json:"rates"`
	Consumption Resources `json:"consumption,omitempty"`
}

type ProductionView struct {
	ConsumptionRates Resources          `json:"consumption_rates,omitempty"`
	Rates            Resources          `json:"rates"`
	History          []ProductionSample `json:"history"`
	WindowSeconds    int                `json:"window_seconds"`
	SampleSeconds    int                `json:"sample_seconds"`
}

type ResourceProduction struct {
	Total      Resources
	Consumed   Resources
	OutPending Resources
	OutBuckets []Resources
	Pending    Resources
	Buckets    []Resources
	History    []ProductionSample
}

func (w *World) receiveProduction(player int, resource string, amount float64) {
	p := w.Players[player]
	p.Resources.Deposit(resource, amount)
	p.Production.Pending.Deposit(resource, amount)
	p.Production.Total.Deposit(resource, amount)
}

// Consumption records actual spending, never escrow reservations, refunds,
// captured cargo or stockpile changes inferred by the frontend.
func (w *World) consume(player int, amount Resources, category string) {
	p := w.Players[player]
	p.Resources.Add(amount.Scale(-1))
	// CanPay tolerates floating-point dust; do not display a negative stockpile
	// when a fully funded fractional repair or upkeep exhausts a resource.
	for _, stock := range []*float64{&p.Resources.Food, &p.Resources.Wood, &p.Resources.Gold, &p.Resources.Stone} {
		if *stock < 0 && *stock > -1e-8 {
			*stock = 0
		}
	}
	w.accountConsumption(player, amount, category)
}
func (w *World) accountConsumption(player int, amount Resources, category string) {
	p := w.Players[player]
	p.Production.Consumed.Add(amount)
	p.Production.OutPending.Add(amount)
	if p.Economy.Consumption == nil {
		p.Economy.Consumption = map[string]Resources{}
	}
	value := p.Economy.Consumption[category]
	value.Add(amount)
	p.Economy.Consumption[category] = value
}

func (w *World) sampleProduction() {
	if w.Tick%productionSampleTicks != 0 {
		return
	}
	for _, p := range w.Players {
		s := &p.Production
		s.Buckets = append(s.Buckets, s.Pending)
		s.OutBuckets = append(s.OutBuckets, s.OutPending)
		s.OutPending = Resources{}
		if len(s.OutBuckets) > productionWindow {
			s.OutBuckets = append([]Resources(nil), s.OutBuckets[len(s.OutBuckets)-productionWindow:]...)
		}
		s.Pending = Resources{}
		if len(s.Buckets) > productionWindow {
			s.Buckets = append([]Resources(nil), s.Buckets[len(s.Buckets)-productionWindow:]...)
		}
		var rates Resources
		var consumed Resources
		for _, out := range s.OutBuckets {
			consumed.Add(out)
		}
		for _, income := range s.Buckets {
			rates.Add(income)
		}
		s.History = append(s.History, ProductionSample{Time: w.Time, Rates: rates, Consumption: consumed})
		if len(s.History) > productionHistory {
			s.History = append([]ProductionSample(nil), s.History[len(s.History)-productionHistory:]...)
		}
	}
}

func (s ResourceProduction) view() ProductionView {
	v := ProductionView{History: append([]ProductionSample{}, s.History...), WindowSeconds: 60, SampleSeconds: 5}
	if len(s.History) > 0 {
		v.Rates = s.History[len(s.History)-1].Rates
		v.ConsumptionRates = s.History[len(s.History)-1].Consumption
	}
	return v
}
