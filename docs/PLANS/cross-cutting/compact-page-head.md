---
status: proposed
date: 2026-11-25
last_reviewed: 2026-11-25
related_specs: [SPEC-003, SPEC-013]
constitution_references: [V.2]
related_plans: [../autonomy/newsroom-handoff-2026-11-25.md, ../ARCHIVE/cross-cutting/control-center-dashboard-mocks/index.html]
evidence: Phone screenshots of the dashboard (Home) and the Newsroom brief at 390 × 844 from e2e/dashboard.visual.spec.ts and e2e/newsroom-briefs.visual.spec.ts (SCREENSHOTS=1), and the owner's screenshots of 2026-11-25.
---

# Compact page heads: less above the first card or headline

**Status:** proposed. Mocks only, nothing built. The owner picks the options below before any code.

Mocks: [`compact-page-head-mocks/index.html`](compact-page-head-mocks/index.html) (`?theme=light` for paper). Renders: `case-1.png`
to `case-5.png` (dark).

## The problem (measured in the mocks, 390 × 844, no callout above the page)

| Page | First card or headline now | Proposed |
| --- | --- | --- |
| Dashboard, Home | 314 px | 178 px |
| Newsroom, Brief | 669 px | 320 px |
| Network (the model to copy) | 230 px | unchanged |

- **Dashboard:** the eyebrow and the 36 px title, then a toolbar that wraps to two rows (Layout, Arrange, Add card, Reset board), then a tab
  strip where "Add board" takes about a third of the width, so only two tabs show. The Layout menu changes nothing on a phone, because
  `DashboardGrid.vue` makes every card full width below the tablet width (`PRESET_COLS` only sets the column span).
- **Newsroom brief:** the page head ("Stacks", "Newsroom", a three-line subtitle and Follow on its own row), then the masthead "Morning" at
  display size, a Morning/Evening control that repeats it, the earlier-briefs select, a row with Write / Set up / Delete, and a three-line
  dateline. Newsroom is the only stack page whose `PageHeader` lacks `subtitle-hidden-on-phone`.

## Proposal

1. **One page head for every page** (`PageHeader`): below `sm` there is no eyebrow and no subtitle, and the title (26 px) shares one row
   with the page's actions. Network already looks like this. Make `subtitle-hidden-on-phone` the default and drop the prop.
2. **One tab style** (`StackTabs`, `DashboardBoardTabs`): 44 px tall, 15 px text below `sm`, the full width for tabs, and the
   `ScrollStrip` fade where they continue.
3. **Dashboard, read vs arrange:** in read mode the head has only **Arrange**. While arranging, the head button is **Done**, and a marked
   bar under the tabs holds Add card, Add board and Reset board (rename and remove stay where they are). Below `sm` Layout is not shown.
   On a wide screen Layout sits beside Arrange in the head.
4. **Newsroom brief:** the Morning/Evening switch is the masthead (no separate "Morning" heading on a phone), with **Set up →** always in
   view beside it. The date is the earlier-briefs menu, on one line with "12 h ago · 395 outlets". ⋯ holds Write a new one now and Delete.
   "N model calls" leaves the dateline, since the Model use block shows it.

## Decisions for the owner

- D1: On a wide screen, does Add card also move into arrange mode, or stay in read mode? The mock moves it (one rule at every width).
- D2: On a wide screen, does the brief keep the "Morning" nameplate (newspaper look, mock section 5) or use the phone's switch-as-masthead?
- D3: ⋯ for Write a new one now and Delete, or two labelled buttons on a second row? The mock uses ⋯ because both are rare, and the
  scheduled brief comes on its own. Material Design's overflow-menu rule ("when actions exceed the space, use a more menu instead of
  cramming") supports ⋯; keep Delete last and in the danger colour, apart from the other items, and give ⋯ an accessible name.

## Phases (after the decisions)

1. `PageHeader` compact on phone; tab style shared. Check: the visual specs still assert 44 px tabs and no sideways scroll; new phone
   assertion that the first card of the dashboard and the first headline of the brief start above 200 px and 340 px.
2. Dashboard read/arrange split (`DashboardToolbar`, `DashboardBoardTabs`). Check: `Dashboard.component.test.ts` and the dashboard visual
   spec for arrange on a phone.
3. Brief head (`NewsroomBriefs.vue` masthead slot, `NewsroomEdition.vue` nameplate and dateline). Check: the Newsroom component tests and
   `e2e/newsroom-briefs.visual.spec.ts` at both widths.
4. SPEC-003 (page head rule) and SPEC-013 (brief head) updated; a learnings line.

## Remaining Work

- The owner answers D1 to D3; then phases 1 to 4.
