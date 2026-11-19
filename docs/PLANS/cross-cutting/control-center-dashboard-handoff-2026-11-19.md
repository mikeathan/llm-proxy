---
status: partial
date: 2026-11-19
last_reviewed: 2026-11-19
related_specs: [SPEC-003, SPEC-006]
constitution_references: [IV.6, I.16]
evidence: branch dashboard-card-controls-and-cards (cut from main at 9fc07f3f, v0.105.0; ADR-0065; SPEC-003 2.68); #271 merged; plan control-center-dashboard.md; frontend/src/components/dashboard/, domain/dashboard.ts, composables/dashboard/, composables/stacks/useEnabledStacks.ts, components/netaudit/TrafficDevices.vue, components/radio/RadioManageDevices.vue; e2e/dashboard.visual.spec.ts
---

# Dashboard of boards and cards: handoff (2026-11-19, second pass)

## State
- **Branch** `dashboard-card-controls-and-cards`, cut from fresh `main` (`9fc07f3f`, after #271 was merged and deployed). **Nothing is committed or pushed**; the work is in the working tree, with `frontend_dist/index.html` rebuilt. Version `0.106.0`, ADR-0065 (`proposed` until you read decision 6), SPEC-003 2.68. No backend change; no Go code reads `VERSION`.
- **Not run on vertex.** The owner's first run (of #271) found the card controls wrong; those are fixed and proved in a real browser, but the new cards have run only against mocked answers.

## Built (this branch)
- **Card controls.** Height is 1x (content) or 2x (at least 28 rem), never a row span; cards flow in board order; "earlier" and "later" move one place on screen and keep focus; only a grip drags (`DashboardGrid.vue`, `DashboardCardFrame.vue`). `e2e/dashboard.visual.spec.ts` measures it (neighbour height unchanged; visual order equals board order).
- **Only what is on.** `useEnabledStacks` reads `GET /admin/api/stacks`; the catalogue and Add board offer a stack's cards and board only while it has a module on (unknown: everything).
- **Polling.** One `IntersectionObserver` in the frame provides `DASHBOARD_CARD_VISIBLE`; `usePolling` skips a tick for a card out of view and runs on return. (A shared global poller, `useGlobalRunActivity`, is not card-owned and is unchanged.)
- **Cards moved off the pages:** `What happened lately` (the Timeline tab is gone), `Traffic by device` (the Traffic tab keeps the look, the router setup, Trace a route and the tables line), `My devices` (Radio; the page keeps Claim, **Add or remove a device**, and the networks). New: `Devices`, `Recent conversations`, `Agent activity`. Timeline, Traffic and My devices are on the shipped boards; the new three are in the catalogue only.
- **Links to a card.** `?card=` on the dashboard; the stack pages link to it; a saved board without it says so and offers "Add it" (`BOARD_HINT`).
- `What is watching` says when the modules could not be read.

## Checks run (2026-11-19)
`npm run build` (lint, types, build) passed; `npx vitest run` 339 files, 3,279 tests passed; `npm run test:visual` 407 passed, 1 skipped (a desktop-only check on the mobile project) before the last CSS change (2x became a minimum height), then the dashboard, radio, timeline and traffic specs re-run: 81 passed, 1 skipped. The full visual run was not repeated after that change. Backend untouched except `VERSION`: `go build ./...`, `go test ./internal/transport/... ./internal/app/...` passed and the complexity tool reported no new violations; the full `go test ./...` was **not** run. `./scripts/check-agent-harness.sh` passed.

## Left, in order
1. **Run it on vertex** (owner): the dashboard with the real boards. Your saved Network and Radio boards predate the new cards: your saved boards predate three cards, and each page links to its card with an **Add it** line: open the Network page's link (Timeline), the Traffic tab's link (Traffic by device) and the Radio page's Add or remove a device link (My devices), pressing **Add it** on each, or use **Add card**; check that the radar reads as before (it measured as tall as its content); try Arrange (resize, earlier, later, drag by the grip). First command: build and deploy as usual.
2. **The four device lists were not merged** (a deviation from the plan, ADR-0065, table in the plan). The owner decides whether any should go: for example the traffic rail could become the Devices card with a traffic column, or My devices could take the Devices row. Needs: your reading of the table.
3. **Assistant: approvals waiting.** There is no list of waiting approvals to read (they live in the open chat as a guardrail banner); a card needs a new endpoint or a shared store. Not built.
4. **The LLM queue card** was not built: `Running now` already shows the model lanes and the queued runs by position.
5. **Home** does not ship the new cards (Recent conversations, Agent activity, Devices). Say which belong on Home.
6. **`useDevices().load()`** reads the inventory, presence and IPv6 state; the Devices card needs only the inventory.

## Traps
- A card made of several `Panel`s must `provide(IN_DASHBOARD_CARD, false)` (pitfall 74). A card's height is never a grid row span (pitfall 75).
- Happy-dom does no layout: check geometry in the browser spec.
- `vue-tsc -p tsconfig.app.json` is the fast typecheck; the lint rules (types only in `src/types/`) run in `npm run build`.
- A card that links from a stack page needs an entry in `BOARD_HINT` with its board and card id.

## First file to open
`frontend/src/components/dashboard/cardRegistry.ts` (the catalogue), then `frontend/src/components/dashboard/DashboardGrid.vue` (the flow) and `docs/decisions/ADR-0065-...`.
