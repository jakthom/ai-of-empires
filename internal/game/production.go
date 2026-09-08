package game

import (
	"context"
	"math"

	"github.com/open-ships/statemachine"
)

type productionRow = statemachine.Transition[ProductionState, ProductionEvent, *entityContext]

var productionMachine *statemachine.Machine[ProductionState, ProductionEvent, *entityContext]

func init() { productionMachine = compileProgram(productionTransitions()) }
func productionTransitions() []productionRow {
	rows := []productionRow{
		{From: ProductionIdle, Event: ProductionPulse, To: ProductionWorking, Guard: hasProduction},
		{From: ProductionIdle, Event: ProductionPulse, To: ProductionIdle},
		{From: ProductionWorking, Event: ProductionPulse, To: ProductionBlocked, Guard: productionBlocked},
		{From: ProductionWorking, Event: ProductionPulse, To: ProductionIdle, Guard: productionComplete, Do: completeProduction},
		{From: ProductionWorking, Event: ProductionPulse, To: ProductionWorking, Do: workProduction},
		{From: ProductionBlocked, Event: ProductionPulse, To: ProductionWorking, Guard: productionUnblocked},
		{From: ProductionBlocked, Event: ProductionPulse, To: ProductionBlocked},
	}
	for _, state := range []ProductionState{ProductionIdle, ProductionWorking, ProductionBlocked} {
		rows = append(rows,
			productionRow{From: state, Event: QueueProduction, To: state, Guard: canQueueProduction, Do: enqueueProduction},
			productionRow{From: state, Event: CancelProduction, To: state, Guard: cancelWaitingTask, Do: refundTask},
			productionRow{From: state, Event: CancelProduction, To: ProductionIdle, Guard: cancelActiveTask, Do: refundTask},
			productionRow{From: state, Event: LoseProduction, To: ProductionIdle, Do: loseProduction},
		)
	}
	return rows
}
func hasProduction(_ context.Context, c *entityContext) error {
	return applicable(len(c.Actor.Tasks) > 0 && c.Actor.life.State() == Active)
}
func canQueueProduction(_ context.Context, c *entityContext) error {
	if c.Task == nil {
		return rule("missing_task", "A production task is required.")
	}
	w, e, t := c.World, c.Actor, c.Task
	p := w.Players[e.Owner]
	if p == nil || e.life.State() != Active {
		return rule("invalid_producer", "Choose a completed producer.")
	}
	if len(e.Tasks) >= 15 {
		return rule("queue_full", "Production queue is full.")
	}
	// Dispatch by task type selects a validation function, never a lifecycle state.
	validators := map[string]func() error{
		"train":    func() error { return w.canTrain(p, e, definitions[t.Product]) },
		"research": func() error { return w.canResearch(p, e, technologies[t.Product]) },
		"age": func() error {
			if e.Type != "town_center" {
				return rule("invalid_producer", "Advance at a Town Center.")
			}
			return w.canAge(p)
		},
	}
	validate, ok := validators[t.Type]
	if !ok {
		return rule("unknown_task", "Unknown production task.")
	}
	return validate()
}
func enqueueProduction(_ context.Context, c *entityContext) error {
	c.World.Players[c.Actor.Owner].Resources.Add(c.Task.Paid.Scale(-1))
	c.Actor.Tasks = append(c.Actor.Tasks, *c.Task)
	c.World.entityEvent(c.Actor, "queued", "Queued "+taskName(*c.Task), 0)
	return nil
}
func cancelWaitingTask(_ context.Context, c *entityContext) error {
	return applicable(c.Index > 0 && c.Index < len(c.Actor.Tasks))
}
func cancelActiveTask(_ context.Context, c *entityContext) error {
	return applicable(c.Index == 0 && len(c.Actor.Tasks) > 0)
}
func refundTask(_ context.Context, c *entityContext) error {
	e := c.Actor
	c.World.entityEvent(e, "cancelled", "Cancelled "+taskName(e.Tasks[c.Index])+"; resources refunded", 0)
	c.World.Players[e.Owner].Resources.Add(e.Tasks[c.Index].Paid)
	e.Tasks = append(e.Tasks[:c.Index], e.Tasks[c.Index+1:]...)
	return nil
}
func loseProduction(_ context.Context, c *entityContext) error {
	for _, task := range c.Actor.Tasks {
		message := "Lost queued " + taskName(task)
		if c.Refund {
			message += "; resources refunded to previous owner"
		}
		c.World.entityEvent(c.Actor, "cancelled", message, 0)
	}
	if c.Refund {
		for _, task := range c.Actor.Tasks {
			c.World.Players[c.SourceOwner].Resources.Add(task.Paid)
		}
	}
	c.Actor.Tasks = nil
	return nil
}
func productionBlocked(_ context.Context, c *entityContext) error {
	e, w := c.Actor, c.World
	if len(e.Tasks) == 0 {
		return applicable(true)
	}
	task := e.Tasks[0]
	if task.Type != "train" {
		return applicable(false)
	}
	n, cap := w.population(e.Owner)
	if n+definitions[task.Product].Population > cap {
		return nil
	}
	if task.Remaining <= 0 {
		_, ok := w.exit(e, &Entity{Type: task.Product, Owner: e.Owner})
		return applicable(!ok)
	}
	return applicable(false)
}
func productionUnblocked(ctx context.Context, c *entityContext) error {
	return applicable(productionBlocked(ctx, c) != nil)
}
func productionComplete(_ context.Context, c *entityContext) error {
	return applicable(len(c.Actor.Tasks) > 0 && c.Actor.Tasks[0].Remaining <= 0)
}
func workProduction(_ context.Context, c *entityContext) error {
	if len(c.Actor.Tasks) > 0 {
		c.Actor.Tasks[0].Remaining = math.Max(0, c.Actor.Tasks[0].Remaining-Step)
	}
	return nil
}
func completeProduction(_ context.Context, c *entityContext) error {
	w, e := c.World, c.Actor
	task := e.Tasks[0]
	effects := map[string]func(*World, *Entity, Task) error{"train": completeTraining, "research": completeResearch, "age": completeAge}
	complete, ok := effects[task.Type]
	if !ok {
		return rule("unknown_task", "Unknown production task.")
	}
	if err := complete(w, e, task); err != nil {
		return err
	}
	e.Tasks = e.Tasks[1:]
	w.entityEvent(e, "completed", "Completed "+taskName(task), 0)
	return nil
}
func completeTraining(w *World, e *Entity, task Task) error {
	pos, ok := w.exit(e, &Entity{Type: task.Product, Owner: e.Owner})
	if !ok {
		return rule("blocked_exit", "Producer exit is blocked.")
	}
	u := w.spawn(task.Product, e.Owner, pos)
	if e.Rally != nil {
		v := *e.Rally
		w.setOrder(u, Order{Kind: "move", Position: &v}, false)
	}
	return nil
}
func completeResearch(w *World, e *Entity, task Task) error {
	before := map[int]float64{}
	for _, u := range w.entities(e.Owner, "") {
		before[u.ID] = w.stats(u).HP
	}
	w.Players[e.Owner].Technologies[task.Product] = true
	for _, u := range w.entities(e.Owner, "") {
		if before[u.ID] > 0 {
			u.HP *= w.stats(u).HP / before[u.ID]
		}
	}
	w.event(e.Owner, technologies[task.Product].Name+" researched.")
	return nil
}
func completeAge(w *World, e *Entity, _ Task) error {
	p := w.Players[e.Owner]
	p.Age++
	w.event(e.Owner, "Your kingdom has reached the "+Ages[p.Age]+".")
	return nil
}
