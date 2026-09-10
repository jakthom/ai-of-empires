package game

import (
	"fmt"
	"sort"
	"strings"
)

// Journal records contain values only. The backing slices are private, append
// only, and never shared with readers. Visibility is captured when an event
// happens, so later exploration or conversion cannot reveal hidden history.
type eventJournal struct {
	records  []Event
	readers  []uint64
	players  map[int][]int
	entities map[int]map[int][]int
	searches []*journalSearch
}

type LogQuery struct {
	After, Before, Limit, EntityID int
	FromStart                      bool
	Search, Category               string
}

type HistoryEntity struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Activity string `json:"activity"`
	Present  bool   `json:"present"`
}

type EventPage struct {
	Events       []Event        `json:"events"`
	OlderCursor  int            `json:"older_cursor"`
	NewerCursor  int            `json:"newer_cursor"`
	LatestCursor int            `json:"latest_cursor"`
	HasOlder     bool           `json:"has_older"`
	HasNewer     bool           `json:"has_newer"`
	Entity       *HistoryEntity `json:"entity,omitempty"`
}

func (j *eventJournal) append(event Event, readers []int) {
	if j.players == nil {
		j.players = map[int][]int{}
		j.entities = map[int]map[int][]int{}
	}
	index := len(j.records)
	j.records = append(j.records, event)
	var mask uint64
	for _, player := range readers {
		mask |= 1 << player
		j.players[player] = append(j.players[player], index)
		if event.EntityID != 0 {
			if j.entities[player] == nil {
				j.entities[player] = map[int][]int{}
			}
			j.entities[player][event.EntityID] = append(j.entities[player][event.EntityID], index)
		}
	}
	j.readers = append(j.readers, mask)
}

func (w *World) record(event Event, entity *Entity, player int) {
	if player == 0 && entity == nil {
		// A world announcement has one separately owned entry per recipient.
		for id := 1; id <= w.Config.Settlements; id++ {
			if w.Players[id] != nil {
				w.record(event, nil, id)
			}
		}
		return
	}
	if player == 0 && entity != nil {
		player = entity.Owner
	}
	w.NextEvent++
	event.ID, event.Tick, event.Player = w.NextEvent, w.Tick, player
	event.UserID = "world"
	if p := w.Players[player]; p != nil {
		event.UserID = p.UserID
	}
	event.Time = w.Time
	if entity != nil {
		event.EntityID, event.EntityType = entity.ID, entity.Type
		event.EntityName = definitions[entity.Type].Name
		if event.State == "" {
			event.State = string(entity.behavior.State())
		}
		event.Activity, event.Position = w.activity(entity), entity.Position
		if event.Resource == "" {
			event.Resource = entity.CargoType
			if event.Resource == "" {
				event.Resource = entity.Resource
			}
		}
	}
	event = observedEvent(event)
	readers := []int{}
	if w.Players[player] != nil {
		readers = append(readers, player)
	}
	w.journal.append(event, readers)
}

func (w *World) entityEvent(e *Entity, kind, message string, amount float64) {
	w.record(Event{Kind: kind, Message: message, Amount: amount}, e, 0)
}

func (w *World) Log(player int, query LogQuery) (EventPage, error) {
	indices := w.journal.players[player]
	page := EventPage{Events: []Event{}}
	if !ValidLogCategory(query.Category) {
		return page, rule("invalid_filter", "Choose an event category from the catalog.")
	}
	if query.EntityID != 0 {
		indices = w.journal.entities[player][query.EntityID]
		if len(indices) == 0 {
			return page, rule("entity_not_found", "No personal history exists for that entity.")
		}
		last := observedEvent(w.journal.records[indices[len(indices)-1]])
		page.Entity = &HistoryEntity{ID: last.EntityID, Name: last.EntityName, Type: last.EntityType, Activity: "Last seen: " + last.Activity}
		if e := w.Entities[query.EntityID]; e != nil && e.Owner == player {
			page.Entity.Activity, page.Entity.Present = w.activity(e), true
		} else if last.Activity == "Destroyed" {
			page.Entity.Activity = "Destroyed"
		}
	}
	indices = w.journal.filtered(player, query, indices)
	if len(indices) == 0 {
		return page, nil
	}
	page.LatestCursor = w.journal.records[indices[len(indices)-1]].ID
	limit := query.Limit
	if limit <= 0 {
		limit = 100
	}
	limit = min(limit, 200)
	start, end := 0, len(indices)
	if query.FromStart || query.After > 0 {
		start = sort.Search(len(indices), func(i int) bool { return w.journal.records[indices[i]].ID > query.After })
		end = min(len(indices), start+limit)
	} else {
		if query.Before > 0 {
			end = sort.Search(len(indices), func(i int) bool { return w.journal.records[indices[i]].ID >= query.Before })
		}
		start = max(0, end-limit)
	}
	page.HasOlder, page.HasNewer = start > 0, end < len(indices)
	for _, index := range indices[start:end] {
		page.Events = append(page.Events, observedEvent(w.journal.records[index]))
	}
	if len(page.Events) > 0 {
		page.OlderCursor, page.NewerCursor = page.Events[0].ID, page.Events[len(page.Events)-1].ID
	}
	return page, nil
}

// Discovery is the viewer's observation, not access to the subject's private
// activity. Project old discovery records safely without rewriting history.
func observedEvent(e Event) Event {
	if e.Kind == "discovered" {
		e.State, e.Activity = "observed", "Observed"
		e.Resource = ""
	}
	return e
}

// Activity is a projection of the state-machine owners, never another mutable
// lifecycle. The frontend displays these labels without deriving game rules.
func (w *World) activity(e *Entity) string {
	if e.life.State() == Destroyed {
		return "Destroyed"
	}
	if e.life.State() == Foundation {
		return "Under construction"
	}
	if e.life.State() == Exhausted {
		return "Depleted — waiting to be reseeded"
	}
	if len(e.Tasks) > 0 {
		if e.production.State() == ProductionBlocked {
			return "Production blocked — waiting for housing or a clear exit"
		}
		t := e.Tasks[0]
		switch t.Type {
		case "train":
			return "Training " + definitions[t.Product].Name
		case "research":
			return "Researching " + technologies[t.Product].Name
		case "age":
			return "Advancing to the next age"
		}
	}
	if e.Type == "trebuchet" && e.siege.State() != SiegePacked {
		return strings.ReplaceAll(string(e.siege.State()), "_", " ")
	}
	if e.Order.Kind == "guard" {
		if e.Order.Threat != 0 {
			return "Defending escort target"
		}
		return "Guarding"
	}
	target := w.Entities[e.Order.Target]
	work := "Gathering"
	if target != nil {
		switch target.Resource {
		case "wood":
			work = "Logging"
		case "stone":
			work = "Mining stone"
		case "gold":
			work = "Mining gold"
		case "food":
			work = "Gathering food"
			if target.Type == "farm" {
				work = "Farming"
			}
			if target.Type == "fish" {
				work = "Fishing"
			}
		}
	}
	switch e.behavior.State() {
	case Caravanning:
		if s := w.Marketplace.Shipments[e.Order.Shipment]; s != nil {
			return "Caravan: " + strings.ReplaceAll(string(s.lifecycle.State()), "_", " ")
		}
		return "Caravan"
	case SeekingResource:
		if target != nil && target.Type == "farm" && farmOccupied(nil, &unitContext{World: w, Actor: e, Target: target}) == nil {
			return "Waiting for an available farm"
		}
		return "Walking to work: " + strings.ToLower(work)
	case Gathering:
		return work
	case Returning:
		if w.dropOff(e) == nil {
			return "Waiting for a " + e.CargoType + " drop-off"
		}
		return "Returning " + e.CargoType
	case Constructing:
		if target != nil {
			return "Building " + definitions[target.Type].Name
		}
		return "Building"
	case Repairing:
		return "Repairing"
	case Moving:
		if e.Order.Kind == "attack_move" {
			return "Marching to attack"
		}
		return "Moving"
	case Chasing:
		return "Chasing an enemy"
	case Attacking, AttackCooldown:
		return "Attacking"
	case ApproachingHeal, Healing:
		return "Healing"
	case ApproachingConvert, Converting:
		return "Converting"
	case RecoveringFaith:
		return "Recovering faith"
	case ApproachingRelic, CollectingRelic:
		return "Collecting a relic"
	case ApproachingDeposit, DepositingRelic:
		return "Depositing a relic"
	case Embarking:
		return "Entering a garrison"
	case Garrisoned:
		return "Garrisoned"
	case Trading:
		return "Trading"
	case ReturningTrade:
		return "Returning trade gold"
	default:
		if definitions[e.Type].Kind == "resource" {
			return "Available"
		}
		if e.Type == "farm" {
			return "Ready to farm"
		}
		return "Idle"
	}
}

type transitionRecorder interface {
	recordTransition(scope, from, to, event string)
}

func (c *unitContext) recordTransition(scope, from, to, event string) {
	if c.World == nil || c.Actor == nil || from == to {
		return
	}
	message := c.World.activity(c.Actor)
	if to == string(Idle) {
		message = "Finished previous activity; now idle"
	}
	if to == string(Returning) && c.Actor.Cargo > 0 {
		message = fmt.Sprintf("Collected %.2f %s; returning to deposit", c.Actor.Cargo, c.Actor.CargoType)
	}
	c.World.record(Event{Kind: "activity", Message: message, Lifecycle: scope, PreviousState: from, State: to, TargetID: c.Actor.Order.Target}, c.Actor, 0)
}

func (c *entityContext) recordTransition(scope, from, to, event string) {
	if c.World == nil || c.Actor == nil {
		return
	}
	if scope == "life" && event == string(DamageEntity) {
		c.World.entityEvent(c.Actor, "damage", fmt.Sprintf("Took %.2f damage", c.Amount), c.Amount)
	}
	if from == to {
		return
	}
	kind, message := scope, c.World.activity(c.Actor)
	if scope == "life" && to == string(Destroyed) {
		kind, message = "destroyed", "Removed from the battlefield"
	}
	if scope == "life" && event == string(BuildWork) {
		message = "Construction completed"
	}
	if scope == "life" && event == string(ReseedFarm) {
		message = "Farm reseeded for 60 wood"
	}
	if scope == "life" && event == string(ExhaustResource) {
		message = "Resource depleted"
	}
	c.World.record(Event{Kind: kind, Message: message, Lifecycle: scope, PreviousState: from, State: to}, c.Actor, 0)
}

func (w *World) recordCommand(player int, command Command, actors []*Entity, err error) {
	message := strings.ReplaceAll(command.Kind, "_", " ")
	if d, ok := definitions[command.Product]; ok {
		message += " " + d.Name
	} else if t, ok := technologies[command.Product]; ok {
		message += " " + t.Name
	} else if command.Kind == "market_buy" || command.Kind == "market_sell" {
		message += " " + command.Product
	}
	if command.Position != nil {
		message += fmt.Sprintf(" at (%.1f, %.1f)", command.Position.X, command.Position.Y)
	}
	if command.Kind == "speed" {
		message = fmt.Sprintf("Game speed set to %g×", command.Value)
	}
	if command.Kind == "pause" {
		message = "Match resumed"
		if w.match.State() == MatchPaused {
			message = "Match paused"
		}
	}
	kind := "order"
	if err != nil {
		kind, message = "order_rejected", "Order declined: "+err.Error()
	} else if command.Queue {
		message = "Queued order: " + message
	} else if len(actors) > 0 {
		message = "Order: " + message
	}
	entry := Event{Kind: kind, Message: message, CommandID: command.ID}
	if err == nil {
		entry.TargetID = command.TargetID
	}
	if len(actors) == 0 {
		w.record(entry, nil, player)
	}
	for _, actor := range actors {
		w.record(entry, actor, player)
	}
}

func taskName(task Task) string {
	switch task.Type {
	case "train":
		return definitions[task.Product].Name
	case "research":
		return technologies[task.Product].Name
	default:
		return "age advancement"
	}
}
