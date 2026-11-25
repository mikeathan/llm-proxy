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

## Built on branch `newsroom-setup-links-and-learning` (not pushed)

- `8a9259ba` v0.136.0: the set-up/read links (SPEC-013 1.31) and the plan archive.
- Next commit, v0.137.0: stories with text keep a third of a section's candidates (SPEC-013 1.32). Checks: `GOWORK=off go test -race -count=1 . ./brief ./watches ./sources` in `stacks/newsroom` (pass). Not run on vertex.

## Agreed with the owner 2026-11-25, not built yet (do next, in this order)

1. **Drop the redirects** `/newsroom/briefs` and `/newsroom/watches` (`frontend/src/router/index.ts:85-86`): no Telegram summary ever carried a link (text only, no https), so nothing breaks. Remove their tests too.
2. **Teach the alert filter** (owner: A yes, B and C as recommended): "Should have matched" (`POST /admin/api/newsroom/feed/keep`, `watches/journal.go` `Keep`) also saves the item's title as a *want* example on its watch; a new "Not for me" button on an alert row (`NewsroomEntryItem.vue`) hides it and saves a *skip* example. Keep the newest 10 of each per watch (a `Examples` field on `watches.Watch`), send them in the analyst prompt (`watches/analyst.go` `prompt`) in fenced blocks as examples the person said they do / do not want, and list them in the watch's Edit form (`NewsroomWatchEditor.vue`) with a remove button each. It changes the prompt: write an ADR and a SPEC-013 entry; check on vertex against real posts. Mobile: the button needs a 44 px tap area.
3. A visual case for the Alerts tab and the new buttons, desktop and phone.

## Left, in order

1. **Owner:** approve a branch and a commit for v0.136.0, then merge and deploy.
2. **On vertex, after new posts arrive:** no "Changed" without a citation; headlines of user posts say they are posts; the sidebar dot
   clears when the alerts are read; an old Telegram summary's `/newsroom/briefs` link lands on the brief (a browser redirect, curl cannot
   see it); "Write a new one" shows its steps; a real Telegram send. None of these was run.
3. **Owner's calls:** an https address for the Telegram link (text only until then); direct feeds (BBC, the Guardian) so more stories
   carry text; dropping the two old redirects once no summary from before v0.132.0 is kept; teaching the filter from "Should have
   matched" (a prompt change, so a SPEC change).
4. **Not built, waiting on a design or on the owner:** Telegram channels as brief sources (the shared slot keyed by channel,
   `stacks/newsroom/brief/types.go`); keyed sources (Reddit, Bluesky, X) when the owner brings credentials; email as a channel (own
   plan, `../cross-cutting/email-connector.md`); held notices as a brief section (`attention-policy.md`, D9 of Briefs).
5. **Tests and measurement:** the visual spec has no Alerts case; review item P3 (`briefs.json` is rewritten whole on each change; the
   pool has its own file since #308) is unmeasured, so measure the file on vertex before changing it.

## Traps

- The Following tab opens on "In your brief"; the watches are on `?kind=alerts`. Link there with `toFollowing('alerts')`.
- The 16 `/design` visual cases have failed on every branch since 2026-11-24; they render none of the Newsroom code.

## Start here

`frontend/src/views/NewsroomView.vue` (tabs and the Alerts view), `stacks/newsroom/newsroom.go`, SPEC-013 (`docs/SPECS/newsroom.md`).
