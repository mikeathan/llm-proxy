---
id: ADR-0070
title: Network is one dashboard tab with sub-tabs, one card per question, and the settings are grouped by what they watch
status: proposed
date: 2026-11-19
subsystems: [admin-ui, network-audit]
supersedes:
superseded_by:
---

# ADR-0070: Network is one dashboard tab with sub-tabs, one card per question, and the settings are grouped by what they watch

## Context
The Network board (ADR-0064, ADR-0066) held thirteen cards in one column: nine of them full width, 2787 px tall at 2000 px wide with almost no
data (measured with Playwright, mocked overview), more with real lists. The owner called it unusable: "you keep scrolling on and on". Five
cards (posture, figures, what changed, standing, ways in) read the same `/netaudit/overview` through `useNetworkSnapshot`, so they were five
views of one read. The owner ruled out more root-level tabs ("adding more and more to top level is not good"). Evidence: card sources read from
the code (2026-11-19); `DashboardBoard` stores only `{id, name, cards}`, so a board has no stored source or group; `MAX_BOARDS` was 12.

## Decision
1. **One tab, sub-tabs under it.** A stack that ships several boards shows one tab in the strip and a row of sub-tabs when it is open. The
   group is a property of the shipped template (`DashboardBoardTemplate.group`), not of the stored layout: boards stay ordinary boards, the
   layout format and the server contract are unchanged. Every board the stack ships is a sub-tab, present on the saved layout or not; opening one
   that is missing adds it (as following a stack's link already did). A shipped sub-board is not renamed or removed on its own.
2. **Network ships five boards:** Overview (`network`, the id the stack's links use), Devices, Traffic, Findings, Activity. Simple/Advanced and
   Help sit at the end of the sub-tab row.
3. **One card per question.** Eight cards are retired and one is new: `network-overview` (verdict and next step, what changed, figures and
   trend, standing, what is watching) replaces posture, changes, figures, standing and coverage; **Devices** has a switch to *Nearby, not on
   your list* (retires `network-nearby`); **Traffic** carries the route trace (retires `network-trace`); **Findings** has a switch to *Ways
   in* once an audit has run with the module on (retires `network-attack-paths`). Audits and Timeline stay. `NETWORK_CARD_BOARD` maps a card to
   its sub-tab so `toNetworkCard` and the old `/network/<tab>` addresses open the board that holds it.
4. **Settings → Watching splits into two groups:** *Devices & Wi-Fi* (device checks, decoys, nearby, Wi-Fi look-alikes) and *Router* (router
   log, router connections, DNS check). `?group=watching` is gone; `MODULE_SETTINGS_GROUP` and every link use the new ids.
5. **`MAX_BOARDS` is 20** in the frontend and in `core/dashboard`, so five Network sub-boards leave room for the owner's own.
6. **Not merged:** the four device lists (ADR-0065 decision 6 stands). Nearby joined the Devices card as a view because its rows carry the
   inventory's `mac`, `name` and `known`; Radio's My devices and Traffic's device picker stay separate.

## Consequences
- The page is one screen per sub-tab at 2000×1300 with the mocked data (Overview, Devices, Traffic, Findings, Activity all 1300 px, from 2787 px).
- A layout saved before this keeps its Network board: the retired card ids are dropped when it loads, the rest stay, and the dashboard offers
  the new Overview card once. **Reset board** (Network → Overview) restores the shipped arrangement. Nothing is rewritten for the owner.
- `dashboard-offered-cards` and saved boards that hold a retired id lose that card silently (pruning); named in the PR.
- A stack with sub-tabs (Radio later) needs only a `group` on its templates and a label in `GROUP_LABELS`.

## Alternatives considered
- **A root-level tab per Network board.** Rejected by the owner: the strip grows with every stack.
- **Sections inside one board** (a `section` on a placement). Needs a stored-layout change and Go validation, for the same screen.
- **Hide cards in Simple mode.** Hidden knowledge (a card that is there in one mode and not in another) with no way to see why.
- **Merge the four device lists into one.** Left for the owner (ADR-0065 decision 6); the sub-tabs make the length problem go away without it.

## Links
- Plan handoff: `docs/PLANS/cross-cutting/network-subtabs-handoff-2026-11-19.md`; SPEC-003 2.74; code: `frontend/src/domain/dashboard.ts` (`boardTabs`),
  `components/dashboard/DashboardBoardTabs.vue`, `cards/NetworkOverviewCard.vue`.
