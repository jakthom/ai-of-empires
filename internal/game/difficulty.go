package game

// Difficulty is display metadata. Private AI policies decide how the opponent
// spends its own resources through the same commands available to the player.
type Difficulty struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type aiPolicy struct {
	Difficulty
	Workers, RaidSize, MilitaryPriorityAt int
	RaidLimit, ArmyLimit, TownCenters     int
	RaidAfter, RaidInterval, WorkerPause  float64
	Upgrades                              bool
	RaidAdvantage                         float64
}

var aiPolicies = []aiPolicy{
	{Difficulty: Difficulty{"peaceful", "Peaceful practice", "Other kingdoms stay idle while you build and explore. Their units can still return fire."}},
	{Difficulty: Difficulty{"easy", "Easy", "Varied kingdom preferences, slower growth, at most 12 villagers and 8 soldiers. Any raids contain at most 4 units, begin after 10 game minutes, and are at least 4 game minutes apart."}, Workers: 12, RaidSize: 3, RaidLimit: 4, ArmyLimit: 8, TownCenters: 1, RaidAfter: 600, RaidInterval: 240, WorkerPause: 15, RaidAdvantage: 1.4},
	{Difficulty: Difficulty{"normal", "Standard", "Varied kingdom preferences and moderate economies. Expansionists can send measured armies; others favor growth or defense."}, Workers: 18, RaidSize: 8, RaidLimit: 12, ArmyLimit: 32, RaidAfter: 240, RaidInterval: 90, MilitaryPriorityAt: 16},
	{Difficulty: Difficulty{"hard", "Hard", "Larger economies with varied preferences. Expansionists can launch earlier, more frequent raids."}, Workers: 22, RaidSize: 6, RaidLimit: 16, ArmyLimit: 40, RaidAfter: 180, RaidInterval: 60, MilitaryPriorityAt: 14},
	{Difficulty: Difficulty{"extra_hard", "Extra hard", "Varied preferences and stronger economies. Expansionists prioritize soldiers after ten villagers and raid with less preparation."}, Workers: 26, RaidSize: 4, RaidLimit: 20, ArmyLimit: 48, RaidAfter: 135, RaidInterval: 45, MilitaryPriorityAt: 10},
	{Difficulty: Difficulty{"expert", "Expert", "Varied preferences, larger economies, and researched upgrades. Expansionists fund armies early; builders continue to favor growth."}, Workers: 30, RaidSize: 3, RaidLimit: 24, ArmyLimit: 60, RaidAfter: 90, RaidInterval: 30, MilitaryPriorityAt: 8, Upgrades: true},
	{Difficulty: Difficulty{"aggressive", "Aggressive", "Every AI kingdom is expansionist: early scouting and frequent, riskier raids against vulnerable settlements."}, Workers: 18, RaidSize: 1, RaidLimit: 8, ArmyLimit: 32, RaidAfter: 60, RaidInterval: 30, MilitaryPriorityAt: 6, RaidAdvantage: .85},
}

func ValidDifficulty(id string) bool {
	for _, policy := range aiPolicies {
		if policy.ID == id {
			return true
		}
	}
	return false
}

func difficultyPolicy(id string) aiPolicy {
	for _, policy := range aiPolicies {
		if policy.ID == id {
			if policy.RaidAdvantage == 0 {
				policy.RaidAdvantage = 1.2
			}
			if policy.TownCenters == 0 {
				policy.TownCenters = 3
			}
			return policy
		}
	}
	return difficultyPolicy("normal") // Also the default for direct domain callers.
}
