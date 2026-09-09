package game

// Production is gross income actually received, measured in game time. Costs,
// refunds and market exchanges never enter these counters. Both the partial
// bucket and the bounded history travel with the checkpoint.
const productionSampleTicks = 100 // five game seconds
const productionWindow = 12       // one game minute
const productionHistory = 60      // five game minutes

type ProductionSample struct {
	Time  float64   `json:"time"`
	Rates Resources `json:"rates"`
}

type ProductionView struct {
	Rates         Resources          `json:"rates"`
	History       []ProductionSample `json:"history"`
	WindowSeconds int                `json:"window_seconds"`
	SampleSeconds int                `json:"sample_seconds"`
}

type ResourceProduction struct {
	Pending Resources
	Buckets []Resources
	History []ProductionSample
}

func (w *World) receiveProduction(player int, resource string, amount float64) {
	p := w.Players[player]
	p.Resources.Deposit(resource, amount)
	p.Production.Pending.Deposit(resource, amount)
}

func (w *World) sampleProduction() {
	if w.Tick%productionSampleTicks != 0 {
		return
	}
	for _, p := range w.Players {
		s := &p.Production
		s.Buckets = append(s.Buckets, s.Pending)
		s.Pending = Resources{}
		if len(s.Buckets) > productionWindow {
			s.Buckets = append([]Resources(nil), s.Buckets[len(s.Buckets)-productionWindow:]...)
		}
		var rates Resources
		for _, income := range s.Buckets {
			rates.Add(income)
		}
		s.History = append(s.History, ProductionSample{Time: w.Time, Rates: rates})
		if len(s.History) > productionHistory {
			s.History = append([]ProductionSample(nil), s.History[len(s.History)-productionHistory:]...)
		}
	}
}

func (s ResourceProduction) view() ProductionView {
	v := ProductionView{History: append([]ProductionSample{}, s.History...), WindowSeconds: 60, SampleSeconds: 5}
	if len(s.History) > 0 {
		v.Rates = s.History[len(s.History)-1].Rates
	}
	return v
}
