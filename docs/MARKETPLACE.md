# Marketplace and regional resources

Open **Trade** in the top bar. Offers, Caravans, Merchants and Resources share the same authenticated Go read model used by REST and MCP. A completed Market or Feudal-age Dock is required to post offers, accept deliveries and exchange resources. Reading the book is available before building one.

## Standing offers

An offer describes what its author gives and wants **per lot**. Offer wood for gold to sell, gold for wood to buy, or wood for stone to barter. Both quantities must be whole numbers from 1 to 500; the resources must differ. Post 1–20 lots, with at most 12 open offers per kingdom. Offers stay open until claimed, cancelled, or invalidated by the offering trading post’s loss or the kingdom's defeat.

Posting removes all advertised goods from spendable stock and reserves them. Accepting removes one lot from the listing and assigns its reserved goods to the delivery. Cancelling returns only unclaimed lots; accepted deliveries continue. The author cannot change prices under an accepted caravan. Cancel and post a new offer to advertise different terms.

`target_player: 0` publishes terms to all kingdoms. A positive target makes the offer visible only to the author and that kingdom. All offers disclose terms and remaining lots, not the author's stockpile or production. Listings do not reveal unexplored Market locations. The accepting player must scout the offering Market first; previously observed Markets can be used from fog memory. A new post is an explicit disclosure of the advertised terms and availability.

## Physical delivery

The accepting player supplies an idle, empty Trade Cart beside a completed owned Market, or a Trade Ship beside a completed owned Dock in Feudal Age. Both carriers cost 100 wood and 50 gold. Carts connect Markets by land; ships connect Docks by water. The kingdoms must be at peace. A carrier holds one lot, up to 500 of either resource on its respective journey. Mixed Market-to-Dock routes are refused before funding.

1. Acceptance loads the requested payment from the accepting kingdom's stockpile into its carrier. The offered goods remain reserved at the offering kingdom.
2. At the offering Market or Dock, the payment enters that kingdom's stockpile and the advertised goods replace it in the carrier.
3. The carrier returns to its matching home post, where the purchased goods enter the accepting kingdom’s stockpile. If that post was lost or became unreachable, another reachable owned post of the same type can receive them.

**Repeat trips** attempts the next lot through the same command boundary after delivery commits. It stops when the offer closes or a new acceptance is refused, including insufficient payment. An offer can be filled by multiple kingdoms; each claimed lot needs its own available carrier. Separate posted offers are not automatically matched against one another.

Movement uses normal collision, pathfinding and unit speed. Peaceful trading grants the assigned caravan passage through the offering kingdom's gates; military units receive no new passage rights. This does not share vision. Blocked paths retry normally. Build/open a route, move the cart, or recall it as appropriate. Trade Ships connect islands through reachable Docks; land carts cannot cross deep water.

| Interruption | Result |
|---|---|
| Stop or move the carrier | Pauses the delivery, retaining cargo. Resume continues without a second charge. Other cargo/trade/garrison orders are refused while it has an active shipment. |
| Recall before pickup | Releases the offering kingdom's reserved goods immediately. Payment returns physically with the carrier. |
| Offering trading post lost, offering kingdom defeated, or conflict begins before pickup | Automatically recalls the delivery. |
| Home post lost | Returns to another reachable owned post of the same type, or waits for one. |
| Carrier destroyed, converted, or accepting kingdom defeated | A lethal attacker captures carried cargo; conversion/defeat otherwise loses undelivered cargo. Offered goods that were never collected are released. Already delivered payment is retained by its recipient. |
| Pause/save/restart | Retains orders, cargo, reservations and lifecycle state; time stops while paused. Restore never replays a payment or refund. |

Participants see their agreed terms and delivery milestones. Only the carrier’s owner sees interruption and route details outside ordinary map visibility. A foreign carrier’s location is included only while visible, including for its trading partner. Third parties do not receive delivery records. Trade receipts remain in each participant's private chronicle. The read model retains recent deliveries (200 records globally, plus older active deliveries); the complete journal remains stored.

## Escorts and spoils

Select mobile soldiers or warships, choose **Guard**, then click a friendly economic unit or trading post. Neutral Supply Caravans can also be protected. Soldiers follow land units; warships follow ships or guard Docks. Guards stay near their charge, intercept nearby hostile forces or witnessed attackers, and return after combat. Their ordinary weapons, health, costs and collisions determine protection; escorting is not an invulnerability bonus. Hold fire disables automatic attacks. A new order interrupts the escort; losing the charge, its ownership or its visibility ends it. Guards gain no foreign vision or military gate access.

A lethal attack immediately credits the killing kingdom with the resources physically aboard the victim. Workers surrender gathered cargo; trade carriers surrender outbound payment/exports or returning goods/proceeds. Supply Caravan spoils include its goods and 240-gold buyer budget. Loaded transport passengers’ resource cargo is included. Cargo is consumed once before removal, so a raided supply cannot also replenish its destination. Private `spoils` events record capture; this transfer does not count as production.

Uncollected reservations remain at the destination and are released by the shipment lifecycle. Destroying an empty Market or Dock does not loot merchant inventory or the kingdom treasury. Deletion and conversion do not award destruction spoils. There is no map pickup or wreck salvage command.

AI assigns up to two nearby idle soldiers or warships to loaded deliveries. Assigned escorts stay out of its raid and rally rosters. Raiders assess visible defending strength, so an adequate escort can discourage a costly attack; concealed troops and cargo quantities are never consulted.

## Local merchants, production and demand

Every kingdom has one home merchant inventory, priced using its starting region. All of that kingdom’s owned Markets and Docks access this same inventory, even if another post is built elsewhere. This prevents instant price arbitrage through the kingdom-wide stockpile and prevents rebuilding from resetting supply. Each neutral Market or Dock has its own inventory and local prices. New maps place outlying Markets and coastal trading Docks where terrain and space permit; existing maps retain their buildings and deposits.

A region initially holds 1,000 food, wood and stone multiplied by the local biome's deposit factors, plus 2,000 gold. Worlds without per-tile biome data use their configured biome; worlds without a biome use the unmodified quantities. Buying and selling changes that region alone. Each exchange transfers real resources in lots of 100. For a commodity stock `s`, the wholesale price per 100 is `clamp(100 + (1000-s)*0.08, 30, 200)`. Purchases cost `ceil(price*1.3)` gold; sales return `floor(price*0.7)`, or `floor(price*0.84)` for Saracens. Stock shortages therefore increase prices and surpluses lower them, within bounds. At 1,000 stock the quotes are 130/70 gold (84 for a Saracen sale).

Food, wood and stone warehouses each hold at most 5,000, including space reserved for accepted incoming deliveries and possible refunds. Cash can accumulate as it circulates; a well-funded till does not prevent buying goods. Merchants reject lots they cannot fund or receive. `market_revision` refers to the selected region: the home inventory for immediate exchanges, or the destination neutral Market for a caravan.

### Supply caravans

Regional Supply Caravans are visible neutral units using ordinary land movement, collision and fog. Each region prepares at most one shipment at a time. After arrival or loss, it waits 90 game seconds before its next departure. Departure can wait for a free source position and a completed receiving trading post. A home caravan serves the eligible Market or Dock nearest that kingdom’s original settlement; neutral caravans serve their own post. These district supply trips remain on land even when supplying a Dock. Home supply caravans may pass their receiving kingdom's gates. Foreign gates and blockades can interrupt them. Players can explicitly attack supply caravans; they do not attack back, and cargo lost with them never arrives.

A caravan brings up to 60 food, 60 wood and 20 stone multiplied by its region's biome factors. Food and wood represent renewable production in the surrounding district; stone represents limited-rate imports from outside the simulated map. Map deposits do not regrow. Supplies are produced only for an actual departure and enter merchant stock only on physical arrival. Goods beyond warehouse capacity leave with the caravan.

Buyers accompany each arrival with a **240-gold spending budget**. They purchase and consume up to 80 food, 60 wood and 30 stone, in that priority order, at the current local wholesale valuation. Only goods actually bought earn merchant gold. Unspent money leaves; no empty route produces a payment. This is an explicit abstraction of surrounding production and consumer demand, not a closed simulation of every household, mine or coin. Regional output can exceed demand for some goods and fall short for others, sustaining export advantages and import needs.

There is no offline production, missed-cycle catch-up or stock reset when a Market is rebuilt. A blocked caravan delays the whole next supply cycle. Inventory, payload, cart, timers and exactly-once delivery/loss survive checkpoint restore.

### Merchant trade routes

The Merchants page selects either your home merchants or an observed neutral Market or Dock. Home exchange remains immediate through `market_buy` / `market_sell`. Regional **Export** and **Import** controls use `trade` with one idle, empty Trade Cart beside an owned Market or Trade Ship beside an owned Dock:

- `trade_mode: "sell"`: carry 100 of `product` to the neutral trading post and return its reserved gold payment.
- `trade_mode: "buy"`: carry gold to the neutral trading post and return 100 of `product`.
- `repeat: true`: attempt another funded lot after returning, using the new local quote. A refusal stops repetition and is recorded in your private log.
- Optional `trade_limit`: minimum sale proceeds or maximum purchase cost in gold per 100 goods, from 1 to 500. Omission permits any price within the ruleset's bounds on future trips.

Both sides reserve their advertised goods/payment at dispatch. The accepted price is fixed for that trip. Travel distance affects time and risk, **not payment**. `home_margin` compares the current purchase price at the source with the sale price at the destination for 100 goods. It is an indicative price difference, not guaranteed profit: later prices, available stocks, journey time and losses can change the result. Imports, exports and barter are exchanges and do not count as newly produced player resources.

Omitting `trade_mode` selects selling. Older clients that omit `product` sell 100 of their largest held food/wood/stone stockpile. New routes default to one trip; request repetition explicitly. Queued or multiple-carrier `trade` commands are refused rather than partially funded. `market_resume` and `market_recall` also control merchant deliveries. Old unfunded outbound routes stop when a save resumes; already-loaded legacy gold can finish its return once.

Only the owner sees home merchant inventory. Neutral stock, prices and supply details are included in `marketplace.markets` while the trading post is in ordinary sight. Remembered trading posts remain locatable on the map but do not stream hidden price changes. Supply cart positions appear only while visible. A player can explicitly order a funded trade at a remembered neutral trading post, subject to current stock, quote revision and price-limit checks; the accepted contract discloses its own terms. Other players do not receive that delivery's private records.

Checkpoint version 7 preserves local inventories, cargo, escorts, supply lifecycles and visible destruction remains. Versions 1–6 remain readable. Version-5 migration divides the former shared stock among home and neutral regions without duplicating it; regional production establishes new surpluses over time. Older pre-marketplace saves initialize regional merchants. Existing offered goods, active player contracts, player stocks and map deposits remain intact.

## Regional scarcity

New worlds default to Mixed Regions. Biomes scale **countryside land-deposit amounts**, not gathering rates or unit/building statistics:

| Biome | Food | Wood | Gold | Stone |
|---|---:|---:|---:|---:|
| Temperate | 120% | 120% | 80% | 80% |
| Desert | 30% | 25% | 220% | 130% |
| Alpine | 40% | 60% | 130% | 240% |
| Tropical | 180% | 220% | 50% | 35% |
| Autumn | 120% | 180% | 55% | 80% |
| Savanna | 220% | 50% | 140% | 50% |

These are designed gameplay specializations. Food here means berries and sheep. Fishing remains tied to waterways, farms retain their wood/renewal costs, and relics retain their own income rules. Deposits within 15 tiles of a starting settlement ignore biome multipliers so each kingdom has a viable opening. The global scarce/standard/abundant setting additionally scales natural deposits, including starting patches, by 70%/100%/175%. Starting stockpiles remain equal.

`Catalog.worlds.economies` supplies the rules and descriptions to the browser and agents. Regional metadata is generated before resources and remains hidden on unexplored map tiles. Checkpoints retain exact deposit amounts; loading an older world does not regenerate or reprice its resources. Biome scarcity creates longer-term regional advantages as starting patches exhaust; trading is an opportunity, not a mandatory prerequisite to surviving the opening.

## REST and MCP

Read `GET /api/v1/games/{id}/marketplace`, `Snapshot.marketplace`, or the player-specific MCP `marketplace` tool. The same filtered value appears in SSE updates. Submit mutations through the existing authenticated `/commands` endpoint or MCP `command`; identity always comes from the credential.

Post a public standing offer at your own Market (replace entity IDs with observed IDs):

```json
{"id":"wood-offer-1","kind":"market_post","entity_ids":[503],"offer":{"give_resource":"wood","give_amount":200,"want_resource":"stone","want_amount":100,"lots":3}}
```

Accept one lot with a cart and continue while available:

```json
{"id":"trade-1","kind":"market_accept","offer_id":1,"entity_ids":[614],"repeat":true}
```

Cancel your unclaimed lots, or control your own caravan:

```json
{"id":"cancel-offer-1","kind":"market_cancel","offer_id":1}
{"id":"resume-caravan-1","kind":"market_resume","shipment_id":1}
{"id":"recall-caravan-1","kind":"market_recall","shipment_id":1}
```

Export wood repeatedly while the Market pays at least 90 gold per 100:

```json
{"id":"export-wood-1","kind":"trade","entity_ids":[614],"target_id":820,"product":"wood","trade_mode":"sell","trade_limit":90,"repeat":true}
```

Import one lot of stone with a maximum price of 180 gold:

```json
{"id":"import-stone-1","kind":"trade","entity_ids":[614],"target_id":820,"product":"stone","trade_mode":"buy","trade_limit":180}
```

Read `marketplace.merchants` for home quotes and `marketplace.markets` for currently observed neutral stock, production, demand and `routes`. The server supplies `can_start`, `reason`, `cart_id`, exact `cost`/`gain` and indicative `home_margin`. These values also travel through SSE and the MCP `marketplace` tool.

Guard the observed carrier with two owned soldiers (or two warships for a Trade Ship):

```json
{"id":"escort-1","kind":"guard","entity_ids":[701,702],"target_id":614}
```

The wire fields `cart_id`, `market_id` and existing marketplace command names remain compatible: they also identify ships and Docks where applicable. Use observed entity types and server-authored route eligibility rather than assuming land transport.

Every new intention needs a unique command ID. Retry an ambiguous request with the identical ID and payload. The membership-scoped receipt boundary prevents duplicate reservations, cargo loading, payments and refunds. Generated TypeScript, OpenAPI and MCP schemas all derive from the Go wire types.

Built-in AI uses only its own inventory and the book it is allowed to read. It advertises surpluses against shortages, compares home quotes with observed regional quotes, and can train/fund carts or ships to accept useful offers and merchant deliveries while retaining economic reserves. It receives no extra resources or private trading information. Negotiated alliances and automatic matching of separate offers remain future work.
