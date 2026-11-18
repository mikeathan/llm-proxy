---
status: partial
date: 2026-11-18
last_reviewed: 2026-11-18
related_specs: [SPEC-006]
constitution_references: [I.16, I.17]
evidence: Session of 2026-11-18. v0.96.0 (PR #262) was merged and deployed; read-only GETs of vertex's radio API one cycle after deploy; the owner asked how to tell whether a device is theirs and for one tap to open a device. Replaces radio-identify-classify-render-handoff-2026-11-17.md (its items 1 and 2 are done; 3 and 4 are below). Delete this file when the branch merges and the vertex checks below are done (harvest first, documentation-stewardship).
---

# Radio: device page in one tap, kind from your name, held classes (handoff, 2026-11-18)

**Branch** `radio-card-kind-fixes`, from `origin/main` at `8ef1197b`; **v0.97.0**; **uncommitted** (the owner approved the branch; commit and
push wait for the owner).

## What vertex showed on v0.96.0 (read-only GETs, one cycle after deploy)
15 of 62 live devices on your Wi-Fi, 9 of 55 `joined_own`, 15 inventory names joined (Mac, M4 MacBook air, iPhone, roku, sinkhole…), 4 radios
`your_ap`; the ASUS mesh is two boxes (`a0be4927`, `7726d4bd`), both yours: one per mesh unit, so correct. No device sent a WPS model. The
owner's card ("Possibly: always here" on their own router `7b1db396`) was opened before the first cycle under rules version 3; the API now
says `your_ap`, 100.

## What is built
- **One tap opens the device page.** A row, a radar dot or a "What changed" entry opens `/radio/device/:id`; the bottom card
  (`RadioDeviceDetail.vue`) and the "showing only" focus mode are removed (`useRadioAround.ts` `open`). Everything the card showed is on the
  page now: first and last heard, the signal word, the "Waiting: …" bar (`DeviceView` gained `listened_cycles`/`min_listened_cycles`), the
  Advanced heard/retries/channels counts, the private-address and network-name notes, and the note under "This is mine".
- **Kind from your name for it** (`kinds.go`, rung after the model name, source `your_name_for_it`, 75): read in memory from the inventory
  for the page only, never for a router's radio, never stored or trained on; matched on whole words (`wordLexicon`), so "Cameron-iPhone" is a
  phone, not a camera. ADR-0058 decision 4 amended.
- **A device you marked yours is never a neighbour's or a passer-by** (`decideYours`, `learn.go`): the likelier of always here and comes
  and goes, at that share of the two, no hysteresis.
- **A held class says so**: `DeviceIdent.Pending` (`ident.go`); the conclusion reads "Changing to: …" instead of the old class at 0%.
- **No "too new to tell"**: the trend is about this run's in-memory trail (reset by every restart), so it is hidden while unknown.

**Checks run (all passed):** `go build ./...`, `go test ./...`, `go run ./tools/check-complexity/` in `backend`; `GOWORK=off go vet` and
`GOWORK=off go test -race ./...` in `stacks/radio`; `go test ./...` in `stacks/sdk`; `npm run build`;
`./scripts/check-agent-harness.sh`. Final run after the last change: `npm test` 3,171, `npm run test:visual` 388 of 388, backend and radio
`-race` green. **Not run:** anything on vertex.

## Left, in order
1. **Owner:** review, then approve commit and push (one pull request). Note: `RadioDeviceDetail.vue` was removed with `git rm`, so its
   deletion is already staged.
2. **Deploy and look on vertex**: the Mac rows should read kind computer ("from your name for it"), "samsung - tv" a TV, the Nest Mini a
   speaker; the Roku you marked (`70f44fb0`) should be always here or comes and goes, not a neighbour's; tapping a row opens the page.
3. **Answer the "Help it learn" card** for a few days; then check the learned line ("gives back N of M") on Around you.
4. **The two Roku records** (`33fb3ee8` on your Wi-Fi, `70f44fb0` not): **not merged.** Both were marked "roku" by the owner. A guess, not
   checked: the Roku's Wi-Fi Direct radio for its remote. `decideYours` stops the second one being sorted a neighbour's. Merging them needs
   evidence they are one box (same address block, the Direct network's name).
5. **Next architecture (needs an ADR and a Constitution item):** Bluetooth and a second-room sensor through ESPHome Bluetooth proxies on the
   LAN, as nodeherder does (research addendum §6 in `docs/audits/2026-11-17-radio-identification-research.md`).

## Traps
- `stacks/radio` is not in `go.work`: run its tests with `GOWORK=off`.
- The inventory name feeds the page's kind only; the classifier's `fixed_kind` feature still uses `KindOf` (stored row). Feeding it the name
  would make training depend on a name that is never stored.
- `lexicon` (model names) still matches substrings; only the name rung uses whole words. The word lists are shared, so a new word affects both.
- The night bit uses the server's local time (`isNight`, `features.go`).

**First file to open:** `stacks/radio/kinds.go`, then `frontend/src/composables/radio/useRadioAround.ts`.
