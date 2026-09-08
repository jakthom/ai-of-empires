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
