package game

import "slices"

// SetPaused is an idempotent intent. Session authorization and control revisions
// live above this boundary; the World remains the sole owner of clock state.
func (w *World) SetPaused(paused bool) error {
	if w.match.State() == MatchFinished {
		return nil
	}
	if paused == (w.match.State() == MatchPaused) {
		return nil
	}
	event := ResumeMatch
	if paused {
		event = PauseMatch
	}
	return fire(w.match, event, &matchContext{World: w})
}

// SetSpeed validates the shared clock independently of a kingdom's defeat.
// The caller records its durable control receipt and audit after saving.
func (w *World) SetSpeed(speed float64) error {
	if w.match.State() == MatchFinished {
		return rule("match_ended", "This match has ended.")
	}
	if !slices.Contains(gameSpeeds, speed) {
		return rule("invalid_speed", "Choose 1×, 1.7×, 3.4×, 8×, 16×, or 32× speed.")
	}
	w.Speed = speed
	return nil
}
