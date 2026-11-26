---
status: complete
date: 2026-11-25
last_reviewed: 2026-11-25
related_specs: [SPEC-003, SPEC-013]
constitution_references: [V.2]
related_plans: [../../autonomy/newsroom-handoff-2026-11-25.md]
evidence: ADR-0064, ADR-0066, ADR-0070, ADR-0081, ADR-0086 read 2026-11-25; frontend/src/constants/dashboard.ts (BOARD_TEMPLATES, GROUP_LABELS); frontend/src/views/NewsroomView.vue; DashboardCardFrame.vue; stacks/newsroom/watches/journal.go (30-day, 200-per-watch bound).
---

# Newsroom on the dashboard: Brief and Feed as sub-tabs, like Network

**Status:** complete. Built in v0.141.0 on branch `newsroom-on-dashboard`; ADR-0089, SPEC-003 2.76 and SPEC-013 1.35 hold the result. Mocks: [`newsroom-on-dashboard-mocks/index.html`](newsroom-on-dashboard-mocks/index.html).

## Why

Network, Radio and Agents show everything to see on the dashboard and keep only control on their page (ADR-0066). Newsroom kept its reader on the
page as "an exception for reading" (ADR-0086), because the alternative then was a second reader in a card. The owner asked (2026-11-25) for the
Newsroom to follow the Network pattern: reading on the dashboard, with the brief laid out exactly as today, and setup and settings on the stack page.
With the reader moved rather than copied, there is still one place to read.

## Shape

- Dashboard › Newsroom becomes a group (`group: 'newsroom'`, `GROUP_LABELS`) with two sub-tabs: **Brief** (the reader, `NewsroomBriefs` as it is)
  and **Feed** (today's Alerts tab). Retire `newsroom-brief` and `newsroom-latest` from the board; Home's `newsroom-today` stays.
- The Newsroom page keeps **Following · Settings**.
- A **reader card** kind: no card frame, always full width, fixed to its sub-board, not offered by Add card or Pin. The section index must sit
  below the dashboard tabs and print without the board chrome.
- The Brief sub-tab marks an edition read (amends ADR-0081); sub-tabs carry the unread dots (`DashboardBoardTabs` has no badge today).
- Redirects: `/newsroom`, `/newsroom/alerts` and `?edition=&section=&story=` (`toBrief`, `stackport.BriefPagePath`) to the board.
- ADR amendments: 0064 (reader card), 0081 (who marks read), 0086 (the exception ends).

## Owner decisions

- **D1** Decided (owner, 2026-11-25): **1A**. The sidebar's Newsroom opens the stack page (Following · Settings), as every other stack does; 1B
  (the sidebar opens the reader) is rejected. The owner made it the rule for every stack: the stack page is for configuration, setup, tweaking and
  modules (ADR-0089).
- **D2** Not answered; built as recommended, **2A**: Clear stays on the Feed exactly as it was on the Alerts tab (no new "Clear ▾" control).
- **D3** "Alerts" is called **Feed** (owner, 2026-11-25). The RSS sense of "feed" in the source forms is unchanged.

## As built

- Page boards (`view` on a board template) instead of a "reader card" kind: no card frame, Pin, Remove or resize to work around.
- The "Following and settings →" link in the sub-tab row was built and dropped: each view already links to its setup, and on a phone it cost a row.
- The sidebar's Newsroom dot moved to Dashboard. The phone summary link gained the `/admin` prefix it always lacked.

## Remaining Work

None. Not run: a check on vertex after deploy (the owner's).
