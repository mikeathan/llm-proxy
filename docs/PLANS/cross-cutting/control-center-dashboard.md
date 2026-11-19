---
status: partial
date: 2026-11-19
last_reviewed: 2026-11-19
related_specs: [SPEC-003, SPEC-006]
constitution_references: [IV.6, I.16]
evidence: ADR-0022 and ADR-0028 ("the overview comes with a third" stack); ADR-0052 (Radio is the third stack); ADR-0061 (the join is only in core/devicejoin); frontend/src/router/index.ts (/ redirects to the assistant, /overview is host health and runs); views/NetworkView.vue and views/RadioView.vue (the tabs and what each mounts); the import tree of components/netaudit and components/radio (who mounts whom, read 2026-11-19); useNetworkOverview.ts, useRadioListening.ts, useMetrics.ts (each its own poll); services/stacks/stacksService.ts (stack(id) only, no list call); frontend/package.json (no grid or drag library); usePersistedState.ts. Five static mocks (A to E) in control-center-dashboard-mocks/, made-up data.
---

# One dashboard of boards and cards (2026-11-19)

The owner asked (2026-11-18) whether the Network and Radio pages, each with its own tabs, views and settings, could instead feed one dashboard.
After reviewing mocks A to D, the owner chose a direction on 2026-11-19: **a Dashboard page whose tabs are boards, and whose boards are made of cards.** The owner
answered the open questions the same day (below) and asked for the work in one branch, `control-center-boards` (v0.105.0). What is built, and what is left, is
in `control-center-dashboard-handoff-2026-11-19.md`.

## The model

| Term | Meaning |
|---|---|
| **Dashboard** | The page. |
| **Board** | A tab of the Dashboard. **Home** is a mixed board. A stack ships its own board (Network, Radio, Agents; also LLM and host, Automations) with a good default layout; it is an ordinary board, not a second system. People can add, rename, remove and re-order boards, and add a blank one. |
| **Card** | A tile. Any source can have cards: LLM and host, Assistant, Automations, Reminders, Network, Radio, Agents. Home can hold a card from any source; the cards offered are those whose stack and module are switched on and ready. Every card shows its source in its header. |

- **One engine.** Every board uses the same card definitions, the same 12-column grid, the same sizes (S, M, M+, L, Full; 1x or 2x tall) and the same layout
  presets. A card is defined once; a board only places it. "Pin to Home" copies a card from a stack board to Home at a small size.
- **A card adapts to its room.** Small shows the headline; medium adds a trend or a short list; large shows the full detail (container queries, no script).
- **Stacks are control and settings.** A stack page keeps what only it can do: switch modules, start and stop, set limits, record decisions. The views move to cards.
- **Saved per person**, per browser first (usePersistedState). Most people only ever edit Home; stack boards come ready-made. The cost of the model is two levels of
  customisation (boards, then cards); good shipped defaults are what keep it cheap.

Mock to open: `control-center-dashboard-mocks/e-boards.html` (the others are history: A stacked, B grid, C device-first, D cards with layout presets).

## Rules the design must keep (verified)

1. **No switches or settings on the dashboard.** ADR-0028: each stack's Modules tab is the one place to switch a module; ADR-0022: a stack keeps its settings on its own page. A card shows a verdict, figures and a link. Actions that need the passphrase (starting Radio listening) and decisions (name a device, "This is fine", "Mine" or "Not mine") stay on the stack page.
2. **Every card links to the exact control section that acts on what it shows**, not to a stack's front page: a problem's "Fix" opens the section that fixes it; "Start listening" opens the listening control. The card header's arrow, a link at the top of each stack board ("Network control and settings"), and an "Open board" link in each stack page's header close the loop in both directions; the sidebar keeps its Stacks entries.
3. **A joined view takes its join from the core.** ADR-0061 and IV.6: only `core/devicejoin` sees both Network and Radio, so a joined card is a Network and Radio feature and reads one core endpoint. Another pair of stacks needs its own core endpoint; the browser never joins.
4. **No new dependency without approval.** `package.json` has no grid or drag library and AGENTS.md makes one ask-first. Container queries and a small hand-written drag (as in the mock) are enough.
5. Owner preferences on record: no more noise, one path per job, Simple and Advanced, mobile, and remove what is useless or repeated.

## What moves, what stays (stack pages)

From `views/NetworkView.vue` and `views/RadioView.vue`:

| Stack | Becomes cards (views) | Stays on the stack page (control, settings, decisions) |
|---|---|---|
| **Network** | Overview (posture, four figures, what changed, standing, coverage, attack paths), Timeline, Traffic by device (headline, map, places), Devices presence | Modules, Settings (every group), Audits (set up and schedule), Devices and Findings as decision screens (name a device, "This is fine"), Traffic router setup, Trace a route, Help |
| **Radio** | Around you (radar, what changed, channels, findings), My devices (read), recently heard | Listening (start, stop, scan plan, keeping), Sensors, Modules, the decision flows (Help it learn, Find my device, confirm a Wi-Fi network) |

Open point for the owner: Network's Devices and Findings tabs mix reading with a decision on each row. This plan keeps them as the decision screens and puts a read-only
summary card on the dashboard; the alternative is a card with the decision buttons, which breaks rule 1.

## Component audit (keep, merge into a card, drop)

Method: the import tree (who mounts whom) and each component's own header comment, read 2026-11-19. It is a proposal from the source, not from running the screens; each
row is confirmed against the running page when its slice is built. **Keep** the radar, the location map and the interactive components the owner named
(`RadioRadarCanvas`, `RadioAroundRadar`, `TrafficMap`, `TrafficDeviceRail`, `TrafficDevicePane`).

### Repeats found

| Repeat | Where | Proposal |
|---|---|---|
| Four device lists | `DevicesCard` (Network), `NearbySection` (Network), `TrafficDeviceRail` (Network), `RadioMyDevices` (Radio) | Keep one device row and one list component fed by the core join (ADR-0061), shown on the stack pages and as an optional Devices card; retire the other lists once it covers their facts. Largest saving in this audit. |
| Two lists of Wi-Fi networks | `RadioViewNetworks` (Around you, view "Networks") and `RadioNetworks` (My devices, "Your Wi-Fi networks") | **Checked when built: not a duplicate.** One is a view (the routers around the house with the devices on each) and stays on the radar card as its Networks view; the other holds the decisions (confirm a name, Not mine, the guest switch) and stays on My devices. Both kept. |
| Two channel pictures | `RadioAirwaves` (channel bars, retries) and `RadioViewChannels` (by channel) | One channels card on `RadioChannelBars`. Drop one. |
| Three ways to say "this is mine" | `RadioAskCard` (Help it learn), `RadioFindMine` (Find my device), add-by-address in `RadioMyDevices` | **Built (owner agreed 2026-11-19):** one section, `RadioClaim` (Claim your devices), on My devices, holding Find my device and Help it learn; add-by-address stays folded under Advanced. The three still write the same records through `useRadioLive`. |
| The status line twice | `RadioAroundStatus` (Around you) and `RadioListening` (Listening) | Kept on the radar card for now (it also says when the picture is stored, not live); to be read again with the Listening page. |
| A link that only points to another tab | `RadioAroundLink` (Listening to Around you) | **Dropped** (built): the stack pages carry one line linking to their board (`StackBoardLink`). |
| Notes as a disclosure | `RadioReadGuide` | Fold into the radar card's help. |
| The overview twice | `NetworkOverviewPanel` with `StandingCard`, `CoverageStrip`, `AttackPathsCard` (Overview tab) and `AuditOverview` (inside Findings) | **Built:** the parts are the Network board's cards, the tab and `NetworkOverviewPanel` are gone. `AuditOverview` stays inside the saved report. |
| Two renders of one report | `FindingsSimple` and `FindingsAdvanced` | Keep: it is the owner's Simple and Advanced choice and both read the same data. |

### The rest, by decision

| Becomes a card | Keep on the stack page | Look at again when built |
|---|---|---|
| `NetworkOverviewPanel`, `StandingCard`, `CoverageStrip`, `AttackPathsCard`, `TimelineTab`, `TrafficHeadline`, `TrafficChart`, `TrafficMap`, `RadioRadar` (with `RadioAroundRadar`, `RadioRadarCanvas`, `RadioViewChanges`, `RadioFilterChips`, `RadioKindPick`), `RadioFindings` | `NetworkSettings` and every settings card (`HelperCard`, `KnownBadCard`, `VulnData*`, `UbuntuDataCard`, `DnsCheckCard`, `DecoysCard`, `FootprintCard`, `WifiCard`, `Scan*Settings`, `RouterLogCard`, `DestinationDataCard`, `OutsideLookupCard`), `AuditsPanel` and `AuditList`, `FindingsReport` and what it mounts, `TrafficRouterSetup`, `TrafficTraceCard`, `RadioListening`, `RadioWindows`, `RadioScanPlan`, `RadioKeeping`, `RadioSensors`, `StackModules`, `ModuleTile` | `TrafficReport`, `TrafficFlows`, `TrafficTalk`, `TrafficWhere`, `TrafficKinds` (13 components behind one tab; some are the "this computer" report only), `CaptureWindowCard`, `DeviceCheckSettings`, `RouterBlocksCard`, `NearbyCard` |

Not in this audit: the 40 other netaudit components and the radio device page (`RadioDeviceView`, `RadioTrack`), which stay as they are until their slice.

## Where extra work and logic is needed

- **Card contract.** Each stack declares its cards and when they show (module on and ready), in the stack manifest or a frontend registry keyed by stack and module. Touches SPEC-003. `StacksService` has `stack(id)` only; a list of enabled stacks is needed.
- **Polling budget.** `useNetworkOverview` starts its own refresh timer, `useRadioListening` polls every 15 s while a run is on, `useMetrics` is shared by a mount count. Many cards mean many timers: a card on a board that is not open, or scrolled away, stops polling; a light summary endpoint per stack may be better than reusing each page's reads.
- **Where the layout is saved.** Per browser through `usePersistedState` (no backend change), or on the server so it follows the owner to another device (a config-persistence change; ask first). Recommended: per browser first.
- **Joined device facts.** A full device view needs the richer core endpoint (ADR-0061); a cross-stack timeline needs new kept history, because the presence check keeps no departure times (ADR-0061, Not done).
- **One home per fact.** The look-alike Wi-Fi finding is one finding owned by the core (ADR-0063), so it shows in the Network card, not Radio's.
- **Card states.** Each card needs the states the pages already have: off, not set up (with the fix), loading, unreadable, and Simple or Advanced.
- **Tests and docs.** Visual specs for the dashboard on desktop and phone; a SPEC-003 amendment; an ADR for "the dashboard shows and links, it never switches".

## Decisions (owner, 2026-11-19)

1. **Home is the landing page** (`/` opens the dashboard).
2. **Decisions stay on the stack pages**: Devices and Findings (Network), Listening, My devices and Sensors (Radio), Modules and Settings. The stack page keeps three jobs (decide, set up, settings) and shows nothing a card shows.
3. **Claiming devices is one section** (built).
4. **The layout is on the server**, for the signed-in owner (sign-in is one passphrase, so one layout per server).
5. **One branch** for the whole slice; PR and handoff at the end.

## Remaining Work

Built on the branch: the server layout and its routes, the board and card engine, Home and the host, automations, reminders, Network (six cards) and Radio (two cards) boards, the Network and Radio page changes, tests and browser specs. Left, in order, with what each needs: see `control-center-dashboard-handoff-2026-11-19.md`.
