---
status: partial
date: 2026-11-25
last_reviewed: 2026-11-25
related_specs: [SPEC-013, SPEC-003]
constitution_references: [IV.6, V.2]
related_plans: [../ARCHIVE/autonomy/morning-paper.md, ../ARCHIVE/autonomy/topic-watch.md, ../ARCHIVE/autonomy/newsroom-review-2026-11-25.md, ../cross-cutting/email-connector.md, attention-policy.md]
evidence: Read of vertex (v0.135.0, commit 90a08290) through the admin API on 2026-11-25 16:42-16:43 UTC, GETs plus one "Check now" on watch 1.
---

# Newsroom: what is left after v0.135.0 (handoff, 2026-11-25)

**Status:** partial. The Newsroom plans are built and archived: Topic Watch (the Alerts), Briefs (was Morning Paper) and the 2026-11-25
review (`../ARCHIVE/autonomy/`). SPEC-013 is the contract. This page holds only what is left.

## Branch and commit state

- #308, #309, #310 and #311 are merged and deployed; vertex reports v0.135.0.
- **Uncommitted, on branch `newsroom-brief-card-fit` (already merged, so it needs a new branch):** v0.136.0, SPEC-013 1.31: one click each way
  between reading and setting up. Alerts → "Set up alerts →"; the brief's masthead → "Set up the brief →"; Following → "Read the brief →" /
  "See alerts →" and a "New watch" button on its Alerts half; Follow on the Brief tab too. Before it the watches sat behind Following's
  "In your brief / Alerts" switch, linked from Alerts only when a watch had a problem, so the owner thought the page was gone. It also
  moves these plans to the archive. Checks run: `npm test` (393 files, 3629 tests, pass), `npm run build` (pass),
  `e2e/newsroom-briefs.visual.spec.ts` (22 pass; screenshots of the brief at desktop and phone and of Following looked at),
  `go build ./...` in `backend` and `stacks/newsroom` (comment edits only), `./scripts/check-agent-harness.sh`.
  **Not run:** the full `npm run test:visual`; the visual spec has no Alerts-tab case.

## Checked on vertex (2026-11-25, after the v0.135.0 deploy)

- **The scheduled brief is full:** Morning issue 8 (06:40 UTC) has 4 top stories, Technology (topics 1 and 2 share the section, by
  category) and News with 5 stories each, 3 Home lines and 3 model calls. Evening issue 6 has the same shape.
- **"Left out by the model" fills:** 4 entries from the 16:30 run, each with its reason.
- **Nothing to judge yet on the new alert prompt:** "Check now" on watch 1 at 16:43 returned `nothing_new`. The two alerts of 25 Nov
  ("Google Releases Gemini Neon for Empathy Tasks" from a Reddit post titled "Gemini Neon on Empathy") were written before the deploy and
  are the overclaiming the new prompt is meant to stop.

## Built on branch `newsroom-setup-links-and-learning`

- `8a9259ba` v0.136.0: the set-up/read links (SPEC-013 1.31) and the plan archive.
- `36335455` v0.137.0: stories with text keep a third of a section's candidates (SPEC-013 1.32).
- v0.138.0: a watch learns from "Should have matched" and the new "Not for me" (ADR-0088, SPEC-013 1.33); the two old redirects removed.
- Checks: `GOWORK=off go test -race -count=1 ./...` in `stacks/newsroom`; `go build ./...`, `go test ./...` and `go run ./tools/check-complexity/`
  in `backend`; frontend `npm test` and `npm run build`; `e2e/newsroom-briefs.visual.spec.ts` with a new Alerts case (touch target, no sideways
  scroll, desktop and phone); `./scripts/check-agent-harness.sh`. **Not run:** anything on vertex; the full `npm run test:visual`.

## After the deploy, on vertex

1. Mark two or three alerts "Not for me" and one left-out post "Should have matched"; after the next check, read the watch's prompt result: do
   similar posts follow the verdicts? (ADR-0088 is proposed until this is seen.)
2. The brief: the next edition should carry more stories with summaries (the Guardian, BBC, Sky feed stories reach the model now).

## Left, in order

1. **Owner:** review and merge the PR for this branch, then deploy; then the two vertex checks above.
2. **Still unchecked on vertex** (they need new posts or a browser): no "Changed" without a citation; headlines of user posts say they are posts;
   the sidebar dot clears when alerts are read; "Write a new one" shows its steps; a real Telegram send.
3. **Owner's separate work:** an https address so the Telegram summary can carry a link (text only until then).
4. **Not built, waiting on a design or on the owner:** Telegram channels as brief sources (the shared slot keyed by channel,
   `stacks/newsroom/brief/types.go`); keyed sources (Reddit, Bluesky, X) when the owner brings credentials; email as a channel (own plan,
   `../cross-cutting/email-connector.md`); held notices as a brief section (`attention-policy.md`, D9 of Briefs).
5. **Measurement:** review item P3 (`briefs.json` is rewritten whole on each change; the pool has its own file since #308) is unmeasured, so
   measure the file on vertex before changing it.

## Traps

- A control beside a link needs its own 44 px box; padding that overlaps the link loses the tap (learnings, 2026-11-25).
- The Following tab opens on "In your brief"; the watches are on `?kind=alerts`. Link there with `toFollowing('alerts')`.
- The 16 `/design` visual cases have failed on every branch since 2026-11-24; they render none of the Newsroom code.

## Start here

`frontend/src/views/NewsroomView.vue` (tabs and the Alerts view), `stacks/newsroom/newsroom.go`, SPEC-013 (`docs/SPECS/newsroom.md`).
