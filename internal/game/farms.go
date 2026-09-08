package game

import "fmt"

// Prefer a farmer already assigned here, then the nearest idle villager.
// Planning is read-only; both action availability and the command use it.
func (w *World) farmReseeder(farm *Entity) (*Entity, error) {
	if farm.Type != "farm" || farm.Owner == 0 {
		return nil, rule("invalid_farm", "Select one of your farms.")
	}
	if farm.life.State() != Exhausted {
		return nil, rule("farm_not_depleted", "Reseed when this farm is depleted. Unfinished farms need a villager to complete construction.")
	}
	if w.Players[farm.Owner].Resources.Wood < 60 {
		return nil, rule("insufficient_resources", "Reseeding a farm requires 60 wood.")
	}
	var best *Entity
	bestAssigned := false
	for _, worker := range w.entities(farm.Owner, "villager") {
		assigned := worker.Order.Kind == "gather" && worker.Order.Target == farm.ID
		if worker.Container != 0 || worker.life.State() != Active || (!assigned && (worker.behavior.State() != Idle || len(worker.Orders) > 0)) || !w.reachableFootprint(worker, farm.Position, definitions[farm.Type].Radius+.7) {
			continue
		}
		if best == nil || assigned && !bestAssigned || assigned == bestAssigned && worker.Position.Distance(farm.Position) < best.Position.Distance(farm.Position) {
			best, bestAssigned = worker, assigned
		}
	}
	if best == nil {
		return nil, rule("no_available_villager", "Free an idle villager on this landmass, or order a villager to work this farm.")
	}
	return best, nil
}

func (w *World) reseedFarm(farm *Entity) error {
	worker, err := w.farmReseeder(farm)
	if err != nil {
		return err
	}
	if err := fire(farm.life, ReseedFarm, &entityContext{World: w, Actor: farm}); err != nil {
		return err
	}
	w.setOrder(worker, Order{Kind: "build", Target: farm.ID}, false)
	w.record(Event{Kind: "order", Message: fmt.Sprintf("Assigned to reseed Farm #%d", farm.ID), TargetID: farm.ID}, worker, 0)
	return nil
}
