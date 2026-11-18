---
status: partial
date: 2026-11-18
last_reviewed: 2026-11-18
related_specs: [SPEC-006]
constitution_references: [I.16, IV.6]
evidence: ADR-0061 (accepted); docs/PLANS/cross-cutting/network-radio-joined-by-tag.md; vertex read-only GET of /admin/api/netaudit/devices 2026-11-18 18:37Z; the tests named below.
---

# Network and Radio joined by tag (handoff)

## Branch state
- **v0.102.0 (the join) is merged:** PR #268, merge commit `2b5c5282`, deployed on vertex and seen working there (the s24's row read "heard
  over the air 3 minutes ago, near this computer, 5 GHz, on your Wi-Fi, Radio thinks: Phone or tablet").
- **v0.103.0 is on branch `network-air-radio-link`**, made from `origin/main` at `2b5c5282`, committed and pushed with its pull request
  open. Merge it on its own; nothing else waits on it.

## What v0.103.0 built
- The Network row says **"heard over the air in Radio's last listen"** for a device heard in Radio's latest cycle, instead of "3 minutes
  ago" (which read as if the device had left while it showed Online). On vertex every heard device had `heard` = `air.last_cycle`, because
  Radio stamps a device with the end of the cycle that heard it (`LastAt`, `stacks/radio/live.go`). An earlier cycle keeps the time.
  Code: `frontend/src/utils/netaudit/air.ts` (`airWords`, `heard >= last_cycle`), `DeviceRow.vue` (prop `airLastCycle`).
- Those "heard …" words **link to the device's Radio page** (`/radio/device/<id>`). The core makes the id from the fact's own tag (first 4
  bytes hex, Radio's `shortID`): `devicejoin.pageOf` → `models.DeviceAir.Page` (`page` in the API). No SDK or Constitution change: the stack
  answers nothing new, and `TestAirFact_CarriesOnlyFixedWords` still holds. ADR-0061 marked accepted, with a consequence line for the link.

## Checks run on v0.103.0 (2026-11-18)
- `cd backend && go build ./... && go test ./...`: pass, no failing package. `go run ./tools/check-complexity/`: OK, no new violations.
- `cd stacks/radio && GOWORK=off go test ./...`: pass (new `TestAirFactTagOpensTheDevicePage`). `cd stacks/sdk && go test ./...`: pass.
- `cd frontend && npm test`: 326 files, 3200 tests pass. `npm run build` (lint): pass, `frontend_dist/index.html` rebuilt.
- `npx playwright test e2e/netaudit-devices.visual.spec.ts`: 26 pass (the air states check the link text and href). **The full
  `npm run test:visual` was not run.**
- `./scripts/check-agent-harness.sh`: PASS.
- **Not run on vertex.**

## What is left, in order
1. **Owner: merge the v0.103.0 pull request and deploy.** Then on vertex open Network → Devices → a heard device: the phrase says "in
   Radio's last listen" and tapping it opens that device's Radio page.
2. **Device page rough edge:** a Radio My devices row reading "Home · on the network" opens a device page that may still say "not heard in the
   last few days". The page could say the network sees it now (the presence is already on `LANInfo`). Not built; small, Radio-only.
3. **Devices matched but not heard:** on vertex the iPhone, Lindiwe's iPhone, the Samsung TV, espressif and nodeherder had an air fact with no
   `heard` (so "not heard over the air lately"). Unverified why: iPhones rotate their private Wi-Fi address, others may be quiet or off the
   listened channels. Look at `/admin/api/radio/air/live` and the scan plan's channels before changing anything.
4. **Parked 3a** (deauthentication burst and devices dropping off): needs the presence check to keep when a device left (it keeps only
   `LastSeen`); a plan of its own and the owner's go-ahead.
5. **Parked 3d** (one look-alike check): the owner decides whether the core's `WifiNetworks` and Radio's own access points become one list
   and whether `radio_rogue_twin` alerts.

## Traps found
- `stacks/radio` is not in `go.work`: test it with `GOWORK=off` from its folder.
- The app is served under `/admin`: a RouterLink's href is `/admin/radio/device/<id>` in the visual suite, `/radio/device/<id>` in a
  memory-history unit test.
- `src/utils` may not export types (lint rule): put interfaces in `src/types/`.
- `json.Unmarshal` into a reused struct keeps a field the new answer omits: decode into a fresh value to test a field is gone.
- The DeviceRow unit tests print happy-dom `AbortError` traces at teardown when a router is mounted; they still pass.

## First file to open
`backend/internal/core/devicejoin/join.go`, then `frontend/src/utils/netaudit/air.ts`.
