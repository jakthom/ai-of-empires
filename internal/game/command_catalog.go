package game

// CommandInfo documents intentions supported by multiplayer and MCP. Fields
// describe the wire contract; Apply remains the authority for all validation.
type CommandInfo struct {
	Kind        string   `json:"kind"`
	Description string   `json:"description"`
	Fields      []string `json:"fields"`
}

func PlayerCommands() []CommandInfo {
	return []CommandInfo{
		{"move", "Move mobile units to a map-space destination.", []string{"entity_ids", "position", "queue?"}},
		{"attack_move", "Move and attack; optionally restrict proactive targeting to another kingdom.", []string{"entity_ids", "position", "target_player?", "queue?"}},
		{"interact", "Resolve a contextual order in Go. Use this to continue an existing foundation.", []string{"entity_ids", "target_id", "queue?"}},
		{"attack", "Attack an observable enemy with eligible combat units.", []string{"entity_ids", "target_id", "queue?"}},
		{"gather", "Gather land resources with villagers or fish with fishing ships; delivery is automatic.", []string{"entity_ids", "target_id", "queue?"}},
		{"repair", "Repair your building with villagers.", []string{"entity_ids", "target_id", "queue?"}},
		{"heal", "Heal your other units with monks.", []string{"entity_ids", "target_id", "queue?"}},
		{"convert", "Attempt to convert an observable enemy with monks.", []string{"entity_ids", "target_id", "queue?"}},
		{"relic", "Collect an observable relic with a monk.", []string{"entity_ids", "target_id", "queue?"}},
		{"deposit_relic", "Deposit a carried relic at your completed monastery.", []string{"entity_ids", "target_id", "queue?"}},
		{"garrison", "Board your eligible building or transport with land units.", []string{"entity_ids", "target_id", "queue?"}},
		{"trade", "Run a repeating trade-cart route to an explored neutral market.", []string{"entity_ids", "target_id", "queue?"}},
		{"build", "Create a building with villagers. Walls and palisades accept an end position; check_placement plans and prices the route.", []string{"entity_ids", "product", "position", "end_position?", "queue?"}},
		{"train", "Queue a catalog unit at one producer.", []string{"entity_ids", "product"}},
		{"research", "Queue a catalog technology at one producer.", []string{"entity_ids", "product"}},
		{"age", "Queue age advancement at one Town Center.", []string{"entity_ids"}},
		{"cancel", "Cancel one producer's task at the zero-based queue index in value.", []string{"entity_ids", "value"}},
		{"rally", "Set one building's rally destination.", []string{"entity_ids", "position"}},
		{"deploy", "Toggle packing or deployment for one trebuchet when permitted.", []string{"entity_ids"}},
		{"unload", "Unload passengers from one building or transport at a clear exit.", []string{"entity_ids"}},
		{"stop", "Interrupt the selected units' current and queued orders.", []string{"entity_ids"}},
		{"delete", "Permanently remove your selected entities; this can defeat your kingdom.", []string{"entity_ids"}},
		{"stance", "Set automatic response: defensive, stand_ground, passive or aggressive in product.", []string{"entity_ids", "product"}},
		{"reseed_farm", "Reseed one depleted owned farm; Go chooses a worker and charges the cost.", []string{"entity_ids"}},
		{"market_buy", "Buy food, wood or stone using one completed market; product names the resource.", []string{"entity_ids", "product"}},
		{"market_sell", "Sell food, wood or stone using one completed market; product names the resource.", []string{"entity_ids", "product"}},
		{"resign", "Resign only your authenticated kingdom.", []string{}},
	}
}
