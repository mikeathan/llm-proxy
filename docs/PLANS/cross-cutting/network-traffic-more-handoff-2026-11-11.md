---
status: proposed
date: 2026-11-11
last_reviewed: 2026-11-11
related_specs: [SPEC-003]
constitution_references: [I.8, I.14]
evidence: owner request 2026-11-11 ("i want more if possible"); docs/PLANS/cross-cutting/network-traffic-view-redesign.md; ADR-0018, ADR-0023, ADR-0045, ADR-0046; backend/internal/core/netaudit/phonehome/watch.go; CONSTITUTION.md I.8 (DNS query lines), I.14.
---

# Network · Traffic: trace any address, and see the other devices' connections (handoff)

The owner asked for more than v0.73.x gives: the hops to places they choose, and what the **other devices** in the house connect to, not only this
computer (vertex). This file is the starting point for a new session. Nothing in it is built.

## Where things stand

- **Merged:** #225 (v0.73.0: direction fix, summary / map / who-it-talked-to, route trace, map), #226 (docs).
- **Open:** #227 on branch `traffic-broadcast-grouping` (v0.73.1: broadcasts are chatter, never internet; the map links to the tables). This file rides on it.
- **Not yet seen on vertex:** a multi-hop trace (Colima's NAT answered only hop 1), the map with real cities (the city table must be **downloaded again**:
  the version-1 index is refused), the look after the direction fix. Do these first; they decide whether Part A's repeat mode is worth building.
- The router is an Asus **RT-AX86U on stock firmware** (ADR-0018). It already sends its log to vertex (UDP 514 redirected to 5514), including
  `dnsmasq ... query[A] name from address` lines that the phone-home watch keeps per device.

## Part A: trace any internet address, and repeat it (small; needs an I.14 amendment)

Today a trace is allowed only to an address the last traffic look saw (Constitution I.14, `permitRouteTrace` in `backend/internal/app/netaudit.go`).

1. **A "Trace a route" card at the top of the Traffic tab**: an input for an internet address or a host name, plus fixed presets (the router's
   upstream DNS from the scope, 1.1.1.1, 8.8.8.8, 9.9.9.9). A name is resolved through the system resolver (one lookup, the name the operator typed;
   say so on screen). The trace itself is the existing `traceroute.Manager`; the result renders with `TrafficRoute.vue` and on `TrafficMap.vue`.
2. **Repeat mode (mtr-like), only if the first vertex traces show hops answer:** N rounds (say 10, one a second) over the hops already found,
   per hop: answered of sent, best / average / worst time. Bound it: 30 hops × 10 rounds × 1 probe, one repeat at a time, 60 s cap.
3. **Amend I.14:** allow an operator-typed or preset internet address (still step-up, still the internet guardrail and the blocked list, still
   never from an agent, automation or stack), add the repeat bounds. Keep "single internet address only" (`traceroute.isTraceable`).
4. Decide with the owner: should a typed **host name** be allowed (it sends one DNS query with that name), or addresses only?

Files: `backend/internal/core/netaudit/traceroute/traceroute.go` (`Start`, add `Repeat`), `backend/internal/app/netaudit.go` (`permitRouteTrace`
takes a "typed" flag), `backend/internal/transport/http/handlers/netaudit_routetrace_handlers.go`, `frontend/src/components/netaudit/TrafficRoute.vue`,
`frontend/src/composables/netaudit/useRouteTrace.ts`, a new `TrafficTraceCard.vue`. Tests: the manager's fake `route` prober already covers hops.

## Part B: the other devices' connections

### B1. What each device looks up, from data already kept (no amendment expected; build first)

The phone-home watch (`core/netaudit/phonehome`, ADR-0018 amendment 2026-11-05) already keeps, per device (hardware address), up to 256 names it
asked the router for, with recent times. I.8 says such a name "is shown as text only", so a view is allowed; check the text again before building.

- A **"Other devices" section on the Traffic tab** (or a tab of its own): one row per device (inventory name, address), how many names it looked up
  today, the newest names, and flags the watch already computes (new name, known-bad name, encrypted DNS, asks for nothing). Opening a device lists
  its names grouped by registrable domain (googleapis.com, apple.com...), newest first.
- **No addresses or bytes:** the router's query line has neither. Do not resolve the names to addresses from this computer (that would send every
  device's lookups out again); say on screen that this is lookups, not traffic.
- Needs: the router log listener and the phone-home module switched on (check on vertex first: Modules tab; the Overview's DNS counters).
- Files: `backend/internal/core/netaudit/phonehome/watch.go` (a read model for the screen), a route under `/admin/api/netaudit/`, frontend
  composable + component. Watch the bounds (128 devices, 256 names) and that the screen never sends a name to a model (I.8).

### B2. The router's connection table (real per-device connections; owner decisions and an amendment)

The only way to see every device's connections with remote addresses (and bytes) without changing the network is the router's own connection
tracking: `/proc/net/nf_conntrack` on the RT-AX86U, read over SSH. With addresses, everything built for vertex applies per device: operator, city,
map, and a trace to any of them.

Measure before designing (the owner runs these; nothing in the repo can):
1. Router: Administration → System → Enable SSH (LAN only), key login. Then `ssh admin@<router> 'wc -l /proc/net/nf_conntrack; head -3 /proc/net/nf_conntrack; cat /proc/sys/net/netfilter/nf_conntrack_acct'`.
   Is the file readable, how many lines, is byte accounting on (`bytes=` fields)?
2. How long one read takes and how big it is (a home network: a few hundred to a few thousand lines).

Then decide with the owner, in an ADR and a Constitution item:
- **A new dependency:** `golang.org/x/crypto/ssh` (AGENTS.md: ask before adding). The app has no SSH client today (learnings, 2026-10-28).
- **A stored secret:** the router SSH key, in the secrets store, step-up to set; the router's host key pinned on first use and checked every time.
- **Bounds:** on the operator's press only (like the traffic look), one read, a line cap, parsed into typed fields (protocol, the two addresses
  and ports, bytes); nothing else of a line kept; the router's other files never read; never from an agent or automation.
- **What it can't show:** NAT has already rewritten the source on the WAN side, but conntrack keeps the original LAN address, so each connection
  can be tied to a device. Connections that ended before the read are not there.

### Rejected or weaker (record in ADR-0023 when Part B is decided)

- **The router's web API** (`appGet.cgi`, Traffic Analyzer database): undocumented, changes with firmware, needs the admin web password.
- **SNMP:** interface counters only, no per-connection data (ADR-0023 already rejects counters).
- **Port mirroring or making vertex the gateway:** changes the network; out of scope.
- **Capturing other devices' traffic on Wi-Fi:** vertex's card in managed mode sees only its own and broadcast frames (I.13 and its measurements).

## Order

1. Merge #227, deploy, re-download the city table, listen, open an internet place and trace it. Write what the trace showed in the learnings ledger.
2. Part A (card + presets), then the I.14 amendment; repeat mode only if step 1 showed answering hops.
3. B1 (lookups per device), after confirming the router log and phone-home module are on.
4. B2: the owner's measurements, then the dependency / secret / amendment decisions, then build.

## Traps from the last session

- Anything that asks "is this address mine?" must use `Scope.LocalAddrs` or the cards' hardware addresses (pitfall 65); `net.InterfaceAddrs` fails on vertex.
- The helper keeps only 4 bytes after the IP header of an ordinary frame: no TCP flags.
- A broadcast to a subnet address is known only by its hardware destination (v0.73.1).
- Colima's user-mode NAT drops ICMP answers past hop 1: test traces on vertex, not in Docker on the Mac.
- The city table at full point precision costs 134 MB to build; keep points at 0.1°.
- After a deploy, "it looks the same" is first a hard reload: a page opened before keeps the old bundle.
- `ls` is aliased to eza here: never use `$(ls ...)` in a script.

## First step

Open `docs/PLANS/cross-cutting/network-traffic-view-redesign.md` (what is built and what the owner must run), then
`backend/internal/core/netaudit/traceroute/traceroute.go` and `backend/internal/app/netaudit.go` (`permitRouteTrace`) for Part A.
