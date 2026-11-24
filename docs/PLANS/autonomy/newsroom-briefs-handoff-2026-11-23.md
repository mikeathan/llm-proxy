---
status: partial
date: 2026-11-23
last_reviewed: 2026-11-23
related_specs: [SPEC-013, SPEC-003]
constitution_references: [I.16, IV.6, V.2]
related_plans: [morning-paper.md, topic-watch.md]
evidence: Branch briefs-dashboard-card-and-unread, cut from origin/main at b68c0f22 (#302, v0.126.0) on 2026-11-23; uncommitted, nothing pushed. Checks as listed below. This file replaces the earlier handoff of the same name, whose work is merged (#298 to #302).
---

# Newsroom Briefs: handoff (2026-11-23, after #302)

## State
- **Branch:** `briefs-dashboard-card-and-unread`, from origin/main at `b68c0f22`. **Uncommitted, not pushed.** Commit `backend/internal/transport/http/frontend_dist/index.html` with the front end. Version file says 0.127.0.
- Merged before this: edition page, Your watches, Home (#299), sections = categories (#300), one-field topic form (#301), sections hold topics (#302). Design, owner answers D1 to D10 and phases: `morning-paper.md`.

## Built on this branch (v0.127.0, ADR-0081, SPEC-013 1.20)
- **Read state on the server:** `Issue.read_at`, `Store.MarkRead`, `Store.Latest`, `POST .../issues/{id}/read`, `GET .../briefs/latest` (manifest lists both).
- **One poller** `useBriefUnread` (30 s, visible page only; module off means no dot, no error). Dots: Newsroom page's Briefs tab (`StackTabItem.badge`, `StackTabs.vue`), the dashboard's Newsroom tab (`DashboardBoardTabs.vue`), "· New" on the edition picker.
- **The Briefs page marks the edition on screen read**, only in the read view on a visible page; the dashboard never does (ADR-0064).
- **Dashboard card `newsroom-brief`** (`cards/NewsroomBriefCard.vue`, 12 columns, first on the Newsroom board; an owner's saved board is offered it by "New on this board"): masthead + New tag, lead with art, top stories in two columns, a chip per section, "Show every section" opens the edition in the card (two columns, grows the card); medium = lead + count; small = one line; empty states say how to set one up or when it is due.
- **Two briefs:** the Briefs page opens on the first unread brief and marks nothing until both lists are loaded (`NewsroomBriefs.vue` `markShown`; test `NewsroomBriefs.unread.component.test.ts`). The card's section chips are square buttons that open the edition at that section.
- Repeated headlines across sections (item 5 of the old list): **investigated, not reproduced**. `candidates` skips any key already `placed` by an earlier section and the pool merges by title key and link; news-search titles already lose their " - Publisher" suffix. A near-duplicate with different wording gets a different key and would repeat: that is the only path the code leaves, and it needs the owner's real pool to confirm: fetch the pasted edition from vertex (`GET /admin/api/newsroom/briefs/issues/{id}`) and compare the `pool_id`s of the repeated headlines. The same id in two sections is a placement bug; different ids are one story worded two ways.

## Checks run (all on the final tree)
Stack `GOWORK=off go test -race . ./sources ./watches ./brief` and `cd stacks/sdk && go test ./...` (pass); backend `go build ./... && go test ./...` (pass) and `go run ./tools/check-complexity/` (OK; it failed once on `handleBriefs` at 13 and was fixed by folding the two routes into `handleIssues`); frontend `npm test` (374 files, 3540 tests) and `npm run build` (pass); `npm run test:visual` (436 passed, 2 skipped; screenshots of the card unread and open at 1440 and 390 looked at); `./scripts/check-agent-harness.sh` (pass). **Not run:** the other themes and print preview; anything on vertex (does `/briefs/latest` answer through the core? does the dot clear when the page is opened?).

## Left, in order
1. **Owner:** review ADR-0077 and ADR-0081; commit, merge, deploy; on vertex open Newsroom on the dashboard (a saved board shows "New on this board: Brief", press Add them) and check the dot clears after opening the Briefs page. Existing editions show unread once.
2. Look at the other themes and print preview of the edition page and card (only the paper theme was looked at).
3. A source count in the masthead needs the issue to store how many sources it read (not stored today).
4. **Your day** (phase 5): needs an owner decision on whether the owner's own reminder text may pass a port (ADR-0068).
5. **Telegram** (phase 6, D3/D4): a brief notice shape so the New brief can go to the phone; then providers with a key.
6. Idea, not built: a dot on the sidebar's Newsroom item (the poller is already shared).

## Traps
- `go test ./...` does not work in `stacks/newsroom` (workspace): name the packages, with `GOWORK=off`. Topic ids and edition ids share one counter in the store (a test's first edition is id 2).
- Tests that mount a component using `useBriefUnread` must unmount it: the state is module-level and a leftover component keeps answering the next test. A `.component.test.ts` name is what gives a test the DOM.
- The mock admin API matches by path suffix: answer `/briefs/issues/5` and `/briefs/latest` separately; a static `latest` answer keeps the dot after the page reads it (make it stateful).
- `HomeLine` must keep exactly `ID, Count, Words`; the newsroom model port is on only while either module is on.

## First command
`cd stacks/newsroom && GOWORK=off go test -race . ./brief`, then `frontend/src/composables/newsroom/useBriefUnread.ts`.
