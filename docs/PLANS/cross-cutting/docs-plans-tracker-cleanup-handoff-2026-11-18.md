---
status: partial
date: 2026-11-18
last_reviewed: 2026-11-18
related_specs: [SPEC-003, SPEC-004, SPEC-006]
constitution_references: [I.15, I.16]
evidence: Branch docs-plans-tracker-cleanup, commits listed below; checks run 2026-11-18 as listed. Vertex read-only GETs of /admin/api/radio/networks and /admin/api/netaudit/wifi on 2026-11-18 (two names in Radio, one in the core).
---

# Branch `docs-plans-tracker-cleanup` (v0.104.0): handoff

**Status:** partial. One branch from `main` at `f96d25d4` (after #269), as the owner asked ("all the work in one branch"), pushed with one
pull request. Nothing here has run on vertex.

## What is built (one commit each)

- Add session_search: search the words of this workspace's earlier conversations
- Bring the plan tracker up to date and make the harness check plan status words
- Show everything the service stores, with its size, under Settings → Storage & logging (data page D1, D2)
- One list of the owner's Wi-Fi networks and one look-alike check over both sensors (ADR-0063)
- Network presence reads the router's list of joined Wi-Fi devices (ADR-0062)
- Measure which WorkspaceManager methods each consumer calls (coupling plan P0)
- Remove the NodeHerder MCP system-prompt subscription nobody read
- Hand over what is left of the branch, record two learnings, and bump to v0.104.0
- Review fixes: no false foreign joins after the upgrade, session_search for the owner only, honest bounds

## Checks run (2026-11-18, on the final tree)

- `cd backend && go build ./... && go test ./...`: pass. `go run ./tools/check-complexity/`: OK, no new violations.
- `cd stacks/radio && GOWORK=off go test ./...`: pass. `cd stacks/sdk && go test ./...`: pass.
- `cd frontend && npm test`: 327 files, 3207 tests pass. `npm run build`: pass. `npm run test:visual`: 394 passed (before the last review
  fixes; `e2e/network-wifi.visual.spec.ts` re-run after them: 2 passed).
- `./scripts/check-agent-harness.sh`: PASS (and it now fails a live plan with an unknown status word; tried with `built`).

## What is left, in order

1. **Owner: merge the pull request and deploy**, then on vertex:
   - Network → Modules: switch on **Joined devices, from the router** (step-up). A phone with its screen off should stay Online on the Devices
     tab. The router's log should show at most one sign-in per 5 minutes from presence (ADR-0062).
   - Network → Settings → Watching → Wi-Fi look-alikes: `netsink-2G` is now in the list (moved from Radio). Its access points that you
     confirmed on the Radio page show "You confirmed it on the Radio page before". Press the "Confirm the … you confirmed on the Radio page"
     button once per network, **right after deploying**: until then they raise nothing, and Radio counts a join to them as neither yours nor
     foreign, so its own-network marks for those boxes are off. Then the Radio page's Your Wi-Fi networks shows them as confirmed (ADR-0063).
   - Settings → Storage & logging → What is stored: read the rows. Any "Not described" row is a store to add to `coreStoreKinds`
     (`backend/internal/app/datausage.go`).
   - A chat: ask about something said in an earlier conversation; the assistant may call `session_search`.
2. **Agenda follow-ups (item 3) wait on the owner's D1 and D4** (`docs/PLANS/autonomy/agenda-reminders-and-followups.md`). Recommended:
   D1 yes with the plan's limits, D4 yes. Then Phase 1 (`RunReason`, with a real producer) and Phase 3 (`schedule_followup`).
3. **Data page D3 and the rest of D4** (`cross-cutting/data-management-page.md`). Both need an SDK change (`Purge` on the storage interface,
   and a way to pause a stack's optional writers). Ask first.
4. **WorkspaceManager P1** (`cross-cutting/workspace-manager-coupling.md` §3a): four small interfaces for seven files.
5. **ADR statuses for the owner to set** (not changed here): those still `proposed` whose work is merged are listed in the pull request.

## Traps found

- The REPL refuses a script with a `\uXXXX` escape: build the backslash with `String.fromCharCode(92)` when a file needs one.
- `gofmt -w` on a whole folder reformats files nobody touched (`bootstrap.go`, `models/footprint.go` are not gofmt-clean on `main`).
  Format only the files you change, and put others back with `git show HEAD:<path> > <path>` (`.text()` drops the last newline).
- `Recorder.ForgetNeighbours` holds `r.mu`, and `currentEpoch` takes it, so code that runs inside the rotation must read `r.epoch`
  directly (`syncAfterRotation`, `stacks/radio/ownlist.go`).
- `NetworkDevice.LastSeen` decides which device holds an address (`lastOwners`, `newestDeviceAt`). A sighting without an address goes in
  `RouterSeen`, and "is it here lately" reads `SeenAt()`.
- Running `npm test` at the same time as `npm run build` gave 8 spurious failures; alone it passes.

## First file to open

`docs/PLANS/README.md`, then this file's item 2.
