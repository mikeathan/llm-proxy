---
id: ADR-0064
title: The dashboard shows and links; the stack pages control
status: accepted
date: 2026-11-19
subsystems: [admin-ui]
supersedes:
superseded_by:
---

# ADR-0064: The dashboard shows and links; the stack pages control

## Context
Each stack (Network, Radio) had one page with its views, its decisions and its settings together, and the app had no one place to see how the
system was. The owner asked (2026-11-18) for a control center that could hold anything: the host and models, the assistant, automations,
reminders and every stack, as cards that can be sized and arranged, with a ready-made view per stack. Evidence: the plan
`cross-cutting/control-center-dashboard.md` (mocks A to E, the component audit), ADR-0022 and ADR-0028 (a stack keeps its settings and its
Modules tab), ADR-0061 (only the core joins Network and Radio), Constitution IV.6.

## Decision
1. **Dashboard, boards, cards.** The Dashboard is the page, boards are its tabs, cards are the tiles. Home mixes cards from any source; a stack
   ships a board of its own cards. There is one card engine (one catalogue, one grid, one set of sizes); a board only places cards.
2. **Cards show and link; they never act.** A card shows a verdict, figures or a list, and its arrow and its links open the stack page's
   control that acts on what it shows. Switching a module, starting a listen, naming a device and accepting a finding stay on the stack page
   (Modules tab, Listening, Devices, Findings, and the one **Claim your devices** section). A stack page never repeats a view a card shows, and
   says where it went with a link to its board.
3. **The layout belongs to the server**, so it follows the owner to every browser (`/admin/api/dashboard`). Sign-in is one passphrase, so this
   is one layout. The board open in a browser is a per-browser preference.
4. **Joined facts come from the core.** A card that shows what Network and Radio know together reads the core's join (ADR-0061); the browser
   never joins.
5. **No new dependency**: container measurement, drag and the grid are hand-written.

## Consequences
- One place to look; each view is built once and shown at three sizes. A card is added by one catalogue entry.
- Two levels of customisation (boards, then cards). Good shipped boards are the answer: most people only edit Home.
- Network's Devices and Findings and Radio's Listening, My devices and Sensors remain pages, because they hold decisions.
- A card that needs a poller must share it (`useNetworkSnapshot`), or a board of six cards polls six times.

## Alternatives considered
- **A page per stack with panels (mock A).** Keeps the views where they were, so the system still has no one place to look.
- **A device-first dashboard (mock C).** Rejected by the owner: the dashboard is for anything, not for devices.
- **Decision buttons on cards.** Breaks "cards never act" and puts passphrase-gated actions on a page whose sizes change.
- **Layout per browser.** Does not follow the owner to a phone; sign-in has no user accounts to key it by, so the server holds one.

## Links
- Plan: `docs/PLANS/cross-cutting/control-center-dashboard.md`; handoff: `control-center-dashboard-handoff-2026-11-19.md`.
- SPEC-003 2.67; code: `frontend/src/components/dashboard/`, `frontend/src/domain/dashboard.ts`, `backend/internal/core/dashboard/`.
