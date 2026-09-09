package game

import "math"

func barrier(typ string) bool { return typ == "wall" || typ == "palisade" || typ == "gate" }

func (w *World) barrierAt(pos Vec) *Entity {
	if w.barriers == nil {
		w.barriers = map[Vec]*Entity{}
		for _, id := range w.IDs {
			if e := w.Entities[id]; e != nil && barrier(e.Type) {
				w.barriers[e.Position] = e
			}
		}
	}
	return w.barriers[pos]
}

func (w *World) barrierLinks(e *Entity, player int) []Vec {
	var links []Vec
	if !barrier(e.Type) {
		return links
	}
	for _, delta := range []Vec{{0, -1}, {1, 0}, {0, 1}, {-1, 0}} {
		other := w.barrierAt(Vec{e.Position.X + delta.X, e.Position.Y + delta.Y})
		if other != nil && other.Owner == e.Owner && (player == e.Owner || player > 0 && w.visibleEntity(player, other)) {
			links = append(links, delta)
		}
	}
	return links
}

func (w *World) straightGate(player int, pos Vec, planned map[Vec]bool) bool {
	horizontal, vertical := false, false
	for _, delta := range []Vec{{0, -1}, {1, 0}, {0, 1}, {-1, 0}} {
		point := Vec{pos.X + delta.X, pos.Y + delta.Y}
		neighbour := w.barrierAt(point)
		if planned[point] || neighbour != nil && neighbour.Owner == player {
			horizontal = horizontal || delta.X != 0
			vertical = vertical || delta.Y != 0
		}
	}
	return !horizontal || !vertical
}

// Wall drags rasterize to cardinally connected tiles, so even diagonal drags
// form a continuous barrier. Endpoints are intent; only Go chooses segments.
func wallRoute(start, end Vec) ([]Vec, error) {
	start, end = snap(start), snap(end)
	dx, dy := math.Abs(end.X-start.X), math.Abs(end.Y-start.Y)
	if dx+dy+1 > 64 {
		return nil, rule("wall_too_long", "Draw up to 64 wall segments at a time.")
	}
	sx, sy := 1., 1.
	if start.X > end.X {
		sx = -1
	}
	if start.Y > end.Y {
		sy = -1
	}
	points := []Vec{start}
	pos := start
	for pos != end {
		// Pick the next axis by distance from the ideal straight line.
		if pos.X != end.X && (pos.Y == end.Y || math.Abs(math.Abs(pos.X+sx-start.X)*dy-math.Abs(pos.Y-start.Y)*dx) <= math.Abs(math.Abs(pos.X-start.X)*dy-(math.Abs(pos.Y-start.Y)+1)*dx)) {
			pos.X += sx
		} else {
			pos.Y += sy
		}
		points = append(points, pos)
	}
	return points, nil
}

// PlanBuilding is side-effect free: callers get the exact new segments and
// aggregate price, or a refusal. Existing friendly barriers may anchor a line;
// an owned wall/palisade at a gate site is replaced only after validation.
func (w *World) PlanBuilding(player int, typ string, start Vec, end *Vec) ([]Vec, Resources, error) {
	d, ok := definitions[typ]
	if !ok || d.Kind != "building" {
		return nil, Resources{}, rule("unknown_product", "Unknown building.")
	}
	if !start.Finite() || !w.inside(start) || end != nil && (!end.Finite() || !w.inside(*end)) {
		return nil, Resources{}, rule("invalid_position", "Choose a site inside the map.")
	}
	points := []Vec{snap(start)}
	if end != nil {
		if typ != "wall" && typ != "palisade" {
			return nil, Resources{}, rule("invalid_wall", "Drag placement is available for walls and palisades.")
		}
		var err error
		points, err = wallRoute(start, *end)
		if err != nil {
			return nil, Resources{}, err
		}
	}
	var sites []Vec
	for _, pos := range points {
		if e := w.barrierAt(pos); e != nil && e.Owner == player && typ != "gate" && barrier(typ) {
			continue
		}
		if err := w.Placement(player, typ, pos); err != nil {
			return points, d.Cost.Scale(float64(len(points))), err
		}
		sites = append(sites, pos)
	}
	price := d.Cost.Scale(float64(len(sites)))
	if barrier(typ) {
		planned := make(map[Vec]bool, len(sites))
		for _, pos := range sites {
			planned[pos] = true
		}
		// Two new segments must not make a corner around a standalone gate.
		// Single-site validation cannot see the other foundations in this order.
		for _, pos := range sites {
			for _, delta := range []Vec{{0, -1}, {1, 0}, {0, 1}, {-1, 0}} {
				gate := w.barrierAt(Vec{pos.X + delta.X, pos.Y + delta.Y})
				if gate != nil && gate.Type == "gate" && gate.Owner == player && !w.straightGate(player, gate.Position, planned) {
					return sites, price, rule("invalid_placement", "Connect walls along one straight line through each gate.")
				}
			}
		}
	}
	if len(sites) == 0 {
		return nil, price, rule("already_built", "A wall already occupies this route.")
	}
	if err := w.canBuild(w.Players[player], d); err != nil {
		return sites, price, err
	}
	if !w.Players[player].Resources.CanPay(price) {
		return sites, price, rule("insufficient_resources", "Gather enough resources for the entire wall line, or draw a shorter line.")
	}
	return sites, price, nil
}
