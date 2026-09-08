package game

import (
	"github.com/open-ships/statemachine"
	"math"
	"sort"
)

func New(cfg Config) *World {
	if cfg.World != (WorldOptions{}) {
		w, err := NewWorld(cfg)
		if err != nil {
			panic(err)
		}
		return w
	}
	if cfg.Settlements < 1 || cfg.Settlements > 6 {
		cfg.Settlements = 2
	}
	if !ValidCivilization(cfg.Civilization) {
		cfg.Civilization = "britons"
	}
	if !ValidDifficulty(cfg.Difficulty) {
		cfg.Difficulty = "normal"
	}
	if cfg.Seed == 0 {
		cfg.Seed = 4817
	}
	if cfg.Mode == "" {
		cfg.Mode = "skirmish"
	}
	w := &World{Config: cfg, Width: 72, Height: 72, Entities: map[int]*Entity{}, Players: map[int]*Player{}, Speed: 1.7, match: statemachine.NewInstance(matchMachine, MatchRunning), NextID: 1, rng: uint64(cfg.Seed), Projectiles: []Projectile{}}
	if cfg.Settlements > 2 {
		w.Width, w.Height = 108, 108
	}
	w.Tiles = make([]Tile, w.Width*w.Height)
	for y := 0; y < w.Height; y++ {
		for x := 0; x < w.Width; x++ {
			t := Tile{Terrain: "grass", Elevation: math.Max(0, math.Sin(float64(x)*.16)*math.Cos(float64(y)*.15)) * .3}
			river := 48 + int(math.Sin(float64(y)*.12)*3)
			if math.Abs(float64(x-river)) < 2 {
				t.Terrain = "water"
				t.Elevation = -.2
				if y >= 32 && y <= 37 || cfg.Settlements > 2 && y >= 73 && y <= 78 {
					t.Terrain = "shallows"
					t.Elevation = -.05
				}
			}
			if (x-14)*(x-14)+(y-12)*(y-12) < 30 {
				t.Terrain = "water"
				t.Elevation = -.2
			}
			if x < 2 || y < 2 || x >= w.Width-2 || y >= w.Height-2 {
				t.Terrain = "cliff"
				t.Elevation = .7
			}
			w.Tiles[y*w.Width+x] = t
		}
	}
	starts := []Vec{{19, 42}, {57, 22}, {89, 45}, {21, 85}, {57, 84}, {89, 86}}
	names := []string{"Your kingdom", "House of Ashford", "House of Briar", "House of Ravenwood", "House of Stonehaven", "House of Dunmere"}
	for id := 1; id <= cfg.Settlements; id++ {
		civ := cfg.Civilization
		name := "Your kingdom"
		if id > 1 {
			civ = civilizations[(id-1)%len(civilizations)].ID
			name = names[id-1]
		}
		w.Players[id] = &Player{ID: id, Start: starts[id-1], Name: name, Civilization: civ, Resources: Resources{Food: 200, Wood: 200, Gold: 100, Stone: 200}, Technologies: map[string]bool{}, lifecycle: statemachine.NewInstance(playerMachine, PlayerCompeting), AI: id > 1, Explored: make([]bool, len(w.Tiles)), Visible: make([]bool, len(w.Tiles)), Memory: map[int]EntityView{}}
		w.Players[id].strategy = statemachine.NewInstance(aiMachine, aiDeveloping)
		w.Players[id].Temperament = initialTemperament(cfg, id)
	}
	w.initializeRelations()
	w.initializeTreaty()
	w.initializeVoyages()
	for i, pos := range starts[:cfg.Settlements] {
		owner := i + 1
		w.spawn("town_center", owner, pos)
		for j := 0; j < 3; j++ {
			w.spawn("villager", owner, Vec{pos.X - 3 + float64(j), pos.Y + 3})
		}
		w.spawn("scout", owner, Vec{pos.X + 4, pos.Y + 3})
		for j := 0; j < 5; j++ {
			w.resource("berries", Vec{pos.X - 6 + float64(j%3), pos.Y - 5 + float64(j/3)})
		}
		for j := 0; j < 4; j++ {
			w.resource("sheep", Vec{pos.X + 3 + float64(j)*.6, pos.Y - 3})
		}
		for j := 0; j < 6; j++ {
			w.resource("gold", Vec{pos.X + 8 + float64(j%3)*1.3, pos.Y + float64(j/3)*1.4})
		}
		for j := 0; j < 4; j++ {
			w.resource("stone", Vec{pos.X - 5 + float64(j%2)*1.4, pos.Y + 7 + float64(j/2)*1.4})
		}
	}
	if cfg.Settlements > 2 {
		for _, pos := range starts[:cfg.Settlements] {
			for j := range 24 {
				angle := float64(j) * math.Pi / 12
				point := Vec{pos.X + math.Cos(angle)*11, pos.Y + math.Sin(angle)*11}
				if w.free(point, .5, 0, false) {
					w.resource("tree", point)
				}
			}
		}
	}
	for _, center := range []Vec{{10, 32}, {28, 31}, {29, 51}, {8, 52}, {36, 13}, {59, 10}, {64, 36}, {38, 49}, {26, 65}, {10, 20}, {31, 8}, {61, 58}} {
		for j := 0; j < 28; j++ {
			angle := w.random() * math.Pi * 2
			r := math.Sqrt(w.random()) * 4.5
			pos := Vec{center.X + math.Cos(angle)*r, center.Y + math.Sin(angle)*r}
			if w.land(pos) && w.free(pos, .4, 0, false) {
				w.resource("tree", pos)
			}
		}
	}
	for _, pos := range []Vec{{34, 24}, {37, 60}, {58, 49}} {
		for j := 0; j < 5; j++ {
			w.resource("gold", Vec{pos.X + float64(j%3)*1.3, pos.Y + float64(j/3)*1.3})
		}
	}
	for _, pos := range []Vec{{36, 34}, {35, 56}, {55, 41}} {
		w.resource("relic", pos)
	}
	for y := 8; y < 64; y += 7 {
		x := 48 + math.Sin(float64(y)*.12)*3
		if w.water(Vec{x, float64(y)}) {
			w.resource("fish", Vec{x, float64(y)})
		}
	}
	w.resource("fish", Vec{14, 12})
	if cfg.Mode == "sandbox" {
		for _, p := range w.Players {
			p.Resources = Resources{Food: 2000, Wood: 2000, Gold: 1500, Stone: 1500}
		}
	}
	w.spawn("market", 0, Vec{59, 51})
	w.rebuildRegions()
	w.refreshVisibility()
	w.event(1, "Your settlers await. Gather food and wood, build houses, and grow your kingdom.")
	return w
}
func (w *World) random() float64 {
	w.rng ^= w.rng << 13
	w.rng ^= w.rng >> 7
	w.rng ^= w.rng << 17
	return float64(w.rng>>11) / float64(uint64(1)<<53)
}
func (w *World) spawn(typ string, owner int, pos Vec) *Entity {
	return w.spawnWithLife(typ, owner, pos, Active)
}
func (w *World) spawnWithLife(typ string, owner int, pos Vec, initial LifeState) *Entity {
	e := &Entity{ID: w.NextID, Type: typ, Owner: owner, Position: pos, Progress: 1, life: statemachine.NewInstance(lifeMachine, initial), behavior: statemachine.NewInstance(unitMachine, Idle), production: statemachine.NewInstance(productionMachine, ProductionIdle), siege: statemachine.NewInstance(siegeMachine, SiegePacked), Order: Order{Kind: "idle"}, Faith: 100, Stance: "defensive", Tasks: []Task{}, Passengers: []int{}}
	w.NextID++
	e.HP = w.stats(e).HP
	if initial == Foundation {
		e.Progress = 0
		e.HP = 1
	}
	w.Entities[e.ID] = e
	w.IDs = append(w.IDs, e.ID)
	if typ == "farm" {
		e.Resource = "food"
		e.Amount = 175
		if w.Players[owner].Technologies["horse_collar"] {
			e.Amount += 75
		}
	}
	w.entityEvent(e, "created", definitions[typ].Name+" created", 0)
	return e
}
func (w *World) resource(typ string, pos Vec) {
	e := w.spawn(typ, 0, pos)
	e.Amount = definitions[typ].HP * w.resourceMultiplier()
	switch typ {
	case "berries", "sheep", "fish":
		e.Resource = "food"
	case "tree":
		e.Resource = "wood"
	case "gold":
		e.Resource = "gold"
	case "stone":
		e.Resource = "stone"
	}
}
func (w *World) inside(p Vec) bool {
	return p.Finite() && p.X >= 1 && p.Y >= 1 && p.X < float64(w.Width-1) && p.Y < float64(w.Height-1)
}
func (w *World) tile(p Vec) Tile {
	if !w.inside(p) {
		return Tile{Terrain: "cliff"}
	}
	return w.Tiles[int(p.Y)*w.Width+int(p.X)]
}
func (w *World) land(p Vec) bool { t := w.tile(p); return t.Terrain != "water" && t.Terrain != "cliff" }
func (w *World) water(p Vec) bool {
	t := w.tile(p)
	return t.Terrain == "water" || t.Terrain == "shallows"
}
func (w *World) event(player int, msg string) {
	w.record(Event{Kind: "notice", Message: msg}, nil, player)
}
func (w *World) population(player int) (n, cap int) {
	for _, id := range w.IDs {
		e := w.Entities[id]
		if e == nil || e.Owner != player {
			continue
		}
		d := definitions[e.Type]
		n += d.Population
		if e.Progress >= 1 {
			cap += d.Housing
		}
	}
	return n, min(cap, 200)
}
func (w *World) hasBuilding(player int, typ string) bool {
	for _, id := range w.IDs {
		e := w.Entities[id]
		if e != nil && e.Owner == player && e.Type == typ && e.Progress >= 1 {
			return true
		}
	}
	return false
}
func (w *World) entities(player int, typ string) []*Entity {
	out := []*Entity{}
	for _, id := range w.IDs {
		e := w.Entities[id]
		if e != nil && (player < 0 || e.Owner == player) && (typ == "" || e.Type == typ) {
			out = append(out, e)
		}
	}
	return out
}
func (w *World) visible(p int, pos Vec) bool {
	if !w.inside(pos) {
		return false
	}
	return w.Players[p].Visible[int(pos.Y)*w.Width+int(pos.X)]
}
func (w *World) visibleEntity(player int, e *Entity) bool {
	return e != nil && e.Container == 0 && (e.Owner == player || w.visible(player, e.Position))
}
func (w *World) remove(id int) {
	if e := w.Entities[id]; e != nil {
		mustFire(e.life, DestroyEntity, &entityContext{World: w, Actor: e})
	}
}
func (w *World) cleanupEntity(e *Entity) {
	mustFire(e.production, LoseProduction, &entityContext{World: w, Actor: e})
	if e.Relic {
		w.resource("relic", e.Position)
	}
	if e.Type == "monastery" {
		for i := 0; i < int(e.Amount); i++ {
			w.resource("relic", Vec{e.Position.X + float64(i)*.4, e.Position.Y})
		}
	}
	for _, pid := range append([]int{}, e.Passengers...) {
		w.remove(pid)
	}
	if e.Container != 0 {
		if holder := w.Entities[e.Container]; holder != nil {
			for i, pid := range holder.Passengers {
				if pid == e.ID {
					holder.Passengers = append(holder.Passengers[:i], holder.Passengers[i+1:]...)
					break
				}
			}
		}
	}
	delete(w.Entities, e.ID)
}
func (w *World) refreshVisibility() {
	for _, p := range w.Players {
		clear(p.Visible)
		if w.Config.World.Reveal == "explored" || w.Config.World.Reveal == "all" {
			for i := range p.Explored {
				p.Explored[i] = true
				p.Visible[i] = w.Config.World.Reveal == "all"
			}
		}
	}
	for _, id := range w.IDs {
		e := w.Entities[id]
		if e == nil || e.Owner == 0 || e.Container != 0 {
			continue
		}
		p := w.Players[e.Owner]
		r := w.stats(e).Sight
		if e.Progress < 1 {
			r = 3
		}
		for y := max(0, int(e.Position.Y-r)); y < min(w.Height, int(e.Position.Y+r)+1); y++ {
			for x := max(0, int(e.Position.X-r)); x < min(w.Width, int(e.Position.X+r)+1); x++ {
				if e.Position.Distance(Vec{float64(x) + .5, float64(y) + .5}) <= r {
					i := y*w.Width + x
					p.Visible[i] = true
					p.Explored[i] = true
				}
			}
		}
	}
	for player := 1; player <= w.Config.Settlements; player++ {
		p := w.Players[player]
		for id, v := range p.Memory {
			if w.visible(p.ID, v.Position) {
				delete(p.Memory, id)
			}
		}
		for _, id := range w.IDs {
			e := w.Entities[id]
			if e == nil || e.Owner == p.ID || !w.visibleEntity(p.ID, e) {
				continue
			}
			if len(w.journal.entities[p.ID][id]) == 0 {
				w.record(Event{Kind: "discovered", Message: definitions[e.Type].Name + " discovered"}, e, p.ID)
			}
			if definitions[e.Type].Kind != "unit" {
				v := w.entityView(e, 0)
				v.Visible = false
				p.Memory[id] = v
			}
		}
	}
}
func sortedKeys(m map[string]bool) []string {
	out := []string{}
	for k, v := range m {
		if v {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}
