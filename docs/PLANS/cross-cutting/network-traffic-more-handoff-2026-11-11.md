---
status: partial
date: 2026-11-11
last_reviewed: 2026-11-11
related_specs: [SPEC-003]
constitution_references: [I.8, I.14]
evidence: owner request 2026-11-11 ("i want more if possible"); docs/PLANS/cross-cutting/network-traffic-view-redesign.md; ADR-0018, ADR-0023, ADR-0045, ADR-0046; backend/internal/core/netaudit/phonehome/watch.go; CONSTITUTION.md I.8 (DNS query lines), I.14.
---

# Network · Traffic: trace any address, and see the other devices' connections (handoff)

The owner asked for more than v0.73.x gives: the hops to places they choose, and what the **other devices** in the house connect to, not only this
computer (vertex). This file is the starting point for a new session. Nothing in it is built.

## Where things stand (updated 2026-11-12, v0.74.0)

- **Merged and deployed:** #225 (v0.73.0), #226, #227 (v0.73.1, broadcasts out of the internet list). The owner then ran the Traffic tab on vertex: a 17-step trace
  reached `91.189.91.61` (Canonical, London) through Sky, Zayo (Newark, Minneapolis, Columbia City); several steps silent; and a look with two internet places.
- **Built on branch `traffic-broadcast-grouping` (uncommitted, v0.74.0, not run on vertex):**
  - **Part A** done: the "Trace a route" card (typed address or presets), Constitution I.14 and ADR-0045 amended, repeat mode (10 rounds, per-step
    answered / best / average / worst), silent steps folded, route on the map. **Decisions taken (reversible):** addresses only, no host names; no
    router-DNS preset (the scope holds no resolver address).
  - **B1** done: "Other devices" on the Traffic tab, from `phonehome.Lookups`; Constitution I.8 amended in one sentence.
  - **UI:** "What it was" is now bars by kind plus, under Advanced, "Every conversation" with one Service column.
- **Checks run:** `go test ./...`, complexity, `GOOS=linux go build`, `npx vitest run`, `npm run build`, the traffic visual spec, the harness check (see the
  session's final report for the exact results). **Not run:** anything on vertex; `-race` is not needed for the new code except `traceroute` (run).
- **B2 built (v0.75.0, same branch, not run against the real router):** the owner measured the RT-AX86U (readable, 245 lines, no bytes) and approved the
  dependency (already in `go.mod`), the stored key and the Constitution text. Constitution I.15, ADR-0047, `platform/routerssh`, `core/netaudit/routerconn`,
  the step-up routes and the "Where they connected" view. **To do on the owner's side:** make a key (`ssh-keygen -t ed25519 -N "" -f router_key`), add
  `router_key.pub` to the router's Authorized Keys with SSH on for LAN only, then in Network → Traffic → Other devices → Where they connected: address, user,
  key, check and trust the router's fingerprint, read. If the router refuses the ed25519 key, try an RSA key (`-t rsa -b 3072`).
- **Left:** the owner's vertex run and first real read; optional byte counts (needs `nf_conntrack_acct` on, which stock firmware loses on reboot); the
  decision about the ~260 KB spikes. Part A's repeat is built, so decision 2 below is answered.

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

## What the owner must do on vertex (before or while Part A is built)

1. Merge #227 and deploy; hard-reload the Network page (Cmd+Shift+R): a page opened before a deploy keeps the old bundle.
2. Settings → Downloaded data:
   - press **Download the table** under "Where each internet address is used" (about 86 MB, asks for the passphrase);
   - check that "Who runs each internet address" is held.
3. Traffic → **Listen now** while vertex does something online (for example an assistant question that searches the web), so the minute has
   internet destinations.
4. Open an internet place → **Trace the route**. Note:
   - how many steps answered, and the times;
   - whether the last step says "Reached";
   - whether the map draws the line.

## Recipes

- **Vertex's version and the bundle it serves:**
  - version: `curl -s -H "Authorization: Bearer $LLM_PROXY_TOKEN" http://vertex.local:4001/admin/api/version`;
  - the UI is under `/admin/` (the root answers 404); the copy text lives in the `stacksService-*.js` chunk, so grep that chunk for a string.
- **Vertex's last traffic look:** `GET http://vertex.local:4001/admin/api/netaudit/host-traffic` with the same header; check:
  - `bytes_in` / `bytes_out` / `bytes_through`, `direction_unknown`, `self_bytes`;
  - the destinations' `shared` and `owner.point`.
- **The probe on Linux, unprivileged, from the Mac:**
  `docker run --rm --user 65534:65534 -e HOME=/tmp -e GOCACHE=/tmp/gocache -e GOFLAGS=-mod=mod -e GOTOOLCHAIN=local -v "$PWD":/src:ro -v ~/go/pkg/mod:/go/pkg/mod:ro -w /src golang:1.26 go test -count=1 -v ./internal/platform/routeprobe/`
  (from `backend/`). Hops past the first never answer under Colima.
- **City index build cost:** a throwaway test that streams the DB-IP `.csv.gz` into `ipplaces.Build` and samples `runtime.MemStats.HeapInuse`
  every 20 ms.
  - Last numbers: 5.3 s, 79 MB, 223,617 places, 28 MB peak.
  - The real file was in an earlier session's scratchpad; download it again for a new measurement.
- **Gates used:**
  - backend: `go test ./...`, `GOOS=linux go build ./...`, `-race` on `app` / `traceroute` / `hosttraffic`, `go run ./tools/check-complexity/`;
  - frontend: `npx vitest run`, `npm run build`, `npx playwright test e2e/network-traffic.visual.spec.ts e2e/netaudit-*.visual.spec.ts`,
    `npm run test:visual`;
  - docs: `./scripts/check-agent-harness.sh`.
  - Screenshots: `SCREENSHOTS=1` writes them under `frontend/test-results/`.

## Decisions waiting on the owner

1. Part A: may a typed **host name** be traced (one DNS query with that name), or addresses only?
2. Part A: build the repeat (mtr-like) mode? It depends on the first vertex trace.
3. Part B2: enable SSH on the router, add `golang.org/x/crypto/ssh`, store the router key as a secret, and amend the Constitution? Run the
   measurements above first.
4. The ~260 KB spikes every 10 s seen in looks: measure which page poll it is, and whether it should be smaller.

## First step

Open `docs/PLANS/cross-cutting/network-traffic-view-redesign.md` (what is built and what the owner must run), then
`backend/internal/core/netaudit/traceroute/traceroute.go` and `backend/internal/app/netaudit.go` (`permitRouteTrace`) for Part A.
