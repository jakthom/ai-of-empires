package game

// Natural deposits are gathered. Neutral economic units and structures can be
// deliberately attacked, just like a peaceful kingdom's economic assets.
func attackableEntity(e *Entity) bool {
	if e == nil {
		return false
	}
	kind := definitions[e.Type].Kind
	return kind == "unit" || kind == "building"
}

func (w *World) declareAttack(attacker int, target *Entity) {
	if attacker == 0 || target == nil || !attackableEntity(target) || w.treatyInForce() {
		return
	}
	affected := w.attackParticipants(target)
	// Stable event order keeps checkpoint/replay deterministic. This only
	// declares diplomacy: defensive units still need a witnessed attack.
	for id := 1; id <= w.Config.Settlements; id++ {
		if id == attacker || !affected[id] || w.Players[id].lifecycle.State() == PlayerDefeated {
			continue
		}
		if r := w.Relations[relationKey(attacker, id)]; r != nil {
			mustFire(r.lifecycle, aggressionObserved, &relationContext{World: w, Relation: r})
		}
	}
}

func (w *World) attackParticipants(target *Entity) map[int]bool {
	if target == nil {
		return nil
	}
	affected := map[int]bool{}
	if target.Owner > 0 {
		affected[target.Owner] = true
	} else {
		market := target.ID
		for _, r := range w.Marketplace.Regions {
			if r.Cart == target.ID {
				market = r.ID
				if r.ID < 0 {
					affected[-r.ID] = true
				}
			}
		}
		for _, s := range w.Marketplace.Shipments {
			if !s.terminal() && (s.Market == market || s.Cart == target.ID) {
				affected[s.Buyer], affected[s.Seller] = true, true
			}
		}
		for _, e := range w.Entities {
			if e.Order.Kind == "guard" && e.Order.Target == target.ID {
				affected[e.Owner] = true
			}
		}
	}
	return affected
}
