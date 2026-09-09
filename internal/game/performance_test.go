package game

import (
	"encoding/json"
	"testing"
)

// An unreachable order is common after construction closes a route. It must
// remain cheap while a unit waits to retry, even in a long-running match.
func BenchmarkBlockedMovement(b *testing.B) {
	w := New(Config{Difficulty: "peaceful", Seed: 4817})
	worker := w.entities(1, "villager")[0]
	goal := Vec{14, 12} // The middle of the lake cannot be reached on foot.
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		w.move(worker, goal, .8, Step)
	}
}

func BenchmarkPopulatedSnapshot(b *testing.B) {
	w := New(Config{Difficulty: "peaceful", Seed: 4817})
	for i := range 120 {
		w.spawn("villager", 1, Vec{18 + float64(i%10)*.5, 40 + float64(i/10)*.5})
	}
	w.refreshVisibility()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = w.View(1)
	}
}

func performanceHugeWorld() *World {
	return New(Config{Difficulty: "peaceful", Seed: 82731, Settlements: 6, World: WorldOptions{Type: "mountain_lakes", Biome: "mixed", Size: "huge", Reveal: "all"}})
}

func BenchmarkHugeSimulation(b *testing.B) {
	w := performanceHugeWorld()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		w.Update()
	}
}

func BenchmarkHugeSnapshotStream(b *testing.B) {
	w := performanceHugeWorld()
	var s SnapshotStream
	s.Next(w.View(1), 0)
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		f := s.Next(w.View(1), float64(i+1)*50)
		if _, err := json.Marshal(f); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHugeCheckpointCapture(b *testing.B) {
	w := performanceHugeWorld()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = w.CaptureCheckpoint()
	}
}
