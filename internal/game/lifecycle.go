package game

import (
	"context"
	"math"
	"slices"

	"github.com/open-ships/statemachine"
)

type entityContext struct {
	Orientation string
	World       *World
	Actor       *Entity
	Amount      float64
	SourceOwner int
	SourceID    int
	Task        *Task
	Index       int
	Refund      bool
}
type lifeRow = statemachine.Transition[LifeState, LifeEvent, *entityContext]

var lifeMachine *statemachine.Machine[LifeState, LifeEvent, *entityContext]

func init() { lifeMachine = compileProgram(lifeTransitions()) }
func lifeTransitions() []lifeRow {
	rows := []lifeRow{
		{From: Foundation, Event: BuildWork, To: Active, Guard: constructionReady, Do: completeBuilding},
		{From: Foundation, Event: BuildWork, To: Foundation, Do: addConstruction},
		{From: Active, Event: RepairWork, To: Active, Guard: canRepair, Do: repairEntity},
		{From: Exhausted, Event: ReseedFarm, To: Foundation, Guard: canReseedEntity, Do: reseedEntity},
		{From: Active, Event: ExhaustResource, To: Exhausted, Guard: depletedFarm},
		{From: Active, Event: ExhaustResource, To: Destroyed, Guard: depletedDeposit, Do: destroyEntity},
	}
	for _, state := range []LifeState{Foundation, Active, Exhausted} {
		rows = append(rows,
			lifeRow{From: state, Event: RotateGate, To: state, Guard: canRotateGate, Do: rotateGate},
			lifeRow{From: state, Event: DamageEntity, To: Destroyed, Guard: lethalDamage, Do: destroyFromDamage},
			lifeRow{From: state, Event: DamageEntity, To: state, Guard: positiveDamage, Do: applyDamage},
			lifeRow{From: state, Event: DestroyEntity, To: Destroyed, Do: destroyEntity},
			lifeRow{From: state, Event: DeleteEntity, To: Destroyed, Do: deleteEntity},
		)
	}
	return rows
}
func depletedFarm(_ context.Context, c *entityContext) error {
	return applicable(c.Actor.Type == "farm" && c.Actor.Amount <= 1e-8)
}
func depletedDeposit(_ context.Context, c *entityContext) error {
	return applicable(definitions[c.Actor.Type].Kind == "resource" && c.Actor.Resource != "" && c.Actor.Amount <= 1e-8)
}
func constructionReady(_ context.Context, c *entityContext) error {
	return applicable(c.Amount > 0 && c.Actor.Progress+c.Amount >= 1-1e-9)
}
func addConstruction(_ context.Context, c *entityContext) error {
	if c.Amount <= 0 {
		return rule("invalid_work", "Construction work must be positive.")
	}
	e := c.Actor
	d := c.World.stats(e)
	amount := math.Min(c.Amount, 1-e.Progress)
	e.Progress += amount
	e.HP = math.Min(d.HP, e.HP+amount*d.HP)
	return nil
}
func completeBuilding(ctx context.Context, c *entityContext) error {
	if err := addConstruction(ctx, c); err != nil {
		return err
	}
	c.Actor.Progress = 1
	c.World.completeBridge(c.Actor)
	if p := c.World.Players[c.Actor.Owner]; p != nil {
		p.Economy.BuildingsCompleted++
		p.Economy.Construction.Add(definitions[c.Actor.Type].Cost)
	}
	c.World.event(c.Actor.Owner, definitions[c.Actor.Type].Name+" completed.")
	return nil
}
func repairCost(c *entityContext) Resources {
	d := c.World.stats(c.Actor)
	amount := math.Min(c.Amount, d.HP-c.Actor.HP)
	return d.Cost.Scale(amount / d.HP * .5)
}
func canRepair(_ context.Context, c *entityContext) error {
	if c.Amount <= 0 || c.Actor.Owner == 0 {
		return rule("invalid_repair", "A friendly entity and positive work are required.")
	}
	if !c.World.Players[c.Actor.Owner].Resources.CanPay(repairCost(c)) {
		return rule("insufficient_resources", "Repair is waiting for resources.")
	}
	return nil
}
func repairEntity(_ context.Context, c *entityContext) error {
	c.World.consume(c.Actor.Owner, repairCost(c), "repairs")
	c.Actor.HP = math.Min(c.World.stats(c.Actor).HP, c.Actor.HP+c.Amount)
	return nil
}
func canReseedEntity(_ context.Context, c *entityContext) error {
	return applicable(c.Actor.Type == "farm" && c.Actor.Amount <= 0 && c.Actor.Owner > 0 && c.World.Players[c.Actor.Owner].Resources.Wood >= 60)
}
func reseedEntity(_ context.Context, c *entityContext) error {
	p := c.World.Players[c.Actor.Owner]
	c.World.consume(p.ID, Resources{Wood: 60}, "farm_reseeding")
	c.Actor.Progress = 0
	c.Actor.HP = 1
	c.Actor.Amount = 175
	if p.Technologies["horse_collar"] {
		c.Actor.Amount += 75
	}
	return nil
}
func positiveDamage(_ context.Context, c *entityContext) error {
	return applicable(c.Amount > 0 && !math.IsNaN(c.Amount) && !math.IsInf(c.Amount, 0))
}
func lethalDamage(ctx context.Context, c *entityContext) error {
	if err := positiveDamage(ctx, c); err != nil {
		return err
	}
	return applicable(c.Amount >= c.Actor.HP)
}
func applyDamage(_ context.Context, c *entityContext) error {
	c.World.accountDamage(c)
	c.Actor.HP -= c.Amount
	return nil
}
func destroyFromDamage(ctx context.Context, c *entityContext) error {
	c.World.accountDamage(c)
	c.Actor.HP = 0
	c.World.captureSpoils(c)
	c.World.leaveAftermath(c.Actor)
	if p := c.World.Players[c.Actor.Owner]; p != nil {
		if definitions[c.Actor.Type].Kind == "building" {
			p.Economy.BuildingsLost++
		} else if definitions[c.Actor.Type].Kind == "unit" {
			p.Economy.UnitsLost++
		}
	}
	if p := c.World.Players[c.SourceOwner]; p != nil && c.Actor.Owner != c.SourceOwner {
		p.Kills++
	}
	if c.Actor.Owner > 0 && definitions[c.Actor.Type].Kind == "building" {
		c.World.event(c.Actor.Owner, definitions[c.Actor.Type].Name+" has been destroyed.")
	}
	return destroyEntity(ctx, c)
}
func deleteEntity(ctx context.Context, c *entityContext) error {
	if c.Actor.life.State() == Foundation {
		c.World.refund(c.Actor.Owner, definitions[c.Actor.Type].Cost.Scale(.75*(1-c.Actor.Progress)))
	}
	return destroyEntity(ctx, c)
}
func destroyEntity(_ context.Context, c *entityContext) error {
	c.World.collapseBridge(c)
	c.World.cleanupEntity(c.Actor)
	return nil
}

var siegeMachine = statemachine.MustCompile([]statemachine.Transition[SiegeState, SiegeEvent, *entityContext]{
	{From: SiegePacked, Event: Deploy, To: SiegeDeploying, Guard: isTrebuchet, Do: beginDeployment},
	{From: SiegeDeployed, Event: Pack, To: SiegePacking, Guard: isTrebuchet, Do: beginDeployment},
	{From: SiegeDeploying, Event: SiegePulse, To: SiegeDeployed, Guard: deploymentReady, Do: finishDeployment},
	{From: SiegeDeploying, Event: SiegePulse, To: SiegeDeploying, Do: progressDeployment},
	{From: SiegePacking, Event: SiegePulse, To: SiegePacked, Guard: deploymentReady, Do: finishDeployment},
	{From: SiegePacking, Event: SiegePulse, To: SiegePacking, Do: progressDeployment},
	{From: SiegePacked, Event: SiegePulse, To: SiegePacked},
	{From: SiegeDeployed, Event: SiegePulse, To: SiegeDeployed},
})

func isTrebuchet(_ context.Context, c *entityContext) error {
	return applicable(c.Actor.Type == "trebuchet" && c.Actor.life.State() == Active && c.Actor.Container == 0)
}
func beginDeployment(_ context.Context, c *entityContext) error {
	c.Actor.DeploymentRemaining = 3
	c.World.setOrder(c.Actor, Order{Kind: "idle"}, false)
	return nil
}
func deploymentReady(_ context.Context, c *entityContext) error {
	return applicable(c.Actor.DeploymentRemaining <= Step)
}
func progressDeployment(_ context.Context, c *entityContext) error {
	c.Actor.DeploymentRemaining -= Step
	return nil
}
func finishDeployment(_ context.Context, c *entityContext) error {
	c.Actor.DeploymentRemaining = 0
	return nil
}

type playerContext struct {
	World  *World
	Player *Player
}

var playerMachine = statemachine.MustCompile([]statemachine.Transition[PlayerState, PlayerEvent, *playerContext]{
	{From: PlayerCompeting, Event: ResignPlayer, To: PlayerDefeated, Do: announceDefeat},
	{From: PlayerCompeting, Event: PlayerPulse, To: PlayerDefeated, Guard: playerEliminated, Do: announceDefeat},
	{From: PlayerCompeting, Event: PlayerPulse, To: PlayerCompeting},
	{From: PlayerDefeated, Event: PlayerPulse, To: PlayerDefeated},
})

func playerEliminated(_ context.Context, c *playerContext) error {
	for _, e := range c.World.entities(c.Player.ID, "") {
		d := definitions[e.Type]
		if d.Kind == "unit" || d.Kind == "building" && !slices.Contains([]string{"wall", "gate", "palisade", "farm", "house"}, e.Type) {
			return applicable(false)
		}
	}
	return nil
}
func announceDefeat(_ context.Context, c *playerContext) error {
	c.World.event(0, c.Player.Name+" has been defeated.")
	return nil
}

type matchContext struct {
	Reason  string
	World   *World
	Winner  int
	Victory bool
}

var matchMachine = statemachine.MustCompile([]statemachine.Transition[MatchState, MatchEvent, *matchContext]{
	{From: MatchRunning, Event: PauseMatch, To: MatchPaused},
	{From: MatchPaused, Event: ResumeMatch, To: MatchRunning},
	{From: MatchRunning, Event: MatchPulse, To: MatchFinished, Guard: matchDecided, Do: completeMatch},
	{From: MatchRunning, Event: MatchPulse, To: MatchRunning},
	{From: MatchPaused, Event: MatchPulse, To: MatchFinished, Guard: matchDecided, Do: completeMatch},
	{From: MatchPaused, Event: MatchPulse, To: MatchPaused},
})

func matchDecided(_ context.Context, c *matchContext) error { return applicable(c.Victory) }
func completeMatch(_ context.Context, c *matchContext) error {
	c.World.Winner = c.Winner
	c.World.VictoryReason = c.Reason
	c.World.event(0, "The battle is over.")
	return nil
}
func (w *World) checkVictory() {
	if w.match.State() == MatchFinished {
		return
	}
	remaining, survivor := 0, 0
	for id := 1; id <= w.Config.Settlements; id++ {
		mustFire(w.Players[id].lifecycle, PlayerPulse, &playerContext{World: w, Player: w.Players[id]})
		if w.Players[id].lifecycle.State() != PlayerDefeated {
			remaining++
			survivor = id
		}
	}
	c := &matchContext{World: w}
	if remaining == 0 || remaining == 1 && w.Config.Settlements > 1 {
		c.Victory, c.Winner = true, survivor
		c.Reason = "conquest"
		if survivor == 0 {
			c.Reason = "no_survivors"
		}
	}
	if !c.Victory {
		for _, id := range w.IDs {
			e := w.Entities[id]
			if e != nil && e.Type == "wonder" && e.life.State() == Active && e.Work >= 600 {
				c.Victory = true
				c.Winner = e.Owner
				c.Reason = "wonder"
				break
			}
		}
	}
	mustFire(w.match, MatchPulse, c)
}
