package game

import "slices"

// Cache immutable projections per player, never the authoritative fog arrays.
// Copy only on change. Previously returned snapshots remain safe to encode
// after the game lock is released, even as visibility changes on the next tick.
func (w *World) mapView(p *Player) MapView {
	if w.viewMaps == nil {
		w.viewMaps = map[int]MapView{}
	}
	v := w.viewMaps[p.ID]
	changed := len(v.Tiles) != len(w.Tiles)
	if changed {
		v = MapView{Width: w.Width, Height: w.Height, Biome: w.WorldOptions().Biome, Tiles: make([]Tile, len(w.Tiles)), Fog: make([]int, len(w.Tiles))}
	}
	for i, tile := range w.Tiles {
		fog := 0
		if p.Explored[i] {
			fog = 1
		} else {
			tile = Tile{Terrain: "unknown"}
		}
		if p.Visible[i] {
			fog = 2
		}
		if tile == v.Tiles[i] && fog == v.Fog[i] {
			continue
		}
		if !changed {
			v.Tiles, v.Fog = slices.Clone(v.Tiles), slices.Clone(v.Fog)
			changed = true
		}
		v.Tiles[i], v.Fog[i] = tile, fog
	}
	w.viewMaps[p.ID] = v
	return v
}
