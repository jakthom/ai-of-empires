package game

import (
	"fmt"
	"math"
	"math/rand/v2"

	"github.com/open-ships/statemachine"
)

// NewWorld is the validated creation boundary. New with an omitted World keeps
// the original layout for simulation fixtures; all HTTP creation uses this path.
func NewWorld(cfg Config) (*World, error) {
	return NewWorldForRoster(cfg, nil)
}

// Kingdom describes the finalized seats. Controllers are supplied by the
// authenticated session service, never by gameplay commands.
type Kingdom struct {
	Name         string `json:"name"`
	Civilization string `json:"civilization"`
	Human        bool   `json:"human"`
}

func NormalizeConfig(cfg Config) (Config, error) {
	options, err := normalizeWorldOptions(cfg.World)
	if err != nil {
		return cfg, err
	}
	cfg.World = options
	if cfg.Settlements == 0 {
		cfg.Settlements = 2
	}
	if cfg.Settlements < 1 || cfg.Settlements > 6 {
		return cfg, rule("invalid_settlements", "Choose between one and six settlements.")
	}
	if cfg.Seed == 0 {
		cfg.Seed = 4817
	}
	if cfg.Seed < 1 || cfg.Seed > 999999999 {
		return cfg, rule("invalid_seed", "Use a seed between 1 and 999999999.")
	}
	if !ValidCivilization(cfg.Civilization) {
		cfg.Civilization = "britons"
	}
	if !ValidDifficulty(cfg.Difficulty) {
		cfg.Difficulty = "normal"
	}
	if cfg.Mode == "" {
		cfg.Mode = "skirmish"
	}
	if cfg.Mode != "skirmish" && cfg.Mode != "sandbox" {
		return cfg, rule("invalid_mode", "Choose skirmish or sandbox.")
	}
	return cfg, nil
}

func NewWorldForRoster(cfg Config, roster []Kingdom) (*World, error) {
	cfg, err := NormalizeConfig(cfg)
	if err != nil {
		return nil, err
	}
	if roster != nil && len(roster) != cfg.Settlements {
		return nil, rule("invalid_roster", "Every kingdom needs a seat.")
	}
	for _, seat := range roster {
		if !ValidCivilization(seat.Civilization) {
			return nil, rule("invalid_civilization", "Choose a civilization from the catalog.")
		}
	}
	options := cfg.World
	size := worldSize(options)
	w := &World{Config: cfg, Generation: 1, Width: size, Height: size, Entities: map[int]*Entity{}, Players: map[int]*Player{}, Speed: 1.7, match: statemachine.NewInstance(matchMachine, MatchRunning), NextID: 1, rng: uint64(cfg.Seed), Projectiles: []Projectile{}}
	rng := rand.New(rand.NewPCG(uint64(cfg.Seed), 0x776f726c64))
	starts := worldStarts(cfg, rng)
	w.generateTerrain(starts, rng)
	if cfg.World.Type == "rivers" {
		if err := w.connectRiver(starts); err != nil {
			return nil, err
		}
	}
	w.rebuildRegions()
	if w.trimSmallWaterPockets() {
		w.rebuildRegions()
	}
	if err := w.validateTerrain(starts); err != nil {
		return nil, rule("generation_failed", err.Error())
	}
	names := []string{"Your kingdom", "House of Ashford", "House of Briar", "House of Ravenwood", "House of Stonehaven", "House of Dunmere"}
	for i, start := range starts {
		id, civ := i+1, cfg.Civilization
		if id > 1 {
			civ = civilizations[i%len(civilizations)].ID
		}
		p := &Player{ID: id, Start: start, Name: names[i], Civilization: civ, Resources: Resources{Food: 200, Wood: 200, Gold: 100, Stone: 200}, Technologies: map[string]bool{}, AI: id > 1, Explored: make([]bool, len(w.Tiles)), Visible: make([]bool, len(w.Tiles)), Memory: map[int]EntityView{}, lifecycle: statemachine.NewInstance(playerMachine, PlayerCompeting), strategy: statemachine.NewInstance(aiMachine, aiDeveloping), Temperament: initialTemperament(cfg, id)}
		if roster != nil {
			p.Name, p.Civilization, p.AI = roster[i].Name, roster[i].Civilization, !roster[i].Human
		}
		if cfg.Mode == "sandbox" {
			p.Resources = Resources{Food: 2000, Wood: 2000, Gold: 1500, Stone: 1500}
		}
		w.Players[id] = p
	}
	w.initializeRelations()
	w.initializeTreaty()
	w.initializeVoyages()
	for i, start := range starts {
		w.seedSettlement(i+1, start)
	}
	w.seedCountryside(starts, rng)
	w.assignRegionalBiomes()
	w.refreshVisibility()
	w.event(1, "Your settlers await. Gather food and wood, build houses, and grow your kingdom.")
	if options.TreatyMinutes > 0 {
		w.event(1, fmt.Sprintf("An initial peace period protects every kingdom for %d game minutes.", options.TreatyMinutes))
	}
	return w, nil
}

func worldStarts(cfg Config, rng *rand.Rand) []Vec {
	n, size := cfg.Settlements, float64(worldSize(cfg.World))
	center := Vec{size / 2, size / 2}
	fraction := map[string]float64{"close": .24, "standard": .31, "far": .38}[cfg.World.Separation]
	radius := size * fraction
	if n > 1 {
		radius = math.Max(radius, 30.5/(2*math.Sin(math.Pi/float64(n))))
	}
	radius = math.Min(radius, size/2-16)
	angle := rng.Float64() * math.Pi * 2
	starts := make([]Vec, n)
	for i := range starts {
		a := angle + float64(i)*2*math.Pi/float64(n)
		starts[i] = snap(Vec{center.X + math.Cos(a)*radius, center.Y + math.Sin(a)*radius})
	}
	if cfg.World.Type == "coast" || cfg.World.Type == "fjords" || cfg.World.Type == "protected" {
		// Two rows keep all six home economies on the mainland even on Small.
		cols := min(n, 3)
		spread := math.Min(size-34, math.Max(float64(cols-1)*29, size*fraction*2))
		for i := range starts {
			x := size / 2
			if cols > 1 {
				x = size/2 - spread/2 + float64(i%cols)*spread/float64(cols-1)
			}
			y := size * .36
			if n > 3 {
				y = 17 + float64(i/3)*(size*.65-32)
			}
			if cfg.World.Type == "protected" {
				y = size / 2
				if n > 3 {
					gap := math.Min(size-34, math.Max(29, size*fraction))
					y += (float64(i/3) - .5) * gap
				}
			}
			starts[i] = snap(Vec{x, y})
		}
	}
	return starts
}

func segmentDistance(p, a, b Vec) float64 {
	dx, dy := b.X-a.X, b.Y-a.Y
	u := math.Max(0, math.Min(1, ((p.X-a.X)*dx+(p.Y-a.Y)*dy)/math.Max(.0001, dx*dx+dy*dy)))
	return p.Distance(Vec{a.X + u*dx, a.Y + u*dy})
}

func (w *World) generateTerrain(starts []Vec, rng *rand.Rand) {
	w.Tiles = make([]Tile, w.Width*w.Height)
	s := float64(w.Width)
	phase, angle := rng.Float64()*6.28, rng.Float64()*6.28
	c, sn := math.Cos(angle), math.Sin(angle)
	center := Vec{s / 2, s / 2}
	islands := archipelagoIslands(starts, s, phase)
	for y := 0; y < w.Height; y++ {
		for x := 0; x < w.Width; x++ {
			p := Vec{float64(x) + .5, float64(y) + .5}
			u, v := (p.X-s/2)*c+(p.Y-s/2)*sn, -(p.X-s/2)*sn+(p.Y-s/2)*c
			t := Tile{Terrain: "grass", Elevation: .13 + .12*math.Sin(p.X*.11+phase)*math.Cos(p.Y*.13-phase)}
			water := false
			switch w.Config.World.Type {
			case "rivers":
				water = math.Abs(u-math.Sin(v*.085+phase)*s*.055) < 3
			case "lakes":
				water = math.Hypot(u-s*.21, v*.85) < s*.12 || math.Hypot(u+s*.21, v*.85) < s*.12
			case "coast":
				water = p.Y > s*.70+math.Sin(p.X*.07+phase)*2.5
			case "islands":
				water = p.Distance(center) > 7
				for _, home := range starts {
					if p.Distance(home) < 13.8 {
						water = false
					}
				}
			case "highlands":
				t.Elevation = .2 + .9*math.Pow(math.Max(0, math.Sin(u*.10+phase)*math.Cos(v*.08)), 2)
				if math.Abs(v-math.Sin(u*.08+phase)*7) < 2.5 && math.Abs(u) > 8 {
					profile := math.Pow(1-math.Abs(v-math.Sin(u*.08+phase)*7)/2.5, .8)
					t = Tile{Terrain: "cliff", Elevation: .55 + (1.2+.7*math.Sin(u*.13)*math.Sin(u*.13))*profile}
				}
			case "mountain_lakes":
				wave := math.Sin(u*.065+phase) * math.Cos(v*.075-phase)
				t.Elevation = .18 + 1.65*math.Pow(math.Max(0, wave), 2)
				if wave > .82 {
					t.Terrain = "cliff"
				}
				water = math.Hypot(u-s*.22, v*.8+s*.12) < s*.1 || math.Hypot(u+s*.23, v*.9-s*.12) < s*.115 || math.Hypot(u-s*.05, v+s*.31) < s*.075
			case "wetlands":
				t.Elevation = .1 + .2*math.Pow(math.Sin(u*.055+phase), 2)
				bend := math.Sin(v*.065+phase) * s * .045
				water = math.Abs(u-bend) < 2.6 || math.Abs(u-bend-s*.23) < 2.2 || math.Abs(u-bend+s*.23) < 2.2
			case "fjords":
				inlet := math.Pow(.5+.5*math.Cos(p.X/s*math.Pi*6+phase), 6)
				shore := s * (.76 - .36*inlet)
				water = p.Y > shore
				t.Elevation = .2 + 1.3*math.Pow(math.Max(0, math.Sin(u*.08+phase)*math.Cos(v*.06)), 2)
				if !water && p.Y > shore-2.5 && p.Y < s*.62 {
					t.Terrain = "cliff"
					t.Elevation += .5
				}
			case "archipelago":
				water = p.Distance(center) > math.Max(7, s*.04)
				for _, island := range islands {
					a := math.Atan2(p.Y-island.Center.Y, p.X-island.Center.X)
					if p.Distance(island.Center) < island.Radius*(.94+.06*math.Sin(3*a+phase)) {
						water = false
					}
				}
			}
			if water {
				t = Tile{Terrain: "water", Elevation: -.2}
			}
			// Safe flat homes are identical in every biome, with a gentle outer lip.
			for _, home := range starts {
				distance := p.Distance(home)
				if distance < 12.8 || w.Config.World.Type == "protected" && math.Abs(p.X-home.X) < 14 && math.Abs(p.Y-home.Y) < 14 {
					t = Tile{Terrain: "grass", Elevation: .12}
				}
			}
			if !w.islandWorld() {
				for _, home := range starts {
					if segmentDistance(p, home, center) < 2.4 {
						if t.Terrain == "water" {
							t = Tile{Terrain: "shallows", Elevation: -.05}
						}
						if t.Terrain == "cliff" {
							t = Tile{Terrain: "grass", Elevation: .25}
						}
					}
				}
			}
			if p.Distance(center) < 6.5 {
				t = Tile{Terrain: "grass", Elevation: .15}
			}
			if x < 2 || y < 2 || x >= w.Width-2 || y >= w.Height-2 {
				t = Tile{Terrain: "cliff", Elevation: .9}
			}
			w.Tiles[y*w.Width+x] = t
		}
	}
}

func (w *World) seedSettlement(owner int, pos Vec) {
	w.spawn("town_center", owner, pos)
	for j := range 3 {
		w.spawn("villager", owner, Vec{pos.X - 2 + float64(j)*2, pos.Y + 3})
	}
	w.spawn("scout", owner, Vec{pos.X + 4, pos.Y + 3})
	for j := range 5 {
		w.resource("berries", Vec{pos.X - 6 + float64(j%3), pos.Y - 5 + float64(j/3)})
	}
	for j := range 4 {
		w.resource("sheep", Vec{pos.X + 3 + float64(j)*.65, pos.Y - 4})
	}
	for j := range 6 {
		w.resource("gold", Vec{pos.X + 7 + float64(j%3)*1.4, pos.Y + float64(j/3)*1.4})
	}
	for j := range 4 {
		w.resource("stone", Vec{pos.X - 6 + float64(j%2)*1.5, pos.Y + 6 + float64(j/2)*1.5})
	}
	for j := range 12 {
		w.resource("tree", Vec{pos.X - 9 + float64(j%3)*.9, pos.Y - 1 + float64(j/3)*.9})
		w.resource("tree", Vec{pos.X + 1 + float64(j%3)*.9, pos.Y + 8 + float64(j/3)*.9})
	}
	if w.Config.World.Type == "protected" {
		// Gates are ordinary owned gates: friends pass, rivals must break in.
		for side := range 4 {
			for j := -11; j <= 11; j++ {
				if j == -1 || j == 1 {
					continue
				}
				typ := "palisade"
				if j == 0 {
					typ = "gate"
				}
				p := Vec{pos.X + float64(j), pos.Y - 12}
				switch side {
				case 1:
					p = Vec{pos.X + 12, pos.Y + float64(j)}
				case 2:
					p = Vec{pos.X - float64(j), pos.Y + 12}
				case 3:
					p = Vec{pos.X - 12, pos.Y - float64(j)}
				}
				// The corners extend beyond the circular home clearing.
				for dy := -1; dy <= 1; dy++ {
					for dx := -1; dx <= 1; dx++ {
						q := Vec{p.X + float64(dx), p.Y + float64(dy)}
						if w.inside(q) {
							w.Tiles[int(q.Y)*w.Width+int(q.X)] = Tile{Terrain: "grass", Elevation: .12}
						}
					}
				}
				w.spawn(typ, owner, p)
			}
		}
		for _, d := range []Vec{{-12, -12}, {12, -12}, {12, 12}, {-12, 12}} {
			w.spawn("palisade", owner, Vec{pos.X + d.X, pos.Y + d.Y})
		}
	}
}

func (w *World) seedCountryside(starts []Vec, rng *rand.Rand) {
	center := Vec{float64(w.Width) / 2, float64(w.Height) / 2}
	reserved := func(p Vec) bool {
		if p.Distance(center) < 8 {
			return true
		}
		for _, home := range starts {
			if p.Distance(home) < 15 || !w.islandWorld() && segmentDistance(p, home, center) < 2.8 {
				return true
			}
		}
		return false
	}
	// Bounded node counts: abundance changes amounts rather than multiplying
	// thousands of entities, snapshot payloads, or pathfinding obstacles.
	count := w.Width * 4
	if w.Config.World.Type == "forest" {
		count = w.Width * 9
	}
	for attempt, placed := 0, 0; attempt < count*15 && placed < count; attempt++ {
		p := Vec{3 + rng.Float64()*float64(w.Width-6), 3 + rng.Float64()*float64(w.Height-6)}
		if reserved(p) || w.tile(p).Terrain != "grass" || !w.free(p, .45, 0, false) {
			continue
		}
		if w.Config.World.Type != "forest" && math.Sin(p.X*.24)*math.Cos(p.Y*.21) < .25 {
			continue
		}
		w.resource("tree", p)
		placed++
	}
	for j := 0; j < w.Width/3; j++ {
		for attempt := 0; attempt < 100; attempt++ {
			p := Vec{5 + rng.Float64()*float64(w.Width-10), 5 + rng.Float64()*float64(w.Height-10)}
			if reserved(p) || w.tile(p).Terrain != "grass" || !w.free(p, 1, 0, false) {
				continue
			}
			typ := "gold"
			if j%2 == 1 {
				typ = "stone"
			}
			w.resource(typ, p)
			break
		}
	}
	w.seedFishing(starts, rng)
	for _, offset := range []Vec{{-3, -2}, {0, -3}, {3, -2}} {
		p := Vec{center.X + offset.X, center.Y + offset.Y}
		if w.free(p, .4, 0, false) {
			w.resource("relic", p)
		}
	}
	for r := 3.; r < 9; r++ {
		p := Vec{center.X, center.Y + r}
		if w.free(p, 2, 0, false) {
			w.spawn("market", 0, p)
			break
		}
	}
	w.rebuildRegions()
}

// Terrain components are derived, never checkpoint authorities. They prevent
// known impossible journeys without revealing entities through fog of war.
func (w *World) rebuildRegions() {
	w.landRegions, w.waterRegions = w.regions(false), w.regions(true)
}
func (w *World) regions(naval bool) []int {
	regions := make([]int, len(w.Tiles))
	next := 0
	passable := func(i int) bool {
		t := w.Tiles[i].Terrain
		if naval {
			return t == "water" || t == "shallows"
		}
		return t != "water" && t != "cliff"
	}
	queue := make([]int, 0, len(w.Tiles))
	for i := range regions {
		if regions[i] != 0 || !passable(i) {
			continue
		}
		next++
		regions[i] = next
		queue = append(queue[:0], i)
		for head := 0; head < len(queue); head++ {
			k := queue[head]
			x, y := k%w.Width, k/w.Width
			for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				nx, ny := x+d[0], y+d[1]
				if nx < 0 || ny < 0 || nx >= w.Width || ny >= w.Height {
					continue
				}
				j := ny*w.Width + nx
				if regions[j] == 0 && passable(j) {
					regions[j] = next
					queue = append(queue, j)
				}
			}
		}
	}
	return regions
}
func (w *World) region(p Vec, naval bool) int {
	if !w.inside(p) {
		return 0
	}
	if len(w.landRegions) != len(w.Tiles) {
		w.rebuildRegions()
	}
	if naval {
		return w.waterRegions[int(p.Y)*w.Width+int(p.X)]
	}
	return w.landRegions[int(p.Y)*w.Width+int(p.X)]
}
func (w *World) sameRegion(a, b Vec, naval bool) bool {
	r := w.region(a, naval)
	return r != 0 && r == w.region(b, naval)
}

func (w *World) reachableFootprint(actor *Entity, goal Vec, reach float64) bool {
	naval := definitions[actor.Type].Naval
	r := w.region(actor.Position, naval)
	if r == 0 {
		return false
	}
	for y := max(1, int(goal.Y-reach)); y < min(w.Height-1, int(goal.Y+reach)+1); y++ {
		for x := max(1, int(goal.X-reach)); x < min(w.Width-1, int(goal.X+reach)+1); x++ {
			p := Vec{float64(x) + .5, float64(y) + .5}
			if p.Distance(goal) <= reach && w.region(p, naval) == r {
				return true
			}
		}
	}
	return false
}
func (w *World) validateTerrain(starts []Vec) error {
	for i, home := range starts {
		if !w.land(home) {
			return fmt.Errorf("The generator could not create a safe settlement. Try another seed.")
		}
		for j := 0; j < i; j++ {
			if home.Distance(starts[j]) < 26 {
				return fmt.Errorf("The settlements need more space. Choose a larger world.")
			}
			connected := w.sameRegion(home, starts[j], false)
			if connected == w.islandWorld() {
				return fmt.Errorf("The generator could not connect the intended routes. Try another seed.")
			}
		}
	}
	return nil
}
