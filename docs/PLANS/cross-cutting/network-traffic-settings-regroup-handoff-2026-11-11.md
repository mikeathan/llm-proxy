---
status: partial
date: 2026-11-11
last_reviewed: 2026-11-11
related_specs: [SPEC-003]
constitution_references: [I.13]
evidence: DB-IP Lite City CSV measured 2026-11-11 (see below); frontend/src/components/netaudit/TrafficReport.vue, TrafficPlaces.vue, TrafficFlows.vue, NetworkSettings.vue; docs/decisions/ADR-0043.
---

# Network: Traffic regrouped, Settings in five groups (handoff)

Branch: still `network-clickable-findings-traffic-owners` with its v0.71.0 commit; the work below is **uncommitted** on top of it and should move to a fresh branch from `main` (the v0.71.0 PR was merged and deployed). `VERSION` is not bumped.

## Built (frontend only; checks run: `npx vitest run` 287 files / 2905 tests pass, `npm run build` passes incl. lint, `npx playwright test e2e/netaudit-*.visual.spec.ts e2e/network-*.visual.spec.ts` 298 pass after fixing the specs)

- Traffic tab: summary + chart, one "Where it went" panel (Places / Operators / Countries), "What it was" (kind chips + conversations). Details in SPEC-003 entry 2.61.
- Settings tab: five groups chosen by `?group=`; every link to a card opens its group.
- **Not run:** `go build` / `go test` (no backend change), `check-agent-harness.sh` was not run before this file was written, nothing seen on a real look from vertex (the screenshot used mock data).

## Built after the owner chose a smaller memory footprint (backend + frontend)

The owner rejected ~75 MB resident. The city table is now an index file searched on disk (ADR-0044): `core/netaudit/ipplaces` (`Build`, `Index`, `Manager`), `TrafficOwner.City/Region`, `currentOwners` composes owner + city, `GET/DELETE destination-places` and the step-up `POST destination-places/download`, the data card drawn twice. Checked on the real 85.7 MB table: build 4.7 s, a 75 MB file, 36 MB heap while building, ~0 after, 7.7 microseconds a lookup. Checks run: `go build ./...`, `go test ./...` (all pass), `go test -race` on netaudit and transport, `go run ./tools/check-complexity/` (no new violations), frontend `npx vitest run` (2908 pass), `npm run build`, the network Playwright specs.

## Remaining Work

1. **Owner, on vertex:** deploy, press "Download the table" under Cities (needs the passphrase; about 86 MB), listen, read the owner lines. Not run against db-ip.com from this program (the real file was fetched by hand and fed to `Build`); the first run proves the month-named URL through `NetworkTools.Open`.
2. Decide whether the Traffic privacy note should name the city table (it names "a public table kept on this computer").
3. The helper-less case: a city shows only for an owner IPtoASN found, so a city alone never appears; change that only if the owner asks.
