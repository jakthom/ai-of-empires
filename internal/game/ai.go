package game

import "math"

// AI submits the same validated commands as a human. It cannot mint resources,
// query the opponent's stockpile, or target an entity outside its own vision.
func (w *World) thinkAI() {
	for player := 1; player <= w.Config.Settlements; player++ {
		if w.Players[player].AI {
			w.thinkPlayer(player)
		}
	}
}

func (w *World) thinkPlayer(player int) {
	p := w.Players[player]
	if (p.lifecycle.State() == PlayerDefeated) || w.Config.Difficulty == "peaceful" {
		return
	}
	w.aiNavy(w.aiObserve(player))
	mustFire(p.strategy, aiAssess, w.aiObserve(player))
}

func (w *World) aiEconomy(c *aiContext, defending bool) {
	if !defending {
		w.aiCommerce(c)
	}
	p, workers, tcs := c.Player, c.Workers, c.Centers
	player, policy := p.ID, c.Policy
	if len(tcs) == 0 {
		w.aiBuild(player, "town_center", c.Home, workers)
		return
	}
	tc := tcs[0]
	for i, e := range workers {
		if e.behavior.State() != Idle || e.Container != 0 || p.voyaging(e.ID) || defending && c.Threat != nil && e.Position.Distance(c.Threat.Position) < 12 {
			continue
		}
		res := "wood"
		switch i % 9 {
		case 0, 1, 2, 3:
			res = "food"
		case 6, 7:
			res = "gold"
		case 8:
			res = "stone"
		}
		target := w.nearest(e.Position, func(t *Entity) bool {
			if !(t.Amount > 0 && t.Resource == res && w.visibleEntity(player, t) && (t.Owner == 0 || t.Owner == player) && (definitions[t.Type].Kind == "resource" || t.Type == "farm") && t.Type != "fish" && w.sameRegion(e.Position, t.Position, false)) {
				return false
			}
			if t.Type == "farm" {
				for _, other := range workers {
					if other.ID != e.ID && other.Order.Target == t.ID {
						return false
					}
				}
			}
			return true
		})
		if target != nil {
			_ = w.Apply(player, Command{Kind: "interact", EntityIDs: []int{e.ID}, TargetID: target.ID})
		} else if res == "food" {
			w.aiBuild(player, "farm", tc.Position, []*Entity{e})
		}
	}
	n, cap := w.population(player)
	military := len(c.Army)
	// A standing army is insurance, not a reason to spend every last unit of
	// food forever. Once funded, save for villagers, technology and expansion.
	armyGoal := max(policy.RaidSize*2, 8+p.Age*4)
	if p.Temperament == aiBuilder {
		armyGoal = 2 + p.Age
	}
	if p.Temperament == aiGuarded {
		armyGoal = 4 + p.Age*2
	}
	if defending {
		armyGoal = max(armyGoal, int(math.Ceil(c.ThreatPower/8*1.3)))
	}
	armyGoal = min(policy.ArmyLimit, armyGoal)
	// Once an opening economy is working, tougher opponents fund soldiers
	// before another villager. A fielded army leaves room to save for growth.
	reserveForArmy := false
	if (defending || p.Temperament == aiExpansionist && policy.MilitaryPriorityAt > 0 && len(workers) >= policy.MilitaryPriorityAt) && military < armyGoal && w.hasBuilding(player, "barracks") {
		w.aiTrainMilitary(player, armyGoal)
		for _, producer := range w.entities(player, "barracks") {
			reserveForArmy = reserveForArmy || producer.life.State() == Active && len(producer.Tasks) == 0
		}
	}
	workerGoal := policy.Workers + max(0, min(len(tcs), policy.TownCenters)-1)*8
	committedWorkers := len(workers)
	for _, center := range tcs {
		for _, task := range center.Tasks {
			if task.Type == "train" && task.Product == "villager" {
				committedWorkers++
			}
		}
	}
	if !reserveForArmy && committedWorkers < workerGoal && w.Time >= p.AIPlan.NextWorkerAt {
		for _, center := range tcs {
			if committedWorkers >= workerGoal || w.Time < p.AIPlan.NextWorkerAt {
				break
			}
			if len(center.Tasks) == 0 && w.canTrain(p, center, definitions["villager"]) == nil {
				if w.Apply(player, Command{Kind: "train", EntityIDs: []int{center.ID}, Product: "villager"}) == nil {
					committedWorkers++
					if policy.WorkerPause > 0 {
						p.AIPlan.NextWorkerAt = w.Time + center.Tasks[len(center.Tasks)-1].Duration + policy.WorkerPause
					}
				}
			}
		}
	}
	if n+3 >= cap {
		w.aiBuild(player, "house", tc.Position, workers)
	}
	openingBuildings := []string{"lumber_camp", "mill", "barracks"}
	if p.Temperament == aiExpansionist && policy.MilitaryPriorityAt > 0 {
		openingBuildings = []string{"barracks", "lumber_camp", "mill"}
	}
	if w.islandWorld() {
		openingBuildings = []string{"lumber_camp", "dock", "barracks"}
	}
	for _, typ := range openingBuildings {
		if !w.hasOrBuilding(player, typ) {
			if w.aiBuild(player, typ, tc.Position, workers) {
				break
			}
		}
	}
	if p.Age > 0 {
		buildings := []string{"blacksmith", "market"}
		if p.Temperament != aiBuilder {
			buildings = append(buildings, "archery_range", "stable")
		}
		if p.Temperament == aiExpansionist {
			buildings = []string{"archery_range", "stable", "blacksmith", "market"}
		}
		for _, typ := range buildings {
			if !w.hasOrBuilding(player, typ) {
				if w.aiBuild(player, typ, tc.Position, workers) {
					break
				}
			}
		}
	}
	if p.Age > 1 {
		buildings := []string{"monastery", "university"}
		if p.Temperament == aiExpansionist {
			buildings = []string{"siege_workshop", "monastery", "castle"}
		}
		for _, typ := range buildings {
			if !w.hasOrBuilding(player, typ) {
				if w.aiBuild(player, typ, tc.Position, workers) {
					break
				}
			}
		}
	}
	if !defending && w.canAge(p) == nil {
		_ = w.Apply(player, Command{Kind: "age", EntityIDs: []int{tc.ID}})
	}
	if policy.Upgrades {
		w.aiResearch(player)
	}
	w.aiTrainMilitary(player, armyGoal)
	if !defending {
		w.aiExpand(c)
	}
}

func (w *World) aiResearch(player int) {
	for _, id := range []string{"loom", "double_bit_axe", "horse_collar", "wheelbarrow", "forging", "scale_armor", "fletching", "bodkin"} {
		technology, ok := technologies[id]
		if !ok {
			continue
		}
		for _, producer := range w.entities(player, technology.Producer) {
			if len(producer.Tasks) == 0 && producer.life.State() == Active && w.canResearch(w.Players[player], producer, technology) == nil {
				_ = w.Apply(player, Command{Kind: "research", EntityIDs: []int{producer.ID}, Product: id})
			}
		}
	}
}

func (w *World) aiTrainMilitary(player, goal int) {
	p := w.Players[player]
	goal = min(goal, difficultyPolicy(w.Config.Difficulty).ArmyLimit)
	committed := 0
	for _, e := range w.entities(player, "") {
		d := definitions[e.Type]
		if d.Kind == "unit" && d.Class != "worker" && d.Class != "trader" && d.Attack > 0 {
			committed++
		}
	}
	// Count reservations across every producer. When restoring an older game,
	// refund queues beyond the budget through the ordinary production machine;
	// existing units retain their normal lives and are never deleted to fit it.
	for _, e := range w.entities(player, "") {
		for i := 0; i < len(e.Tasks); {
			task := e.Tasks[i]
			if task.Type != "train" || definitions[task.Product].Attack == 0 || definitions[task.Product].Class == "worker" || definitions[task.Product].Class == "trader" {
				i++
				continue
			}
			if committed >= goal {
				if w.Apply(player, Command{Kind: "cancel", EntityIDs: []int{e.ID}, Value: float64(i)}) != nil {
					return
				}
				continue
			}
			committed++
			i++
		}
	}
	for _, e := range w.entities(player, "") {
		if committed >= goal {
			return
		}
		if len(e.Tasks) > 0 || e.life.State() != Active {
			continue
		}
		product := ""
		switch e.Type {
		case "barracks":
			product = "militia"
		case "archery_range":
			product = "archer"
		case "stable":
			product = "scout"
			if p.Age >= 2 {
				product = "knight"
			}
		case "siege_workshop":
			product = "ram"
		case "castle":
			product = uniqueFor[p.Civilization]
		}
		if product != "" && w.canTrain(p, e, definitions[product]) == nil {
			if w.Apply(player, Command{Kind: "train", EntityIDs: []int{e.ID}, Product: product}) == nil {
				committed++
			}
		}
	}
}
func (w *World) hasOrBuilding(player int, typ string) bool { return len(w.entities(player, typ)) > 0 }
func (w *World) aiBuild(player int, typ string, center Vec, workers []*Entity) bool {
	if len(workers) == 0 {
		return false
	}
	d := definitions[typ]
	if w.canBuild(w.Players[player], d) != nil {
		return false
	}
	for _, e := range w.entities(player, typ) {
		if e.life.State() == Foundation {
			return false
		}
	}
	var worker *Entity
	for _, e := range workers {
		if e.Container == 0 && (!w.Players[player].voyaging(e.ID) || w.sameRegion(center, w.Players[player].NavalPlan.GoalLand, false)) && w.sameRegion(e.Position, center, false) && e.behavior.State() != Constructing && e.behavior.State() != Moving && (worker == nil || e.Cargo < worker.Cargo) {
			worker = e
		}
	}
	if worker == nil {
		return false
	}
	for r := 5.; r < 16; r += 2 {
		for a := 0.; a < math.Pi*2; a += .6 {
			pos := snap(Vec{center.X + math.Cos(a)*r, center.Y + math.Sin(a)*r})
			// Keep walkable lanes between buildings so the economy can deliver
			// cargo and newly trained armies can leave the settlement.
			clear := true
			if typ != "farm" {
				for _, building := range w.entities(player, "") {
					bd := definitions[building.Type]
					if bd.Kind == "building" && building.Type != "farm" && pos.Distance(building.Position) < d.Radius+bd.Radius+1.5 {
						clear = false
						break
					}
				}
			}
			if !clear {
				continue
			}
			if w.sameRegion(worker.Position, pos, false) && w.Placement(player, typ, pos) == nil {
				err := w.Apply(player, Command{Kind: "build", EntityIDs: []int{worker.ID}, Product: typ, Position: &pos})
				return err == nil
			}
		}
	}
	return false
}
