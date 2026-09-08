package game

import (
	"context"
	"math"
	"slices"

	"github.com/open-ships/statemachine"
)

type aiState string
type aiEvent string

const (
	aiDeveloping aiState = "developing"
	aiDefending  aiState = "defending"
	aiRaiding    aiState = "raiding"
	aiRecovering aiState = "recovering"
	aiAssess     aiEvent = "assess"
)

var aiStates = []aiState{aiDeveloping, aiDefending, aiRaiding, aiRecovering}

// The strategy instance is the sole lifecycle owner. This private checkpoint
// data contains objectives and deadlines, never a second copy of its state.
type aiPlan struct {
	TargetID, TargetOwner int
	TargetPosition        Vec
	Goal                  Vec
	Army                  []int
	CommittedPower        float64
	RaidDeadline          float64
	RecoverUntil          float64
	NextRaidAt            float64
	NextWorkerAt          float64
	ScoutID               int
	ScoutGoal             *Vec
	ScoutDeadline         float64
	Surveyed              map[int]float64
}

type aiContext struct {
	World                              *World
	Player                             *Player
	Policy                             aiPolicy
	Previous                           aiState
	Home                               Vec
	Workers, Centers, Army             []*Entity
	RaidArmy                           []*Entity
	Contacts                           []aiContact
	Power, CampaignPower, CampaignRisk float64
	Threat                             *aiContact
	ThreatPower, HomePower             float64
	Opportunity                        *aiOpportunity
	ObjectiveKnown                     bool
}

type aiRow = statemachine.Transition[aiState, aiEvent, *aiContext]

var aiMachine *statemachine.Machine[aiState, aiEvent, *aiContext]

func init() {
	rows := []aiRow{}
	for _, state := range aiStates {
		// Protecting one's own settlement always interrupts an expedition.
		rows = append(rows, aiRow{From: state, Event: aiAssess, To: aiDefending, Guard: aiHomeThreatened, Do: aiDefend})
	}
	rows = append(rows,
		aiRow{From: aiRaiding, Event: aiAssess, To: aiRecovering, Guard: aiCampaignUnprofitable, Do: aiRecover},
		aiRow{From: aiRaiding, Event: aiAssess, To: aiRaiding, Do: aiContinueRaid},
		aiRow{From: aiDefending, Event: aiAssess, To: aiRecovering, Do: aiRecover},
		aiRow{From: aiRecovering, Event: aiAssess, To: aiRecovering, Guard: aiRecoveryPending, Do: aiRecover},
	)
	for _, state := range []aiState{aiDeveloping, aiRecovering} {
		rows = append(rows,
			aiRow{From: state, Event: aiAssess, To: aiRaiding, Guard: aiProfitableRaid, Do: aiBeginRaid},
			aiRow{From: state, Event: aiAssess, To: aiDeveloping, Do: aiDevelop},
		)
	}
	aiMachine = compileProgram(rows)
}

func aiHomeThreatened(_ context.Context, c *aiContext) error {
	return applicable(c.Threat != nil)
}

func aiCampaignUnprofitable(_ context.Context, c *aiContext) error {
	plan := c.Player.AIPlan
	return applicable(!c.World.aiWantsConflict(c.Player, plan.TargetOwner) || !c.ObjectiveKnown || c.World.Time >= plan.RaidDeadline ||
		c.CampaignPower < plan.CommittedPower*.55 || c.CampaignRisk > 1.6)
}

func aiRecoveryPending(_ context.Context, c *aiContext) error {
	return applicable(c.World.Time < c.Player.AIPlan.RecoverUntil)
}

func aiProfitableRaid(_ context.Context, c *aiContext) error {
	return applicable(c.Opportunity != nil && c.Opportunity.Score > 0 &&
		len(c.RaidArmy) >= c.Policy.RaidSize && c.World.Time >= c.Policy.RaidAfter &&
		c.World.Time >= c.Player.AIPlan.RecoverUntil && c.World.Time >= c.Player.AIPlan.NextRaidAt)
}

func (c *aiContext) recordTransition(_ string, from, to, _ string) {
	// Strategy is private to this kingdom. Other players see only the commands
	// and activities of entities within their own vision.
	c.World.event(c.Player.ID, "Strategy changed from "+from+" to "+to+".")
}

func aiDevelop(_ context.Context, c *aiContext) error {
	c.Player.AIPlan.Army = nil
	c.Player.AIPlan.TargetID, c.Player.AIPlan.TargetOwner = 0, 0
	c.World.aiEconomy(c, false)
	c.World.aiScout(c)
	aiRally(c, false)
	return nil
}

func aiBeginRaid(ctx context.Context, c *aiContext) error {
	plan, target := &c.Player.AIPlan, c.Opportunity
	plan.TargetID, plan.TargetOwner, plan.Goal = target.ID, target.Owner, target.Goal
	plan.TargetPosition = target.Position
	plan.CommittedPower = c.Power
	plan.NextRaidAt = c.World.Time + c.Policy.RaidInterval
	plan.RaidDeadline = c.World.Time + math.Max(100, c.Home.Distance(target.Goal)/.8+70)
	plan.Army = nil
	plan.ScoutGoal = nil
	for _, e := range c.RaidArmy {
		plan.Army = append(plan.Army, e.ID)
	}
	aiMarch(c, c.RaidArmy, plan.Goal, true, true)
	return aiContinueRaid(ctx, c)
}

func aiContinueRaid(_ context.Context, c *aiContext) error {
	plan := &c.Player.AIPlan
	campaign := []*Entity{}
	for _, e := range c.Army {
		if slices.Contains(plan.Army, e.ID) {
			campaign = append(campaign, e)
		}
	}
	// A raid has a fixed roster. Fresh production stays home for the next
	// expedition instead of turning a small party into an endless stream.
	aiMarch(c, campaign, plan.Goal, true, false)
	aiRally(c, false)
	c.World.aiEconomy(c, false)
	return nil
}

func aiDefend(_ context.Context, c *aiContext) error {
	plan := &c.Player.AIPlan
	plan.TargetID, plan.TargetOwner = 0, 0
	plan.Army, plan.ScoutGoal = nil, nil
	// A hopeless charge does not help the economy. Withdraw behind the Town
	// Center, while endangered workers move away from the observed attackers.
	goal := c.Threat.Position
	if c.HomePower < c.ThreatPower*.8 {
		goal = aiAway(c.World, c.Home, c.Threat.Position, 5)
		aiMarch(c, c.Army, goal, false, c.Previous != aiDefending)
	} else {
		for _, e := range c.Army {
			aiStance(c, e, "defensive")
			if e.Order.Kind != "attack" || e.Order.Target != c.Threat.ID {
				_ = c.World.Apply(c.Player.ID, Command{Kind: "attack", EntityIDs: []int{e.ID}, TargetID: c.Threat.ID})
			}
		}
	}
	for _, worker := range c.Workers {
		if worker.Container != 0 || worker.Position.Distance(c.Threat.Position) > 11 {
			continue
		}
		safe := aiAway(c.World, c.Home, c.Threat.Position, 6)
		aiMarch(c, []*Entity{worker}, safe, false, false)
	}
	c.World.aiEconomy(c, true)
	return nil
}

func aiRecover(_ context.Context, c *aiContext) error {
	plan := &c.Player.AIPlan
	entering := c.Previous != aiRecovering
	if entering {
		plan.RecoverUntil = c.World.Time + 45
		plan.TargetID, plan.TargetOwner = 0, 0
		plan.Army, plan.ScoutGoal = nil, nil
	}
	aiRally(c, entering)
	c.World.aiEconomy(c, false)
	return nil
}

func aiRally(c *aiContext, force bool) {
	goal := aiAway(c.World, c.Home, Vec{float64(c.World.Width) / 2, float64(c.World.Height) / 2}, 4)
	for _, e := range c.Army {
		if slices.Contains(c.Player.AIPlan.Army, e.ID) {
			continue
		}
		if !force && e.ID == c.Player.AIPlan.ScoutID && c.Player.AIPlan.ScoutGoal != nil {
			continue
		}
		if e.Position.Distance(c.Home) > 9 || force {
			aiMarch(c, []*Entity{e}, goal, false, force)
		} else {
			aiStance(c, e, "defensive")
		}
	}
}

func aiStance(c *aiContext, e *Entity, stance string) {
	if e.Stance != stance {
		_ = c.World.Apply(c.Player.ID, Command{Kind: "stance", EntityIDs: []int{e.ID}, Product: stance})
	}
}

// Preserve active paths and combat windups on ordinary assessment pulses.
// Interruptions (retreat, defense, new campaign) deliberately replace orders.
func aiMarch(c *aiContext, army []*Entity, goal Vec, attack, force bool) {
	kind, stance := "move", "passive"
	if attack {
		kind, stance = "attack_move", "defensive"
	}
	ids := []int{}
	for _, e := range army {
		aiStance(c, e, stance)
		redirect := e.Order.Position != nil && e.Order.Position.Distance(goal) > 4 && (e.Order.Kind == "move" || e.Order.Kind == "attack_move")
		if force || e.behavior.State() == Idle || redirect || !attack && e.Order.Kind != "move" || attack && e.Order.Kind == "move" {
			if !force && e.Position.Distance(goal) <= 1 {
				continue
			}
			ids = append(ids, e.ID)
		}
	}
	if len(ids) > 0 {
		targetPlayer := 0
		if attack {
			targetPlayer = c.Player.AIPlan.TargetOwner
		}
		_ = c.World.Apply(c.Player.ID, Command{Kind: kind, EntityIDs: ids, Position: &goal, TargetPlayer: targetPlayer})
	}
}

func aiAway(w *World, origin, danger Vec, distance float64) Vec {
	dx, dy := origin.X-danger.X, origin.Y-danger.Y
	length := math.Hypot(dx, dy)
	if length < .01 {
		dx, dy, length = 1, 0, 1
	}
	return Vec{math.Max(3, math.Min(float64(w.Width-4), origin.X+dx/length*distance)), math.Max(3, math.Min(float64(w.Height-4), origin.Y+dy/length*distance))}
}
