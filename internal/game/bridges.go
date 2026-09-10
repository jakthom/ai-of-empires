package game

import (
	"fmt"
	"math"
)

// Terrain flags are derived navigation/projection caches. The bridge entity's
// life instance is the owner; restoring a save always rebuilds these flags.
func (w *World) restoreBridgeDecks() error {
	for i := range w.Tiles {
		w.Tiles[i].Bridge = false
		w.Tiles[i].DeckElevation = 0
	}
	for _, e := range w.Entities {
		if e.Type != "bridge" {
			continue
		}
		if !w.inside(e.Position) || !w.water(e.Position) || math.IsNaN(e.DeckElevation) || math.IsInf(e.DeckElevation, 0) || e.DeckElevation < -.2 || e.DeckElevation > 2 || e.BuildGroup <= 0 || e.BuildGroup >= w.NextID || (e.Orientation != "east_west" && e.Orientation != "north_south") {
			return fmt.Errorf("invalid saved bridge")
		}
		if e.life.State() != Active {
			continue
		}
		i := int(e.Position.Y)*w.Width + int(e.Position.X)
		if w.Tiles[i].Bridge {
			return fmt.Errorf("overlapping saved bridges")
		}
		w.Tiles[i].Bridge = true
		w.Tiles[i].DeckElevation = e.DeckElevation
	}
	w.rebuildRegions()
	return nil
}

func (w *World) bridgePlan(player int, start Vec, end *Vec) ([]Vec, Resources, error) {
	if end == nil {
		return nil, Resources{}, rule("bridge_banks", "Drag from one riverbank to the opposite bank.")
	}
	a, b := snap(start), snap(*end)
	if math.Abs(b.X-a.X) >= math.Abs(b.Y-a.Y) {
		b.Y = a.Y
	} else {
		b.X = a.X
	}
	if !w.inside(a) || !w.inside(b) || !w.Players[player].Explored[int(a.Y)*w.Width+int(a.X)] || !w.Players[player].Explored[int(b.Y)*w.Width+int(b.X)] {
		return nil, Resources{}, rule("unexplored", "Explore both banks before building a bridge.")
	}
	n := int(a.Distance(b))
	if n < 2 || n > 32 || !w.land(a) || !w.land(b) {
		return nil, Resources{}, rule("bridge_banks", "Connect two land banks in a straight line, up to 32 tiles apart.")
	}
	dx, dy := (b.X-a.X)/float64(n), (b.Y-a.Y)/float64(n)
	points := []Vec{}
	for i := 1; i < n; i++ {
		p := Vec{a.X + dx*float64(i), a.Y + dy*float64(i)}
		if !w.water(p) {
			return points, Resources{}, rule("bridge_water", "The span between the banks must cross water.")
		}
		if err := w.Placement(player, "bridge", p); err != nil {
			return points, Resources{}, err
		}
		points = append(points, p)
	}
	price := definitions["bridge"].Cost.Scale(float64(len(points)))
	if err := w.canBuild(w.Players[player], definitions["bridge"]); err != nil {
		return points, price, err
	}
	if !w.Players[player].Resources.CanPay(price) {
		return points, price, rule("insufficient_resources", "Gather enough wood and stone for the whole bridge.")
	}
	return points, price, nil
}

func (w *World) BridgeElevation(start Vec, end *Vec) float64 {
	if end == nil {
		return 0
	}
	a, b := snap(start), snap(*end)
	if math.Abs(b.X-a.X) >= math.Abs(b.Y-a.Y) {
		b.Y = a.Y
	} else {
		b.X = a.X
	}
	return math.Max(w.tile(a).Elevation, w.tile(b).Elevation) + .03
}

func (w *World) completeBridge(e *Entity) {
	if e.Type != "bridge" {
		return
	}
	i := int(e.Position.Y)*w.Width + int(e.Position.X)
	w.Tiles[i].Bridge = true
	w.Tiles[i].DeckElevation = e.DeckElevation
	w.rebuildRegions()
}

func (w *World) collapseBridge(c *entityContext) {
	e := c.Actor
	if e.Type != "bridge" || e.life.State() != Active {
		return
	}
	i := int(e.Position.Y)*w.Width + int(e.Position.X)
	w.Tiles[i].Bridge = false
	w.Tiles[i].DeckElevation = 0
	w.rebuildRegions()
	for _, id := range append([]int(nil), w.IDs...) {
		u := w.Entities[id]
		if u == nil || u.Container != 0 || definitions[u.Type].Kind != "unit" || definitions[u.Type].Naval || int(u.Position.Y)*w.Width+int(u.Position.X) != i || w.land(u.Position) {
			continue
		}
		mustFire(u.life, DamageEntity, &entityContext{World: w, Actor: u, Amount: u.HP, SourceOwner: c.SourceOwner, SourceID: c.SourceID})
	}
}
