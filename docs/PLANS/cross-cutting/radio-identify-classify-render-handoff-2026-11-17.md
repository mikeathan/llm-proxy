---
status: partial
date: 2026-11-17
last_reviewed: 2026-11-17
related_specs: [SPEC-006]
constitution_references: [I.16, I.17]
evidence: Session of 2026-11-17. The owner called the Radio page useless (a row read "SKY UK 2a6e3b0d"); research in docs/audits/2026-11-17-radio-identification-research.md; decisions in ADR-0058. Replaces radio-ui-redesign-handoff-2026-11-15.md (its open items are below or done). Delete this file when the branch merges and the vertex checks below are done (harvest first, documentation-stewardship).
---

# Radio: identify, classify and render (handoff, 2026-11-17)

**Branch** `radio-identify-classify-render`, from `origin/main` at `f397d0d6`; **v0.96.0**; **committed and pushed** as one pull request (the owner approved commit and push on 2026-11-18).

## What is built
- **Capture** (`stacks/radio`): the network a device uses, from data, RTS and block-ack frames and join requests (`links.go`); "on your Wi-Fi" now fires (vertex had 0 of 56 marked); guest networks are no evidence; one box per router (`boxKey`); WPS make, model and type, never the WPS device name; vendor hint; request signature (`identity.go`); one night bit and a movement count a day (`features.go`); migration 8.
- **Classification**: rules version 3, a naive Bayes seeded from version 2 and retrained each cycle from the owner's answers, hard evidence and capped self-training; recency-weighted share; signals read against the owner's access points; hysteresis (2 cycles, 0.15 margin); Unknown under 0.55 (`learn.go`, `decisions.go`). Kind (what it is) from an evidence ladder with label spreading by maker (`kinds.go`).
- **LAN join**: SDK port `lan` (`stacks/sdk/ports.go`), core adapter `backend/internal/core/stackport/lan.go`, wired in `newRadioHost`; inventory names and the gateway's neighbouring addresses as tags.
- **API**: `ident` on every live dot; `GET air/device/{id}`, `GET air/networks`, `GET air/ask`, `PUT air/kind/{id}`, `PUT air/class/{id}`, `PUT networks/{id}/guest`; calibrate at a known distance (`{"metres": n}`).
- **UI**: names never fall back to the short id; rows lead with whose it probably is; device page `/radio/device/:id` (solo radar, what it is, whose and why, chances and factors, evidence line, network, answers, history); Networks view; "Help it learn" card; radar zoom (buttons, wheel, pinch), median framing, "N beyond the edge", label collision avoidance; passers-by hidden by default with a count; presence strip legend; guest switch.

**Checks run (all passed):** `go build`, `go test ./...` and `go run ./tools/check-complexity/` in `backend`; `go test ./...` in `stacks/sdk`; `GOWORK=off go vet` and `go test` (and `-race`) in `stacks/radio`; `go test` in `stacks/network` and `stacks/agents`; `npm test` (3,168), `npm run build`, `npm run test:visual` (388); `./scripts/check-agent-harness.sh`. **Not run:** anything on vertex.

## Left, in order
1. **Owner:** review the pull request, including ADR-0058 and the I.16 amendment. Merging accepts both.
2. **Deploy and look on vertex** (read-only GETs as in the 2026-11-17 audit): `joined_own` and `own_cycles` should be non-zero for devices on your Wi-Fi within a cycle; `GET radio/air/networks` should show your ASUS mesh as one box; the SKY row should read "SKY UK router" (or its model if it sends WPS). The first cycle after deploy logs class changes under version 3 (expected).
3. **Answer the "Help it learn" card** for a few days; then check the learned line ("gives back N of M") on Around you. Thresholds stay provisional until that agreement is measured.
4. **Next architecture (needs an ADR and a Constitution item):** Bluetooth and a second-room sensor through ESPHome Bluetooth proxies on the LAN, as nodeherder does (see the research addendum §6).

## Traps
- `stacks/radio` is not in `go.work`: run its tests with `GOWORK=off`.
- A test that swings a device's signal by 20 dB each cycle now reads as movement (correctly); an access point's wobble is about 8 dB.
- `vue-tsc` in `npm run build` also checks the tests; a type and a component with the same name in one test file clash (`RadioDeviceView`).
- The radar fits the median device: a far device sits on the rim and is counted, not drawn at its distance.
- The night bit uses the server's local time (`isNight`, `features.go`); a systemd unit running in UTC shifts it by an hour in British Summer Time. Check `timedatectl` on vertex if the night share looks off.
- A label is written to the disk: the "Mine" default is make and model (`defaultLabel`), never the inventory name or the broadcast network name; a test reads the file for an inventory name.

**First file to open:** `stacks/radio/links.go`, then `stacks/radio/learn.go`.
