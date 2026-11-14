---
status: partial
date: 2026-11-13
last_reviewed: 2026-11-13
related_specs: [SPEC-006]
constitution_references: [I.5, I.8, I.11, I.16, I.17, II.3, III.6, IV.6]
evidence: Sessions of 2026-11-13 (R0 to R2 built, then the first run of the helper on vertex with the owner). Replaces the earlier version of this file; delete it when R8 (or the last phase the owner keeps) merges (harvest first, documentation-stewardship).
---

# Radio stack: handoff for the next session (2026-11-13): R3 part 2 and R4 onward

Start here, then open only what you need. **R0 to R2 are merged and the capture path is verified on vertex** (v0.85.2). **R3 part 1 is built on branch `radio-r3-unit-pipeline-sessions` (v0.86.0, see "Built since" below; not merged, not run on vertex).** The next session continues with **R3 part 2 (tracker and classes) and R4 to R8** from [`radio-stack.md`](radio-stack.md) §9, phase by phase, each ending with its check, stopping only for the ask-first items and the owner-run vertex checks below.

## State (verified)

- **Merged to `main`:** #245 (R1 skeleton), #246 (I.16, frame parser, vault adapter, SQLite store), #247 (radio helper class, bounded sessions, I.17, ADR-0054), #248 (helper status reads every class's gates, labels), #249 (the jailed `ip`/`iw` steps may make a Unix socket, v0.85.2). **PR #250 (docs only, this file and the ledger) may still be open: merge order is #250 only.**
- **Verified on vertex (2026-11-13, with the owner):** `radio_probe` returns `iw list`; a 10 s `radio_listen` on channel 60 with the USB adapter `wlxf0090da058a3` runs end to end (helper unit ran 14 s, no warning, adapter back to `managed`, built-in `wlp2s0` untouched, SSH never dropped). The browser got `parse_error: no reader for the output of "radio_listen"`, which is correct: nothing in the service reads window bytes yet. **Not verified:** that the frames carry frequency 5300 (nothing reads them).
- **`setup.sh --helper` now installs every helper class** (net and radio); `--helper=net` or `--helper=radio` narrows it; the choice is remembered. Installing switches nothing on: each action stays off until the owner turns it on with the passphrase (Network, Settings, Privileged helper card).
- **Capture must not default on** (UK-law question is an open risk, ADR-0053). Everything you build that starts a listen must be behind a bounded session (I.17) the owner starts, never at boot or on a timer of its own.

## Built since (R3 part 1, branch `radio-r3-unit-pipeline-sessions`, v0.86.0; checks run: `go build/test` for backend and `stacks/radio -race`, complexity, `npm test` 3056 pass, `npm run build`; **not run:** `npm run test:visual`, anything on vertex)

- `stacks/radio/window.go`: strict window reader + `Ingest` (window to `Parse` to `Recorder.Record`) + plain-data `WindowSummary`; contract test in `backend/internal/app/radio_window_contract_test.go`; the core's manual `radio_listen` now answers counts (`summarizeRadioWindow`), so the `parse_error` is gone.
- `stacks/radio/listen.go`: the run (one goroutine owned by the stack, asks with no grant, stops on denial, time, frame cap, repeated failure, stop, switch-off, service stop), `Plan` (defaults and caps, at most 200 windows an hour), routes under `/admin/api/radio/listening` and `.../plan`; the stack now requires `PortPrivileged` (late-bound in the core: `latePrivileged`).
- `stacks/radio/devices.go`, `recorder.go`, `store.go`: owner devices (add, list, remove, never shown an address), **forget neighbours = tag-epoch rotation** (`Recorder.ForgetNeighbours`, `Store.Rotate`), tests for the three R3 checks (re-enter after restart, old tags stop matching, no raw address in the file).
- `core/stackport/keycache.go`: the vault reads the key once, dropped on factory reset.
- Frontend: `RadioListening.vue` (adapter, hours, start, listening until, stop, why it ended with its fix, last windows), `RadioOwnerDevices.vue` (My devices, Forget the neighbours), `useRadioListening` (plan, then session, then start; undone if a step fails; stop ends the radio sessions).
- **Not built from the earlier list:** the Sensors page's "not read yet" from `radio_probe` (parse `iw list` in the service, bounded, I.11; a place to keep it); hourly rollups; the owner's own SSIDs; the `PrivilegedResult` report field (ask first; the frames' radiotap frequency stands in, `WindowSummary.ChannelMatched`).
- **Owner check on vertex, one paste at a time:** deploy; Radio page, switch the stack on in Modules; Start listening for 1 hour (passphrase prompt); read the last windows (expect `on the channel asked` on channel 60); stop; add the phone's address under My devices and see it matched next session; Forget the neighbours.

## What exists (read these, do not rebuild)

| Piece | Where | State |
|---|---|---|
| Frame parser (`Parse`, radiotap + 802.11 to typed facts), fuzzed, real-frame fixtures, replay test | `stacks/radio/frame.go`, `testdata/`, `replay_test.go` | done |
| Window bytes decoder (`wire.DecodeRadio` returns records and a trailer: stop reason, frames, kernel seen/dropped, elapsed) | `backend/internal/platform/privhelper/wire/radio.go` | done; the service does not call it yet |
| SQLite store with 20 MB cap, 14/30-day retention, eviction, `ForgetOthers`, `StorageUse` | `stacks/radio/store.go` | done |
| `Recorder` (`Record(ctx, Frame, at)`, `AddOwnerDevice`): the only code that sees an address; keyed tags for others, sealed address for the owner's | `stacks/radio/recorder.go` | done, nothing calls `Record` |
| Vault adapter (HKDF subkey per purpose, HMAC tags, AES-GCM) | `core/stackport/vault.go` | done; reads the master key from disk on each call |
| Stack (no goroutine, no write; discovery from `/sys`; capability list; route `GET /admin/api/radio/sensors`) | `stacks/radio/radio.go`, `sensors.go` | done |
| Helper actions `radio_probe` and `radio_listen` (`{iface, channel, seconds}`, one channel a call, USB only, never the connection, restore on every path, `Result.Report` carries the channel read-back) | `backend/internal/platform/privhelper/{catalog/radio.go,helper/,capture/radio.go}`, `cmd/llm-proxy-helper/main.go`, `docs/services/llm-proxy-helper-radio*` | done, verified |
| Bounded sessions: `platform/privhelper/session`, `port.ForStack`, `helperRoutes.StackPrivileged("radio")` (a per-stack port that serves a request with no grant only while an open session of that stack approved exactly that request), routes `GET/POST/DELETE /admin/api/helper/sessions` (POST is step-up) | `backend/internal/app/privhelper.go` (`sessionStacks`), `transport/http/helpersessions.go` | done; no UI, no scheduler |
| Radio page: Sensors and Modules | `frontend` (Stacks group) | done; layout is temporary (owner's word) |

## What is left, in order (each item: what it needs from the owner)

**Phase R3, part 1: remaining**
1. The Sensors page's "not read yet": parse `radio_probe` (`iw list`) in the stack with strict bounds (text is untrusted, I.11) and keep the last read; map each `phy` to an interface through `/sys/class/net/<if>/phy80211/name`. Needs a design for where the privileged call comes from (a session, or a step-up route with `Action`).
2. **Ask first:** `stacksdk.PrivilegedResult` has no field for `Result.Report` (the channel read-back). Until the owner answers, `ChannelMatched` from the radiotap frequency is the check.
3. The owner's own networks (SSIDs `netsink`, `netsink-2G`, `netsink-Guest`) as evidence for classing a device; hourly rollups if a page needs them.
4. The vertex check above.

**Phase R3, part 2: tracker and classes v1** (plan §4): rules as versioned data (`classifier_v<N>`) in the database, thresholds **from vertex's measured captures** (the owner's two pcaps are on their Mac, see below), owner labels as ground truth, classes Infrastructure, Personal, Neighbour, Transient, Unknown. Nothing is reclassified silently.

**R4 findings and digest** (plan §7): finding ids by the ADR-0036 rule (`radio_rogue_twin`, `radio_deauth_burst`, `radio_open_or_weak_ap`, `radio_pmf_off`, `radio_hidden_ap_near`, `radio_unknown_strong_device`, `radio_join_foreign_network`, `radio_sensor_blind`), the typed bounded read-only digest tool (precedent: `network_findings`, ADR-0049; every device-chosen string inside the untrusted-data fence, I.8; other people's devices only as counts by class), an automation template and an Analyse button. *Check:* no device-chosen string outside the fence; byte-identical wording tests (ADR-0038 rule 1). Adding a tool: `engineering-practices` checklist.

**R5 screens** (plan §8): Air overview, Devices, Neighbourhood, Timeline and patterns, Airwaves, Sensors, Modules, Settings; Simple and Advanced, mobile first, no hidden knowledge. *Check:* Vitest, `npm run build`, `npm run test:visual`, viewed at 390 px. Commit `frontend_dist/index.html` with every frontend change.

**R6 position** (plan §5): distance bands from signal (always with uncertainty), then multi-sensor. *Needs the owner:* a walked, labelled device on vertex and the stated uncertainty containing the truth. vertex's own card (about -18 dBm) is a known anchor.

**R7 learning loop** (plan §4): proposed groups, versioned classifier, label-driven thresholds; every change logged in the Timeline with its reason and confidence.

**R8 Bluetooth driver (optional):** `hci0` is on vertex; proves the plug-in point. Skip unless the owner asks.

Also independent and waiting: the data page ([`data-management-page.md`](data-management-page.md) D1).

## Ask first, and stop for

- Any SDK port or `PrivilegedResult` field (public interface), a new third-party dependency, any change to `CONSTITUTION.md`, a change to I.16/I.17 or ADR-0052/0053/0054 wording.
- Any git mutation beyond what the owner already approved: **the owner has said "stop ping-ponging" and approved branch, commit, push and PR for this work**, but not a force-push, a remote branch delete (the auto-mode classifier refused one) or a merge.
- **Vertex checks are the owner's.** You cannot SSH there and the admin token is refused on step-up routes. Give the owner one paste at a time and read what comes back; do not hand them a bisect of five things.

## Running things on vertex (the recipe that worked)

Deploy: `sudo ./setup.sh --install --build --unattended --helper` after pulling. Then, in the **signed-in admin browser tab's console** (a step-up route needs the passphrase and a token is refused): confirm first with `fetch('/admin/api/auth/reauth', {method:'POST', headers:{'Content-Type':'application/json'}, body: JSON.stringify({passphrase: prompt('Passphrase')})})`, then call the route in the same snippet (the confirmation is short). Look at the helper with `iw dev` and `journalctl -u 'llm-proxy-helper-radio@*' -n 40 --no-pager`. The adapter is `wlxf0090da058a3`; **never** `wlp2s0` (it carries the owner's SSH).

## Traps found in this session

- **The jail refuses `socket(AF_UNIX)` to every child**, and `iw dev <name> ...` resolves the name with `if_nametoindex`, which opens one, so a jailed `iw dev X set type` failed with `No such file or directory (-2)` while `iw list` and `ip` worked. Fixed for the fixed `ip`/`iw` steps only (`Helper.step`); the `radio-listen` program keeps the refusal. Any new jailed program that resolves an interface name will hit this. To tell a unit property from the helper's own jail, run the same steps unjailed and under `systemd-run -p ...` with the unit's properties.
- **A squash-merged branch is gone from your history's point of view:** a PR merged while you were still pushing leaves the later commits out of `main`. Check `gh pr view <n> --json state` before pushing more to a branch, and cut a fresh branch from `origin/main` (cherry-pick) when it merged.
- The helper's status read only the net unit's gates drop-in, so radio actions showed "not installed" (fixed: `readAllHelperActions`). Each class has its own `llm-proxy-helper-<class>@.service.d/10-gates.conf`.
- macOS `sed -i` needs a suffix and a failed `sed` in an `&&` chain silently skips what follows: edit with Python or the Edit tool. Run `stacks/*` tests with `GOWORK=off` from the folder. `stacks/radio` is not in `go.work`.
- The scheduler test cannot advance a fake clock during `Run`, so the wait between cycles is the nominal `cycle - channels x window` (the real run does include setup time; a cycle is a little longer than its setting).
- A stack route that declares no `Action` and no step-up is safe to start a run because the core serves the helper request only under an open session; do not add a grant path to the run.
- A chained `git switch && cherry-pick && push && push --delete` was refused by the auto-mode classifier; run git steps one at a time.
- `radio_listen`'s raw bytes are never returned to the browser (`keepsToItself`); the answer is the parsed summary. No service parser exists yet, hence the `parse_error` above.

## Facts the owner can still supply (they sharpen the classifier and the findings)

On vertex (kernel 7.0.0-42), router `fc:34:97:35:28:f4` on 5 GHz channel 60 (DFS, 5300 MHz), mesh node `24:4b:fe:30:5a:6x`, SSIDs `netsink`, `netsink-2G`, `netsink-Guest`. Unidentified: `f0:24:f9:ee:81:1c` (steady ~1.2 kB/s upload on 2.4 GHz at -44 dBm, 21% retries; a hub or camera?), `14:c1:4e:6b:06:0b` (bursts, 26% retries, probes for `netsink`), `0c:91:60:bd:5f:6e` (about -18 dBm: probably vertex's own card, compare `ip link show wlp2s0`), `72:0a:56:29:c8:8a` (tried to join a neighbour's SKY router five times: a stale saved network?), the hidden AP `d6:be:dc:51:7b:cf` named "Streaming Stick" on the router's channel (whose?), and **the Zigbee hub's address** (the owner switched Wi-Fi networks on a phone a few times during the capture, which explains the extra authentication and deauthentication frames; no times were noted, so no frame is matched to an event).

## Traps (also in the ledger entry)

tshark here prints subtypes as `0x0008`, booleans `True`/`False`, SSIDs hex, reason codes `0x0003` · `tshark -c N` with `-r` counts packets read, not matched · `radiotap.present.*` exists on every frame, filter `== 1` · `sudo` makes an `out-*` folder root-owned · the Realtek once heard 0 frames on channel 1, so read back the channel after setting it · an 802.11ac adapter decoded **0 Wi-Fi 6 frames** from this router (so per-device bytes on 5 GHz undercount; control and management frames are complete) · the service unit has no netlink, so capability detail needs a privileged one-shot probe · `0e8d:c616` is the Bluetooth half of the built-in card, not an adapter.

## Recovery of the capture scripts (removed from the tree on purpose)

`run.sh` (finds every USB adapter, monitor mode, per-channel survey, a guided event capture with typed notes) and `analyse.sh` (the tshark report) were added in PRs #241 to #243 and are removed by this branch. The newest version, with the run-as-any-user fix, is **commit `826ef161` on `origin/wifi-monitor-analyse-as-any-user` (PR #244, left open and unmerged on purpose; do not delete that branch; it will not merge cleanly once the files are gone)**; the version on main before this branch is `ddfa9d2b`. Restore with `git show 826ef161:docs/audits/2026-11-13-wifi-monitor-n0/analyse.sh` (and `run.sh`). Reasons to restore: a Wi-Fi 6 adapter measurement on 5 GHz, or fixtures for R2. Not yet fixed in any version: the legend's reason codes print as `0x0003` unless the one-line hex fix from the 2026-11-13 session is applied (convert column 10 as column 2 is converted).

## First files to open

`radio-stack.md` §2 to §9 · `stacks/radio/radio.go`, `recorder.go`, `store.go`, `frame.go` · `backend/internal/platform/privhelper/wire/radio.go` (`DecodeRadio`) · `backend/internal/app/privhelper.go` (`sessionStacks`, `StackPrivileged`) · `backend/internal/platform/privhelper/session/` and `port/` · `stacks/sdk/manifest.go` and `ports.go` · `stacks/agents/` (the smallest working stack) and ADR-0051 (how a stack gets a new port) · ADR-0034, ADR-0053, ADR-0054, Constitution I.11, I.16, I.17 · `docs/architecture/network-capabilities.md` (read before proposing any network feature) · `docs/learnings/network-audit.md` (the 2026-11-13 Radio entries). Skills: `task-planning` (start here: this is multi-phase), `tdd-guide`, `engineering-practices`, `documentation-stewardship`, `pr-commit-message`. First command: `git log -1 && ./scripts/check-agent-harness.sh`.
