package game

import (
	"sort"
	"sync"
)

var catalogOnce sync.Once
var definitionList []Definition
var technologyList []Technology

func orderedCatalog() ([]Definition, []Technology) {
	catalogOnce.Do(func() {
		for _, d := range definitions {
			definitionList = append(definitionList, d)
		}
		for _, t := range technologies {
			technologyList = append(technologyList, t)
		}
		sort.Slice(definitionList, func(i, j int) bool { return definitionList[i].ID < definitionList[j].ID })
		sort.Slice(technologyList, func(i, j int) bool { return technologyList[i].ID < technologyList[j].ID })
	})
	return definitionList, technologyList
}

var Ages = []string{"Dark Age", "Feudal Age", "Castle Age", "Imperial Age"}
var definitions = map[string]Definition{}
var technologies = map[string]Technology{}
var civilizations = []Civilization{
	{"britons", "Britons", "Archers & settlement", "Foot archers gain range in Castle and Imperial Age."},
	{"franks", "Franks", "Cavalry & castles", "Mounted melee units have 20% more health."},
	{"goths", "Goths", "Infantry & numbers", "Infantry costs 20% less."},
	{"teutons", "Teutons", "Armor & fortifications", "Infantry gains 2 melee armor."},
	{"japanese", "Japanese", "Infantry & fishing", "Infantry attacks 25% faster."},
	{"chinese", "Chinese", "Technology & archery", "Technology research costs 10% less."},
	{"byzantines", "Byzantines", "Defense & endurance", "Buildings have 25% more health."},
	{"persians", "Persians", "Economy & heavy cavalry", "Villagers train 15% faster."},
	{"saracens", "Saracens", "Camels & commerce", "Market exchange returns 20% more gold."},
	{"turks", "Turks", "Gunpowder & gold", "Gold gathering is 20% faster."},
	{"vikings", "Vikings", "Infantry & seafaring", "Infantry has 20% more health."},
	{"mongols", "Mongols", "Horse archers & hunting", "Food gathering is 20% faster."},
	{"celts", "Celts", "Forests & siege", "Wood gathering is 15% faster."},
}
var uniqueFor = map[string]string{"britons": "longbowman", "franks": "axeman", "goths": "huskarl", "teutons": "teutonic_knight", "japanese": "samurai", "chinese": "chu_ko_nu", "byzantines": "cataphract", "persians": "war_elephant", "saracens": "mameluke", "turks": "janissary", "vikings": "berserk", "mongols": "mangudai", "celts": "woad_raider"}

func init() {
	building := func(id, name string, age int, cost Resources, hp, radius, time float64, house int, drop ...string) {
		definitions[id] = Definition{ID: id, Name: name, Kind: "building", Age: age, Cost: cost, HP: hp, Radius: radius, Time: time, Housing: house, Sight: 9, DropOff: drop, Description: "Construct " + name + " with villagers."}
	}
	building("town_center", "Town Center", 0, Resources{Wood: 275, Stone: 100}, 2400, 2, 100, 5, "food", "wood", "gold", "stone")
	building("house", "House", 0, Resources{Wood: 25}, 550, .9, 25, 5)
	building("mill", "Mill", 0, Resources{Wood: 100}, 600, 1.3, 35, 0, "food")
	building("lumber_camp", "Lumber Camp", 0, Resources{Wood: 100}, 600, 1.2, 35, 0, "wood")
	building("mining_camp", "Mining Camp", 0, Resources{Wood: 100}, 600, 1.2, 35, 0, "gold", "stone")
	building("farm", "Farm", 0, Resources{Wood: 60}, 480, 1.25, 15, 0)
	building("barracks", "Barracks", 0, Resources{Wood: 175}, 1200, 1.6, 50, 0)
	building("archery_range", "Archery Range", 1, Resources{Wood: 175}, 1500, 1.6, 50, 0)
	building("stable", "Stable", 1, Resources{Wood: 175}, 1500, 1.6, 50, 0)
	building("blacksmith", "Blacksmith", 1, Resources{Wood: 150}, 1800, 1.3, 40, 0)
	building("market", "Market", 1, Resources{Wood: 175}, 1800, 1.6, 60, 0)
	building("tower", "Watch Tower", 1, Resources{Wood: 35, Stone: 125}, 850, .7, 80, 0)
	building("wall", "Stone Wall", 1, Resources{Stone: 5}, 900, .48, 10, 0)
	building("gate", "Stone Gate", 1, Resources{Stone: 30}, 1500, .9, 40, 0)
	building("palisade", "Palisade", 0, Resources{Wood: 3}, 150, .48, 7, 0)
	building("castle", "Castle", 2, Resources{Stone: 650}, 4800, 2, 200, 20)
	building("siege_workshop", "Siege Workshop", 2, Resources{Wood: 200}, 1800, 1.6, 40, 0)
	building("monastery", "Monastery", 2, Resources{Wood: 175}, 2100, 1.5, 40, 0)
	building("university", "University", 2, Resources{Wood: 200}, 2100, 1.6, 60, 0)
	building("dock", "Dock", 0, Resources{Wood: 150}, 1800, 1.6, 35, 0, "food")
	building("wonder", "Wonder", 3, Resources{Wood: 1000, Gold: 1000, Stone: 1000}, 4800, 2.5, 600, 0)
	for _, id := range []string{"tower", "castle", "town_center"} {
		d := definitions[id]
		d.Attack = 6
		d.Range = 8
		d.Reload = 2
		d.Projectile = true
		if id == "castle" {
			d.Attack = 14
			d.Range = 10
		}
		definitions[id] = d
	}
	for _, id := range []string{"archery_range", "stable"} {
		d := definitions[id]
		d.Prerequisite = "barracks"
		definitions[id] = d
	}
	unit := func(id, name, producer, class string, age int, cost Resources, hp, speed, attack, rng, tm float64) {
		definitions[id] = Definition{ID: id, Name: name, Kind: "unit", Age: age, Cost: cost, HP: hp, Radius: .25, Speed: speed, Attack: attack, Range: rng, Reload: 2, Sight: 7, Population: 1, Producer: producer, Class: class, Time: tm, Projectile: rng > 2, Description: name + " · " + class}
	}
	unit("villager", "Villager", "town_center", "worker", 0, Resources{Food: 50}, 25, .8, 3, .6, 25)
	unit("militia", "Militia", "barracks", "infantry", 0, Resources{Food: 50, Gold: 20}, 40, .9, 4, .65, 21)
	unit("spearman", "Spearman", "barracks", "infantry", 1, Resources{Food: 35, Wood: 25}, 45, 1, 3, .8, 22)
	unit("archer", "Archer", "archery_range", "archer", 1, Resources{Wood: 25, Gold: 45}, 30, .96, 4, 4, 35)
	unit("skirmisher", "Skirmisher", "archery_range", "archer", 1, Resources{Food: 25, Wood: 35}, 30, .96, 3, 4, 26)
	unit("scout", "Scout Cavalry", "stable", "cavalry", 1, Resources{Food: 80}, 45, 1.5, 5, .75, 30)
	unit("knight", "Knight", "stable", "cavalry", 2, Resources{Food: 60, Gold: 75}, 100, 1.35, 10, .8, 30)
	unit("camel", "Camel Rider", "stable", "cavalry", 2, Resources{Food: 55, Gold: 60}, 100, 1.4, 6, .9, 22)
	unit("cavalry_archer", "Cavalry Archer", "archery_range", "mounted_archer", 2, Resources{Wood: 40, Gold: 60}, 50, 1.4, 6, 4, 35)
	unit("monk", "Monk", "monastery", "monk", 2, Resources{Gold: 100}, 30, .7, 0, 0, 51)
	unit("ram", "Battering Ram", "siege_workshop", "siege", 2, Resources{Wood: 160, Gold: 75}, 175, .6, 2, .8, 36)
	unit("mangonel", "Mangonel", "siege_workshop", "siege", 2, Resources{Wood: 160, Gold: 135}, 50, .6, 40, 7, 46)
	unit("trebuchet", "Trebuchet", "castle", "siege", 3, Resources{Wood: 200, Gold: 200}, 150, .65, 180, 15, 50)
	unit("hand_cannoneer", "Hand Cannoneer", "archery_range", "archer", 3, Resources{Food: 45, Gold: 50}, 40, .96, 17, 7, 34)
	unit("bombard_cannon", "Bombard Cannon", "siege_workshop", "siege", 3, Resources{Wood: 225, Gold: 225}, 80, .7, 40, 11, 56)
	unit("fishing_ship", "Fishing Ship", "dock", "worker", 0, Resources{Wood: 75}, 60, 1.25, 0, 0, 40)
	unit("galley", "Galley", "dock", "ship", 1, Resources{Wood: 90, Gold: 30}, 110, 1.36, 6, 5, 45)
	unit("fire_ship", "Fire Ship", "dock", "ship", 1, Resources{Wood: 75, Gold: 45}, 120, 1.3, 10, 2, 40)
	unit("transport", "Transport Ship", "dock", "ship", 1, Resources{Wood: 125}, 150, 1.4, 0, 0, 46)
	unit("trade_cart", "Trade Cart", "market", "trader", 1, Resources{Wood: 100, Gold: 50}, 70, 1.3, 0, 0, 50)
	for _, id := range []string{"fishing_ship", "galley", "fire_ship", "transport"} {
		d := definitions[id]
		d.Naval = true
		d.Radius = .45
		definitions[id] = d
	}
	for _, id := range []string{"knight", "camel"} {
		d := definitions[id]
		d.Armor = 2
		d.PierceArmor = 2
		definitions[id] = d
	}
	for _, id := range []string{"mangonel", "trebuchet", "bombard_cannon"} {
		d := definitions[id]
		d.MinRange = 2
		d.Reload = 6
		definitions[id] = d
	}
	d := definitions["ram"]
	d.PierceArmor = 100
	d.Reload = 5
	definitions["ram"] = d
	d = definitions["skirmisher"]
	d.PierceArmor = 3
	d.MinRange = 1
	definitions["skirmisher"] = d
	for _, v := range []struct{ id, name, base string }{
		{"longbowman", "Longbowman", "archer"}, {"axeman", "Throwing Axeman", "militia"}, {"huskarl", "Huskarl", "militia"}, {"teutonic_knight", "Teutonic Knight", "militia"}, {"samurai", "Samurai", "militia"}, {"chu_ko_nu", "Chu Ko Nu", "archer"}, {"cataphract", "Cataphract", "knight"}, {"war_elephant", "War Elephant", "knight"}, {"mameluke", "Mameluke", "camel"}, {"janissary", "Janissary", "hand_cannoneer"}, {"berserk", "Berserk", "militia"}, {"mangudai", "Mangudai", "cavalry_archer"}, {"woad_raider", "Woad Raider", "militia"},
	} {
		d := definitions[v.base]
		d.ID = v.id
		d.Name = v.name
		d.Producer = "castle"
		d.Age = 2
		d.HP += 20
		d.Attack += 3
		d.Cost = Resources{Food: 60, Gold: 50}
		d.Description = "Civilization unique unit"
		definitions[v.id] = d
	}
	d = definitions["longbowman"]
	d.Range = 6
	definitions[d.ID] = d
	d = definitions["teutonic_knight"]
	d.Armor = 8
	d.Speed = .65
	definitions[d.ID] = d
	d = definitions["huskarl"]
	d.PierceArmor = 6
	definitions[d.ID] = d
	d = definitions["war_elephant"]
	d.HP = 400
	d.Speed = .65
	d.Attack = 18
	d.Radius = .45
	d.Cost = Resources{Food: 200, Gold: 75}
	definitions[d.ID] = d
	d = definitions["woad_raider"]
	d.Speed = 1.3
	definitions[d.ID] = d
	for _, id := range []string{"axeman", "mameluke"} {
		d := definitions[id]
		d.Range = 3
		d.Projectile = true
		definitions[id] = d
	}
	for _, v := range []struct {
		id, name, res  string
		amount, radius float64
	}{
		{"tree", "Forest", "wood", 100, .35}, {"berries", "Forage Bush", "food", 125, .45}, {"sheep", "Sheep", "food", 100, .25}, {"gold", "Gold Deposit", "gold", 800, .65}, {"stone", "Stone Deposit", "stone", 350, .65}, {"fish", "Fish Shoal", "food", 400, .45}, {"relic", "Relic", "", 1, .25},
	} {
		definitions[v.id] = Definition{ID: v.id, Name: v.name, Kind: "resource", Radius: v.radius, HP: v.amount, Description: "Explore and gather."}
	}
	tech := func(id, name, producer string, age int, cost Resources, tm float64, desc string) {
		technologies[id] = Technology{ID: id, Name: name, Producer: producer, Age: age, Cost: cost, Time: tm, Description: desc}
	}
	tech("loom", "Loom", "town_center", 0, Resources{Gold: 50}, 25, "Villagers gain 15 health and armor.")
	tech("wheelbarrow", "Wheelbarrow", "town_center", 1, Resources{Food: 175, Wood: 50}, 75, "Villagers carry more and walk 10% faster.")
	tech("double_bit_axe", "Double-Bit Axe", "lumber_camp", 1, Resources{Food: 100, Wood: 50}, 25, "Wood gathering improves by 20%.")
	tech("horse_collar", "Horse Collar", "mill", 1, Resources{Food: 75, Wood: 75}, 20, "New farms contain 75 additional food.")
	tech("gold_mining", "Gold Mining", "mining_camp", 1, Resources{Food: 100, Wood: 75}, 30, "Gold gathering improves by 15%.")
	tech("forging", "Forging", "blacksmith", 1, Resources{Food: 150}, 50, "Melee attack increases by 1.")
	tech("fletching", "Fletching", "blacksmith", 1, Resources{Food: 100, Gold: 50}, 30, "Ranged attack and range increase by 1.")
	tech("scale_armor", "Scale Armor", "blacksmith", 1, Resources{Food: 100}, 40, "Military units gain 1 melee and pierce armor.")
	tech("man_at_arms", "Man-at-Arms", "barracks", 1, Resources{Food: 100, Gold: 40}, 40, "Militia gain 10 health and 2 attack.")
	tech("crossbow", "Crossbowman", "archery_range", 2, Resources{Food: 175, Gold: 125}, 35, "Archers gain 5 health, 1 attack and 1 range.")
	tech("pikeman", "Pikeman", "barracks", 2, Resources{Food: 215, Gold: 90}, 45, "Spearmen gain health, attack and cavalry bonus damage.")
	tech("bodkin", "Bodkin Arrow", "blacksmith", 2, Resources{Food: 200, Gold: 100}, 35, "Ranged attack and range increase by another 1.")
	t := technologies["bodkin"]
	t.Prerequisite = "fletching"
	technologies[t.ID] = t
	tech("ballistics", "Ballistics", "university", 2, Resources{Wood: 300, Gold: 175}, 60, "Projectiles follow moving targets.")
	tech("sanctity", "Sanctity", "monastery", 2, Resources{Gold: 120}, 60, "Monks gain 15 health.")
	tech("redemption", "Redemption", "monastery", 2, Resources{Gold: 475}, 50, "Monks can convert siege weapons and ordinary buildings.")
	tech("chemistry", "Chemistry", "university", 3, Resources{Food: 300, Gold: 200}, 100, "Unlocks gunpowder units and adds 1 ranged attack.")
	tech("cavalier", "Cavalier", "stable", 3, Resources{Food: 300, Gold: 300}, 100, "Knights gain 20 health and 2 attack.")
	tech("champion", "Champion", "barracks", 3, Resources{Food: 750, Gold: 350}, 100, "Militia gain 30 health and 6 attack.")
	tech("elite", "Elite Guard", "castle", 3, Resources{Food: 700, Gold: 500}, 80, "Unique units gain 25 health and 3 attack.")
	tech("siege_engineers", "Siege Engineers", "university", 3, Resources{Food: 500, Wood: 600}, 60, "Siege gains 1 range and bonus building damage.")
}

func GetCatalog() Catalog {
	c := Catalog{RulesVersion: RulesVersion, Civilizations: civilizations, Ages: Ages, Definitions: []Definition{}, Technologies: []Technology{}}
	c.Speeds = append([]float64{}, gameSpeeds...)
	for _, policy := range aiPolicies {
		c.Difficulties = append(c.Difficulties, policy.Difficulty)
	}
	c.LogFilters = append([]LogFilter{}, logFilters...)
	c.SettlementCounts = []int{1, 2, 3, 4, 5, 6}
	defs, techs := orderedCatalog()
	for _, d := range defs {
		d.DropOff = append([]string(nil), d.DropOff...)
		c.Definitions = append(c.Definitions, d)
	}
	for _, t := range techs {
		c.Technologies = append(c.Technologies, t)
	}
	return c
}
func ValidCivilization(id string) bool {
	for _, c := range civilizations {
		if c.ID == id {
			return true
		}
	}
	return false
}
func (w *World) stats(e *Entity) Definition {
	d := definitions[e.Type]
	p := w.Players[e.Owner]
	if p == nil {
		return d
	}
	t := p.Technologies
	if p.Civilization == "byzantines" && d.Kind == "building" {
		d.HP *= 1.25
	}
	if p.Civilization == "franks" && d.Class == "cavalry" {
		d.HP *= 1.2
	}
	if p.Civilization == "vikings" && d.Class == "infantry" {
		d.HP *= 1.2
	}
	if p.Civilization == "teutons" && d.Class == "infantry" {
		d.Armor += 2
	}
	if p.Civilization == "japanese" && d.Class == "infantry" {
		d.Reload /= 1.25
	}
	if p.Civilization == "britons" && d.Class == "archer" {
		d.Range += float64(max(0, p.Age-1))
	}
	if e.Type == "villager" {
		if t["loom"] {
			d.HP += 15
			d.Armor += 1
			d.PierceArmor += 2
		}
		if t["wheelbarrow"] {
			d.Speed *= 1.1
		}
	}
	if e.Type == "monk" && t["sanctity"] {
		d.HP += 15
	}
	if d.Kind == "unit" && d.Class != "worker" {
		if t["scale_armor"] {
			d.Armor++
			d.PierceArmor++
		}
		if t["forging"] && !d.Projectile {
			d.Attack++
		}
	}
	if d.Projectile && d.Attack > 0 {
		for _, k := range []string{"fletching", "bodkin"} {
			if t[k] {
				d.Attack++
				d.Range++
			}
		}
		if t["chemistry"] {
			d.Attack++
		}
	}
	if e.Type == "militia" {
		if t["man_at_arms"] {
			d.Name = "Man-at-Arms"
			d.HP += 10
			d.Attack += 2
		}
		if t["champion"] {
			d.Name = "Champion"
			d.HP += 30
			d.Attack += 6
		}
	}
	if e.Type == "archer" && t["crossbow"] {
		d.Name = "Crossbowman"
		d.HP += 5
		d.Attack++
		d.Range++
	}
	if e.Type == "spearman" && t["pikeman"] {
		d.Name = "Pikeman"
		d.HP += 10
		d.Attack++
	}
	if e.Type == "knight" && t["cavalier"] {
		d.Name = "Cavalier"
		d.HP += 20
		d.Attack += 2
	}
	if e.Type == uniqueFor[p.Civilization] && t["elite"] {
		d.Name = "Elite " + d.Name
		d.HP += 25
		d.Attack += 3
	}
	if d.Class == "siege" && t["siege_engineers"] {
		d.Range++
	}
	return d
}
func (w *World) cost(p *Player, d Definition) Resources {
	c := d.Cost
	if p.Civilization == "goths" && d.Class == "infantry" {
		c = c.Scale(.8)
	}
	return c
}
func techCost(p *Player, t Technology) Resources {
	if p.Civilization == "chinese" {
		return t.Cost.Scale(.9)
	}
	return t.Cost
}
