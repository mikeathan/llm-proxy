---
status: partial
date: 2026-11-11
last_reviewed: 2026-11-12
related_specs: [SPEC-003]
constitution_references: [I.8, I.14, I.15]
evidence: owner request 2026-11-11 (pasted router read for "M4 MacBook air 192.168.50.251", 171 connections, 79 places; "the map has disappeared"); frontend/src/components/netaudit/TrafficTab.vue, TrafficReport.vue:96-117, TrafficRouterConn.vue, TrafficMap.vue, TrafficTraceCard.vue, composables/netaudit/useRouteTrace.ts; backend/internal/core/netaudit/routerconn/parse.go; web research listed below. Option D chosen by the owner 2026-11-12; Phase 1 built in v0.76.0 (not run on vertex).
---

# Network · Traffic: one page organised by device, with the map and the route for whatever is selected

## Status (2026-11-12)

The owner picked **option D** of the mockups: B's big map with A's device list, which searches and sorts so 30+ devices work, and on a phone becomes a
device picker with the places opening under the company. **Phase 1 is built (v0.76.0, frontend only, branch `router-setup-change-address`, uncommitted,
not run on vertex):** the shared `PlaceRow` model, the map and company groups for router devices, the inline route, `TrafficDeviceRail` / `TrafficDevicePane` /
`TrafficPlaceGroups` / `TrafficGroupPlaces` / `TrafficDeviceNames` / `TrafficRouterSetup` / `TrafficHeadline`, and the retirement of `TrafficOtherDevices`,
`TrafficRouterConn` and `TrafficLookups`. Recorded in SPEC-003 2.64. **Left out of Phase 1 on purpose:** the "New today" filter and stat and any per-device
history (they need decision 3 below), the mockup's home dot (the program does not know where the network is), and the "Trace any address" control moved
to a header menu (the card stays where it was). "Needs a look" uses the flags the phone-home watch already raises, joined by hardware address. **Next:**
the owner's run on vertex, then Phase 2 once decision 3 is answered; decisions 1 and 2 are answered (device-first, company by default).

**Phase 2 built too (v0.77.0, 2026-11-12, not run on vertex), after the owner's yes to decisions 3 and 4 (the second on condition that no agent can
branch the security layer):** the Router connections module, a bounded history of companies and countries, "New today" on the page, the nightly read in the
report's own code, and findings `router_new_company` / `router_new_country`. ADR-0048, Constitution I.15 amended, SPEC-003 2.65. The proposal's idea of
matching connections against the known-bad list by address is **dropped**: that list holds names only. Left: the owner's run on vertex (switch the module
on, press Read the router, wait a day for the first learning period to end), then judge the learning day and the findings' noise; phases 3 and 4 below are
otherwise done or dropped (device footprint line and the assistant tool are not built).

## The proposal (as put to the owner)

## What is wrong today (verified in the code)

1. **The map belongs to the wrong data.** `TrafficReport.vue:96-117` draws the map only from `summary` (the last look at *this computer's* packets) and
   only when that look had internet rows. The router read (every other device, 171 connections for the owner's Mac) has `owner.point` on each place
   (`RouterPlace.owner`, `types/netaudit.ts:1336`) but `TrafficRouterConn.vue` never passes it to `TrafficMap`. With no look, or an idle one, the
   map is simply absent: that is "the map has disappeared".
2. **Three separate islands on one scroll:** the trace card, the look report (headline, map, talk list, kinds, conversations) and "Other devices"
   (two views, lookups / connections). The same question, "where did this device go?", is answered in different panels with different rows.
3. **The trace result is detached from where you pressed Trace.** `requestTrace` only prefills the card at the top (`useRouteTrace.ts`); the
   operator loses their place in a 79-place list, and the route shows on the card's own map, not next to the place.
4. **The list is flat and long.** A device with 79 places shows 10, sorted by connection count, one row per address:port:proto. Google
   alone is five rows (142.251.155.120, .119, 142.251.30.95 ...). Nobody reads that; people look for *who* (Google, Alibaba, Anthropic), not addresses.
5. **Router data is not used by the audit at all** (`routerconn` is imported only by the app wiring and its handler; no finding reads it).
   The known-bad list (`knownbad`) matches names only, so a connection to a listed address is never flagged.

## What professional tools do (and what to copy)

| Product | Pattern | Take |
|---|---|---|
| [GlassWire](https://www.glasswire.com/userguide/) | Traffic Monitor with three views of the *same* selection: graph, usage table, geo map. Click a peak: it freezes and lists the apps and hosts. Table ranks by app, host, type, country. Country filter. | One selection drives every view. Rank by who, not by address. |
| [Little Snitch Network Monitor](https://help.obdev.at/littlesnitch6/lsm-overview) / [map](https://help.obdev.at/littlesnitch5/lsm-map-view) | List on the left, map on the right. Regroup the list (Process > Domain > Server, or Country > Process > Domain). Selecting a process or connection limits the map to its routes; clicking a place on the map filters the list; clicking it again clears. Servers with no location are put in a marked placeholder spot so they are never mistaken for real. | **List and map as one linked control.** Regroupable list. Honest "no location" bucket. |
| [PingPlotter](https://www.pingplotter.com/fix-your-network/getting-started/the-trace-graph/) | A hop table (address, loss, latency) beside a latency graph, plus a per-hop timeline; double-click a point to focus the trace on that moment. | The hop table is the primary view of a route; a map is secondary. Repeat mode (already built) gets a timeline later. |
| [Firewalla](https://help.firewalla.com/hc/en-us/articles/24739086338323-Firewalla-Feature-Network-Flows) | Device list first; open a device for its flow chart (60 min / 24 h / 30 d) and Network Flows, filterable by time. | **Device is the entry point**, with a time window. |
| [UniFi Flows](https://help.ui.com/hc/en-us/articles/32201256219799) | One flows table with preset views (All, Blocked, Threats) and saved filters. | Preset filters like "Flagged" and "New today" instead of a settings maze. |

(Fing's device screen and UniFi's per-client country view were not found; not relied on.)

## Proposed design: "Who is talking to whom" as one master/detail page

Entry is the **device**, because that is how people think ("what is my phone doing?"). Everything else is a view of the selected device.

```
Traffic                                   [Read the router]  [Listen on this computer]   Simple | Advanced
┌ Devices (left, 1 column) ┐┌ M4 MacBook Air · 192.168.50.251 ─────────────────────────────────────────────┐
│ ▣ This computer (vertex) ││ 171 connections · 79 places · read 2 min ago                                  │
│ ▣ M4 MacBook Air   171   ││ ┌ Map (all places of this device; click a dot to filter; "no location" tray) ┐│
│ ▣ Pixel 8           62   ││ └────────────────────────────────────────────────────────────────────────────┘│
│ ▣ Smart TV          14 ⚑ ││ Group by: [Company] Country  Service      Filter: [All] Flagged  New  Search  │
│ ▣ ... (search)           ││  Google LLC            6 places · 61 conn   ▸                                  │
└──────────────────────────┘│  Alibaba (US)          2 places ·  9 conn   ▸  Frankfurt, Germany              │
                            │  Anthropic, PBC        1 place  ·  4 conn   ▸  San Francisco                   │
                            │   ▾ opened: each address:port proto, state, [Route]                           │
                            │ ┌ Route to 160.79.104.10 (opens here, under the row, not at the page top) ─────┐│
                            │ │ hop table + same map with the line; Repeat; Reached/Not reached               ││
                            │ └────────────────────────────────────────────────────────────────────────────┘│
                            │ Lookups (names it asked the router for) · same device, tab: Connections | Names│
                            └───────────────────────────────────────────────────────────────────────────────┘
```

Rules:

1. **One selected device drives everything on the right** (list, map, route, lookups). "This computer" is one more device in the list, with
   its own look; its packets and the router's table become two *sources* of the same row model (below), shown with a small source tag.
2. **Group by company by default** (owner name from IPtoASN, already in `owner`), switchable to country or service port. A group row shows places,
   connections, the city or "N cities". Opening it lists addresses. This turns 79 rows into about 15 readable ones.
3. **Map is always shown for the selected device** when any place has a point; places with no point sit in a labelled "No location" tray
   (Little Snitch's placeholder idea) and the page says how many. Click dot = filter the list, click again = clear; select a row = highlight
   its dot. Dot size = connections (the router has no bytes), and the legend says so.
4. **Route opens inline under the row you pressed**, uses the same map (line drawn) and a hop table first (PingPlotter), with the existing
   repeat button. The "Trace a route" card for an arbitrary typed address stays, but as a small "Trace any address" control in the page header
   menu, not a full-width panel.
5. **Presets, not settings:** `All · Flagged · New today · Unlocated`. "New today" needs a stored previous read (see Phase 2).
6. **Setup moves out of the way:** the router's 3-step setup becomes a collapsed card shown only while not ready; when ready, the page has one
   "Read the router" button, and the fingerprint line goes to Advanced.
7. Simple shows: device list, map, company groups. Advanced adds: per-address rows, ports, state, hop statistics, source tags, the per-second chart.
8. **Mobile:** the device list becomes a top selector (a sheet); map above, groups below; route in a bottom sheet.

### Shared row model (the refactor that makes this cheap)

One `PlaceRow { address, port, proto, count, bytes?, owner, source: 'host'|'router', deviceKey }` produced by two small adapters (from `HostTraffic` and from `RouterDevice`). `mapDots`, the company grouping and the detail all take `PlaceRow[]`. Today `mapDots` takes `TalkRow` (host only), which is why the router data cannot reach the map. New components: `TrafficDeviceList`, `TrafficDevicePane`, `TrafficPlaceGroups`, `TrafficInlineRoute`; `TrafficMap` gains `selected` and `unlocated` props and a `pick` that toggles. Retire `TrafficOtherDevices` panel and the top-level `TrafficReport` map panel after the move.

## How this plugs into the Network audit

The page is only a lens. The value for the audit comes from three small, additive uses of data already collected. None sends a new packet.

1. **Flag known-bad by address, not only by name.** Run each router place's address through the abuse.ch list's addresses (the list already holds them,
   `knownbad`) and through the blocked list; a hit becomes a finding "*Device X connected to a listed address*" and a ⚑ on the device row.
2. **Router-read history -> "new place" / "new company" / beaconing findings.** Keep, per device, the set of owners and (address, port) seen across reads
   (bounded, like the phone-home watch's 256 names). A device that suddenly talks to a new country or a new company, or to the same address at a fixed
   rhythm, gets the existing `new_place` / `beaconing` flags that the lookups view already has. This ties the two sources: *what it asked for* (names)
   with *where it connected* (addresses), shown in the same device pane, and a name that resolved to a listed address is stronger evidence than either.
3. **Device "footprint" for the findings tab and the assistant:** a one-line summary per device ("62 places, 9 companies, 4 countries, 1 unlocated")
   in the Devices tab card and in the `network_findings` assistant tool, so a finding's detail can link to the device pane with the map pre-filtered.
4. **Scheduled read (optional, needs an owner decision):** today a read is only on the operator's press (I.15). The history in (2) only works with
   reads over time; the options are a manual "Read now" (no amendment), or a nightly read inside the audit run (amends I.15: the router is read
   by the audit). Recommend starting manual and deciding after the owner has used it for a week.

Findings must stay honest: a city is approximate, a company is the registered owner of the address, a connection is not a transfer of data
(no byte counts from the router), and the router's table holds only live connections at the moment of the read.

## Phases

| Phase | What | Needs | Checks |
|---|---|---|---|
| 0 | Mockup of the master/detail page (Simple + Advanced + phone) for the owner's pick. | owner picks | none |
| 1 (built v0.76.0) | Shared `PlaceRow` model and adapters; map for router devices; company grouping; inline route. Frontend only. Fails first: a router place with a point appears on the map; map shows without a host look. | nothing | vitest, build, visual specs |
| 2 (built v0.77.0) | Per-device history of router reads (bounded), "new today" preset, `new_place` flags for addresses. Backend + UI. | owner OK on stored history (SPEC/ADR-0047 amendment) | go test, complexity |
| 3 (partly built v0.77.0) | Findings: new company/country built; listed address dropped (the list has no addresses); device footprint line and assistant tool not built. | Phase 2 | go test |
| 4 (partly built v0.77.0) | Scheduled read built (the audit's own code, module switch, I.15 amended); per-hop latency timeline not built. | owner decision (I.15), given | |

## Decisions for the owner

1. Is **device-first master/detail** the right entry, with "this computer" as one of the devices? (Recommended: yes.)
2. **Group by company** by default in Simple? (Recommended: yes; per-address rows in Advanced.)
3. Allow Phase 2's **stored read history** (addresses, ports, owners per device, bounded, no payload, nothing leaves this computer)?
4. A nightly router read inside the audit, or manual only for now? (Recommended: manual first.)

## Sources

[GlassWire user guide](https://www.glasswire.com/userguide/) · [Little Snitch Network Monitor](https://help.obdev.at/littlesnitch6/lsm-overview) and [map view](https://help.obdev.at/littlesnitch5/lsm-map-view) · [PingPlotter trace graph](https://www.pingplotter.com/fix-your-network/getting-started/the-trace-graph/) · [Firewalla network flows](https://help.firewalla.com/hc/en-us/articles/24739086338323-Firewalla-Feature-Network-Flows) · [UniFi Flows](https://help.ui.com/hc/en-us/articles/32201256219799). Search-result summaries, not hands-on use of each product.
