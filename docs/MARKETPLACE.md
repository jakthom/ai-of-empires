# Marketplace and regional resources

Open **Trade** in the top bar. Offers, Caravans, Merchants and Resources share the same authenticated Go read model used by REST and MCP. A completed Feudal-age Market is required to post offers, accept deliveries and exchange resources. Reading the book is available before building one.

## Standing offers

An offer describes what its author gives and wants **per lot**. Offer wood for gold to sell, gold for wood to buy, or wood for stone to barter. Both quantities must be whole numbers from 1 to 500; the resources must differ. Post 1–20 lots, with at most 12 open offers per kingdom. Offers stay open until claimed, cancelled, or invalidated by the offering Market's loss or the kingdom's defeat.

Posting removes all advertised goods from spendable stock and reserves them. Accepting removes one lot from the listing and assigns its reserved goods to the delivery. Cancelling returns only unclaimed lots; accepted deliveries continue. The author cannot change prices under an accepted caravan. Cancel and post a new offer to advertise different terms.

`target_player: 0` publishes terms to all kingdoms. A positive target makes the offer visible only to the author and that kingdom. All offers disclose terms and remaining lots, not the author's stockpile or production. Listings do not reveal unexplored Market locations. The accepting player must scout the offering Market first; previously observed Markets can be used from fog memory. A new post is an explicit disclosure of the advertised terms and availability.

## Physical delivery

The accepting player supplies an idle, empty Trade Cart beside a completed owned Market. The Markets must be connected by land, and the kingdoms must be at peace. The cart carries one lot, up to 500 of either resource on its respective journey.

1. Acceptance loads the requested payment from the accepting kingdom's stockpile into its cart. The offered goods remain reserved at the offering kingdom.
2. At the offering Market, the payment enters that kingdom's stockpile and the advertised goods replace it in the cart.
3. The cart returns to its home Market, where the purchased goods enter the accepting kingdom's stockpile. If that Market was lost or became unreachable, another reachable owned Market can receive them.

**Repeat trips** attempts the next lot through the same command boundary after delivery commits. It stops when the offer closes or a new acceptance is refused, including insufficient payment. An offer can be filled by multiple kingdoms; each claimed lot needs its own available cart. Separate posted offers are not automatically matched against one another.

Movement uses normal collision, pathfinding and unit speed. Peaceful trading grants the assigned caravan passage through the offering kingdom's gates; military units receive no new passage rights. This does not share vision. Blocked paths retry normally. Build/open a route, move the cart, or recall it as appropriate. Isolated islands require future naval freight; a land cart cannot cross deep water.

| Interruption | Result |
|---|---|
| Stop or move the cart | Pauses the delivery, retaining cargo. Resume continues without a second charge. Other cargo/trade/garrison orders are refused while it has an active shipment. |
| Recall before pickup | Releases the offering kingdom's reserved goods immediately. Payment returns physically with the cart. |
| Offering Market lost, offering kingdom defeated, or conflict begins before pickup | Automatically recalls the delivery. |
| Home Market lost | Returns to another reachable owned Market, or waits for one. |
| Cart destroyed, converted, or accepting kingdom defeated | Undelivered cargo is lost. Offered goods that were never collected are released. Already delivered payment is retained by its recipient. |
| Pause/save/restart | Retains orders, cargo, reservations and lifecycle state; time stops while paused. Restore never replays a payment or refund. |

Participants see their agreed terms and delivery milestones. Only the cart's owner sees interruption and route details outside ordinary map visibility. A foreign cart's location is included only while visible, including for its trading partner. Third parties do not receive delivery records. Trade receipts remain in each participant's private chronicle. The read model retains recent deliveries (200 records globally, plus older active deliveries); the complete journal remains stored.

## Finite merchants

All owned Markets access one shared merchant inventory. It initially contains 1,000 food, 1,000 wood, 1,000 stone and 2,000 gold. It never automatically restocks. Player purchases replenish merchant gold, and player sales replenish the purchased commodity. Merchants refuse sales that would take that commodity above 5,000 and refuse exchanges they cannot fund.

Each exchange trades a lot of 100 food, wood or stone against gold. Initial quotes are 130 gold to buy and 70 gold to sell (84 gold for Saracens). Buying reduces commodity stock and raises subsequent prices; selling increases stock and lowers them. Quote calculation, rounding, stock checks and execution belong to Go. The selected Market's actions and `marketplace.merchants.actions` supply the exact `cost`, `gain`, `enabled` and refusal `reason`.

Pass `market_revision` from the displayed merchant view to require that quote. A changed stock revision returns `market_changed`; reread and submit a new intention. Omission accepts the current execution-time quote, preserving the existing command contract. Neutral-Market trade routes withdraw their distance-based gold from this same finite treasury and wait when it is empty. Rebuilding Markets does not reset stock or prices.

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

Every new intention needs a unique command ID. Retry an ambiguous request with the identical ID and payload. The membership-scoped receipt boundary prevents duplicate reservations, cargo loading, payments and refunds. Generated TypeScript, OpenAPI and MCP schemas all derive from the Go wire types.

Built-in AI uses only its own inventory and the book it is allowed to read. It advertises surpluses against shortages and can train/fund carts to accept useful, observed offers. It receives no extra resources or private trading information. Negotiated alliances, naval freight and automatic matching of separate offers remain future work.
