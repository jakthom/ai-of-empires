# Player MCP endpoints

The Go backend exposes **one MCP endpoint per player membership per game**:

```text
http://127.0.0.1:9090/api/v1/games/<game-id>/memberships/<membership-id>/mcp
```

All gameplay tools for that player live at this URL. A second player receives a different URL and credential. A URL identifies the player; it is not a secret or a substitute for authentication. The game currently has memberships, not global user accounts.

Use the same player's MCP access for an assistant helping manage their kingdom. Invite and claim a separate friend seat for an agent opponent. Externally controlled seats use the existing `human` controller: the built-in AI does not also issue orders for them. Separate kingdoms begin at peace, but formal alliances and shared allied vision are not implemented.

## Get your connection details

With ordinary browser membership authentication or your normal session bearer:

| Request | Result |
|---|---|
| `GET /api/v1/games/{id}/agent` | Your `game_id`, `membership_id`, `player_id` and relative `mcp_url`; no token |
| `POST /api/v1/games/{id}/agent` | The same information plus a restricted MCP `token`; no request body |

The POST returns only the requesting player's credential. It cannot choose another member, even for the game owner. Retrieving it again returns the same credential for the current membership version and hosting epoch. It does not rotate browser credentials or interrupt play.

For a game already open in your browser, this can be run in that game's developer console:

```js
const saved = JSON.parse(
  sessionStorage.getItem('aoe.tab-session.v1') ||
  localStorage.getItem('aoe.session.v1') || 'null'
);
if (!saved?.membership_id) throw new Error('Open a multiplayer-capable saved game first.');
const response = await fetch(`/api/v1/games/${saved.match_id}/agent`, {
  method: 'POST',
  headers: saved.token ? { Authorization: `Bearer ${saved.token}` } : {}
});
const connection = await response.json();
if (!response.ok) throw new Error(connection.error.message);
({ url: new URL(connection.mcp_url, location.origin).href, token: connection.token });
```

Configure the agent's MCP client with:

- **Transport:** Streamable HTTP.
- **URL:** the absolute URL above, using the returned `mcp_url`.
- **Authorization header:** `Bearer <returned MCP token>`.

Use the restricted `mcp_…` token, not your browser/session token or rejoin code. Do not place the token in the URL. This server uses preconfigured bearer credentials; OAuth discovery/consent and a stdio bridge are not implemented. The client must support an Authorization header.

For an opponent, create a game with a friend seat, issue an invitation from the lobby, and have that seat claimed separately. Programmatic setup uses `POST /api/v1/invites/claim` with `{id, secret, name}`, then that returned session token to call `POST /api/v1/games/{id}/agent`. Give the playing agent only the resulting MCP URL/token. Invitation claim is explicit and single-use; seeing a game ID does not permit joining. The [multiplayer guide](MULTIPLAYER.md) covers late joins and replacement.

## Isolation and permissions

Every MCP HTTP request, including initialization and tool discovery, verifies the game ID, membership ID and restricted token. Cookie-only access, ordinary session tokens, another player's URL, and caller-supplied acting-player fields are rejected. The endpoint has no mutable “current player” and no shared observation cache or MCP session baseline.

The MCP credential does not authorize ordinary REST endpoints. In particular, it cannot download databases/archives, retrieve the seed through lobby configuration, mint membership credentials, replace seats, delete games or move games. Those administrative operations are not MCP tools. An agent assisting the owner can use the owner's normal **gameplay** controls: start, resume and speed. Every member may pause under the existing multiplayer policy.

All observations come from `matches.Access.View` and Go's player projection:

- Enemy units outside sight are absent; unexplored tiles remain unknown.
- Remembered static objects have `visible: false` and retain their last observation.
- Visible enemies reveal map-observable properties, but not production/research queues, rally points, cargo, stance, passengers, private order destinations or internal AI plans. Activity labels do not disclose private research or marching intentions.
- Only the player's own economy, available actions and production rates appear.
- Events and entity history remain private to their original user. A discovery record is your observation, not access to that enemy's earlier or later activity. Old discovery records and remembered snapshots are sanitized on read without rewriting the immutable journal.
- Map-region and entity filters narrow an authorized snapshot; they never grant more visibility.

The server does not receive or distribute agent chat, prompts or reasoning. Observable gameplay effects remain visible under the normal rules. Two agents using the **same membership** intentionally share one kingdom's view and command authority.

Rejoining rotates the membership credentials and invalidates its existing MCP token. Replacing a seat creates a new membership and endpoint. Hosting-epoch changes also invalidate the token. Ordinary server restarts preserve it. To revoke a shared MCP token with the current implementation, recover the membership using its private rejoin code and fetch new MCP credentials; the old browser credentials also change. Independent per-agent keys, expiry and narrower permissions such as economy-only assistance remain future work.

## Tools and gameplay coverage

All **32 implemented multiplayer command kinds** are reachable through `POST /api/v1/games/{id}/commands`. MCP routes the same Go `Command` through `matches.Access.Apply`. It adds no simulation, privileged AI path, free resource grants or clock stepping.

| Gameplay | Command kinds / API |
|---|---|
| Movement and work orders | `move`, `attack_move`, `interact`, `stop`, `stance`; optional queued orders |
| Economy and construction | `gather` (including fishing), `build` (including wall routes/gates), `repair`, `reseed_farm` |
| Production and progression | `train`, `research`, `age`, `cancel`, `rally` |
| Combat and monks | `attack`, `heal`, `convert`, `relic`, `deposit_relic`, `deploy` |
| Garrison and sea transport | `garrison`, `unload`; ship movement uses `move` |
| Trade | `market_buy`, `market_sell`, `trade`, `market_post`, `market_accept`, `market_cancel`, `market_resume`, `market_recall` |
| Removal and defeat | `delete`, `resign` |
| Placement | `check_placement`, equivalent to the authenticated `/placement` query |
| Shared clock | `start_game`, `pause_game`, `resume_game`, `set_speed` |
| Observation and outcomes | `observe`, `map_region`, `read_log`, `game_status`; Go determines visibility, combat, costs, timers and victory |

Automatic delivery, resource depletion, production completion, gate passage, attack timing and victory do not need independent mutation endpoints. Player intentions start or interrupt these existing Go lifecycles. Mechanics still listed as future scope in [README](../README.md#implemented-scope), such as formal alliances and formations, do not become available merely through MCP.

The endpoint advertises 16 tools:

| Tool | Use |
|---|---|
| `game_status` | Your identity/seat, shared lobby and match status, owner flag and control revision; no seed or foreign membership IDs |
| `catalog` | Commands and required/optional fields, units, buildings, technologies, civilizations, world options and log filters |
| `observe` | Your snapshot with at most 200 entities per page (100 by default); optional `owner` and `entity_ids` filters |
| `marketplace` | Published offers, participant-only private offers/deliveries, and shared finite merchant stock/quotes; see [trading examples](MARKETPLACE.md) |
| `map_region` | Up to 32×32 fog-filtered tiles in row-major order |
| `check_placement` | Go validates/prices a building or wall route without spending resources |
| `command` | A typed gameplay intention with a unique ID |
| `read_log` | Search/paginate your immutable log; optional `entity_id` gives your history of that entity |
| `start_game` | Owner starts the configured lobby |
| `pause_game` | Any member explicitly pauses the shared clock |
| `resume_game` | Owner explicitly resumes with the observed revision |
| `set_speed` | Owner sets a supported speed with the observed revision |
| `save_game` | Checkpoint through normal membership access |
| `connect_player` | Open a separate gameplay presence connection |
| `heartbeat` | Refresh your gameplay presence connection |
| `leave_player` | Disconnect your gameplay presence connection |

`catalog.commands` is also available over REST. MCP input and output schemas are generated from Go wire types, with command kinds constrained to that catalog. Unknown fields, including nested fields such as `position.hp`, are refused. Gameplay errors use MCP `isError: true` with `{"error":{"code":…, "message":…}}` in text content. Successful results provide both structured JSON and text content.

## Agent play loop

1. Read `game_status`, then `catalog`. The owner starts the configured lobby if it is not started.
2. Call `observe` with `owner` set to your `player_id` to find your workers and producers. Call it with `owner: 0` to examine observable resources, or without an owner filter for all observable entities.
3. Read `snapshot.entities[].actions` and `snapshot.build_options`. Go supplies costs, enabled flags and refusal reasons. Use `map_region` to examine terrain and `check_placement` to assess a site.
4. Issue `command` with a fresh ID and actual observed entity IDs. Coordinates are map-space X/Y, not screen pixels. Acceptance is not completion: read later observations to track execution.
5. Catch up on history using `read_log` with `after: last_cursor`; follow `has_newer` using `newer_cursor`. Use `entity_id` or `q` for a bounded investigation.

For example, after identifying your worker and a visible resource:

```json
{"id":"agent-work-0001","kind":"gather","entity_ids":[12],"target_id":80}
```

These IDs are illustrative. For a new building, use `build` with `product` and `position`; to resume an existing foundation, use `interact` with its `target_id`.

`observe` returns `{snapshot, map_included, has_more, next_entity_id}`. Unless `include_map: true`, `snapshot.map` contains dimensions/biome but empty tile/fog arrays. Entity pages are ordered by ID; keep filters unchanged and pass `next_entity_id` as `after_entity_id`. Each page is a fresh observation at its reported tick, not a transactionally frozen multi-page world.

Retry an uncertain command with the **same ID and identical payload**. REST and MCP share receipts, resource charging and limits for that membership; two transports cannot double-spend the same intention. Another membership has a separate ID namespace. New attempts after rule refusals need new IDs. Receipts survive committed checkpoints; a crash can lose commands since the last checkpoint, as with REST.

## Presence and timing

MCP is stateless; a protocol connection is not a game presence connection. Model reasoning and observation reads do not advance the world or count as heartbeats.

For assistance while a human plays, keep the human browser connected. The owner may start/resume with an absent agent seat; existing orders continue normally. An agent need not open a presence connection merely to send commands.

For explicit agent disconnect detection, or play without any open human browser, the **client runtime** must call `connect_player` and then `heartbeat` with the returned `connection_id` every four real seconds, independently of model reasoning. Use `leave_player` when stopping. The existing 12-second heartbeat expiry and 15-second disconnect grace apply; with no connected players the game pauses and saves. MCP does not automatically resume a paused game.

## Transport and validation

The backend uses the [official Go MCP SDK](https://github.com/modelcontextprotocol/go-sdk/releases/tag/v1.7.0), with stateless Streamable HTTP and JSON responses. Clients may use current protocol discovery or the older initialize handshake. Every POST must accept both `application/json` and `text/event-stream`; SDK clients handle these headers. GET/DELETE on the MCP endpoint return 405. Requests are limited to 32 KiB including the protocol envelope. Private responses use `Cache-Control: no-store`, and the normal same-origin and shutdown boundaries apply.

Run `make check`. Tests use real MCP clients over HTTP and cover different member URLs, scope-restricted credentials, fog and history isolation, visible enemy privacy, malformed inputs, bounded reads, shared controls, concurrent/cross-transport retries, cancellation refunds, presence ownership, restart and credential revocation. No test-only gameplay authority is exposed.
