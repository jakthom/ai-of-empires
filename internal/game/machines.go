package game

import (
	"context"
	"fmt"

	"github.com/open-ships/statemachine"
)

// Every lifecycle has one Instance owner. Tables, guards and effects are shared;
// a World serializes all Fire calls and owns the data passed to those effects.
type MatchState string
type MatchEvent string

const (
	MatchRunning  MatchState = "running"
	MatchPaused   MatchState = "paused"
	MatchFinished MatchState = "finished"
	PauseMatch    MatchEvent = "pause"
	ResumeMatch   MatchEvent = "resume"
	MatchPulse    MatchEvent = "pulse"
)

type LifeState string
type LifeEvent string

const (
	Foundation      LifeState = "foundation"
	Active          LifeState = "active"
	Destroyed       LifeState = "destroyed"
	Exhausted       LifeState = "exhausted"
	BuildWork       LifeEvent = "build_work"
	RepairWork      LifeEvent = "repair_work"
	DamageEntity    LifeEvent = "damage"
	DestroyEntity   LifeEvent = "destroy"
	DeleteEntity    LifeEvent = "delete"
	ReseedFarm      LifeEvent = "reseed"
	ExhaustResource LifeEvent = "exhaust"
)

type ProductionState string
type ProductionEvent string

const (
	ProductionIdle    ProductionState = "idle"
	ProductionWorking ProductionState = "working"
	ProductionBlocked ProductionState = "blocked"
	QueueProduction   ProductionEvent = "queue"
	ProductionPulse   ProductionEvent = "pulse"
	CancelProduction  ProductionEvent = "cancel"
	LoseProduction    ProductionEvent = "lose"
)

type SiegeState string
type SiegeEvent string

const (
	SiegePacked    SiegeState = "packed"
	SiegeDeploying SiegeState = "deploying"
	SiegeDeployed  SiegeState = "deployed"
	SiegePacking   SiegeState = "packing"
	Deploy         SiegeEvent = "deploy"
	Pack           SiegeEvent = "pack"
	SiegePulse     SiegeEvent = "pulse"
)

type PlayerState string
type PlayerEvent string

const (
	PlayerCompeting PlayerState = "competing"
	PlayerDefeated  PlayerState = "defeated"
	PlayerPulse     PlayerEvent = "pulse"
	ResignPlayer    PlayerEvent = "resign"
)

type FlightState string
type FlightEvent string

const (
	Flying      FlightState = "flying"
	Impacted    FlightState = "impacted"
	FlightPulse FlightEvent = "pulse"
)

type UnitState string
type UnitEvent string

const (
	Idle               UnitState = "idle"
	Moving             UnitState = "moving"
	SeekingResource    UnitState = "seeking_resource"
	Gathering          UnitState = "gathering"
	Returning          UnitState = "returning"
	Constructing       UnitState = "constructing"
	Repairing          UnitState = "repairing"
	Chasing            UnitState = "chasing"
	Attacking          UnitState = "attacking"
	AttackCooldown     UnitState = "attack_cooldown"
	ApproachingHeal    UnitState = "approaching_heal"
	ApproachingConvert UnitState = "approaching_convert"
	RecoveringFaith    UnitState = "recovering_faith"
	ApproachingRelic   UnitState = "approaching_relic"
	ApproachingDeposit UnitState = "approaching_deposit"
	Healing            UnitState = "healing"
	Converting         UnitState = "converting"
	CollectingRelic    UnitState = "collecting_relic"
	DepositingRelic    UnitState = "depositing_relic"
	Trading            UnitState = "trading_outbound"
	ReturningTrade     UnitState = "trading_returning"
	Caravanning        UnitState = "caravanning"
	Embarking          UnitState = "embarking"
	Garrisoned         UnitState = "garrisoned"
	UnitPulse          UnitEvent = "pulse"
	OrderMove          UnitEvent = "order_move"
	OrderGather        UnitEvent = "order_gather"
	OrderBuild         UnitEvent = "order_build"
	OrderRepair        UnitEvent = "order_repair"
	OrderAttack        UnitEvent = "order_attack"
	OrderHeal          UnitEvent = "order_heal"
	OrderConvert       UnitEvent = "order_convert"
	OrderRelic         UnitEvent = "order_relic"
	OrderDepositRelic  UnitEvent = "order_deposit_relic"
	OrderTrade         UnitEvent = "order_trade"
	OrderCaravan       UnitEvent = "order_caravan"
	OrderEmbark        UnitEvent = "order_embark"
	StopOrder          UnitEvent = "stop_order"
	QueueOrder         UnitEvent = "queue_order"
	Disembark          UnitEvent = "disembark"
)

type unitContext struct {
	World         *World
	Actor         *Entity
	Target        *Entity
	Candidate     *Entity
	DropOff       *Entity
	Order         *Order
	PreserveQueue bool
	Roll          float64
}
type unitRow = statemachine.Transition[UnitState, UnitEvent, *unitContext]

var unitRows []unitRow
var unitMachine *statemachine.Machine[UnitState, UnitEvent, *unitContext]

func init() {
	mobile := func(_ context.Context, c *unitContext) error {
		if definitions[c.Actor.Type].Kind != "unit" {
			return rule("immobile", "This entity cannot move.")
		}
		if c.Actor.Type == "trebuchet" && c.Actor.siege.State() != SiegePacked {
			return rule("deployed", "Pack this trebuchet first.")
		}
		return nil
	}
	worker := func(_ context.Context, c *unitContext) error {
		if definitions[c.Actor.Type].Class != "worker" {
			return rule("not_worker", "This unit cannot gather resources.")
		}
		return nil
	}
	villager := func(_ context.Context, c *unitContext) error {
		if c.Actor.Type != "villager" {
			return rule("not_builder", "This requires a villager.")
		}
		return nil
	}
	monk := func(_ context.Context, c *unitContext) error {
		if c.Actor.Type != "monk" {
			return rule("not_monk", "This requires a monk.")
		}
		return nil
	}
	states := []UnitState{Idle, Moving, SeekingResource, Gathering, Returning, Constructing, Repairing, Chasing, Attacking, AttackCooldown, ApproachingHeal, Healing, ApproachingConvert, RecoveringFaith, Converting, ApproachingRelic, CollectingRelic, ApproachingDeposit, DepositingRelic, Trading, ReturningTrade, Caravanning, Embarking}
	destinations := []unitRow{
		{Event: OrderCaravan, To: Caravanning, Guard: caravanUnit},
		{Event: OrderMove, To: Moving, Guard: mobile},
		{Event: OrderGather, To: SeekingResource, Guard: worker},
		{Event: OrderBuild, To: Constructing, Guard: villager},
		{Event: OrderRepair, To: Repairing, Guard: villager},
		{Event: OrderAttack, To: Chasing, Guard: func(_ context.Context, c *unitContext) error {
			if definitions[c.Actor.Type].Attack <= 0 {
				return rule("cannot_attack", "This entity cannot attack.")
			}
			return nil
		}},
		{Event: OrderHeal, To: ApproachingHeal, Guard: monk},
		{Event: OrderConvert, To: ApproachingConvert, Guard: monk},
		{Event: OrderRelic, To: ApproachingRelic, Guard: monk},
		{Event: OrderDepositRelic, To: ApproachingDeposit, Guard: monk},
		{Event: OrderTrade, To: Trading, Guard: func(_ context.Context, c *unitContext) error {
			if c.Actor.Type != "trade_cart" {
				return rule("not_trader", "This requires a trade cart.")
			}
			return nil
		}},
		{Event: OrderEmbark, To: Embarking, Guard: mobile},
		{Event: StopOrder, To: Idle},
	}
	for _, from := range states {
		for _, row := range destinations {
			row.From = from
			row.Do = acceptOrder
			unitRows = append(unitRows, row)
		}
		unitRows = append(unitRows, unitRow{From: from, Event: QueueOrder, To: from, Guard: queueHasRoom, Do: appendOrder})
	}
	unitRows = append(unitRows, movementTransitions()...)
	unitRows = append(unitRows, unitRow{From: Caravanning, Event: UnitPulse, To: Caravanning})
	unitRows = append(unitRows, economyTransitions()...)
	unitRows = append(unitRows, combatTransitions()...)
	unitRows = append(unitRows, monkTransitions()...)
	unitRows = append(unitRows, transportTransitions()...)
	unitMachine = compileProgram(unitRows)
}
func compileProgram[S, E comparable, T any](rows []statemachine.Transition[S, E, T]) *statemachine.Machine[S, E, T] {
	machine, err := statemachine.Compile(rows)
	if err != nil {
		panic(fmt.Errorf("invalid built-in transition table: %w", err))
	}
	return machine
}
func fire[S, E comparable, T any](instance *statemachine.Instance[S, E, T], event E, data T) error {
	before := instance.State()
	_, err := instance.Fire(context.Background(), event, data)
	if err != nil || (before == instance.State() && any(event) != any(DamageEntity)) {
		return err
	}
	if recorder, ok := any(data).(transitionRecorder); ok && err == nil {
		scope := "activity"
		switch any(before).(type) {
		case LifeState:
			scope = "life"
		case ProductionState:
			scope = "production"
		case SiegeState:
			scope = "siege"
		}
		// Observe only committed transitions; this never chooses a state or
		// fires a lifecycle recursively from its own transition effect.
		recorder.recordTransition(scope, fmt.Sprint(before), fmt.Sprint(instance.State()), fmt.Sprint(event))
	}
	return err
}
func mustFire[S, E comparable, T any](instance *statemachine.Instance[S, E, T], event E, data T) {
	if err := fire(instance, event, data); err != nil {
		panic(fmt.Errorf("simulation transition: %w", err))
	}
}

var orderEvents = map[string]UnitEvent{"move": OrderMove, "attack_move": OrderMove, "gather": OrderGather, "build": OrderBuild, "repair": OrderRepair, "attack": OrderAttack, "heal": OrderHeal, "convert": OrderConvert, "relic": OrderRelic, "deposit_relic": OrderDepositRelic, "trade": OrderTrade, "garrison": OrderEmbark, "idle": StopOrder, "stop": StopOrder}

func orderEvent(kind string) UnitEvent {
	if kind == "caravan" {
		return OrderCaravan
	}
	return orderEvents[kind]
}
func caravanUnit(_ context.Context, c *unitContext) error {
	return applicable(c.Actor.Type == "trade_cart" && c.Actor.Container == 0)
}
