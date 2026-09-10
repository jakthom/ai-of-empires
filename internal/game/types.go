// Package game owns the authoritative simulation. It has no HTTP, rendering,
// filesystem, wall-clock, or goroutine dependencies. A caller serializes access.
package game

import (
	"math"

	"github.com/open-ships/statemachine"
)

const Step = 0.05
const RulesVersion = "frontier-1"

type Vec struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

func (a Vec) Distance(b Vec) float64 { return math.Hypot(a.X-b.X, a.Y-b.Y) }
func (a Vec) Finite() bool {
	return !math.IsNaN(a.X) && !math.IsNaN(a.Y) && !math.IsInf(a.X, 0) && !math.IsInf(a.Y, 0)
}

type Resources struct {
	Food  float64 `json:"food"`
	Wood  float64 `json:"wood"`
	Gold  float64 `json:"gold"`
	Stone float64 `json:"stone"`
}

func (r Resources) CanPay(c Resources) bool {
	return r.Food+1e-8 >= c.Food && r.Wood+1e-8 >= c.Wood && r.Gold+1e-8 >= c.Gold && r.Stone+1e-8 >= c.Stone
}
func (r *Resources) Add(c Resources) {
	r.Food += c.Food
	r.Wood += c.Wood
	r.Gold += c.Gold
	r.Stone += c.Stone
}
func (r Resources) Scale(n float64) Resources {
	return Resources{r.Food * n, r.Wood * n, r.Gold * n, r.Stone * n}
}
func (r *Resources) Deposit(kind string, n float64) {
	switch kind {
	case "food":
		r.Food += n
	case "wood":
		r.Wood += n
	case "gold":
		r.Gold += n
	case "stone":
		r.Stone += n
	}
}

type Definition struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Kind         string    `json:"kind"`
	Description  string    `json:"description"`
	Cost         Resources `json:"cost"`
	Age          int       `json:"age"`
	Time         float64   `json:"time"`
	HP           float64   `json:"hp"`
	Radius       float64   `json:"radius"`
	Speed        float64   `json:"speed"`
	Attack       float64   `json:"attack"`
	Armor        float64   `json:"armor"`
	PierceArmor  float64   `json:"pierce_armor"`
	Range        float64   `json:"range"`
	MinRange     float64   `json:"min_range"`
	Reload       float64   `json:"reload"`
	Sight        float64   `json:"sight"`
	Population   int       `json:"population"`
	Housing      int       `json:"housing"`
	Producer     string    `json:"producer,omitempty"`
	Prerequisite string    `json:"prerequisite,omitempty"`
	Class        string    `json:"class,omitempty"`
	DropOff      []string  `json:"drop_off,omitempty"`
	Naval        bool      `json:"naval"`
	Projectile   bool      `json:"projectile"`
}

type Technology struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	Producer     string    `json:"producer"`
	Age          int       `json:"age"`
	Cost         Resources `json:"cost"`
	Time         float64   `json:"time"`
	Prerequisite string    `json:"prerequisite,omitempty"`
}
type Civilization struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Bonus       string `json:"bonus"`
}
type Catalog struct {
	Commands         []CommandInfo  `json:"commands"`
	RulesVersion     string         `json:"rules_version"`
	Definitions      []Definition   `json:"definitions"`
	Technologies     []Technology   `json:"technologies"`
	Civilizations    []Civilization `json:"civilizations"`
	Ages             []string       `json:"ages"`
	Speeds           []float64      `json:"speeds"`
	Difficulties     []Difficulty   `json:"difficulties"`
	LogFilters       []LogFilter    `json:"log_filters"`
	SettlementCounts []int          `json:"settlement_counts"`
	Worlds           WorldCatalog   `json:"worlds"`
}

type Tile struct {
	Biome     string  `json:"biome,omitempty"`
	Terrain   string  `json:"terrain"`
	Elevation float64 `json:"elevation"`
}
type Order struct {
	Shipment     int    `json:"shipment,omitempty"`
	Kind         string `json:"kind"`
	Target       int    `json:"target,omitempty"`
	Position     *Vec   `json:"position,omitempty"`
	TargetPlayer int    `json:"target_player,omitempty"`
	Initiated    bool   `json:"initiated,omitempty"`
	DefendFrom   *Vec   `json:"defend_from,omitempty"`
}
type Task struct {
	Type      string    `json:"type"`
	Product   string    `json:"product"`
	Remaining float64   `json:"remaining"`
	Duration  float64   `json:"duration"`
	Paid      Resources `json:"paid"`
}

// Entity is private world state. HTTP never serializes it directly.
type Entity struct {
	ID                  int
	Type                string
	Owner               int
	Position            Vec
	HP                  float64
	Progress            float64
	Amount              float64
	Resource            string
	Cargo               float64
	CargoType           string
	Order               Order
	Orders              []Order
	Tasks               []Task
	Rally               *Vec
	Path                []Vec
	PathGoal            Vec
	PathTarget          int
	Repath              float64
	Cooldown            float64
	Work                float64
	Source              int
	behavior            *statemachine.Instance[UnitState, UnitEvent, *unitContext]
	life                *statemachine.Instance[LifeState, LifeEvent, *entityContext]
	production          *statemachine.Instance[ProductionState, ProductionEvent, *entityContext]
	siege               *statemachine.Instance[SiegeState, SiegeEvent, *entityContext]
	DeploymentRemaining float64
	Passengers          []int
	Container           int
	Relic               bool
	Faith               float64
	Stance              string
}
type Player struct {
	UserID       string
	UserAliases  []string
	ID           int
	Start        Vec
	Name         string
	Civilization string
	Resources    Resources
	Production   ResourceProduction
	Age          int
	Technologies map[string]bool
	AI           bool
	Temperament  aiTemperament
	AIPlan       aiPlan
	NavalPlan    navalPlan
	voyage       *statemachine.Instance[voyageState, voyageEvent, *voyageContext]
	strategy     *statemachine.Instance[aiState, aiEvent, *aiContext]
	lifecycle    *statemachine.Instance[PlayerState, PlayerEvent, *playerContext]
	Kills        int
	Gathered     Resources
	Explored     []bool
	Visible      []bool
	Memory       map[int]EntityView
}
type Projectile struct {
	flight      *statemachine.Instance[FlightState, FlightEvent, *flightContext]
	ID          int
	Owner       int
	Position    Vec
	Destination Vec
	Source      int
	Target      int
	Damage      float64
	Splash      float64
	Speed       float64
	Kind        string
}

// ProjectileView exposes only currently observable presentation data, never
// the hidden target, destination, damage, or lifecycle instance.
type ProjectileView struct {
	ID       int    `json:"id"`
	Owner    int    `json:"owner"`
	Position Vec    `json:"position"`
	Kind     string `json:"kind"`
}
type Event struct {
	UserID        string  `json:"user_id"`
	ID            int     `json:"id"`
	Tick          int     `json:"tick"`
	Time          float64 `json:"time"`
	Message       string  `json:"message"`
	Player        int     `json:"player_id"`
	Kind          string  `json:"kind"`
	EntityID      int     `json:"entity_id,omitempty"`
	EntityName    string  `json:"entity_name,omitempty"`
	EntityType    string  `json:"entity_type,omitempty"`
	State         string  `json:"state,omitempty"`
	PreviousState string  `json:"previous_state,omitempty"`
	Lifecycle     string  `json:"lifecycle,omitempty"`
	CommandID     string  `json:"command_id,omitempty"`
	TargetID      int     `json:"target_id,omitempty"`
	Activity      string  `json:"activity,omitempty"`
	Resource      string  `json:"resource,omitempty"`
	Amount        float64 `json:"amount,omitempty"`
	Position      Vec     `json:"position"`
}
type Config struct {
	Name         string       `json:"name,omitempty"`
	Settlements  int          `json:"settlements,omitempty"`
	Seed         int64        `json:"seed"`
	Civilization string       `json:"civilization"`
	Difficulty   string       `json:"difficulty"`
	Mode         string       `json:"mode"`
	World        WorldOptions `json:"world,omitempty"`
}
type World struct {
	Marketplace  marketplace
	Config       Config
	Tick         int
	Time         float64
	Width        int
	Height       int
	Tiles        []Tile
	Entities     map[int]*Entity
	IDs          []int
	Players      map[int]*Player
	Relations    map[string]*relationship
	Incidents    map[int]map[int]aggression
	Projectiles  []Projectile
	journal      eventJournal
	NextID       int
	NextEvent    int
	Speed        float64
	match        *statemachine.Instance[MatchState, MatchEvent, *matchContext]
	Winner       int
	aiClock      float64
	visibleClock float64
	rng          uint64
	Generation   int
	peacePeriod  *statemachine.Instance[treatyState, treatyEvent, *World]
	landRegions  []int
	waterRegions []int
	viewMaps     map[int]MapView
	barriers     map[Vec]*Entity
}

// Command contains intent only; never caller-supplied HP, costs, velocities or owners.
type Command struct {
	TradeMode      string            `json:"trade_mode,omitempty"`
	TradeLimit     *int              `json:"trade_limit,omitempty"`
	Offer          *TradeOfferIntent `json:"offer,omitempty"`
	OfferID        int               `json:"offer_id,omitempty"`
	ShipmentID     int               `json:"shipment_id,omitempty"`
	Repeat         bool              `json:"repeat,omitempty"`
	MarketRevision *int              `json:"market_revision,omitempty"`
	EndPosition    *Vec              `json:"end_position,omitempty"`
	ID             string            `json:"id"`
	Kind           string            `json:"kind"`
	EntityIDs      []int             `json:"entity_ids,omitempty"`
	TargetID       int               `json:"target_id,omitempty"`
	TargetPlayer   int               `json:"target_player,omitempty"`
	Position       *Vec              `json:"position,omitempty"`
	Product        string            `json:"product,omitempty"`
	Queue          bool              `json:"queue,omitempty"`
	Value          float64           `json:"value,omitempty"`
}
type RuleError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *RuleError) Error() string { return e.Message }
func rule(code, msg string) error  { return &RuleError{code, msg} }

type Action struct {
	Kind        string     `json:"kind"`
	Product     string     `json:"product,omitempty"`
	Label       string     `json:"label"`
	Description string     `json:"description"`
	Cost        Resources  `json:"cost"`
	Gain        *Resources `json:"gain,omitempty"`
	Duration    float64    `json:"duration"`
	Enabled     bool       `json:"enabled"`
	Reason      string     `json:"reason,omitempty"`
}
type EntityView struct {
	Connections   []Vec    `json:"connections,omitempty"`
	AppearanceAge int      `json:"appearance_age,omitempty"`
	ID            int      `json:"id"`
	Type          string   `json:"type"`
	Kind          string   `json:"kind"`
	Name          string   `json:"name"`
	Owner         int      `json:"owner"`
	Position      Vec      `json:"position"`
	HP            float64  `json:"hp"`
	MaxHP         float64  `json:"max_hp"`
	Radius        float64  `json:"radius"`
	Progress      float64  `json:"progress"`
	Amount        float64  `json:"amount,omitempty"`
	Resource      string   `json:"resource,omitempty"`
	Cargo         float64  `json:"cargo,omitempty"`
	CargoType     string   `json:"cargo_type,omitempty"`
	State         string   `json:"state"`
	Stance        string   `json:"stance,omitempty"`
	Activity      string   `json:"activity"`
	Visible       bool     `json:"visible"`
	Actions       []Action `json:"actions"`
	Tasks         []Task   `json:"tasks"`
	Rally         *Vec     `json:"rally,omitempty"`
	Deployed      bool     `json:"deployed"`
	Passengers    []int    `json:"passengers"`
	Container     int      `json:"container,omitempty"`
	Relic         bool     `json:"relic"`
	Faith         float64  `json:"faith,omitempty"`
}
type PlayerView struct {
	ID           int            `json:"id"`
	Name         string         `json:"name"`
	Civilization string         `json:"civilization"`
	Resources    Resources      `json:"resources"`
	Production   ProductionView `json:"production"`
	Age          int            `json:"age"`
	AgeName      string         `json:"age_name"`
	Population   int            `json:"population"`
	Capacity     int            `json:"capacity"`
	Limit        int            `json:"limit"`
	Idle         int            `json:"idle"`
	Workers      int            `json:"workers"`
	Military     int            `json:"military"`
	Technologies []string       `json:"technologies"`
	Defeated     bool           `json:"defeated"`
	Kills        int            `json:"kills"`
}
type OpponentView struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Civilization string `json:"civilization"`
	Defeated     bool   `json:"defeated"`
	Relation     string `json:"relation"`
	Temperament  string `json:"temperament"`
}
type MapView struct {
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Biome  string `json:"biome"`
	Tiles  []Tile `json:"tiles"`
	Fog    []int  `json:"fog"`
}
type Snapshot struct {
	Marketplace MarketplaceView `json:"marketplace"`
	// Filled by the session boundary for shared control concurrency.
	ControlRevision int              `json:"control_revision,omitempty"`
	Version         string           `json:"version"`
	Difficulty      Difficulty       `json:"difficulty"`
	Settlements     int              `json:"settlements"`
	World           WorldOptions     `json:"world"`
	TreatyRemaining float64          `json:"treaty_remaining"`
	Tick            int              `json:"tick"`
	Time            float64          `json:"time"`
	Speed           float64          `json:"speed"`
	Paused          bool             `json:"paused"`
	Status          string           `json:"status"`
	Winner          int              `json:"winner"`
	Player          PlayerView       `json:"player"`
	Opponents       []OpponentView   `json:"opponents"`
	Map             MapView          `json:"map"`
	Entities        []EntityView     `json:"entities"`
	Projectiles     []ProjectileView `json:"projectiles"`
	Events          []Event          `json:"events"`
	EventCursor     int              `json:"event_cursor"`
	BuildOptions    []Action         `json:"build_options"`
}
