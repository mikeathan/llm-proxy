---
id: ADR-0088
title: A watch learns from the owner's verdicts as a short list of example titles sent with what it looks for
status: proposed
date: 2026-11-25
subsystems: [autonomy]
supersedes:
superseded_by:
---

# ADR-0088: A watch learns from the owner's verdicts as a short list of example titles sent with what it looks for

## Context
A watch's model judges each new post against the owner's "look for" text (ADR-0073). On vertex it let through memes, promos and user posts the owner
did not want, and left out a few they did (ADR-0087 keeps those under "Left out by the model" with "Should have matched"). The owner had no way to
correct the filter except rewriting the "look for" text. Owner answers (2026-11-25): a "Not for me" button on alerts, yes; keep the newest 10 of each
kind; list them in the watch's form, each removable.

## Decision
We will keep, on each watch, two short lists of post titles the owner judged: `examples.want` ("Should have matched") and `examples.skip` ("Not for
me"), newest first, at most 10 each, a title in one list only (the newest verdict wins). `Store.Teach` records a verdict: it moves the entry (into
the feed, or out of it into the left-out list with the reason "You said: not for me"), marks it read and updates the lists. The analyst prompt sends
both lists in their own fenced blocks beside WHAT THE PERSON WANTS and the system text says to treat a new item like the posts it resembles and to
follow the person when the examples disagree with the model's own reading. A watch with examples but no "look for" text is judged too, so the button
works on every watch. The watch form shows the lists and saves them back, so removing one forgets it.

## Consequences
- The filter improves from two clicks, with no prompt for the owner to write; the prompt grows by at most 20 short lines.
- Example titles are untrusted internet text fed back into a prompt: they are fenced, one bounded line each, and only the owner can add one.
- A watch saved by the form replaces its lists with what the form sent: any other writer of `PUT /watches/{id}` must send them too.
- Not measured yet: whether the local model follows the examples. The first check is on vertex after deploy.

## Alternatives considered
- **Rewrite the "look for" text from the verdicts with the model** — a model would edit the owner's own words; rejected (one path the owner controls).
- **Skip words or keywords learned from verdicts** — cheap, but a title shares words with the posts the owner wants; titles as examples keep the meaning.
- **Unbounded history** — grows the prompt on a small local model; 10 of each was the owner's choice.

## Links
- SPEC-013 1.33; `stacks/newsroom/watches/watch.go` (`Examples`, `Judges`), `journal.go` (`Teach`), `analyst.go` (`examplesBlock`); routes
  `POST /admin/api/newsroom/feed/keep` and `/feed/skip`; `frontend/src/components/newsroom/NewsroomEntryItem.vue`, `NewsroomWatchExamples.vue`.
- Plan: `docs/PLANS/autonomy/newsroom-handoff-2026-11-25.md`.
