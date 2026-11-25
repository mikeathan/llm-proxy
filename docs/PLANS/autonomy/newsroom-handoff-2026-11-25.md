---
status: partial
date: 2026-11-25
last_reviewed: 2026-11-25
related_specs: [SPEC-013, SPEC-003]
constitution_references: [IV.6, V.2]
related_plans: [../ARCHIVE/autonomy/morning-paper.md, ../ARCHIVE/autonomy/topic-watch.md, ../ARCHIVE/autonomy/newsroom-review-2026-11-25.md, ../cross-cutting/email-connector.md, attention-policy.md]
evidence: Read of vertex (v0.135.0, commit 90a08290) through the admin API on 2026-11-25 16:42-16:43 UTC, GETs plus one "Check now" on watch 1; GETs of /version and the brief issues on vertex (v0.138.0) at 18:52 UTC.
---

# Newsroom: what is left after v0.138.0 (handoff, 2026-11-25)

**Status:** partial. The Newsroom plans are built and archived: Topic Watch (the Alerts), Briefs (was Morning Paper) and the 2026-11-25
review (`../ARCHIVE/autonomy/`). SPEC-013 is the contract. This page holds only what is left.

## Branch and commit state

- #308 to #312 are merged and deployed; vertex reports v0.138.0 (commit 2e72cdcc, built 2026-11-25 18:42 UTC).
- **v0.139.0 and v0.140.0, branch `newsroom-copy-fix-and-compact-page-heads`, PR #313 open:** the brief's index row no longer
  shows the "j and k move between stories · o opens one" hint (the owner: no instructions on the page; the keys still work), and "Copy as text" goes through `utils/clipboard.ts` `copyText`, so it works on plain HTTP (vertex.local had no
  clipboard API, so the copy always failed there). Plus the
  compact page heads ([`../ARCHIVE/cross-cutting/compact-page-head.md`](../ARCHIVE/cross-cutting/compact-page-head.md), SPEC-013 1.34): a
  small outline Follow, the brief head on two lines with ⋯ for Write and Delete, and the dashboard behind Edit (SPEC-003 2.75).

## Checked on vertex (2026-11-25, after the v0.135.0 deploy)

- **The scheduled brief is full:** Morning issue 8 (06:40 UTC) has 4 top stories, Technology (topics 1 and 2 share the section, by
  category) and News with 5 stories each, 3 Home lines and 3 model calls. Evening issue 6 has the same shape.
- **"Left out by the model" fills:** 4 entries from the 16:30 run, each with its reason.
- **Nothing to judge yet on the new alert prompt:** "Check now" on watch 1 at 16:43 returned `nothing_new`. The two alerts of 25 Nov
  ("Google Releases Gemini Neon for Empathy Tasks" from a Reddit post titled "Gemini Neon on Empathy") were written before the deploy and
  are the overclaiming the new prompt is meant to stop.

## After the deploy, on vertex

1. Mark two or three alerts "Not for me" and one left-out post "Should have matched"; after the next check, read the watch's prompt result: do
   similar posts follow the verdicts? (ADR-0088 is proposed until this is seen.)
2. The brief: the next edition should carry more stories with summaries (the Guardian, BBC, Sky feed stories reach the model now).

## Left, in order

1. **Owner:** the two vertex checks above. On 2026-11-25 18:52 UTC the latest issues (8, 7, 6) were all written before the v0.138.0
   deploy (3, 1 and 4 of 14 stories with a summary), so check 2 waits for the next edition.
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
