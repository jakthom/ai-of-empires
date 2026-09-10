package game

type GlobalCommodityView struct {
	Resource   string  `json:"resource"`
	Demand     int     `json:"demand"`
	Supply     int     `json:"supply"`
	BuyOrders  int     `json:"buy_orders"`
	SellOrders int     `json:"sell_orders"`
	BestBid    float64 `json:"best_bid,omitempty"`
	BestAsk    float64 `json:"best_ask,omitempty"`
}

// Publicly advertised quantities only. Aggregating private offers would reveal
// foreign demand even if their individual listings were correctly filtered.
func (w *World) globalMarket() []GlobalCommodityView {
	book := []GlobalCommodityView{}
	for _, resource := range []string{"food", "wood", "stone"} {
		v := GlobalCommodityView{Resource: resource}
		for _, o := range w.Marketplace.Offers {
			if o.lifecycle.State() != offerOpen || o.Terms.TargetPlayer != 0 {
				continue
			}
			t := o.Terms
			if t.GiveResource == "gold" && t.WantResource == resource {
				v.Demand += t.WantAmount * o.Remaining
				v.BuyOrders++
				v.BestBid = max(v.BestBid, 100*float64(t.GiveAmount)/float64(t.WantAmount))
			}
			if t.GiveResource == resource && t.WantResource == "gold" {
				v.Supply += t.GiveAmount * o.Remaining
				v.SellOrders++
				price := 100 * float64(t.WantAmount) / float64(t.GiveAmount)
				if v.BestAsk == 0 || price < v.BestAsk {
					v.BestAsk = price
				}
			}
		}
		book = append(book, v)
	}
	return book
}
