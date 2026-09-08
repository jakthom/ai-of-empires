package game

// Flat starting clearances can interrupt a meandering river. Rejoin its arms
// with navigable shallows around those homes, preserving both land crossings
// and the shared water route. This bounded deterministic search runs only at
// creation; it consumes no simulation randomness.
func (w *World) connectRiver(starts []Vec) error {
	allowed := make([]bool, len(w.Tiles))
	center := Vec{float64(w.Width) / 2, float64(w.Height) / 2}
	for i, t := range w.Tiles {
		p := Vec{float64(i%w.Width) + .5, float64(i/w.Width) + .5}
		allowed[i] = t.Terrain != "cliff" && p.Distance(center) > 6.5
		for _, home := range starts {
			if p.Distance(home) < 13.2 {
				allowed[i] = false
			}
		}
		if t.Terrain == "water" || t.Terrain == "shallows" {
			allowed[i] = true
		}
	}
	for attempt := 0; attempt < w.Width; attempt++ {
		regions := w.regions(true)
		root := 0
		split := false
		for _, r := range regions {
			if r != 0 {
				if root == 0 {
					root = r
				}
				if r != root {
					split = true
					break
				}
			}
		}
		if !split {
			return nil
		}
		parents := make([]int, len(w.Tiles))
		queue := make([]int, 0, len(w.Tiles))
		for i, r := range regions {
			parents[i] = -1
			if r == root {
				parents[i] = i
				queue = append(queue, i)
			}
		}
		end := -1
		for head := 0; head < len(queue) && end < 0; head++ {
			i := queue[head]
			x, y := i%w.Width, i/w.Width
			for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				nx, ny := x+d[0], y+d[1]
				if nx < 1 || ny < 1 || nx >= w.Width-1 || ny >= w.Height-1 {
					continue
				}
				j := ny*w.Width + nx
				if parents[j] >= 0 || !allowed[j] {
					continue
				}
				parents[j] = i
				queue = append(queue, j)
				if regions[j] != 0 && regions[j] != root {
					end = j
					break
				}
			}
		}
		if end < 0 {
			return rule("generation_failed", "The river needs more room around the settlements. Choose a larger world or another seed.")
		}
		for parents[end] != end {
			// Widen where possible so ships can pass one another at crossings.
			x, y := end%w.Width, end/w.Width
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					nx, ny := x+dx, y+dy
					if nx < 1 || ny < 1 || nx >= w.Width-1 || ny >= w.Height-1 {
						continue
					}
					i := ny*w.Width + nx
					if allowed[i] && w.Tiles[i].Terrain == "grass" {
						w.Tiles[i] = Tile{Terrain: "shallows", Elevation: -.05}
					}
				}
			}
			end = parents[end]
		}
	}
	return rule("generation_failed", "The river could not be connected. Choose another seed.")
}
