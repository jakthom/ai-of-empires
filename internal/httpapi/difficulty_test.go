package httpapi

import (
	"testing"

	"crowns/internal/game"
	"crowns/internal/matches"
)

func TestCreateAcceptsCatalogDifficultiesAndRejectsUnknown(t *testing.T) {
	for _, difficulty := range game.GetCatalog().Difficulties {
		t.Run(difficulty.ID, func(t *testing.T) {
			s := testServer()
			config := game.Config{Civilization: "britons", Difficulty: difficulty.ID, Seed: 4817, Mode: "skirmish"}
			session := decodeResponse[matches.Session](t, request(t, s, "POST", "/api/v1/matches", "", config), 201)
			view := decodeResponse[game.Snapshot](t, request(t, s, "GET", "/api/v1/matches/"+session.MatchID, session.Token, nil), 200)
			if view.Difficulty != difficulty {
				t.Fatal("created match lost its authoritative difficulty")
			}
			config.Difficulty = "impossible"
			err := decodeResponse[ErrorBody](t, request(t, s, "POST", "/api/v1/matches", "", config), 422)
			if err.Error.Code != "invalid_difficulty" {
				t.Fatalf("unexpected difficulty validation: %+v", err)
			}
		})
	}
}
