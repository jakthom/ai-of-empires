package httpapi

import (
	"testing"

	"crowns/internal/game"
	"crowns/internal/matches"
)

func TestCreateValidatesWorldCatalogAndProjectsOptions(t *testing.T) {
	s := testServer()
	cfg := game.Config{Civilization: "britons", Difficulty: "easy", Seed: 813, Mode: "skirmish", Settlements: 6, World: game.WorldOptions{Type: "islands", Biome: "desert", Size: "large", Resources: "scarce", Separation: "far", Reveal: "explored", TreatyMinutes: 20}}
	seat := decodeResponse[matches.Session](t, request(t, s, "POST", "/api/v1/matches", "", cfg), 201)
	view := decodeResponse[game.Snapshot](t, request(t, s, "GET", "/api/v1/matches/"+seat.MatchID, seat.Token, nil), 200)
	if view.World != cfg.World || view.Map.Width != 160 || view.Map.Biome != "desert" || view.TreatyRemaining != 1200 {
		t.Fatalf("world settings lost: %+v", view.World)
	}
	cfg.World.Type = "invalid"
	err := decodeResponse[ErrorBody](t, request(t, s, "POST", "/api/v1/matches", "", cfg), 422)
	if err.Error.Code != "invalid_world_type" {
		t.Fatalf("unstructured validation: %+v", err)
	}
}
