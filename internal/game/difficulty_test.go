package game

import "testing"

func TestDifficultiesKeepEqualOpeningAndPublishMetadata(t *testing.T) {
	for _, difficulty := range GetCatalog().Difficulties {
		w := New(Config{Difficulty: difficulty.ID, Seed: 4817})
		if w.View(1).Difficulty != difficulty || !ValidDifficulty(difficulty.ID) {
			t.Fatal("snapshot and catalog difficulty disagree")
		}
		for _, typ := range []string{"villager", "scout", "town_center"} {
			if len(w.entities(1, typ)) != len(w.entities(2, typ)) {
				t.Fatalf("%s gives extra starting %s", difficulty.ID, typ)
			}
		}
		if w.Players[1].Resources != w.Players[2].Resources {
			t.Fatalf("%s gives extra starting resources", difficulty.ID)
		}
	}
}

func TestConventionalDifficultyPressureRampsGradually(t *testing.T) {
	levels := []string{"easy", "normal", "hard", "extra_hard", "expert"}
	previous := difficultyPolicy(levels[0])
	for _, id := range levels[1:] {
		current := difficultyPolicy(id)
		if current.Workers <= previous.Workers {
			t.Fatalf("%s must build a larger economy than %s", id, previous.ID)
		}
		if current.RaidLimit <= previous.RaidLimit || current.ArmyLimit <= previous.ArmyLimit {
			t.Fatalf("%s must permit more military pressure than %s", id, previous.ID)
		}
		if current.RaidInterval >= previous.RaidInterval {
			t.Fatalf("%s must allow more frequent raids than %s", id, previous.ID)
		}
		if current.RaidAfter >= previous.RaidAfter {
			t.Fatalf("%s must begin raids earlier than %s", id, previous.ID)
		}
		previous = current
	}
}

func TestAdvancedExpansionistsReachPlayerWithOpeningEconomy(t *testing.T) {
	for _, test := range []struct {
		id      string
		seconds int
	}{{"extra_hard", 600}, {"expert", 600}, {"aggressive", 180}} {
		t.Run(test.id, func(t *testing.T) {
			w := New(Config{Difficulty: test.id, Seed: 4817})
			w.Players[2].Temperament = aiExpansionist
			defenders := w.entities(1, "")
			for range int(float64(test.seconds) / Step) {
				w.Update()
				for _, defender := range defenders {
					if defender.HP < w.stats(defender).HP {
						t.Logf("%s inflicted first damage after %.1f game seconds", test.id, w.Time)
						return
					}
				}
			}
			t.Fatalf("no attack by %d seconds: workers %d, resources %+v", test.seconds, len(w.entities(2, "villager")), w.Players[2].Resources)
		})
	}
}

func TestExpertResearchPaysResourcesAndRunsProductionLifecycle(t *testing.T) {
	w := New(Config{Difficulty: "expert"})
	before := w.Players[2].Resources
	w.aiResearch(2)
	w.Update()
	tc := w.entities(2, "town_center")[0]
	if len(tc.Tasks) != 1 || tc.Tasks[0].Product != "loom" || tc.production.State() != ProductionWorking || w.Players[2].Resources.Gold != before.Gold-50 {
		t.Fatal("expert research must use paid, queued production")
	}
	w.aiResearch(2)
	if len(tc.Tasks) != 1 || w.Players[2].Resources.Gold != before.Gold-50 {
		t.Fatal("expert queued or charged for research twice")
	}
}
