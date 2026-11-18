---
status: partial
date: 2026-11-18
last_reviewed: 2026-11-18
related_specs: [SPEC-006]
constitution_references: [I.16, IV.6]
evidence: ADR-0061; docs/PLANS/cross-cutting/network-radio-joined-by-tag.md (chosen design); the tests named below; no run on vertex.
---

# Network and Radio joined by tag (handoff)

**Branch `network-radio-join`**, made from `main` at `05cb1edb` (v0.101.0). Committed and pushed on 2026-11-18 with the pull request open for review; v0.102.0 is in `VERSION`.
The design and the reasons are in `network-radio-joined-by-tag.md` (Chosen design) and ADR-0061.

## What is built
- SDK (`stacks/sdk/ports.go`, `manifest.go`): `LANDevice.Presence`, `LANPresence`; `AirFact`, `AirReport`, `AirReporter`,
  `MaxAirFacts` and the word lists `AirBands`, `AirRings`, `AirKinds`; `Manifest.ReportsAir` (needs `PortLAN`); `StackFinding.Device`.
- Core: `core/devicejoin` (the join, `NewDeviceNote`), `stackport.AskedPurposes` and presence on the `lan` port, `netaudit.PresenceOf`
  (online within 30 min, unknown when the check is off or has not looked for an hour), `netaudit.StackFindingSources`, the
  `radio_join_foreign_network` wording and guide, per-id name checks, the alert's `AirNote`, `DeviceView.Air` and `InventoryView.Air`.
- App: `internal/app/devicejoin.go` (the one wiring place), `stackHost.Air` and the tag mapping in `stackHost.Offered`,
  `radioHostDeps` (newRadioHost had six parameters), the devices route `.WithAir`, the nightly report pulling Network and Radio.
- Radio: `stacks/radio/airreport.go` (`Air`, `Findings`), presence on `LANInfo` and `PresenceDevice.Network`, the manifest's
  `ReportsAir` and `Finds`.
- UI: `utils/netaudit/air.ts` and the phrase on `DeviceRow.vue`'s "First seen" line; `rowState` in `utils/radio/myDevices.ts` and
  "Home · on the network" on `RadioMyDeviceRow.vue`. No element was added to a closed row or a list.

## Checks run (2026-11-18, on the branch as it stands)
- `cd backend && go build ./... && go test ./...`: pass (no failing package).
- `cd backend && go run ./tools/check-complexity/`: OK, 49 known-debt functions, no new violation (it also scans `stacks/`).
- `cd stacks/sdk && go test ./...`: pass. `cd stacks/radio && GOWORK=off go test ./...`: pass.
- `cd frontend && npm test`: 326 files, 3197 tests pass. Run together with the visual suite and the Go tests, 10 unrelated component
  tests (Settings, Activity, AppSidebar, BottomTabs, HostStats) failed under the load and all passed when `npm test` ran alone.
- `cd frontend && npm run build` (lint included): pass; `frontend_dist/index.html` rebuilt. `npm run test:visual`: 394 pass, with new states
  in `netaudit-devices.visual.spec.ts` (`air`, Simple and Advanced at phone width) and `radio.visual.spec.ts` (My devices, `network: online`).
- `./scripts/check-agent-harness.sh`: PASS.
- **Not run:** anything on vertex. **Not covered by a test:** the real Radio stack hosted end to end in `internal/app` (hosting it needs a fake
  adapter and helper). The round trip is tested with the real vault, `lan` port, purposes memory, join and stack host and a stack that tags
  as Radio does (`internal/app/devicejoin_test.go`); Radio's side is tested in `stacks/radio/airreport_test.go`.

## What is left, in order
1. **Owner: read the I.16 amendment** (Constitution I.16, "The air report") and ADR-0061, in particular that the new-device alert now carries a
   fixed phrase about where Radio heard the device. Merging the pull request is the approval; ADR-0061 then becomes accepted.
2. **Owner: review and merge the pull request, then deploy.**
3. **Check on vertex after deploy** (read-only GETs): `/admin/api/netaudit/devices` has `air.state: "listening"` during a watch and an
   `air` object on the phone's row; `/admin/api/radio/devices/mine/presence` has `network: "online"` for `4ded1379` between hearings; the
   Radio page shows "Home · on the network" for it instead of "Not heard lately".
4. **Parked 3a** (deauthentication burst and devices dropping off): needs the presence check to keep when a device left (it keeps only
   `LastSeen`); Radio counts deauthentication senders per day. Needs a plan of its own and the owner's go-ahead.
5. **Parked 3d** (one look-alike check): Radio's `radio_rogue_twin` raises no alert and is about an access point, so switching
   `wifi_lookalikes` off while Radio listens would lose the alert. The owner decides whether the two lists of confirmed access points
   become one (the core's `WifiNetworks` and Radio's own access points) and whether Radio alerts.

6. **A known rough edge:** a My devices row that reads "Home · on the network" opens the device page, which answers "not in the picture: it
   has not been heard in the last few days" when the air has not heard it lately. The device page could say the network sees it; not built.

## Traps found
- `stacks/radio` is not in `go.work`: run its tests with `GOWORK=off` from the folder (the baseline `go test ./...` from the repo root fails
  with "outside module roots").
- `json.Unmarshal` into a reused struct keeps a field the new answer omits (`omitempty`): a test of "the field is gone" must decode into a
  fresh value.
- `NetauditDevicesHandlers.respondChange` called `List(w, nil)`; a list that reads the request's context needs the request passed through.
- Radio's `presenceOf` already names the owner-hours function in `airdevices.go`; the LAN filter is `lanPresence`.

## First file to open
`backend/internal/core/devicejoin/join.go`, then `internal/app/devicejoin.go`.
