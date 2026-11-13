---
status: partial
date: 2026-11-13
last_reviewed: 2026-11-13
related_specs: [SPEC-006]
constitution_references: [I.5, I.8, I.11, II.3, III.6, IV.6]
evidence: Session of 2026-11-13 (investigation, owner-run vertex captures, literature search, plan set). Replaces nothing; delete this file when R1 merges (harvest first, documentation-stewardship).
---

# Radio stack: handoff for the next session (2026-11-13)

Start here, then open only what you need. **Nothing is built: this is investigation, measurement and design. Docs only.**

## State

- **Branch `radio-stack-plan`**, from `origin/main` (`ddfa9d2b`, v0.83.x), **one local commit, not pushed, no PR**. (A chained commit, push and PR command was once refused by the auto-mode classifier as an external write; run git steps one at a time and ask.) Merge order: this docs PR has no code dependency.
- It carries: the plan [`radio-stack.md`](radio-stack.md), [`data-management-page.md`](data-management-page.md), ADR-0052 and ADR-0053 (both `proposed`), the literature digest [`../../audits/2026-11-13-wifi-sensing-research.md`](../../audits/2026-11-13-wifi-sensing-research.md), a ledger entry in [`../../learnings/network-audit.md`](../../learnings/network-audit.md) (the 2026-11-13 Wi-Fi entry holds every measured number and six tool traps), index rows and the Network hub links. **It also deletes the owner-run capture scripts** (see Recovery).
- Checks run: `./scripts/check-agent-harness.sh` PASS. Not run: any Go or frontend build (no code changed).
- Local leftovers, safe to ignore or delete with the owner's say: branch `wifi-monitor-findings-ledger` (no commits of mine), merged branches `wifi-monitor-mode-measurement-script`, `wifi-monitor-analyser-fixes`.

## Owner decisions already made (2026-11-13, quote-level)

New **Radio** stack (not a layer of Network) · retention **14 days for neighbours' devices, 30 days for everything else** (daily rollups 30 days by my assumption, adjustable) · small bounded storage that cannot fill the disk · **SQLite yes** (`modernc.org/sqlite` in the stack; it is the ask-first item, answered yes) · hashed or encrypted at minimum information but matchable after a restart or re-entry (design: keyed tag for others, sealed address for the owner's) ·
classes **Infrastructure, Personal, Neighbour, Transient** (I added **Unknown**) with dynamically adjustable rules · generic for **any number of adapters**, extensible, pluggable, stack on or off, **does nothing with no adapter**, unused capabilities reported in the UI · a system-wide data management page.

## What is left, in order (each item: what it needs from the owner)

1. **R0.** Owner reads ADR-0052, ADR-0053 and plan §6 (the draft Constitution **I.16 Radio Sensor** text) and accepts or edits them. **Needs:** (a) accept the ADRs; (b) **windows or a long-lived sensor** for capture (the plan assumes duty-cycled windows, 20 s a channel every 5 min; a long-lived unit needs a bigger amendment); (c) the **UK-law risk**: accept it as open or take advice (a persistent tag is a pseudonymous identifier; search found no guidance on passive capture of neighbours' frames; not legal advice). CONSTITUTION.md is **not** edited yet: its text rides in the implementing PR.
2. **R1 skeleton** (plan §9): `stacks/radio` module, manifest, two new SDK ports (`vault`: `Tag`, `Seal`, `Open`; a storage report), discovery from `/sys/class/net`, capability records, Sensors page, idle behaviour. Ask-first: new SDK ports, `modernc.org/sqlite` in the stack's `go.mod`. TDD; the check is that with no adapter it starts no goroutine and writes nothing.
3. R2 to R8 as in the plan. R2 needs **a pcap from vertex** to replay (capture one with tcpdump on a monitor interface; it holds neighbours' addresses: keep it out of the repo, derive a small synthetic fixture).
4. [`data-management-page.md`](data-management-page.md) D1 can start independently of Radio.

## Facts the owner can still supply (they sharpen the classifier and the findings)

On vertex (kernel 7.0.0-42), router `fc:34:97:35:28:f4` on 5 GHz channel 60 (DFS, 5300 MHz), mesh node `24:4b:fe:30:5a:6x`, SSIDs `netsink`, `netsink-2G`, `netsink-Guest`. Unidentified: `f0:24:f9:ee:81:1c` (steady ~1.2 kB/s upload on 2.4 GHz at -44 dBm, 21% retries; a hub or camera?), `14:c1:4e:6b:06:0b` (bursts, 26% retries, probes for `netsink`), `0c:91:60:bd:5f:6e` (about -18 dBm: probably vertex's own card, compare `ip link show wlp2s0`), `72:0a:56:29:c8:8a` (tried to join a neighbour's SKY router five times: a stale saved network?), the hidden AP `d6:be:dc:51:7b:cf` named "Streaming Stick" on the router's channel (whose?), and **the Zigbee hub's address** (toggles were not matched: no typed notes).

## Traps (also in the ledger entry)

tshark here prints subtypes as `0x0008`, booleans `True`/`False`, SSIDs hex, reason codes `0x0003` · `tshark -c N` with `-r` counts packets read, not matched · `radiotap.present.*` exists on every frame, filter `== 1` · `sudo` makes an `out-*` folder root-owned · the Realtek once heard 0 frames on channel 1, so read back the channel after setting it · an 802.11ac adapter decoded **0 Wi-Fi 6 frames** from this router (so per-device bytes on 5 GHz undercount; control and management frames are complete) · the service unit has no netlink, so capability detail needs a privileged one-shot probe · `0e8d:c616` is the Bluetooth half of the built-in card, not an adapter.

## Recovery of the capture scripts (removed from the tree on purpose)

`run.sh` (finds every USB adapter, monitor mode, per-channel survey, a guided event capture with typed notes) and `analyse.sh` (the tshark report) were added in PRs #241 to #243 and are removed by this branch. The newest version, with the run-as-any-user fix, is **commit `826ef161` on `origin/wifi-monitor-analyse-as-any-user` (PR #244, left open and unmerged on purpose; do not delete that branch; it will not merge cleanly once the files are gone)**; the version on main before this branch is `ddfa9d2b`. Restore with `git show 826ef161:docs/audits/2026-11-13-wifi-monitor-n0/analyse.sh` (and `run.sh`). Reasons to restore: a Wi-Fi 6 adapter measurement on 5 GHz, or fixtures for R2. Not yet fixed in any version: the legend's reason codes print as `0x0003` unless the one-line hex fix from the 2026-11-13 session is applied (convert column 10 as column 2 is converted).

## First files to open

[`radio-stack.md`](radio-stack.md) §1 to §3 and §6 · `stacks/sdk/manifest.go` (`Manifest`, ports) and `stacks/sdk/ports.go` · `stacks/agents/` (the smallest working stack: `agents.go`, `watch.go`) and ADR-0051 (how a stack gets a new port: `PortSecretScan` is the template) · `backend/internal/platform/db/db.go` · `backend/internal/platform/secretcrypto/crypto.go` · ADR-0034 and Constitution I.11 (the helper) · `docs/architecture/network-capabilities.md` (read before proposing any network feature). First command: `git log -1 && ./scripts/check-agent-harness.sh`.
