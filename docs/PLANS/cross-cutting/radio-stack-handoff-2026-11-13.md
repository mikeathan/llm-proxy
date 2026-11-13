---
status: partial
date: 2026-11-13
last_reviewed: 2026-11-13
related_specs: [SPEC-006]
constitution_references: [I.5, I.8, I.11, II.3, III.6, IV.6]
evidence: Session of 2026-11-13 (investigation, owner-run vertex captures, literature search, plan set). Replaces nothing; delete this file when R2 merges (harvest first, documentation-stewardship).
---

# Radio stack: handoff for the next session (2026-11-13)

Start here, then open only what you need. **R0 and R1 are merged (#245). R2 is built except the privileged helper actions (item 3 below): that is the next work.**

## State

- **R0 and R1 are merged** (PR #245, v0.84.0, on `main` as `00af060f`). **Branch `radio-r2-capture`** (commit `e8d0052e`: I.16 and the parser; the vault, store and recorder are after it) is cut from that `origin/main`, empty except for this handoff (the temporary capture script is deleted on purpose; the owner does not want it kept, and the two captures it made are the only input R2 needs). Nothing pushed, no PR. Run git steps one at a time and ask (a chained commit, push and PR was once refused by the auto-mode classifier).
- **Decided** (owner, 2026-11-13): ADR-0052 and ADR-0053 accepted; SDK ports; `modernc.org/sqlite`; duty-cycled windows; the UK-law question is an **open risk** (capture must not default on before the owner takes advice). Constitution I.16 (plan §6) is **not** in `CONSTITUTION.md`: it is the first item of R2.
- **What R1 built** (plan "Progress"): `stacks/radio` (idle by construction: no goroutine, no write; discovery from `/sys`; capability list), SDK `PortVault`/`Vault`/`StorageReporter`, `models.RadioConfig` + storage `Radio()` view, registry stack `radio`, `hostedStacks()`, the Radio page. The owner ran it on vertex (2026-11-13): the Sensors page works (built-in card refused, the USB Realtek 'Can listen'); the owner calls the layout temporary.
- **Real captures exist on the owner's Mac** (not in the repo, hold neighbours' addresses): `~/radio-pcap/wlxf0090da058a3-ch60-20261113-152701.pcap` (5 GHz, channel 60, 81,798 packets, 8.7 MB: 25k CTS, 20k RTS, 3,388 beacons, 3,632 data frames, 54 probe responses, 30 probe requests, 13 authentication / deauthentication / disassociation frames) and `…-ch1-20261113-152954.pcap` (2.4 GHz, 2,288 packets: 1,357 beacons, 253 probe responses, 116 probe requests, 132 data, 1 authentication and 1 association request). Link type radiotap (`IEEE802_11_RADIO`); each frame carries signal per antenna, frequency and rate; the adapter is the Realtek `wlxf0090da058a3`. The Mac has `tcpdump` but not `tshark`. The owner has **not yet said when they toggled a phone's Wi-Fi**, so the join and leave frames are not matched to an event: ask.
- **Checks run at the end of R1 (all pass):** backend build, tests, complexity; `stacks/*` tests (`GOWORK=off`); frontend `npm test`, `npm run build`, `npm run test:visual`; harness. Baselines for R2: run the same.

## Owner decisions already made (2026-11-13, quote-level)

New **Radio** stack (not a layer of Network) · retention **14 days for neighbours' devices, 30 days for everything else** (daily rollups 30 days by my assumption, adjustable) · small bounded storage that cannot fill the disk · **SQLite yes** · hashed or encrypted at minimum information but matchable after a restart or re-entry (keyed tag for others, sealed address for the owner's) · classes **Infrastructure, Personal, Neighbour, Transient** (I added **Unknown**), rules adjustable · generic for **any number of adapters**, extensible, pluggable, stack on or off, **does nothing with no adapter**, unused capabilities listed in the UI · a system-wide data management page · "r0 yes to all". **Owner constraint for captures: never cut vertex's connection** (the owner works over SSH, and the built-in card `wlp2s0` carries it: I once told them to use it, they lost the connection; a new capture, if ever needed, must refuse any interface that carries the connection and any non-USB adapter, and restore the adapter on every exit path).

## What is left, in order (each item: what it needs from the owner)

1. **R2 Capture** (plan §9), in this order, TDD:
   1. **DONE** (owner approved 2026-11-13, commit `e8d0052e`): Constitution I.16 text into `CONSTITUTION.md` (draft in plan §6) and the harness check; **ask the owner to read the diff before building on it** (it is a Constitution change: ask-first).
   2. **DONE** (`stacks/radio/frame.go`, fuzzed 3M execs clean, fixtures in `testdata/`, replay matches tcpdump; commit `e8d0052e`): the **parser**: radiotap and the 802.11 header from bytes, strict bounds, fuzzed (`go test -fuzz`), in `stacks/radio`, typed facts out (a beacon with its security fields, a probe request, an association, authentication, deauthentication and disassociation with reason code, a data or control frame as a count and a signal sample). Fixtures: **derive small synthetic frames** from the two captures (anonymise every address); never commit a real pcap. A replay test: the synthetic fixture gives the same typed facts as `tcpdump -nn -e` shows for the same frames.
   3. **LEFT.** Helper actions `radio_probe` and `radio_listen` in the closed catalogue (I.11, ADR-0034; read `backend/internal/platform/privhelper` and the catalogue first), CAP_NET_ADMIN question (the helper has CAP_NET_RAW only: this is the plan's §6 open point), channel read-back, adapter back to managed after each window, USB-only and never the connection's interface (reuse the script's rule in Go).
   4. **DONE** (second commit on the branch): the core `stackport.Vault` adapter over `platform/secretcrypto` (subkey per purpose) and `modernc.org/sqlite` in `stacks/radio/go.mod` (ask-first, **already approved**), the store with the 20 MB cap, retention (14 / 30 days) and eviction, a test that floods rows and a test that greps the DB for any raw address of another person.
   5. **DONE** `StorageReporter` implemented by the stack. Owner agreed (2026-11-13): the helper runs jailed `ip` and `iw` (no netlink library); R2 order was adapter and store first, helper actions last.
2. R3 to R8 as in the plan. The data page [`data-management-page.md`](data-management-page.md) D1 can start independently.

## Next work: helper actions `radio_probe` and `radio_listen` (brief for a fresh session)

Decided with the owner (2026-11-13): the helper runs **jailed `ip` and `iw`** (no netlink library, no new dependency); a new privilege class **`radio`** with its own unit, `CAP_NET_ADMIN` (plus `CAP_NET_RAW` to read frames), off by default, the three gates and step-up of I.11 (Constitution I.16 is in force once this branch merges). Start in TDD, with a fake `Runner` (nothing needs root to test).

What was read in `backend/internal/platform/privhelper` (verify before relying on it):
- `catalog/catalog.go`: `Action{ID, Class, Switch, Caps, Jail, Result}` built with `row(...)` (typed params, `check` to scope and canonicalise, `build` to make the whole argument list); classes `ClassNet`, `ClassLog` (add `ClassRadio`). `catalog/capture.go` is the template (`capture_window`: strict interface-name regex, seconds range, program is the helper's own binary).
- `helper/helper.go`: `Config{Class, Actions (root-owned drop-in), Capability (ONE capability, int), Programs, Runner, Identify, MainPID, PtraceScope, Ambient}`. **Open point:** `Capability` is a single number and the `radio` class needs two (`CAP_NET_ADMIN` and `CAP_NET_RAW`); check whether `Ambient.Raise` can be called twice or the config must become a list. `helper/caps_linux_test.go` shows how the capability is tested under a unit (`scripts/check-helper-under-unit.sh`).
- Also touched by `capture_window`, so by a new action: `wire/` (typed results, caps like `CaptureMaxSeconds`), `cmd/llm-proxy-helper/main.go` (maps program names), `client/` (the service side), `internal/app/privhelper.go` and `capturewindow.go` (the route, grant and switch), `scripts/install-helper.sh` (one unit per class, drop-in), `internal/archtest/boundary_test.go`, `transport/http/helperroutes_test.go`.

What the two actions must do (Constitution I.16, read it first):
- `radio_probe`: one shot, nothing sent; returns an adapter's capabilities (bands, widths, modes, from `iw phy`/`iw dev`) as bounded typed records. This fills the "not read yet" capabilities on the Sensors page.
- `radio_listen`: one named adapter, monitor mode, listed channels, a window (initial 20 s a channel, a cycle every 5 min, both settings with caps), stop on time or frame cap; returns radiotap plus 802.11 header bytes only (512 for a beacon, 64 for anything else), never parsed in the helper. Set the channel, **read it back**, discard a window that did not land on it. **Refuse** a non-USB adapter and any interface that carries the host's default route or an SSH session. Return the adapter to managed mode on **every** exit path (including cancel, timeout and crash of the capture program).
- The stack side then feeds frames to `Parse` and `Recorder.Record` (that is R3: the pipeline, the owner-device list, the duty cycle scheduler, hourly rollups). A cached master key in the vault adapter belongs to R3 (see the ledger).

Owner constraints that still stand: never cut vertex's connection (the owner works over SSH; the built-in card `wlp2s0` carries it); capture must not default on (UK-law risk is open); do not commit a real pcap.

## Facts the owner can still supply (they sharpen the classifier and the findings)

On vertex (kernel 7.0.0-42), router `fc:34:97:35:28:f4` on 5 GHz channel 60 (DFS, 5300 MHz), mesh node `24:4b:fe:30:5a:6x`, SSIDs `netsink`, `netsink-2G`, `netsink-Guest`. Unidentified: `f0:24:f9:ee:81:1c` (steady ~1.2 kB/s upload on 2.4 GHz at -44 dBm, 21% retries; a hub or camera?), `14:c1:4e:6b:06:0b` (bursts, 26% retries, probes for `netsink`), `0c:91:60:bd:5f:6e` (about -18 dBm: probably vertex's own card, compare `ip link show wlp2s0`), `72:0a:56:29:c8:8a` (tried to join a neighbour's SKY router five times: a stale saved network?), the hidden AP `d6:be:dc:51:7b:cf` named "Streaming Stick" on the router's channel (whose?), and **the Zigbee hub's address** (the owner switched Wi-Fi networks on a phone a few times during the capture, which explains the extra authentication and deauthentication frames; no times were noted, so no frame is matched to an event).

## Traps (also in the ledger entry)

tshark here prints subtypes as `0x0008`, booleans `True`/`False`, SSIDs hex, reason codes `0x0003` · `tshark -c N` with `-r` counts packets read, not matched · `radiotap.present.*` exists on every frame, filter `== 1` · `sudo` makes an `out-*` folder root-owned · the Realtek once heard 0 frames on channel 1, so read back the channel after setting it · an 802.11ac adapter decoded **0 Wi-Fi 6 frames** from this router (so per-device bytes on 5 GHz undercount; control and management frames are complete) · the service unit has no netlink, so capability detail needs a privileged one-shot probe · `0e8d:c616` is the Bluetooth half of the built-in card, not an adapter.

## Recovery of the capture scripts (removed from the tree on purpose)

`run.sh` (finds every USB adapter, monitor mode, per-channel survey, a guided event capture with typed notes) and `analyse.sh` (the tshark report) were added in PRs #241 to #243 and are removed by this branch. The newest version, with the run-as-any-user fix, is **commit `826ef161` on `origin/wifi-monitor-analyse-as-any-user` (PR #244, left open and unmerged on purpose; do not delete that branch; it will not merge cleanly once the files are gone)**; the version on main before this branch is `ddfa9d2b`. Restore with `git show 826ef161:docs/audits/2026-11-13-wifi-monitor-n0/analyse.sh` (and `run.sh`). Reasons to restore: a Wi-Fi 6 adapter measurement on 5 GHz, or fixtures for R2. Not yet fixed in any version: the legend's reason codes print as `0x0003` unless the one-line hex fix from the 2026-11-13 session is applied (convert column 10 as column 2 is converted).

## First files to open

[`radio-stack.md`](radio-stack.md) §1 to §3 and §6 · `stacks/sdk/manifest.go` (`Manifest`, ports) and `stacks/sdk/ports.go` · `stacks/agents/` (the smallest working stack: `agents.go`, `watch.go`) and ADR-0051 (how a stack gets a new port: `PortSecretScan` is the template) · `backend/internal/platform/db/db.go` · `backend/internal/platform/secretcrypto/crypto.go` · ADR-0034 and Constitution I.11 (the helper) · `docs/architecture/network-capabilities.md` (read before proposing any network feature). First command: `git log -1 && ./scripts/check-agent-harness.sh`.
