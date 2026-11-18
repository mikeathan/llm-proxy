---
status: partial
date: 2026-11-18
last_reviewed: 2026-11-18
related_specs: [SPEC-006]
constitution_references: [I.15, I.16, I.17]
evidence: v0.97.0 (PR #263) merged and deployed by the owner on 2026-11-18 (vertex reports v0.97.0, commit 85bc643d). Read-only GETs of vertex's radio API found two page bugs, fixed in v0.98.0 on branch radio-page-chances-and-ask-name, which also carries the focus windows and the router's list of joined devices (ADR-0059, I.15 amended) the owner asked for; the router was measured with the owner's own SSH. Analysis and measurements: docs/audits/2026-11-18-radio-continuous-monitoring.md (§4a radar movement, §5 the bugs, §7 focus windows, §8 the router read). Delete this file when v0.98.0 is merged and checked on vertex (harvest first, documentation-stewardship).
---

# Radio: v0.98.0 page fixes, focus windows and the router's list of joined devices (handoff, 2026-11-18)

**Branch** `radio-page-chances-and-ask-name`, from `origin/main` at `85bc643d`; **v0.98.0**; **uncommitted** (the owner approved the
branch; commit and push wait for the owner). The branch tracks `origin/main`: push it with an explicit name
(`git push -u origin radio-page-chances-and-ask-name`), never a bare `git push`. Nothing on vertex was changed; today's 7-day watch
(started 08:00:40 UTC) runs until 2026-11-23 21:00 UTC, and a deploy ends it (a restart ends every session).

## What v0.98.0 holds
- **The device page's chances agree with the class of a device you marked** (`vector.houses`, `ownedVector`, `learn.go`; `explain`,
  `devicepage.go`). The owner's word is set only when sorting and explaining, never on a training vector.
- **The offered name never carries the guessed kind** (`defaultLabel`, `frontend/src/utils/radio/identity.ts`).
- **A device you marked can be renamed** ("Rename" on its page; `PUT /admin/api/radio/air/mine/{id}`, `renameMine`, `Store.RenameOwner`).
  A new admin route beyond the literal fix: a marked device cannot be marked again, so there was no other way to correct the Macs' names.
- **Focus windows between sweeps** (`stacks/radio/focus.go`; audit §7): the idle time after each full sweep is spent on the channels your
  devices were heard on, weighted by their frames. The sweep is unchanged, so unknown devices are found as before and nothing stored
  changes (a test compares the stored rows and track). They move the dot, set "moving now", have their own frame budget, size the session
  through `windows_per_hour` (144 for the default plan), and turn off for the run on a refusal without ending it. The owner said on
  2026-11-18 that I.16 covers them as written (ADR-0059).
- **The mesh router's list of joined devices during a watch** (ADR-0059; **Constitution I.15 amended**, the owner approved on 2026-11-18;
  audit §8): `cat /tmp/clientlist.json` over one kept-open SSH connection, at most once every 30 s, closed after 2 minutes idle and on
  every setup change, reset and shutdown; parsed strictly (fuzzed), addresses on this network dropped; given to the Radio stack only, through
  its own port `meshclients`, as tags. Each dot says "via <box>" (box named from the inventory, the gateway as the main router), the page
  says the box, band, its signal and when it was read, and "moving now" also comes from the router's readings. Never stored, never "yours".
  Vertex's router setup is already done (`192.168.50.1:23`, key saved, host key pinned: read-only GET 2026-11-18).

**Checks run (all passed):** `go build ./...`, `go test ./...`, `go run ./tools/check-complexity/` in `backend` (re-run after the focus
work: see the final report); `GOWORK=off go vet ./...` and `GOWORK=off go test -race ./...` in `stacks/radio`; `go test ./...` in
`stacks/sdk`; `npm test` (3,179), `npm run build`, `npm run test:visual`; `./scripts/check-agent-harness.sh`; the benchmarks
`GOWORK=off go test -run '^$' -bench 'FocusIngest|SweepIngest' -benchtime 20x .` in `stacks/radio`. **Not run:** anything on vertex.

## Left, in order
1. **Owner:** review, then approve commit and push (one pull request; commit `backend/internal/transport/http/frontend_dist/index.html`).
2. **Owner: read the Constitution change on its own** (`git diff HEAD -- CONSTITUTION.md`: one new I.15 bullet and two sentences changed)
   and ADR-0059; merging the pull request is the approval of the security review. Note: the next watch's passphrase dialog asks for about
   twice the uses (24,204 for 7 days, against 12,108).
3. **Deploy, start a new watch, and look on vertex:** your phone's row says "via ZenWiFi_XD4-28F0" (or the main router) and its page the
   router's signal; the listening line counts "extra windows on your devices' channels"; your phone's
   row says "moving now" when carried; `70f44fb0`'s chances show only always here / comes and goes; rename the Macs (`991ba3f9`,
   `bd105ebf`). After a day, read the phone's track (`GET /admin/api/radio/air/track/4ded1379`) and compare its heard share with 29%.
   **Measure the helper's cost**, which the benchmark does not cover: each window spawns about 8 short processes in the helper (link
   down/up, monitor/managed mode, channel, info) and moves up to about 2 MB, and focus roughly doubles the windows. Compare the cumulative
   CPU time of the service and helper processes (`ps -o pid,time,rss,comm` or `/proc/<pid>/stat`) over an hour with and without focus.
4. **Router read: done.** Measured (audit §8.1) and built (§8.2). The commands below are kept as the recipe to re-check another router.
5. **Answer the Help it learn card** for a few days, then check the learned line ("gives back N of M") on Around you.
6. **The two Roku records** (`33fb3ee8`, `70f44fb0`): not merged; merging needs evidence they are one box.

## Commands for the router read (item 4)
Run from the Mac, read-only. **None is verified on this router**: they are the usual ASUS candidates. Each address keeps its first three
bytes (the maker) and its last byte, so entries still match between commands but not with a device.

```sh
# The owner's router login (port 23, its own key), as a function: r '<command on the router>'
r() { ssh -o IdentitiesOnly=yes -i ~/.ssh/router_key_v2 -p 23 admin@192.168.50.1 "$@"; }
M='s/(([0-9A-Fa-f]{2}:){3})([0-9A-Fa-f]{2}:){2}([0-9A-Fa-f]{2})/\1xx:xx:\4/g'
# 1. Firmware, mode and radios
r 'nvram get firmver; nvram get buildno; nvram get extendno; nvram get sw_mode; nvram get wl_ifnames; nvram get cfg_master'
# 2. Clients on each radio, with signal, idle time and state, and how long the listing takes
r 'for i in $(nvram get wl_ifnames); do echo "== $i $(date +%s)"; wl -i $i assoclist; echo "listed $(date +%s)"; for m in $(wl -i $i assoclist | cut -d" " -f2); do echo "-- $m"; wl -i $i sta_info $m | grep -iE "rssi|idle|state|in network"; done; done' 2>&1 | sed -E "$M"
# 3. AiMesh: does the main router know the satellite's clients?
r 'ls -la /tmp/*client* /jffs/.sys/*client* /tmp/*.json 2>/dev/null; nvram get cfg_clientlist | head -c 400' 2>&1 | sed -E "$M"
r 'for f in /tmp/clientlist.json /jffs/.sys/cfg_mnt/clientlist.json; do [ -f "$f" ] && { echo "== $f"; head -c 1500 "$f"; echo; }; done' 2>&1 | sed -E "$M"
# 4. One connection, several sessions? (the "keep one connection open" design)
ssh -o IdentitiesOnly=yes -i ~/.ssh/router_key_v2 -p 23 -o ControlMaster=yes -o ControlPath=/tmp/asus-%r@%h-%p -o ControlPersist=60 -fN admin@192.168.50.1
ssh -o ControlPath=/tmp/asus-%r@%h-%p -p 23 admin@192.168.50.1 'echo second session ok'
ssh -o ControlPath=/tmp/asus-%r@%h-%p -p 23 -O exit admin@192.168.50.1
# 5. The satellite: repeat 1 and 2 with r2() { ssh -o IdentitiesOnly=yes -i ~/.ssh/router_key_v2 -p 23 admin@<satellite address> "$@"; }
```

What the output decides: (2) whether per-client signal and idle time are readable, and what one read costs the router; (3) whether one read
of the main router covers clients on both mesh boxes or the satellite must be read too; (4) whether one open connection can serve every
read (if the second command fails, the fallback is a login per read, at most once a minute).

## Traps
- `stacks/radio` is not in `go.work`: run its tests with `GOWORK=off`.
- The picture already changes window by window (`LiveSnapshot`); an earlier version of this file said once a cycle, which was wrong.
- Sweep frames and focus frames have separate budgets (`framesPerHour`, `focusFramesPerHour`): only sweep frames can end a watch.
- A session's uses are one pool: focus windows run only when the start says `"focus": true` (the page sends it because it sized the
  session from `windows_per_hour`); a start without it only sweeps (`TestAStartThatDidNotAskForFocusOnlySweeps`).
- Smoothing the dot with a median of readings was tried and dropped (audit §7): it delays a real move.

**First file to open:** `stacks/radio/mesh.go` and `stacks/radio/focus.go`, then `backend/internal/core/netaudit/routerconn/mesh.go`.
