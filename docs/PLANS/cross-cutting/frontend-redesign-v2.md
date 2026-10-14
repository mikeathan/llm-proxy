---
status: active
last_reviewed: 2026-10-14
---

# Frontend Redesign v2 — Assistant-first shell, calmer look, Settings tabs, more themes

**Status:** active — approved by the operator 2026-10-14 (design review in-session; the mockups were not committed).
**Date:** 2026-10-14
**Related Specs:** SPEC-003 (Admin UI — amended to v2.11 in this plan), SPEC-007 §V (run scheduler, read-only consumer)
**Constitution refs:** V.2 (spec-first), V.3 (no half-finished work), IV.4 (no dead code)
**Related plans:** [`assistant-ui/assistant-automation-workbench-layout.md`](../assistant-ui/assistant-automation-workbench-layout.md) (proposed, gated — chat + editor split and the automation workbench; orthogonal: this plan changes the shell, look, Settings and the composer, and does not implement the split)
**Supersedes:** the "Design language" decisions of [`frontend-redesign-retro.md`](frontend-redesign-retro.md) that this plan changes (listed below). That plan stays `complete`.
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
| V3 | **Settings tabs.** Six group tabs on top of the existing `/settings/:section` routes — General · Models · Safety · Connectors · Appearance · System; groups with several sections show chips (providers, Security/Guardrails, Communication/Search/MCP). Shared save model and single page guard unchanged. |
| V4 | **Assistant ergonomics.** Message box ≥ 3 lines, 16 px on touch, 44 px targets; a phone full-screen editor for long drafts; a `[history \| + New]` control; history collapsible (persisted) with a "New" row pinned in the history footer; the answer reads in a looser column (line height ≈ 1.6, larger block gaps). |
| V5 | **Base look.** Areas are separated by tone and space, not borders; borders stay on inputs and the composer; one 8 px radius (composer 12 px); the accent appears only on the primary button, the active-nav marker, focus, and the logo; run state keeps its own colours. Retro signatures stay: mono micro-labels and numerals, the primary button's hard 2 px offset. |
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

## Risks

- **Two chats at once.** `useAssistant()` calls own separate connections but share `messages`/`sessions`; two mounted chats duplicate steps (documented in `useAssistant.ts`). Mitigation: the panel mounts only when open and not on the Assistant route (one owner, tested).
- **Visual baselines churn** (Phase 4). Regenerated once, listed in the PR.
- **Contrast.** Every new preset is added to `check-contrast.cjs`; accents near state colours are avoided.

## Remaining Work

Everything (plan just opened). Not designed yet: Automations, Reminders, Models and Activity pages on a phone (tables), empty/error/loading polish of the new surfaces, arrow-key handling for radio groups elsewhere. **Phase 1b (optional, after the presets land):** a per-user accent swatch set over any preset (stored through `usePersistedState`; every preset × swatch pair added to the contrast gate). Custom themes already cover changing colours, so this is a convenience, not a gap.
