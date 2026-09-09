package game

// Gameplay specializations for new countryside deposits. Starting stockpiles,
// home patches, farms, fishing and existing saved deposits keep their amounts.
type BiomeEconomy struct {
	Biome       string    `json:"biome"`
	Deposits    Resources `json:"deposits"`
	Description string    `json:"description"`
}

func biomeEconomies() []BiomeEconomy {
	return []BiomeEconomy{
		{"temperate", Resources{Food: 1.2, Wood: 1.2, Gold: .8, Stone: .8}, "Balanced food and woodland; smaller mineral deposits."},
		{"desert", Resources{Food: .3, Wood: .25, Gold: 2.2, Stone: 1.3}, "Rich gold and stone; scarce food and timber."},
		{"alpine", Resources{Food: .4, Wood: .6, Gold: 1.3, Stone: 2.4}, "Rich stone and gold; limited food and timber."},
		{"tropical", Resources{Food: 1.8, Wood: 2.2, Gold: .5, Stone: .35}, "Abundant food and timber; scarce minerals."},
		{"autumn", Resources{Food: 1.2, Wood: 1.8, Gold: .55, Stone: .8}, "Large wood reserves and good food; limited gold."},
		{"savanna", Resources{Food: 2.2, Wood: .5, Gold: 1.4, Stone: .5}, "Abundant natural food and good gold; scarce timber and stone."},
	}
}
func (w *World) biomeDepositMultiplier(pos Vec, resource string) float64 {
	if w.Generation < 2 {
		return 1
	}
	for _, p := range w.Players {
		if p.Start.Distance(pos) < 15 {
			return 1
		}
	}
	biome := w.tile(pos).Biome
	if biome == "" {
		biome = w.Config.World.Biome
	}
	for _, profile := range biomeEconomies() {
		if profile.Biome == biome {
			return profile.Deposits.Amount(resource)
		}
	}
	return 1
}
