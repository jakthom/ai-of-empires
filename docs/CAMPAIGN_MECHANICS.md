# Campaign economy, construction and reports

These are implemented `frontier-1` rules. They are a deliberate game economy, not Age of Empires parity or a complete model of a real economy. Go owns all quantities, timing, visibility, orders and outcomes.

## Population and resource flows

Each occupied population slot consumes **2 food per game minute**, including garrisoned units and ships. Queued units do not eat. Pausing stops upkeep; higher game speed advances upkeep and production together. Consumption cannot overdraw the treasury. Insufficient food begins a shortage; 60 uninterrupted game seconds without sufficient food causes famine. Famine halves gathering, construction and production speed. A supplied tick restores full speed. Hunger does not directly kill units.

The HUD and **Stats** show both production and consumption for food, wood, gold and stone. Solid lines show production; dashed lines show consumption. Go records five-second samples of the resources actually produced or spent during the preceding game minute. The first minute includes only elapsed activity. Charts retain five game minutes, with sample inspection, an accessible table and CSV export. They do not infer production from stockpile changes.

Production includes delivered harvests, fish and relic income. Consumption includes construction, training, research, age advancement, repairs, farm planting, settled outgoing trade and population upkeep. Reserving an offer or peace payment is not consumption. Completed trade payments are counted when exchanged; incoming goods are not production. Refunds are reported separately from gross spending. Raided cargo is a transfer rather than production. These accounts are not a stock reconciliation: opening resources, escrow, inbound transfers and losses are distinct from production and consumption.

AI pays the same upkeep. It prefers nearby food and new farms to distant natural food when it can afford planting, redirects villagers waiting for occupied farms, and assigns additional food workers when reserves are low. Upkeep competes with recruitment and can delay raids.

## Global market and regional scarcity

**Trade → Global market** shows publicly funded demand and supply for food, wood and stone, the number of buy/sell listings, and best bid/ask prices in gold per 100 resources. Gold is the settlement currency. Buy and Sell prepare a public offer with editable quantities and price. Browse opens the matching standing offers. Private listings never enter this aggregate.

Offers stay open until filled, cancelled or invalidated by loss of their trading post. The offering kingdom reserves its goods or gold. Another kingdom scouts the matching Market or Dock and sends an idle carrier to accept a lot. Payment and goods travel physically; travel time, gates, bridges, guards and cargo raids matter. Automatic matching of independent orders is not implemented. Home merchants also provide immediate exchanges against their finite local inventory; other regional merchants require a route. See [Marketplace](MARKETPLACE.md) for exact cargo, pricing and repetition rules.

Biome scarcity, finite deposits and regional consumer demand remain in force. Supply caravans replenish merchants at bounded rates. Public demand reports do not expose hidden merchant inventories or private kingdom stockpiles.

## Construction and maintenance

Building footprints are integer tile widths reported by the catalog. Odd widths use tile centers; even widths use grid intersections. The server validates and returns the snapped sites. Farms are **2×2 tiles**, so neighboring fields can share an edge without a gap. Existing saved building positions are preserved.

Drag walls or palisades to place a batch. Shift-place repeated buildings to queue a batch. A construction order inherits its foundation's batch; after finishing, a villager continues the nearest reachable unfinished foundation in that batch. A replacement villager receives the same continuation by interacting with any remaining foundation. Explicit queued orders take priority. Stop cancels the worker's orders, leaving paid foundations available for takeover. No continuation charges for a foundation twice.

The Build guide explains each building's purpose, strategic importance, footprint, housing, drop-off, training, research and combat capabilities where applicable. Go supplies these descriptions and availability reasons. Select a damaged building and choose **Repair building** to assign an idle villager, or order selected villagers to Repair. **Work farm** assigns an idle worker to a planted field. **Reseed farm** and automatic farmer reseeding cost 60 wood.

Gates support Auto align, east–west and north–south orientations. Press **R** during gate placement or use the on-screen buttons. Adjacent barriers constrain orientation; impossible perpendicular placements are refused. A standalone selected gate can be rotated. Gates can replace an owned barrier segment.

## Bridges

Choose **Bridge** and drag from one explored land bank to another. The server chooses a horizontal or vertical span, up to 32 tiles between banks, with water or shallows in between. Each water segment costs **20 wood + 5 stone**, takes **12 game seconds** for one builder before work modifiers, and has **400 HP**. Banks are not charged as segments.

Builders continue along the span as completed segments become walkable. Land routes, caravans and armies can cross; ships continue beneath the deck. Bridge entity life is authoritative; navigation and tile presentation flags are derived and rebuilt on load. Destroying a segment removes that crossing and kills land units on its water cell; ships remain afloat. Bridges can be repaired. Incomplete bridges retain their paid foundations and builder orders through snapshots.

## Acts of war and reparations

A validated attack or conversion order against a peaceful kingdom declares conflict immediately, before the first hit. Queued attacks declare when they begin. Initial peace periods still prohibit attacks. Default return-fire units require witnessed aggression, while deliberate aggressive orders may initiate conflict.

Unowned economic units and structures can be explicitly attacked, including neutral Markets, Docks and supply caravans. Natural deposits remain gathering targets. Raiding neutral trade puts the attacker in conflict with kingdoms currently using the target's delivery route or guarding it; a home supply caravan implicates its home kingdom. Unrelated kingdoms remain at peace. Destroyed carriers yield their carried cargo through the existing exactly-once spoils mechanism; warehouses and whole treasuries are not looted.

Open **Kingdom relationships → Reparations & peace**. An offer reserves a whole-number gold payment to a kingdom currently in conflict. Offers expire after **120 game seconds**; the sender can withdraw and the recipient can accept or decline. Refusal, withdrawal, expiry, defeat or an already-restored peace returns the reserved payment once.

An AI accepts at least **max(50, ceil(1.1 × damage value suffered)) gold**. Damage value weights lost HP by the victim entity's resource cost; the UI shows the current threshold. Human recipients decide for themselves. Acceptance transfers the reserved gold, clears mutual retaliation incidents, recalls mutual attacks and cancels projectiles in flight. Guards retain their escort orders. AI honors purchased peace for **five game minutes**, unless attacked again. Open offers and all outcomes survive saves without replaying escrow, refunds or settlement. Limits: one pending offer per directed kingdom pair and 1,000 offers per campaign.

## God mode and campaign reports

The game owner can enable **God mode** from the match menu and return to kingdom vision with the visible banner. It reveals terrain and battlefield entities only to that owner's observation connection. It does not change any player's fog, AI input, command permissions or gameplay-agent view. Foreign production queues and orders remain private; world statistics have their own owner authorization.

**Stats** offers private live accounting to every kingdom. The owner can compare all kingdoms during play; after the match finishes, all game members can read the world report. The full report includes:

- Resource production, consumption, spending by purpose, refunds, stocks, incoming and outgoing trade, and food shortage totals.
- GDP, GDP per game minute and per current person, stock value and completed construction investment.
- Workers and assignments, idle workers, military, population peaks, buildings, foundations, trained/lost units, destroyed buildings, technologies, damage and exploration.
- One-minute campaign histories of cumulative production/consumption, GDP, population and active buildings, bounded to the latest 4,320 samples.
- The official conquest/wonder result plus separate economic, development, trade, technology, military, population and exploration awards. Ties are preserved. Awards do not change the match's victory rules.
- A generated kingdom-by-kingdom summary, full JSON download and resource-rate CSV.

**GDP is a fixed-price production proxy:** food, wood and gold each have value 1; stone has value 1.3. Trade, gifts, refunds and captured stock are excluded. Construction is reported as completed investment, including replanted farms, and is not added to GDP. Development rate is completed buildings per elapsed game minute since accounting began. This definition avoids treating repeated resales as new production.

Version-8 checkpoints preserve the new accounting and lifecycles. Older checkpoints remain readable; cumulative new statistics start when that save is loaded, with `accounting_since` identifying the coverage. Existing retained production samples may predate the new cumulative accounts. Replay seeking, player-selected line/box/flank formations, formal alliance treaties, auto-matched market orders and additional match-ending victory modes remain future work.

## Named starting points and SQLite downloads

**Match menu → Snapshots & exports** saves an immutable named point at the current game time. The owner can retain up to 100 named points per game. Starting from one forks a separate paused game with a new ID, fresh player access, the saved world, orders, economy and lifecycle state. AI seats are retained; other human seats become vacant. The source game and saved point do not advance or change when the copy is played. Repeating the same fork request ID returns the same copy.

**Download SQLite file** exports the current game as a consistent standalone database, including the named snapshot library. It contains private state and recovery data. Portable `.aoegame` archives preserve the selected world but do not bundle the source's named snapshot library. See [saved-game transfer instructions](../README.md#saved-games).

Screenshots from public-control browser validation: [marching ranks](screenshots/army-formation.png), [final achievement report](screenshots/campaign-report.png), and [completed river crossing](screenshots/river-bridge.png).
