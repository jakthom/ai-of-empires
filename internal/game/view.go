package game

import (
	"context"
	"fmt"
	"sort"
)

// View is the only read model exposed to a player. No handler can serialize the
// World, inspect the opposing economy, or bypass fog filtering.
func (w *World) View(player int) Snapshot {
	p := w.Players[player]
	defs, _ := orderedCatalog()
	n, capacity := w.population(player)
	v := Snapshot{
		Version: RulesVersion, Tick: w.Tick, Time: w.Time, Speed: w.Speed, Paused: w.match.State() == MatchPaused,
		Difficulty:  difficultyPolicy(w.Config.Difficulty).Difficulty,
		Settlements: w.Config.Settlements,
		World:       w.WorldOptions(), TreatyRemaining: w.TreatyRemaining(),
		Status: string(w.match.State()), Winner: w.Winner,
		Player:    PlayerView{ID: p.ID, Name: p.Name, Civilization: p.Civilization, Resources: p.Resources, Age: p.Age, AgeName: Ages[p.Age], Population: n, Capacity: capacity, Limit: 200, Technologies: sortedKeys(p.Technologies), Defeated: (p.lifecycle.State() == PlayerDefeated), Kills: p.Kills},
		Opponents: []OpponentView{}, Entities: []EntityView{}, Projectiles: []ProjectileView{}, Events: []Event{}, BuildOptions: []Action{},
		Map: w.mapView(p),
	}
	v.Effects = []BattlefieldEffectView{}
	for _, effect := range w.Aftermath {
		if w.visible(player, effect.View.Position) {
			v.Effects = append(v.Effects, effect.View)
		}
	}
	v.Player.Production = p.Production.view()
	v.Marketplace = w.marketplaceView(player)
	for id := 1; id <= w.Config.Settlements; id++ {
		if id != player {
			other := w.Players[id]
			preference := temperamentName(other.Temperament)
			if !other.AI {
				preference = "Player controlled"
			}
			v.Opponents = append(v.Opponents, OpponentView{ID: other.ID, Name: other.Name, Civilization: other.Civilization, Defeated: other.lifecycle.State() == PlayerDefeated, Relation: string(w.relation(player, id)), Temperament: preference})
		}
	}
	seen := map[int]bool{}
	for _, id := range w.IDs {
		e := w.Entities[id]
		if e == nil {
			continue
		}
		if e.Owner == player {
			d := definitions[e.Type]
			if d.Kind == "unit" {
				if d.Class == "worker" {
					v.Player.Workers++
					if e.behavior.State() == Idle {
						v.Player.Idle++
					}
				} else {
					v.Player.Military++
				}
			}
		}
		if e.Owner == player || w.visibleEntity(player, e) {
			v.Entities = append(v.Entities, w.entityView(e, player))
			seen[id] = true
		}
	}
	ids := []int{}
	for id := range p.Memory {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		if !seen[id] {
			v.Entities = append(v.Entities, observedEntity(p.Memory[id]))
		}
	}
	for _, projectile := range w.Projectiles {
		if w.visible(player, projectile.Position) {
			v.Projectiles = append(v.Projectiles, ProjectileView{ID: projectile.ID, Owner: projectile.Owner, Position: projectile.Position, Kind: projectile.Kind})
		}
	}
	page, _ := w.Log(player, LogQuery{Limit: 40})
	v.Events, v.EventCursor = page.Events, page.LatestCursor
	for _, d := range defs {
		if d.Kind == "building" {
			a := action("build", d.ID, d.Name, d.Description, d.Cost, d.Time, w.canBuild(p, d))
			v.BuildOptions = append(v.BuildOptions, a)
		}
	}
	return v
}
func (w *World) entityView(e *Entity, player int) EntityView {
	d := w.stats(e)
	v := EntityView{ID: e.ID, Type: e.Type, Kind: d.Kind, Name: d.Name, Owner: e.Owner, Position: e.Position, HP: e.HP, MaxHP: d.HP, Radius: d.Radius, Progress: e.Progress, Amount: e.Amount, Resource: e.Resource, State: string(e.behavior.State()), Visible: true, Actions: []Action{}, Tasks: []Task{}, Passengers: []int{}, Deployed: e.siege.State() == SiegeDeployed, Container: e.Container, Relic: e.Relic}
	v.Activity = w.activity(e)
	v.DamageStage = w.damageStage(e)
	v.Connections = w.barrierLinks(e, player)
	if d.Kind == "building" {
		if owner := w.Players[e.Owner]; owner != nil {
			v.AppearanceAge = owner.Age
		}
	}
	if e.life.State() == Foundation {
		v.State = string(Foundation)
	}
	if e.Owner != player || player == 0 {
		return observedEntity(v)
	}
	if e.Order.Kind == "guard" {
		v.GuardTarget = e.Order.Target
	}
	v.Cargo = e.Cargo
	v.Stance = e.Stance
	v.CargoType = e.CargoType
	v.Faith = e.Faith
	v.Tasks = append(v.Tasks, e.Tasks...)
	v.Passengers = append(v.Passengers, e.Passengers...)
	if e.Rally != nil {
		r := *e.Rally
		v.Rally = &r
	}
	if e.production.State() == ProductionBlocked {
		v.State = "production_blocked"
	}
	if e.Type == "trebuchet" && e.siege.State() != SiegePacked {
		v.State = string(e.siege.State())
	}
	p := w.Players[player]
	defs, techs := orderedCatalog()
	if e.Container == 0 && e.life.State() == Active && (d.Kind == "unit" || d.Kind == "building") {
		v.Actions = append(v.Actions, action("interact", "", "Give order", "Choose a resource, target, or destination. Shortcut: Q.", Resources{}, 0, nil))
	}
	if d.Kind == "building" && e.life.State() == Active {
		for _, u := range defs {
			if u.Kind == "unit" && u.Producer == e.Type {
				if u.Producer == "castle" && u.Class != "siege" && u.ID != uniqueFor[p.Civilization] {
					continue
				}
				v.Actions = append(v.Actions, action("train", u.ID, u.Name, u.Description, w.cost(p, u), u.Time, w.canTrain(p, e, u)))
			}
		}
		for _, t := range techs {
			if t.Producer == e.Type && !p.Technologies[t.ID] {
				v.Actions = append(v.Actions, action("research", t.ID, t.Name, t.Description, techCost(p, t), t.Time, w.canResearch(p, e, t)))
			}
		}
		if e.Type == "town_center" && p.Age < 3 {
			cost, time := agePrice(p.Age)
			v.Actions = append(v.Actions, action("age", "", Ages[p.Age+1], "Advance your entire kingdom to the next age.", cost, time, w.canAge(p)))
		}
		if tradingPost(e) {
			for _, resource := range []string{"food", "wood", "stone"} {
				for _, offer := range []struct{ kind, label string }{{"market_sell", "Sell "}, {"market_buy", "Buy "}} {
					q, err := w.quoteExchange(p, e, offer.kind, resource)
					a := action(offer.kind, resource, offer.label+resource, q.Description, q.Cost, 0, err)
					a.Gain = &q.Gain
					v.Actions = append(v.Actions, a)
				}
			}
		}
	}
	if e.Type == "farm" {
		worker, err := w.farmReseeder(e)
		description := "Pay 60 wood to reseed this depleted farm. An assigned farmer or the nearest idle villager rebuilds it, then resumes farming."
		if worker != nil {
			description = fmt.Sprintf("Pay 60 wood. Villager #%d will reseed this farm and resume farming.", worker.ID)
		}
		v.Actions = append(v.Actions, action("reseed_farm", "", "Reseed farm", description, Resources{Wood: 60}, definitions["farm"].Time, err))
	}
	if d.Kind == "unit" && e.Container == 0 {
		if e.Type == "fishing_ship" {
			v.Actions = append(v.Actions, action("gather", "", "Fish", "Choose a fish shoal. This ship brings its catch to your nearest reachable Dock as food.", Resources{}, 0, nil))
		}
		if tradeCarrier(e) {
			var err error
			if w.tradeHome(e) == nil {
				err = rule("market_required", "Build a reachable home Market or Dock to receive trade.")
			}
			v.Actions = append(v.Actions, action("trade", "", "Trade route", "Start beside your home Market or Dock. The Marketplace shows funded cart and ship routes, local prices, purchases, and repeat controls.", Resources{}, 0, err))
		}
		if e.Type == "villager" {
			for _, building := range defs {
				if building.Kind == "building" {
					v.Actions = append(v.Actions, action("build", building.ID, building.Name, building.Description, building.Cost, building.Time, w.canBuild(p, building)))
				}
			}
		}
		for _, cmd := range []struct {
			event                    UnitEvent
			kind, label, description string
		}{{OrderGuard, "guard", "Guard", "Follow a friendly unit or trading post, protect it from nearby attackers, then return to formation."}, {OrderMove, "move", "Move", "Choose a destination."}, {OrderAttack, "attack_move", "Attack move", "Move and attack other kingdoms along the way. This can start a conflict."}, {StopOrder, "stop", "Stop", "Cancel the current order."}} {
			// Query only player commands. Permitted also evaluates pulse guards,
			// which require the full simulation context (targets, cargo, RNG roll).
			// Next checks the command guard without firing any transition effects.
			if _, err := unitMachine.Next(context.Background(), e.behavior.State(), cmd.event, &unitContext{World: w, Actor: e}); err == nil {
				v.Actions = append(v.Actions, action(cmd.kind, "", cmd.label, cmd.description, Resources{}, 0, nil))
			}
		}
		if e.Type == "monk" {
			var conversionError error
			if w.treatyInForce() {
				conversionError = rule("peace_period", "Conversions are disabled until the initial peace period ends.")
			}
			v.Actions = append(v.Actions, action("convert", "", "Convert", "Choose an enemy unit.", Resources{}, 0, conversionError), action("heal", "", "Heal", "Choose a friendly unit.", Resources{}, 0, nil))
		}
		if e.Type == "trebuchet" {
			label := "Deploy"
			event := Deploy
			if e.siege.State() == SiegeDeployed {
				label = "Pack"
				event = Pack
			}
			_, err := siegeMachine.Next(context.Background(), e.siege.State(), event, &entityContext{Actor: e})
			if err != nil {
				err = rule("deploying", "Wait for packing or deployment to finish.")
			}
			v.Actions = append(v.Actions, action("deploy", "", label, "Trebuchets must deploy before firing.", Resources{}, 3, err))
		}
	}
	if d.Attack > 0 && e.Container == 0 && e.life.State() == Active {
		v.Actions = append(v.Actions,
			action("stance", "defensive", "Return fire", "Respond to attacks on this unit or nearby friends, with limited pursuit. Passing kingdoms are left in peace. Villagers defend only themselves.", Resources{}, 0, nil),
			action("stance", "stand_ground", "Hold position", "Return fire against attackers in range. Do not pursue them.", Resources{}, 0, nil),
			action("stance", "aggressive", "Aggressive", "Automatically attack nearby kingdoms. This can start a conflict. Villagers still defend only themselves.", Resources{}, 0, nil),
			action("stance", "passive", "Hold fire", "Do not respond automatically, even when attacked. Explicit attack orders still apply.", Resources{}, 0, nil))
	}
	if len(e.Passengers) > 0 {
		v.Actions = append(v.Actions, action("unload", "", "Unload", "Release units at a valid nearby exit.", Resources{}, 0, nil))
	}
	if d.Kind != "resource" {
		v.Actions = append(v.Actions, action("delete", "", "Delete", "Permanently remove this entity.", Resources{}, 0, nil))
	}
	return v
}

// observedEntity also sanitizes observations restored from older checkpoints.
// A visible building does not reveal its production, and a moving unit does
// not reveal its destination, queued intent, cargo or recovery timers.
func observedEntity(v EntityView) EntityView {
	v.GuardTarget = 0
	v.State, v.Activity = "observed", "Observed"
	if v.Kind == "resource" {
		v.Activity = "Available"
	} else if v.Progress < 1 {
		v.Activity = "Under construction"
	} else if v.Type == "farm" && v.Amount <= 0 {
		v.Activity = "Depleted"
	}
	v.Cargo, v.Faith, v.Container = 0, 0, 0
	v.CargoType, v.Stance, v.Rally = "", "", nil
	v.Actions, v.Tasks, v.Passengers = []Action{}, []Task{}, []int{}
	return v
}
func action(kind, product, label, description string, cost Resources, duration float64, err error) Action {
	a := Action{Kind: kind, Product: product, Label: label, Description: description, Cost: cost, Duration: duration, Enabled: err == nil}
	if err != nil {
		a.Reason = err.Error()
	}
	return a
}
