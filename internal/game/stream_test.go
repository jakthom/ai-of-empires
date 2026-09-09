package game

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
	"testing"
)

func applyStreamFrame(v Snapshot, f SnapshotFrame) Snapshot {
	if f.Snapshot != nil {
		return *f.Snapshot
	}
	d := f.Delta
	v.ControlRevision, v.Tick, v.Time, v.Speed, v.Paused, v.Status, v.Winner, v.TreatyRemaining = d.ControlRevision, d.Tick, d.Time, d.Speed, d.Paused, d.Status, d.Winner, d.TreatyRemaining
	v.Projectiles, v.EventCursor = d.Projectiles, d.EventCursor
	if d.Player != nil {
		v.Player = *d.Player
	}
	if d.Marketplace != nil {
		v.Marketplace = *d.Marketplace
	}
	if d.Opponents != nil {
		v.Opponents = *d.Opponents
	}
	if d.Events != nil {
		v.Events = *d.Events
	}
	if d.BuildOptions != nil {
		v.BuildOptions = *d.BuildOptions
	}
	if len(d.Cells) > 0 {
		v.Map.Tiles, v.Map.Fog = slices.Clone(v.Map.Tiles), slices.Clone(v.Map.Fog)
		for _, cell := range d.Cells {
			v.Map.Tiles[cell.Index], v.Map.Fog[cell.Index] = cell.Tile, cell.Fog
		}
	}
	entities := map[int]EntityView{}
	for _, e := range v.Entities {
		entities[e.ID] = e
	}
	for _, id := range d.RemovedEntities {
		delete(entities, id)
	}
	for _, e := range d.Entities {
		entities[e.ID] = e
	}
	v.Entities = make([]EntityView, 0, len(entities))
	for _, e := range entities {
		v.Entities = append(v.Entities, e)
	}
	return v
}

func sortedSnapshotJSON(v Snapshot) []byte {
	v.Entities = slices.Clone(v.Entities)
	slices.SortFunc(v.Entities, func(a, b EntityView) int { return a.ID - b.ID })
	b, _ := json.Marshal(v)
	return b
}

func TestDeltaStreamReconstructsEachPlayersAuthorizedView(t *testing.T) {
	w := New(Config{Seed: 4817, Difficulty: "peaceful", Settlements: 2, Mode: "sandbox"})
	streams := []*SnapshotStream{{}, {}}
	views := make([]Snapshot, 2)
	for sample := range 60 {
		if sample == 10 {
			e := w.entities(1, "villager")[0]
			if err := w.Apply(1, Command{Kind: "move", EntityIDs: []int{e.ID}, Position: &Vec{30, 40}}); err != nil {
				t.Fatal(err)
			}
		}
		if sample == 20 {
			w.remove(w.entities(1, "villager")[1].ID)
		}
		if sample == 40 {
			if err := w.SetPaused(true); err != nil {
				t.Fatal(err)
			}
		}
		for range 4 {
			w.Update()
		}
		for i, s := range streams {
			want := w.View(i + 1)
			f := s.Next(want, float64(sample*50))
			if f.Sequence != sample+1 || sample == 0 && f.Snapshot == nil || sample > 0 && (f.Delta == nil || f.Base != sample) {
				t.Fatal("invalid stream baseline")
			}
			// Exercise the actual JSON boundary, including omitted fields and
			// empty arrays, before applying the patch.
			data, err := json.Marshal(f)
			if err != nil {
				t.Fatal(err)
			}
			var wire SnapshotFrame
			if err := json.Unmarshal(data, &wire); err != nil {
				t.Fatal(err)
			}
			views[i] = applyStreamFrame(views[i], wire)
			if !bytes.Equal(sortedSnapshotJSON(want), sortedSnapshotJSON(views[i])) {
				t.Fatalf("player %d differs at sample %d", i+1, sample)
			}
		}
	}
	// A new connection never depends on a previous tab's private baseline.
	var reconnect SnapshotStream
	if f := reconnect.Next(w.View(2), 0); f.Sequence != 1 || f.Base != 0 || f.Snapshot == nil {
		t.Fatal("reconnect did not rebase")
	}
}

func TestMapProjectionIsImmutableAndPrivate(t *testing.T) {
	w := New(Config{Difficulty: "peaceful"})
	v := w.View(1)
	before, _ := json.Marshal(v.Map)
	next := w.View(1)
	if &v.Map.Tiles[0] != &next.Map.Tiles[0] {
		t.Fatal("unchanged terrain was copied")
	}
	i := 0
	w.Players[1].Explored[i], w.Players[1].Visible[i] = true, true
	w.Tiles[i] = Tile{Terrain: "cliff", Elevation: 7}
	next = w.View(1)
	if next.Map.Fog[i] != 2 || next.Map.Tiles[i].Elevation != 7 {
		t.Fatal("new observations were lost")
	}
	after, _ := json.Marshal(v.Map)
	if !bytes.Equal(before, after) {
		t.Fatal("updating fog changed an in-flight snapshot")
	}
	other := w.View(2)
	if other.Map.Tiles[i].Terrain != "unknown" {
		t.Fatal("another player's map revealed unseen terrain")
	}
	w.Players[1].Visible[i] = false
	if m := w.View(1).Map; m.Fog[i] != 1 || m.Tiles[i] != next.Map.Tiles[i] {
		t.Fatal("forgot explored terrain when visibility ended")
	}
}

func TestIdleDeltaOmitsMapAndEntitiesButClearsOptionalEntityFields(t *testing.T) {
	w := New(Config{Difficulty: "peaceful"})
	var s SnapshotStream
	v := w.View(1)
	s.Next(v, 0)
	f := s.Next(w.View(1), 50)
	if len(f.Delta.Cells) != 0 || len(f.Delta.Entities) != 0 || f.Delta.Player != nil {
		t.Fatal("resent unchanged read models")
	}
	e := w.entities(1, "town_center")[0]
	e.Rally = &Vec{22, 40}
	withRally := w.View(1)
	s.Next(withRally, 100)
	e.Rally = nil
	want := w.View(1)
	f = s.Next(want, 150)
	got := applyStreamFrame(withRally, f)
	if !bytes.Equal(sortedSnapshotJSON(got), sortedSnapshotJSON(want)) {
		t.Fatal("optional field survived replacement")
	}
	if reflect.DeepEqual(withRally.Entities, want.Entities) {
		t.Fatal("test did not change an entity")
	}
}
