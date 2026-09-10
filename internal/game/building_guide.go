package game

import (
	"fmt"
	"slices"
	"strings"
)

// Catalog copy describes implemented rules. Capabilities come from production
// definitions and technologies, the same sources used by command validation.
func describeBuildings() {
	purpose := map[string][2]string{
		"bridge":         {"Build a crossing between riverbanks.", "Drag from land to land. Each water tile costs 20 wood and 5 stone. Soldiers, workers and carts can cross completed sections; ships pass underneath. Protect the crossing: collapsing sections can drown troops."},
		"town_center":    {"Train villagers and advance your kingdom.", "Your economic center: settle near resources to shorten deliveries."},
		"house":          {"Make room for 5 more people.", "Build before reaching your population cap so training can continue."},
		"mill":           {"Collect food close to berries and farms.", "Shorter deliveries keep villagers gathering for more of their workday."},
		"lumber_camp":    {"Collect wood beside a forest.", "Wood funds buildings, farms, ships and many military units."},
		"mining_camp":    {"Collect gold and stone near deposits.", "Gold supports advanced troops and trade; stone supports durable defenses."},
		"farm":           {"Plant a renewable food source.", "One villager works each farm. Farmers replant depleted fields when 60 wood is available."},
		"barracks":       {"Train infantry and unlock other military buildings.", "An early defense against raids, with spearmen to counter cavalry."},
		"archery_range":  {"Train ranged troops.", "Support your front line with archers; skirmishers counter enemy archers."},
		"stable":         {"Train cavalry for scouting, raids and escorts.", "Fast mounted troops protect exposed workers and intercept trade raiders."},
		"blacksmith":     {"Improve your army's weapons and armor.", "Research strengthens existing troops as well as future recruits."},
		"market":         {"Exchange local goods and dispatch Trade Carts.", "Buy scarce resources, sell surpluses and trade with distant markets or other kingdoms. Deliveries carry real cargo."},
		"tower":          {"Defend a small area with arrows.", "Cover workers, trade routes and approaches. Attack orders can start a conflict."},
		"wall":           {"Build a durable stone barrier.", "Close approaches and funnel attackers toward defended gates."},
		"gate":           {"Let your units cross a defensive wall.", "Automatically aligns with connected walls. Your units can pass; enemies must break through."},
		"palisade":       {"Build an inexpensive timber barrier.", "Delay early raids and protect workers while stronger defenses are being built."},
		"castle":         {"Fortify a position and train unique troops.", "Defend an important settlement or trade corridor and provide population capacity."},
		"siege_workshop": {"Train siege engines.", "Break fortifications with rams and support assaults with ranged siege."},
		"monastery":      {"Train monks, heal troops and secure relics.", "Relics stored here generate gold; monks can also convert eligible enemies."},
		"university":     {"Research advanced military technology.", "Improve projectiles and siege, and unlock gunpowder with Chemistry."},
		"dock":           {"Train ships and collect fish.", "Connect your economy across water with Fishing Ships, transports and funded Trade Ships; galleys protect sea lanes."},
		"wonder":         {"Pursue a timed Wonder victory.", "An expensive alternative victory objective. Defend it through the victory countdown."},
	}
	for id, copy := range purpose {
		d := definitions[id]
		d.Description, d.Importance = copy[0], copy[1]
		d.Capabilities = append(d.Capabilities, fmt.Sprintf("Grid footprint: %d × %d tiles", d.Footprint, d.Footprint))
		if d.Housing > 0 {
			d.Capabilities = append(d.Capabilities, fmt.Sprintf("Population capacity +%d", d.Housing))
		}
		if len(d.DropOff) > 0 {
			d.Capabilities = append(d.Capabilities, "Drop-off: "+strings.Join(d.DropOff, ", "))
		}
		units, techs := []string{}, []string{}
		for _, unit := range definitions {
			if unit.Producer == id {
				if id == "castle" && unit.Class != "siege" {
					continue
				}
				units = append(units, unit.Name)
			}
		}
		if id == "castle" {
			units = append(units, "Civilization unique unit")
		}
		for _, tech := range technologies {
			if tech.Producer == id {
				techs = append(techs, tech.Name)
			}
		}
		slices.Sort(units)
		slices.Sort(techs)
		if len(units) > 0 {
			d.Capabilities = append(d.Capabilities, "Trains: "+strings.Join(units, ", "))
		}
		if len(techs) > 0 {
			d.Capabilities = append(d.Capabilities, "Research: "+strings.Join(techs, ", "))
		}
		if d.Attack > 0 {
			detail := fmt.Sprintf("Base defense: %.0f attack · %.0f range", d.Attack, d.Range)
			if id == "town_center" {
				detail += " · requires garrisoned units"
			}
			d.Capabilities = append(d.Capabilities, detail)
		}
		if id == "farm" {
			d.Capabilities = append(d.Capabilities, "175 food per planting before farm upgrades", "The builder starts farming; replanting costs 60 wood")
		}
		if id == "market" || id == "dock" {
			d.Capabilities = append(d.Capabilities, "Local exchange, regional deliveries and kingdom offers")
		}
		if id == "monastery" {
			d.Capabilities = append(d.Capabilities, "Each stored relic produces 0.5 gold per game second")
		}
		definitions[id] = d
	}
}
