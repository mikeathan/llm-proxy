---
id: ADR-0082
title: One story reworded is one candidate, and an outlet under two spellings is one outlet
status: proposed
date: 2026-11-24
subsystems: [autonomy]
supersedes:
superseded_by:
---

# ADR-0082: One story reworded is one candidate, and an outlet under two spellings is one outlet

## Context

The Morning brief on vertex (2026-11-24, issue 3) showed one event, the Nvidia board and its CEO, as four stories, and counted "The Guardian" and "The Guardian UK" as two outlets. The pool already merges stories by exact title key or link (ADR-0075), and an earlier check had found no placement bug: the repeats had different pool ids.

## Decision

- **Outlets are counted by `OutletKey`**: lower case, without a leading "the" and without a regional or web ending (` uk`, ` us`, ` usa`, `.com`, `.co.uk`). A story names each outlet once, under its first spelling.
- **Rewordings are folded when an edition is written, not in the pool.** `SimilarKeys` calls two title keys one story when they share at least four content words and at least 60% of the words of both together. Candidates are ranked first, then each is folded into the best-ranked similar one: reports add up, a text a headline lacked is taken, and the folded key counts as placed so a later section does not show it. The pool is untouched, so a wrong fold costs one edition, not stored data.
- **Yesterday's story is recognised under another wording.** An edition records only the shown story's key, so the already-shown rule also matches a similar key (the best-reported earlier story decides); without it the folded rewording would come back as new the next morning.
- **Different angles on one event are left to the model**: they share a name and little else, so wording cannot tell them apart. Both prompts now say that two stories about the same event count as one and only the best is chosen.
- Each issue stores `outlets_read` (distinct outlets over the recent pool of its news topics), and the masthead says "read from N outlets". Older issues have none and show nothing.

## Consequences

The four-headline Nvidia case is not fixed by wording alone: it depends on the model following the prompt. A small model may ignore it; the next vertex edition will show it. The thresholds (`minSharedWords`, `similarShare`) are one place to tune.

## Alternatives considered

- **Merge in the pool** — permanent, and a wrong merge loses a story for three days.
- **Embeddings or a model call per pair** — a call per candidate pair spends the daily model limit (D8) for a cleanup the prompt may already do.
- **Trim every " - Publisher" and "UK" in the reader** — the outlet name is the owner's evidence of who reported it; only the counting is normalised.

## Links

- SPEC-013 1.21; `stacks/newsroom/brief/similar.go`, `compose.go`; ADR-0075.
