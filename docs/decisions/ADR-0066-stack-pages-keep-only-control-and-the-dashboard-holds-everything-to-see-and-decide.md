---
id: ADR-0066
title: The stack pages keep only control; the dashboard holds everything to see and to decide about an item, for a stack that is on
status: proposed
date: 2026-11-19
subsystems: [admin-ui]
supersedes:
superseded_by:
---

# ADR-0066: The stack pages keep only control; the dashboard holds everything to see and to decide about an item, for a stack that is on

## Context
After v0.106.0 the Network page still had six tabs and Radio four, so the owner met the same facts on a stack page and on a card, and
asked (2026-11-19) for the stack pages to keep only what is relevant to a stack and everything else to be on the dashboard, shown only for a
stack that is enabled, with links and back buttons that still work. The same session reported a gap under short cards (a tall card made its
whole row tall, and moving the bottom card pulled the next big one down). Evidence: ADR-0028 (Modules is the one place to switch), ADR-0064,
ADR-0065, `router/index.ts`, `components/dashboard/cardRegistry.ts`.

## Decision
1. **A stack page is Modules and Settings, plus what needs the passphrase.** Network: Modules, Settings. Radio: Listening (starting a run asks
   for the passphrase), Modules. Agents: Lock down, Modules. (Amended 2026-11-19, v0.111.0: Radio's Sensors tab became the card "Radios on this computer" and Agents' Activity tab became the Agent activity card on a new Agents board; `/radio/sensors` redirects to its card.) Router setup moved to Network Settings → Watching.
2. **Everything to see is a card.** Network: Devices, Findings, Audits, Nearby, Trace a route, Traffic by device (with the look). Radio: Claim and
   manage devices, Your Wi-Fi networks (My devices and the radar were already cards). The shipped boards hold them.
3. **A card may carry the decision about one item** (name, approve or forget a device; accept a finding; claim, add or remove a radio
   device). ADR-0064's "no switches or settings on the dashboard" is unchanged: a module switch, a limit and a setup step stay on the stack page.
4. **A stack that has no module on has no menu entry, no board and no cards** (the saved layout is kept, so they return with the stack).
   One shared read of `GET /stacks` feeds the menu and the dashboard. An "Add a stack" entry (`/stacks`) shows only while one is off, as the way
   to its Modules page; an unknown list shows everything.
5. **Network and Radio in the menu open their own page** (Network: Modules; Radio: Listening), which looks like a stack page, not like the
   dashboard. (First built as "the menu opens the board"; the owner found a stack that looked like the Dashboard, with the board tabs and
   the new-cards banner, wrong on 2026-11-19, and it was reverted in v0.108.0.) The stack page's "Open the board" link and the dashboard's
   Network and Radio board tabs are the way to the cards. The old tab addresses (`/network/devices|findings|traffic|audits`, `/radio/devices`) redirect to their card and keep `?finding=` and `?device=`.
6. **A board saved before a card existed is told once** ("New on this board: …", Add them or Not now, remembered per browser); nothing is added
   behind the owner's back. "Put every board back as shipped" is in the Add board menu, after a confirmation.
7. **The grid packs like bricks.** Rows are 4 px and a cell spans as many as its measured height needs (`ResizeObserver`); sparse placement,
   never `dense`. A short card leaves no hole, and moving a card changes only it and the cards after it. This replaces ADR-0065 decision 2
   ("no gap filling"); decision 1 (height is a minimum, never a span the owner sets) stands.

## Consequences
- Simple/Advanced, Help and the "what is missing before a scan" banner moved to the top of any board that holds a Network or Radio card.
- Pitfall 75 is rewritten. The browser spec asserts no hole under a short card.
- Rejected: a drag-and-drop grid library (ask-first dependency); keeping the tabs and hiding them (two ways to the same view).
