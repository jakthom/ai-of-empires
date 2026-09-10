package game

func tradeCarrier(e *Entity) bool {
	return e != nil && (e.Type == "trade_cart" || e.Type == "trade_ship")
}
func tradingPost(e *Entity) bool { return e != nil && (e.Type == "market" || e.Type == "dock") }
func tradePostType(carrier *Entity) string {
	if carrier != nil && carrier.Type == "trade_ship" {
		return "dock"
	}
	return "market"
}
func (w *World) tradingPosts(owner int) []*Entity {
	posts := []*Entity{}
	for _, e := range w.entities(owner, "") {
		if tradingPost(e) {
			posts = append(posts, e)
		}
	}
	return posts
}
func (w *World) tradeCarriers(owner int) []*Entity {
	carriers := []*Entity{}
	for _, e := range w.entities(owner, "") {
		if tradeCarrier(e) {
			carriers = append(carriers, e)
		}
	}
	return carriers
}
func tradeRadius(post *Entity) float64 {
	if post == nil {
		return definitions["market"].Radius
	}
	return definitions[post.Type].Radius
}

// New maritime maps receive a few neutral trading docks on usable coasts.
// Existing checkpoints never regenerate posts or terrain. Land supply carts
// serve the dock's district; player cargo crosses water in Trade Ships.
func (w *World) seedTradingDocks(starts []Vec) {
	for _, start := range starts {
		var best *Vec
		distance := 50.0
		for y := 4; y < w.Height-4; y += 2 {
			for x := 4; x < w.Width-4; x += 2 {
				pos := Vec{float64(x) + .5, float64(y) + .5}
				d := pos.Distance(start)
				if d < 18 || d >= distance || !w.land(pos) {
					continue
				}
				water := false
				for _, offset := range []Vec{{2, 0}, {-2, 0}, {0, 2}, {0, -2}} {
					water = water || w.water(Vec{pos.X + offset.X, pos.Y + offset.Y})
				}
				if !water || !w.free(pos, definitions["dock"].Radius, 0, false) {
					continue
				}
				near := false
				for _, dock := range w.entities(0, "dock") {
					near = near || dock.Position.Distance(pos) < 20
				}
				if !near {
					best = &pos
					distance = d
				}
			}
		}
		if best != nil {
			w.spawn("dock", 0, *best)
		}
	}
}
