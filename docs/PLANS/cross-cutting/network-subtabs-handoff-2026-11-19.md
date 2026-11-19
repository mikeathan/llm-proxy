---
status: partial
date: 2026-11-19
last_reviewed: 2026-11-19
related_specs: [SPEC-003]
constitution_references: []
evidence: Built 2026-11-19 on branch network-subtabs-and-scrollbar-fix from origin/main c91f1d95 (v0.112.1), one commit, pushed, PR open. Checks run - npm test (348 files, 3365 tests, exit 0), npm run build (lint, palette, contrast, type-check, exit 0), the whole Playwright suite (422 passed), go build, go test ./..., check-complexity (no new violations), check-agent-harness. Not run - anything on vertex, a browser at the owner's 2000 px screen with real data.
---

# Network sub-tabs and the scrollbar fix: handoff

**Status:** partial (2026-11-19). Decision: ADR-0070 (proposed); contract: SPEC-003 2.74.

## Branch and state
Branch `network-subtabs-and-scrollbar-fix`, one commit on `origin/main` c91f1d95, pushed, PR open. Merge it on its own; it depends on no other branch. The built `backend/internal/transport/http/frontend_dist/index.html` is committed with it. VERSION is 0.113.0.

## Built
- **Scrollbar flicker** (the owner's report on the big screen): `html { scrollbar-gutter: stable }` and the same on a card's scrolling list. Pitfall 76.
- **Network is one tab with five sub-tabs** (Overview, Devices, Traffic, Findings, Activity), Simple/Advanced and Help at the end of the sub-tab row. The group is a field of the shipped template (`boardTabs`, `groupOf`); the stored layout is unchanged. Opening a sub-tab the saved layout lacks adds it (when its stack is on).
- **13 cards became 6:** new `network-overview` (posture, what changed, figures, standing, what is watching); Devices has *Nearby, not on your list*; Traffic carries the trace; Findings has *Ways in*; Audits and Timeline stay. Eight ids retired: posture, changes, figures, standing, coverage, nearby, trace, attack-paths.
- **Settings → Watching split** into *Devices & Wi-Fi* (`devices`) and *Router* (`router`); `?group=watching` is gone, every link and `MODULE_SETTINGS_GROUP` use the new ids.
- `MAX_BOARDS` 12 to 20 (frontend and `core/dashboard`).

## Measured (Playwright, 2000×1300, mocked data, so floors)
The Network board was 2787 px (2.1 screens). Each sub-tab is now 1300 px (one screen) with the same mocks. Real device, finding and traffic lists make every page longer; look at it with the owner's data.

## Left, in order
1. **Owner: look at it on the real server with real data** at the big screen and on a phone, and confirm the scrollbar no longer flickers (first file to open: `frontend/src/components/dashboard/cards/NetworkOverviewCard.vue`).
2. **Owner: Reset board on Network → Overview once.** A layout saved before this keeps its Network board: the eight retired ids are dropped when it loads, the rest stay, and the dashboard offers "Network overview" once. Looks like five big cards plus a callout until reset (screenshot taken in the review). Nothing is rewritten for the owner.
3. **Owner decisions:** accept or change ADR-0070; the four device lists are still not merged (ADR-0065 decision 6; Nearby joined Devices as a view, Radio My devices and Traffic's picker stay apart); whether the repeated labels on a one-card sub-tab (the tab, the pill, the card header and the NETWORK chip) should lose the chip.
4. Radio could use the same `group` mechanism when it grows; it is one screen today.

## Traps
- The stored board has no group or source: grouping is read from the shipped template by id, so an owner's own board is never grouped and a renamed shipped board keeps its id.
- `?board=` must name the sub-board that holds a card; use `toNetworkCard` (`NETWORK_CARD_BOARD`), never a hand-built query.
- A `Panel` in a card draws only its content (pitfall 74): the overview uses plain `MicroLabel` headings; Devices, Traffic and Findings provide `IN_DASHBOARD_CARD=false` for their own panels.
- Local `main` was stale; the PR is based on `origin/main`.
