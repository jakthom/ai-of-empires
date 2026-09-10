package game

// The building action selects an idle worker. Explicit villager orders can
// reassign a busy worker or add more builders to a repair.
func (w *World) maintenanceWorker(target *Entity, kind string) (*Entity, error) {
	if target.Owner == 0 || definitions[target.Type].Kind != "building" {
		return nil, rule("invalid_target", "Select your own building.")
	}
	if kind == "repair" && (target.life.State() != Active || target.HP >= w.stats(target).HP) {
		return nil, rule("repair_unneeded", "This building must be completed and damaged to repair it.")
	}
	if kind == "gather" && (target.Type != "farm" || target.life.State() != Active || target.Amount <= 0) {
		return nil, rule("invalid_farm", "Plant or reseed this farm before assigning a farmer.")
	}
	var best *Entity
	for _, e := range w.entities(target.Owner, "villager") {
		if e.Container != 0 || e.life.State() != Active {
			continue
		}
		if e.Order.Kind == kind && e.Order.Target == target.ID {
			return nil, rule("work_assigned", "A villager is already assigned. Select additional villagers to help.")
		}
		if e.behavior.State() != Idle || len(e.Orders) > 0 || !w.reachableFootprint(e, target.Position, definitions[target.Type].Radius+.7) {
			continue
		}
		if best == nil || e.Position.Distance(target.Position) < best.Position.Distance(target.Position) {
			best = e
		}
	}
	if best == nil {
		return nil, rule("no_available_villager", "Free an idle villager nearby, or select a villager and give an order directly.")
	}
	return best, nil
}
func (w *World) assignMaintenance(target *Entity, kind string) error {
	worker, err := w.maintenanceWorker(target, kind)
	if err != nil {
		return err
	}
	w.setOrder(worker, Order{Kind: kind, Target: target.ID}, false)
	return nil
}
