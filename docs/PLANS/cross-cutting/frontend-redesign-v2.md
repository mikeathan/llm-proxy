---
status: partial
last_reviewed: 2026-10-14
---

# Frontend Redesign v2 — Assistant-first shell, calmer look, Settings tabs, more themes

**Status:** partial — approved by the operator 2026-10-14 (design review in-session; the mockups were not committed). Phases 0–5 delivered 2026-10-14 (one PR); the remaining work below is follow-up.
**Date:** 2026-10-14
**Related Specs:** SPEC-003 (Admin UI — amended to v2.11 in this plan), SPEC-007 §V (run scheduler, read-only consumer)
**Constitution refs:** V.2 (spec-first), V.3 (no half-finished work), IV.4 (no dead code)
**Related plans:** [`assistant-ui/assistant-automation-workbench-layout.md`](../assistant-ui/assistant-automation-workbench-layout.md) (proposed, gated — chat + editor split and the automation workbench; orthogonal: this plan changes the shell, look, Settings and the composer, and does not implement the split)
**Supersedes:** the "Design language" decisions of [`frontend-redesign-retro.md`](../ARCHIVE/cross-cutting/frontend-redesign-retro.md) that this plan changes (listed below). That plan stays `complete`.
**Skills:** `task-planning`, `frontend-vue-engineer`, `assistant-ui-patterns`, `tdd-guide`, `documentation-stewardship`

---

## Problem

The operator finds the console noisy and hard to use, in four concrete ways:

1. **Settings is one long list.** Every category is a route, but the nav is a sidebar (a `<select>` below `lg`) and the long forms stack panels.
2. **The assistant is hard to reach and cramped.** It is a hover-revealed icon on the Workspaces row (no hover on touch). The message box starts at one row with 14 px text (iOS zooms inputs under 16 px).
3. **The look is boxy.** A hairline surrounds every panel, header and list; the shapes are square (2 px radius); colour is spread across many surfaces.
4. **Not mobile-oriented.** One drawer for everything; no bottom navigation; the workspace is chosen by navigating to a list.

## Decisions (operator-approved)

| # | Decision |
|---|---|
| V1 | **Assistant first.** Assistant is the first destination and the home of the shell; **Workspaces leaves the sidebar** (D10 changes) — a global **workspace chip** in the header chooses the active workspace (default: last used, `useLastWorkspace`) and a **Workspace** destination holds Files · Memory · Playbooks · Heartbeat · Settings. |
| V2 | **Ask field + panel.** A compact "Ask the agent…" field in the header opens the assistant as a side panel on any page (full screen on phones). On the Assistant route it is hidden. Only one `AssistantChat` is ever mounted (each `useAssistant()` owns an SSE connection). |
| V3 | **Settings tabs.** Six group tabs on top of the existing `/settings/:section` routes — General · Models · Safety · Connectors · Appearance · System; groups with several sections show chips (providers, Security/Limits/Guardrails, Communication/Search/MCP). Shared save model and single page guard unchanged. |
| V4 | **Assistant ergonomics.** Message box ≥ 3 lines, 16 px on touch, 44 px targets; a phone full-screen editor for long drafts; a `[history \| + New]` control; history collapsible (persisted) with a "New" row pinned in the history footer; the answer reads with deliberate spacing (a turn without steps carries no empty timeline wrapper, the user message's hover actions float, larger gaps between turns). |
| V5 | **Base look.** Areas are separated by tone and space, not borders; borders stay on inputs and the composer; 6/8 px radii (`--radius-sm/md`; no new radius token); the accent appears only on the primary button, the active-nav marker, focus, and the logo; run state keeps its own colours. Retro signatures stay: mono micro-labels and numerals, the primary button's hard 2 px offset. |
| V6 | **Phone navigation.** Bottom tab bar — Assistant · Workspace · Automations · Reminders · More (Overview, Models, Activity, Settings). |
| V7 | **Themes.** More dark and light presets (a second dark theme besides the soft default), each passing the contrast gate. Custom themes, import/export and Follow system are unchanged. |

Rules it relaxes from the retro plan: square 2 px corners (→ 8 px), hairline panel borders (→ tone), the seven-destination rail (→ Assistant first, Workspaces under the chip). Rules it keeps: no shadows except the primary button's offset, semantic tokens only, no new dependencies.

## Phases (one commit each; `npm test && npm run lint && npm run build` green before the next)

0. **Docs first** — this plan, SPEC-003 v2.3 amendment, registration. *Accept:* `./scripts/check-agent-harness.sh` passes.
1. **Themes** — one TypeScript list of presets that `presets.ts`, `tokenSheet.ts`, `types/theme.ts` import (today the list is restated in five files); new blocks in `styles/tokens.css`; `public/theme-boot.js`; `scripts/check-contrast.cjs`. *Accept:* every new preset passes all contrast pairs; the picker lists them; `/design` visual baselines added.
2. **Settings tabs** — `SettingsTabs` (groups + chips) replacing `SettingsNav`; pure grouping in `domain/settings.ts`. *Accept:* every existing section is reachable in ≤ 2 taps; deep links unchanged; keyboard (arrow keys) works; unit tests for the grouping.
3. **Assistant** — composer, `[history | + New]`, collapsible persisted history, footer "New" row, phone full-screen editor, reading-column spacing. *Accept:* composer tests (3 rows, expand), history persistence, existing chat tests green.
4. **Base look** — primitives (`Panel`, header, sidebar, session list, cards) move from lines to tone; radius tokens. *Accept:* visual baselines regenerated deliberately and listed in the PR; contrast gate green.
5. **Shell** — workspace chip, Workspace destination, ask field + panel (`AssistantChat` `variant="panel"`), phone bottom tabs + More. *Accept:* route tests for the new destinations; a run survives opening/closing the panel (SSE replays on reconnect); never two `AssistantChat` mounted.

## As built (2026-10-14)

- **0 Docs** — this plan, SPEC-003 v2.11 (§II.1, II.2, II.7, III, IV, V), pitfalls 49–51, `assistant-ui-patterns`.
- **1 Themes** — `retro-midnight`, `retro-forest`, `retro-plum` (dark), `retro-paper-cool`, `retro-sand` (light); `PRESET_IDS` in `types/theme.ts`; the contrast script reads ids from `tokens.css`. Phase 1b (accent over any preset) **built later the same day**: eight swatches plus any colour, `theme/accent.ts`, proven for every preset × swatch.
- **2 Settings tabs** — `SettingsTabs` + `ScrollStrip` (+ `useScrollEdges`); `getSettingsTabGroups`; `SettingsNav` removed. General is split into four short sections (`local`, `runs`, `memory`, `storage`) over one form: `GlobalSettings` takes a `section` and shows only its panels (all stay mounted).
- Security (the one long page, ~1,950 px) is split the same way: `security` (Sandboxing, Effective enforcement) and `limits` (Resource limits, live terminals, Reset controls) — `SecuritySettings` takes a `section`, both stay mounted over one draft, and the Save row also shows on `limits` while there is an unsaved edit.
- **3 Assistant** — `ComposeSheet`, header cap, persisted list state, footer New, quiet status, `dvh` height, spacing.
- **4 Look** — Panel, header, sidebar, chat chrome by tone; radii 6/8 px; `/design` baselines regenerated.
- **5 Shell** — keep-alive chat closed on leave (own commit), ask panel (`AssistantChat variant="panel"`), assistant-first navigation (`utils/navigation.ts`) with a phone bottom bar, workspace chip (`useActiveWorkspace`), home route. The workspace page header's picker is gone. On a phone the assistant is fixed between the top strip and the bar and the workspace header is hidden there.

Deviations from the mockups: the panel header has no recent-chats menu (its conversation list is the same overlay as on a phone); the header's "Ask" field is an icon while the panel is open; Settings is the only page that says "All workspaces" (Overview, Automations, Activity and Models show the workspace the shell opens, which is what the ask panel talks to).

## Risks

- **Two chats at once.** `useAssistant()` calls own separate connections but share `messages`/`sessions`; two mounted chats duplicate steps (documented in `useAssistant.ts`). Mitigation: the panel mounts only when open and not on the Assistant route (one owner, tested).
- **Visual baselines churn** (Phase 4). Regenerated once, listed in the PR.
- **Contrast.** Every new preset is added to `check-contrast.cjs`; accents near state colours are avoided.

## Remaining Work

- Automations, Reminders, Models and Activity on a phone (tables and logs need a "fewer details" layout); their empty/error/loading polish.
- Arrow-key handling for radio groups and the new sheets (More, workspace menu); a focus trap in the phone ask dialog.
- Decide whether Overview, Automations and Activity should read "All workspaces" in the chip (today only Settings does).
- A recent-chats menu in the ask panel's title (prototyped in the mockup).
