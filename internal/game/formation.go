package game

import (
	"context"
	"math"
	"sort"
)

// March is immutable order data. Each unit's Moving instance still owns its
// order, interruptions and completion. There is no second group lifecycle.
// The pulse derives a shared pace from current members, never stale leaders.
type March struct {
	ID                      int
	Origin, Forward, Offset Vec
	Distance, Speed         float64
}
type marchSample struct {
	progress float64
	arrival  float64
	members  int
	blocked  bool
}

func marchOrigin(e *Entity, queued bool) Vec {
	if queued && e.behavior.State() != Idle {
		orders := append([]Order{e.Order}, e.Orders...)
		for i := len(orders) - 1; i >= 0; i-- {
			if orders[i].Position != nil && (orders[i].Kind == "move" || orders[i].Kind == "attack_move") {
				return *orders[i].Position
			}
		}
	}
	return e.Position
}

func (w *World) planMarch(es []*Entity, orders []Order, queued bool) {
	for _, naval := range []bool{false, true} {
		indices := []int{}
		origin := Vec{}
		speed, spacing := math.Inf(1), 1.2
		for i, e := range es {
			if (orders[i].Kind != "move" && orders[i].Kind != "attack_move") || definitions[e.Type].Naval != naval {
				continue
			}
			indices = append(indices, i)
			point := marchOrigin(e, queued)
			origin.X += point.X
			origin.Y += point.Y
			speed = math.Min(speed, w.stats(e).Speed)
			spacing = math.Max(spacing, definitions[e.Type].Radius*3.4+.35)
		}
		if len(indices) < 2 {
			continue
		}
		origin.X /= float64(len(indices))
		origin.Y /= float64(len(indices))
		goal := *orders[indices[0]].Position
		distance := origin.Distance(goal)
		if distance < .1 {
			distance = .1
		}
		forward := Vec{(goal.X - origin.X) / distance, (goal.Y - origin.Y) / distance}
		if origin == goal {
			forward = Vec{0, -1}
		}
		right := Vec{-forward.Y, forward.X}
		rank := func(e *Entity) int {
			d := definitions[e.Type]
			if d.Class == "siege" {
				return 2
			}
			if d.Projectile || d.Class == "worker" || e.Type == "monk" {
				return 1
			}
			return 0
		}
		sort.SliceStable(indices, func(i, j int) bool {
			a, b := es[indices[i]], es[indices[j]]
			if rank(a) != rank(b) {
				return rank(a) < rank(b)
			}
			ap, bp := marchOrigin(a, queued), marchOrigin(b, queued)
			pa, pb := ap.X*forward.X+ap.Y*forward.Y, bp.X*forward.X+bp.Y*forward.Y
			if pa != pb {
				return pa > pb
			}
			return a.ID < b.ID
		})
		columns := int(math.Ceil(math.Sqrt(float64(len(indices)))))
		rows := (len(indices) + columns - 1) / columns
		id := w.NextID
		w.NextID++
		occupied := []struct {
			position Vec
			radius   float64
		}{}
		for row := 0; row < rows; row++ {
			group := indices[row*columns : min(len(indices), (row+1)*columns)]
			sort.Slice(group, func(i, j int) bool {
				a, b := es[group[i]], es[group[j]]
				ap, bp := marchOrigin(a, queued), marchOrigin(b, queued)
				pa, pb := ap.X*right.X+ap.Y*right.Y, bp.X*right.X+bp.Y*right.Y
				if pa == pb {
					return a.ID < b.ID
				}
				return pa < pb
			})
			for col, i := range group {
				across := (float64(col) - float64(len(group)-1)/2) * spacing
				along := (float64(rows-1)/2 - float64(row)) * spacing
				offset := Vec{right.X*across + forward.X*along, right.Y*across + forward.Y*along}
				position := Vec{goal.X + offset.X, goal.Y + offset.Y}
				available := func(p Vec) bool {
					if !w.freeFor(es[i], p) {
						return false
					}
					for _, slot := range occupied {
						if p.Distance(slot.position) < definitions[es[i].Type].Radius+slot.radius+.2 {
							return false
						}
					}
					return true
				}
				// Obstructed edge slots find distinct nearby ground. Collapsing
				// several slots onto the center would prevent their completion.
				if !available(position) {
					preferred := position
					found := false
					for radius := spacing; radius <= spacing*float64(columns+2) && !found; radius += spacing {
						for angle := 0.; angle < 2*math.Pi; angle += math.Pi / 8 {
							candidate := Vec{preferred.X + math.Cos(angle)*radius, preferred.Y + math.Sin(angle)*radius}
							if available(candidate) {
								position, found = candidate, true
								break
							}
						}
					}
					if !found {
						position = goal
					}
				}
				occupied = append(occupied, struct {
					position Vec
					radius   float64
				}{position, definitions[es[i].Type].Radius})
				offset = Vec{position.X - goal.X, position.Y - goal.Y}
				orders[i].Position = &position
				orders[i].March = &March{ID: id, Origin: origin, Forward: forward, Offset: offset, Distance: distance, Speed: speed}
			}
		}
	}
}

func (w *World) sampleMarches() {
	clear(w.marches)
	if w.marches == nil {
		w.marches = map[int]marchSample{}
	}
	for _, id := range w.IDs {
		e := w.Entities[id]
		if e == nil || e.Order.March == nil || e.behavior.State() != Moving || e.Container != 0 {
			continue
		}
		m := e.Order.March
		progress := (e.Position.X-m.Origin.X-m.Offset.X)*m.Forward.X + (e.Position.Y-m.Origin.Y-m.Offset.Y)*m.Forward.Y
		s := w.marches[m.ID]
		if s.members == 0 || progress < s.progress {
			s.progress = progress
		}
		s.members++
		s.arrival = math.Max(s.arrival, m.Offset.Distance(Vec{})+1)
		w.marches[m.ID] = s
	}
	// A narrow passage temporarily releases the rank targets for every member.
	// Checking the whole footprint ahead prevents front ranks turning back
	// through a gate while the rear is still waiting on its approach.
	for _, id := range w.IDs {
		e := w.Entities[id]
		if e == nil || e.Order.March == nil || e.behavior.State() != Moving {
			continue
		}
		m := e.Order.March
		s := w.marches[m.ID]
		for ahead := 0.; ahead <= 3; ahead += .5 {
			p := min(m.Distance, max(0, s.progress)+ahead)
			goal := Vec{m.Origin.X + m.Forward.X*p + m.Offset.X, m.Origin.Y + m.Forward.Y*p + m.Offset.Y}
			if !w.freeFor(e, goal) {
				s.blocked = true
				break
			}
		}
		w.marches[m.ID] = s
	}
}

func moveArrival(e *Entity) float64 {
	if e.Order.March != nil {
		return .16
	}
	return .8
}

func advanceMarch(_ context.Context, c *unitContext) error {
	w, e := c.World, c.Actor
	goal, dt := *e.Order.Position, Step
	if m := e.Order.March; m != nil {
		s := w.marches[m.ID]
		// Release shared pacing across the whole arrival footprint. Otherwise
		// soldiers already at their final slots can trap the last ranks behind
		// intermediate targets that keep pulling them away from their slots.
		if s.members > 1 && !s.blocked && s.progress < m.Distance-s.arrival {
			progress := math.Min(m.Distance, math.Max(0, s.progress)+.9)
			goal = Vec{m.Origin.X + m.Forward.X*progress + m.Offset.X, m.Origin.Y + m.Forward.Y*progress + m.Offset.Y}
			if !w.freeFor(e, goal) {
				goal = Vec{m.Origin.X + m.Forward.X*progress, m.Origin.Y + m.Forward.Y*progress}
			}
			// An obstructed slot uses its individual route through a choke.
			if !w.freeFor(e, goal) || len(e.Path) == 0 && e.Repath > 0 {
				goal = *e.Order.Position
			}
		}
		// Stragglers can catch up at their own speed; the front keeps the
		// formation's slowest pace. No unit exceeds its authoritative speed.
		if e.Position.Distance(goal) < 1.8 {
			dt *= math.Min(1, m.Speed/w.stats(e).Speed)
		}
	}
	w.move(e, goal, moveArrival(e), dt)
	return nil
}
