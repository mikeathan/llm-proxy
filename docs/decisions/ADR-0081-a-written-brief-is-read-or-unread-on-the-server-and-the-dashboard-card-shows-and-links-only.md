---
id: ADR-0081
title: A written brief is read or unread on the server, one poller feeds every dot, and the dashboard card shows and links only
status: proposed
date: 2026-11-23
subsystems: [autonomy]
supersedes:
superseded_by:
---

# ADR-0081: A written brief is read or unread on the server, one poller feeds every dot, and the dashboard card shows and links only

## Context

The owner asked for the Newsroom's brief on the dashboard, big when there is one, and a small mark on the tab while a brief is written and not yet read (2026-11-23). Nothing recorded whether an edition had been read. The owner reads on the computer and on a phone, so a dot kept in one browser would be wrong on the other. ADR-0064 says a dashboard card shows and links; ADR-0070 says pages control.

## Decision

- **Read state is the server's.** `Issue.read_at` is set once by `POST /briefs/issues/{id}/read` (idempotent; the first time stays). `GET /briefs/latest` returns the newest issue of each edition that still exists with a `read` flag, so a dot never asks for the editions themselves. Only the newest issue of an edition counts: a morning paper missed is not a pile owed. Issues made before this change have no `read_at`, so the newest of each shows as unread once.
- **The Briefs page marks it read**: an edition on screen, in the read view, on a visible page. The dashboard card never does: it shows the edition and links to the page, so a dot stays until the brief is opened where it is controlled.
- **One poller.** `useBriefUnread` holds the state at module level and polls every 30 s while any component that uses it is mounted and the page is visible; the Newsroom page's tab, the dashboard's Newsroom tab, the edition picker and the card all read the same list. A server that cannot answer (module off, stack stopped) shows no dot and no error.
- The card is a card of the existing catalogue (`newsroom-brief`, 12 columns), placed above the headlines card on the Newsroom board; a saved board is offered it by "New on this board". It has four sizes (one line, lead and count, the full masthead with the lead and top stories and a chip per section, and "Show every section" opening the whole edition inside the card).

## Consequences

One more route pair in the stack manifest (`TestManifestDeclaresEveryBriefRoute` guards it). The dot clears on any screen that shows the page. Opening the page counts as reading even if the owner scrolls past; the card's own expand does not count.

## Alternatives considered

- **Keep it in the browser (localStorage)** — wrong on the second device, lost with the browser's data.
- **A dot per issue, a count of everything unread** — a missed edition would nag for a month; the newest is what the owner wants to know about.
- **The card marks it read when expanded** — breaks ADR-0064 and makes the dot disappear without the page being opened.

## Links

- SPEC-013 1.20; `stacks/newsroom/brief/issues.go`; `frontend/src/composables/newsroom/useBriefUnread.ts`; `NewsroomBriefCard.vue`.

## Amendment (2026-11-25): tabs in place and a Today card on Home

The large card no longer expands the edition at its foot (the control and the section chips both did that off-screen). A tab list switches the body in place, nothing opens elsewhere, and the card still only shows and links. A short `newsroom-today` card joins Home's shipped layout, because the brief now mixes news, reminders, the home and the service; reading it in full stays on the Newsroom board. Rejected: moving the brief to Home only (the Newsroom board is where the whole thing is read), and show-and-link with no tabs (the owner chose tabs).
