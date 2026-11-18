---
status: partial
date: 2026-11-18
last_reviewed: 2026-11-18
related_specs: [SPEC-003]
constitution_references: [I.16, I.17]
evidence: vertex read-only GETs on 2026-11-18 (`/admin/api/radio/air/live`, `/air/track/0733101c`, `/air/track/4ded1379`, `/air/device/2a29ce75`); ledger entry of 2026-11-18 "Radio v0.100.0" in docs/learnings/network-audit.md. Replaces radio-device-identity-and-my-devices-handoff-2026-11-18.md (its items harvested into radio-stack.md → Remaining work). Delete this file when v0.100.0 is merged and checked on vertex.
---

# Radio: kind filter, phone-width rows, 0 dBm readings, Find my device (handoff, 2026-11-18)

**Branch** `radio-filters-mobile-and-radar-movement`, from `origin/main` at `e4c80f84` (v0.99.0, #265); **v0.100.0**; committed and pushed
as a pull request on the owner's say-so (2026-11-18); merge order: alone. Nothing on vertex was changed.

## What v0.100.0 holds
- **0 dBm is no reading** (`stacks/radio/frame.go`, `maxReceivedDBm`): the parser kept the strongest antenna field, and the sensor (USB
  `rtw88_8822bu`) gave fields of 0 dBm (taken as a chain with no reading; an assumption), so 0 won. Seven devices sat on the radar's "room"
  ring at 0 dBm, and 25 more had a stored max of 0 dBm, which made them "strong" for the classifier. A stored max above −1 dBm now gives no
  signal evidence (`FeaturesOf`), so those devices are not drawn as usual dots (`baseline.has`) and may re-sort until the bad days age out of
  the window (7 days); the hidden-network finding ignores such beacons (`hiddenNear`).
- **This computer's own card never moves** (`fixedToSensor`, `live.go`): no trend, "moving now" or unusual shift.
- **Kind drop-down** beside the chips (`RadioKindPick.vue`, `matchesKind`/`kindCounts` in `utils/radio/around.ts`): combined with the chip
  ("Yours" + "Phone or tablet"); "Not known yet" for no kind; router = `isRouter`. The Routers chip is gone (one way per job).
- **Phone-width device row** (`RadioDeviceRow.vue`): the reading drops under the name below `sm`; a spec with the owner's exact row.
- **My devices**: the hour squares are gone (`RadioPresenceStrip.vue` deleted). The last-day sentence names the stretches as clock times
  ("Heard in 6 of the last 24 hours: 07:00–09:00 and since 18:00").
- **Find my device**: each name opens the device page; each row says whose it probably is, its router box, rising signal and "moving now";
  this computer's own card is never offered; one line says whether the strongest leads the next by 10 dB or more.
- **How to read this**: the motion line now explains quiet phones and why a small move may not show.
- **The helper thins control frames before its 4,000-frame cap** (ADR-0060, **proposed, amends Constitution I.16**: merging is the owner's
  approval of that security change). Channel 60 filled the cap in about 4 s of its 20 s; the helper now skips CTS/ACK and keeps 16 control
  frames per sender a window (`wire.RadioThinner`), and the Listening page says "N repeats left out". **The helper binary must be
  reinstalled on vertex** for this to take effect: `sudo scripts/install-helper.sh radio --install --bin ./llm-proxy-helper` (or
  `setup.sh --helper`). Not measured on vertex yet.

**Checks run (all passed):** `GOWORK=off go vet ./...`, `GOWORK=off go test ./...` and `GOWORK=off go test -race .` in `stacks/radio`;
`npm test`, `npm run build`, `npx playwright test e2e/radio.visual.spec.ts` (18) in `frontend`; see the final report for the rest.
Also `go test ./...`, `go vet` and `go run ./tools/check-complexity/` in `backend`, `FuzzRadioThinner` (two 20 s runs pass; one earlier run
exited 1 with no crasher saved, read as the fuzz harness, not the code), `npm run test:visual` (390) and `./scripts/check-agent-harness.sh`.
**Not run:** v0.100.0 on vertex.

## Left, in order
1. **Owner:** review and merge the PR (merging approves the I.16 amendment), deploy, reinstall the radio helper.
2. After deploy, read-only on vertex: in `GET /admin/api/radio/air/live` no usual dot is stronger than about −20 dBm apart from the host card
   `374a25da`, which has no `trend`/`moving`; expect some classes to change while the polluted days age out.
3. After the helper is reinstalled: the Listening page's channel-60 windows run about 20 s (no "Busy channel" badge) with "N repeats left
   out"; compare how often `4ded1379` is heard (`GET /admin/api/radio/air/track/4ded1379`) with the 5–30 minutes of 2026-11-18.
4. **Owner:** move a phone while using it (screen on) and watch its dot over two readings; a phone with its screen off is heard only every
   5–30 minutes (track of `4ded1379` on 2026-11-18).
5. The carried items in `radio-stack.md` → Remaining work.

## Traps
- `stacks/radio` is not in `go.work`: run its tests with `GOWORK=off`.
- The radar's angle is a hash (direction is not measured), so a dot only moves nearer or farther.

**First file to open:** `stacks/radio/frame.go` (`takeField`), then `frontend/src/utils/radio/around.ts`.
