package game

import (
	"reflect"
	"slices"
)

// SnapshotFrame is the opt-in delta-v1 SSE contract. Sequence belongs to this
// connection, not the simulation tick (commands also change paused worlds).
// Every connection starts with Snapshot. Delta applies only to Base; missing
// frames require reconnecting for a new complete authorized snapshot.
type SnapshotFrame struct {
	Sequence int            `json:"sequence"`
	Base     int            `json:"base"`
	SampleMS float64        `json:"sample_ms"`
	Snapshot *Snapshot      `json:"snapshot,omitempty"`
	Delta    *SnapshotDelta `json:"delta,omitempty"`
}

type MapCell struct {
	Index int  `json:"index"`
	Tile  Tile `json:"tile"`
	Fog   int  `json:"fog"`
}

type SnapshotDelta struct {
	ControlRevision int              `json:"control_revision"`
	Tick            int              `json:"tick"`
	Time            float64          `json:"time"`
	Speed           float64          `json:"speed"`
	Paused          bool             `json:"paused"`
	Status          string           `json:"status"`
	Winner          int              `json:"winner"`
	TreatyRemaining float64          `json:"treaty_remaining"`
	Player          *PlayerView      `json:"player,omitempty"`
	Opponents       *[]OpponentView  `json:"opponents,omitempty"`
	Cells           []MapCell        `json:"cells,omitempty"`
	Entities        []EntityView     `json:"entities,omitempty"`
	RemovedEntities []int            `json:"removed_entities,omitempty"`
	Projectiles     []ProjectileView `json:"projectiles"`
	Events          *[]Event         `json:"events,omitempty"`
	EventCursor     int              `json:"event_cursor"`
	BuildOptions    *[]Action        `json:"build_options,omitempty"`
}

// SnapshotStream owns only the last authorized read model for one subscriber.
// It never reads a World, shares another user's baseline, or retains a backlog.
type SnapshotStream struct {
	previous *Snapshot
	sequence int
	entities map[int]EntityView
}

func (s *SnapshotStream) Next(v Snapshot, sampleMS float64) SnapshotFrame {
	s.sequence++
	f := SnapshotFrame{Sequence: s.sequence, Base: s.sequence - 1, SampleMS: sampleMS}
	p := s.previous
	if p == nil || p.Version != v.Version || p.World != v.World || p.Player.ID != v.Player.ID || p.Map.Width != v.Map.Width || p.Map.Height != v.Map.Height || p.Map.Biome != v.Map.Biome {
		f.Base, f.Snapshot = 0, &v
	} else {
		d := &SnapshotDelta{ControlRevision: v.ControlRevision, Tick: v.Tick, Time: v.Time, Speed: v.Speed, Paused: v.Paused, Status: v.Status, Winner: v.Winner, TreatyRemaining: v.TreatyRemaining, Projectiles: v.Projectiles, EventCursor: v.EventCursor}
		if !reflect.DeepEqual(p.Player, v.Player) {
			d.Player = &v.Player
		}
		if !slices.Equal(p.Opponents, v.Opponents) {
			d.Opponents = &v.Opponents
		}
		if !slices.Equal(p.Events, v.Events) {
			d.Events = &v.Events
		}
		if !reflect.DeepEqual(p.BuildOptions, v.BuildOptions) {
			d.BuildOptions = &v.BuildOptions
		}
		// Map views share immutable arrays until fog or terrain changes.
		if len(v.Map.Tiles) > 0 && (&p.Map.Tiles[0] != &v.Map.Tiles[0] || &p.Map.Fog[0] != &v.Map.Fog[0]) {
			for i, tile := range v.Map.Tiles {
				if tile != p.Map.Tiles[i] || v.Map.Fog[i] != p.Map.Fog[i] {
					d.Cells = append(d.Cells, MapCell{Index: i, Tile: tile, Fog: v.Map.Fog[i]})
				}
			}
		}
		for _, e := range v.Entities {
			if old, exists := s.entities[e.ID]; !exists || !reflect.DeepEqual(old, e) {
				d.Entities = append(d.Entities, e)
			}
			delete(s.entities, e.ID)
		}
		for id := range s.entities {
			d.RemovedEntities = append(d.RemovedEntities, id)
		}
		slices.Sort(d.RemovedEntities)
		f.Delta = d
	}
	s.entities = make(map[int]EntityView, len(v.Entities))
	for _, e := range v.Entities {
		s.entities[e.ID] = e
	}
	s.previous = &v
	return f
}
