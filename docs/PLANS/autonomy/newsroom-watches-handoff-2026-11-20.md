---
status: partial
date: 2026-11-21
last_reviewed: 2026-11-21
related_specs: [SPEC-013, SPEC-003]
constitution_references: [I.1, III.6, IV.6, V.2]
related_plans: [topic-watch.md, morning-paper.md]
evidence: Branch newsroom-source-error-reporting, cut from origin/main at v0.120.0 (#292) on 2026-11-21, v0.123.0; committed as one commit and pushed, with one pull request open (find it with `gh pr list --head newsroom-source-error-reporting`; merge order: nothing else is stacked on it). The newest section below ("Added 2026-11-22") is the state; the earlier sections are history that is still true. Checks as listed in each section.
---

# Newsroom watches: handoff

## State
- **Current branch (2026-11-21):** `newsroom-source-error-reporting`, cut from origin/main at v0.120.0, v0.123.0. It carries the unread-source fix, the news-search source and Briefs: collector, edition pass, the Briefs tab and the model-calls setting (see the newest sections below). **One commit, pushed, PR open; the owner merges.** The older watches work in this file is already on main.
- **Original branch of this file (2026-11-20):** `newsroom-watches`, cut from `newsroom-source-help-and-checks`.
- ADR-0073 (proposed) decides the redesign; SPEC-013 is at 1.1. This file replaces `newsroom-handoff-2026-11-20.md`, whose list of what to build is done (core model port, notice port, registration, page).

## Built (2026-11-20)
- **Rename, topic to watch, everywhere:** `stacks/newsroom/watches/`, routes `/admin/api/newsroom/watches`, tools `newsroom_follow` and `newsroom_watches`, SDK `NoticeRequest.Watch`, module id `watches`, file `watches.json`, frontend types, components, constants and tests.
- **What to look for:** `look_for`, `keywords`, `skip` on a watch; `Watch.Gate` (keyword stage); the model call now also returns `matches` with a reason; items not matched are recorded as seen and named in the row; no judgement keeps every item.
- **First check shows up to five findings** marked "First check" (`Runner.showFirst`); nothing is sent.
- **Page:** Feed, Watches, Settings. The watch form has suggestions (Product releases, Security problems, Price drops, Job posts, Outages, Anything new), What to look for, keywords, skip words, and the per-source guide. Sources, Help and the source art are deleted. Entries show "Why:" and a "First check" chip.
- Removed the unused `Search`, `Paper`, `Section` fields.

## Checks run
`cd stacks/newsroom && GOWORK=off go test ./... -race` pass; `cd backend && go build ./... && go test ./...` pass; `go run ./tools/check-complexity/` pass (`parseAnalysis` was split); `cd frontend && npm test` (3408) and `npm run build` pass, `frontend_dist/index.html` rebuilt (commit it). **Not run:** `npm run test:visual`; anything on vertex; the real model's relevance judgement (only a scripted model was tested); a look at the page in a browser or on a phone.

## Left, in order
1. **Owner:** commit and merge; deploy; **re-create "Ai post news" on vertex** (the file and key names changed, there is no shim). Use What to look for, then press Check now.
2. **Check the model's judgement live** on vertex with a real local model: does it return the `matches` list as JSON, or does it fall back to "not compared"? A model that answers in prose keeps every item (fail open), so a weak model means no filtering, not hidden items.
3. **Hot-topic suggestions: built in v0.118.0 as the source look-up** (ADR-0074, `stacks/newsroom/sources/lookup.go`); Mastodon trending tags are offered, the Hacker News front page is not (it does not help choose a search). Original plan: a keyless route in the stack that reads Mastodon trending tags (`/api/v1/trends/tags`) and the Hacker News front page through the guarded `Fetch`, and offers them as chips that fill the form (a Mastodon tag source, or a Hacker News search). Not built; needs a GET-only fetch of a JSON list, so the existing adapters' pattern fits.
4. Visual baseline for the new page.

## Added 2026-11-21 (uncommitted, on the merged `newsroom-source-finder` branch; a new branch is the owner's call)
- Headline + summary per item (`digests`), "Checking…" on a running watch, a period filter (Everything / Today / 7 / 14 days) and Clear that follows the filter. Checks run: stack `-race`, backend build/test, complexity, frontend test + build, harness. **Not run:** `npm run test:visual`; vertex (does the real model return `digests` without breaking the JSON? the model port's output cap was not found in the code); a look on a phone. Commit `frontend_dist/index.html`. Notices sent to a channel still carry the original titles (`Digests` stays inside the stack).

## Added 2026-11-21 evening (uncommitted, on the merged `newsroom-headlines-and-filters` branch; a new branch is the owner's call)
- News window `max_age` (default 72 h, form picker), `Entry.Posted` and "posted …" on rows, the period control on the dashboard card, a fallback headline when the model wrote none. Checks run: stack `go test . ./sources ./watches`, backend `go build` and `internal/app` tests, complexity, frontend test + build, harness. **Not run:** `npm run test:visual`; vertex. Missing headlines were the primary model (Qwen3.6) leaving `item` off its digests; fixed by a firmer prompt and a by-position fallback (see the learnings line of 2026-11-21).
- Left: on vertex, run Check now and confirm new findings carry a headline and only recent posts appear.

## Added 2026-11-22: D8 and the edition pass (same branch, uncommitted)
- Built: `newsroom.model_calls_per_day` (D8, settings.yml only; the screen field comes with the Briefs screens); `brief/compose.go`, `publish.go`, `issues.go`, `timing.go` (the collector and publisher share one clock/schedule type); routes to make and read editions; SPEC-013 1.10, ADR-0075 point 4. `sources.JSONObject` is now shared. Checks run: `GOWORK=off go test ./...` in `stacks/newsroom`, backend `go build`, tests of app, stackport, models and http, complexity (clean), harness.
- Not run: `npm run test:visual` (no front-end change this slice), anything on vertex. Not checked: the real model's answer to the section prompt.
- Also built the same day: the **Model calls** setting (`GET|PUT /admin/api/newsroom/briefs/settings`, the box under Briefs, Set up), and the **Briefs tab** (`frontend/src/components/newsroom/NewsroomBriefs.vue` and friends, SPEC-013 1.11): read an edition, write it now, add/edit/delete topics and editions, read a topic now. Checks run: `npm test` (all pass), `npm run build` (lint, type check, build; `frontend_dist/index.html` rebuilt). **Not run:** `npm run test:visual`. The tab was looked at in headless Chromium with mocked answers (dark desktop, paper phone, setup form) and two things fixed: a note colour that used a token that does not exist, and a new form with no space above the list.
- Next, in order: the mocks' masthead, lead story and art; then Your watches and Home (phase 4).

## Added 2026-11-21 late night: Briefs, first slice (same branch, uncommitted; v0.122.0)
- **This branch is not usable by the owner yet for Briefs:** the back end collects headlines, but there is no Briefs screen and no edition. Do not report it as shippable on its own; it is safe to merge (watches behave as before, plus the error-reporting fix and the news-search source).
- Built: the stack's **model gate** (`newsroom/modelgate.go`); **`sources.Registry`**, `Spec` and `Build` moved from `watches` into `sources`; the **news-search kind** (`sources/news.go`) for watches too (form Region field, guide, look-up, SPEC); the **Brief module**: `newsroom/brief/` (types, store, pool, collector), `briefroutes.go`, a second module in the manifest and core (`models.NewsroomConfig.BriefsDisabled`, `newsroomBriefsSwitch`, the schedule port for the newsroom host, `stacks_about.go`). ADR-0075 and SPEC-013 1.8; plan `morning-paper.md` (now Briefs) has the design, D1-D10 answers and the mocks in `briefs-mocks/`.
- Checks run: stack `go test -race . ./sources ./watches ./brief`; backend `go build ./...`, `go test ./...` (one failure, `TestStreamHeartbeat_NoStillThinkingDuringActiveStreaming` in `core/assistant`, passes alone x3 and is unrelated: timing under load), `check-complexity` (five new functions were refactored); frontend `npm test` (3457) and `npm run build`; harness. **Not run:** `npm run test:visual`; anything on vertex.
- Left, in order (full list in the plan's Remaining Work): D8 (a settable model-call limit; the 200 a day is a constant in `stackport/model.go`) before the edition pass ships; the edition pass; the Briefs screens from the mocks; Your watches and Home; Your day (SDK reminders and agenda port); Telegram with Home lines (SDK brief notice shape); keyed providers (D4).
- Traps: a brief topic must not read Telegram until a slot keyed by channel exists (the floor is per reader instance and shared with watches); the news-search link is a Google page (owner: click one in a normal browser, a consent page may show first); `Edition.Enabled` has no default, so a create must send `enabled`; stack tests that need the Brief module must give the host a schedule port (`startBriefs` in `briefroutes_test.go`); `go test ./...` does not work in `stacks/newsroom` (workspace), name the packages.
- The newsroom model port is enabled only while the **Watches** switch is on (`internal/app/stacks.go`: `Model: s.stackModelPort(..., func() bool { return !policy.Get().Disabled })`). The edition pass must change that to "either module on", or an edition would be refused whenever Watches is off.
- First command: `cd stacks/newsroom && go test -race . ./sources ./watches ./brief`, then `brief/collector.go`.

## Added 2026-11-21 night (uncommitted, branch `newsroom-source-error-reporting`, cut from origin/main at v0.120.0 / #292)
- v0.121.0: unread sources kept on the watch (`last_problems`), shown on the row and the dashboard card; a Telegram channel with no public preview (`AnthropicAI_moon` on the owner's watch) says so, once. Checks run: stack `go test . ./sources ./watches`, backend build + test + complexity, frontend test (3454) + build, harness. **Not run:** `npm run test:visual`; vertex.
- Left: deploy, then on vertex the "AI news" row should show "Source not read" for `AnthropicAI_moon`; remove that source (or replace it) in the form. The first real finding with the cloud model still has to confirm headlines + summaries come back (the 18:52 run found nothing to digest).

## Traps
- The first-check feed entries break any test that expected an empty feed after a baseline; `harness.forgetFirstFindings` removes them for tests about later runs.
- `stacks/sdk` `NoticeRequest` is shared with the core, so the rename reached `stackport/notice.go`.
- Persisted keys changed: an old `topics.json` is ignored, not read.

## First command
`cd stacks/newsroom && go test -race . ./sources ./watches ./brief` (name the packages; `./...` does not work from here), then `stacks/newsroom/brief/collector.go`. For the watches themselves: `watches/runner.go` (`Run`, `baseline`, `showFirst`).
