# AI of Empires — Browser RTS Specification

Version: 1.0  
Date: 2026-09-06  
Deliverable: product, gameplay, content, and technical specification  
Working title: AI of Empires

## 1. Product definition and reference

Build a full browser-based historical real-time strategy game with the gameplay mechanisms, economic decisions, military counters, settlement development, and four-age progression of **Age of Empires II: Definitive Edition**. A player begins with a small settlement, explores an unknown map, gathers resources, advances technologically, builds an army, and defeats rival civilizations. The game must support both deliberate empire building and demanding competitive play.

Age of Empires II is the assumed reference because the request names the series without specifying an installment. This specification uses its medieval ruleset consistently; it does not mix in Age of Empires I's ancient ages, Age of Empires III's shipment system, or Age of Empires IV's landmark advancement. The intended setting spans roughly 500–1600 CE across Europe, North Africa, the Middle East, and Asia. Dates guide content and art rather than restricting random-map matchups.

“Exactly like” means **mechanical fidelity**, including the small interactions that affect strategy: carrying and depositing resources, finite deposits, worker travel, population blocks, production queues, attack delays, minimum range, armor classes, projectiles, conversion, walls, and civilization-specific technology restrictions. Similar-looking units attached to a simplified economy would not meet the requirement.

The game will have its own title, code, artwork, interface treatment, sound, dialogue, maps, and campaign scripts. Historical names and familiar functional terms such as Villager, Town Center, Castle Age, and Trebuchet keep the rules understandable.

### 1.1 Reference and precision policy

The official learning materials establish the reference's broad structure: economic expansion, technology research, military development, and four ages. [Official getting-started guide](https://www.ageofempires.com/learn-to-play/getting-started-aoe2/).

For reproducible numerical reference, use the medieval technology-tree dataset at **SiegeEngineers/aoe2techtree commit `b9d494df6921d4080df69b22f9dbb7a4d1dcd9f0`**, specifically its root `data/` directory. This is a fixed reference snapshot, not a claim that it represents the newest official game build. The costs and times in §§6–8 were checked against that snapshot. [Pinned reference data](https://github.com/SiegeEngineers/aoe2techtree/blob/b9d494df6921d4080df69b22f9dbb7a4d1dcd9f0/data/data.json).

Three kinds of requirement appear below:

- **Reference mechanics:** reproduce the AoE II behavior; do not substitute a different strategic rule for implementation convenience.
- **Reference data:** reproduce the selected units, technologies, civilization availability, and numerical attributes from the pinned snapshot, with explicit provenance.
- **Product decisions:** the browser architecture, performance budgets, launch content quantity, original campaigns, matchmaking operations, and learning tools specified here.

The technology-tree dataset is not a complete simulation specification. It does not by itself establish pathfinding, all economic rates, collision behavior, conversion probability, or every special interaction. During implementation, record reproducible reference observations for these behaviors in a conformance ledger. A field with no verified reference must be marked unverified, never silently assigned an invented “exact” value. Reference observations must identify the actual game build used. Where that build conflicts with the pinned data, preserve the conflict and resolve it explicitly in a versioned ruleset change.

This document specifies the game to build. It does not claim that a game or a mechanically identical engine already exists.

### 1.2 Definition of the complete product

The complete release includes the full land and naval economy, all four ages, the shared military and technology systems, 13 fully differentiated civilizations, procedural maps, skirmish AI, campaigns, competitive multiplayer, custom lobbies, saves, replays, spectating, and a scenario editor. Intermediate milestones in §21 are implementation stages, not replacements for this scope.

The 13-civilization launch catalog recreates the breadth of the classic medieval roster. Reproducing every later expansion civilization or every original campaign is not assumed by this specification. Expansion mechanisms must fit the same extensible rules engine; adding a civilization must not require a new game loop.

## 2. Player experience and match contract

### 2.1 Design pillars

1. **Economy creates military power.** Every worker assignment, building, and technology has an opportunity cost.
2. **Information creates advantage.** Scouting reveals resource locations, enemy plans, and routes for attack.
3. **Counters reward adaptation.** Army composition, positioning, upgrades, and control matter together.
4. **Ages change available strategies.** Advancement enables new options while temporarily competing with immediate production.
5. **Civilizations create different plans.** Shared foundations make them learnable; bonuses and missing technologies make them distinct.
6. **The browser supports a complete RTS.** It must preserve army scale, manual control, long matches, and tactical responsiveness.

### 2.2 Standard random-map settings

| Setting | Default |
|---|---|
| Players | 2–8, human and/or AI; solo practice permits one player |
| Teams | 1v1, 2v2, 3v3, 4v4, or free-for-all |
| Starting age | Dark Age |
| Maximum age | Imperial Age |
| Starting resources | 200 food, 200 wood, 100 gold, 200 stone, before civilization modifiers |
| Starting assets | One Town Center, three Villagers, one Scout Cavalry, before civilization/map modifiers |
| Population limit | 200 per player; custom options 25–500 |
| Starting housing | Town Center supplies five population capacity |
| Visibility | Unexplored map, line of sight, and fog of war |
| Victory | Conquest in ranked; configurable in custom games |
| Game speed | Normal: 1.7 simulation seconds per real second |
| Expected duration | Usually 20–60 real minutes; no forced match timeout |
| Team diplomacy | Locked in ranked; configurable in custom games |

These starting settings are the intended reference baseline and must be included in conformance testing. Civilization exceptions apply before the first player command. A settings panel must expose the actual effective start rather than displaying generic values for an exceptional civilization.

Unless explicitly labeled “real,” durations elsewhere in this document are **simulation seconds**. Match timers and build orders display simulation time. Network deadlines, loading budgets, and reconnect windows use real time.

### 2.3 Representative match flow

| Phase | Player decisions | Required gameplay |
|---|---|---|
| Opening settlement | Villager queue, houses, scouting, safe food | Herdables, hunting, berries, lumber, early scouting and worker vulnerability |
| Early expansion | More economy or early pressure | Feudal units, defensive walls/towers, resource denial, counter units |
| Kingdom building | Add Town Centers, attack, or advance | Knights, monks, siege, castles, relics, trade, wider map control |
| Imperial warfare | Choose final upgrades and attack fronts | Trebuchets, gunpowder, elite units, expensive technology commitments |
| Resource exhaustion | Protect remaining resources and recurring income | Farms, trade, relics, market exchange, wood/food counter armies |

Players can rush, defend, boom, fast-advance, raid, siege, or contest water. None of these strategies is forced by a scripted sequence. A lost Town Center is recoverable if the player retains a viable economy or allies.

## 3. World, terrain, and visibility

Use a continuous movement world over a square logical terrain grid, presented through a fixed isometric camera. World tiles, entity positions, and rendering pixels must be separate coordinate systems. Terrain height affects rendering and combat where the reference allows it.

Required terrain includes grassland, dirt, desert, snow, shallow water, deep water, shore, forests, hills, cliffs, and crossings. Forests and buildings block movement; cutting trees or destroying structures can open routes. Water requires naval movement or transport. Visual terrain variations must not introduce unlisted movement or gathering bonuses.

### 3.1 Exploration and fog

- Unexplored cells conceal terrain, resources, and enemies.
- Explored but unseen cells retain terrain and last-observed stationary information; moving enemies disappear.
- Visible cells update entities, resource depletion, attacks, and ownership.
- Enemy buildings remembered under fog show their last known state, not their current hidden construction, destruction, health, or production.
- Units provide line of sight even while idle, subject to garrison and transport rules.
- Shared allied vision follows the reference ruleset and relevant technology. Team membership alone must not accidentally bypass its unlock conditions.
- Fog applies consistently to the main view, minimap, tooltips, selection, sounds, command errors, and network payloads.
- Scouting technology increases only the properties defined by its effect; it must not reveal the map wholesale.

### 3.2 Resources and wildlife

Maps contain trees, gold deposits, stone deposits, forage bushes, herdable animals, huntable animals, hostile wildlife, shore fish, and deep-water fish. Wildlife supports aggression, fleeing, pursuit limits, and ownership changes where applicable. Players can scout with herdables, lure dangerous hunt toward a Town Center, and push fleeing animals. Carcasses decay according to the reference; killed food must not become an inexhaustible deposit.

Resource collision shapes and accessible gathering slots matter. A deposit behind a forest cannot be harvested through the forest. Depleted resources update pathing, visibility memory, and worker retargeting.

## 4. Economy and population

The reference uses food, wood, gold, and stone. Workers and the time required to collect and spend them are central to the economy. [Official resource and control guide](https://www.ageofempires.com/learn-to-play/control-resources-aoe2/).

### 4.1 Resource model

| Resource | Sources | Principal uses | Long-game constraint |
|---|---|---|---|
| Food | Herdables, hunting, forage, fish, farms | Villagers, many units, economic upgrades, age advancement | Natural food depletes; farms consume wood to renew |
| Wood | Trees | Buildings, farms, archers, ships, siege | Forest depletion expands travel and opens terrain |
| Gold | Mines, relics, allied trade, market exchange | Advanced units, technology, age advancement | Mines are finite; recurring income is strategically important |
| Stone | Deposits and market exchange | Castles, towers, stone fortifications, Town Centers | Scarcity limits fortification and expansion |

Gathering is an explicit work cycle: move to source → acquire a valid work position → gather into carried inventory → walk to a compatible drop-off → deposit → return. Stockpiles increase at deposit time. A worker killed while carrying resources loses the carried amount. Force-drop-off, reassignment, source depletion, and loss of a drop-off require deterministic transitions.

Town Centers accept all four resources. Mills accept food; lumber camps accept wood; mining camps accept gold and stone; docks accept appropriate naval economic cargo. Drop-off eligibility includes civilization-specific exceptions. Path distance, not straight-line distance alone, determines a usable destination.

Gathering rates, carrying capacities, decay rates, and work animations are distinct data fields. Technologies and civilization bonuses modify the relevant field. Do not collapse hunting, herding, farming, and foraging into a single uniform food rate.

### 4.2 Villager behaviors

Villagers can gather, construct, repair, attack, garrison, ungarrison, and perform queued tasks. They have health and can be raided or converted. A player can select idle workers and inspect worker counts by task. Task counts distinguish working, traveling, building, repairing, and idle states rather than counting every selected economic unit as productive.

Farm creation costs wood. Each farm has a finite food yield and normally one farmer. Reseeding reconstructs an exhausted farm with another wood payment. Optional automatic reseeding follows the reference's resource and queue behavior and can be disabled. An unaffordable reseed must leave a visible idle/exhausted state without granting food.

Fish traps provide a renewable naval food economy with construction/rebuild costs. Fishing ships remain distinct from trade and military vessels.

### 4.3 Population

All population-consuming entities use their declared population cost. Housing raises current capacity up to the configured cap. Houses supply five capacity by default; Town Centers and castles supply capacity according to their reference data.

Queueing pays the cost immediately; housing availability controls production according to the reference queue rules. Units cannot be granted for free or spawned beyond the applicable production cap. A population-blocked queue displays its reason and resumes when capacity is available. Losing housing does not kill existing units. Conversion and special civilization rules can create over-cap states; those states do not authorize unrestricted new training.

### 4.4 Market, trade, and tribute

- Exchange food, wood, and stone for gold and buy them with gold in discrete lots. Prices respond to buying and selling; fees and price floors/ceilings are ruleset data. Display a quote before the order and the authoritative price on execution.
- Market pricing scope, per-lot rounding, simultaneous order ordering, and technology discounts must match the reference. Prices must not differ merely because clients receive events at different times.
- Trade carts travel between the owner's market and another eligible player's market; trade cogs use docks. Income depends on the actual reference distance and map-size rules. Own-to-own routes cannot create gold.
- Trade needs travel and delivery. Blocked routes, destroyed destinations, and killed traders interrupt income. Switching endpoints cannot repeatedly redeem the same cargo.
- Relics generate gold only while properly deposited in monasteries. Their rate and civilization modifiers are data, independent from mining upgrades.
- Players may send resource tribute with the applicable fee. Transport and trade technologies reduce the specified costs; tribute is recorded for postgame analysis.

## 5. Construction, repair, and production

### 5.1 Placement and building lifecycle

Villagers place grid-aligned foundations on valid terrain. Placement checks footprint, slope, obstructions, terrain class, exploration, and sufficient resources. Docks straddle valid shoreline. Walls and gates support drag placement with previewed cost and gaps.

Pay the building cost when the foundation is accepted. Construction requires a worker at a valid position. Foundations have construction progress, hit points, collision behavior, and cancellation value. Multiple builders accelerate construction using the reference scaling curve, not a freely chosen linear multiplier.

Buildings become operational only at the required completion point. Destroying or deleting a foundation, canceling a building, and canceling a queue use distinct reference refund rules. Partial construction cannot generate positive resource loops. Completed buildings can be deleted by their owner with a deliberate command; selection alone must never delete them.

Repair consumes worker time and the specified resources. Repair rate, cost, and limits differ where required for buildings, ships, and siege. Workers stop or visibly stall when resources are exhausted. Repair cannot revive a destroyed entity.

### 5.2 Queue rules

Each producer has an ordered queue with one active task unless a reference exception applies. Unit training and technology research compete for its production time. A Town Center researching the next age therefore stops training Villagers in that queue.

Queues support multiple items, batches, cancellation, reorder where supported by the chosen control model, rally points, and clear blocked-state indicators. A global technology cannot be researched twice through two producers. Cancellation and simultaneous commands are resolved by the authoritative simulation.

Completed units appear at valid exit positions. A blocked exit cannot discard a paid unit, teleport it through walls, or duplicate it; preserve the completion state until a legal release is possible. Destroyed producers dispose of their queue and refunds according to the reference behavior. Garrisoned or transported units retain their real population usage.

## 6. Ages and technology

### 6.1 Age advancement

| Age entered | Cost | Base research time | Standard completed prerequisites | Strategic change |
|---|---|---:|---|---|
| Dark Age | Starting age | — | Starting settlement | Villagers, scouting, basic economy, militia |
| Feudal Age | 500 food | 130 s | Two distinct eligible Dark Age building types | Archers, skirmishers, scouts, spears, towers, market, blacksmith |
| Castle Age | 800 food, 200 gold | 160 s | Two distinct eligible Feudal Age building types | Additional Town Centers, knights, monks, castles, workshop siege |
| Imperial Age | 1,000 food, 800 gold | 190 s | Two distinct eligible Castle Age building types, or a castle | Final unit upgrades, trebuchets, advanced siege and gunpowder |

Age costs and times above are reference data. The age names in this table are player-facing names; internal names in historical datasets can differ. [Pinned technology records](https://github.com/SiegeEngineers/aoe2techtree/blob/b9d494df6921d4080df69b22f9dbb7a4d1dcd9f0/data/data.json).

Standard eligible prerequisite sets are: Dark—mill, lumber camp, mining camp, dock, barracks; Feudal—archery range, stable, blacksmith, market; Castle—monastery, university, siege workshop, with castle as the alternative. Two buildings of the same type do not satisfy a two-type requirement. Houses, farms, walls, and towers do not substitute for these sets. Civilization exceptions must be represented explicitly.

Research occurs in a Town Center and applies to the player globally on completion. Losing unrelated prerequisite buildings after research begins follows the verified reference behavior; implement it as a fixture rather than inferring it from the placement checks. Losing the researching Town Center interrupts its work under the queue rules.

Advancement changes the available technology tree and architectural appearance. It does not automatically grant every newly available military upgrade. Existing units only receive changes from effects actually awarded by the age or civilization.

### 6.2 Technology families

| Research location | Required technology families |
|---|---|
| Town Center | Villager protection, carrying/movement efficiency, age advancement, relevant vision/team information technologies |
| Mill | Farm-yield upgrades, associated farming improvements |
| Lumber camp | Successive wood-gathering upgrades |
| Mining camp | Gold and stone gathering upgrades |
| Blacksmith | Infantry and cavalry attack; infantry armor; cavalry armor; archer attack/range; archer armor |
| Barracks | Infantry line upgrades and infantry-specific improvements |
| Archery range | Archer/skirmisher/cavalry archer upgrades and accuracy/firing improvements |
| Stable | Scout, knight, and camel line upgrades; movement and cavalry health technologies |
| Siege workshop | Ram, mangonel, and scorpion line upgrades |
| University | Masonry, architecture, ballistics, chemistry, siege engineering, tower/wall and defensive improvements |
| Monastery | Conversion reach/eligibility/resistance, faith recovery, healing, movement, monk survival and special conversion rules |
| Market | Trade movement, tribute efficiency, market fees |
| Dock | Fishing efficiency, naval upgrades, ship armor/movement/cost and transport capacity |
| Castle | Unique unit elite upgrade, unique technologies, training efficiency, relevant intelligence technologies |

The complete availability and upgrade graph for each launch civilization is defined by its entry in the pinned data and supporting verified effect records. The table summarizes families; it does not authorize omitting individual nodes. Technologies such as Ballistics and Chemistry must change their real mechanical properties rather than being generic damage upgrades.

Technologies have an ID, cost, research time, producer, minimum age, prerequisites, effect list, civilization availability, and repeatability policy. Effects apply to existing and future eligible entities according to the reference, exactly once. Upgrade completion must preserve appropriate health proportions, carried cargo, orders, ownership, and garrison state.

The technology-tree screen shows available, completed, locked, and permanently unavailable nodes. Selecting a node explains its cost, time, prerequisite path, and exact effect. A missing civilization technology is visibly unavailable rather than absent without explanation.

## 7. Buildings

The following is the shared building catalog; civilization restrictions still apply. Costs are in food (F), wood (W), gold (G), and stone (S). Construction times are for one unmodified builder and are simulation seconds.

| Building | Earliest standard age | Cost | Time | Function |
|---|---|---|---:|---|
| Town Center | Dark; additional centers normally Castle | 275 W, 100 S | 100 | Villagers, ages, drop-off, housing, garrison defense |
| House | Dark | 25 W | 25 | Five population capacity |
| Mill | Dark | 100 W | 35 | Food drop-off and farm technologies |
| Lumber camp | Dark | 100 W | 35 | Wood drop-off and technologies |
| Mining camp | Dark | 100 W | 35 | Gold/stone drop-off and technologies |
| Farm | Dark | 60 W | 15 | Finite renewable food source |
| Barracks | Dark | 175 W | 50 | Infantry and infantry technologies |
| Dock | Dark | 150 W | 35 | Naval economy, military, and technologies |
| Palisade wall segment | Dark | 3 W | 7 | Early blocking fortification |
| Archery range | Feudal | 175 W | 50 | Ranged units and technologies |
| Stable | Feudal | 175 W | 50 | Mounted units and technologies |
| Blacksmith | Feudal | 150 W | 40 | Military equipment upgrades |
| Market | Feudal | 175 W | 60 | Exchange, trade, trade technologies |
| Stone wall segment | Feudal | 5 S | 10 | Durable blocking fortification |
| Stone gate | Feudal | 30 S | 70 | Controlled passage through fortifications |
| Watch tower | Feudal | 35 W, 125 S | 80 | Static ranged defense |
| Siege workshop | Castle | 200 W | 40 | Mobile siege weapons |
| Monastery | Castle | 175 W | 40 | Monks, relic storage, religious technologies |
| University | Castle | 200 W | 60 | Advanced military/building technologies |
| Castle | Castle | 650 S | 200 | Major defense, unique units, trebuchets, unique technologies |
| Wonder | Imperial | 1,000 W, 1,000 G, 1,000 S | 3,500 | Timed victory in enabled modes |

These figures are selected records from the pinned reference data. The complete catalog also includes palisade gates, outposts, fish traps, upgraded walls, guard towers, keeps, and bombard towers, with their unlocks and costs defined by the same ruleset. [Pinned building records](https://github.com/SiegeEngineers/aoe2techtree/blob/b9d494df6921d4080df69b22f9dbb7a4d1dcd9f0/data/data.json).

Gates automatically permit eligible friendly passage and can be locked. Enemy units cannot use an allied opening unless the physical opening and reference rules permit it. Wall segments join visually without changing their collision footprint. Defensive buildings use their own range, minimum range, projectile, upgrade, and garrison-arrow calculations. An empty Town Center must not receive the firing power of a fully staffed one.

## 8. Units, combat, and army control

The reference separates infantry, cavalry, ranged units, and siege into dedicated production buildings; its siege includes long-range trebuchets and other specialized weapons. [Official military guide](https://www.ageofempires.com/learn-to-play/military-and-economy-aoe2/).

### 8.1 Shared unit lines

| Family | Progression / variants | Role and counter relationships |
|---|---|---|
| Worker | Villager | Economy and construction; vulnerable to raiding |
| Militia infantry | Militia → Man-at-Arms → Long Swordsman → Two-Handed Swordsman → Champion | Sustained infantry pressure; threatens many low-gold counters and buildings |
| Anti-cavalry infantry | Spearman → Pikeman → Halberdier | Cost-efficient cavalry counter; vulnerable to ranged attacks and suitable infantry |
| Foot archer | Archer → Crossbowman → Arbalester | Ranged damage; countered by skirmishers, siege, and successful cavalry engagements |
| Skirmisher | Skirmisher → Elite Skirmisher | Anti-archer specialist with minimum range; weak against many melee units |
| Light cavalry | Scout Cavalry → Light Cavalry → Hussar | Scouting, raiding, monk pressure; vulnerable to spear units |
| Heavy cavalry | Knight → Cavalier → Paladin | Durable, mobile power; answered by spears, camels, monks, and resource-efficient counters |
| Camel | Camel Rider → Heavy Camel Rider | Mounted anti-cavalry unit with its own armor classes |
| Cavalry archer | Cavalry Archer → Heavy Cavalry Archer | Mobile ranged pressure; needs upgrades and space |
| Gunpowder infantry | Hand Cannoneer | Anti-infantry ranged specialist with accuracy and firing delay |
| Religious unit | Monk | Heal, convert, collect and deposit relics |
| Ram | Battering Ram → Capped Ram → Siege Ram | Soaks arrows and attacks buildings; vulnerable to melee; eligible infantry garrison effects |
| Stone-throwing siege | Mangonel → Onager → Siege Onager | Area damage against masses; minimum range, friendly-fire and tree-clearing interactions |
| Bolt siege | Scorpion → Heavy Scorpion | Piercing damage through formations |
| Long-range siege | Trebuchet | Must deploy to attack; strong against fortified positions |
| Cannon siege | Bombard Cannon | Mobile artillery for siege duels and building attacks |
| Demolition infantry | Petard | One-use attack against structures |
| Siege transport | Siege Tower | Carries eligible infantry and supports crossing enemy walls |
| Civilization units | Unique standard and elite forms | Specialized roles and explicit counter profiles |

Unlock ages and available terminal upgrades differ by civilization. A civilization with no Paladin cannot acquire it merely by reaching Imperial Age. Unit names in this table identify reference functions; original display names may be used if tooltips preserve clarity.

### 8.2 Selected numerical anchors

These are base creation costs and times from the pinned snapshot. Civilization bonuses and research can modify them. They are not costs remembered from a different patch.

| Unit | Cost | Train time |
|---|---|---:|
| Villager | 50 F | 25 s |
| Scout Cavalry | 80 F | 30 s |
| Militia | 50 F, 20 G | 21 s |
| Spearman | 35 F, 25 W | 22 s |
| Archer | 25 W, 45 G | 35 s |
| Skirmisher | 25 F, 35 W | 26 s |
| Knight | 60 F, 75 G | 30 s |
| Camel Rider | 55 F, 60 G | 22 s |
| Monk | 100 G | 51 s |
| Hand Cannoneer | 45 F, 50 G | 34 s |
| Mangonel | 160 W, 135 G | 46 s |
| Scorpion | 75 W, 75 G | 30 s |
| Trebuchet | 200 W, 200 G | 50 s |
| Bombard Cannon | 225 W, 225 G | 56 s |
| Petard | 65 F, 20 G | 25 s |
| Fishing Ship | 75 W | 40 s |
| Transport Ship | 125 W | 46 s |
| Trade Cog | 100 W, 50 G | 36 s |
| Galley | 90 W, 30 G | 45 s |

[Pinned unit records](https://github.com/SiegeEngineers/aoe2techtree/blob/b9d494df6921d4080df69b22f9dbb7a4d1dcd9f0/data/data.json).

### 8.3 Damage and attack resolution

Every combat entity defines hit points, attack types, attack values by armor class, armor values by class, reload period, attack delay, attack range, minimum range, sight, movement speed, collision size, and target eligibility. Ranged entities also define projectile speed, accuracy, tracking behavior, splash area, and pass-through behavior where applicable.

Do not implement a single global rock-paper-scissors multiplier. Resolve damage through the reference attack/armor-class matching rules, applicable bonus damage, elevation modifiers, and minimum-damage rule. Store the full class arrays even when the interface shows only melee and pierce armor. Camel, cavalry, archer, siege, building, and unique-unit classifications can overlap.

Attack resolution must reproduce these interactions:

- Wind-up and projectile release are separate from reload. Orders issued before release can cancel an attack where allowed; already released projectiles continue under their own rules.
- Projectile travel and accuracy allow moving targets to evade attacks. Tracking technology changes aim behavior; it does not make every weapon an instant hit.
- Units cannot fire within a weapon's minimum range unless an applicable technology removes it.
- Hill advantage, attack bonuses, armor upgrades, splash, and friendly fire are evaluated in the correct order.
- Area effects use world-space geometry and deterministic inclusion rules. Graphics quality cannot alter which entities are hit.
- Attack-ground commands work for eligible siege and may hit friendly units where the reference permits it.
- Target death, conversion, garrison entry, and loss of vision have explicit consequences for in-flight projectiles and queued attacks.
- Melee reach, collision, and engagement slots control how many units can surround a target.

Combat conformance fixtures must include both single exchanges and repeated attacks; identical damage with an incorrect reload period is still a mismatch.

### 8.4 Orders, stances, and formations

Required commands: move, attack target, attack-move, stop, patrol, guard, follow, garrison, unload, rally, attack ground where supported, pack/unpack, and delete. Shift queues orders. Control groups retain living members through upgrades; removed or converted units leave groups appropriately.

Support aggressive, defensive, stand-ground, and no-attack stances with distinct acquisition and pursuit behavior. Support line, box, staggered, and flank formations. Formation spacing, travel speed, regrouping, and narrow-passage behavior must be observable and tested. Mixed armies must not permanently stall because one unit cannot reach its assigned slot.

The default is return fire, with bounded pursuit of actual attackers of the unit or nearby friends. Mere proximity to another settlement must not start combat. Aggressive stance and explicit attack orders are deliberate choices to initiate combat. Stand-ground retaliates without pursuit; no-attack suppresses automatic retaliation. Idle workers defend only themselves. The current ruleset implements these firing policies; formations remain release scope.

Units require global pathfinding, local avoidance, obstacle updates, and recovery from congestion. Repath work must be bounded without making units ignore collisions. Walling a moving army creates a real obstruction. Destroying a wall or cutting a tree invalidates affected paths promptly.

### 8.5 Monks and relics

Monks can heal eligible allied units, attempt conversion of eligible enemy entities, and carry one relic. Conversion uses the reference eligibility, range, attempt timing, randomness, resistance, and faith recovery rules. Model it as a state machine with a seeded random stream. It must not become guaranteed after an arbitrary universal channel duration.

Conversion transfers ownership, population accounting, command authority, and eligible state. Buildings and siege require the relevant technology where applicable. Certain units are resistant or immune. Multiple monks targeting one unit must not duplicate the conversion or produce multiple ownership changes in one resolution.

Relics are world entities. A carrying monk's death drops its relic. A monastery's destruction releases stored relics. A carrying monk's conversion handles its relic atomically. Relic income and victory ownership update on actual deposit/removal, not proximity.

### 8.6 Transport and garrison edge cases

Capacity is measured using declared passenger size. Embark/disembark requires valid reachable locations. Transport destruction kills or handles passengers according to the reference; it cannot strand live units at invalid water coordinates. Rams, siege towers, Town Centers, castles, and transports have different passenger eligibility and effects. Garrison healing, firing contribution, movement bonuses, and evacuation must be modeled separately.

## 9. Naval gameplay

Water is a complete strategic layer, available in the main release.

| Naval family | Required progression / behavior |
|---|---|
| Fishing Ship | Gather fish, deposit food, work fish traps |
| Trade Cog | Travel between eligible docks for gold |
| Transport Ship | Load, carry, and unload land armies |
| Galley | Galley → War Galley → Galleon; ranged fleet backbone |
| Fire ship | Fire Galley → Fire Ship → Fast Fire Ship; close-range naval combat |
| Demolition ship | Demolition Raft → Demolition Ship → Heavy Demolition Ship; consumable area damage |
| Cannon ship | Cannon Galleon → Elite Cannon Galleon where available; bombard coastal fortifications |
| Unique vessel | Civilization-specific naval units, including the Viking longboat role |

Naval counters depend on range, speed, formation, splash, and investment. Shoreline towers and castles can contest water within real range. Dock placement, fish distribution, landing sites, and trade routes must matter. A map generator must never create a mandatory landing objective with no valid unloading location.

## 10. Civilizations and historical identity

Launch with **13 civilizations**. Each has its own full technology availability, economic and military bonuses, team bonus, unique unit, applicable unique technologies, architecture association, language treatment, and AI preferences. These entries identify required strategic identities; numerical effects come from the pinned reference and verified effect records rather than the summaries below.

| Civilization | Strategic identity to preserve | Distinctive unit role |
|---|---|---|
| Britons | Foot archery, range, and supporting economy | Long-range bow infantry |
| Franks | Heavy cavalry and castle-supported pressure | Melee-damage throwing infantry |
| Goths | Infantry volume and production pressure | Infantry resistant to arrow fire |
| Teutons | Durable infantry, defensive and siege play | Heavily armored slow infantry |
| Japanese | Infantry combat and fishing economy | Infantry specialized against unique units |
| Chinese | Flexible technology access, distinctive opening, ranged play | Multi-projectile crossbow infantry |
| Byzantines | Defensive durability, cost-efficient counters, flexibility | Cavalry with anti-infantry specialization |
| Persians | Town Center economy and heavy mounted warfare | Powerful but expensive war elephant |
| Saracens | Market flexibility, camels, and mixed armies | Ranged anti-cavalry mounted unit |
| Turks | Gunpowder, gold dependence, and particular counter-unit limitations | Long-range gunpowder infantry |
| Vikings | Infantry, economic efficiency, and naval strength | Regenerating infantry and multi-arrow warship |
| Mongols | Hunting opening, mobile archery, and siege | Mounted archer with anti-siege capability |
| Celts | Infantry movement, wood economy, and siege pressure | Fast raiding infantry |

Civilization differences must affect actual legal actions and statistics. A portrait, color, and single cosmetic unit do not constitute a civilization. Team bonuses follow the reference's recipients, activation conditions, and stacking policy. Converted units, allied producers, and team-access exceptions require explicit tests.

Civilization selection includes random and mirrored choices, a concise strengths/limitations summary, and the complete technology tree. All launch civilizations are usable in skirmish and multiplayer from the start. Historical campaign progress does not unlock competitive power.

Historical presentation uses distinguishable architecture families, materials, equipment, and voices. Each civilization must retain identifiable units and buildings across all ages while preserving shared functional silhouettes. Campaign texts distinguish documented events from invented connective scenes.

Additional civilizations may be added as content packs. The engine must support data-defined exceptional starts, free technologies, resource substitutions, special production buildings, alternative housing rules, auras, charge attacks, regeneration, and transformed units when their own rules and conformance fixtures are supplied. These extensions must not change the base rules for unrelated civilizations.

## 11. Game modes, diplomacy, and victory

### 11.1 Required modes

| Mode | Required configuration |
|---|---|
| Tutorial | Guided economy, scouting, ages, combat, siege, naval and combined-arms lessons |
| Skirmish | Any launch civilization versus configurable AI, with save/load |
| Ranked random map | 1v1 and team queues, fixed ruleset, balanced map pool, Conquest |
| Custom random map | Public/private lobbies, human/AI slots, teams, map seed and settings |
| Deathmatch | Large reference starting stockpiles and advanced military pacing |
| Regicide | King per player; king death defeats that player |
| Wonder Race | Economy-focused Wonder objective with explicitly displayed rules |
| King of the Hill | Hold a neutral objective for the configured duration |
| Campaign | Original scripted historical missions, difficulty levels, persistence |
| Scenario | Editor-authored objectives, triggers, units, terrain and diplomacy |

Mode presets own their starting assets, resources, age, enabled victory conditions, and technology state. A Deathmatch preset must not accidentally retain standard random-map stockpiles. Exact reference presets are part of the conformance data; custom overrides are displayed before players ready up.

### 11.2 Victory semantics

- **Conquest:** eliminate or obtain the resignation of all opponents. Defeat detection uses the reference's relevant-entity rules rather than requiring destruction of every wall segment. A recoverable player is not defeated solely for losing their Town Center.
- **Standard victory:** enable Conquest plus reference Wonder and relic hold conditions. A completed Wonder starts its announced countdown; its destruction cancels that countdown. Required relic control starts the relic countdown; losing control cancels it. Countdown length and team aggregation come from mode data.
- **Regicide:** the king's death triggers defeat even if the player retains a large army. Kings can use eligible transport and garrison systems.
- **Wonder Race / Hill:** expose objective, ownership, timer, interruption/reset behavior, and win event clearly before the match begins.
- **Scenario:** named trigger objectives determine victory and defeat; they can override normal elimination when the mission requires it.

Victory checks occur in a defined end-of-tick phase after damage, conversion, death, and objective ownership resolve. Simultaneous opposing terminal victories produce a draw unless that mode supplies an explicit precedence rule. This tie policy is a product decision and must be recorded in match metadata.

Custom diplomacy supports ally, neutral, and enemy relationships, locked teams, allied victory, shared vision rules, and tribute. Diplomatic changes must update attack permissions, trade eligibility, gate passage, and objective aggregation atomically. Ranked teams remain fixed.

Peaceful coexistence is a valid outcome for independent settlements. The current ruleset starts kingdom pairs at peace, records actual attacks and conversion attempts as conflict, and restores peace after five game minutes without attacks. This relationship lifecycle persists with each session. Formal treaty negotiation, alliances, shared vision, and tribute remain release scope.

## 12. Maps and procedural generation

### 12.1 Launch map catalog

Create eight original map scripts covering these layouts:

1. **Open Plains:** open land, distributed woods and exposed secondary resources.
2. **Walled Basin:** enclosed starting settlements and a contested outer economy.
3. **Forest Marches:** dense woods, chokepoints, and tree-cutting opportunities.
4. **River Kingdoms:** shared waterways, crossings, fishing and mixed warfare.
5. **Twin Seas:** land approaches with separate valuable fishing zones.
6. **Island Crowns:** separated starting islands, naval control and landings.
7. **Coastal Frontier:** continuous mainland with a significant shared coast.
8. **Highland Relics:** elevation, constrained approaches, and contested relics.

Map scripts choose logical size by player count: target 120×120 for 1v1, 168×168 for four players, 220×220 for six, and 240×240 for eight. These sizes are product defaults and may be tuned with performance and balance evidence. Custom size can be independent from player count.

### 12.2 Fairness and determinism

A map is generated from a seed, script version, ruleset version, player count, team arrangement, and size. The same inputs reproduce identical terrain, deposits, animals, relics, and starts. Terrain layout, decoration, wildlife, and AI must use separate seeded random streams.

For ranked scripts, validate before match allocation:

- Starting Town Centers and units have legal footprints and useful access to food, wood, gold, and stone.
- The script's intended land/water connectivity holds; required resources and objectives are reachable by the intended movement class.
- Equivalent player starts have comparable path distances and accessible resource quantities. Initial acceptance targets: quantities within 10% and primary resource path distances within 15% of the script's target, except documented intentional asymmetry.
- Team placement follows the selected arrangement; opponents cannot overlap or spawn inside each other's starting clearance.
- Relics and neutral resources offer contestable expansion opportunities.
- Every landlocked or island start is intentional for that script.

Reject invalid seeds using a deterministic retry sequence. After a bounded 100 attempts, fail match setup with a retryable explanation; never start an invalid ranked map. Validate at least 10,000 seeds per ranked script/player-count configuration before release.

## 13. AI, tutorials, and campaigns

### 13.1 Skirmish AI

AI issues the same commands as a human through the authoritative rules. It scouts, maintains villager production, allocates economy, advances ages, creates counter units, researches useful upgrades, builds defenses, expands, controls siege, trades with allies, collects relics, and handles naval maps.

Seven difficulty choices cover Peaceful practice, Easy, Standard, Hard, Extra hard, Expert, and an Aggressive raiding style. Difficulty varies planning competence, reaction time, attention budget, and tactical execution while preserving the same economic and production rules for humans and AI. Easy must provide breathing room through smaller military budgets, bounded raids, longer intervals, and less constant economic attention; merely waiting to assemble a larger army is not an acceptable implementation of Easy. The current ruleset implements paid economy/construction/research, scouting, bounded military budgets and raid rosters, defense, retreat/recovery, and safe expansion above Easy. Difficulty levels obey fog and resource accounting. Any future optional handicap that grants resources or knowledge must be explicitly labeled in lobby settings and excluded from ranked play.

Each civilization maximizes its own survival and development. Free-for-all AI kingdoms must evaluate all other kingdoms using the same criteria regardless of human or AI control; they are not an implicit team against the human. Target selection balances observed military risk, distance, economic benefit, and opportunity cost. Self-preservation can interrupt an offensive, and developing the economy is preferable to an unprofitable war. Knowledge comes from scouting and retained observations, not hidden armies, stockpiles, or undiscovered starting coordinates. Strategy lifecycles, campaign rosters, and timing commitments persist in checkpoints.

Settlements need not all pursue military dominance. Seeded Builder, Defensive, and Expansionist preferences vary economic and military priorities independently of difficulty. Builders grow and defend locally; defensive kingdoms may counterattack actual hostilities; expansionists may initiate profitable raids. Aggressive difficulty explicitly selects expansionist preferences. Routine proximity must not be mistaken for an attack, and a raid against one kingdom must not automatically attack unrelated peaceful kingdoms on its route. Preferences and relationships are visible in the kingdom panel and preserved in saves.

AI strategy modules include early infantry pressure, archers, scout raids, fast Castle, multi-Town-Center economy, castle pressure, naval opening, and late combined arms. Selection depends on civilization, map, scouting, and opponent behavior. AI must recover from blocked buildings, exhausted resources, lost production, and unsuccessful attacks.

### 13.2 Learning content

Provide ten short lessons: camera/selection, Villager production, gathering/drop-off, houses/buildings, scouting/animals, age advancement, counters/upgrades, monks/siege, naval economy/transport, and a complete practice match. Each has a visible objective, an explanation of why it matters, and a restartable exercise.

Optional practice tools show idle Town Center time, unspent resources, housing blocks, worker distribution, and a build-order timeline. Hints and scripted pauses are available in lessons and unranked practice. Ranked play retains manual economic and tactical decisions.

### 13.3 Original campaign catalog

Launch with three campaigns of five missions each:

| Campaign theme | Setting | Mechanical teaching arc |
|---|---|---|
| Border Kingdoms | Medieval western European frontier | Settlement defense → raids → allied support → contested expansion → major siege |
| Cities of the Crescent | Medieval eastern Mediterranean and neighboring trade routes | Caravan protection → city economy → relic/monk play → coastal warfare → multi-front campaign finale |
| Riders of the Steppe | Medieval Inner Asian expansion | Hunting/scouting → mobile warfare → resource denial → siege integration → large combined-arms battle |

These are original scenario premises, not claims about particular historical events. Before campaign production, each mission receives a documented historical setting, research bibliography, original script, objective graph, map brief, enemy behavior, optional objectives, difficulty changes, and success/failure conditions.

The campaign engine supports timed reinforcements, diplomacy changes, escort targets, named units, objective regions, dialogue, cinematics, technology restrictions, tribute checks, alternate paths, and persistent mission completion. Named characters use explicit scenario rules; standard competitive units do not gain unlisted hero abilities.

## 14. Interface, input, accessibility, and presentation

### 14.1 Match interface

Keep the map dominant. The HUD contains:

- Resource totals, population/capacity, age, match time, and idle-worker count.
- Selection details with health, armor, attack, range, ownership, and relevant bonuses.
- Contextual command grid showing enabled actions and explanations for unavailable ones.
- Local production queue and an optional global production panel.
- Minimap with terrain, visible entities, last-known information, pings, and camera bounds.
- Notifications for attack, idle economy, completed research, population blocks, objectives, and ally messages.
- Score/team panel, chat, diplomacy, technology tree, pause and settings access.

Notifications prioritize threats and collapse repeated events. Clicking an event moves the camera to its legitimate known location. A resource shortage says which resource and how much; an illegal building preview identifies the placement problem without disclosing hidden enemies.

### 14.2 Controls

Mouse and keyboard are the primary competitive input. Left-click selects; drag selects groups; shift modifies selection or queues orders; right-click issues a context-sensitive order. Double-click selects the same unit type in the current view. Support control groups 0–9, camera bookmarks, select-all-of-type commands, idle-worker cycling, producer cycling, and a remappable grid command layout.

Camera controls include edge scroll, keyboard pan, drag pan, zoom, minimap jump, and optional fullscreen. Prevent accidental browser scrolling while the match surface owns focus. Preserve normal typing and browser shortcuts when chat, text fields, or menus own focus. Touch devices may browse profiles and replays; competitive touch play is a separate future input project.

### 14.3 Accessibility

Provide scalable HUD/text, high-contrast selection, colorblind-friendly player palettes with non-color identifiers, subtitles, independent volume controls, reduced flashes, reduced motion, persistent tooltips, and fully remappable controls. Essential warnings use both visual and audio channels. Menu and lobby controls expose semantic labels and keyboard navigation. Avoid claiming that the spatial battlefield is fully screen-reader playable without a separately validated interaction design.

### 14.4 Art and audio direction

Use detailed original isometric sprites or pre-rendered models, readable silhouettes, earth-toned terrain, and strong ownership accents. Age advancement visibly develops structures from simple settlement materials to mature fortified architecture. Units show their line and upgrade tier clearly at normal zoom. Cosmetic skins preserve footprint, selection shape, and gameplay recognition.

Required animation states include idle, travel, work, attack, damage/death, construction, destruction, and relevant deployment/transport states. Audio covers commands, impacts, construction, gathering, alerts, ambient environments, naval action, and adaptive music. Hidden enemy sounds must not reveal exact positions beyond the reference's information rules. Match audio begins after an appropriate user gesture and recovers after device changes.

## 15. Browser runtime and simulation architecture

### 15.1 Platform target

Ship as an HTTPS web application that starts from a link without a native installer. Target desktop Chrome, Edge, Firefox, and Safari, testing their current stable and previous major releases at each product release. Require keyboard/mouse and a functioning WebGL 2 context for playable matches. Use runtime capability checks and a useful unsupported-device screen.

Use WebGL 2 for the initial battlefield renderer; it exposes a graphics context on an HTML canvas. WebGPU is an optional later renderer and cannot be a prerequisite for the core game. [MDN WebGL 2 reference](https://developer.mozilla.org/en-US/docs/Web/API/WebGL2RenderingContext).

### 15.2 Implementation decision: Go authority and Three.js presentation

The user's implementation decision supersedes the earlier proposed Rust/WASM architecture. Go owns every game mechanic and physics calculation in all modes. Three.js renders a thin browser UI served by the Go process. No authoritative browser simulation is permitted.

| Component | Responsibility |
|---|---|
| TypeScript application shell | Menus, HUD, settings, input, accessibility, and presentation state |
| Three.js / WebGL 2 renderer | Terrain, original geometry, animation, fog, selection, and overlays from server snapshots |
| Go simulation package | Fixed-step rules, physics, pathfinding, AI, economy, visibility, and victory |
| `open-ships/statemachine` instances | Typed lifecycle ownership with pure guards and named transition effects |
| Go match service | Serialized commands/ticks, sessions, authentication, idempotency, and clocks |
| Versioned HTTP API | Intent commands, placement queries, player-filtered snapshots, and reconnect; initial transport is REST plus SSE |
| Generated contracts | OpenAPI and TypeScript wire types derived from Go DTOs |
| Persistence services (future) | Accounts, metadata, campaign content, authoritative saves, and replays |

Every gameplay lifecycle uses the same state/event/guard/effect protocol, with separate owners for independent concerns. Ordinary calculations and command routing remain functions. The UI receives display metadata and available actions from the backend and never duplicates gameplay validation. See `docs/STATE_MACHINES.md` and `docs/API.md` for the implemented boundaries. `README.md` records current scope and gaps against this long-term specification.

### 15.3 Simulation contract

- Fixed simulation step: target 20 ticks per simulation second. At normal speed this requires 34 ticks per real second. Rendering is independent and interpolated.
- Use deterministic fixed-point state for positions, resource fractions, work progress, and gameplay timers. Preserve sub-tick event timestamps where needed for reference attack timings.
- Stable entity IDs, iteration order, command order, collision tie-breaking, and seeded randomness are mandatory.
- A tick processes validated commands, movement/work, attacks/projectiles, completion events, deaths/conversions, visibility, and victory in a documented stable order. Conformance findings may refine this order before the first stable ruleset.
- Simulation code must not read wall-clock time, renderer frame rate, random platform entropy, or network arrival order as gameplay inputs.
- The renderer cannot grant resources, resolve attacks, own fog, or decide victory.
- Serialize all future-affecting state, including RNG streams, queues, path state, cargo, projectiles, conversion attempts, timers, and scenario triggers.

The fixed-step choice is an implementation target. If it cannot reproduce observed timing within the conformance tolerances, increase event precision or simulation frequency; do not round away strategic behavior to preserve the target.

### 15.4 Browser lifecycle

Browsers may throttle background tabs and suspend animation callbacks. A worker is not a guarantee against suspension. [MDN Page Visibility API](https://developer.mozilla.org/en-US/docs/Web/API/Page_Visibility_API).

Single-player also runs on the Go server. The server continues when a browser is hidden unless the player explicitly pauses. A restored client consumes current authorized state and never applies elapsed wall-clock time as browser physics. Offline play, if added, must use a local Go server; browser-only simulation is outside this architecture.

In multiplayer, the dedicated server continues the match. Returning clients resynchronize to current authorized state; they do not replay unbounded local time. Handle visibility changes, socket loss, WebGL context loss, audio suspension, and page reload as distinct lifecycle events. Losing rendering must not reset the match or refund commands.

### 15.5 Loading and local storage

Load the menu shell first, then the selected map/civilization assets. Cache versioned assets for repeat play. Cached presentation assets alone cannot provide offline gameplay; a running Go server is always required. Display real loading stages and progress rather than a permanently animated splash screen.

Use SQLite on the Go backend for authoritative single-player sessions and periodic checkpoints. The current implementation supports named/ID resume and preserves immutable event history across process restarts. Browser storage holds reconnect credentials and presentation preferences, not simulation authority. IndexedDB may hold settings and cached replay metadata, with schema migration and quota-failure handling. Portable save export/import remains a release goal. [MDN IndexedDB guide](https://developer.mozilla.org/en-US/docs/Web/API/IndexedDB_API/Using_IndexedDB).

Use atomic save records/checksums and keep the previous valid checkpoint until a new one commits. Storage failure must produce a visible recoverable error. Never promise that browser storage alone is a permanent backup.

## 16. Multiplayer, services, and competitive integrity

### 16.1 Authority and information flow

Use a dedicated authoritative server for online matches. Clients send intents such as “these units move here,” not positions, damage, resource balances, or victory claims. The server validates ownership, target eligibility, command rate, prerequisites, costs, and timing.

The online client receives a filtered view of the world: own state, legitimately shared allied state, visible enemies, and allowed last-known information. Do not send complete hidden state and merely cover it with a fog shader. The online client is not a full deterministic replica of hidden enemy simulation; full deterministic replay runs only where access is authorized.

Commands carry match ID, player identity, monotonic sequence, command kind, selected entity IDs, target/position, and queue flag. The server assigns execution ticks. Duplicate sequence numbers cannot spend resources twice. Rejected commands return a generic visibility-safe reason. Input feedback such as selection and order markers is immediate; actual outcomes follow server acknowledgment.

Target authoritative state updates at 10–20 updates per real second, adaptively compressed, while simulating at the required gameplay rate. Use interpolation for visible movement and bounded cosmetic prediction. Never predict hidden enemy actions or allow client prediction to decide damage.

### 16.2 Lobby and matchmaking

Lobbies expose all rules, civilization choices, team arrangements, map, spectator policy, and asset/ruleset versions before ready-up. Changing gameplay settings clears ready states. Private invite links carry scoped lobby access, not account credentials.

Ranked matchmaking considers rating, team size, region/latency, and search duration. Maintain separate 1v1 and team ratings with uncertainty for new accounts. Commit a match result once through an idempotent server transaction. Resignation, defeat, disconnect forfeiture, and server failure are distinct result reasons.

### 16.3 Pause, disconnect, and reconnect

Single-player allows unrestricted pause and save. Custom multiplayer exposes pause policy in the lobby. Ranked product default: each team receives two tactical pauses of up to 60 real seconds, with a visible timer and abuse controls.

A disconnected ranked player has a 120-real-second reconnect window while the match continues. Units execute existing orders and normal stances; no substitute AI takes over. Reconnect restores command authority only after authenticating the same seat and receiving a consistent state snapshot plus subsequent deltas. At expiry, record the player's forfeit; surviving teammates continue under the selected team rules. Custom games may allow explicit AI takeover.

If a match server fails, attempt recovery from its checkpoint plus durable command log. If authoritative continuity cannot be proven, mark the match interrupted/no contest rather than fabricating a winner or changing ratings.

### 16.4 Service and abuse requirements

Enforce command ownership, resource integrity, bounded command payloads, rate limits, authenticated connections, and server-side result validation. Match replays and diagnostics must make suspicious outcomes reviewable. Chat supports mute, block, report, and host moderation. A ban or chat mute does not alter simulation statistics.

Use pseudonymous player IDs in public results. Keep authentication secrets out of replay files and client logs. Define configurable retention for private chat, diagnostics, and uploaded saves before operating a public service.

## 17. Saves, replays, spectating, and editor

### 17.1 Save/load

Single-player supports manual slots, autosave, quicksave, and export/import. Saves contain a schema version, exact ruleset and simulation version, map seed/script, complete authoritative state, scenario progress, and integrity checksum. Unsupported versions produce an explanation; never silently reinterpret an old save using changed combat rules.

Loading must preserve ownership, population, carried resources, technologies, active queues, projectiles, deployed siege, garrisons, relics, conversion progress, AI state, and objective timers. An autosave during an attack must not reroll its outcome on each load.

### 17.2 Replays and spectators

Record the initial state, seed, version manifest, ordered commands, necessary external events, periodic state hashes, and seek checkpoints. Replays support pause, speed changes, timeline seeking, player POV, authorized omniscient view, statistics, and bookmarks.

Live spectators use a separate feed and an enforced server-side delay of at least 120 real seconds for ranked play. Delay applies to state, events, scores, and chat—not only video frames. Active players and defeated teammates cannot obtain undelayed omniscient state through an alternate endpoint. Full replay access is granted after match completion according to lobby privacy settings.

Postgame analysis shows resource collection/spending, economy and military population, age timings, technologies, idle production, units lost, trade/relic income, and the decisive timeline. Clearly distinguish observed metrics from coaching interpretations.

### 17.3 Scenario editor

Provide terrain painting, elevation, resource/entity placement, player/civilization configuration, technology restrictions, diplomacy, objectives, trigger conditions/actions, dialogue, and test-from-here execution. Validate unreachable objectives and invalid entity references.

Scenario logic uses a declarative bounded trigger language rather than arbitrary uploaded JavaScript. Enforce limits on entities, trigger evaluations, spawned units, and execution time. Export an explicitly versioned package containing scenario data and referenced original assets. Ranked play uses only approved rulesets and map scripts.

## 18. Content data and ruleset management

Keep gameplay data separate from code and visual assets. Required definitions:

| Definition | Mandatory fields |
|---|---|
| Unit | ID, line/tier, producer, cost, time, population, HP, attack/armor classes, movement/collision, LOS, ranges, attack timing, projectile, cargo/garrison, abilities, availability, presentation IDs |
| Building | ID, age/prerequisites, footprint/terrain, cost, construction/repair, HP/armor, production/research, housing, drop-off, garrison, attack, cancellation behavior |
| Technology | ID, age, producer, cost/time, prerequisite graph, effect operations, availability, stacking/repeatability |
| Civilization | ID, start modifiers, availability graph, bonuses, unique units/technologies, team bonus, architecture/audio references |
| Map script | ID/version, seed inputs, size rules, placement phases, fairness constraints, validation and retry policy |
| Mode | Start state, settings, victory rules, timers, diplomacy, scoring and exception policy |
| Scenario | Terrain/entities, players, objectives, triggers, dialogue, difficulty modifications |
| Ruleset manifest | Version, source revisions, content hashes, simulation compatibility, conformance status, explicit deviations |

A representative command envelope is:

```json
{
  "matchId": "match-uuid",
  "sequence": 1042,
  "kind": "move",
  "entityIds": [318, 319, 322],
  "target": { "xFixed": 15360, "yFixed": 26624 },
  "queue": false
}
```

Coordinate scale is defined by the protocol version; coordinates are integers on the wire. The server derives player identity from the authenticated connection and assigns execution time. A client cannot impersonate a player by adding a field.

Content validation rejects missing IDs, unreachable or cyclic prerequisites, negative costs, invalid ranges, unsupported effects, unavailable producers, missing presentation references, and contradictory civilization overrides. Every technology shown in the UI must correspond to the same authoritative definition used in simulation.

Maintain a conformance ledger with: behavior ID, reference build/source, setup, input sequence, measured outcome, expected tolerance, implementation fixture, and status. Explicit deviations require a named product decision and a ruleset version bump. Changing balance never mutates an ongoing match or silently changes an old replay.

## 19. Performance and reliability budgets

These are acceptance targets, not benchmark results. Measure with release builds and published fixture maps.

| Area | Acceptance target |
|---|---|
| Baseline device | Intel Core i5-1135G7, Iris Xe, 8 GB RAM, 1920×1080, low settings; also Apple M1/8 GB Safari at release QA |
| Common battle rendering | 60 FPS target; p95 frame time ≤20 ms in a 1v1 400-population fixture |
| Large battle rendering | p95 frame time ≤33.3 ms in an eight-player, 1,600-population fixture with 800 units visible |
| Scale envelope | Up to 1,600 standard population units, 2,000 buildings, 2,000 active projectiles, and map-sized resources in the eight-player fixture |
| Local input feedback | p95 ≤50 ms from input to selection/order-marker display |
| Online execution | p95 ≤200 ms from command send to authoritative execution on a stable 100 ms RTT connection |
| Simulation | Sustains 34 ticks/real second at normal speed; p95 tick CPU ≤15 ms on the baseline server hardware |
| Cold menu load | ≤3 real seconds on 25 Mbps / 50 ms RTT, excluding external login |
| First playable tutorial | ≤15 real seconds on that connection; initial required compressed assets ≤30 MB |
| Memory | ≤1 GB steady-state tab memory and ≤1.5 GB transient peak on the standard eight-player fixture |
| Normal online bandwidth | Target average ≤128 KB/s down and ≤20 KB/s up per player; reconnect snapshots measured separately |
| Reconnect | Resume ≤10 real seconds after transport recovers on the stated connection |
| Soak | Four-hour match with no crash, lost authoritative state, or monotonic memory leak |

Large custom 500-population settings require a pre-match performance notice and separate stress measurements; they do not weaken the standard 200-population guarantee. Lowering shadows, particles, texture resolution, and animation detail may improve performance. Lowering simulation accuracy, deleting unseen armies, or changing unit costs may not.

Record p50/p95/p99 CPU, frame, memory, bandwidth, and command-latency measurements. A performance gate passes only with the specified entity mix, not with thousands of idle placeholders that never pathfind, fight, gather, or update fog.

## 20. Verification and acceptance criteria

Each numbered requirement below must become an executable fixture, reproducible playtest, or content review before the game release. These are requirements for the future implementation, not tests claimed to have run for this document.

| ID | Requirement | Evidence required |
|---|---|---|
| ECON-01 | Complete gathering cycle | Stockpile changes only on eligible deposit; death, reassignment, depletion, and lost drop-off preserve accounting |
| ECON-02 | Real opportunity costs | Queue and research spend once; canceled/destroyed tasks use verified refunds; no negative-stock or refund loop |
| ECON-03 | Renewable and finite economies | Trees/mines/natural food exhaust; farm/trap renewal costs wood; trade/relic income follows travel/storage rules |
| POP-01 | Housing and population | Reproduce queue blocking, destroyed housing, conversions over cap, transport occupancy, and civilization exceptions |
| AGE-01 | Four ages | Correct costs/times, distinct prerequisites, production contention, cancellation, global unlocks, and architecture changes |
| TECH-01 | Complete technology graph | Every launch-civilization node is reachable or intentionally unavailable; effects apply once to correct entities |
| BUILD-01 | Construction/repair | Valid placement, multi-builder scaling, attackable foundations, refund behavior, repair spending, wall/gate collision |
| COMBAT-01 | Reference combat results | Per-hit damage and target eligibility match fixtures; attack and reload timing within one simulation step or finer required event precision |
| COMBAT-02 | Projectile and siege behavior | Moving-target misses, tracking, minimum range, splash/friendly fire, deployment, garrison and tree removal match reference fixtures |
| COMBAT-03 | Conversion and relics | Eligibility/timing distributions, resistance, faith, ownership transfer, cargo/drop and income/victory state remain correct |
| PATH-01 | Useful army movement | 200-unit mixed group crosses a two-tile passage without persistent deadlock; impassable routes fail visibly; openings trigger repaths |
| NAVAL-01 | Complete water match | Fish, build traps, trade, fight with all naval families, land transports, and finish a naval-map game |
| CIV-01 | Thirteen real civilizations | Availability, bonuses, team effects, unique units/technologies and exceptional starts match the selected ruleset |
| FOG-01 | Correct information boundary | Player payloads, minimap, sounds, errors and spectator endpoints disclose only authorized state |
| MAP-01 | Fair deterministic maps | 10,000-seed suite per ranked configuration meets connectivity, placement and resource constraints |
| MODE-01 | All victory modes | Each victory condition, interruption, team interaction and same-tick tie has a fixture |
| AI-01 | Complete opponents | Every difficulty finishes land and naval games; AI obeys declared information/economy rules and recovers from disruptions |
| CONTENT-01 | Full launch content | 13 civilizations, eight maps, ten lessons, three five-mission campaigns, original art/audio and functional editor |
| NET-01 | Multiplayer integrity | 1v1 through 4v4; duplication/reorder/reconnect tests; unauthorized commands rejected; results committed once |
| SAVE-01 | Exact continuity | Save/load at complex combat ticks produces matching future hashes for the same command sequence |
| REPLAY-01 | Reproducibility | Go reference replays produce matching authoritative hashes; seek and uninterrupted playback agree |
| WEB-01 | Browser lifecycle | Background/foreground, reload, offline transitions, WebGL loss, storage full and interrupted download recover as specified |
| PERF-01 | Full-scale performance | Published measurements satisfy §19 on the declared machines and browsers |
| UX-01 | Playable controls | Remapping, groups, queues, chat focus, HUD scaling, minimap, notifications and accessibility checks pass user flows |

### 20.1 Reference conformance method

Use controlled setups with no civilization bonus first, then repeat with each applicable modifier. Capture starting state, exact commands, and observable results. Use repeated trials with confidence intervals for stochastic conversion and accuracy behavior; a single matching result does not prove a probability distribution.

Cover numerical data mechanically, then verify behavior that data cannot establish: work cycles, animation delays, targeting, collision, multi-worker building, market rounding, projectile interaction, conversions, and visibility transitions. Compare game outcomes and input timing, not visual resemblance alone.

Pathfinding need not reproduce undocumented implementation internals, but it must reproduce the reference's legal movement, collision constraints, tactical opportunities, and practical responsiveness. Any observed difference that changes a build order, engagement, wall interaction, or counter relationship is a fidelity defect until resolved or explicitly accepted as a deviation.

### 20.2 End-to-end release scenario

On a fresh supported browser, load the game, finish an introductory lesson, start a seeded skirmish, advance through all four ages, create a mixed army, use monks and siege, save and reload, and win. Then complete an eight-player online match including allied trade, naval combat, a reconnect, a delayed spectator, postgame results, and replay seeking. Finally export and import an editor scenario and play it to its scripted conclusion.

The release is incomplete if this sequence requires developer tools, manual database edits, placeholder mechanics, or an external native game client.

## 21. Development plan and release gates

Implementation follows dependency gates rather than an unsupported calendar estimate. Team size, asset production capacity, and desired fidelity validation depth determine schedule; estimate them after the simulation prototype.

| Stage | Deliverable | Exit gate |
|---|---|---|
| 1. Rules foundation | Versioned data model, reference ledger, Go authoritative simulation and typed lifecycle core, terrain, camera, selection | Seeded replay/hash agreement; selected economic and combat reference fixtures |
| 2. Economic settlement | Workers, resources, drop-offs, construction, production, housing, Dark/Feudal ages | Complete gathering-to-production loop; resource/refund/population integrity |
| 3. Full land warfare | All ages, shared land roster, technologies, monks, siege, walls, pathfinding and fog | Full 1v1 land match with reference interactions and save/load |
| 4. Complete strategic systems | Naval play, trade/relics, all victory modes, procedural map validation | Complete land/water matches and economic exhaustion scenarios |
| 5. Multiplayer | Dedicated authority, filtered state, lobbies, teams, reconnect, results, replay/spectator feeds | Stable 4v4 under latency/loss; no hidden-state exposure; deterministic server recovery |
| 6. Full content | All 13 civilizations, eight maps, AI levels, ten lessons, 15 campaign missions, editor, original assets | Catalog complete; every civilization and mission playable to completion |
| 7. Release hardening | Browser matrix, accessibility, optimization, moderation, telemetry and operations | All §20 gates and §19 budgets pass; no unresolved critical fidelity defects |

Build small vertical slices within these stages, but preserve the final scope. A two-civilization skirmish is a development milestone, not the complete requested game.

### 21.1 Principal engineering risks and responses

| Risk | Response |
|---|---|
| Superficial fidelity hides strategically different rules | Build the conformance ledger early; prioritize economic timings, counters, and pathing over decorative polish |
| Crowded pathfinding exceeds CPU budget | Benchmark actual mixed armies early; use hierarchical routing, bounded updates and deterministic local avoidance |
| Full-state replication exposes fogged enemies | Design authoritative filtered replication before adding ranked play |
| Browser suspension breaks matches | Keep online authority on the server; persist Go simulation checkpoints; exercise lifecycle failures routinely |
| Civilization exceptions scatter through code | Use validated effect operations and explicit special-behavior modules with fixtures |
| Full content production overwhelms implementation | Establish original asset families and campaign tooling early; measure throughput without removing required gameplay |
| Balance changes break replays | Pin rules, manifests and simulation versions per match; retain compatible replay execution |

## 22. Additional depth without changing competitive fundamentals

The “and more” portion of the product adds capabilities around the full strategy game:

- **Practice laboratory:** configurable armies, technologies, resources, terrain height, and repeatable engagement trials.
- **Build-order trainer:** timed objectives, optional pause/retry, and explanations based on actual recorded economy.
- **Replay coaching:** idle production, population-block duration, resource float, scouting gaps, upgrade timing and trade losses.
- **Cooperative scenarios:** shared campaign objectives, separate economies, allied tribute and coordinated battles.
- **Scenario sharing:** portable packages, previews, tags, version compatibility and reporting.
- **Historical codex:** concise sourced entries for civilizations, equipment, architecture, and campaign settings.
- **Community competition:** lobby presets, observer controls, brackets via shareable match identifiers, and exportable results.

Persistent account progression tracks campaign completion, lessons, achievements, rating, and optional cosmetic identity. It does not grant stronger units, faster gathering, better technology, or a higher population limit in competitive matches. The initial distribution model is a freely accessible browser game; monetization, if introduced, is confined to cosmetics or clearly separate authored content and must preserve competitive rules.

## 23. Document acceptance and handoff

This specification fulfills the requested document scope by defining:

| Requested element | Where specified |
|---|---|
| Browser-based video game | §§14–19: runtime, controls, browser lifecycle, networking, storage, performance |
| Age of Empires mechanical fidelity | §§1, 3–11, 18, 20: reference policy, detailed systems, data provenance, conformance |
| Similar historical setting | §§1, 10, 13–14: medieval scope, civilizations, original campaigns and presentation |
| Similar development and progression | §§2, 4–8: economic opening, settlement growth, four ages, technology and military upgrades |
| Additional depth | §§11–13, 17, 22: modes, AI, campaigns, editor, replays, practice and community tools |
| Markdown deliverable | This `GAME_SPEC.md` file |

Product defaults are specified throughout so implementation can begin without another planning document. Exact behavioral parity remains an implementation validation obligation, with the reference-data limitations and required evidence stated in §§1.1, 18, and 20. The development team must not label proposed or unverified behavior as proven reference behavior.
