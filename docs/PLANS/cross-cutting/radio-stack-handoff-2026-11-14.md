---
status: partial
date: 2026-11-14
last_reviewed: 2026-11-14
related_specs: [SPEC-006]
constitution_references: [I.5, I.8, I.11, I.16, I.17, II.3, III.6, IV.6]
evidence: Sessions of 2026-11-13 and 2026-11-14 (R0 to R3 merged; the first live listening run on vertex; the owner's paste of the Listening tab). Replaces the earlier version of this file; delete it when R8 (or the last phase the owner keeps) merges (harvest first, documentation-stewardship).
---

# Radio stack: handoff for the next session (2026-11-14): useful screens first, then findings, then the rest

Start here, then open only what you need. **R0 to R3 are merged: #251 (v0.86.0) and #252 (v0.87.0, R3 part 2). The service is listening on vertex.** Nothing of R3 part 2 has been judged on vertex beyond the owner's one paste (below). The owner's verdict on that paste: **the Radio UI is temporary: it lacks useful, interactive data.** The next work is therefore R5's useful core first, then R4, as listed under "What is left". All of it goes on **one fresh branch from `origin/main`** (suggested name `radio-useful-screens-and-findings`); the owner approved branch, commit, push and PR for radio work, not a force-push, a remote branch delete or a merge.

**What the owner saw (2026-11-14, Listening tab, cycle 2 of the first hour):** 12,025 frames heard, 2 of 6 channels in cycle 2; the "Last windows" list reads as run-together text (`ch 604000 frames cut short at the 4,000-frame limit`: channel and count have no gap and there is no band or time); **Around you: 31 devices, every one "Not enough evidence yet"** (12 cycles are needed before sorting, so the section is a count of five rows with nothing to open); the log said "22 devices first sorted" while the card said 31. No row can be opened.

## Built in v0.87.0 (merged as #252)

Checks run before merge: `stacks/radio` `GOWORK=off go test -race ./...` (pass), `go vet`, a fuzz run of `FuzzParseProbe` (8 s, clean); `backend` `go build ./...`, tests of `internal/app`, `internal/platform/storage`, `internal/core`, `models` (pass), `go run ./tools/check-complexity/` (no new debt; three new functions were refactored under 12); `frontend` `npm test` (3063 pass), `npm run build` (pass), `npm run test:visual` (372 pass before the Radio spec; the new `e2e/radio.visual.spec.ts` passes at desktop and mobile, 390 px viewed). **Not run:** `./scripts/check-agent-harness.sh` is run at the end of this file's edit (see the final message), nothing on vertex, `go test ./...` for the whole backend module.

- **The Sensors page, redone.** The owner called it horrible: it said "Listen to nearby Wi-Fi: not built yet" beside a card that was listening, and gave the built-in card six capability rows. `Describe` (`stacks/radio/sensors.go`) now derives each capability from state (listening now, the probe, eligibility). Each sensor leads with one role (Listening now, Can listen, Will not be used, No driver yet) and a line of what the probe read; capabilities are under Details; a radio the stack will not use gets two rows. New uses: `ready`, `unsupported` (the hardware cannot: the 8822bu has no Wi-Fi 6). `Unused` counts only what could still be built.
- **The Radio page is four tabs:** Listening (the run, with the cycle position and the frame-cap note, and **Around you**), Sensors, My devices (with **Your networks**), Modules. `RADIO_TABS = ['sensors','devices','modules']`; the page without a tab is Listening.
- **The probe (R3 part 1's missing item).** `probe.go` parses `iw list` (bounded, fixed word lists, fuzzed). `radio_probe` heads `Plan.Requests`, so one session approves it and the run reads it once at the start; the last read is kept in the `docs` table and mapped to an interface through `/sys/class/net/<if>/phy80211/name`.
- **Storage.** `migrate.go` (`PRAGMA user_version`, tested against a v0.86.0 file: vertex has a live `radio.db`). New: `daily` and `air_days` (day-level only, ADR-0053; a *cycle* is one window on every channel and a device counts once per cycle), `docs`, `own_aps`, `class_log`; `devices` gains `random`, `joined_own`, `class_ver`. Rotation, prune and eviction handle the new tables.
- **Classes v1 (R3 part 2).** `classes.go`: `Rules` as a versioned document (`classifier_v1`, **provisional and not measured**, the page says so), rule order as a table, `ClassifyAll` after every finished cycle, every change a `class_log` row with the rules version and a fixed reason word, the owner's word (`class_ver = 0`) never overwritten. Routes: `GET /admin/api/radio/air` (counts by class only: no tag, no address).
- **Own networks.** `own_networks` in `models.RadioConfig` (core) and `Settings`; a beacon with one of the names *proposes* a sealed access point, the owner confirms (`PUT/GET /admin/api/radio/networks`, `POST .../{id}/confirm`, `DELETE .../{id}`); an association request to a confirmed one sets `joined_own`, the strongest class evidence. Rotation re-tags the owner's access points.
- **Owner devices** show their class and who decided it; the add form takes an optional class (the owner's word).

## What is left, in order (all on one branch; each numbered item is a commit with its own test)

**A. Useful screens (R5 core): the owner's complaint. No new capture; read what is already stored.** Test-first; viewed at 390 px; `npm test`, `npm run build`, `npm run test:visual`.
1. **Readable "Last windows".** One row per window: time, channel and band (2.4/5/6 GHz), frames, an "own" count, and the cut-short flag as a plain badge ("stopped at the 4,000-frame limit: a busy channel, counts are a floor"). Fix the run-together text (no gap between channel and count in the current markup) and the order. Data comes from the existing listen state in `stacks/radio/listen.go`; check what a window record holds before adding a field.
2. **Around you becomes openable.** Each class row opens a list of its devices: a tag short-id (never an address; ADR-0053), signal band (strong/medium/weak, from stored signal), first and last heard, cycles seen of cycles run, retry share, channels. A filter by class and a sort. Needs a new read route beside `GET /admin/api/radio/air` (counts by class only today): `GET /admin/api/radio/air/devices?class=` returning bounded rows, **no address and no device-chosen string except a name already fenced**; check `store.go` for which columns exist before a migration (the `devices` table has `random`, `joined_own`, `class_ver`).
3. **Say what "not enough evidence" is waiting for.** A progress line per device or a bar for the cycle count (needs 12 of N). Resolve the 22-versus-31 mismatch between the card and the class log (find which counts a device once per cycle).
4. **My devices shows presence.** Last heard, signal band over the day, joined-own evidence, so the owner sees the phone arrive and leave. The hourly rollup in item 6 feeds this.
5. **Neighbourhood and Airwaves tabs** (§8): counts by class near the house, channel use, retries and airtime by sender class, the router's channel. Counts only, no per-neighbour rows.
6. **Owner hourly rollups** (the owner's own per-day rows exist; no page reads hours). Add the table and migration (`migrate.go`, version bump, tested against a v0.87.0 file: vertex has a live `radio.db`), then a presence-by-hour strip for the owner's devices (§8 Timeline and patterns).
7. **Simple and Advanced** (owner rule): Simple shows who is here and what changed; Advanced shows the tables above. One path per job; problems shown with the fix; plain failures.

**B. R4 findings and digest (§7).** The finding ids: `radio_rogue_twin`, `radio_deauth_burst`, `radio_open_or_weak_ap`, `radio_pmf_off`, `radio_hidden_ap_near`, `radio_unknown_strong_device`, `radio_join_foreign_network`, `radio_sensor_blind`; the stack offers ids, the core words them (ADR-0036). A typed, bounded, read-only digest tool for the agent (precedent: `network_findings`, ADR-0049), each claim with a code-set tier (seen, inferred, estimated), a "could not see" list, other people's devices as counts by class only, every device-chosen string inside the untrusted-data fence (I.8). An automation template and an **Analyse** button that saves a report. *Check:* digest has no device-chosen string outside the fence; byte-identical wording tests (ADR-0038 rule 1). Existing evidence to seed fixtures: the owner's unidentified devices listed under "Facts the owner can still supply".

**C. Classifier fit (R7 start; needs a day or more of vertex's own listening).** Read `GET /admin/api/radio/air`, compare with what the owner knows, publish `classifier_v2` (`PutDoc` plus `SetMetaInt("classifier_current")`; there is no editing route yet), the owner's labels as ground truth. The numbers in `DefaultRules` are guesses until then; the page says so.

**D. R6 position** (distance bands from one sensor, then several; stated uncertainty; measured with a walked device on vertex) and **R7 the rest** (proposed groups, versioned classifier, label-driven thresholds, a label changes a class and the change shows in the Timeline with its reason). **R8 Bluetooth** is optional.

**E. Open design points found on the way:** a removed ("Not mine") access point is proposed again on its next beacon until its name is removed; there is no UI for the scan plan (channels, window, cycle) because `models.RadioConfig` holds only `enabled` and `own_networks`; the 4,000-frame cap truncates busy channels (channel 60), so counts there are a floor (say so wherever a count is shown).

**F. Owner's vertex check, one paste at a time** (deploy per "Running things on vertex"): start listening for 1 hour; Sensors names the adapter as listening and shows its bands and standards (the probe ran); Around you fills as cycles complete; add the phone under My devices with and without a kind; add `netsink`, `netsink-2G`, `netsink-Guest` under Your networks and confirm the router and mesh node; see devices that join them sorted; **Forget the neighbours** empties Around you and the confirmed access points stay confirmed.

**G. Ask first (unchanged):** the `PrivilegedResult` report field, any SDK port, any change to I.16/I.17 or ADR-0052/0053/0054 wording, a force-push, a remote branch delete, a merge. Also independent: the data page ([`data-management-page.md`](data-management-page.md) D1).

**Done in the meantime (not radio):** PR #253 (v0.87.1) makes the tokens/sec reader ignore the new llama.cpp prompt-processing speed lines. The new llama.cpp release's `no API key / CORS` and `Flash Attention ... CPU` warnings are not fatal (`docs/learnings/platform.md`).

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
