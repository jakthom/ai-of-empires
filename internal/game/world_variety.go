package game

import "math"

type landscapeIsland struct {
	Center Vec
	Radius float64
}

// Reserving home economies and land passes can leave tiny deep-water patches
// beside a crossing. Turn them into shallows before seeding the new layouts so
// fishing supplies stay beside a usable lake. Naval routes remain connected.
func (w *World) trimSmallWaterPockets() bool {
	switch w.Config.World.Type {
	case "mountain_lakes", "wetlands", "fjords", "archipelago":
	default:
		return false
	}
	visited := make([]bool, len(w.Tiles))
	changed := false
	for i, tile := range w.Tiles {
		if visited[i] || tile.Terrain != "water" {
			continue
		}
		visited[i] = true
		cells := []int{i}
		for next := 0; next < len(cells); next++ {
			x, y := cells[next]%w.Width, cells[next]/w.Width
			for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				nx, ny := x+d[0], y+d[1]
				if nx < 0 || ny < 0 || nx >= w.Width || ny >= w.Height {
					continue
				}
				index := ny*w.Width + nx
				if !visited[index] && w.Tiles[index].Terrain == "water" {
					visited[index] = true
					cells = append(cells, index)
				}
			}
		}
		if len(cells) < 16 {
			for _, index := range cells {
				w.Tiles[index] = Tile{Terrain: "shallows", Elevation: 0}
			}
			changed = true
		}
	}
	return changed
}

func archipelagoIslands(starts []Vec, size, phase float64) []landscapeIsland {
	islands := []landscapeIsland{}
	for i, home := range starts {
		radius := math.Min(42, size*.145)
		for j, other := range starts {
			if i != j {
				radius = math.Min(radius, home.Distance(other)*.42)
			}
		}
		islands = append(islands, landscapeIsland{home, math.Max(13.8, radius)})
	}
	// Offshore islets never bridge the water gap between home kingdoms.
	for i := 0; i < 12; i++ {
		a := float64(i)*math.Pi/6 + phase
		p := Vec{size/2 + math.Cos(a)*size*.42, size/2 + math.Sin(a)*size*.42}
		radius := math.Min(7, size*.035)
		clear := true
		for _, island := range islands {
			if p.Distance(island.Center) < radius+island.Radius+4 {
				clear = false
			}
		}
		if clear {
			islands = append(islands, landscapeIsland{p, radius})
		}
	}
	return islands
}

// Regional scenery is checkpointed terrain metadata. It has no influence on
// walkability, supplies or civilization bonuses, and unknown cells hide it.
func (w *World) assignRegionalBiomes() {
	if w.Config.World.Biome != "mixed" {
		return
	}
	phase := float64(w.Config.Seed%997) / 997 * math.Pi * 2
	for i := range w.Tiles {
		x, y := float64(i%w.Width)/float64(w.Width), float64(i/w.Width)/float64(w.Height)
		latitude := y + .065*math.Sin(x*11+phase)
		biome := "temperate"
		switch {
		case latitude < .19 || w.Tiles[i].Elevation > 1.25:
			biome = "alpine"
		case latitude < .43 && x > .5+.08*math.Sin(y*9+phase):
			biome = "autumn"
		case latitude > .72 && x < .45:
			biome = "desert"
		case latitude > .62 && x < .7:
			biome = "savanna"
		case latitude > .65:
			biome = "tropical"
		}
		w.Tiles[i].Biome = biome
	}
}
