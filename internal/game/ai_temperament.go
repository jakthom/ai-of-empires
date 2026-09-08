package game

type aiTemperament string

const (
	aiBuilder      aiTemperament = "builder"
	aiGuarded      aiTemperament = "defensive"
	aiExpansionist aiTemperament = "expansionist"
)

func initialTemperament(cfg Config, player int) aiTemperament {
	if cfg.Difficulty == "aggressive" {
		return aiExpansionist
	}
	if cfg.Difficulty == "peaceful" {
		return aiBuilder
	}
	// Rotate a mixture through the seats without consuming the simulation RNG.
	// The same seed/session retains its preferences across restarts.
	return []aiTemperament{aiBuilder, aiGuarded, aiExpansionist}[(uint64(cfg.Seed)/3+uint64(player+1))%3]
}

func (w *World) aiWantsConflict(p *Player, opponent int) bool {
	if w.treatyInForce() {
		return false
	}
	return p.Temperament == aiExpansionist || p.Temperament == aiGuarded && w.relation(p.ID, opponent) == inConflict
}

func temperamentName(t aiTemperament) string {
	return map[aiTemperament]string{aiBuilder: "Builder", aiGuarded: "Defensive", aiExpansionist: "Expansionist"}[t]
}
