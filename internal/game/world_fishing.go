package game

import (
	"math/rand/v2"
	"sort"
)

// Fish belong to waterways, independently of their visual biome. Start with
// three spaced shoals near each kingdom's closest water, then populate the
// remaining river, lake or sea. Abundance scales the finite food per shoal.
func (w *World) seedFishing(starts []Vec, rng *rand.Rand) {
	water := []Vec{}
	for i, tile := range w.Tiles {
		if tile.Terrain == "water" {
			water = append(water, Vec{float64(i%w.Width) + .5, float64(i/w.Width) + .5})
		}
	}
	shoals := []Vec{}
	populated := map[int]bool{}
	place := func(p Vec) bool {
		for _, other := range shoals {
			if w.sameRegion(p, other, true) && p.Distance(other) < 2.5 {
				return false
			}
		}
		w.resource("fish", p)
		shoals = append(shoals, p)
		populated[w.region(p, true)] = true
		return true
	}
	for _, start := range starts {
		nearby := append([]Vec{}, water...)
		sort.SliceStable(nearby, func(i, j int) bool { return nearby[i].Distance(start) < nearby[j].Distance(start) })
		placed := 0
		for _, p := range nearby {
			if place(p) {
				placed++
			}
			if placed == 3 {
				break
			}
		}
	}
	for y := 4; y < w.Height-4; y += 5 {
		for x := 4; x < w.Width-4; x += 5 {
			p := Vec{float64(x) + rng.Float64(), float64(y) + rng.Float64()}
			if w.tile(p).Terrain == "water" && rng.Float64() < .45 {
				place(p)
			}
		}
	}
	// Small disconnected lakes must not lose their entire economy to chance.
	for _, p := range water {
		if !populated[w.region(p, true)] {
			place(p)
		}
	}
}
