package game

import (
	"context"
	"math"
)

// Even footprints sit on grid intersections; odd footprints sit at tile
// centers. Placement and previews use the same authoritative snap function.
func buildingPosition(typ string, p Vec) Vec {
	if n := definitions[typ].Footprint; n > 0 && n%2 == 0 {
		return Vec{math.Round(p.X), math.Round(p.Y)}
	}
	return snap(p)
}

func footprintOverlaps(a Vec, side float64, b Vec, other float64) bool {
	return math.Abs(a.X-b.X) < (side+other)/2-1e-8 && math.Abs(a.Y-b.Y) < (side+other)/2-1e-8
}

func (w *World) nextConstruction(e *Entity) *Entity {
	if e.Order.BuildGroup == 0 || len(e.Orders) > 0 {
		return nil
	}
	return w.nearest(e.Position, func(next *Entity) bool {
		return next.ID != e.Order.Target && next.Owner == e.Owner && next.BuildGroup == e.Order.BuildGroup && next.life.State() == Foundation && w.reachableFootprint(e, next.Position, definitions[next.Type].Radius+.8)
	})
}

func constructionContinues(_ context.Context, c *unitContext) error {
	return applicable(c.Candidate != nil && (c.Target == nil || c.Target.Owner != c.Actor.Owner || c.Target.life.State() == Active))
}

func continueConstruction(_ context.Context, c *unitContext) error {
	c.Actor.Order.Target = c.Candidate.ID
	c.Actor.Order.BuildGroup = c.Candidate.BuildGroup
	c.Actor.Path = nil
	c.Actor.Repath = 0
	c.World.record(Event{Kind: "retargeted", Message: "Continuing the construction batch", TargetID: c.Candidate.ID}, c.Actor, 0)
	return nil
}

func queuedConstructionFinished(_ context.Context, c *unitContext) error {
	return applicable(len(c.Actor.Orders) > 0 && c.Target != nil && c.Target.life.State() == Active)
}
