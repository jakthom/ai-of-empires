package game

import (
	"iter"
	"math"
	"slices"
)

// A broad-phase index of entity references, rebuilt for each simulation pulse.
// It is never authoritative or persisted. Outside a pulse, queries use the
// world directly so command validation and detached restores need no cache.
type entityGrid struct {
	width  int
	cells  [][]*Entity
	radius float64
	active bool
}

func (w *World) beginSpatial() {
	width := (w.Width + 3) / 4
	count := width * ((w.Height + 3) / 4)
	if w.spatial == nil || len(w.spatial.cells) != count {
		w.spatial = &entityGrid{width: width, cells: make([][]*Entity, count)}
	}
	g := w.spatial
	for i := range g.cells {
		clear(g.cells[i])
		g.cells[i] = g.cells[i][:0]
	}
	g.radius = 0
	g.active = true
	for _, id := range w.IDs {
		if e := w.Entities[id]; e != nil {
			g.add(e)
		}
	}
}
func (g *entityGrid) cell(p Vec) int {
	return min(len(g.cells)-1, max(0, int(p.Y)/4*g.width+int(p.X)/4))
}
func (g *entityGrid) add(e *Entity) {
	i := g.cell(e.Position)
	g.cells[i] = append(g.cells[i], e)
	g.radius = math.Max(g.radius, definitions[e.Type].Radius)
}
func (w *World) positionEntity(e *Entity, p Vec) {
	if g := w.spatial; g != nil && g.active && g.cell(e.Position) != g.cell(p) {
		i := g.cell(e.Position)
		g.cells[i] = slices.DeleteFunc(g.cells[i], func(v *Entity) bool { return v == e })
		e.Position = p
		g.add(e)
		return
	}
	e.Position = p
}
func (w *World) nearby(p Vec, radius float64) iter.Seq[*Entity] {
	return func(yield func(*Entity) bool) {
		g := w.spatial
		if g == nil || !g.active {
			for _, id := range w.IDs {
				if e := w.Entities[id]; e != nil {
					if !yield(e) {
						return
					}
				}
			}
			return
		}
		for y := max(0, int(math.Floor((p.Y-radius)/4))); y <= min((len(g.cells)-1)/g.width, int((p.Y+radius)/4)); y++ {
			for x := max(0, int(math.Floor((p.X-radius)/4))); x <= min(g.width-1, int((p.X+radius)/4)); x++ {
				for _, e := range g.cells[y*g.width+x] {
					if w.Entities[e.ID] == e && !yield(e) {
						return
					}
				}
			}
		}
	}
}
func (w *World) collisionRadius(radius float64) float64 {
	if g := w.spatial; g != nil && g.active {
		return radius + g.radius
	}
	return radius + 6
}

func (w *World) nearestWithin(pos Vec, radius float64, pred func(*Entity) bool) *Entity {
	var best *Entity
	distance := math.Inf(1)
	for e := range w.nearby(pos, radius) {
		if e.Container != 0 || !pred(e) {
			continue
		}
		d := e.Position.Distance(pos)
		if d < distance || d == distance && (best == nil || e.ID < best.ID) {
			best, distance = e, d
		}
	}
	return best
}

// Open ground needs a collision-checked segment, not a whole-map A* search.
func (w *World) directPath(e *Entity, goal Vec, reach float64) []Vec {
	distance := e.Position.Distance(goal)
	if distance <= reach {
		return nil
	}
	travel := distance - math.Max(0, reach-1e-6)
	dx, dy := (goal.X-e.Position.X)/distance, (goal.Y-e.Position.Y)/distance
	end := Vec{e.Position.X + dx*travel, e.Position.Y + dy*travel}
	d := definitions[e.Type]
	for i, steps := 0, max(1, int(math.Ceil(travel*4))); i <= steps; i++ {
		t := float64(i) / float64(steps)
		p := Vec{e.Position.X + dx*travel*t, e.Position.Y + dy*travel*t}
		if !w.inside(p) || d.Naval && !w.water(p) || !d.Naval && !w.land(p) {
			return nil
		}
	}
	mid := Vec{(end.X + e.Position.X) / 2, (end.Y + e.Position.Y) / 2}
	for o := range w.nearby(mid, w.collisionRadius(travel/2+d.Radius)) {
		if o.ID == e.ID || o.Container != 0 {
			continue
		}
		od := definitions[o.Type]
		if od.Kind == "unit" {
			if od.Naval != d.Naval || o.Position.Distance(e.Position) < d.Radius+od.Radius {
				continue
			}
			// Units travelling in the same direction use reciprocal steering
			// for spacing; routing around every rank recreates a whole-map
			// search for each soldier. Opposing and stationary traffic remains
			// a route obstacle, and every accepted step still checks collision.
			if e.behavior.State() == Moving && o.behavior.State() == Moving && e.Order.Position != nil && o.Order.Position != nil {
				a, b := *e.Order.Position, *o.Order.Position
				ax, ay, bx, by := a.X-e.Position.X, a.Y-e.Position.Y, b.X-o.Position.X, b.Y-o.Position.Y
				if ax*bx+ay*by > .95*math.Hypot(ax, ay)*math.Hypot(bx, by) {
					continue
				}
			}
		} else if o.Type == "bridge" || o.Type == "farm" || o.Type == "fish" || o.Type == "relic" || o.Type == "sheep" || o.Type == "berries" || o.Type == "gate" && w.canUseGate(e, o) {
			continue
		}
		t := min(travel, max(0, (o.Position.X-e.Position.X)*dx+(o.Position.Y-e.Position.Y)*dy))
		if o.Position.Distance(Vec{e.Position.X + dx*t, e.Position.Y + dy*t}) < d.Radius+od.Radius-.05 {
			return nil
		}
	}
	return []Vec{end}
}
