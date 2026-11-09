---
status: partial
date: 2026-11-09
last_reviewed: 2026-11-09
related_specs: [SPEC-003, SPEC-006]
constitution_references: [I.5, I.7, I.8, I.11, II.3]
evidence: CONSTITUTION.md I.8 and I.11; backend/internal/platform/privhelper/{capture,wire,catalog}/ (read 2026-11-09); core/netaudit/phonehomefindings.go:113 (the DNS finding's only rule); docs/PLANS/cross-cutting/network-capture-window-plan.md; verify-deployment run against vertex 2026-11-09 (64 open findings, 9 of them "does not use the router's DNS"); mockups at https://claude.ai/artifact/KvsXHYiLkx9UpZgs6vs6rF (private; boards A, B, C).
---

# Network: a calmer Findings tab, and a traffic view for the computer running the program

Branch `network-host-traffic-and-findings-ui` (from origin/main, v0.66.0). The owner asked for both in one piece of work (2026-11-09), design first, and approved on 2026-11-09: both Findings layouts, the Traffic view (mockup C), demoting the DNS finding, the Constitution I.13 bounds, names as well as addresses and sizes, pure Go. **Built on this branch, uncommitted** (v0.67.0): S1 to S4 and S6 below, except the TLS server name (S5) and the guardrail-override fix. Not run on vertex.

## 1. Findings tab: three decisions for the owner

The screenshot (24 problems; 9 rows of "A device does not use the router's DNS", identical wording, bare IPs, a truncated `Accept…`) shows noise. The only rule behind that finding is "online a day and not seen asking the router's DNS" (`phonehomefindings.go:113`). It cannot tell a device that bypasses the router from one the router never logs, a phone with Private DNS, or a Chromecast. **Not verified:** why those nine are silent.

**Layout (mockups A and B):**
- **A, Needs you / Notes (recommended).** High and medium findings and anything new sit on top as full cards. Low findings sit below as one collapsed row per check; an open row shows device chips (your name, else maker, else IP) and one "Accept all N". A note that gets worse moves up. Simple/Advanced stays as it is.
- **B, by device.** One card per device with all its findings; worst device first. Better for "what is wrong with this thing", worse for "everyone has the same note".
- Both can ship (a "By check / By device" switch) but that doubles the tests; I recommend A first.

**The rule (my recommendation: demote it).** The "not heard asking the router" fact becomes a small chip on the device row (Devices tab), not a finding. `phone_home_encrypted_dns`, `new_place`, `beaconing` and `known_bad` stay findings (they say something specific). Effect after the next audit: **9 findings "resolve"**, so `verify-deployment.py --compare ~/vertex-findings-baseline.json` will list them as gone; that is the change, not a regression. This is a SPEC-003 behaviour change and needs the owner's yes. The alternative is to keep it as a finding but only for devices seen sending DNS elsewhere, which needs flow data this program does not have until section 2 runs on the device in question (only this computer).

**Smaller extras that ride along:** the S6 close-out (the handoff and S6 plan harvested into the P3b design and the ledger, docs only, no decision needed); device names instead of bare IPs wherever a finding names a device. **Not included, waiting on the owner:** the approved-terminal-override fix (two options in `docs/learnings/guardrails.md:44`).

## 2. Traffic on this computer (mockup C)

A button on the Network page: "Listen now" for up to 60 seconds on the default interface, then a picture of this computer's own traffic. It is step-up, on demand only, never scheduled and never started by an agent (I.11). It sees all of this computer's own traffic whatever the Wi-Fi vantage, because the packets are its own.

**What it shows:** one plain sentence (bytes in and out, how concentrated, how much encrypted); where the data went (ranked destinations with names, "no name seen" when none); a kind-of-traffic bar (HTTPS, DNS, QUIC/UDP, other); bytes per second across the window with the busiest second marked; a conversations table (destination, port, kind, sent, received); and a status line (complete, stopped at the frame limit, estimated). SVG and CSS only: the frontend has no chart library (`frontend/package.json`) and none is needed.

### Design (pure Go, no tshark, no new package)

- **A new catalogue action `capture_host`** with its own command, filter, record layout and caps. **`capture_window` stays byte-identical** (`CaptureSnap` 64, its tests pin the layout). The helper is the same binary and the same packet-socket code path; the unit already holds `CAP_NET_RAW`.
- **Filter (classic BPF, built in code, fixed):** IPv4 and IPv6 TCP, UDP and ICMP only (no ARP, no other EtherType). An ordinary frame is cut **exactly after the first four bytes of its L4 header**, which are the ports (the filter computes the length from the IPv4 header length: 38 bytes without options, 58 for IPv6, never more than 78), so the addresses, ports and size are kept and **no payload byte** is. A DNS message (port 53) keeps up to **512** bytes, because a name is in it. A first version kept a flat 128 bytes, which on a TCP frame is about 70 bytes of payload; review caught it and the filter now cuts at the ports (verified on a real kernel).
- **Volume is bounded by the helper without reading a frame.** Window 60 s at most; at most 60,000 records and 8 MiB of output; 12 presses an hour. A busy host would exceed that in seconds, so ordinary data frames are **sampled 1 in N in the kernel filter** (N starts at 1, and the capture program, counting kept frames a second, re-attaches the filter with N doubled when the count passes a budget). DNS and handshake frames are never sampled. Each record carries the N in force, so the service multiplies bytes back and the screen says "estimated". A count of frames is not parsing a frame.
- **Records are variable length** (8-byte header: milliseconds, kept length, sampling N; then the bytes), so a typical IPv4 record is 46 bytes and only a DNS message pays for 512.
- **The helper parses nothing** (I.11 stands): the service reads the headers, builds a flow table and discards every raw byte before it stores anything.
- **Service side:** new core package `core/netaudit/hosttraffic` (next to `framewire`): reads Ethernet, IPv4/IPv6, TCP/UDP headers into typed fields; aggregates while streaming into a capped table (512 flows and a single "other" bucket; never all records in memory); reads DNS answers (A and AAAA, compression pointers bounded, at most 16 answers a message) into an address to name map; in the SNI slice reads the first handshake record's server name. A frame that is short or malformed is counted as unreadable, never guessed.
- **Names are untrusted text** and follow I.8's rule: plain DNS names only (letters, digits, hyphen, underscore, dot, lower case, at most 253 characters), taken from a DNS answer or a TLS server name seen in the same window for that address, never from a reverse lookup (nothing is sent). Names appear as text on this view only: never in a finding, alert, log, prompt, model summary or the Timeline summary. **Only DNS answers name a conversation today: a TLS server name was not built (S5), so a conversation whose name was never looked up in the window shows its address.**
- **What is kept:** only the last summary (totals, the capped flow list with names, per-second bytes, status), in one stack-agnostic store registered for factory reset (`withReset`), with the time. No frame bytes, no per-packet record.
- **Route:** `POST /admin/api/helper/capture_host` (generic helper route, step-up, one-use grant, added to the step-up table; the route test already ties routes to the catalogue) and `GET /admin/api/netaudit/host-traffic`.
- **Platform:** Linux only. On a Mac the card says "this computer cannot listen" (`StopUnsupported`); I have not checked a macOS route and do not claim one.

### Installing it (no package to install)

1. Deploy the new version.
2. Re-run `sudo ./setup.sh --install --build --unattended --helper` (or `sudo scripts/install-helper.sh net --install --bin backend/llm-proxy-helper --group llm-proxy`; both now list `capture_host`) so the root-owned drop-in lists it (one of I.11's three gates).
3. Switch the action on in Network settings (a step-up switch). Then press Listen.

### Facts to check before the build (each can change the design)

| Fact | Why | Who |
|---|---|---|
| `resolvectl status` on vertex | **Answered 2026-11-09:** DNS over TLS is off; the default route is `wlp2s0`; servers are AdGuard public DNS (`94.140.14.14`, `94.140.15.15`) then the router, so DNS answers are readable on `wlp2s0`. | Owner (done) |
| Does a received frame on vertex exceed the MTU (TSO/GSO)? | Bytes come from the IP length field, not the cut frame, and large offloaded frames report their own size. | First real window, from the summary |
| Does the filter test's BPF interpreter support scratch memory? | A TLS handshake test needs the TCP header length (X-register arithmetic). If extending it is large, the SNI slice drops out and DNS answers carry the names (QUIC and Encrypted Client Hello hide the server name anyway). | Me, in slice S4 |

## 3. Constitution I.13 and ADR-0040

Written (proposed; in force when the owner merges the pull request that carries them): `CONSTITUTION.md` I.13 holds the bounds the owner approved, with the corrections the review made (ordinary frames keep headers and the ports only; the browser never gets the raw bytes), and `docs/decisions/ADR-0040-the-traffic-look-is-its-own-helper-action-with-its-own-limits.md` holds the decision and the alternatives.

## 5. Slices (each its own commit; `go build ./...` after each Go edit; TDD; no slice is started before the owner's pick)

| Slice | Content | Gate |
|---|---|---|
| S0 | This plan, mockups, README row | Owner picks layout A/B/both, the DNS rule, and approves the I.13 bounds |
| S1 | Findings tab (chosen layout, "Accept all", names for devices), the DNS rule, S6 close-out; tests pin old wording first | frontend `npm test`, `npm run build`, visual test, committed `frontend_dist/index.html` |
| S2 | `capture_host` in the helper: catalogue row, wire layout, filter with sampling and its portable interpreter test, capture program, fake in `helpertest`, fitness tests; ADR-0040; I.13 into CONSTITUTION | `go test ./...`, complexity, Linux container run of the packet code |
| S3 | `hosttraffic` parser and aggregator (DNS answers), store, route, step-up entry, summary | tests with captured-shape fixtures; fuzz-style tests for malformed input |
| S4 | The Traffic card: the picture in mockup C, states for off, unsupported, running, empty, error, cut at the limit | visual spec |
| S5 | TLS server name (only if the filter interpreter extension is small) | measured |
| S6 | SPEC-003 section, learnings, harness, version 0.67.0 | `check-agent-harness.sh` |

## Remaining Work (handoff, 2026-11-09)

**Branch and commit state.** Branch `network-host-traffic-and-findings-ui`, cut from `origin/main` (b2c4e2a9, v0.66.0 deployed). **Nothing is committed or pushed**; all of it is in the working tree (VERSION 0.67.0). When committing, include `backend/internal/transport/http/frontend_dist/index.html` (the built UI) with the frontend change; `./scripts/check-agent-harness.sh` passes. Suggested commits: Findings UI and the DNS demotion; `capture_host` (helper, filter, wire, route, Constitution I.13, ADR-0040); the Traffic tab; docs.

**Built.** S1 Findings: "Needs you / Notes", names beside addresses, "This is fine for all N", By kind / By device, the top sentence counts only what needs you, `phone_home_around_dns` removed and `dns_silent` shown on the device row. S2 `capture_host` (pure Go; ordinary frames cut at the ports; DNS up to 512 bytes; kernel sampling; response frame 16 MiB; `setup.sh` and `install-helper.sh` list the action). S3 `core/netaudit/hosttraffic`, `models.HostTraffic`, `persistence.HostTrafficStore`, `GET /admin/api/netaudit/host-traffic`, and the capture routes no longer send raw bytes to the browser (`HelperDeps.OmitRaw`). S4 the Traffic tab. S6 SPEC-003 2.55, the ledger, API reference and setup text.

**Checks run (all on this machine unless noted).** `go build ./...` (also GOOS=linux amd64 and arm64), `go test ./...` all packages ok, `go run ./tools/check-complexity/` clean, gofmt clean on every file I touched (pre-existing: `app/bootstrap.go`, `app/netaudit.go` one line, four handler test files), `npm test` (277 files, 2,805 tests), `npm run build`, `npm run test:visual` (354 passed, two new specs), `check-agent-harness.sh` PASS. The packet code ran on a real Linux kernel in a container with `CAP_NET_RAW` (`docker run --security-opt seccomp=unconfined -v "$PWD":/src -w /src golang:1.26 go test ./internal/platform/privhelper/capture/ -count=1 -v -run Live`): the kernel accepts the filter at every rate, a DNS message is kept whole, an ordinary datagram at exactly 38 bytes, sampling thins and rate 1 keeps all. (The `helper` package's capability test fails in that container only because it runs as root with every capability; run it with `-u 1000:1000 -e HOME=/tmp -e GOCACHE=/tmp/gocache` and it passes. I did not touch that package.)

**Checks NOT run.** Nothing on vertex. Not run end to end: the helper's runner, client and the route together (only the pieces, with fakes), so a ceiling between the program and the browser is read, not measured (the runner's `MaxOutput` is set from the catalogue's 8 MiB, the client refuses output over the cap, and the frame limit is 16 MiB). Not run: macOS (the card says it cannot listen). Not measured: how a busy window behaves (frames the kernel drops while the filter is replaced are counted in the trailer and shown).

**Traps found.** (1) A 128-byte snap keeps payload; the cut must be at the ports (a BPF `ret A` computed from the IPv4 header length). (2) A burst of 400 datagrams before the reader started kept 112 at rate 1: the packet socket's queue overflows, and the kernel's drop count is the honest answer. (3) The helper's response frame was 1 MiB, under the 8 MiB output cap; it is 16 MiB now (requests stay 4 KiB). (4) The helper route sent the raw output to the browser; capture actions now omit it. (5) `beforeEach(() => mock.mockReset())` returns the mock and Vitest calls it as cleanup (`docs/learnings/admin-ui.md`). (6) `dnsSilence` runs the phone-home judge on each `GET /admin/api/netaudit/devices`; the list is not polled, so it is cheap enough, but a faster list would want a silent-only call.

**Not built, in order.** (1) The owner's checks on vertex: deploy v0.67.0, re-run the helper install (`sudo ./setup.sh --install --build --unattended --helper`), switch "Traffic on this computer" on in Settings, press Listen once, then `scripts/verify-deployment.py --base http://vertex.local:4001 --workspace-id workspace-1 --automation network-audit --compare ~/vertex-findings-baseline.json` after the next audit (the nine "not using the router's DNS" findings should be gone; the two new ones, `.241` and `.7`, still need the owner's answer). (2) S5, a TLS server name for conversations with no DNS answer (the filter would need the TCP header length; its own amendment of I.13). (3) The approved-terminal-override fix, which waits for the owner's choice (`docs/learnings/guardrails.md:44`). (4) The S6 close-out of `network-stacks-handoff-2026-11-09.md`, which waits for the owner's vertex checks listed in it. **First file to open:** this plan, then `backend/internal/platform/privhelper/capture/filter_host.go`.
