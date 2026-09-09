package game

import (
	"context"
	"fmt"
	"math"
	"slices"
)

var gameSpeeds = []float64{1, 1.7, 3.4, 8, 16, 32}

func (w *World) Apply(player int, c Command) error {
	// Capture selected actors before an accepted deletion removes them. A
	// replay is handled by matches before this boundary, so it logs only once.
	actors := []*Entity{}
	seen := map[int]bool{}
	for _, id := range c.EntityIDs {
		if e := w.Entities[id]; e != nil && e.Owner == player && !seen[id] {
			actors = append(actors, e)
			seen[id] = true
		}
	}
	err := w.apply(player, c)
	if w.Players[player] != nil {
		w.recordCommand(player, c, actors, err)
	}
	return err
}

func (w *World) apply(player int, c Command) error {
	p := w.Players[player]
	if p == nil {
		return rule("player_not_found", "Player does not exist.")
	}
	if w.match.State() == MatchFinished || (p.lifecycle.State() == PlayerDefeated) {
		return rule("match_ended", "This match has ended.")
	}
	if c.Position != nil && !w.inside(*c.Position) {
		return rule("invalid_position", "Choose a position inside the map.")
	}
	switch c.Kind {
	case "pause":
		event := PauseMatch
		if w.match.State() == MatchPaused {
			event = ResumeMatch
		}
		return fire(w.match, event, &matchContext{World: w})
	case "speed":
		return w.SetSpeed(c.Value)
	case "resign":
		if err := fire(p.lifecycle, ResignPlayer, &playerContext{World: w, Player: p}); err != nil {
			return err
		}
		w.checkVictory()
		return nil
	}
	if w.match.State() == MatchPaused {
		return rule("paused", "Resume the match before issuing an order.")
	}
	if len(c.EntityIDs) == 0 || len(c.EntityIDs) > 200 {
		return rule("invalid_selection", "Select between 1 and 200 units or buildings.")
	}
	es := []*Entity{}
	seen := map[int]bool{}
	for _, id := range c.EntityIDs {
		e := w.Entities[id]
		if e == nil || e.Owner != player {
			return rule("invalid_selection", "Selection contains an unavailable entity.")
		}
		if !seen[id] {
			es = append(es, e)
			seen[id] = true
		}
	}
	first := es[0]
	if slices.Contains([]string{"train", "research", "age", "cancel", "rally", "market_sell", "market_buy", "reseed_farm", "unload", "deploy"}, c.Kind) && len(es) != 1 {
		return rule("invalid_selection", "Select one unit or building for this action.")
	}
	switch c.Kind {
	case "train":
		if len(es) != 1 {
			return rule("invalid_selection", "Select one producer.")
		}
		d, ok := definitions[c.Product]
		if !ok || d.Kind != "unit" {
			return rule("unknown_product", "Unknown unit.")
		}
		if err := w.canTrain(p, first, d); err != nil {
			return err
		}
		paid := w.cost(p, d)
		duration := d.Time
		if p.Civilization == "persians" && d.ID == "villager" {
			duration /= 1.15
		}
		task := Task{"train", d.ID, duration, duration, paid}
		return fire(first.production, QueueProduction, &entityContext{World: w, Actor: first, Task: &task})
	case "research":
		t, ok := technologies[c.Product]
		if !ok {
			return rule("unknown_product", "Unknown technology.")
		}
		if err := w.canResearch(p, first, t); err != nil {
			return err
		}
		task := Task{"research", t.ID, t.Time, t.Time, techCost(p, t)}
		return fire(first.production, QueueProduction, &entityContext{World: w, Actor: first, Task: &task})
	case "age":
		if first.Type != "town_center" || first.Progress < 1 {
			return rule("invalid_producer", "Advance at a completed Town Center.")
		}
		if err := w.canAge(p); err != nil {
			return err
		}
		cost, duration := agePrice(p.Age)
		task := Task{"age", fmt.Sprint(p.Age + 1), duration, duration, cost}
		return fire(first.production, QueueProduction, &entityContext{World: w, Actor: first, Task: &task})
	case "cancel":
		if len(first.Tasks) == 0 {
			return rule("empty_queue", "There is nothing to cancel.")
		}
		i := int(c.Value)
		if c.Value != float64(i) || i < 0 || i >= len(first.Tasks) {
			return rule("invalid_queue_index", "Choose a valid queue item.")
		}
		return fire(first.production, CancelProduction, &entityContext{World: w, Actor: first, Index: i})
	case "build":
		for _, e := range es {
			if e.Type != "villager" || e.Container != 0 {
				return rule("invalid_builder", "Select villagers to construct a building.")
			}
			if c.Queue && len(e.Orders) >= 64 {
				return rule("queue_full", "The order queue is full.")
			}
		}
		if c.Position == nil {
			return rule("position_required", "Choose where to build.")
		}
		d, ok := definitions[c.Product]
		if !ok || d.Kind != "building" {
			return rule("unknown_product", "Unknown building.")
		}
		if err := w.canBuild(p, d); err != nil {
			return err
		}
		pos := snap(*c.Position)
		sites, price, err := w.PlanBuilding(player, d.ID, pos, c.EndPosition)
		if err != nil {
			return err
		}
		for _, worker := range es {
			if c.Queue && len(worker.Orders)+len(sites) > 64 {
				return rule("queue_full", "The construction order queue is full.")
			}
		}
		p.Resources.Add(price.Scale(-1))
		for i, site := range sites {
			if old := w.barrierAt(site); d.ID == "gate" && old != nil {
				w.entityEvent(old, "replaced", "Replaced by a gate", 0)
				w.remove(old.ID)
			}
			e := w.spawnWithLife(d.ID, player, site, Foundation)
			for _, worker := range es {
				w.setOrder(worker, Order{Kind: "build", Target: e.ID}, c.Queue || i > 0)
			}
		}
		return nil
	case "rally":
		if definitions[first.Type].Kind != "building" || c.Position == nil {
			return rule("invalid_rally", "Select a building and a rally location.")
		}
		pos := *c.Position
		first.Rally = &pos
		return nil
	case "market_sell", "market_buy":
		return w.exchange(p, first, c.Kind, c.Product)
	case "reseed_farm":
		return w.reseedFarm(first)
	case "unload":
		if len(first.Passengers) == 0 {
			return rule("empty_garrison", "There are no units inside.")
		}
		unloaded := 0
		for _, id := range append([]int{}, first.Passengers...) {
			if e := w.Entities[id]; e != nil {
				if err := fire(e.behavior, Disembark, &unitContext{World: w, Actor: e}); err == nil {
					unloaded++
				}
			}
		}
		if unloaded == 0 {
			return rule("blocked_exit", "There is no clear exit. Move the transport closer to land.")
		}
		return nil
	case "deploy":
		if first.Type != "trebuchet" {
			return rule("invalid_unit", "Only trebuchets deploy.")
		}
		event := Deploy
		if first.siege.State() == SiegeDeployed {
			event = Pack
		}
		if err := fire(first.siege, event, &entityContext{World: w, Actor: first}); err != nil {
			return err
		}
		return nil
	case "delete":
		for _, e := range es {
			if w.Entities[e.ID] != nil {
				if err := fire(e.life, DeleteEntity, &entityContext{World: w, Actor: e}); err != nil {
					return err
				}
			}
		}
		w.refreshVisibility()
		w.checkVictory()
		return nil
	}
	// Validate the whole selection before applying any order.
	if c.TargetPlayer != 0 && (c.Kind != "attack_move" || c.TargetPlayer == player || w.Players[c.TargetPlayer] == nil) {
		return rule("invalid_target_player", "Choose another kingdom for an attack-move order.")
	}
	target := w.Entities[c.TargetID]
	if c.TargetID != 0 && !w.visibleEntity(player, target) {
		return rule("invalid_target", "Target is unavailable.")
	}
	if c.Queue {
		for _, e := range es {
			if len(e.Orders) >= 64 {
				return rule("queue_full", "The order queue is full.")
			}
		}
	}
	orders := make([]Order, 0, len(es))
	for _, e := range es {
		d := definitions[e.Type]
		if e.Container != 0 {
			return rule("garrisoned", "Unload the selected units first.")
		}
		o := Order{Kind: c.Kind, Target: c.TargetID, Position: c.Position, TargetPlayer: c.TargetPlayer}
		if c.Kind == "interact" {
			if target == nil {
				return rule("invalid_target", "Choose a target.")
			}
			td := definitions[target.Type]
			switch {
			case e.Type == "monk" && target.Type == "relic":
				o.Kind = "relic"
			case e.Type == "monk" && target.Type == "monastery" && target.Owner == player && e.Relic:
				o.Kind = "deposit_relic"
			case e.Type == "monk" && target.Owner > 0 && target.Owner != player:
				o.Kind = "convert"
			case e.Type == "monk" && target.Owner == player && td.Kind == "unit":
				o.Kind = "heal"
			case e.Type == "trade_cart" && target.Type == "market" && target.Owner == 0:
				o.Kind = "trade"
			case e.Type == "villager" && target.Owner == player && target.Progress < 1:
				o.Kind = "build"
			case d.Class == "worker" && target.Resource != "" && (td.Kind == "resource" || target.Type == "farm") && (target.Owner == 0 || target.Owner == player):
				o.Kind = "gather"
			case e.Type == "villager" && target.Owner == player && td.Kind == "building" && target.HP < w.stats(target).HP:
				o.Kind = "repair"
			case target.Owner == player && w.garrisonCapacity(target) > 0 && d.Kind == "unit":
				o.Kind = "garrison"
			case target.Owner > 0 && target.Owner != player:
				o.Kind = "attack"
			default:
				return rule("invalid_target", "That unit cannot interact with this target.")
			}
		}
		switch o.Kind {
		case "stop":
			o.Kind = "idle"
		case "stance":
			if !slices.Contains([]string{"defensive", "aggressive", "stand_ground", "passive"}, c.Product) {
				return rule("invalid_stance", "Unknown stance.")
			}
		case "move", "attack_move":
			if d.Kind != "unit" || d.Speed == 0 || c.Position == nil {
				return rule("invalid_order", "Select mobile units and a destination.")
			}
			if e.Type == "trebuchet" && e.siege.State() != SiegePacked {
				return rule("deployed", "Pack the trebuchet before moving.")
			}
		case "attack":
			if w.treatyInForce() {
				return rule("peace_period", "Attacks are disabled until the initial peace period ends.")
			}
			if d.Attack <= 0 || target == nil || target.Owner == player || target.Owner == 0 {
				return rule("invalid_target", "Choose an enemy for a combat unit.")
			}
		case "gather":
			// An exhausted owned farm is still a valid work destination. The
			// unit and farm lifecycle effects perform reseeding and payment.
			reseed := target != nil && target.Type == "farm" && target.Owner == player && target.life.State() == Exhausted
			if d.Class != "worker" || target == nil || target.Resource == "" || (target.Amount <= 0 && !reseed) || (target.Owner != 0 && target.Owner != player) || target.Progress < 1 {
				return rule("invalid_target", "Choose an available resource for a worker.")
			}
			if d.Naval != (target.Type == "fish") {
				return rule("invalid_target", "Fishing ships gather fish; villagers gather land resources.")
			}
			if reseed && p.Resources.Wood < 60 {
				return rule("insufficient_resources", "Reseeding a farm requires 60 wood.")
			}
		case "build", "repair":
			if e.Type != "villager" || target == nil || target.Owner != player || definitions[target.Type].Kind != "building" {
				return rule("invalid_target", "Choose your own building for a villager.")
			}
		case "heal":
			if e.Type != "monk" || target == nil || target.Owner != player || definitions[target.Type].Kind != "unit" || target.Type == "monk" {
				return rule("invalid_target", "Monks heal other friendly units.")
			}
		case "convert":
			if w.treatyInForce() {
				return rule("peace_period", "Conversions are disabled until the initial peace period ends.")
			}
			if e.Type != "monk" || target == nil || target.Owner == 0 || target.Owner == player || target.Type == "town_center" || target.Type == "castle" || target.Type == "wonder" {
				return rule("invalid_target", "That target cannot be converted.")
			}
			if (definitions[target.Type].Class == "siege" || definitions[target.Type].Kind == "building") && !p.Technologies["redemption"] {
				return rule("technology_required", "Research Redemption to convert buildings and siege.")
			}
		case "relic", "deposit_relic":
			if e.Type != "monk" || target == nil {
				return rule("invalid_target", "Choose a relic or monastery for a monk.")
			}
			if o.Kind == "relic" && (target.Type != "relic" || e.Relic) {
				return rule("invalid_target", "This monk cannot carry another relic.")
			}
			if o.Kind == "deposit_relic" && (!e.Relic || target.Type != "monastery" || target.Owner != player || target.Progress < 1) {
				return rule("invalid_target", "Choose a completed monastery for the relic.")
			}
		case "garrison":
			if target == nil || target.Owner != player || target.Progress < 1 || w.garrisonCapacity(target) == 0 || d.Kind != "unit" || d.Naval {
				return rule("invalid_target", "Choose an eligible friendly garrison.")
			}
		case "trade":
			if e.Type != "trade_cart" || target == nil || target.Type != "market" || target.Owner != 0 {
				return rule("invalid_target", "Choose an explored neutral Market for a Trade Cart.")
			}
			if !w.reachableFootprint(e, target.Position, definitions[target.Type].Radius+.7) {
				return rule("unreachable_market", "Choose a neutral Market reachable by land.")
			}
			if w.tradeHome(e) == nil {
				return rule("market_required", "Build a Market on this landmass to receive trade gold.")
			}
		default:
			return rule("unknown_command", "Unknown command.")
		}
		if c.Kind != "stance" {
			o.Initiated = o.Kind == "attack" || o.Kind == "attack_move"
			if _, err := unitMachine.Next(context.Background(), e.behavior.State(), orderEvent(o.Kind), &unitContext{Actor: e}); err != nil {
				return rule("invalid_state", err.Error())
			}
		}
		orders = append(orders, o)
	}
	for i, e := range es {
		if c.Kind == "stance" {
			e.Stance = c.Product
		} else {
			w.setOrder(e, orders[i], c.Queue)
		}
	}
	return nil
}

func snap(p Vec) Vec { return Vec{math.Floor(p.X) + .5, math.Floor(p.Y) + .5} }
func (w *World) canBuild(p *Player, d Definition) error {
	if p.Age < d.Age {
		return rule("age_required", Ages[d.Age]+" is required.")
	}
	if d.ID == "town_center" && p.Age < 2 && w.hasBuilding(p.ID, "town_center") {
		return rule("age_required", "Castle Age is required for additional Town Centers.")
	}
	if d.Prerequisite != "" && !w.hasBuilding(p.ID, d.Prerequisite) {
		return rule("building_required", "Build a "+definitions[d.Prerequisite].Name+" first.")
	}
	if !p.Resources.CanPay(d.Cost) {
		return rule("insufficient_resources", "Gather more resources to build "+d.Name+".")
	}
	return nil
}
func (w *World) canTrain(p *Player, e *Entity, d Definition) error {
	if e.Progress < 1 || d.Producer != e.Type {
		return rule("invalid_producer", "This building cannot train that unit.")
	}
	if p.Age < d.Age {
		return rule("age_required", Ages[d.Age]+" is required.")
	}
	if d.Producer == "castle" && d.Class != "siege" && d.ID != uniqueFor[p.Civilization] {
		return rule("civilization_required", "This unit belongs to another civilization.")
	}
	if (d.ID == "hand_cannoneer" || d.ID == "bombard_cannon") && !p.Technologies["chemistry"] {
		return rule("technology_required", "Research Chemistry first.")
	}
	if len(e.Tasks) >= 15 {
		return rule("queue_full", "The production queue is full.")
	}
	if !p.Resources.CanPay(w.cost(p, d)) {
		return rule("insufficient_resources", "Gather more resources to train "+d.Name+".")
	}
	return nil
}
func (w *World) pending(player int, kind, product string) bool {
	for _, e := range w.entities(player, "") {
		for _, t := range e.Tasks {
			if t.Type == kind && (product == "" || t.Product == product) {
				return true
			}
		}
	}
	return false
}
func (w *World) canResearch(p *Player, e *Entity, t Technology) error {
	if e.Progress < 1 || e.Type != t.Producer {
		return rule("invalid_producer", "Research this technology at the correct completed building.")
	}
	if p.Age < t.Age {
		return rule("age_required", Ages[t.Age]+" is required.")
	}
	if p.Technologies[t.ID] || w.pending(p.ID, "research", t.ID) {
		return rule("already_researched", "This technology is complete or already queued.")
	}
	if t.Prerequisite != "" && !p.Technologies[t.Prerequisite] {
		return rule("technology_required", "Research "+technologies[t.Prerequisite].Name+" first.")
	}
	if len(e.Tasks) >= 15 {
		return rule("queue_full", "The production queue is full.")
	}
	if !p.Resources.CanPay(techCost(p, t)) {
		return rule("insufficient_resources", "Gather more resources to research "+t.Name+".")
	}
	return nil
}
func agePrice(age int) (Resources, float64) {
	switch age {
	case 0:
		return Resources{Food: 500}, 130
	case 1:
		return Resources{Food: 800, Gold: 200}, 160
	default:
		return Resources{Food: 1000, Gold: 800}, 190
	}
}
func (w *World) canAge(p *Player) error {
	if p.Age >= 3 {
		return rule("maximum_age", "You have reached Imperial Age.")
	}
	if w.pending(p.ID, "age", "") {
		return rule("already_queued", "Age advancement is already queued.")
	}
	sets := [][]string{{"mill", "lumber_camp", "mining_camp", "dock", "barracks"}, {"archery_range", "stable", "blacksmith", "market"}, {"monastery", "university", "siege_workshop"}}
	n := 0
	for _, typ := range sets[p.Age] {
		if w.hasBuilding(p.ID, typ) {
			n++
		}
	}
	if p.Age == 2 && w.hasBuilding(p.ID, "castle") {
		n = 2
	}
	if n < 2 {
		return rule("building_required", "Build two different buildings from your current age first.")
	}
	cost, _ := agePrice(p.Age)
	if !p.Resources.CanPay(cost) {
		return rule("insufficient_resources", "Gather more resources to advance to "+Ages[p.Age+1]+".")
	}
	return nil
}

// Placement validates terrain and collision on the server. The error deliberately
// does not identify unseen obstructions. The client displays only a preview.
func (w *World) Placement(player int, typ string, pos Vec) error {
	d, ok := definitions[typ]
	if !ok || d.Kind != "building" {
		return rule("unknown_product", "Unknown building.")
	}
	pos = snap(pos)
	if typ == "gate" && !w.straightGate(player, pos, nil) {
		return rule("invalid_placement", "Gates need a straight wall section. Choose walls along one axis.")
	}
	for y := pos.Y - d.Radius; y <= pos.Y+d.Radius; y += .5 {
		for x := pos.X - d.Radius; x <= pos.X+d.Radius; x += .5 {
			q := Vec{x, y}
			if !w.inside(q) || !w.Players[player].Explored[int(y)*w.Width+int(x)] {
				return rule("invalid_placement", "Explore the entire building site first.")
			}
			if typ != "dock" && !w.land(q) {
				return rule("invalid_placement", "Choose clear land for this building.")
			}
		}
	}
	if typ == "dock" {
		land, water := false, false
		for _, v := range []Vec{{pos.X - 2, pos.Y}, {pos.X + 2, pos.Y}, {pos.X, pos.Y - 2}, {pos.X, pos.Y + 2}} {
			land = land || w.land(v)
			water = water || w.water(v)
		}
		if !land || !water {
			return rule("invalid_placement", "Docks need a shoreline between land and water.")
		}
	}
	for _, id := range w.IDs {
		e := w.Entities[id]
		if e == nil || e.Container != 0 {
			continue
		}
		ed := definitions[e.Type]
		if barrier(typ) && barrier(e.Type) && e.Owner == player {
			distance := e.Position.Distance(pos)
			if e.Type == "gate" && distance == 1 && !w.straightGate(player, e.Position, map[Vec]bool{pos: true}) {
				return rule("invalid_placement", "Connect walls along the gate's existing wall line.")
			}
			if distance == 1 || typ == "gate" && distance < .01 && e.Type != "gate" {
				continue
			}
		}
		if ed.Kind == "unit" {
			if typ != "farm" && e.Position.Distance(pos) < d.Radius+ed.Radius+.15 {
				return rule("invalid_placement", "Units occupy this site. Move them or choose another location.")
			}
			continue
		}
		if e.Position.Distance(pos) < d.Radius+ed.Radius+.15 {
			return rule("invalid_placement", "This site is obstructed. Choose another location.")
		}
	}
	return nil
}
func (w *World) garrisonCapacity(e *Entity) int {
	switch e.Type {
	case "town_center":
		return 15
	case "castle":
		return 20
	case "tower":
		return 5
	case "transport":
		return 10
	case "ram":
		return 4
	}
	return 0
}
