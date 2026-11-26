---
id: ADR-0086
title: The Newsroom opens on the brief, and every card is a front page that links into it
status: proposed
date: 2026-11-25
subsystems: [autonomy, admin-ui]
supersedes:
superseded_by:
---

# ADR-0086: The Newsroom opens on the brief, and every card is a front page that links into it

## Context

After v0.131.0 the same brief was drawn three times, each with its own code and its own pollers: the Today card on Home, a big Brief card on the
Newsroom board (by then a second reader with tabs, a side column and Model use), and the Briefs tab. Every link from a card went to the top of
the Briefs tab, without the edition, so picking an edition on a card and opening it could show (and mark read) another one. The Newsroom opened
on the Feed, and a watch and a brief topic were set up in two tabs. The review is in
`docs/PLANS/ARCHIVE/autonomy/newsroom-review-2026-11-25.md`, with mocks in `newsroom-redesign-mocks/`. The owner chose option C, built as B first,
without the generated halftone art.

## Decision

1. **The Newsroom page is a reader.** It opens on the brief (`/newsroom`). The tabs are **Brief · Alerts · Following · Settings**: Alerts is
   the old Feed, and Following holds what is followed, the brief's topics and editions ("In your brief") and the watches ("Alerts"). The old
   `/newsroom/briefs` (already sent in phone summaries) and `/newsroom/watches` redirect; the core's summary links to `/newsroom`.
2. **The brief is laid out as a paper**: a nameplate (the name and one mono dateline, no heavy rules), the lead and the other top stories, every section
   open, an end mark with Print and Copy as text. A sticky index of the sections is one row of links under the nameplate at every width
   (amended 2026-11-25: a left column of its own left most of a wide page empty, and the Brief tab now uses the whole page width). Beside the news, in reading order: what needs a look (critical home facts, said once), the chips, your day, alerts, the rest of Home,
   Model use. Model use no longer depends on the Home switch (amends ADR-0085's placement). Each story shows a tick per outlet.
3. **Cards are front pages and only link (ADR-0064 held).** The Brief card shows what needs a look, the lead, two headlines from each of up to
   three sections and the sections as links; no tabs. (Amended 2026-11-25, v0.135.0: as many columns as sections, up to three, with more
   headlines when there are fewer, the edition named once, a section label links to its section, and the footer lists only the sections without a column.) The Today card keeps its short form. Every headline, section and "read" link opens the
   brief at that place: `/newsroom?edition=&section=&story=` (`toBrief`). Cards share one store for the editions and each written issue,
   one for the chips and one for Model use (`sharedPoll`), so a board asks once however many cards show it.
4. **One way in to follow.** The Follow form on Following makes a brief topic, a watch, or both, from one set of words; a story offers more or less
   like this (a search or skip words added to its topic, the words picked by the owner) and Follow this story. The stored topic and watch stay as
   they were (1.28).
5. **No pictures of the news.** The line-art section marks stay; no generated or remote image is drawn for a story.

## Consequences

- One reader, reached the same way from everywhere; a link says what it points at, so the dot clears for the edition the owner opened.
- The board's Brief card is shorter and no longer duplicates the page; reading in a card is gone.
- Old links keep working through two redirects that can go once no summary older than this release is kept.
- The Briefs "Set up" view moved to Following; tests and the visual spec follow it.
- ADR-0066 keeps stack pages for control and the dashboard for seeing; reading a brief was already on the Newsroom page (ADR-0080), and this
  keeps it there as the one place to read, with the dashboard showing a front page that links in. This is an exception for reading, not a
  move of controls to the dashboard.

## Alternatives considered

- **A · the card is the reader** — cheapest, but two places to read a brief, against ADR-0064 and "one path per job".
- **Halftone art per story** — mocked; the owner rejected large images.

## Links

- Plan: `docs/PLANS/ARCHIVE/autonomy/newsroom-review-2026-11-25.md`
- SPEC-013 1.27

## Amendment (2026-11-25): the exception for reading ends

ADR-0089: the reader and the feed (renamed from Alerts) moved to the dashboard's Newsroom tab as page boards; the Newsroom page keeps Following and Settings and opens on Following. Decisions 2 to 5 stand on the dashboard; decision 1's tabs are Following · Settings, and `toBrief` opens `/dashboard?board=newsroom`.
