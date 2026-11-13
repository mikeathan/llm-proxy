---
status: partial
date: 2026-11-13
last_reviewed: 2026-11-13
related_specs: [SPEC-006]
constitution_references: [I.5, I.8, I.11, II.3, III.6, IV.6]
evidence: Session of 2026-11-13 (investigation, owner-run vertex captures, literature search, plan set). Replaces nothing; delete this file when R2 merges (harvest first, documentation-stewardship).
---

# Radio stack: handoff for the next session (2026-11-13)

Start here, then open only what you need. **R0 is accepted and R1 (the skeleton) is built, uncommitted; R2 onward is not started.**

## State

- **Branch `radio-stack-plan`**, from `origin/main` (`ddfa9d2b`). **One local commit (the docs) plus the R1 build, uncommitted, not pushed, no PR**; version bumped to 0.84.0 in `VERSION`. (A chained commit, push and PR command was once refused by the auto-mode classifier as an external write; run git steps one at a time and ask.) The docs commit also deletes the owner-run capture scripts (see Recovery).
- **R0 is done** (owner, 2026-11-13: "r0 yes to all"): ADR-0052 and ADR-0053 are `accepted`; ports, `modernc.org/sqlite`, duty-cycled windows accepted; UK-law risk accepted as **open** (capture must not default on before the owner takes advice). Constitution I.16 (plan §6) is still **not** in `CONSTITUTION.md`: it rides in the R2 PR.
- **R1 is built** (plan "Progress"): `stacks/radio/` (`radio.go`, `sensors.go`, tests), SDK `PortVault`/`Vault`/`StorageUse`/`StorageReporter` (`stacks/sdk/manifest.go`, `ports.go`), core wiring (`backend/models/radio_config.go`, `backend/internal/platform/storage/radio_view.go`, `backend/internal/app/stacks.go` `radioModules`/`newRadioHost`, `netaudit.go` `hostedStacks`, `routes.go`, `stacks_about.go`, `core/stack/registry.go`, `backend/go.mod`), frontend (`views/RadioView.vue`, `components/radio/RadioSensors.vue`, composable, service, types, constants, `nav-radio.svg`, nav and route entries; the built `frontend_dist/index.html` is modified: the owner wants it committed with frontend changes).
- **Checks run (all pass):** `go build ./...`, `go test ./...` for `internal/app`, `internal/core/...`, `internal/transport/...`, `internal/archtest`, `internal/platform/storage`, `models`; `GOWORK=off go vet` and `go test -race` in `stacks/radio`; `go test` in `stacks/sdk`; `go run ./tools/check-complexity/` (one new function went over 12; fixed by `hostedStacks`); frontend `npm test` (3043 pass) and `npm run build`; `npm run test:visual` (372 pass, run before the last doc edits; no Radio page visual spec was added). **Not run:** a real run on vertex (the page has only been exercised by tests and a fake `/sys`); `go test ./...` across the whole backend after the last edit (the touched packages were run).
- Local leftovers, safe to ignore or delete with the owner's say: branch `wifi-monitor-findings-ledger` (no commits of mine), merged branches `wifi-monitor-mode-measurement-script`, `wifi-monitor-analyser-fixes`.

## Owner decisions already made (2026-11-13, quote-level)

New **Radio** stack (not a layer of Network) · retention **14 days for neighbours' devices, 30 days for everything else** (daily rollups 30 days by my assumption, adjustable) · small bounded storage that cannot fill the disk · **SQLite yes** · hashed or encrypted at minimum information but matchable after a restart or re-entry (design: keyed tag for others, sealed address for the owner's) · classes **Infrastructure, Personal, Neighbour, Transient** (I added **Unknown**) with dynamically adjustable rules · generic for **any number of adapters**, extensible, pluggable, stack on or off, **does nothing with no adapter**, unused capabilities reported in the UI · a system-wide data management page · **R0: yes to all** (ADRs, new SDK ports, windows, UK-law as an open risk).

## What is left, in order (each item: what it needs from the owner)

1. **Review and commit R1.** Needs the owner: look at the diff (`git diff HEAD`), say whether to commit and push (one command at a time). Try the page on vertex: plug in an adapter, open Radio, press Look again; the built-in card should show "Will not listen", a USB adapter "Can listen".
2. **R2 Capture** (plan §9): the I.16 text into `CONSTITUTION.md`, helper actions `radio_probe` and `radio_listen` (closed catalogue, I.11), the parser (fuzzed), `modernc.org/sqlite` in `stacks/radio/go.mod`, the store with caps and retention, the vault *adapter* in the core (a `stackport.Vault` over `secretcrypto`, subkey per purpose). **Needs a pcap from vertex** to replay (tcpdump on a monitor interface; it holds neighbours' addresses: keep it out of the repo, derive a small synthetic fixture).
3. R3 to R8 as in the plan.
4. [`data-management-page.md`](data-management-page.md) D1 can start independently of Radio; the Radio stack will implement `StorageReporter` when it stores.

## Facts the owner can still supply (they sharpen the classifier and the findings)

On vertex (kernel 7.0.0-42), router `fc:34:97:35:28:f4` on 5 GHz channel 60 (DFS, 5300 MHz), mesh node `24:4b:fe:30:5a:6x`, SSIDs `netsink`, `netsink-2G`, `netsink-Guest`. Unidentified: `f0:24:f9:ee:81:1c` (steady ~1.2 kB/s upload on 2.4 GHz at -44 dBm, 21% retries; a hub or camera?), `14:c1:4e:6b:06:0b` (bursts, 26% retries, probes for `netsink`), `0c:91:60:bd:5f:6e` (about -18 dBm: probably vertex's own card, compare `ip link show wlp2s0`), `72:0a:56:29:c8:8a` (tried to join a neighbour's SKY router five times: a stale saved network?), the hidden AP `d6:be:dc:51:7b:cf` named "Streaming Stick" on the router's channel (whose?), and **the Zigbee hub's address** (toggles were not matched: no typed notes).

## Traps (also in the ledger entry)

tshark here prints subtypes as `0x0008`, booleans `True`/`False`, SSIDs hex, reason codes `0x0003` · `tshark -c N` with `-r` counts packets read, not matched · `radiotap.present.*` exists on every frame, filter `== 1` · `sudo` makes an `out-*` folder root-owned · the Realtek once heard 0 frames on channel 1, so read back the channel after setting it · an 802.11ac adapter decoded **0 Wi-Fi 6 frames** from this router (so per-device bytes on 5 GHz undercount; control and management frames are complete) · the service unit has no netlink, so capability detail needs a privileged one-shot probe · `0e8d:c616` is the Bluetooth half of the built-in card, not an adapter.

## Recovery of the capture scripts (removed from the tree on purpose)

`run.sh` (finds every USB adapter, monitor mode, per-channel survey, a guided event capture with typed notes) and `analyse.sh` (the tshark report) were added in PRs #241 to #243 and are removed by this branch. The newest version, with the run-as-any-user fix, is **commit `826ef161` on `origin/wifi-monitor-analyse-as-any-user` (PR #244, left open and unmerged on purpose; do not delete that branch; it will not merge cleanly once the files are gone)**; the version on main before this branch is `ddfa9d2b`. Restore with `git show 826ef161:docs/audits/2026-11-13-wifi-monitor-n0/analyse.sh` (and `run.sh`). Reasons to restore: a Wi-Fi 6 adapter measurement on 5 GHz, or fixtures for R2. Not yet fixed in any version: the legend's reason codes print as `0x0003` unless the one-line hex fix from the 2026-11-13 session is applied (convert column 10 as column 2 is converted).

## First files to open

[`radio-stack.md`](radio-stack.md) §1 to §3 and §6 · `stacks/sdk/manifest.go` (`Manifest`, ports) and `stacks/sdk/ports.go` · `stacks/agents/` (the smallest working stack: `agents.go`, `watch.go`) and ADR-0051 (how a stack gets a new port: `PortSecretScan` is the template) · `backend/internal/platform/db/db.go` · `backend/internal/platform/secretcrypto/crypto.go` · ADR-0034 and Constitution I.11 (the helper) · `docs/architecture/network-capabilities.md` (read before proposing any network feature). First command: `git log -1 && ./scripts/check-agent-harness.sh`.
