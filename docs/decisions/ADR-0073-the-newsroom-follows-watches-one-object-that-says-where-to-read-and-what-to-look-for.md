---
id: ADR-0073
title: The Newsroom follows watches, one object that says where to read and what to look for
status: proposed
date: 2026-11-20
subsystems: [autonomy]
supersedes:
superseded_by:
---

# ADR-0073: The Newsroom follows watches, one object that says where to read and what to look for

Proposed: in force when the owner merges the pull request that carries it. It changes how the Newsroom stack (SPEC-013) is shaped; ADR-0064, ADR-0070 and ADR-0071 and ADR-0072 stay accepted.

## Context
- The first Newsroom (v0.114 to v0.115) had a "topic" that was only a name plus sources. Nothing said what to look for, so a topic reported every new post of its sources. The model step compared items with earlier ones but was never told what the owner cares about.
- The page had five tabs (Feed, Topics, Sources, Settings, Help). Topics and Sources were two views of one object, Sources could not add anything, and Help was static text. The owner could not tell where to set anything up.
- A new topic recorded what was already there and showed nothing, so the feed looked broken on day one.

## Decision
1. **One object, a Watch**, renamed from "topic" everywhere (code, routes, file, tools, screens). It has a name, one to ten sources, optional `look_for` text (the owner's words, 300 characters), optional keywords and skip words (up to ten each), an interval, a channel and an on/off flag. The unused Search, Paper and Section fields are removed.
2. **Two-stage filter.** Keywords and skip words are a plain case-insensitive match that needs no model. What passes goes to the model in one batched call that is told what the owner wants and returns the items that match with a short reason. Items filtered out by either stage are recorded as seen so they are not judged again.
3. **Fail open.** A model answer with no "matches" list, a refused model call, or no model keeps every item (the row says "not compared"). Only a usable judgement can hide an item, and the row then names what it hid.
4. **Untrusted text.** `look_for` and every item are fenced in the prompt and cannot close their own block; the answer is checked against what was shown, as before.
5. **The first check shows a few findings.** A watch's first check still announces nothing, but puts up to five of the matching items it found in the feed, marked "First check". With `look_for` set and no model it shows none, because it cannot judge.
6. **The page has Feed, Watches and Settings.** Settings stays because ADR-0067 puts a stack's module switches there. Sources and Help are gone: the watch form carries the per-source guide and suggestions, and the Watches list shows each watch's status and what it looks for.
7. **Persisted names change with the rename** (`watches.json`, the `watches` key, `watch_id`). There is no migration shim; a watch made before this version must be added again.

## Consequences
- Easier: one place to set up what to follow; the feed carries only what the owner asked for and says why; the assistant's follow tool takes the same fields, so chat and screen are one path.
- Harder: a relevance judgement can hide an item (point 3 limits when, and the row names it); one more model call per batch of new items, within the existing daily limit.
- Cost: the owner's one existing watch on vertex has to be re-created.

## Alternatives considered
- **Keep topics and sources as two tabs** — the owner's own finding was that it does not make sense.
- **Keep the persisted names and rename the screens only** — less churn, but the code and the data would then disagree about what the object is.
- **Fail closed on a bad model answer** — would hide items on every model hiccup.

## Links
- `stacks/newsroom/watches/` (`watch.go` Gate, `analyst.go`, `runner.go` baseline and showFirst), `frontend/src/components/newsroom/NewsroomWatchEditor.vue`, SPEC-013, `docs/PLANS/ARCHIVE/autonomy/newsroom-review-2026-11-25.md`.
