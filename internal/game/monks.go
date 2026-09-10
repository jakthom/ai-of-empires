package game

import (
	"context"
	"math"
)

type monkGuard = func(context.Context, *unitContext) error
type monkEffect = func(context.Context, *unitContext) error

// Ordered rows make lifecycle decisions explicit. Guards only observe; effects
// execute through Instance.Fire. A tick driver cannot set a destination state.
func monkTransitions() []unitRow {
	rows := []unitRow{}
	approaches := []struct {
		from, to      UnitState
		valid         monkGuard
		rangeToTarget float64
	}{
		{ApproachingHeal, Healing, validHealingTarget, 7},
		{ApproachingConvert, RecoveringFaith, validConversionTarget, 7},
		{ApproachingRelic, CollectingRelic, validRelicTarget, 1},
		{ApproachingDeposit, DepositingRelic, validDepositTarget, 1},
	}
	for _, approach := range approaches {
		rows = append(rows,
			unitRow{From: approach.from, Event: UnitPulse, To: Idle, Guard: invalid(approach.valid), Do: finishMonkOrder},
			unitRow{From: approach.from, Event: UnitPulse, To: approach.to, Guard: withinRange(approach.rangeToTarget), Do: resetMonkWork},
			unitRow{From: approach.from, Event: UnitPulse, To: approach.from, Do: approachMonkTarget(approach.rangeToTarget)},
		)
	}
	rows = append(rows,
		unitRow{From: Healing, Event: UnitPulse, To: Idle, Guard: invalid(validHealingTarget), Do: finishMonkOrder},
		unitRow{From: Healing, Event: UnitPulse, To: ApproachingHeal, Guard: invalid(withinRange(7)), Do: resetMonkWork},
		unitRow{From: Healing, Event: UnitPulse, To: Idle, Guard: healingComplete, Do: finishMonkOrder},
		unitRow{From: Healing, Event: UnitPulse, To: Healing, Do: applyHealing},

		unitRow{From: RecoveringFaith, Event: UnitPulse, To: Idle, Guard: invalid(validConversionTarget), Do: finishMonkOrder},
		unitRow{From: RecoveringFaith, Event: UnitPulse, To: ApproachingConvert, Guard: invalid(withinRange(7)), Do: resetMonkWork},
		unitRow{From: RecoveringFaith, Event: UnitPulse, To: Converting, Guard: faithReady, Do: beginConversion},
		unitRow{From: RecoveringFaith, Event: UnitPulse, To: RecoveringFaith},

		unitRow{From: Converting, Event: UnitPulse, To: Idle, Guard: invalid(validConversionTarget), Do: finishMonkOrder},
		unitRow{From: Converting, Event: UnitPulse, To: ApproachingConvert, Guard: invalid(withinRange(7)), Do: resetMonkWork},
		unitRow{From: Converting, Event: UnitPulse, To: Idle, Guard: conversionSucceeded, Do: convertTarget},
		unitRow{From: Converting, Event: UnitPulse, To: Converting, Do: accumulateConversion},

		unitRow{From: CollectingRelic, Event: UnitPulse, To: Idle, Guard: invalid(validRelicTarget), Do: finishMonkOrder},
		unitRow{From: CollectingRelic, Event: UnitPulse, To: ApproachingRelic, Guard: invalid(withinRange(1))},
		unitRow{From: CollectingRelic, Event: UnitPulse, To: Idle, Do: collectRelic},

		unitRow{From: DepositingRelic, Event: UnitPulse, To: Idle, Guard: invalid(validDepositTarget), Do: finishMonkOrder},
		unitRow{From: DepositingRelic, Event: UnitPulse, To: ApproachingDeposit, Guard: invalid(withinRange(1))},
		unitRow{From: DepositingRelic, Event: UnitPulse, To: Idle, Do: depositRelic},
	)
	return rows
}

func (w *World) tickMonk(e *Entity) { w.behave(e) }

func invalid(guard monkGuard) monkGuard {
	return func(ctx context.Context, c *unitContext) error {
		if guard(ctx, c) != nil {
			return nil
		}
		return rule("guard_declined", "Target remains valid.")
	}
}
func validMonkTarget(c *unitContext) error {
	if c.Target == nil || !c.World.visibleEntity(c.Actor.Owner, c.Target) || c.Target.life.State() != Active {
		return rule("invalid_target", "Target is unavailable.")
	}
	return nil
}
func validHealingTarget(_ context.Context, c *unitContext) error {
	if err := validMonkTarget(c); err != nil {
		return err
	}
	if c.Target.Owner != c.Actor.Owner || definitions[c.Target.Type].Kind != "unit" || c.Target.Type == "monk" {
		return rule("invalid_target", "Monks heal other friendly units.")
	}
	return nil
}
func validConversionTarget(_ context.Context, c *unitContext) error {
	if c.World.treatyInForce() {
		return rule("peace_period", "Conversions are disabled during the initial peace period.")
	}
	if err := validMonkTarget(c); err != nil {
		return err
	}
	t := c.Target
	if t.Owner == 0 || t.Owner == c.Actor.Owner || t.Type == "town_center" || t.Type == "castle" || t.Type == "wonder" {
		return rule("invalid_target", "This target cannot be converted.")
	}
	d := definitions[t.Type]
	if (d.Kind == "building" || d.Class == "siege") && !c.World.Players[c.Actor.Owner].Technologies["redemption"] {
		return rule("technology_required", "Redemption is required.")
	}
	return nil
}
func validRelicTarget(_ context.Context, c *unitContext) error {
	if err := validMonkTarget(c); err != nil {
		return err
	}
	if c.Target.Type != "relic" || c.Actor.Relic {
		return rule("invalid_target", "This relic cannot be collected.")
	}
	return nil
}
func validDepositTarget(_ context.Context, c *unitContext) error {
	if err := validMonkTarget(c); err != nil {
		return err
	}
	if c.Target.Type != "monastery" || c.Target.Owner != c.Actor.Owner || !c.Actor.Relic {
		return rule("invalid_target", "A carried relic and friendly monastery are required.")
	}
	return nil
}
func withinRange(r float64) monkGuard {
	return func(_ context.Context, c *unitContext) error {
		if c.Target == nil || c.Actor.Position.Distance(c.Target.Position) > r+definitions[c.Target.Type].Radius+.05 {
			return rule("out_of_range", "Target is out of range.")
		}
		return nil
	}
}
func healingComplete(_ context.Context, c *unitContext) error {
	if c.Target.HP < c.World.stats(c.Target).HP {
		return rule("healing_incomplete", "Healing continues.")
	}
	return nil
}
func faithReady(_ context.Context, c *unitContext) error {
	if c.Actor.Faith < 100 {
		return rule("faith_recovering", "Faith is recovering.")
	}
	return nil
}
func conversionSucceeded(_ context.Context, c *unitContext) error {
	if c.Actor.Work >= 12 || c.Actor.Work >= 4 && c.Roll < Step*.18 {
		return nil
	}
	return rule("conversion_pending", "Conversion continues.")
}
func resetMonkWork(_ context.Context, c *unitContext) error {
	c.Actor.Work = 0
	c.Actor.Path = nil
	return nil
}
func finishMonkOrder(ctx context.Context, c *unitContext) error {
	c.Actor.Order = Order{Kind: "idle"}
	return resetMonkWork(ctx, c)
}
func approachMonkTarget(r float64) monkEffect {
	return func(_ context.Context, c *unitContext) error {
		c.World.move(c.Actor, c.Target.Position, r+definitions[c.Target.Type].Radius, Step)
		return nil
	}
}
func applyHealing(_ context.Context, c *unitContext) error {
	c.Target.HP = math.Min(c.World.stats(c.Target).HP, c.Target.HP+Step*2.5)
	if c.Target.HP >= c.World.stats(c.Target).HP {
		c.World.entityEvent(c.Actor, "healed", "Finished healing "+definitions[c.Target.Type].Name, 0)
		c.World.entityEvent(c.Target, "healed", "Restored to full health by a monk", 0)
	}
	return nil
}
func beginConversion(ctx context.Context, c *unitContext) error {
	c.World.noteAggression(c.Actor.ID, c.Actor.Owner, c.Target)
	return resetMonkWork(ctx, c)
}
func accumulateConversion(_ context.Context, c *unitContext) error { c.Actor.Work += Step; return nil }
func convertTarget(ctx context.Context, c *unitContext) error {
	w, e, t := c.World, c.Actor, c.Target
	oldOwner := t.Owner
	w.noteAggression(e.ID, e.Owner, t)
	w.record(Event{Kind: "converted", Message: "Converted to the opposing kingdom"}, t, oldOwner)
	healthFraction := t.HP / w.stats(t).HP
	t.Owner = e.Owner
	t.HP = healthFraction * w.stats(t).HP
	w.entityEvent(t, "converted", "Joined a new kingdom by conversion", 0)
	w.record(Event{Kind: "converted", Message: "Converted " + definitions[t.Type].Name, TargetID: t.ID}, e, 0)
	w.setOrder(t, Order{Kind: "idle"}, false)
	mustFire(t.production, LoseProduction, &entityContext{World: w, Actor: t, SourceOwner: oldOwner, Refund: true})
	// Garrison ownership follows its converted container. Unload before orders.
	for _, id := range t.Passengers {
		if passenger := w.Entities[id]; passenger != nil {
			w.record(Event{Kind: "converted", Message: "Garrison converted to the opposing kingdom"}, passenger, oldOwner)
			passenger.Owner = e.Owner
			w.entityEvent(passenger, "converted", "Joined the kingdom with its converted garrison", 0)
		}
	}
	w.recordPopulationPeak(e.Owner)
	e.Faith = 0
	w.event(e.Owner, definitions[t.Type].Name+" converted to your kingdom.")
	return finishMonkOrder(ctx, c)
}
func collectRelic(ctx context.Context, c *unitContext) error {
	c.Actor.Relic = true
	c.World.entityEvent(c.Actor, "relic", "Collected a relic", 0)
	c.World.entityEvent(c.Target, "relic", "Collected by a monk", 0)
	c.World.remove(c.Target.ID)
	return finishMonkOrder(ctx, c)
}
func depositRelic(ctx context.Context, c *unitContext) error {
	c.Target.Amount++
	c.Actor.Relic = false
	c.World.entityEvent(c.Actor, "relic", "Deposited a relic at the monastery", 0)
	c.World.entityEvent(c.Target, "relic", "Received a relic; earning gold", 0)
	c.World.event(c.Actor.Owner, "A relic now brings gold to your monastery.")
	return finishMonkOrder(ctx, c)
}
