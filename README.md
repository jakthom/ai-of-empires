# AI of Empires

A playable historical RTS with a Go server and a Three.js battlefield. Go owns the simulation, AI, visibility, resources, pathfinding, combat, and outcomes. The browser renders snapshots and sends player intentions.

## Gameplay

Grow a settlement with farms, resource camps, homes, and military buildings.

![A Feudal Age settlement with a Town Center, farms, houses, and a barracks in the Three.js battlefield.](docs/screenshots/settlement.png)

Follow each villager's current activity and immutable history, and search the live event log to locate individual entities.

![A farming villager's History tab beside the battlefield, with the expanded event log filtered to Villager 3.](docs/screenshots/villager-history.png)

Rotate and tilt the perspective to explore raised ground, recessed rivers, and exposed banks.

![A scout beside a river and ford, viewed from a lower angle that shows the depth of the banks.](docs/screenshots/perspective.png)

## Run

Use Go 1.26 (or Go with automatic toolchain downloads enabled) and Node.js 22.12+.

```sh
make run
```

Open **http://127.0.0.1:9090**. This installs the locked frontend dependencies, builds the UI, and starts the Go server with the UI embedded. No external assets or CDN requests are needed at runtime.

To build a standalone executable:

```sh
make build
./bin/ai-of-empires
```

The server defaults to `127.0.0.1:9090` and stores games in `data/ai-of-empires.sqlite`. Use `-addr` to change the address and `-db /path/to/games.sqlite` to change the database. `-db :memory:` creates disposable sessions for testing.

For frontend development, keep the Go server running and run `npm --prefix web run dev`; Vite proxies `/api` to Go. Restart the Go server after rebuilding embedded assets when testing the production page.

## Play

Choose a civilization, optional game name, and **1–6 settlements including yours**. One settlement is solo play; additional settlements are competing AI kingdoms. Each starts with one Town Center, three villagers, and one scout. Games with three or more settlements use a larger map. Sandbox begins with extra resources; peaceful difficulty disables the opponents' AI.

Choose **Peaceful practice**, **Easy**, **Standard**, **Hard**, **Extra hard**, **Expert**, or **Aggressive** when starting a match. The current difficulty stays in the top bar. Each active AI also has a seeded **Builder**, **Defensive**, or **Expansionist** preference. Builders prioritize growth and a small defensive force; defensive kingdoms may counter-raid a kingdom already attacking them; expansionists can start profitable conflicts. Higher difficulty increases their budgets and planning pressure without making every kingdom expansionist. Aggressive explicitly makes all AI expansionist, with possible raids after one game minute. Expert researches upgrades. All levels use the same starting population, resources, costs, production clock, and gathering rules as the player; civilization bonuses apply equally. The AI automatically queues villagers and assigns work, so its economy can grow more consistently than a manually managed settlement.

**Easy** develops one Town Center and up to **12 villagers**, spaces villager orders by the normal training duration plus 15 game seconds, and budgets **8 military units including queued production**. It sends at most **4 units per raid**, waits until **10 game minutes** before launching, and leaves at least **4 game minutes between launches**. It can defend itself sooner. These are game-clock times; increasing speed accelerates every kingdom equally. Raids have fixed rosters, so new soldiers cannot turn a small party into an endless stream. Existing units in older saves remain alive; excess AI training queues are refunded, and future orders obey the new limits.

Kingdoms start **at peace**. Passing troops do not trigger combat or an AI defense. Actual attacks and conversion attempts start a conflict only between the kingdoms involved; five game minutes without attacks restore peace. The kingdom panel above the world lists each neighbor's relationship with you and preference. AI kingdoms pursue their own interests and evaluate human and AI opponents equally. Unfavorable raids give way to development, and costly expeditions retreat. Above Easy, developed economies can expand to safe, observed deposits in Castle Age. Strategies use current vision and remembered buildings, without access to hidden armies, resources, or other kingdoms' starting coordinates. Formal alliances, negotiated treaties, and shared vision remain future work.

Military units and armed buildings default to **Return fire**. Idle defenders respond to actual recent attackers of themselves or nearby friends, with a six-tile pursuit limit around the defense position; villagers defend only themselves when idle. They do not target innocent units just because they belong to the attacker's kingdom. **Hold position** returns fire within range without pursuit; **Hold fire** disables automatic response; **Aggressive** opts into proximity attacks. Explicit **Attack**, **Attack move**, and hostile contextual orders can start conflicts regardless of the automatic stance. A normal Move order continues to its destination. AI raids target one chosen kingdom along their route and may defend against others that actually attack them.

- **Click** to select an element. **Hold the left mouse button briefly, then drag** to pan (180 ms hold delay). **Hold both left and right mouse buttons**, then drag left/right to rotate or up/down to tilt; release either button to stop. **Shift + arrow keys** also rotate and tilt, including on a trackpad. **Shift-click** adds or removes a selection, and **Shift-drag** selects a group, replacing your previous selection.
- Select your units, click **Give order** (or press **Q**), then click a resource, entity, or destination. The same control sets a selected building's rally point when you click the ground.
- For quick orders, use **Option/Alt-click**, **Control-click on Mac**, or **right-click** with a mouse. Secondary click on a trackpad works too if enabled. No secondary click is required to play.
- Hold **Shift** while issuing orders to queue them and keep targeting. Click **Cancel** or press **Escape** to return to selection.
- Select villagers and open **Build** to place a building. Go checks the site and charges the cost.
- Hover over a building to see its type and activity. Select it to train units, research, advance ages, or cancel queued work. New units emerge in clear spaces beside their own producer; a blocked exit waits for room. Units route around occupied space and steer past nearby traffic.
- **H** selects the Town Center; **.** selects idle villagers; **A** starts attack move; **S** stops selected units.
- Click **Pan view** (or press **P**), then drag to move across the map. Click it again or press Escape to return to normal selection and camera gestures. Arrow keys, middle-drag, and clicking the minimap also pan, following the current camera angle. **Scroll or pinch** to zoom around the ground under your cursor; **+ / −** zooms around the center of the view.
- **Reset view / R** restores the starting camera angle and zoom at your current location. **H** returns to your Town Center. Camera gestures preserve your selection and send no gameplay commands.
- **Cmd/Ctrl + a number** assigns a control group; the number recalls it. To remove a selection, choose **Delete**, then **Confirm removal**; Delete or Mac Delete/Backspace also confirms while removal is armed.
- Space pauses; the speed button cycles through 1×, 1.7×, 3.4×, 8×, 16×, and 32×. The match menu lets you choose a speed directly.
- Selected entities display their current activity (farming, logging, mining, and more). Choose the **History** tab after **Research** to read that entity's immutable log in the command panel, retained even after removal. The global tray stays independent.
- Farms automatically reseed for 60 wood while a villager remains assigned. To restart an empty farm, select a villager, choose **Give order (Q)**, then click the farm. The villager rebuilds it and resumes farming; you need 60 wood available.
- **Event log** at the bottom expands the live kingdom chronicle. Drag its top edge to resize down to one line; with the edge focused, use arrows to resize or Home to collapse. **Older / Newer** browse history and **Follow live** returns to new events.
- Search the full retained log by activity, message, resource, or exact identity (for example, **villager 3** or **#3**). Combine search with the event category selector. **Locate** centers the camera on that entity and opens its History tab; for entities no longer visible it visits the event's recorded location. Entity names filter the tray to their history. **Clear** or **All events** removes the filters.

## Saved games

Games autosave every **10 seconds**, on **Save now**, on **Save and leave**, and during a graceful server shutdown. Open the match menu to see the game name, session ID, and last successful save. Starting another match saves the previous one. The **Saved games** tab searches names and IDs; use a Resume button or enter the exact name or ID. Names are unique without regard to case, with an 80-character limit.

Reloading or reopening the same browser restores its last session. Closing the page requests a final checkpoint; if the browser cannot send it, the server checkpoints and unloads the game after 30 seconds without requests. Use **Save and leave** to stop the clock immediately. A hidden tab still plays; use Pause for a break. Offline time is never simulated. Restarting the server resumes running games from their checkpoint when the browser reconnects; paused games stay paused. An abrupt process failure can lose progress since the last autosave.

Checkpoints include orders, movement, cargo, production, research, combat, fog, random state, every state-machine owner, kingdom preferences and relationships, recent attacks, AI objectives and timers, immutable history, and command receipts. SQLite commits them together. Version-1 and version-2 checkpoints migrate to peaceful relationships and return-fire defaults; old AI attack orders are recalled for reassessment, while resources, entities, queues, and history remain intact. Failed saves keep the active game in memory, report the failure in the menu, and retry. The session library is local to this server: anyone with access to it can list and resume its games. Resuming by name or ID issues a new bearer token and disconnects older connections to that game. Keep the default loopback address for personal use.

To back up games, stop the server gracefully and copy the database, or use SQLite's online backup facility. Do not copy a live WAL database without its associated state. The SQLite format is versioned; unsupported checkpoints fail explicitly rather than starting a replacement game.

## Architecture

| Boundary | Ownership |
|---|---|
| `internal/game` | Fixed 50 ms simulation steps, rules, AI, state machines, player read models |
| `internal/matches` | Serialized match access, clocks, bearer tokens, idempotent commands, SQLite checkpoints and session unloading |
| `internal/httpapi` | Strict JSON transport, authentication, REST commands, SSE snapshots, static serving |
| `web/src` | Three.js geometry, camera, snapshot interpolation, selection, HUD, input |

All gameplay lifecycles use [`open-ships/statemachine`](https://github.com/open-ships/statemachine), with one `Instance` owning each state. Guards observe; transition effects mutate; the tick loop emits events. See [the lifecycle design](docs/STATE_MACHINES.md).

The [API guide](docs/API.md) describes requests and reconnect behavior. [OpenAPI](api/openapi.json) and [TypeScript wire types](web/src/api.generated.ts) are generated from Go DTOs with `go run ./cmd/contracts`. Internal aggregates never become client authority. Action labels, costs, availability, and refusal reasons come from the backend.

## Implemented scope

The `frontier-1` ruleset includes a seeded river map, four ages, construction and repair, resource cargo and drop-off, farms, research and production queues, population limits, combat and projectiles, monks and relics, garrisoning, trade, siege deployment, ships, fog of war, server AI, and conquest/wonder victory. Thirteen civilization choices have simplified bonuses and unique units. All visual models are original procedural geometry. A perspective camera, continuous terrain with exposed banks and cliffs, soft directional shadows, and buildings detailed on multiple sides give the battlefield depth. Terrain relief is visually amplified from the server’s elevation data; movement, terrain rules, and height advantages remain in Go.

This is an initial playable ruleset. Its values and civilization availability are **not verified Age of Empires parity**. [GAME_SPEC.md](GAME_SPEC.md) remains the larger product target: full civilization trees, campaigns, scenario editing, human multiplayer and matchmaking, replays, audio, formations, complete reference rules, and large-army performance certification remain future work.

Up to 16 sessions can be active at once; unloaded saved games remain in SQLite. Browser-only offline simulation is excluded by the Go authority requirement. Current deterministic checks cover checkpoint continuation and repeat runs of this Go implementation; cross-platform replay equivalence is not established, and simulation quantities currently use `float64`. Pre-persistence builds cannot export their in-memory games into the new checkpoint format.

Slowdown fixes include respecting failed-route retry timers, spatially indexing path obstacles, caching the static catalog when projecting snapshots, reusing rings and projectile meshes, batching farm crops, and animating only moving model parts. Browser history pages remain bounded; complete immutable journals intentionally accumulate in the backend and database. See [performance measurements](docs/PERFORMANCE.md).

## Validate

```sh
make check
```

This checks generated contracts, TypeScript, Go vet, and Go tests with the race detector. Behavior tests cover lifecycle interruption, terminal states, resource/refund conservation, conversion, projectiles, deployment, command retries, API authority, SSE, immutable history and visibility, higher speeds, independent AI targeting and combat, retreat/recovery, Easy army and raid budgets, human/AI economic parity, and checkpoint continuation/migration. `make build` also compiles the production UI and Go executable.

The repository also includes Playwright regression tests and an isolated Chrome MCP server for agent-driven UI validation:

```sh
make browser-install  # install the pinned Chromium browser
make test-e2e         # run browser tests against a fresh Go server
make test-chrome      # run the same tests in installed Google Chrome
```

Tests exercise actual WebGL rendering, match startup/reload, saved-game search/resume, settlement count, building hover, browser memory retention, production/refunds, primary-click gathering and rallying, Mac and mouse order gestures, camera controls, building placement, keyboard controls, compact layouts, startup recovery, and match reset. They retain screenshots, videos, and traces on failure. See [browser testing and agent setup](docs/BROWSER_TESTING.md) for interactive Chrome tools and reports.
