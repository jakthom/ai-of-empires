# Architecture rules

- Go is the sole authority for game mechanics, physics, visibility, costs, timers, AI, and outcomes.
- Model all gameplay lifecycles with `github.com/open-ships/statemachine`. Use typed states and events, pure guards, and named transition effects executed through `Instance.Fire`.
- Each lifecycle has one state owner. Never maintain a mutable duplicate state field or call `Machine.Next` to execute effects.
- A simulation tick emits events; transition tables decide lifecycle changes. Ordinary math and pathfinding are functions called by effects, not additional state machines.
- Command switches may route typed requests. Do not put lifecycle transitions and their effects in independent state switches.
- Guards must not mutate entities, consume RNG, perform I/O, or fire another machine. Sample randomness once into event data before firing.
- A transition effect must not synchronously fire its own instance. Dispatch queued follow-up orders after the current event commits.
- The frontend only projects authenticated snapshots and sends intentions. No frontend movement authority, damage, economy, placement validation, AI, or victory logic.
- Preserve the versioned API contract and generate TypeScript/OpenAPI artifacts from Go wire types.
- Add behavioral tests for lifecycle changes, invalid transitions, interruption, and exactly-once effects.
- Keep the long-term scope in GAME_SPEC.md and report implemented scope honestly in README.md.

# Browser validation

- This repo includes Playwright and an isolated Chrome MCP server. Use them to validate UI work; a browser extension connection is not required.
- Run `make test-chrome` for installed Google Chrome, or `make browser-install` then `make test-e2e` for the pinned Chromium build. `npm --prefix web run test:browsers` runs both.
- The suite builds and serves the real Go app on `127.0.0.1:18080`. It owns that server and refuses to reuse an existing one. Leave the development server on port 9090 alone.
- Use public UI controls and inspect our test session's HTTP responses. Do not introduce test-only game authority, mutate frontend snapshots, or read personal browser profiles.
- Inspect screenshots in `web/test-results/`; failures retain traces and video. Open the HTML report with `npm --prefix web run test:report`. Add a regression when fixing a material UI interaction.
- For interactive agent tools, open Codex at this repository root so `.codex/config.toml` loads the pinned `playwright` MCP server. Start the application with `make run`, then use browser navigation, snapshots, screenshots, and vision tools for canvas interaction.
- The MCP server uses fresh Chrome sessions and does not attach to personal tabs. Save browser artifacts under `web/.browser-artifacts/`. See `docs/BROWSER_TESTING.md` for setup and troubleshooting.
