package game

import (
	"bytes"
	"github.com/open-ships/statemachine"
	"reflect"
	"testing"
)

// Destroy mutable exported fields after capturing to detect new persistence
// fields accidentally sharing maps, slices or pointed-to data with play.
func eraseCheckpointReferences(v reflect.Value) {
	switch v.Kind() {
	case reflect.Pointer:
		if !v.IsNil() {
			eraseCheckpointReferences(v.Elem())
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).IsExported() {
				eraseCheckpointReferences(v.Field(i))
			}
		}
	case reflect.Map:
		for _, key := range v.MapKeys() {
			eraseCheckpointReferences(v.MapIndex(key))
		}
		v.Clear()
	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			eraseCheckpointReferences(v.Index(i))
			if v.Index(i).CanSet() {
				v.Index(i).SetZero()
			}
		}
	default:
		if v.CanSet() {
			v.SetZero()
		}
	}
}

func TestCapturedCheckpointHasNoMutableAliases(t *testing.T) {
	w := New(Config{Mode: "sandbox", Difficulty: "expert", Settlements: 4})
	e := w.entities(1, "villager")[0]
	e.Orders = []Order{{Kind: "move", Position: &Vec{31, 40}, DefendFrom: &Vec{20, 40}}}
	e.Rally, e.Path, e.Passengers = &Vec{30, 40}, []Vec{{24, 40}}, []int{123}
	p := w.Players[1]
	p.Economy.Consumption = map[string]Resources{"food_upkeep": {Food: 2}}
	p.Economy.History = []EconomicSample{{Time: 10, Population: 4}}
	p.Production.OutBuckets = []Resources{{Food: 2}}
	p.UserAliases = []string{"previous-user"}
	p.AIPlan.Army, p.AIPlan.ScoutGoal, p.AIPlan.Surveyed = []int{e.ID}, &Vec{30, 41}, map[int]float64{2: 1}
	p.NavalPlan.Crew = []int{e.ID}
	p.Memory[e.ID] = EntityView{Rally: &Vec{20, 40}, Actions: []Action{{Gain: &Resources{Gold: 1}}}, Tasks: []Task{{Remaining: 2}}, Connections: []Vec{{20, 41}}, Passengers: []int{1}}
	w.Projectiles = []Projectile{{ID: 10001, Owner: 1, Position: Vec{20, 40}, flight: statemachine.NewInstance(flightMachine, Flying)}}
	want, err := w.Checkpoint()
	if err != nil {
		t.Fatal(err)
	}
	captured := w.CaptureCheckpoint()
	eraseCheckpointReferences(reflect.ValueOf(w))
	got, err := captured.Encode()
	if err != nil || !bytes.Equal(want, got) {
		t.Fatalf("captured state changed after live mutation: %v", err)
	}
}
