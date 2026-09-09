package game

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"

	"github.com/open-ships/statemachine"
)

// Checkpoints are a private, versioned persistence format, never an API read
// model. Instances are restored in place without replaying transition effects.
type entityStates struct {
	Behavior   UnitState
	Life       LifeState
	Production ProductionState
	Siege      SiegeState
}
type checkpoint struct {
	Version               int
	Rules                 string
	World                 *World
	Entities              map[int]entityStates
	Players               map[int]PlayerState
	Strategies            map[int]aiState
	Voyages               map[int]voyageState
	Relations             map[string]relationState
	Flights               []FlightState
	Match                 MatchState
	Treaty                treatyState
	AIClock, VisibleClock float64
	RNG                   uint64
}

// JournalRecord preserves the original audience as well as the immutable event.
// It is stored separately so autosaves append history instead of rewriting it.
type JournalRecord struct {
	Event   Event
	Player  int
	Readers uint64
}

func (w *World) JournalSince(after int) []JournalRecord {
	start := sort.Search(len(w.journal.records), func(i int) bool { return w.journal.records[i].ID > after })
	records := make([]JournalRecord, 0, len(w.journal.records)-start)
	for i := start; i < len(w.journal.records); i++ {
		e := w.journal.records[i]
		records = append(records, JournalRecord{e, e.Player, w.journal.readers[i]})
	}
	return records
}

func (w *World) checkpointState() checkpoint {
	c := checkpoint{Version: 4, Rules: RulesVersion, World: w, Treaty: w.peacePeriod.State(), Entities: map[int]entityStates{}, Players: map[int]PlayerState{}, Strategies: map[int]aiState{}, Voyages: map[int]voyageState{}, Relations: map[string]relationState{}, Match: w.match.State(), AIClock: w.aiClock, VisibleClock: w.visibleClock, RNG: w.rng}
	for id, e := range w.Entities {
		c.Entities[id] = entityStates{e.behavior.State(), e.life.State(), e.production.State(), e.siege.State()}
	}
	for id, p := range w.Players {
		c.Players[id] = p.lifecycle.State()
		c.Strategies[id] = p.strategy.State()
		c.Voyages[id] = p.voyage.State()
	}
	for _, p := range w.Projectiles {
		c.Flights = append(c.Flights, p.flight.State())
	}
	for key, relation := range w.Relations {
		c.Relations[key] = relation.lifecycle.State()
	}
	return c
}

func (w *World) Checkpoint() ([]byte, error) { return json.Marshal(w.checkpointState()) }

// CheckpointData is a detached persistence value, never a running simulation.
// Capture while holding the game lock; Encode can run concurrently with play.
type CheckpointData struct{ state checkpoint }

func (w *World) CaptureCheckpoint() CheckpointData {
	c := w.checkpointState()
	c.World = w.freezeCheckpointWorld()
	return CheckpointData{state: c}
}
func (c CheckpointData) Encode() ([]byte, error) { return json.Marshal(c.state) }

func Restore(data []byte, journal []JournalRecord) (*World, error) {
	return RestoreForUsers(data, journal, nil)
}

// RestoreUser separates the current controller from proven historical
// ownership. LegacyID stays empty when an older seat may have changed hands.
type RestoreUser struct{ ID, LegacyID string }

// Users supplies authenticated ownership when upgrading checkpoints that
// predate event identities. Existing identities are never overwritten.
func RestoreForUsers(data []byte, journal []JournalRecord, users map[int]RestoreUser) (*World, error) {
	var c checkpoint
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("decode checkpoint: %w", err)
	}
	if (c.Version < 1 || c.Version > 4) || c.Rules != RulesVersion {
		return nil, fmt.Errorf("unsupported checkpoint version %d / %q", c.Version, c.Rules)
	}
	w := c.World
	if w == nil || w.Width < 3 || w.Height < 3 || len(w.Tiles) != w.Width*w.Height || len(w.Players) != w.Config.Settlements || w.Config.Settlements < 1 || w.Config.Settlements > 6 || len(c.Entities) != len(w.Entities) || len(c.Players) != len(w.Players) || len(c.Flights) != len(w.Projectiles) || !slices.Contains([]MatchState{MatchRunning, MatchPaused, MatchFinished}, c.Match) {
		return nil, fmt.Errorf("invalid checkpoint structure")
	}
	if w.Generation > 0 {
		options, err := normalizeWorldOptions(w.Config.World)
		if err != nil || options != w.Config.World || w.Width != worldSize(options) || w.Height != w.Width {
			return nil, fmt.Errorf("invalid checkpoint world options")
		}
	}
	treaty := treatyExpired
	if c.Version >= 4 {
		treaty = c.Treaty
		if !slices.Contains([]treatyState{treatyActive, treatyExpired}, treaty) {
			return nil, fmt.Errorf("invalid checkpoint treaty")
		}
	}
	w.peacePeriod = statemachine.NewInstance(treatyMachine, treaty)
	w.rebuildRegions()
	w.match = statemachine.NewInstance(matchMachine, c.Match)
	w.aiClock, w.visibleClock, w.rng = c.AIClock, c.VisibleClock, c.RNG
	for id, e := range w.Entities {
		s := c.Entities[id]
		if e == nil || e.ID != id || definitions[e.Type].ID == "" || !e.Position.Finite() || !slices.Contains([]UnitState{Idle, Moving, SeekingResource, Gathering, Returning, Constructing, Repairing, Chasing, Attacking, AttackCooldown, ApproachingHeal, ApproachingConvert, RecoveringFaith, ApproachingRelic, ApproachingDeposit, Healing, Converting, CollectingRelic, DepositingRelic, Trading, ReturningTrade, Embarking, Garrisoned}, s.Behavior) || !slices.Contains([]LifeState{Foundation, Active, Destroyed, Exhausted}, s.Life) || !slices.Contains([]ProductionState{ProductionIdle, ProductionWorking, ProductionBlocked}, s.Production) || !slices.Contains([]SiegeState{SiegePacked, SiegeDeploying, SiegeDeployed, SiegePacking}, s.Siege) {
			return nil, fmt.Errorf("invalid checkpoint entity %d", id)
		}
		e.behavior = statemachine.NewInstance(unitMachine, s.Behavior)
		e.life = statemachine.NewInstance(lifeMachine, s.Life)
		e.production = statemachine.NewInstance(productionMachine, s.Production)
		e.siege = statemachine.NewInstance(siegeMachine, s.Siege)
		if c.Version < 3 && e.Stance == "aggressive" {
			e.Stance = "defensive"
		}
	}
	for id := 1; id <= w.Config.Settlements; id++ {
		p, state := w.Players[id], c.Players[id]
		if p == nil || p.ID != id || len(p.Explored) != len(w.Tiles) || len(p.Visible) != len(w.Tiles) || !slices.Contains([]PlayerState{PlayerCompeting, PlayerDefeated}, state) {
			return nil, fmt.Errorf("invalid checkpoint player %d", id)
		}
		p.lifecycle = statemachine.NewInstance(playerMachine, state)
		if p.UserID == "" {
			p.UserID = users[id].ID
			if p.UserID == "" {
				p.UserID = fmt.Sprintf("kingdom:%d", id)
			}
		}
		strategy := aiDeveloping
		if c.Version >= 2 {
			strategy = c.Strategies[id]
			if !slices.Contains(aiStates, strategy) || !p.AIPlan.Goal.Finite() || !p.AIPlan.TargetPosition.Finite() || p.AIPlan.ScoutGoal != nil && !p.AIPlan.ScoutGoal.Finite() {
				return nil, fmt.Errorf("invalid checkpoint strategy %d", id)
			}
		} else {
			// Saves made before independent strategies keep all entity orders,
			// resources and history. Planning starts on the next AI assessment.
			p.AIPlan = aiPlan{}
		}
		p.strategy = statemachine.NewInstance(aiMachine, strategy)
		voyage := voyageIdle
		if c.Version >= 4 {
			voyage = c.Voyages[id]
			if !slices.Contains(voyageStates, voyage) || !p.NavalPlan.HomeWater.Finite() || !p.NavalPlan.GoalWater.Finite() || !p.NavalPlan.GoalLand.Finite() {
				return nil, fmt.Errorf("invalid checkpoint voyage %d", id)
			}
		}
		p.voyage = statemachine.NewInstance(voyageMachine, voyage)
		if c.Version < 3 {
			p.Temperament = initialTemperament(w.Config, id)
			if p.AI {
				// Older saves have no distinction between unsolicited aggression
				// and retaliation. Reassess peacefully without replaying effects.
				p.AIPlan = aiPlan{NextRaidAt: w.Time + 45}
				p.strategy = statemachine.NewInstance(aiMachine, aiDeveloping)
				for _, e := range w.entities(id, "") {
					if e.Order.Kind == "attack" || e.Order.Kind == "attack_move" {
						e.Order, e.Orders, e.Path, e.Work = Order{Kind: "idle"}, nil, nil, 0
						e.behavior = statemachine.NewInstance(unitMachine, Idle)
					}
				}
			}
		} else if !slices.Contains([]aiTemperament{aiBuilder, aiGuarded, aiExpansionist}, p.Temperament) {
			return nil, fmt.Errorf("invalid checkpoint temperament %d", id)
		}
	}
	if c.Version < 3 {
		w.initializeRelations()
	} else {
		count := w.Config.Settlements * (w.Config.Settlements - 1) / 2
		if len(w.Relations) != count || len(c.Relations) != count {
			return nil, fmt.Errorf("invalid checkpoint relationships")
		}
		for a := 1; a <= w.Config.Settlements; a++ {
			for b := a + 1; b <= w.Config.Settlements; b++ {
				key := relationKey(a, b)
				r, state := w.Relations[key], c.Relations[key]
				if r == nil || r.A != a || r.B != b || !slices.Contains([]relationState{atPeace, inConflict}, state) {
					return nil, fmt.Errorf("invalid checkpoint relationship %s", key)
				}
				r.lifecycle = statemachine.NewInstance(relationMachine, state)
			}
		}
		if w.Incidents == nil {
			w.Incidents = map[int]map[int]aggression{}
		}
	}
	for i := range w.Projectiles {
		if !slices.Contains([]FlightState{Flying, Impacted}, c.Flights[i]) {
			return nil, fmt.Errorf("invalid projectile state")
		}
		w.Projectiles[i].flight = statemachine.NewInstance(flightMachine, c.Flights[i])
	}
	previous := 0
	for _, record := range journal {
		if record.Event.ID != previous+1 || record.Event.ID > w.NextEvent {
			return nil, fmt.Errorf("invalid journal sequence")
		}
		previous = record.Event.ID
		record.Event.Player = record.Player
		if record.Event.UserID == "" {
			record.Event.UserID = "legacy:unattributed"
			if user := users[record.Player].LegacyID; user != "" {
				record.Event.UserID = user
			} else if p := w.Players[record.Player]; p != nil && users == nil {
				// Legacy single-player credentials always controlled player one.
				// AI records remain indexed only for their own kingdom.
				record.Event.UserID = p.UserID
			}
		}
		w.journal.append(record.Event, nil)
		w.journal.readers[len(w.journal.readers)-1] = record.Readers
	}
	if previous != w.NextEvent {
		return nil, fmt.Errorf("checkpoint journal is incomplete")
	}
	w.indexPrivateJournal()
	return w, nil
}

func (w *World) Status() string { return string(w.match.State()) }
