---
id: ADR-0089
title: A stack's page is for setting it up; everything to read, see or decide about an item is on its dashboard tab, with no exception for reading
status: accepted
date: 2026-11-25
subsystems: [admin-ui, autonomy]
supersedes:
superseded_by:
---

# ADR-0089: A stack's page is for setting it up; everything to read, see or decide about an item is on its dashboard tab, with no exception for reading

## Context
ADR-0066 moved everything to see from the Network, Radio and Agents pages to the dashboard and left their pages for control. The Newsroom kept its
reader and its feed on its own page, as "an exception for reading" (ADR-0086), because the alternative then was a second reader inside a card. After
v0.140.0 the owner asked why the Newsroom differs from the other stacks (2026-11-25) and set the rule for every stack, present and future: *"stack
page is for any form of configuration, setup, tweaking and modules of course"*. They chose (mocks in
`docs/PLANS/ARCHIVE/autonomy/newsroom-on-dashboard-mocks/`) that the menu's Newsroom opens the stack page like every other stack (option 1A, not a
menu entry that opens the dashboard), that the brief keeps its layout exactly, and that "Alerts" is called **Feed**.

## Decision
1. **A stack's page holds configuration, setup, tweaking and its modules, and nothing to read.** Two kinds of control stay there too: an action
   that needs the passphrase (Radio's Listening, Agents' Lock down), and writing or editing the things a stack acts on (the Newsroom's watches,
   topics and editions on Following; Reminders' reminders).
2. **Everything to read, see or decide about one item is on the stack's dashboard tab**, as cards or, when the thing is one page to read, as a
   **page board**: a shipped board template with a `view` (`DashboardBoardTemplate.view`, `components/dashboard/boardViews.ts`) that draws one
   component in place of the grid. A page board has no cards to add, move or reset; the stored layout and the server are unchanged.
3. **The menu entry of a stack opens its stack page.** The dashboard's tab is the way to read; the stack page links to it ("Open the Newsroom
   tab"), and each view links back to where it is set up.
4. **The Newsroom** is one dashboard tab with two page boards: **Brief** (`newsroom`, the reader, `NewsroomBriefs` unchanged and full width) and
   **Feed** (`newsroom-feed`, what the watches found, at the usual page width, with Keep, Not for me and Clear as before). Its stack page has
   Following (the page without a tab) and Settings. The sub-tabs carry the unread dots; the sidebar's dot moves from Newsroom to Dashboard.
   `toBrief` and the core's phone summary link (`stackport.BriefPagePath`, now with the `/admin` prefix it lacked) open the Brief board.

## Consequences
- ADR-0086's exception for reading ends; its reader, its cards as front pages and its links (`?edition=&section=&story=`) stand, on the dashboard.
- ADR-0081: the Brief board is the reader, so it marks an edition read; a card still never does.
- A future stack ships its views as cards or page boards and a page with Modules and Settings (`docs/architecture/checklists.md` → Adding a stack).
- On a phone the first headline starts about 70 px lower than on the old Newsroom page (the dashboard's tabs and the sub-tab row sit above the
  masthead; 343 px from the title, measured in `e2e/newsroom-briefs.visual.spec.ts`). Printing a brief no longer prints the page head and tabs.
- Old links to `/newsroom/alerts` show Not found (no redirect, as the owner removed the earlier Newsroom redirects); `/newsroom` opens Following.

## Alternatives considered
- **The menu's Newsroom opens the reader on the dashboard (1B)** — one click less to read, but a stack entry that lands on the dashboard is what
  was reverted for Network in v0.108.0, and the owner rejected it again.
- **The brief as a card of the grid** — the card frame, sizes, Pin and Remove would wrap and break the paper layout; a page board keeps it exact.
- **A "Following and settings →" link at the end of the sub-tab row** — mocked and built, then dropped: each view already links to its setup, and
  the extra row pushed the first headline down on a phone.

## Links
- Plan: `docs/PLANS/ARCHIVE/autonomy/newsroom-on-dashboard.md`; SPEC-003 2.76, SPEC-013 1.35.
- Code: `frontend/src/components/dashboard/boardViews.ts`, `frontend/src/views/DashboardView.vue`, `frontend/src/components/newsroom/NewsroomFeedView.vue`,
  `frontend/src/views/NewsroomView.vue`.
