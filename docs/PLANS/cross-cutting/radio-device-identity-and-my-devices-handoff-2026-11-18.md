---
status: partial
date: 2026-11-18
last_reviewed: 2026-11-18
related_specs: [SPEC-006]
constitution_references: [I.15, I.16, I.17]
evidence: v0.98.0 (PR #264, commit 3835fabf) merged, deployed and checked on vertex read-only on 2026-11-18 12:39 UTC (focus windows counting, router "via <box>" on 14 rows). v0.99.0 built on branch radio-device-identity-and-my-devices; ledger entries of 2026-11-18 in docs/learnings/network-audit.md. Replaces radio-card-kind-fixes-handoff-2026-11-18.md (harvested). Delete this file when v0.99.0 is merged and checked on vertex.
---

# Radio: this computer's own Wi-Fi, a calmer My devices tab, and what is next (handoff, 2026-11-18)

**Branch** `radio-device-identity-and-my-devices`, from `origin/main` at `3835fabf`; **v0.99.0**; **uncommitted** (committing, pushing and the
pull request need the owner's OK). Nothing on vertex was changed. Merge order: alone, nothing waits on it.

## Start here (new session)
1. `git status` on the branch: the whole change is in the working tree.
2. Once merged and deployed, check vertex read-only with `$LLM_PROXY_TOKEN` against `http://vertex.local:4001`:
   `GET /admin/api/radio/air/device/374a25da` has `ident.this_computer: "wlp2s0"`, kind `computer`, kind source `this_computer`.
   The ids are the current epoch's: if `374a25da` is gone, find the row with `this_computer` in `GET /admin/api/radio/air/live`.
3. Open Radio → My devices on the phone and the desktop and ask the owner whether it reads well; then the items below.

## What v0.99.0 holds
- **This computer's own Wi-Fi card is named as such** (`stacks/radio/host.go`): `HostRadios` reads the wireless interfaces' addresses from
  `/sys` (the same records the Sensors tab reads), the LAN index tags them like the inventory's (`LANInfo.Self`, `lan.go`), and the device
  reads "This computer's Wi-Fi (wlp2s0)", kind computer at 100% from `this_computer`, and likely yours. Nothing new is stored or shown: the
  address is only tagged in memory. The Hui Zhou Gaoshengda device the owner could not place is very likely this (ledger 2026-11-18); step 2 of Start here confirms it.
- **Module makers name no kind**: Gaoshengda and GSD Viet Nam left the maker → TV rule (`kinds.go`); `shortBrand` drops a leading city.
- **My devices redesigned** (`RadioMyDevices.vue`, `RadioMyDeviceRow.vue`, `utils/radio/myDevices.ts`): home first, one row per device that
  opens its page, last heard, "via <box>", "moving now", the last day in one sentence; a warning for names shared by two or more devices;
  the hour squares, class, signal and Remove (with a confirm) in Advanced; adding by address folded away. The distance-by-hour chart is gone.
- **Where in the house** (`utils/radio/where.ts`, `whereOf`): one phrase from the two listeners there are. "Near this computer" when the radar
  puts it in the sensor's room; "Near <box>" when a mesh box hears it at −60 dBm or stronger (the sorting's StrongDBm); otherwise "In the
  house, joined to <box>" or "Far from this computer, joined to <box>", never a spot between listeners; without the router the radar's ring,
  with its outer ring said as "Far from this computer" (your own devices two floors down sit on it). Shown first on each My devices row and as
  the lead line of the device page's Where it is panel when the router lists the device. Frontend only: no new data, permission or storage.
  Previewed on vertex's live data on 2026-11-18: 6 devices "Near this computer", 6 "Near Asus router", the second Mac "In the house, joined to
  ZenWiFi_XD4-28F0".
- **Your Wi-Fi networks grouped by name** (`RadioNetworks.vue`, `utils/radio/networks.ts`): unconfirmed access points are asked about at the
  top; the per-access-point list, Not mine, guest and Remove name are Advanced.

**Checks run (all passed):** read-only on vertex, `GET /admin/api/radio/air/device/a4c371c6` (a marked device not heard now) answers 200 with `mine: true`, so every My devices row's link works; `GOWORK=off go vet ./...`, `GOWORK=off go test ./...` and `GOWORK=off go test -race .` in `stacks/radio`;
`go build ./...` and `go run ./tools/check-complexity/` in `backend`; `npm test` (3,193), `npm run build`, `npm run test:visual` (388; the radio spec rerun after the last device-page change) in
`frontend`; `./scripts/check-agent-harness.sh`. **Not run:** v0.99.0 on vertex; `go test ./...` in `backend` (no backend code changed).

## Left, in order
1. **Owner:** rename the four "Phone or tablet" devices and "Apple" (My devices now says which; each row opens the page with Rename).
2. **Owner:** after a day of the current watch, compare the phone's heard share with 29%: `GET /admin/api/radio/air/track/4ded1379`.
3. **Measure the helper's cost of focus windows** (carried from v0.98.0): about 8 short helper processes per window, roughly twice the
   windows. Compare the service's and the helper's CPU time over an hour with and without focus (`ps -o pid,time,rss,comm`). Needs a shell
   on vertex (this session's SSH key is refused there).
4. **Answer the Help it learn card** for a few days, then read "gives back N of M" on Around you (carried).
5. **The two Roku records** (`33fb3ee8`, `70f44fb0`): not merged; merging needs evidence they are one box (carried).
6. **Next phase, owner to choose** (docs/audits/2026-11-18-radio-continuous-monitoring.md §3, §4a): rooms and movement need a second kind of
   listener; the audit recommends ESPHome Bluetooth proxies (an ADR and a Constitution item). The "where in the house" line from the box and
   the radar's zone is built (above); ask the owner whether its words match where the devices really are.

## Traps
- `stacks/radio` is not in `go.work`: run its tests with `GOWORK=off`.
- The host's interface addresses are read once, when the recorder is made: a connection that randomises its address is caught after a restart.
- The live picture's ids are the first 8 hex digits of the owner list's 64-digit ids (`myDeviceRows`).
- Remove deletes the owner's row, its events and its hour history (`Store.DeleteOwner`); the dialog says so.

**First file to open:** `stacks/radio/host.go`, then `frontend/src/components/radio/RadioMyDevices.vue`.
