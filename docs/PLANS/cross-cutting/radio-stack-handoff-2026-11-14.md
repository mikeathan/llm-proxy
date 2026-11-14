---
status: partial
date: 2026-11-14
last_reviewed: 2026-11-14
related_specs: [SPEC-006]
constitution_references: [I.5, I.8, I.11, I.16, I.17, II.3, III.6, IV.6]
evidence: Sessions of 2026-11-13 and 2026-11-14 (R0 to R3 part 1 merged; the first live listening run on vertex; R3 part 2 built). Replaces the earlier version of this file; delete it when R8 (or the last phase the owner keeps) merges (harvest first, documentation-stewardship).
---

# Radio stack: handoff for the next session (2026-11-14): fit the classifier, then R4 onward

Start here, then open only what you need. **R0 to R3 part 1 are merged (#251, v0.86.0) and the service is listening on vertex** (the owner's first run heard 9,571 frames in the first cycle, channel 60 cut at the helper's 4,000-frame cap). **R3 part 2 is built on branch `radio-r3-sensors-redo-probe-classes` (v0.87.0): nothing committed, nothing pushed, not run on vertex** (the commit was denied by the harness because the owner had not asked for one; commit and push need the owner's word).

## Built since the last handoff (v0.87.0, branch `radio-r3-sensors-redo-probe-classes`, all uncommitted)

Checks run: `stacks/radio` `GOWORK=off go test -race ./...` (pass), `go vet`, a fuzz run of `FuzzParseProbe` (8 s, clean); `backend` `go build ./...`, tests of `internal/app`, `internal/platform/storage`, `internal/core`, `models` (pass), `go run ./tools/check-complexity/` (no new debt; three new functions were refactored under 12); `frontend` `npm test` (3063 pass), `npm run build` (pass), `npm run test:visual` (372 pass before the Radio spec; the new `e2e/radio.visual.spec.ts` passes at desktop and mobile, 390 px viewed). **Not run:** `./scripts/check-agent-harness.sh` is run at the end of this file's edit (see the final message), nothing on vertex, `go test ./...` for the whole backend module.

- **The Sensors page, redone.** The owner called it horrible: it said "Listen to nearby Wi-Fi: not built yet" beside a card that was listening, and gave the built-in card six capability rows. `Describe` (`stacks/radio/sensors.go`) now derives each capability from state (listening now, the probe, eligibility). Each sensor leads with one role (Listening now, Can listen, Will not be used, No driver yet) and a line of what the probe read; capabilities are under Details; a radio the stack will not use gets two rows. New uses: `ready`, `unsupported` (the hardware cannot: the 8822bu has no Wi-Fi 6). `Unused` counts only what could still be built.
- **The Radio page is four tabs:** Listening (the run, with the cycle position and the frame-cap note, and **Around you**), Sensors, My devices (with **Your networks**), Modules. `RADIO_TABS = ['sensors','devices','modules']`; the page without a tab is Listening.
- **The probe (R3 part 1's missing item).** `probe.go` parses `iw list` (bounded, fixed word lists, fuzzed). `radio_probe` heads `Plan.Requests`, so one session approves it and the run reads it once at the start; the last read is kept in the `docs` table and mapped to an interface through `/sys/class/net/<if>/phy80211/name`.
- **Storage.** `migrate.go` (`PRAGMA user_version`, tested against a v0.86.0 file: vertex has a live `radio.db`). New: `daily` and `air_days` (day-level only, ADR-0053; a *cycle* is one window on every channel and a device counts once per cycle), `docs`, `own_aps`, `class_log`; `devices` gains `random`, `joined_own`, `class_ver`. Rotation, prune and eviction handle the new tables.
- **Classes v1 (R3 part 2).** `classes.go`: `Rules` as a versioned document (`classifier_v1`, **provisional and not measured**, the page says so), rule order as a table, `ClassifyAll` after every finished cycle, every change a `class_log` row with the rules version and a fixed reason word, the owner's word (`class_ver = 0`) never overwritten. Routes: `GET /admin/api/radio/air` (counts by class only: no tag, no address).
- **Own networks.** `own_networks` in `models.RadioConfig` (core) and `Settings`; a beacon with one of the names *proposes* a sealed access point, the owner confirms (`PUT/GET /admin/api/radio/networks`, `POST .../{id}/confirm`, `DELETE .../{id}`); an association request to a confirmed one sets `joined_own`, the strongest class evidence. Rotation re-tags the owner's access points.
- **Owner devices** show their class and who decided it; the add form takes an optional class (the owner's word).

## What is left, in order (what each needs from the owner)

1. **Owner decision: commit, push, PR.** The branch is uncommitted. `git status` then the owner says go (the push must check `gh pr view --json state` first if a PR exists).
2. **Owner check on vertex, one paste at a time** (deploy per "Running things on vertex"): Start listening for 1 hour, see the Sensors page name the adapter as listening and show its bands and standards (the probe ran), see Around you fill as cycles complete (it stays "not enough evidence" until 12 cycles), add the phone under My devices with and without a kind, add `netsink`, `netsink-2G`, `netsink-Guest` under Your networks and confirm the router and mesh node, see devices that join them sorted. Then **Forget the neighbours** and see Around you empty and the confirmed access points still confirmed.
3. **Fit `classifier_v1` to measured data (needs a day or more of vertex's own listening).** The numbers in `DefaultRules` are cautious guesses. After a day: read `GET /admin/api/radio/air`, compare counts with what the owner knows, and publish `classifier_v2` (a new document, `PutDoc` plus `SetMetaInt("classifier_current")`; there is no editing route yet: R7). The owner's labels are the ground truth.
4. **Owner hourly rollups** (the owner's own per-day rows exist; no page reads hours yet). Build when R5's Timeline needs them.
5. **R4 findings and digest, R5 screens, R6 position, R7 learning loop, R8 Bluetooth** as in [`radio-stack.md`](radio-stack.md) §9, phase by phase. Open design points found on the way: a removed ("Not mine") access point is proposed again on its next beacon until its name is removed; there is no UI for the scan plan (channels, window, cycle) because `models.RadioConfig` only holds `enabled` and `own_networks`.
6. **Ask first (unchanged):** the `PrivilegedResult` report field, any SDK port, any change to I.16/I.17 or ADR-0052/0053/0054 wording, a force-push, a remote branch delete, a merge. Also independent: the data page ([`data-management-page.md`](data-management-page.md) D1).

## What exists (read these, do not rebuild)

| Piece | Where | State |
|---|---|---|
| Frame parser (`Parse`, radiotap + 802.11 to typed facts), fuzzed, real-frame fixtures, replay test | `stacks/radio/frame.go`, `testdata/`, `replay_test.go` | done |
| Window bytes decoder (the helper's `wire.DecodeRadio`; the stack has its own strict copy, `stacks/radio/window.go`, with a contract test) | `backend/internal/platform/privhelper/wire/radio.go` | done |
| SQLite store with 20 MB cap, 14/30-day retention, eviction, `ForgetOthers`, `StorageUse` | `stacks/radio/store.go` | done |
| `Recorder` (`Record`, `BeginCycle`/`EndCycle`, `AddOwnerDevice`, own networks): the only code that sees an address; keyed tags for others, sealed address for the owner's | `stacks/radio/recorder.go` | done; the run feeds it |
| Vault adapter (HKDF subkey per purpose, HMAC tags, AES-GCM) | `core/stackport/vault.go` | done; reads the master key from disk on each call |
| Stack (no goroutine, no write; discovery from `/sys`; capability list; route `GET /admin/api/radio/sensors`) | `stacks/radio/radio.go`, `sensors.go` | done |
| Helper actions `radio_probe` and `radio_listen` (`{iface, channel, seconds}`, one channel a call, USB only, never the connection, restore on every path, `Result.Report` carries the channel read-back) | `backend/internal/platform/privhelper/{catalog/radio.go,helper/,capture/radio.go}`, `cmd/llm-proxy-helper/main.go`, `docs/services/llm-proxy-helper-radio*` | done, verified |
| Bounded sessions: `platform/privhelper/session`, `port.ForStack`, `helperRoutes.StackPrivileged("radio")` (a per-stack port that serves a request with no grant only while an open session of that stack approved exactly that request), routes `GET/POST/DELETE /admin/api/helper/sessions` (POST is step-up) | `backend/internal/app/privhelper.go` (`sessionStacks`), `transport/http/helpersessions.go` | done; the Listening card and the scheduler use it |
| Radio page: Listening, Sensors, My devices, Modules | `frontend/src/views/RadioView.vue` | tabs rebuilt 2026-11-14; R5 replaces the data views |

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
