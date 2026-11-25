---
id: ADR-0087
title: Left-out alerts stay in the journal with their own bound, and a deleted brief keeps its slot
status: proposed
date: 2026-11-25
subsystems: [autonomy]
supersedes:
superseded_by:
---

# ADR-0087: Left-out alerts stay in the journal with their own bound, and a deleted brief keeps its slot

## Context

On vertex (2026-11-25, read-only GETs) the one watch "AI news" had 13 alerts, 10 unread: only clicking an alert's link marked it read, and nothing
outside the Alerts tab said alerts were waiting. Five of the 13 were marked "Changed" with a made-up earlier fact ("was: unknown", "No new models
found"). Each check left six or so posts out as not relevant, and the only trace was a cut-off line on the watch's row. The owner also asked for a way to
delete old briefs; written briefs only aged out (30 days, 100 at most).

## Decision

1. **A change cites its earlier fact.** The model's `changes[]` entry carries `of`: 0 for the last known state (only when one was shown) or the number
   of a recently reported title. Code drops a change that cites nothing or something not shown, the same way it checks a repeat's citation. The
   reason text is still never read.
2. **Left-out items are journal entries marked `left_out`,** with the model's reason in `why`, kept 7 days and at most 50 per watch, capped apart from
   the findings so they never push one out. The feed, its unread count and the brief's watches block skip them; one flag on the feed route lists them;
   "Should have matched" puts one into the feed, read. Teaching the filter from that call is not built.
3. **One unread count route** (`GET .../feed/unread`) feeds one shared poller for the Alerts tab and the sidebar dot, like the brief's dot.
4. **Deleting a brief keeps its slot.** `Made` (the scheduler's "already written for this ready time") also reads a list of deleted slots, kept for
   the issue retention, so a deleted brief is not written again. The already-shown record is kept, so the next edition does not repeat its stories.

## Alternatives

- A separate file for left-out items: a second store and lock for data that already has the journal's shape and retention rules. Rejected.
- Showing left-out items in the feed, greyed: they would count as unread or need a second read rule, and the feed would fill with what the owner
  asked to filter. Rejected.
- Letting a deleted brief be written again at its next tick: deleting today's brief would bring it back a minute later. Rejected.
- Telling the model which items were wrongly left out ("should have matched" as an example in the prompt): a prompt change; left for the owner.

## Consequences

- `watches.json` grows by up to 50 short entries per watch; `briefs.json` by one small record per deleted brief for 30 days.
- A model that leaves out what it should have kept is now visible, and one click undoes it.
- SPEC-013 1.29.

## Evidence

- `stacks/newsroom/watches/analyst.go` (`rawChange.cites`), `journal.go` (`LeftOut`, `capEntries`, `Keep`, `UnreadCount`), `runner.go` (`keepAside`),
  `brief/issues.go` (`DeleteIssues`, `DeletedSlot`), tests `TestAChangeMustCiteWhereTheEarlierFactWasWritten`,
  `TestWhatTheModelLeftOutIsKeptAsideWithItsReason`, `TestLeftOutEntriesHaveTheirOwnBound`, `TestStore_DeleteAnIssueOrTheOlderOnesOfItsEdition`
  (each fails when its rule is removed).
- Plan: `docs/PLANS/ARCHIVE/autonomy/newsroom-review-2026-11-25.md` (Alerts follow-up).
