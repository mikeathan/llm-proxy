---
status: partial
date: 2026-11-23
last_reviewed: 2026-11-23
related_specs: [SPEC-013, SPEC-003]
constitution_references: [I.16, IV.6, V.2]
related_plans: [morning-paper.md, topic-watch.md]
evidence: Branch newsroom-briefs-screens, cut from origin/main at 322f5002 (#298) on 2026-11-23; uncommitted, nothing pushed. Checks as listed below. This file replaces newsroom-watches-handoff-2026-11-20.md, whose work is all merged (#292 to #298).
---

# Newsroom Briefs: handoff (2026-11-23)

## State
- **Branch:** `newsroom-briefs-screens`, from origin/main at `322f5002`. **Uncommitted, not pushed.** Commit `backend/internal/transport/http/frontend_dist/index.html` with the front end. Nothing is stacked on it.
- The watches work of 2026-11-20 to 2026-11-22 is merged; the design, owner answers D1 to D10 and the phases are in `morning-paper.md`.

## Built on this branch
- **Edition page** (`NewsroomEdition.vue`, `NewsroomPlate.vue`, `utils/newsroom/plate.ts`): masthead, lead story with line art, a card per topic with its plate, 3 stories a card and "Show more", Collapse all.
- **Your watches** (phase 4, half): `Edition.Watches`, `Issue.Watches`, `Publisher.Watches`, `stacks/newsroom/briefwatches.go`, `NewsroomEditionWatches.vue`, editor switch.
- **Home** (phase 4, half): new SDK port `briefhome` (`stacksdk.Home`, `HomeLine`), `backend/internal/core/stackport/home.go`, `app/stacks.go` `homePort`, `radiodigest.Findings`, `Edition.Home`, `Issue.Home`, `NewsroomEditionHome.vue`, editor switch. **ADR-0077 is proposed**: counts and the core's fixed words only, never into a model, fetch or notice. `checkPorts` now also checks the notice port.
- SPEC-013 1.13 and 1.14.

## Checks run
Stack `GOWORK=off go test -race . ./sources ./watches ./brief`; `cd stacks/sdk && go test ./...`; backend `go build ./... && go test ./...`; `go run ./tools/check-complexity/`; frontend `npm test` and `npm run build`; harness. See the final message of the session for the last result of each. **Not run:** `npm run test:visual`; the page in a browser or on a phone; anything on vertex (does the Home port read the live inventory and Radio digest? does the presence line say "cannot say" when the check is off?).

## Left, in order
1. **Owner:** review and accept or change ADR-0077; commit, merge, deploy; on vertex switch "Include my network and radio" and "Include what my watches found" on an edition, press Write it now, check the blocks.
2. **Built 2026-11-23:** the side column (Home, Your watches beside the news on a desktop, under it on a phone) and a minimal print rule. **Looked at 2026-11-23** in Chromium at 1440 and 390 px with invented data (spec `frontend/e2e/newsroom-briefs.visual.spec.ts`, the edition-page test: side column right of the news on desktop and under it on a phone, cards capped at three, no sideways scroll; screenshots by `SCREENSHOTS=1`). **Left:** print preview (not looked at), the other themes (only paper was looked at), Your day in the side column, the dashboard card (links only, ADR-0064), the masthead's source count.
3. **Your day** (phase 5): a reminders and agenda port in the SDK, same pattern as Home (typed, core-brokered, no free text the stack did not own); the owner's own reminder text is the owner's (ADR-0068), so a decision is needed on whether it may pass.
4. **Telegram** (phase 6): a brief notice shape in the SDK so Home counts can go to the phone (D3 link address); then providers with a key (phase 7, D4).
5. Nothing from the pasted edition of 2026-11-23: every story showed "Headline only" and the same headlines repeated across sections. Cause not investigated: news-search items carry no text, and the pool de-duplicates inside a topic, not across sections.

## Traps
- A brief topic must not read Telegram until a slot keyed by channel exists. A create must send `enabled`. `go test ./...` does not work in `stacks/newsroom` (workspace): name the packages, with `GOWORK=off`.
- `HomeLine` must keep exactly `ID, Count, Words` (a test fails otherwise); the Home lines are stored after Compose so the model never sees them (`TestPublisher_HomeBlockIsStoredAndNeverShownToTheModel`).
- The newsroom model port is on only while either module is on (`internal/app/stacks.go`).

## First command
`cd stacks/newsroom && GOWORK=off go test -race . ./sources ./watches ./brief`, then `backend/internal/core/stackport/home.go`.
