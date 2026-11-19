---
id: ADR-0065
title: A dashboard card's height is a minimum, its order is the order seen, and a stack's cards are offered while the stack is on
status: proposed
date: 2026-11-19
subsystems: [admin-ui]
supersedes:
superseded_by:
---

# ADR-0065: A dashboard card's height is a minimum, its order is the order seen, and a stack's cards are offered while the stack is on

## Context
The owner tried the dashboard on vertex (2026-11-19) and found the card controls wrong: making a card taller or wider made the next card
bigger, and "earlier" and "later" did not seem to move a card. Two causes in `DashboardGrid`: a 2x card was a grid `grid-row: span 2` on
auto-sized rows (the rows grew to fit it, and every card in them stretched), and `grid-auto-flow: row dense` let a later card fill a gap, so
moving a card one place in the list often changed nothing on screen. The whole cell was also `draggable`, which competes with the card's own
buttons. The same session asked for the handoff's remaining slices: the Timeline, Traffic and My devices views as cards, one device list, only
the cards of a stack that is on, and cards for the assistant and agents. Evidence: ADR-0064, plan `ARCHIVE/cross-cutting/control-center-dashboard.md`
(the component audit), ADR-0061, `GET /admin/api/stacks` (already lists every stack with its counts).

Status is `proposed` until the owner has read decision 6 (the device lists): it is accepted when they merge the branch, as ADR-0061 was.

## Decision
1. **Height is 1x (as tall as the content) or 2x (at least 28 rem).** It is never a row span, so resizing one card cannot change another.
   A fixed 2x height was tried first and measured: the radar's content is 960 px, so a fixed 28 rem box gave the owner's main Radio view a
   scroll of its own. A minimum keeps all of a long card, and a short card gets the room.
2. **Cards flow in board order, with no gap filling.** "Earlier" and "later" move a card one place on screen. Focus stays on the button that was
   pressed (or moves to the other when the end is reached). Only a grip on the card starts a drag.
3. **A stack's cards and board are offered only while the stack has a module on**, from the list the server already keeps (`GET /stacks`, no
   new interface). A card already on a board stays and shows its own state. When the list cannot be read, everything is offered.
4. **A card in view polls; one out of view waits.** The frame holds the one `IntersectionObserver`; `usePolling` reads it.
5. **A stack page that moved a view links to its card (`?card=`)**, and when the owner's saved board does not have the card yet the dashboard
   says so with "Add it". The stored layout contract is unchanged; no card is added behind the owner's back.
6. **The four device lists are not merged.** They answer different questions (see the table in the plan): the Network list decides, the
   nearby list is about devices that are not in the inventory, the traffic rail is a picker weighted by bytes, My devices is presence of the
   owner's marked devices. One read-only **Devices** card (the inventory, with the core join's presence and what Radio heard) joins the
   catalogue. Nothing is retired. **This deviates from the plan's proposal and needs the owner.**

## Consequences
- A saved 2x keeps meaning "tall": the radar (2x on the shipped Radio board) is as tall as its content, as before (measured in `e2e/radio.visual.spec.ts`). On a card whose content is already taller than 28 rem, 2x changes nothing; that is the price of never scrolling inside a card.
- A page that links to a card must name the card in `BOARD_HINT`; a card moved off a page needs the page's link text changed with it.
- A card built from several Panels must `provide(IN_DASHBOARD_CARD, false)` for its subtree (pitfall 74).

## Alternatives considered
- **Keep the row span and set `align-items: start`.** Rejected: rows still grew to the tallest span, so a neighbour's row moved.
- **A fixed 2x height with the card scrolling inside.** Tried and rejected: it clips the radar (content 960 px) behind a scrollbar of its own.
- **A drag library or a grid-layout package.** Rejected: ask-first, and arrows are the accessible path.
- **Add moved cards to every saved board on load.** Rejected: it would change a layout the owner arranged.
- **One device row for all four lists.** Rejected: it would drop facts or put controls on the dashboard (ADR-0064).
