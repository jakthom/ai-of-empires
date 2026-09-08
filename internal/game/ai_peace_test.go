package game

import "testing"

func TestSeededKingdomsHaveDifferentStablePreferences(t *testing.T) {
	for _, difficulty := range []string{"easy", "normal", "hard", "extra_hard", "expert"} {
		w := New(Config{Difficulty: difficulty, Settlements: 6, Seed: 4817})
		seen := map[aiTemperament]bool{}
		for id := 2; id <= 6; id++ {
			p := w.Players[id]
			seen[p.Temperament] = true
			if p.Temperament != initialTemperament(w.Config, id) {
				t.Fatal("preference is not stable for a seed")
			}
		}
		if len(seen) != 3 {
			t.Fatalf("%s did not offer a mixture of civilian and military preferences", difficulty)
		}
	}
	for id := 2; id <= 6; id++ {
		if initialTemperament(Config{Difficulty: "aggressive"}, id) != aiExpansionist {
			t.Fatal("aggressive mode must retain its explicit military preference")
		}
	}
}

func TestBuildersAndDefensiveKingdomsDoNotRaidUnprovoked(t *testing.T) {
	for _, preference := range []aiTemperament{aiBuilder, aiGuarded} {
		t.Run(string(preference), func(t *testing.T) {
			w := strategyWorld()
			p := w.Players[2]
			p.Temperament = preference
			revealStrategyWorld(w, 2)
			p.Resources = Resources{Food: 1000, Wood: 1000, Gold: 1000, Stone: 1000}
			for _, time := range []float64{600, 1200, 3600, 7200} {
				w.Time = time
				w.thinkPlayer(2)
				if p.strategy.State() != aiDeveloping || p.AIPlan.TargetOwner != 0 || w.relation(1, 2) != atPeace {
					t.Fatal("profitable proximity started an unprovoked raid")
				}
			}
			tc := w.entities(2, "town_center")[0]
			if len(tc.Tasks) != 1 || tc.Tasks[0].Product != "villager" || p.Resources.Food != 950 || !w.hasOrBuilding(2, "farm") {
				t.Fatal("peaceful kingdom did not invest in villagers and food production")
			}
			w.hit(tc, 1, 1)
			w.thinkPlayer(2)
			if preference == aiBuilder && p.strategy.State() != aiDeveloping {
				t.Fatal("builder started a counter-raid")
			}
			if preference == aiGuarded && (p.strategy.State() != aiRaiding || p.AIPlan.TargetOwner != 1) {
				t.Fatal("defensive kingdom did not counter-raid its aggressor")
			}
			w.Time += peaceAfter + 1
			w.pulseRelations()
			w.thinkPlayer(2)
			if p.strategy.State() == aiRaiding {
				t.Fatal("renewed peace did not end the counter-raid")
			}
		})
	}
}

func TestBuilderMilitaryProductionIsSmallExceptDuringAnAttack(t *testing.T) {
	w := strategyWorld()
	p := w.Players[2]
	p.Temperament = aiBuilder
	p.Resources = Resources{Food: 10000, Wood: 10000, Gold: 10000}
	for _, e := range w.entities(2, "militia") {
		w.remove(e.ID)
	}
	w.spawn("barracks", 2, Vec{48, 44})
	w.spawn("barracks", 2, Vec{48, 50})
	w.spawn("barracks", 2, Vec{54, 44})
	w.thinkPlayer(2)
	queued := 0
	for _, b := range w.entities(2, "barracks") {
		queued += len(b.Tasks)
	}
	if queued != 2 {
		t.Fatalf("builder funded %d soldiers instead of a small defense", queued)
	}
	attacker := w.spawn("knight", 3, Vec{44, 44})
	worker := w.entities(2, "villager")[0]
	w.refreshVisibility()
	w.hitFrom(worker, 3, attacker.ID, 1)
	w.thinkPlayer(2)
	if p.strategy.State() != aiDefending {
		t.Fatal("builder ignored an actual attack on its economy")
	}
}

func TestSettlementsCanDevelopTogetherAtPeace(t *testing.T) {
	if testing.Short() {
		t.Skip("peaceful development soak")
	}
	w := New(Config{Difficulty: "normal", Settlements: 3, Seed: 4817})
	stepWorld(w, 12000)
	for id := 2; id <= 3; id++ {
		if len(w.entities(id, "villager")) < 8 {
			t.Fatal("peaceful kingdom did not develop")
		}
		if w.relation(1, id) != atPeace {
			t.Fatal("peace with a passive human was broken")
		}
	}
	for _, record := range w.JournalSince(0) {
		if record.Event.Kind == "attack" {
			t.Fatal("builders and defensive kingdoms fought without provocation")
		}
	}
}
