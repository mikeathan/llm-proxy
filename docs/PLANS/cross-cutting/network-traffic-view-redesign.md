---
status: partial
date: 2026-11-11
last_reviewed: 2026-11-11
related_specs: [SPEC-003]
constitution_references: [I.2, I.13, I.14]
evidence: the last host-traffic summary read from vertex through GET /admin/api/netaudit/host-traffic on 2026-11-11 (look taken 13:34 UTC on wlp2s0); backend/internal/app/hosttraffic.go:35; backend/internal/core/netaudit/hosttraffic/hosttraffic.go:128; docs/architecture/pitfalls.md #65; ADR-0043; ADR-0044.
---

# Network · Traffic: correct numbers first, then a view that answers "where does my traffic go"

The owner's verdict on the deployed v0.72.0 Traffic tab (2026-11-11): messy, too much information, not useful, only one location, the
chart is not useful, a clicked row tells nothing, and the "Who runs these servers" and "Cities" sections download data whose use is
invisible. They asked for where traffic goes, how many hops it makes, and the location and information of each hop, like other
popular tools.

**Most of what looked bad is wrong data, not only bad design.** Phase 0 fixes the numbers; the redesign is judged only after it.

## Evidence: the look on vertex (read 2026-11-11)

| Field | Value | What it means |
|---|---|---|
| `bytes_in` / `bytes_out` | 0 / 0 | No packet was recognised as this computer's. |
| `bytes_through` | 1,664,770 | Every byte counted as "passing through", hence the headline "nothing was sent or received". |
| top destination | `192.168.50.251`, 1,485,688 B (89%) | The owner's Mac, which was viewing this page. |
| ports of that destination | 58071, 58072, 58105, 58106, 58107, 58073 (tcp) | The Mac's ephemeral ports, one row per browser connection, not the service. |
| `192.168.50.60:4001` | 32,714 B | The other half of the same conversation: the browser's requests to this program, shown as a destination. |
| `internet_bytes` | 672 B in 61 s | Almost no internet traffic: vertex is an idle server. Hence one country. |
| owners | Cloudflare AS13335, country `US`, city Toronto, Ontario, 104 B | The city table is downloaded and works; the screen showed "United States" (registration) while the machine is in Toronto, and the city never appeared. |
| names | none | No TLS server name was read: a hello is read only for traffic going *out*, and nothing was "out". |

### Bug A: which way a packet went (pitfall 65, third time)

`ownAddresses` (`backend/internal/app/hosttraffic.go:35`) lists addresses with `net.InterfaceAddrs`, which needs netlink. The unit on vertex
refuses netlink, so it returns nothing. `tally.direction` (`hosttraffic.go:128`) then calls every packet `dirThrough`. Consequences seen:
the wrong headline, this computer listed as a destination of itself, no server names, and HTTPS shown as 104 B. Pitfall 65 names the
call and the fix (`Scope.LocalAddrs` / `cachedOwnAddress`); the decoys (v0.45.1) and Nearby (v0.41.0) hit the same trap.

### Bug B: the port of a conversation is the remote's port, so incoming connections split per client port

For a packet coming in, `direction` keys the flow on the sender's port. When a browser connects *to* this computer, that is its ephemeral
port. The result: one row per connection, and `kindOf` on an ephemeral port gives "Other (TCP) 91%". Fixing A alone does not fix this. The
frames are Ethernet frames with the TCP header kept, so the TCP flags (SYN without ACK says who opened the connection) can be read.

### Issue C: the look watches itself

On a server, the biggest talker during a look is the owner's own browser loading this page. The look must say so and set it apart: "Your
browser viewing this page: 1.4 MB". The regular spikes of about 260 KB every 10 s on the chart may be the page's own polling; not measured.

### Issue D: what the look can and cannot see

It sees only the traffic of the computer it runs on (vertex), not the owner's Mac or phone. The page must say this plainly, because
"where does my traffic go" usually means the device in your hand.

## Answers to the owner's questions

- **Only one location:** vertex sent 672 bytes to the internet in that minute, all to one Cloudflare address. With A fixed, a busy
  minute shows more, but an idle server will always be mostly local chatter.
- **"Who runs these servers" (IPtoASN, 11 MB):** names the operator of each internet address (Cloudflare, Google, Amazon) and the country it
  is *registered* in. It is used today only in a grey line under a row and in the Operators/Countries bars.
- **"Cities" (DB-IP Lite City, 75 MB on disk):** where the address is *used*, roughly (city level is often wrong for CDNs and backbone
  routers). It is used today only in that same grey line, which is why it looks unused. Toronto vs "United States" above is the
  registration vs location gap the screen never explains.
- **Hops:** not possible from what is captured: a capture sees the two ends of a conversation, never the routers between. Hops need active
  probes (traceroute). See Phase 3. It needs a Constitution amendment because it sends packets to internet addresses.

## Proposed design

One question per part, Simple by default, Advanced for the details (the owner's rule: no hidden knowledge, one path per job).

1. **Headline:** one sentence and three numbers: sent, received, to the internet. Below it, one line for the page's own traffic
   ("Your browser viewing this page: 1.4 MB, not counted below"), and one line saying this is vertex only.
2. **Who it talked to:** one list, grouped by *who*, not by address:port. A row is a name (device name from the inventory, DNS or TLS name,
   or the operator), the service ("HTTPS", "this program :4001"), a direction arrow, the amount, and for internet rows the place as
   "Toronto, Canada · Cloudflare". Three groups: **Internet** (open), **This network** (open), **Broadcast chatter** (closed by default:
   mDNS, SSDP and the like are noise to most owners).
3. **A row opened:** what we know and how sure: name and where it came from, operator and AS number, "used in Toronto (approximate) ·
   registered in the United States", the conversations, and for an internet row **Trace the route** (Phase 3). The device scans stay for
   a device on this network.
4. **The chart:** replaced by a thin activity strip in the headline (Advanced only), and opening a row highlights the seconds it was
   active. The full per-second chart moves to Advanced.
5. **The tables:** the two download cards leave the Traffic tab. In their place one status line ("Operator and city names: on, tables
   from 11 Nov · Manage") and, where an operator or city would appear without a table, an inline "Download to see who runs this". The
   cards live only in Settings → Data.
6. **What it was** (kinds) folds into the headline as one line ("92% encrypted web, 3% name lookups") instead of its own panel of chips.

### Route view (Phase 3, needs the owner's approval and an amendment)

Like mtr/PingPlotter, on demand and per destination: the hops from this computer to one internet address, each with its address, round-trip
time, operator (IPtoASN, on this computer) and approximate city (DB-IP, on this computer), drawn as a horizontal path: *this computer →
router → ISP (Virgin Media, London) → … → Cloudflare (Toronto)*. Honest limits shown on screen: many routers do not answer (shown as "no
answer", not hidden), backbone cities are often wrong, and a route is one path at one moment.

Mechanism to verify before proposing text for the Constitution: unprivileged UDP probes with rising TTL and `IP_RECVERR` (the way
`tracepath` works without root) from the service. Unprivileged ICMP is out (vertex's `ping_group_range` is "1 0"), and nmap is out (the scan
scope forbids public addresses). Bounds to propose: started only by the operator's press on one address seen in the last look, at most 30
hops, 3 probes a hop, one trace at a time, nothing sent anywhere to look up a hop.

### Map (Phase 4, optional, owner's choice)

ADR-0043 rejected a world map (false precision for CDNs, a new dependency). With the city table and the route view, a small map of the
route's hops is possible. Choosing it supersedes that part of ADR-0043; read ADR-0023 first. Recommendation: build the path diagram first
and add a map only if the path is not enough.

## Phases

| Phase | What | Needs | Checks |
|---|---|---|---|
| 0 | Fix A (own addresses through `Scope.LocalAddrs`; the capture interface's MAC from `/sys/class/net/<iface>/address` for IPv6, if the unit can read it) and B (who opened the conversation, from TCP flags; key on the server's port). A "cannot tell" state says "could not tell which way traffic went", never "nothing was sent". Failing-first tests for each. | nothing | `go test`, the owner's look on vertex |
| 1 | Mark this page's own traffic and set it apart; say the look is vertex only; measure the 10 s spikes. | nothing | look on vertex while browsing |
| 2 | The redesign above: grouped list, row detail, chart to Advanced, table cards to Settings, kinds as one line. Mockup first for the owner's pick. | owner picks the mockup | vitest, build, visual specs |
| 3 | Route view: measure the UDP+`IP_RECVERR` probe under the unit, then an ADR and a Constitution I.x item, then build. | owner approves the amendment | live trace on vertex |
| 4 | Map of a route's hops. | owner chooses to supersede ADR-0043 | visual specs |

## Owner's decisions (2026-11-11)

All phases, in one branch: the route view (Constitution I.14, ADR-0045), the map (ADR-0046, superseding ADR-0043's rejection), and broadcast chatter closed by default.

## Built (branch `network-traffic-route-map`, from `origin/main`, v0.73.0, uncommitted)

- **Phase 0:** `hosttraffic.Self` (addresses from `Scope.LocalAddrs`, cards from `/sys/class/net`, cards learned from frames, this program's port); `servicePort` keys a
  conversation on the service's port (`served`), `self` marks this program's own, `self_bytes` and `direction_unknown` on the summary.
- **Phase 1:** the summary sets this program's page apart and says only this computer is seen.
- **Phase 2:** the Traffic tab as summary / map / who it talked to (SPEC-003 2.62), Advanced holds the chart, totals, operators and conversations; the table cards moved to
  Settings → Downloaded data. No mockup round: the owner asked for the whole of it at once.
- **Phase 3:** `platform/routeprobe` (Linux, unprivileged UDP + `IP_RECVERR`), `core/netaudit/traceroute`, `GET`/step-up `POST /admin/api/netaudit/route-trace`,
  `TrafficRoute.vue`.
- **Phase 4:** `TrafficMap.vue` on a built-in Natural Earth outline; the city index is version 2 (country and a point per place, 0.1°; ADR-0044 amended).

**Checks run:** backend `go build ./...`, `GOOS=linux go build ./...` and `go vet` on the changed packages, `go test ./...` (all pass), `go test -race` on
`traceroute`, `go run ./tools/check-complexity/` (no new violations); `routeprobe` tests on Linux in Docker as uid 65534 (pass); the city index measured on the real
DB-IP table; frontend `npx vitest run` (288 files, 2924 tests) and `npm run build` (lint and type-check pass), `npx playwright test e2e/network-traffic.visual.spec.ts`
(12 pass, screenshots read). **Not run:** anything on vertex; a full multi-hop trace (Colima drops the answers past hop 1).

## Remaining Work

1. **Owner, on vertex:** deploy, then **download the city table again** (Settings → Downloaded data): the version-1 file is refused, so cities and the map are off
   until then. Listen once and check the headline says what was sent and received, and that the browser's own traffic is set apart.
2. **Owner, on vertex:** open an internet place and press "Trace the route"; confirm hops answer under the unit (no IP filter, `AF_INET` allowed: expected to work).
3. Measure the ~260 KB spikes every 10 s seen in the first look (likely a page poll); not investigated.
4. Merging this branch is the owner's approval of Constitution I.14.
5. Carried from the regroup handoff (now deleted; its work is in SPEC-003 2.61 and ADR-0044): a city shows only for an address IPtoASN found an owner for, so a
   city alone never appears; change that only if the owner asks.

**Branch state:** `network-traffic-route-map`, made from `origin/main` (21a4161c) and tracking it, so push with `git push -u origin network-traffic-route-map`, never a
bare `git push`. Nothing is committed or pushed. Start from `backend/internal/core/netaudit/hosttraffic/hosttraffic.go` (`direction`, `servicePort`) and
`frontend/src/components/netaudit/TrafficReport.vue`.
