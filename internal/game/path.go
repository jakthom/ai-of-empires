package game

import (
	"container/heap"
	"math"
)

func (w *World) free(p Vec, r float64, ignore int, naval bool) bool {
	if !w.inside(p) {
		return false
	}
	if naval && !w.water(p) || !naval && !w.land(p) {
		return false
	}
	for _, id := range w.IDs {
		e := w.Entities[id]
		if e == nil || id == ignore || e.Container != 0 {
			continue
		}
		d := definitions[e.Type]
		if d.Kind == "unit" || e.Type == "farm" || e.Type == "relic" || e.Type == "fish" || e.Type == "gate" || e.Type == "sheep" || e.Type == "berries" {
			continue
		}
		if p.Distance(e.Position) < r+d.Radius {
			return false
		}
	}
	return true
}

type pathNode struct {
	i int
	f float64
}
type nodeHeap []pathNode

func (h nodeHeap) Len() int { return len(h) }
func (h nodeHeap) Less(i, j int) bool {
	if h[i].f == h[j].f {
		return h[i].i < h[j].i
	}
	return h[i].f < h[j].f
}
func (h nodeHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *nodeHeap) Push(v any)   { *h = append(*h, v.(pathNode)) }
func (h *nodeHeap) Pop() any     { a := *h; v := a[len(a)-1]; *h = a[:len(a)-1]; return v }

type pathObstacle struct {
	position Vec
	radius   float64
}
type pathObstacles struct {
	width, height int
	cells         [][]pathObstacle
}

func (o *pathObstacles) add(obstacle pathObstacle) {
	for y := max(0, int((obstacle.position.Y-obstacle.radius)/4)); y <= min(o.height-1, int((obstacle.position.Y+obstacle.radius)/4)); y++ {
		for x := max(0, int((obstacle.position.X-obstacle.radius)/4)); x <= min(o.width-1, int((obstacle.position.X+obstacle.radius)/4)); x++ {
			o.cells[y*o.width+x] = append(o.cells[y*o.width+x], obstacle)
		}
	}
}

// A path edge must have the same clearance as continuous unit movement. Free
// tile centers alone do not guarantee that a unit can pass between them.
func (w *World) clearPathSegment(from, to Vec, naval bool, obstacles *pathObstacles) bool {
	dx, dy := to.X-from.X, to.Y-from.Y
	lengthSquared := dx*dx + dy*dy
	steps := max(1, int(math.Ceil(math.Sqrt(lengthSquared)*4)))
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		p := Vec{from.X + dx*t, from.Y + dy*t}
		if !w.inside(p) || naval && !w.water(p) || !naval && !w.land(p) {
			return false
		}
	}
	// Only nearby obstacle footprints can intersect a short path edge. This
	// keeps a route search independent of forests on the other side of a map.
	for y := max(0, int(math.Min(from.Y, to.Y)/4)); y <= min(obstacles.height-1, int(math.Max(from.Y, to.Y)/4)); y++ {
		for x := max(0, int(math.Min(from.X, to.X)/4)); x <= min(obstacles.width-1, int(math.Max(from.X, to.X)/4)); x++ {
			for _, obstacle := range obstacles.cells[y*obstacles.width+x] {
				t := 0.0
				if lengthSquared > 0 {
					t = math.Max(0, math.Min(1, ((obstacle.position.X-from.X)*dx+(obstacle.position.Y-from.Y)*dy)/lengthSquared))
				}
				x, y := from.X+dx*t-obstacle.position.X, from.Y+dy*t-obstacle.position.Y
				if x*x+y*y < obstacle.radius*obstacle.radius {
					return false
				}
			}
		}
	}
	return true
}

func (w *World) path(e *Entity, goal Vec, reach float64) []Vec {
	path := w.findPath(e, goal, reach, true)
	if len(path) == 0 {
		// Traffic can temporarily fill every coarse path cell in a narrow
		// passage. Approach on a static route so reciprocal steering can
		// make room, instead of waiting forever for both sides to move first.
		return w.findPath(e, goal, reach, false)
	}
	return path
}
func (w *World) findPath(e *Entity, goal Vec, reach float64, avoidUnits bool) []Vec {
	d := w.stats(e)
	n := len(w.Tiles)
	blocked := make([]bool, n)
	obstacles := &pathObstacles{width: (w.Width + 3) / 4, height: (w.Height + 3) / 4}
	obstacles.cells = make([][]pathObstacle, obstacles.width*obstacles.height)
	for i, t := range w.Tiles {
		blocked[i] = t.Terrain == "cliff" || (!d.Naval && t.Terrain == "water") || (d.Naval && t.Terrain != "water" && t.Terrain != "shallows")
	}
	for _, id := range w.IDs {
		o := w.Entities[id]
		if o == nil || id == e.ID || o.Container != 0 {
			continue
		}
		od := definitions[o.Type]
		if od.Kind == "unit" {
			// Route around occupied space, including workers already at a
			// resource. Nearby overlapping traffic is separated by steering.
			if !avoidUnits || od.Naval != d.Naval || o.Position.Distance(e.Position) < d.Radius+od.Radius {
				continue
			}
		} else if o.Type == "farm" || o.Type == "fish" || o.Type == "relic" || o.Type == "sheep" || o.Type == "berries" || o.Type == "gate" && o.Owner == e.Owner && o.life.State() == Active {
			continue
		}
		r := od.Radius + d.Radius - .05
		obstacles.add(pathObstacle{o.Position, r})
		for y := max(0, int(o.Position.Y-r)); y < min(w.Height, int(o.Position.Y+r)+1); y++ {
			for x := max(0, int(o.Position.X-r)); x < min(w.Width, int(o.Position.X+r)+1); x++ {
				if o.Position.Distance(Vec{float64(x) + .5, float64(y) + .5}) < r {
					blocked[y*w.Width+x] = true
				}
			}
		}
	}
	start := int(e.Position.Y)*w.Width + int(e.Position.X)
	blocked[start] = false
	costs := make([]float64, n)
	parents := make([]int, n)
	closed := make([]bool, n)
	for i := range costs {
		costs[i] = math.Inf(1)
		parents[i] = -1
	}
	costs[start] = 0
	q := &nodeHeap{}
	heap.Push(q, pathNode{start, 0})
	end := -1
	var arrival Vec
	dirs := []struct{ x, y int }{{1, 0}, {0, 1}, {-1, 0}, {0, -1}, {1, 1}, {-1, 1}, {-1, -1}, {1, -1}}
	for q.Len() > 0 {
		cur := heap.Pop(q).(pathNode).i
		if closed[cur] {
			continue
		}
		closed[cur] = true
		x, y := cur%w.Width, cur/w.Width
		pos := Vec{float64(x) + .5, float64(y) + .5}
		if cur == start {
			pos = e.Position
		}
		distance := pos.Distance(goal)
		if distance <= reach {
			end = cur
			arrival = pos
			break
		}
		// Small deposits can block every tile center inside interaction range.
		// Finish with an exact, collision-checked point on the approach instead
		// of requiring the worker's final position to be a tile center.
		if distance <= reach+math.Sqrt2 {
			arrivalRadius := math.Max(0, reach-1e-6)
			candidate := Vec{goal.X + (pos.X-goal.X)*arrivalRadius/distance, goal.Y + (pos.Y-goal.Y)*arrivalRadius/distance}
			if w.clearPathSegment(pos, candidate, d.Naval, obstacles) {
				end, arrival = cur, candidate
				break
			}
		}
		for _, v := range dirs {
			nx, ny := x+v.x, y+v.y
			if nx < 1 || ny < 1 || nx >= w.Width-1 || ny >= w.Height-1 {
				continue
			}
			ni := ny*w.Width + nx
			if blocked[ni] || closed[ni] {
				continue
			}
			if v.x != 0 && v.y != 0 && (blocked[y*w.Width+nx] || blocked[ny*w.Width+x]) {
				continue
			}
			if !w.clearPathSegment(pos, Vec{float64(nx) + .5, float64(ny) + .5}, d.Naval, obstacles) {
				continue
			}
			c := costs[cur] + math.Hypot(float64(v.x), float64(v.y))
			if c < costs[ni] {
				costs[ni] = c
				parents[ni] = cur
				h := math.Max(0, Vec{float64(nx) + .5, float64(ny) + .5}.Distance(goal)-reach)
				heap.Push(q, pathNode{ni, c + h})
			}
		}
	}
	if end < 0 {
		return nil
	}
	out := []Vec{}
	for i := end; i != start && i >= 0; i = parents[i] {
		out = append(out, Vec{float64(i%w.Width) + .5, float64(i/w.Width) + .5})
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	if len(out) == 0 || out[len(out)-1].Distance(arrival) > 1e-8 {
		out = append(out, arrival)
	}
	return out
}
func (w *World) move(e *Entity, goal Vec, reach, dt float64) bool {
	if e.Position.Distance(goal) <= reach {
		return true
	}
	if e.Type == "trebuchet" && e.siege.State() != SiegePacked {
		return false
	}
	e.Repath -= dt
	if len(e.Path) == 0 && e.Repath > 0 && e.PathGoal.Distance(goal) <= 1.5 {
		return false
	}
	if len(e.Path) == 0 || e.PathGoal.Distance(goal) > 1.5 || e.Repath <= 0 {
		e.Path = w.path(e, goal, reach)
		e.PathGoal = goal
		e.Repath = 2
		if len(e.Path) == 0 {
			return false
		}
	}
	dest := e.Path[0]
	dx, dy := dest.X-e.Position.X, dest.Y-e.Position.Y
	dist := math.Hypot(dx, dy)
	step := w.stats(e).Speed * dt
	if dist < step {
		e.Path = e.Path[1:]
		step = dist
	}
	if dist <= 1e-9 {
		return false
	}
	pos := Vec{e.Position.X + dx/dist*step, e.Position.Y + dy/dist*step}
	// Buildings and terrain are authoritative; a late obstruction invalidates paths.
	if !w.freeFor(e, pos) {
		e.Path = nil
		e.Repath = 0
		return false
	}
	if !w.clearUnitStep(e, pos) {
		found := false
		// Both sides of head-on traffic prefer their own right side. No
		// random sampling or teleporting; each accepted step stays walkable.
		for _, angle := range []float64{math.Pi / 4, -math.Pi / 4, math.Pi / 2, -math.Pi / 2, 3 * math.Pi / 4, -3 * math.Pi / 4} {
			x, y := dx/dist*step, dy/dist*step
			candidate := Vec{e.Position.X + x*math.Cos(angle) - y*math.Sin(angle), e.Position.Y + x*math.Sin(angle) + y*math.Cos(angle)}
			if w.freeFor(e, candidate) && w.clearUnitStep(e, candidate) {
				pos, found = candidate, true
				break
			}
		}
		if !found {
			e.Path = nil
			e.Repath = math.Min(e.Repath, .5)
			return false
		}
	}
	e.Position = pos
	return e.Position.Distance(goal) <= reach
}

func (w *World) clearUnitStep(actor *Entity, pos Vec) bool {
	d := definitions[actor.Type]
	for _, id := range w.IDs {
		o := w.Entities[id]
		if o == nil || id == actor.ID || o.Container != 0 {
			continue
		}
		od := definitions[o.Type]
		if od.Kind != "unit" || od.Naval != d.Naval {
			continue
		}
		distance := pos.Distance(o.Position)
		if distance < (d.Radius+od.Radius)*.85 && distance <= actor.Position.Distance(o.Position)+1e-9 {
			return false
		}
	}
	return true
}
func (w *World) freeFor(e *Entity, pos Vec) bool {
	d := definitions[e.Type]
	if !w.inside(pos) || d.Naval && !w.water(pos) || !d.Naval && !w.land(pos) {
		return false
	}
	for _, id := range w.IDs {
		o := w.Entities[id]
		if o == nil || id == e.ID || o.Container != 0 {
			continue
		}
		od := definitions[o.Type]
		if od.Kind == "unit" || o.Type == "farm" || o.Type == "fish" || o.Type == "relic" || o.Type == "sheep" || o.Type == "berries" || o.Type == "gate" && o.Owner == e.Owner && o.life.State() == Active {
			continue
		}
		if pos.Distance(o.Position) < d.Radius+od.Radius-.05 {
			return false
		}
	}
	return true
}
func (w *World) exit(b, unit *Entity) (Vec, bool) {
	d := definitions[b.Type]
	for r := d.Radius + .7; r < d.Radius+4; r += .5 {
		for a := 0.; a < math.Pi*2; a += .3 {
			pos := Vec{b.Position.X + math.Cos(a)*r, b.Position.Y + math.Sin(a)*r}
			if w.freeFor(unit, pos) && w.unitSpace(unit, pos, 1) {
				return pos, true
			}
		}
	}
	return Vec{}, false
}

func (w *World) unitSpace(actor *Entity, pos Vec, clearance float64) bool {
	d := definitions[actor.Type]
	for _, id := range w.IDs {
		o := w.Entities[id]
		if o == nil || o.ID == actor.ID || o.Container != 0 {
			continue
		}
		other := definitions[o.Type]
		if other.Kind == "unit" && other.Naval == d.Naval && pos.Distance(o.Position) < (d.Radius+other.Radius)*clearance {
			return false
		}
	}
	return true
}
