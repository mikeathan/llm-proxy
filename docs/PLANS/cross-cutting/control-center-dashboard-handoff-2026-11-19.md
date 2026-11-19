---
status: partial
date: 2026-11-19
last_reviewed: 2026-11-19
related_specs: [SPEC-003, SPEC-006]
constitution_references: [IV.6, I.16]
evidence: branch control-center-boards (cut from main at f96d25d4, v0.103.0); ADR-0064; SPEC-003 2.67; plan control-center-dashboard.md; frontend/src/components/dashboard/, domain/dashboard.ts, composables/dashboard/, views/DashboardView.vue; backend/internal/core/dashboard/, transport/http/handlers/dashboard_handlers.go; e2e/dashboard.visual.spec.ts
---

# Dashboard of boards and cards: handoff (2026-11-19)

## State
- **Branch** `control-center-boards`, cut from `main` at `f96d25d4` (v0.103.0), version bumped to v0.105.0. The pull request is in the tracker row. It does **not** contain the 9 commits of `docs-plans-tracker-cleanup` (PR #270, which takes v0.104.0, ADR-0062 and ADR-0063); this branch's ADR is **0064** and its version **0.105.0** so the two do not collide. If this merges first, v0.104.0 is skipped; if #270 merges first, nothing changes. Either order merges: they both touch `docs/PLANS/README.md` and `docs/subsystems/admin-ui.md` (keep both rows) and `frontend_dist/index.html` (rebuild after the merge).
- **Not run on vertex.** The dashboard was run in a browser against mocked answers (`e2e/dashboard.visual.spec.ts`), never against the real server and real data.

## Built
- **Server:** `GET`, `PUT`, `DELETE /admin/api/dashboard` (`core/dashboard`, `persistence.DashboardStore`, `handlers/dashboard_handlers.go`), validated and bounded, forgotten by a factory reset, with tests at each layer.
- **Engine:** `domain/dashboard.ts` (pure layout rules), `useDashboard` (edit at once, saves in order, revert and toast on failure), `DashboardView` with board tabs, toolbar, grid, card frame, catalogue, drag and keyboard move, layouts, pin to Home and `?board=`.
- **Cards (12):** host health, running now, recent runs, reminders, network posture, what changed, figures, what is watching, ways in, standing, radio around you (the radar), radio findings. Each shows a small, medium or large version from its own width.
- **Stack pages:** Network lost its Overview tab (Devices is the page without a tab); Radio lost Around you and gained **Claim your devices**; both have a line to their board. `/dashboard` replaced `/overview` and is where `/` lands; the phone bar is Dashboard, Assistant, Workspace, Automations, More (Reminders moved under More).
- **Removed:** `OverviewView`, `NetworkOverviewPanel`, `RadioAroundLink` and their tests.

## Checks
The commands run and their results are in the pull request description. Backend: `go build ./...`, `go test ./...`, `go run ./tools/check-complexity/`. Frontend: `npm run build` (lint, types, build), `npm test`, `npm run test:visual`. Harness: `./scripts/check-agent-harness.sh`.

## Left, in order
1. **Run it on vertex** (owner): open `/dashboard`; look at Home, the Network and Radio boards with real data; resize a card; reload; open it from a second browser (the layout should follow). Needs: the build deployed. First command: `cd frontend && npm run build`, then the usual deploy.
2. **Cards the stack pages still own as views** (each its own slice: move the view to a card, remove the page copy): Network **Timeline** (`TimelineTab`) and **Traffic** (headline, by-device map and places; the router setup and Trace a route stay), Radio **My devices** (the read part) and **recently heard**. Pin to Home works for any card in the catalogue.
3. **One device list.** Four lists remain (`DevicesCard`, `NearbySection`, `TrafficDeviceRail`, `RadioMyDevices`). Make one row and one list component fed by the core join (ADR-0061), offered as an optional **Devices** card in the Network catalogue; retire the others once it covers their facts. Needs: the owner's check of what each list shows today that the others do not.
4. **Offer only cards whose stack or module is on.** The catalogue lists every card; a card whose stack is off shows its own off or empty state. The design's contract (a stack declares its cards and when they show) needs a list of enabled stacks (`StacksService` has `stack(id)` only).
5. **Cards for the sources the mocks had and the catalogue has not:** assistant (approvals waiting, recent conversations), agents, the LLM queue. The engine takes each with one catalogue entry.
6. **Polling.** `useNetworkSnapshot` shares the Network reads; the radar card polls its own (15 s); Host health shares `useMetrics`. A card on a board that is not open is not mounted and does not poll; a card scrolled out of view on the open board still does.

## Traps
- A `Panel` inside a card draws only its content (`IN_DASHBOARD_CARD`): a card built from an existing panel component needs no edit, but the title and actions are the card's. Pitfalls 72 and 73.
- The card's version is chosen by its measured width, not CSS (`useCardSize`); in a test it is large unless a `ResizeObserver` and `matchMedia` are stubbed (`DashboardCardFrame.component.test.ts`).
- A browser check that counts cards must wait for them: they load as they are first shown.
- The `What is watching` card is empty while the modules cannot be read (`CoverageStrip` draws nothing without modules); give it its own line in slice 4.
- A test that mounts a card with a router link needs the real route table (`createAppRouter`), not a one-route stand-in.

## First file to open
`frontend/src/components/dashboard/cardRegistry.ts` (the catalogue), then `frontend/src/domain/dashboard.ts` (the rules) and `docs/PLANS/cross-cutting/control-center-dashboard.md` (the design and the audit).
