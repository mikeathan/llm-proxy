---
status: partial
date: 2026-11-13
last_reviewed: 2026-11-13
related_specs: [SPEC-006]
constitution_references: [I.5, I.8, I.11, II.3, III.6, IV.6]
evidence: Session of 2026-11-13 (investigation, owner-run vertex captures, literature search, plan set). Replaces nothing; delete this file when R2 merges (harvest first, documentation-stewardship).
---

# Radio stack: handoff for the next session (2026-11-13)

Start here, then open only what you need. **R0, R1 and the first R2 slice are merged (#245, #246). The helper actions are built (item 3) on branch `ci-faster-backend-test-and-coverage` (uncommitted, never run on a real host); the next work is the list under "After the helper actions".**

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
   3. **BUILT, UNCOMMITTED, NOT RUN ON A HOST** (branch `ci-faster-backend-test-and-coverage`): helper actions `radio_probe` and `radio_listen` in the new `radio` class (see the plan's Progress). Decisions: one channel a call (the stack loops over the list), channel read-back returned in `Result.Report` and also checkable from the radiotap frequency of the frames, USB-only and never-the-connection guard in `helper/radioguard.go`, `ip` and `iw` jailed. Tests use fakes; nothing ran as root.
   4. **DONE** (second commit on the branch): the core `stackport.Vault` adapter over `platform/secretcrypto` (subkey per purpose) and `modernc.org/sqlite` in `stacks/radio/go.mod` (ask-first, **already approved**), the store with the 20 MB cap, retention (14 / 30 days) and eviction, a test that floods rows and a test that greps the DB for any raw address of another person.
   5. **DONE** `StorageReporter` implemented by the stack. Owner agreed (2026-11-13): the helper runs jailed `ip` and `iw` (no netlink library); R2 order was adapter and store first, helper actions last.
2. R3 to R8 as in the plan. The data page [`data-management-page.md`](data-management-page.md) D1 can start independently.

## After the helper actions (brief for a fresh session)

Verify first, on vertex, with the owner (needs root and the USB adapter `wlxf0090da058a3`; never the built-in `wlp2s0`, which carries the owner's SSH): `sudo scripts/install-helper.sh radio --install --bin backend/llm-proxy-helper --group llm-proxy`, switch the two actions on in host settings, call `radio_probe` (does the jailed `iw list` run? is `/sys` or `/etc` needed in the jail?), then one `radio_listen` on channel 60 for 10 s (does the jailed `ip`/`iw` get `CAP_NET_ADMIN`? does the adapter return to managed? do frames come back with frequency 5300?). Unverified guesses: Landlock lets `ip` and `iw` run and reach netlink; `RestrictAddressFamilies=AF_UNIX AF_NETLINK AF_PACKET` is enough for the unit (the main-pid check uses systemctl); `ThreadAmbient.Raise` twice on one thread works.

Then, in order:
1. **Scheduled windows run under a bounded session: built, needs the owner's reading.** Constitution I.17 and ADR-0054 (both `proposed`; the owner's merge accepts them). The core has `platform/privhelper/session`, the `Standing` flag on the two radio actions, `port.ForStack` (a per-stack port that serves a request with no grant only while an open session of that stack approved exactly that request), and the routes `GET/POST/DELETE /admin/api/helper/sessions` (POST is step-up). R3 gives the radio stack `helperRoutes.StackPrivileged("radio")` as its `Privileged` port and calls it with no grant. Still to build: the Radio page control to start a session (hours, adapter, channels), a visible "listening until" state and a stop button; the scheduler itself.
2. **`Result.Report` is not carried by the SDK.** `stacksdk.PrivilegedResult` has no field for it (adding one changes a public interface: ask first). Until then the service can check a window's channel from the radiotap frequency in the frames.
3. **The service side of the radio unit:** status (installed? switched on?) for the Radio page (the net status reads `helperGatesFile`), the Sensors page filling "not read yet" from `radio_probe` (the stack parses `iw list`, mapping each `phy` to an interface through `/sys/class/net/<if>/phy80211/name`), then R3 (the pipeline from window bytes to `Parse` and `Recorder.Record`, the scheduler, hourly rollups, a cached master key in the vault adapter).
4. `setup.sh --helper` installs only the net class; the radio unit is installed with `scripts/install-helper.sh radio` (decided so capture cannot arrive by accident); `setup.sh --no-helper` removes both.

Owner constraints that still stand: never cut vertex's connection; capture must not default on (UK-law risk is open); do not commit a real pcap.

## Facts the owner can still supply (they sharpen the classifier and the findings)

On vertex (kernel 7.0.0-42), router `fc:34:97:35:28:f4` on 5 GHz channel 60 (DFS, 5300 MHz), mesh node `24:4b:fe:30:5a:6x`, SSIDs `netsink`, `netsink-2G`, `netsink-Guest`. Unidentified: `f0:24:f9:ee:81:1c` (steady ~1.2 kB/s upload on 2.4 GHz at -44 dBm, 21% retries; a hub or camera?), `14:c1:4e:6b:06:0b` (bursts, 26% retries, probes for `netsink`), `0c:91:60:bd:5f:6e` (about -18 dBm: probably vertex's own card, compare `ip link show wlp2s0`), `72:0a:56:29:c8:8a` (tried to join a neighbour's SKY router five times: a stale saved network?), the hidden AP `d6:be:dc:51:7b:cf` named "Streaming Stick" on the router's channel (whose?), and **the Zigbee hub's address** (the owner switched Wi-Fi networks on a phone a few times during the capture, which explains the extra authentication and deauthentication frames; no times were noted, so no frame is matched to an event).

## Traps (also in the ledger entry)

tshark here prints subtypes as `0x0008`, booleans `True`/`False`, SSIDs hex, reason codes `0x0003` · `tshark -c N` with `-r` counts packets read, not matched · `radiotap.present.*` exists on every frame, filter `== 1` · `sudo` makes an `out-*` folder root-owned · the Realtek once heard 0 frames on channel 1, so read back the channel after setting it · an 802.11ac adapter decoded **0 Wi-Fi 6 frames** from this router (so per-device bytes on 5 GHz undercount; control and management frames are complete) · the service unit has no netlink, so capability detail needs a privileged one-shot probe · `0e8d:c616` is the Bluetooth half of the built-in card, not an adapter.

## Recovery of the capture scripts (removed from the tree on purpose)

`run.sh` (finds every USB adapter, monitor mode, per-channel survey, a guided event capture with typed notes) and `analyse.sh` (the tshark report) were added in PRs #241 to #243 and are removed by this branch. The newest version, with the run-as-any-user fix, is **commit `826ef161` on `origin/wifi-monitor-analyse-as-any-user` (PR #244, left open and unmerged on purpose; do not delete that branch; it will not merge cleanly once the files are gone)**; the version on main before this branch is `ddfa9d2b`. Restore with `git show 826ef161:docs/audits/2026-11-13-wifi-monitor-n0/analyse.sh` (and `run.sh`). Reasons to restore: a Wi-Fi 6 adapter measurement on 5 GHz, or fixtures for R2. Not yet fixed in any version: the legend's reason codes print as `0x0003` unless the one-line hex fix from the 2026-11-13 session is applied (convert column 10 as column 2 is converted).

## First files to open

For the helper actions: `backend/internal/platform/privhelper/catalog/radio.go`, `helper/helper.go` (`runAround`) and `helper/radioguard.go`, `capture/radio.go`, `wire/radio.go`, `cmd/llm-proxy-helper/main.go`, `docs/services/llm-proxy-helper-radio@.service`.


[`radio-stack.md`](radio-stack.md) §1 to §3 and §6 · `stacks/sdk/manifest.go` (`Manifest`, ports) and `stacks/sdk/ports.go` · `stacks/agents/` (the smallest working stack: `agents.go`, `watch.go`) and ADR-0051 (how a stack gets a new port: `PortSecretScan` is the template) · `backend/internal/platform/db/db.go` · `backend/internal/platform/secretcrypto/crypto.go` · ADR-0034 and Constitution I.11 (the helper) · `docs/architecture/network-capabilities.md` (read before proposing any network feature). First command: `git log -1 && ./scripts/check-agent-harness.sh`.
