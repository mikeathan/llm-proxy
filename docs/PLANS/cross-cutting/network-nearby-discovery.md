---
status: partial
date: 2026-11-03
last_reviewed: 2026-11-03
related_specs: [SPEC-003]
constitution_references: [I.1, I.2, I.3, I.6, I.7, I.8, I.9 (proposed), II.2, V.1]
evidence: Recon of 2026-11-03 in the repo at v0.40.0 (§2). No packet was sent for this plan; every protocol fact marked "unmeasured" waits for the probes in §6. Nothing is built.
---

# Nearby: mDNS and SSDP discovery on Network → Devices

**Status:** partial (2026-11-03): Phases 1–4 are **built and tested on branch `network-nearby-discovery`, uncommitted, v0.41.0**; the owner approved the plan and D1–D6, and dropped 3b. **Not done: the Phase 0 probes (nothing was sent on any network), so every parser is tested on messages written to the documented layout, not on captures, and nothing has run against a real network.** The I.9 text is in `CONSTITUTION.md` as *proposed* (in force when the owner merges). Classification: cross-cutting (backend + frontend), subsystem **network-audit**.
**Related:** [ADR-0024](../../decisions/ADR-0024-nearby-discovery-two-multicast-datagrams-bounded-replies.md) (the decision and the I.9 text), [ADR-0016](../../decisions/ADR-0016-active-ipv6-discovery-one-echo-unprivileged.md), [ADR-0019](../../decisions/ADR-0019-outside-lookup-is-opt-in-and-fails-open-to-unknown.md), [ADR-0020](../../decisions/ADR-0020-dns-check-only-records-and-never-reads-silence-as-agreement.md), [ADR-0022](../../decisions/ADR-0022-modules-is-the-navigation-group-for-feature-areas.md), [ADR-0023](../../decisions/ADR-0023-network-ideas-rejected-or-already-built.md); plans [`network-security-roadmap.md`](network-security-roadmap.md), [`network-guard-module.md`](network-guard-module.md).

## 1. Goal and non-goals

**Goal.** Show what the devices on the network say they are (service types, UPnP device type, maker, model) in a **Nearby** section of Network → Devices, and raise a few findings from it. Opt-in, bounded, typed, macOS and Linux, unprivileged.

**Out of scope (separate PRs, per the request):** host IPv6 and new-interface checks, macOS neighbour/hostwatch parity, host packages vs KEV, Certificate Transparency, **passive** multicast listening (needs its own amendment after this proves useful). Also not here: anything ADR-0023 rejects (router counters, public-IP change, version drift, threat lists, `http-title`, `dns-recursion`).

## 2. Recon: what was verified, and what the request had slightly wrong

| Claim | Check (file) | Result |
|---|---|---|
| Existing mDNS naming can be reused | `core/netaudit/names/mdns.go`, `prober.go` | **Partly.** Reusable: the wire reader (`readName`, `readRecord`, compression-pointer limits, `Clean`). Not reusable as is: `Query`/`ParseReply` are hard-wired to one PTR for one reverse name; the `Prober` dials **one device** through the guarded dialer. IPv4 only. |
| A multicast question can go through the guarded dialer | `core/tools/network.go` `validateIP` → `network.CheckAlwaysBlocked` | **No.** Multicast and link-local are always refused, and the dial returns a connected UDP socket (replies only from one peer). **This forces a new Constitution item (I.9), not "if needed".** |
| The router UPnP client has no model/firmware parsing | `core/netaudit/upnp/upnp.go` (`Description` holds only `Gateway`, `Service`, `ControlPath`, `ServiceType`) | **Confirmed.** But the real page (`upnp/testdata/rootdesc_asus.xml`) *does* carry `manufacturer`, `modelName` ("ASUS Wireless Router"), `modelNumber` ("RT-AX86U") and, on the inner devices, `MiniUPnP daemon version 2.3.11`; the `SERVER` header adds `AsusWRT/4.1.52`. **No firmware version exists anywhere in the fixture**, so "router firmware vs KEV/NVD" cannot be built; "the router's UPnP daemon vs KEV/NVD" can (§5.3). `AsusWRT/4.1.52` looks like a kernel version; unverified, so it is not parsed. |
| `platform/neighbors` is Linux-only | `neighbors.go` (`ErrUnavailable`) | Confirmed; Nearby does not depend on it. On a Mac the MAC link is simply absent (rows are keyed by address). |
| Existing finding kinds, no duplicates | grep `rtsp`, `ipp`, `ollama`, `11434`, `jupyter`, `prometheus`, `consul`, `portainer`, `registry` in `core/netaudit` | **None built** (hits are the OUI registry and a disk-space hint). Port 631 (IPP), 9100, 5000, 8000, 8080 are in `DefaultPorts`; 11434 and 8500 are in neither scan list; 8888 and 9000 are in `ThoroughPorts` only (`ports.go:48,55`). |
| Reply-capping, step-up and card patterns exist | `dnscollector.go` (`refusedBySettings`, bounded log), `exposure.go` (`Begin`/`Carry`), `authenticate.go` `stepUpExact` | Yes; reused (§4). |
| `Vendor` exists on `DeviceView` | `devicesview.go:19` | Yes, **but only on the v0.40.0 commit `69422c71`, which `main` (local) does not contain** (local `main` is at `3d0feb86`, #181; `git log main..HEAD` lists `5b323508` (#182) and `69422c71`). The new branch must be based on `69422c71` or wait for it to merge: **owner decision D1**. |

## 3. Owner decisions (answered 2026-11-03: D1 yes, D2 yes, D3 yes, D4 yes, D5 yes, D6 yes; the original recommendation is each bullet's text)

**D1:** v0.40.0 is deployed and in `origin/main` (#183, `2614fe0b`); the branch `network-nearby-discovery` is based on it (the tree equals `69422c71`'s). The `Vendor` blocker below is gone. **D2 approved in principle:** I.9 is written into `CONSTITUTION.md` only in the PR that carries the code, after the Phase 0 probes confirm the packets.

- **D1 Branch base** (blocks coding): base on `69422c71` (v0.40.0, unmerged), or wait for it. Next version: **0.41.0** (new feature, minor).
- **D2 Constitution I.9** (blocks Phase 1): approve the text in ADR-0024, or choose the fallback (unicast only, no amendment; §8).
- **D3 Step-up for the switch** (recommended yes): the switch opens a socket that reads from many devices, like I.6 and I.8 (I.7's one fixed packet needed none).
- **D4 The host's own services** (blocks Phase 4): vertex runs llama.cpp on 8081 and this app on 4001. The AI-endpoint check will say llama.cpp is open to the network. Recommended: report it like any device (it is true), accept with a note if intended; the alternative (skip this computer) hides a real exposure. Precedent: the owner chose to skip only a *named* device for `hidden_device`.
- **D5 IPP** (blocks Phase 3c): recommended **identify only** (make and model into the Nearby row), no finding, because an open IPP printer is the normal home setup. The request lists IPP as a check; say if a specific finding is wanted.
- **D6 Schedule**: recommended **Check now** plus once per nightly audit when on; no background loop. A loop (every N minutes) would need its own interval setting.

## 4. Design

**Packages.** `platform/mcast` (the one socket; `//go:build unix`, with a `_other.go` stub returning `ErrUnavailable`; no `x/net`, options via `SyscallConn` and `x/sys`). `core/netaudit/dnswire` (the DNS wire reader extracted from `names/mdns.go`, with fuzz tests; `names` keeps its API and uses it). `core/netaudit/nearby` (typed DNS-SD and SSDP parsing, the collector, the stored result). `models.NearbyDevice` in a store `persistence.NearbyStore` (one file, at most 256 devices, registered with `withReset`).

**A look** (`Collector.Look(ctx)`): settings check (switch on; Guardrails local-network on, else status `lan_off`, nothing sent) → single-flight and 30 s minimum gap, enforced in the collector → pick interfaces (up, multicast, not loopback, an IPv4 inside the scan networks, ≤ 4) → per interface open the socket, send the two datagrams (§6), read ≤ 5 s under the I.9 counts (512 datagrams, 32 per sender, 4,096 B) → for each reply drop sources outside `scope.Policy()`, this host's own addresses and `BlockedIPs` → parse → fetch ≤ 32 UPnP description pages through `NetworkTools.HTTPClient()` (LOCATION must equal the replier: reuse `upnp.ParseSSDP`, generalised from "the gateway" to "the replier") → unicast per-service follow-ups only if the first look shows they are needed (decided after probe 1; may be cut) → merge into the stored result. Every context is derived from the application root (II.2); the socket has a deadline and `defer Close`; a test asserts no goroutine outlives a cancelled look.

**What is kept per device** (everything else is dropped in parsing): address; MAC if the inventory knows that address (else the row says "not in your device list"); service types (validated pattern, ≤ 16); ports from service records; UPnP device type (URN shape), maker, model, model number, cleaned names (≤ 40 characters); `first_heard`, `last_heard`; which source (mDNS, SSDP) heard it. TXT values are not kept (they hold hardware addresses and account hints).

**Fail open to unknown** (ADR-0019/0020): no reply, a refused socket, `lan_off`, a blocked interface and a disabled switch each produce a fixed status phrase, never "nothing is there". A device unheard for 7 days shows "not heard lately", is never dropped by a quiet look, and expires from the list after 30 days.

**API.** `GET /admin/api/netaudit/nearby` (state + devices), `PUT …/nearby` (switch; step-up, in `stepUpExact`), `POST …/nearby/look` (Check now; the server refuses a second look inside 30 s with a plain message). A route test classifies each (Constitution I.5).

**Keying.** Rows by MAC where known, else by address; `Finding.Device` stays the IPv4 address (the ADR-0017 contract is not touched).

## 5. Phases (each ends with its acceptance tests; TDD, Red→Green→Refactor)

### Phase 0: decisions and evidence (no feature code)
D1–D6 answered; the probes of §6 run with approval; fixtures committed with names, serials, UUIDs and hardware addresses replaced (ADR-0007 style; a test fails on a MAC-shaped string in `testdata`). *Accept:* ledger entry with the captures' facts; the unmeasured items in §2/§6 turned into measured or struck.

### Phase 1: discovery foundation
Build the list in §4. *Accept (all automated unless marked):* parser tests from the fixtures; `go test -fuzz` runs of `dnswire` and the SSDP/description parsers for a fixed time with no panic or unbounded allocation; a hostile-reply test (name with markup, bidi controls, a 64 KB label, a compression loop, 10,000 datagrams, a LOCATION on another host) passes; the socket is not opened with the switch off or with Guardrails local-network off (spy on the opener); a reply from outside scope, from the host itself or from a blocked address is dropped; bounds hold at 513 datagrams and 33 from one sender; cancelled look leaves no goroutine; `GOOS=linux` and `darwin` build and `go vet`; the stub builds elsewhere; `go run ./tools/check-complexity/`. *By hand (owner approves):* one look on vertex and one on the Mac; the result lists devices the owner recognises; a `tcpdump` during the look shows exactly the two datagrams (plus guarded follow-ups) and no packet to a non-scope address.

### Phase 2: the Nearby UI (§7)
*Accept:* component tests for every state (off, never run, running, nothing heard, failed with each fixed reason, empty filter) in Simple and Advanced; a hostile-name test (text appears as text, no element is created, bidi controls are stripped); `npm test`, `npm run build`, and visual specs at 390 px and 1280 px in a light and a dark preset (`retro-dark`, `retro-paper`) with no horizontal scroll; Help entry added and pinned by a test (it must say what is sent and that silence is not proof); `frontend_dist/index.html` committed.

### Phase 3: findings (all in `RunReporter.Augment`, derived from stored state, never from "Check now" events)
Findings are fixed wording, carry no device text, and **do not resolve on a quiet look**: copy `Exposure.Begin`/`Carry` (ledger 2026-11-01: a status set only by the look itself let every early exit read as "resolved").
- **3a `rogue_gateway`** (medium): a description whose device type is an Internet Gateway Device, from an address that is not the inventory's default gateway. Wording: "This device offers to open internet ports for others, and it is not your router." Evidence of false positives is unmeasured (a mesh node may do this); the first look decides whether the severity stays. Resolves only when the same device answers and no longer claims it.
- **3b dropped by the owner (2026-11-03):** the router's UPnP daemon vs KEV/NVD is not worth building without a firmware version (UPnP is off by default and the match would not name what can be patched). The router's model is still shown in the Nearby row with "firmware version not given by the router".
- **3c `rtsp_no_auth`** (high when a stream is readable): for a device that advertises `_rtsp._tcp` or answers on 554/8554 in the scan. One TCP connection through the guarded dialer to the replier's own address on the **fixed ports 554 and 8554 only**, one `OPTIONS` and one `DESCRIBE` of `/`, **no credentials sent**; `200` = readable, `401/403` = protected, anything else = unknown (not a finding). Runs in the nightly audit, not on Check now. This is a new probe shape: it gets the I.7-style security review in the PR and the exact bytes in the Constitution text if the owner wants it there.
- **3d IPP identify** (per D5): `printer-make-and-model` read into the Nearby row only.
*Accept:* each finding has a positive and a negative test and a mutation check (a test must fail without the rule); a quiet look after a finding leaves it open; hostile description text never reaches a finding; guides (`fixguide.go`) and checkTitles added; the report and Help list them.

### Phase 4: unauthenticated AI and admin endpoints (independent of Nearby; may be its own PR)
A new `exposures`-table family with one fixed GET per service and a typed check of the reply (JSON field present), no credentials: Ollama 11434 (`/api/tags`), llama.cpp server and vLLM and other OpenAI-compatible APIs (`/v1/models`, plus llama.cpp's `/props`), Jupyter 8888 (`/api/status`), Prometheus 9090 and node_exporter 9100 (`/metrics` begins with a `# HELP`), Consul 8500 (`/v1/status/leader`), Portainer 9000 (`/api/system/status`), Docker registry 5000 (`/v2/`). 11434 and 8500 join `DefaultPorts` and `ThoroughPorts` (a rule never fires for a port nobody scans, ledger 2026-10-28); the added sweep cost is **to be measured** (the nine-port precedent was +16 s on a /24). Port 9100 is also JetDirect printing, so the check reads the reply before it names the service. *Accept:* fixtures from real services (colima containers, as for Elasticsearch/etcd) with the service name nmap prints captured, not hand-written (ledger 2026-10-30: X11); host-own-service handling per D4; the check works with Nearby off.

### Phase 5: close-out
SPEC-003 changelog for the UI; `docs/architecture/network-audit.md`; ledger lines; ADR-0024 → accepted and I.9 into `CONSTITUTION.md` only on the owner's merge; plan archived after the live check on vertex; `./scripts/check-agent-harness.sh`.

## 6. Probes for the fixtures (nothing is sent without the owner's approval of each)

Sent by a throwaway script **outside the repo**, run by the owner on **vertex first, then the Mac**, ephemeral source port, TTL 1, listening 5 s, printing sender address and raw bytes; a `tcpdump -n -i <iface> 'udp and (port 5353 or port 1900)'` beside it (as in the 2026-10-29 Landlock capture). Names, serials, UUIDs, hardware addresses and account hints are replaced before anything is committed.

| # | Packet | Destination | Purpose |
|---|---|---|---|
| 1 | mDNS question, 46 bytes: `12 34 00 00 00 01 00 00 00 00 00 00` `09 _services 07 _dns-sd 04 _udp 05 local 00` `00 0c 00 01` (PTR, class IN, **no** unicast bit) | `224.0.0.251:5353` | Do responders answer a multicast question from an ephemeral port by unicast, with our id or 0? How many, from whom? |
| 2 | The same question with class `80 01` (QU bit set) | `224.0.0.251:5353` | Only if probe 1 hears little; closes the ledger's open question about whether the QU bit changes the answer |
| 3 | SSDP, 101 bytes of text: `M-SEARCH * HTTP/1.1` / `HOST: 239.255.255.250:1900` / `MAN: "ssdp:discover"` / `MX: 2` / `ST: upnp:rootdevice` / blank line (CRLF endings) | `239.255.255.250:1900` | Who answers, how many datagrams each, and the `LOCATION` shapes |
| 4 | HTTP `GET` of a `LOCATION` that is on its sender's own address, for at most five devices the owner names | that device | Real description pages for the parser; which carry maker/model |
| 5 | After seeing 1: one unicast `PTR` for a service type that device advertised (for example `_ipp._tcp.local`), to that device's `:5353`; then one `SRV` and `TXT` for the instance it returned | one printer, one other device the owner names | Decide whether per-service follow-ups are worth building (they may be cut) |
| 6 | Only if the owner has a camera and names it: `OPTIONS rtsp://<ip>:554/ RTSP/1.0` then `DESCRIBE rtsp://<ip>:554/ RTSP/1.0`, each with `CSeq`, no credentials | that camera | The reply shapes of the `rtsp_no_auth` rule; without a camera the rule stays parked |

## 7. UI

Placement: a **Nearby** card on Network → Devices, below the device list, with Check now; **its switch is on the Settings tab** (ADR-0022: no other tab changes how the network is watched), and the card's off state links there. Tokens and components only: `Panel`, `ToggleField` (with `details`), `Callout`, `SegmentedControl`, `Disclosure`, `StatusTag`, `useNetauditViewMode`. Three mockups are in the scratchpad for the owner's pick (reported in the session; the HTML files are deleted after the build, as before).

- **Simple:** one summary line ("N devices heard, M need a look. Last checked …") and **Check now**; a rogue-gateway alert pinned above the list with a Fix button that opens the existing guided fix; a row or card per device: best name, type icon labelled "advertises as" (the type comes from untrusted lists), status chip, service chips; a problem is one sentence plus **Fix**.
- **Advanced:** a table (raw service types, UPnP device type, port, name source, last heard) and **Copy the results**, built like the DNS card's `copyText` of a bounded JSON report.
- **Desktop:** table, a detail panel, a filter bar (type, status, new). **Phone:** one card per device, detail in a bottom sheet, one Filter button, 44 px targets, no horizontal scroll (the `flex-[1_1_10rem]` lesson, ledger 2026-11-01).
- **Status words:** "Heard just now", "Not heard lately", "Needs a look", "New"; never "offline" or "gone". **"New" is the inventory's own `new` field** (unapproved and seen within 7 days), not a second definition (ledger 2026-11-02: they were the same set). A device the inventory does not know shows "Not in your device list".
- **States:** off (what is sent, the switch), never run, running, nothing heard (says silence is not proof), failed (fixed reason: local network off in Guardrails, no usable interface, socket refused, not available on this system), empty filter.

## 8. Fallback if I.9 is not approved
Unicast only: ask each device in the inventory, through the guarded dialer, for its service types (`_services._dns-sd._udp.local` PTR to its `:5353`) and send the SSDP datagram to each device's `:1900`. No amendment, no new socket; Linux-only discovery of *which* devices (the neighbour table), so a Mac finds none unless addresses are typed in. Phases 2–4 are unchanged.

## 9. What was built, where it differs from the plan, and what is left

**Built (2026-11-03):** `dnswire` (shared reader, fuzzed; `names` uses it), `platform/mcast` (the socket, tested on loopback incl. bounds and cancel), `core/netaudit/nearby` (probes, mDNS parser, collector with `Request`/`Run`, fuzzed), `upnp.Identity`/`Client.Identify`, `models.NearbyState` + `persistence.NearbyStore`, `NearbyConfig`, `rogue_gateway`, `BuildNearbyView`, the three routes (the switch is step-up), `ServiceChecks` (`rtsp_no_auth`, `ai_api_exposed`, `admin_service_exposed`) with guides, titles and remediation, nine added scan ports, the Devices **Nearby** card, the Settings switch card, Help topic, SPEC-003 2.25, 34 visual specs (phone and desktop, retro-dark and retro-paper), `go test ./...`, `check-complexity`, `npm test` and `npm run build` pass.

**Differs from the plan:** (1) the **switch is on the Settings tab** (ADR-0022's rule that no other tab changes how the network is watched); the Devices card shows the off state and links there. (2) `rtsp_no_auth` runs on any open 554/8554 the **scan** found; it does not read `_rtsp._tcp` from Nearby (one path, works with Nearby off). (3) **Portainer is not built**: the direction of `/api/users/admin/check` (204 vs 404 for "no admin yet") is from memory, so it needs a real container capture first (ledger rule). (4) The per-service unicast follow-ups (probe 5) were **cut**; the first look decides if they are ever needed. (5) 3b dropped, IPP identify-only (D5) is **not built** either: the printer's `printer-make-and-model` needs an IPP request that has no capture yet. (6) The Nearby row reads maker/model/name from the UPnP page and the service instance name from mDNS only. (7) The audit asks for a fresh look in the background and reads the kept state, so a night's report may show the previous look's gateway claim.

**Left (needs the owner):** Phase 0 probes 1–3 on vertex and the Mac (§6: approve the exact packets, then replace the hand-built parser tests with fixtures from real answers and strike the "unmeasured" claims: unicast answers to an ephemeral-port multicast question, id 0 vs our id, the QU-bit question); a live look on vertex; the sweep-cost measure for the nine added ports; merging (which brings I.9 into force) and the deploy; ADR-0024 → accepted; archive this plan after the live check.
