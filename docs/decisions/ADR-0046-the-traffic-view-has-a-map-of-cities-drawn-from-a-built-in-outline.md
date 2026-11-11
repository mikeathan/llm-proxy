---
id: ADR-0046
title: The Traffic view has a map of cities, drawn from an outline built into the page
status: proposed
date: 2026-11-11
subsystems: [network-audit]
supersedes:
superseded_by:
---

# ADR-0046: The Traffic view has a map of cities, drawn from an outline built into the page

## Context

ADR-0043 rejected a world map ("false precision for CDN-served traffic, a new dependency"). Since then the city table (ADR-0044) gives a city and a point per address, and
the route trace (ADR-0045) gives a city per router. On 2026-11-11 the owner said the Traffic page shows "only one location", that the downloaded tables seem unused, and
chose to add a map. The look on vertex that day held one internet address, used in Toronto and registered in the United States; the screen said only "United States".

## Decision

We will draw a world map on the Traffic tab when a look has internet places: one dot per city (joined by city, sized by traffic, the open place marked), and, for an
open place that has been traced, a line through its routers that have a city. The land is one SVG path generated once from Natural Earth's 1:110m land (public domain,
as packaged by world-atlas 2.0.2), equirectangular, Antarctica left out, in `frontend/src/constants/worldLand.ts` (47 KB); no map library and no tiles are fetched. The
city table keeps each place's point to a tenth of a degree (index version 2, ADR-0044 amended), which is all a world map can show. The screen says a dot is a city, never
a machine, and that the line starts at the first router with a city, not at this computer (whose place is not known). This supersedes ADR-0043's rejection of a map.

## Consequences

- Without the city table the map is replaced by one line saying how to get it; the tables are managed in Settings → Downloaded data, not on the Traffic tab.
- A CDN address shows the hub that served it; a backbone router's city can be wrong. Both are labelled approximate.
- The page grows by about 47 KB of path data in the Network chunk.

## Alternatives considered

- **A map library (Leaflet, D3-geo) with tiles.** A heavy dependency, and tiles fetched from a third party on every view.
- **No map, ranked bars only (ADR-0043).** The owner found the bars hard to read and asked for locations they can see.
- **Keep the exact points.** Nearly triples the places of the real table (559,661 against 223,617) and the memory a download takes to build the index (134 MB against 28 MB peak heap).

## Links

- Plan: `docs/PLANS/cross-cutting/network-traffic-view-redesign.md`; ADR-0043 (superseded on the map), ADR-0044 (index version 2), ADR-0045.
- Code: `frontend/src/components/netaudit/TrafficMap.vue`, `frontend/src/constants/worldLand.ts`, `frontend/src/utils/netaudit/traffic.ts` (`mapDots`, `routePoints`).
