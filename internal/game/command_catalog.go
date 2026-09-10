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
		{"peace_offer", "Offer gold reparations to a kingdom in conflict. Gold is reserved until accepted, refused, withdrawn or expired.", []string{"target_player", "value"}},
		{"peace_accept", "Accept a peace payment addressed to your kingdom; recall mutual attacks and restore peace.", []string{"offer_id"}},
		{"peace_decline", "Decline a peace payment addressed to your kingdom.", []string{"offer_id"}},
		{"peace_withdraw", "Withdraw your pending peace offer and recover reserved gold.", []string{"offer_id"}},
		{"guard", "Escort an observable friendly unit, Market, Dock or neutral supply caravan. Engage nearby threats and return to the protected target. Soldiers escort on land; warships at sea.", []string{"entity_ids", "target_id", "queue?"}},
		{"move", "Move mobile units in direction-aligned ranks to a map-space destination. Groups share a pace and negotiate narrow passages.", []string{"entity_ids", "position", "queue?"}},
		{"attack_move", "Move and attack; optionally restrict proactive targeting to another kingdom.", []string{"entity_ids", "position", "target_player?", "queue?"}},
		{"interact", "Resolve a contextual order in Go. Use this to continue an existing foundation.", []string{"entity_ids", "target_id", "queue?"}},
		{"attack", "Attack an observable foreign or neutral economic entity. Starting an attack against a peaceful kingdom immediately declares conflict.", []string{"entity_ids", "target_id", "queue?"}},
		{"gather", "Gather land resources with villagers or fish with fishing ships; delivery is automatic.", []string{"entity_ids", "target_id", "queue?"}},
		{"repair", "Repair your building with villagers.", []string{"entity_ids", "target_id", "queue?"}},
		{"repair_building", "Assign an idle villager to repair one damaged building.", []string{"entity_ids"}},
		{"work_farm", "Assign an idle villager to work one planted farm.", []string{"entity_ids"}},
		{"rotate_gate", "Rotate a standalone gate or choose its wall alignment.", []string{"entity_ids", "orientation?"}},
		{"heal", "Heal your other units with monks.", []string{"entity_ids", "target_id", "queue?"}},
		{"convert", "Attempt to convert an observable enemy with monks.", []string{"entity_ids", "target_id", "queue?"}},
		{"relic", "Collect an observable relic with a monk.", []string{"entity_ids", "target_id", "queue?"}},
		{"deposit_relic", "Deposit a carried relic at your completed monastery.", []string{"entity_ids", "target_id", "queue?"}},
		{"garrison", "Board your eligible building or transport with land units.", []string{"entity_ids", "target_id", "queue?"}},
		{"trade", "Ship 100 goods to sell, or gold to buy 100 goods, at an explored neutral trading post. Use one idle Trade Cart at your Market or Trade Ship at your Dock, with a matching destination. Repeat reprices each trip within trade_limit (minimum sale or maximum purchase gold).", []string{"entity_ids", "target_id", "product?", "trade_mode?", "trade_limit?", "market_revision?", "repeat?"}},
		{"build", "Create grid-aligned buildings with villagers. Walls/palisades accept an end position; bridges require two riverbanks. Gates accept auto, east_west or north_south orientation. check_placement returns snapped sites and total price. Queued repeated buildings and replacement workers continue their construction batch.", []string{"entity_ids", "product", "position", "end_position?", "orientation?", "queue?"}},
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
		{"market_buy", "Buy 100 food, wood or stone from your home merchants. Stock and prices are local; supplies arrive by caravan. Supply the observed home market revision to require that quote.", []string{"entity_ids", "product", "market_revision?"}},
		{"market_sell", "Sell 100 food, wood or stone to your home merchants at their current local quote.", []string{"entity_ids", "product", "market_revision?"}},
		{"market_post", "Post a standing buy, sell or barter offer at your completed Market or Feudal-age Dock. Reserves give_amount times lots; optional target_player makes the terms private to that kingdom.", []string{"entity_ids", "offer"}},
		{"market_cancel", "Close your offer and refund its unclaimed lots. Accepted caravans continue.", []string{"offer_id"}},
		{"market_accept", "Accept one lot using an idle, empty Trade Cart at your Market or Trade Ship at your Dock. Scout the matching destination first. Payment travels outward and purchased goods return. repeat continues while stock and payment remain available.", []string{"entity_ids", "offer_id", "repeat?"}},
		{"market_resume", "Resume your interrupted caravan without paying or reserving goods twice.", []string{"shipment_id"}},
		{"market_recall", "Recall your caravan before collection. Seller goods are released; your payment returns with the cart.", []string{"shipment_id"}},
		{"resign", "Resign only your authenticated kingdom.", []string{}},
	}
}
