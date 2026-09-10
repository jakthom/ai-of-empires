# Performance investigation — September 6, 2026

The investigation reproduced significant backend work, rather than establishing
a Chrome memory leak. These measurements use this repository on an Apple M1 Pro;
they are regression evidence, not a guarantee of 32× playback for every army size.

## Backend measurements

| Scenario | Before | After | Allocation before → after |
|---|---:|---:|---:|
| Unreachable movement, average per simulation call | 25.25 ms | 0.115 ms | 295,762 → 15,286 bytes |
| Snapshot with 120 additional villagers | 3.89 ms | 0.685 ms | 8,348,432 → 1,584,663 bytes |

The movement benchmark repeatedly orders the same worker toward an unreachable
lake point. Previously an empty path retried every 50 ms despite an existing retry
timer. It now respects that timer. Path edges also query spatial buckets instead
of checking every obstacle on the map. The result includes both traffic-aware
routing and its fallback to a static route when a narrow passage is occupied.
The average includes waiting calls; it is not the latency of every full search.

The snapshot benchmark exposed rebuilding and sorting the complete static catalog
for each owned entity. The projection now shares an ordered immutable catalog.
Snapshot allocations fell approximately 81%. Go still calculates every action's
availability and resource costs; caching did not move gameplay rules into the UI.

Reproduce with:

```sh
go test ./internal/game -run '^$' \
  -bench 'BenchmarkBlockedMovement|BenchmarkPopulatedSnapshot' \
  -benchtime=2s -count=1
```

CPU and allocation profiles can be added with `-cpuprofile=cpu.pprof` and
`-memprofile=heap.pprof`. Compare on the same machine with other CPU-heavy work
stopped. Original profiles for this investigation were written under `/tmp/aoe-before-*`.

## Browser checks

Playwright uses a fresh Chrome profile and only public game controls. A retention
test switches between two- and six-settlement maps seven times, collects garbage,
and records Chrome DevTools heap, document, and DOM-node metrics. Comparing the
third and seventh equally sized maps, Chrome used approximately 6.41 MB and
6.64 MB of JavaScript heap. The document count stayed at 1 and node count at 2,013.
Chromium passed the same bounded-retention check.

An additional Chrome run advanced **64 game minutes at 32× in 120 wall-clock
seconds**. A villager gathered and delivered 600 wood, then became idle after
depleting its available sources. After the initial warm-up, JavaScript heap
samples were 6.87, 7.03, 6.91, and 6.96 MB; document count stayed at 1 and DOM
nodes settled at 2,691. This light economy workload showed no continuing heap
growth or slowdown. Raw samples and a final screenshot are in
`web/.browser-artifacts/long-game-memory.json` and `long-game.png`.

The renderer now reuses selection rings and projectile geometry, stores only the
model parts that need animation, disposes instance buffers when removing models,
and draws a farm's 63 crop plants as one instanced mesh. Terrain capacity and
camera bounds follow the selected game's dimensions. History pages retain a
bounded number of DOM rows rather than every historical event.

```sh
npm --prefix web run test:browsers -- sessions.spec.ts
```

The suite writes `browser-memory.json` in its campaign-switching test directories
under `web/test-results/`. It also checks compact setup/resume, building hover,
autosave visibility, save-failure recovery, browser closure, and Back navigation.

These JavaScript measurements do not measure all GPU/driver memory and do not
replace the four-hour, full-army soak target in GAME_SPEC.md. Complete immutable
journals intentionally grow in the backend and SQLite; active sessions retain
their history in memory. Checkpoints append new journal records to SQLite rather
than repeatedly serializing all history. Unloading a saved session releases its
in-memory world, command cache, and journal.

## Recovery and movement validation

Go behavior tests cover deterministic checkpoint continuation, private lifecycle
restoration without repeated effects, command retries across restart, SQLite
rollback on failed saves, equal settlement openings, producer exits without
overlap, stationary blockers, opposing traffic, and single-tile passages.

A separate real-process check stops and restarts the compiled Go server using a
temporary SQLite database on port 18081. Its paused four-settlement game restores
the exact tick, resources, queue, and event cursor; a fresh browser context can
then resume it by name. Local artifacts live in `web/.browser-artifacts/`.

All validation uses disposable servers. The existing development server on port
9090 and its in-memory match were left untouched.

## Larger worlds and textured buildings — September 8, 2026

Huge (224×224) and Giant (288×288) were checked in installed Chrome through the
public creation, Start, pause, leave and resume controls, with six settlements,
Mixed Regions, Mountain Lakes, full map reveal and Peaceful practice. In isolated
three-second opening samples, both maps had a median animation-frame interval of
16.7 ms (about 60 fps); the 95th percentile was 16.8 ms for Huge and 16.7 ms for
Giant. This measures a quiet opening, not sustained armies or network delivery.
The world-options browser tests attach their timing samples to the HTML report.

Building textures are generated once per building type, age and surface, with
128×128 pixel maps shared across entities and owners. The lazy cache has a fixed
upper bound of 21 types × 4 ages × 5 surfaces; advancing ages and rejoining games
reuse those materials. Removing or replacing a model releases its private
geometry and fog materials while retaining shared textures. Browser checks cover
every catalog building in all four ages, including material reuse.

Giant contains 82,944 terrain cells, so snapshots, fog updates, generation and
checkpoints cost more than on Small. Terrain remains chunked and culled by the
camera. The short rendering sample does not establish a long-game memory bound
or guarantee 32× simulation with a developed six-kingdom economy.

## High-speed scheduling and delivery — September 8, 2026

A second investigation compared commit `0afec69` with this change on the same
Apple M1 Pro and installed Chrome. The public-UI scenario uses seed 82731, six
settlements, Huge (224×224), Mountain Lakes, Mixed Regions, Everything visible,
and Peaceful practice. After an eight-second warmup it samples 15 seconds at
16×, followed by 15 seconds at 32×. Both versions have 1,376 entities: 24 units,
seven buildings, and 1,345 resources. One villager moves 18.35 map units during
the first segment; it is stationary in the second. This is a light opening
workload, not a developed economy or battle.

| Measurement | Before | After |
|---|---:|---:|
| Existing SSE stream bytes, 15 seconds at 16× | 551.32 MB | 0.526 MB |
| Existing SSE stream bytes, 15 seconds at 32× | 512.29 MB | 0.543 MB |
| Stream delivery gap, p95 at 16× | 98.85 ms | 50.93 ms |
| Stream delivery gap, p95 at 32× | 110.48 ms | 50.92 ms |
| Observed simulation/wall time at 32× | about 30× | about 32× |
| Animation-frame interval, p95 | 16.7 ms | 16.7 ms |
| Animation frames above 50 ms / long tasks | 0 / 0 | 0 / 0 |

Byte counts come from Chrome CDP `Network.dataReceived` on the UI's existing
SSE connection during the sample, without opening a duplicate stream. They
exclude the initial full snapshot and other HTTP requests. CDP reports transport
chunks rather than complete SSE messages, so the old stream can have several
arrivals per frame. Clock estimates use HTTP snapshots bracketing the browser
sample and include a small request-timing error. Animation-frame timing alone
does not establish smooth unit movement; separate presentation checks exercise
jitter, observed turns, stalled delivery, and reconnection.

The CPU profile also identified rebuilding fog-memory views for every resource
and kingdom on each visibility refresh. Permanently revealed worlds now use
current observations directly; fogged worlds share each refresh's immutable
public entity projection while keeping each player's memory separate. In the
Huge simulation microbenchmark, mean tick cost fell from **1.544 ms to 0.785 ms**
and allocations from **793,049 to 16,023 bytes per tick**. Capturing a detached
Huge-world checkpoint takes about **0.832 ms**, with encoding and database I/O
performed afterward, outside the simulation lock. These averages are not bounds
on a particular pathfinding, combat, or autosave operation.

The changes address different parts of the pipeline:

- Each loaded game has one clock worker using elapsed wall time, unchanged
  50 ms physics steps, batches of at most eight ticks / roughly 4 ms, and at most
  250 ms of wall-time debt. One physics tick remains atomic; an overloaded host
  slows playback instead of changing mechanics or attempting unlimited catch-up.
- The UI opts into 20 Hz `delta-v1` frames. Only changed entities and map cells
  cross the wire after the complete initial view. Defaults remain compatible
  with the original 10 Hz full-snapshot API. Each subscriber owns one private
  baseline, and reconnects always start with current authenticated state.
- Rendering keeps at most eight position observations per moving entity or
  projectile and uses a 100–300 ms adaptive delay. It never predicts beyond the
  last position. Removed/hidden entities disappear immediately.
- Terrain skips unchanged cells and the minimap reuses a cached bitmap when
  drawing camera outlines and unit markers. Patch metadata uses numeric base
  IDs so it cannot retain a chain of old full maps.
- Each game has at most one periodic checkpoint writer. It captures detached
  data under the game lock, then encodes and commits outside it. Explicit saves,
  membership changes, shutdown, and deletion join the older writer before
  committing newer state. Leased SQLite connections use 100 ms busy waits and
  retry complete transactions, allowing cancellation during lock contention.
- Request admission replaces a finished lease from its saved checkpoint before
  authorizing a new request. This closes the resume race between a game's clock
  finishing its unload and removing it from the shared registry.

Reproduce the isolated browser measurement (one worker, no competing benchmarks):

```sh
npm --prefix web run test:chrome -- high-speed.spec.ts --workers=1
```

The report attaches `high-speed-baseline.json` and a gameplay screenshot.
`web/.browser-artifacts/high-speed-final.log` contains the local final sample.
Use a detached `0afec69` checkout with the same harness for the old baseline.
For the backend measurements:

```sh
go test ./internal/game -run '^$' \
  -bench 'BenchmarkHugeSimulation|BenchmarkHugeSnapshotStream|BenchmarkHugeCheckpointCapture' \
  -benchtime=2s -count=1
```

Validation includes the Go race suite, cancellation while SQLite is locked,
newer saves winning over older autosaves, deterministic checkpoint capture,
private delta reconstruction, independent game clocks, stalled presentation,
and resume during lease removal. The full Chrome run passed 139 checks and
exposed a resume race plus a flaky reload check; after the admission fix all
37 affected world, difficulty, session, and multiplayer checks passed.

Large armies, expensive routes, many simultaneous hosted games, and long-lived
journals can still exhaust a host's CPU or memory. The four-hour full-army soak
remains outside these measurements.


## Formations, crowded movement and economy streams — September 10, 2026

These measurements compare commit `27f95c7` with the campaign-economy change on the same Apple M1 Pro. Both simulation benchmarks run exactly 300 ticks, so they cover the same amount of game time. The army fixture has 120 militia moving on cleared ground within a six-settlement Huge world; its AI is disabled. It exercises individual movement orders and collisions. Formation behavior is checked separately through Go and public browser controls.

| Mean tick | Before | After | Bytes allocated before → after |
|---|---:|---:|---:|
| Quiet Huge world | 1.015 ms | 0.786 ms | 18,942 → 19,461 |
| Moving 120-soldier army | 27.607 ms | 1.278 ms | 4,255,344 → 37,409 |

The army workload improved about **21.6×**, with **99.1% fewer allocated bytes**. A per-tick entity grid narrows collision and target queries; clear routes use collision-checked direct segments instead of repeated whole-map searches. Ordinary movement no longer searches for combat targets every tick. Reaching an intermediate path endpoint resets its retry timer, removing a recurring stop during formation movement. Ranks share a pace on clear ground, release around obstructions and during their final approach, and leave enough spacing for later arrivals to enter their slots.

A Chrome regression trains 24 militia using public controls, selects them with three villagers and a scout, assembles the scattered group, then marches all **28 units at 1×**. Its eight-second rendering sample recorded a **16.8 ms p95 animation interval**, with **zero frames above 50 ms**. The test also checks in-transit rank width and completion of the initial assembly. Go regressions cover interrupted and queued marches, losses, save restoration, gates, map edges and late arrivals.

The existing Huge-map browser scenario uses seed 82731, six peaceful settlements and 1,457 observed entities (24 units, 18 buildings and 1,415 resources). After eight seconds of warmup, each speed has a 15-second sample:

| Browser measurement | 16× | 32× |
|---|---:|---:|
| Animation interval, p95 | 16.7 ms | 16.8 ms |
| Frames above 50 ms / long tasks | 0 / 0 | 0 / 0 |
| Observed game seconds per wall second | 16.17 | 32.45 |
| Live-stream arrival gap, p95 | 51.0 ms | 57.1 ms |
| Decoded SSE body | 9.98 MB | 10.34 MB |
| Encoded stream chunks | 0.77 MB | 0.89 MB |

Every private SSE connection now negotiates gzip and flushes each frame. Chrome CDP reports decoded `dataLength` and encoded `encodedDataLength` separately; compression reduced these stream payloads by about **91–92%**. Figures exclude the initial snapshot and other HTTP requests. The villager moves 18.35 tiles during the first sample and is stationary in the second. Clock ratios include bracketing-request timing error. The extra resource-flow and food-accounting fields make decoded messages larger than the preceding build; compression limits their transport cost.

Reproduce the fixed simulation window and isolated rendering checks with:

```sh
go test ./internal/game -run '^$' \
  -bench 'BenchmarkHugeSimulation$|BenchmarkArmySimulation$' -benchtime=300x -count=1
npm --prefix web run test:chrome -- army-movement.spec.ts high-speed.spec.ts --workers=1
```

Run browser measurements without other CPU-heavy jobs. These are bounded regression workloads, not a completed four-hour army/economy soak or a guarantee of 32× speed under every battle, forest or network condition. Charts and campaign histories have documented retention limits in [Campaign mechanics](CAMPAIGN_MECHANICS.md).
