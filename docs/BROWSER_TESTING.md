# Browser testing and agent access

Playwright drives the real Go-served application. Google Chrome uses its installed browser binary through the `chrome` channel; Chromium uses the version downloaded for the locked Playwright release. Every test receives a fresh browser context and creates its own match through the UI.

## Install and run

From the repository root:

```sh
npm --prefix web ci
make browser-install
make test-e2e
make test-chrome
```

Google Chrome must be installed for `test-chrome`. The Chromium target does not require Chrome. For Linux CI, install browser system dependencies with `cd web && npx playwright install --with-deps chromium` before running the tests.

The runner builds the UI and Go executable, starts a dedicated server at **http://127.0.0.1:18080** with `-db :memory:`, and shuts it down after the run. It never reuses an existing listener, so a stale executable cannot pass the suite. Its disposable SQLite database never touches development saves. The normal development server at port 9090 can remain running.

Useful commands:

| Command | Use |
|---|---|
| `make test-chrome` | Headless Google Chrome regression suite |
| `make test-e2e` | Headless pinned Chromium regression suite |
| `npm --prefix web run test:browsers` | Both browser targets |
| `make browser-ui` | Playwright's interactive test runner |
| `npm --prefix web run test:headed` | Watch tests run in Chrome |
| `npm --prefix web run test:chrome -- --grep 'production'` | Run one relevant interaction test |
| `npm --prefix web run test:report` | Open the most recent HTML report |

`web/playwright.config.ts` owns browser settings and test-server lifecycle. `web/tests/e2e/fixtures.ts` owns test sessions and cleanup. The tests fail on uncaught browser exceptions and verify server responses and snapshots for successful UI commands.

## Evidence

Screenshots live in `web/test-results/`, with desktop and compact battlefield captures. Failures retain `trace.zip`, screenshots, an error snapshot, and video; the HTML report links them. Open a trace with `cd web && npx playwright show-trace test-results/<test>/trace.zip`.

These artifacts are ignored by Git because they describe local test runs. Tests use semantic controls or stable DOM IDs, wait for actual responses/state, and keep all game authority in Go. Only the startup-recovery test injects a failed catalog response; the successful retry reaches the real server.

`web/tests/e2e/controls.spec.ts` covers primary-click gathering and rallying, queued orders, Control-click, Option/Alt-click, mouse right-click, Cmd control groups, removal confirmation, and camera pan/zoom. Secondary-click tests count actual HTTP commands to catch duplicate pointer/context-menu handling. The gathering test uses the visually verified seed 4817 opening and checks that Go reduces the resource amount. These are automated browser gestures; physical trackpad hardware and Safari are separate validation targets.

`web/tests/e2e/camera.spec.ts` covers direct drag rotation and tilt, reset, small click movements, interrupted drags, compact controls, and panning after rotation/zoom. It compares rendered views and the minimap, verifies camera gestures send no gameplay commands, and uses actual Go rally responses to check that the same ground point stays beneath the pointer while panning or zooming, including trackpad pinch and zoom limits. Shift-drag selection remains covered by the gathering tests.

`web/tests/e2e/economy.spec.ts` verifies that primary-click orders result in stone and wood being gathered and delivered to the Town Center. It also interrupts farm construction, resumes it with a contextual order, and checks that the completed farm produces and delivers food.

## Chrome tools for agents

`.codex/config.toml` registers the repository's locked `@playwright/mcp` dependency as the `playwright` server. Open Codex at the repository root, with dependencies installed. The server starts through:

```sh
npm --silent --prefix web run browser:mcp
```

It launches **isolated headless Google Chrome**, with vision and developer tools enabled for the Three.js canvas, at a 1440×960 viewport. It communicates over stdio, has no network listener, and uses no personal Chrome profile or browser extension. The configuration adds no credentials or changes to global agent settings.

For a visible Chrome window, an MCP client can use `npm --silent --prefix web run browser:mcp:headed` instead. This command speaks MCP and is not a standalone web server; use it through an MCP client.

Agent workflow:

1. Start the application with `make run` unless the development server is already running.
2. Navigate the browser tool to `http://127.0.0.1:9090`.
3. Inspect the accessibility snapshot, start a match, and use screenshots/vision for battlefield gestures.
4. Inspect console/network failures and save screenshots under `web/.browser-artifacts/`.
5. Add any material regression to `web/tests/e2e/` and run the relevant Chrome tests.

Codex reads project MCP configuration for trusted projects. A session opened before this configuration was added may need to be reopened to expose the tools. Confirm the configuration with `codex mcp get playwright --json`. These are project-scoped settings supported by the [official MCP configuration documentation](https://learn.chatgpt.com/docs/extend/mcp?surface=cli).

If an agent session does not expose MCP tools, the repo's Playwright test commands still work through its shell. No browser extension is needed for either path. If Chrome cannot launch, check that Google Chrome is installed; the Chromium target can independently validate the app after `make browser-install`.

Browser channel behavior follows [Playwright's browser documentation](https://playwright.dev/docs/browsers). MCP options follow the [official Playwright MCP README](https://github.com/microsoft/playwright-mcp).
