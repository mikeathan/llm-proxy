---
id: ADR-0044
title: A city table is an index file searched on disk, never held in memory
status: proposed
date: 2026-11-11
subsystems: [network-audit]
supersedes:
superseded_by:
---

# ADR-0044: A city table is an index file searched on disk, never held in memory

## Context

ADR-0043 named the operator and country of a destination from IPtoASN and rejected "DB-IP Lite alongside IPtoASN" because DB-IP's free table was country only. The owner asked
on 2026-11-11 for more location per destination. DB-IP Lite **City** (CC BY 4.0, monthly, no key) has a city, a region and coordinates per range. It was downloaded and measured the
same day: `dbip-city-lite-2026-11.csv.gz` is 85.7 MB (692 MB expanded), 7,777,816 rows (3.60 M IPv4, 4.18 M IPv6); joining neighbouring rows of one place leaves 3.40 M IPv4 and
3.86 M IPv6 runs and 193,997 distinct places. Held the way the owner table is (sorted ranges in memory) it would cost about 75 MB resident; the owner said that is too much.

## Decision

We will add the city table as an **optional second table that is never loaded into memory**.

- **One pass builds a compact index file.** `core/netaudit/ipplaces.Build` reads the CSV as it streams out of the download (never the 692 MB, never the 86 MB held at once), keeps
  neighbouring rows of one place as one run, ends a run with a "no place" record at a gap, drops an overlapping or out-of-order row, makes every name plain text, and writes
  fixed-width records: IPv4 as start + place id (8 bytes), IPv6 by its first 64 bits + place id (12 bytes), then the places once each. Only the places (a few hundred thousand short
  strings, about 36 MB of heap at the peak, measured) are held while it runs. Measured on the real file: 4.7 s, a 75 MB file, 7,255,000 records.
- **A lookup is a binary search on the file**: about 22 small `ReadAt` calls, 7.7 microseconds an address (measured). The operating system's page cache holds what was read; the program
  holds a file handle and a 32-byte header. Resident memory does not depend on the table.
- **It only adds to an owner.** `currentOwners` fills `Region` and `City` of an owner IPtoASN found; an address IPtoASN does not know stays unowned, so the by-operator and by-country lists
  are unchanged and a city never creates a holder.
- **Downloaded on a press, like the owner table.** `POST /admin/api/netaudit/destination-places/download` is a step-up route (it makes this computer ask db-ip.com), goes through
  `NetworkTools.Open`, tries this month's file and then last month's (the name carries the month, built from the first of the month), is bounded (160 MiB compressed, 2 GiB expanded, ten
  minutes) and keeps the old index when anything fails; a temporary file is renamed over the old one and never left behind.
- **It is approximate and says so, and it is credited.** The table says where an address is used; for a content network that is the hub that serves it (1.1.1.1 reads "Sydney"). The
  card says so, and shows the licence's credit line (`ipplaces.Attribution`).

## Consequences

- The file is about 75 MB of disk, an 86 MB download, in return for almost no memory. The owner table (11 MB on disk, about 20 MB in memory) is unchanged.
- The city, region and holder's name are text somebody else chose: shown only as text, never in a finding, alert, log, prompt or the Timeline (the guard test in `hosttraffic` lists them).
- Disk-backed means a damaged file is found at lookup time, not at start: `Open` checks the header and the file size, and a read error reads as "no place".
- A table older than a month is still used; the card says when it was downloaded. There is no automatic refresh (Constitution I.2).

## Alternatives considered

- **Hold the ranges in memory.** About 75 MB resident (27 MB for IPv4 alone). Rejected by the owner.
- **IPv4 only, in memory.** About 27 MB resident and no IPv6 city; the disk index gives both families for less memory.
- **Coarser keys (a /24 for IPv4).** Smaller, but a range smaller than a /24 would answer for its neighbours: false precision.
- **A per-address online lookup, MaxMind GeoLite2, a map.** As ADR-0043 rejected them.

## Links

- Plan: `docs/PLANS/cross-cutting/network-traffic-settings-regroup-handoff-2026-11-11.md`; ADR-0043; SPEC-003 changelog 2.61.
