---
status: partial
date: 2026-11-25
last_reviewed: 2026-11-26
related_specs: [SPEC-013, SPEC-003]
constitution_references: [IV.6, V.2]
related_plans: [../ARCHIVE/autonomy/morning-paper.md, ../ARCHIVE/autonomy/topic-watch.md, ../ARCHIVE/autonomy/newsroom-review-2026-11-25.md, ../cross-cutting/email-connector.md, attention-policy.md]
evidence: Read of vertex (v0.135.0, commit 90a08290) through the admin API on 2026-11-25 16:42-16:43 UTC, GETs plus one "Check now" on watch 1; GETs of /version and the brief issues on vertex (v0.138.0) at 18:52 UTC. On 2026-11-26 (v0.141.0): GETs of /version, the issues, /system/storage, the feed and watches; POST feed/skip [23, 26, 27] and feed/keep [38] and one "Check now" on watch 1 (09:30 UTC); the analyst runs from a scratch harness against /v1 on vertex.
---

# Newsroom: what is left after v0.141.0 (handoff, updated 2026-11-26)

**Status:** partial. The Newsroom plans are built and archived: Topic Watch (the Alerts), Briefs (was Morning Paper) and the 2026-11-25
review (`../ARCHIVE/autonomy/`). SPEC-013 is the contract. This page holds only what is left.

## Branch and commit state

- #308 to #314 are merged and deployed; vertex reports v0.141.0 (commit e41f49e1, built 2026-11-26 09:20 UTC).
- Branch `models-fix-and-cleanups` (v0.142.0, not committed when this was written) carries only this page's update for the Newsroom.

## Checked on vertex (2026-11-25, after the v0.135.0 deploy)

- **The scheduled brief is full:** Morning issue 8 (06:40 UTC) has 4 top stories, Technology (topics 1 and 2 share the section, by
  category) and News with 5 stories each, 3 Home lines and 3 model calls. Evening issue 6 has the same shape.
- **"Left out by the model" fills:** 4 entries from the 16:30 run, each with its reason.
- **Nothing to judge yet on the new alert prompt:** "Check now" on watch 1 at 16:43 returned `nothing_new`. The two alerts of 25 Nov
  ("Google Releases Gemini Neon for Empathy Tasks" from a Reddit post titled "Gemini Neon on Empathy") were written before the deploy and
  are the overclaiming the new prompt is meant to stop.

## Checked on 2026-11-26

- **Delete this brief / older briefs:** the owner tested both on vertex; they work.
- **P3 measured, nothing to change:** `GET /admin/api/system/storage` on vertex: `briefs.json` 104,578 bytes, `briefs-pool.json` 1,040,944
  bytes, `watches.json` 94,116 bytes. Rewriting 100 KB whole per change and 1 MB per collection (every 2 h in a window) costs nothing; issues
  are bounded (30 days, 100).
- **More stories with summaries (v0.138.0):** the evening brief of 25 Nov (issue 9) had 8 of 14 stories with a summary (3, 1 and 4 before).
  The morning brief of 26 Nov (issue 10) had 2 of 14: its lead events came from news searches, which give headlines only, and the model
  chose them over feed stories with text. The candidate places held for stories with text (SPEC-013 1.32) work; the model is not bound to
  pick them.
- **Do the watch's verdicts steer it? (ADR-0088)** The owner's verdicts were given on watch 1 for the test (Not for me on alerts 23, 26 and
  27; Should have matched on left-out post 38); "Check now" found nothing new, so the stack's own `watches.Analyst` was run on six made-up
  posts against vertex's Qwen3.6 35B A3B, with and without the verdicts. Over 10 rounds each (one call with the verdicts lost to a 502), the verdicts made no measurable difference: a post like the skipped animation film was shown in 2 of 9 runs with them and 3 of 10 without; a post like the wanted "Opus 5.6??" one in 8 of 9 with and 8 of 10 without; the chess-like post 0 of 9 and 1 of 10; the controls (an open-weights release, a meme) were right every time. The look-for text already decides these posts for this model, so ADR-0088 stays proposed: steering is not shown. If it is wanted, the verdicts need a stronger place in the prompt, measured the same way (harness: `watches.Analyst` with a `/v1` model, inputs with and without `Want`/`Skip`).

## Left, in order

1. **Owner, needs new posts or a browser:** no "Changed" without a citation; headlines of user posts say they are posts (the 36 posts left
   out since the deploy were all named as posts, memes or non-release news); the dot clears when alerts are read; "Write a new one" shows its
   steps; a real Telegram send (no edition has `send_to` set).
2. **Parked by the owner (future work):** an https address so the Telegram summary can carry a link (since v0.141.0 the link is
   `/admin/dashboard?board=newsroom`); Telegram channels as brief sources (the shared slot keyed by channel, `stacks/newsroom/brief/types.go`);
   keyed sources (Reddit, Bluesky, X) when the owner brings credentials; email as a channel (`../cross-cutting/email-connector.md`); held
   notices as a brief section (`attention-policy.md`, D9 of Briefs).

## Traps

- A control beside a link needs its own 44 px box; padding that overlaps the link loses the tap (learnings, 2026-11-25).
- The Following tab opens on "In your brief"; the watches are on `?kind=alerts`. Link there with `toFollowing('alerts')`. Since v0.141.0 the brief and the feed are read on the dashboard (`toBrief`, `toFeed`) and the Newsroom page is Following and Settings (ADR-0089).

## Start here

`frontend/src/views/NewsroomView.vue` (tabs and the Alerts view), `stacks/newsroom/newsroom.go`, SPEC-013 (`docs/SPECS/newsroom.md`).
