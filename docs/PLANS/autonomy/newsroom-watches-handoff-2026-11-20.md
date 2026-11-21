---
status: partial
date: 2026-11-20
last_reviewed: 2026-11-20
related_specs: [SPEC-013, SPEC-003]
constitution_references: [I.1, III.6, IV.6, V.2]
related_plans: [topic-watch.md, morning-paper.md]
evidence: Branch newsroom-watches, cut from newsroom-source-help-and-checks (fd42ec8c) on 2026-11-20; nothing committed. Checks as listed below.
---

# Newsroom watches: handoff

## State
- **Branch:** `newsroom-watches`, cut from `newsroom-source-help-and-checks`. **Nothing is committed or pushed** (git is the owner's).
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

## Traps
- The first-check feed entries break any test that expected an empty feed after a baseline; `harness.forgetFirstFindings` removes them for tests about later runs.
- `stacks/sdk` `NoticeRequest` is shared with the core, so the rename reached `stackport/notice.go`.
- Persisted keys changed: an old `topics.json` is ignored, not read.

## First command
`cd stacks/newsroom && GOWORK=off go test ./... -race`, then `stacks/newsroom/watches/runner.go` (`Run`, `baseline`, `showFirst`).
