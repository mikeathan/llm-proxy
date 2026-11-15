---
status: partial
date: 2026-11-15
last_reviewed: 2026-11-15
related_specs: [SPEC-006]
constitution_references: [I.16, I.17, II.3]
evidence: Session of 2026-11-15 (PR #257, v0.92.0). A discussion handover, not a build plan: the owner asked to talk through a better UI, visualisation, rendering and radar panel for the Radio page while a one-day watch runs on vertex. Delete this file when the redesign merges or the owner drops it (harvest first, documentation-stewardship).
---

# Radio UI redesign: handover for the discussion (2026-11-15)

**State.** PR #257 (branch `radio-fix-missing-routes`, v0.92.0, one commit on `origin/main` at fe630e50) is pushed and open: the wildcard-route fix, the 7-day watch with resume, the bell notices and the faster recorder. Nothing is built for the redesign and no mockup exists. **Do not start building: this is a conversation first**, then mockups (as in the earlier UI redesigns: three options, the owner picks, then build). The 1-day watch on vertex is the owner's next step; its findings may change what is worth showing.

**Rules that already bind this work:** `.agents/rules/frontend-vue-engineer.md` (mandatory), the owner's standing UI bar (clean, professional, simple; one path per job; problems shown with their fix; Simple and Advanced views; mobile first-class; no hidden knowledge), the visual regression spec `frontend/e2e/radio.visual.spec.ts`, Constitution I.16 (counts and words only: no address, no name from a frame on screen) and I.16's "position is an estimate with its uncertainty, never a fact".

## What is on the page today (read before proposing)

The Radio page has four tabs: **Listening** (the run, resume card, scan plan, What is kept, windows list, then Around you), **Sensors**, **My devices** (with Your networks and the hourly presence strip), **Modules**. Components in `frontend/src/components/radio/`:
- `RadioRadar.vue` + `RadioRadarCanvas.vue`: the radar, one list beside it (`RadioDeviceList.vue`), a device opened below (`RadioDeviceDetail.vue`, with `RadioTrack.vue`: distance over the run, from memory or the disk track), `RadioAroundStatus.vue` (live state and "next reading in"), `RadioReadGuide.vue`, `RadioFindings.vue`, `RadioAirwaves.vue` (channels).
- Data: `GET radio/air/live` (the radar: dots with an estimated range, rings, "usual" hollow dots, unusual flag, calibration model), `radio/air/track/{id}`, `radio/air/devices`, `radio/air/airwaves`, `radio/devices/mine/presence`. Polled every 15 s only while visible; the picture changes once a cycle (5 minutes by default).
- Types in `frontend/src/types/radio.ts`, words in `frontend/src/constants/radio.ts`.

## What the owner has said about it (verbatim where recorded)
- 2026-11-14: "the Radio UI is temporary: it lacks useful, interactive data." (Since improved by the Around you tab, but the owner has now asked again for a better UI, visualisation, rendering and radar panel.)
- 2026-11-15: wants to use the wait for the vertex day-run to design "better UI, visualisation, rendering and radar panel".

## Facts the design must respect (measured or read from code)
- **One sensor gives a range, not a position.** A dot is a distance band from signal strength; there is no direction until two sensors exist (R6, not built). The radar must not imply bearing. Distance needs calibration against an owner device held one metre away; until then it is a coarse ring.
- **Time resolution is a cycle** (5 minutes by default; 20 s a channel, about 6 channels, 40% duty). The picture is a sequence of snapshots, not a live feed; "live" should read as "last reading N seconds ago, next in M".
- **Memory vs disk.** The per-device signal track is memory only unless the owner turns on the disk track (ADR-0055: off by default, 24 or 48 hours). Neighbours are a keyed tag with a day-level history for 14 days; only the owner's devices have hourly strips for 30 days. A time scrubber therefore replays this run (memory) or the last 24 to 48 hours only when the disk track is on.
- **The class sorting is provisional** (`classifier_v1`, unmeasured; 12 cycles listened before a device is sorted). A design that leans on class colours must say "provisional" and handle many "not enough evidence yet" devices.
- **Retries read 0% on every frame** (open: adapter or driver may hide the flag); do not design around retry share until that is explained.
- **Radar endpoint cost is unmeasured** (`GET radio/air/live` reads all devices and up to 500 single-row lookups on one database connection). A richer radar that polls more or reads more needs a cache per finished cycle first.
- **The owner's phone is the main user.** Mobile width (390 px visual project) is first-class, the composer-style bigger touch targets apply (`max-sm:!h-11`).

## Directions put to the owner (none chosen)
1. **"What changed" first:** a calm summary of arrivals, departures and unusual devices over the last hours, the radar secondary. Answers "is anything new?" without reading a picture.
2. **A time scrubber on the radar:** step back through recent cycles (this run, or 24 to 48 hours with the disk track). Needs the tracks it replays; shows gaps honestly.
3. **A per-device presence strip for neighbours,** as the owner's devices have, from the day-level history (coarser: day, not hour, per ADR-0053).
4. **A compact status header** that makes the watch state obvious on a phone: listening or not, time left, next reading, resume; the bell notice lands on it.
Not yet asked, worth asking: what the owner actually checks the page for (new device? my phone home? neighbour noise?), whether the radar picture earns its space on a phone, whether a text "what changed" is better than any picture, and Simple vs Advanced for the radar.

## Questions to settle in the discussion, then record the answers here
1. What question should the page answer in five seconds? Rank: new or unusual device, are my devices home, channel congestion, am I being watched or deauthed.
2. Radar: keep as the hero, make it secondary, or replace with a list-first view with a small range chart?
3. Is replay (scrubber) worth building before there is a day of real vertex data?
4. Do class colours stay while the classifier is provisional?
5. Any wish for sound, a haptic or a push (the bell is built; connectors were declined for the watch notices)?

## Next steps, in order
1. Hold the discussion; write the owner's answers into the section above.
2. Read the day-run result (the owner pastes the Listening tab, the What is kept panel and a few device rows) so mockups use real numbers.
3. Build **three static mockups** (the owner picks), following the earlier redesign mockup recipe in the project memory; no data endpoint changes until one is chosen.
4. Then build the chosen one on a fresh branch from `origin/main`, with the visual spec updated (desktop and 390 px mobile), `npm test`, `npm run build`, `npm run test:visual`.

**First file to open:** `frontend/src/components/radio/RadioRadar.vue` (the composition of the Around you tab), then `RadioRadarCanvas.vue`. The vertex recipe is in `radio-stack-handoff-2026-11-14.md` ("Running things on vertex").
