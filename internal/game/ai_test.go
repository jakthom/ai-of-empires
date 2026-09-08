package game

import "testing"

func TestStandardExpansionistAttacksWithOpeningEconomy(t *testing.T) {
	w := New(Config{Difficulty: "normal", Seed: 4817})
	w.Players[2].Temperament = aiExpansionist
	defenders := w.entities(1, "")
	for range 18000 {
		w.Update()
		for _, defender := range defenders {
			if defender.HP < w.stats(defender).HP {
				t.Logf("First damage to the player's %s after %.1f game seconds", defender.Type, w.Time)
				return
			}
		}
	}
	for _, e := range w.entities(2, "") {
		t.Logf("%s #%d at %+v: %s progress %.2f, queue %d target %d cargo %.1f", e.Type, e.ID, e.Position, w.activity(e), e.Progress, len(e.Tasks), e.Order.Target, e.Cargo)
	}
	t.Fatalf("AI never attacked by 15 game minutes: resources %+v", w.Players[2].Resources)
}

func TestSolidFoundationCannotTrapAUnit(t *testing.T) {
	w := New(Config{Difficulty: "peaceful"})
	worker := w.entities(1, "villager")[0]
	site := snap(worker.Position)
	before := w.Players[1].Resources
	err := w.Apply(1, Command{Kind: "build", EntityIDs: []int{worker.ID}, Product: "house", Position: &site})
	if err == nil || w.Players[1].Resources != before || len(w.entities(1, "house")) != 0 || worker.behavior.State() != Idle {
		t.Fatal("a solid building must not enclose a unit or charge for the rejected site")
	}
	if err := w.Placement(1, "farm", site); err != nil {
		t.Fatalf("walkable farms can be placed under workers: %v", err)
	}
}
