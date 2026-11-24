---
id: ADR-0084
title: The Home block ranks its facts by a level the core sets and folds when nothing needs a look
status: proposed
date: 2026-11-24
subsystems: [autonomy]
supersedes:
superseded_by:
---

# ADR-0084: The Home block ranks its facts by a level the core sets and folds when nothing needs a look

## Context

The owner found the Home block of a brief boring: every fact was a count and a full sentence in the order the core produced them, so a routine "16 devices online" looked like a warning and a real warning looked routine. The Briefs page also spent about 450 px (four stacked rows) above the brief, and on a phone the Home block sat under every section. The owner asked for a Home panel that shows what needs a look, links to where it is acted on, shows something serious where it will be seen, and works on a phone and in Telegram. ADR-0077 fixed `HomeLine` to `ID, Count, Words` and its test fails on a fourth field, so a ranking needs either a table kept in two places or an amendment.

## Decision

1. **`HomeLine` gains one field, `Level`** (amends ADR-0077): one of `critical`, `check`, `info`, set by the core from one table keyed by id (`stackport.homeLevels`) and never from anything a device, a frame or a stack sent. A fixed word, not text. A line without one (an edition written before levels) is read as routine. A test holds the table to every id the core can word, so a new fact cannot be left unranked, and the reflecting test now lists the four fields.
2. **Critical is kept to one fact on purpose:** an access point of the owner's that is open or uses an outdated protection (`radio_open_or_weak_ap`). A device the owner has not marked is `check`, not critical, because a new install has many; a rogue twin is `check` because the words themselves say it may be a neighbour's. Widening the set is one table row and a test.
3. **The core ranks, the stack stores, the screen shows.** `Lines` sorts critical, check, info (stable) before the cut at sixteen, so the cut never drops an urgent line.
4. **The block folds when nothing needs a look** ("All quiet"), and opens with the lines to check first, each one a link to the page where it is acted on (a Radio fact on Radio, everything else on Network); routine facts sit below, quietly. A critical line is red, the block gets a red outline, and the dashboard Brief card says "Needs attention" in its header at every size.
5. **The phone message** puts a critical fact first, right under the brief's name with a warning mark, and not again under "At home". The level comes from the core's table by id, so a stack cannot make a line look critical on a phone.
6. **Layout.** The Briefs reading page is one slim row (views, edition, "Write a new one now"); "Delete all briefs" moves to the setup view where the things it deletes are; today's brief is one quiet line, not a card (an old or missing one stays a card); "Earlier briefs" sits in the masthead. On a phone the side column (Home, Your day, Your watches) falls between the top stories and the sections.

## Alternatives considered

- **A second table of ids and levels in the frontend** — two places to keep, and the phone message would need the core's anyway.
- **Derive the level from Radio's tier** — the tier is how sure the evidence is (seen, inferred, estimated), not how serious the fact is.
- **Several critical facts at the start** — the owner found a louder panel "too much"; one fact that is serious on its own is easier to widen than to narrow.
- **A banner and a pop-up for critical** — rejected by the owner as too much; a red line, a red outline, a header tag and the phone's first line carry it.

## Consequences

- Easier: a quiet day is one line; a bad one is at the top of the page, the card and the phone message.
- Harder: `Level` is a fifth thing the stack's issue JSON carries (already stored as is, so no migration); the ids' levels are the owner's to approve (Constitution I.16 wording rule stands: counts and the core's words only).
- Not done: a sidebar dot on Network for a critical fact, chips for failed automations and blocked delivery (no ready count to read), shorter radio sentences for the phone, deep links to a single finding.

## Links

- SPEC-013 1.23; `stacks/sdk/ports.go` (`HomeLine`, `HomeLevel*`), `backend/internal/core/stackport/home.go` (`homeLevels`, `HomeLevel`), `notice.go` (`briefHomeLines`); `frontend/src/components/newsroom/NewsroomEditionHome.vue`, `NewsroomEdition.vue`, `NewsroomBriefs.vue`, `utils/newsroom/homeLines.ts`, the dashboard `NewsroomBriefCard.vue`; ADR-0077, ADR-0083.
