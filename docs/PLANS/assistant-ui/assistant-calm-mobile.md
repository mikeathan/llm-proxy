---
status: partial
date: 2026-11-26
last_reviewed: 2026-11-26
related_specs: [SPEC-003, SPEC-007]
related_plans: [assistant-ui/assistant-automation-workbench-layout.md, cross-cutting/frontend-redesign-v2.md]
evidence: docs/audits/2026-11-26-assistant-automations-service-review.md
---

# A calmer assistant and automations, phone first

**Status:** partial — owner approved the mocks on 2026-11-26 ("go ahead"), with one change: desktop chat full width.
Phases 1–4 built on branch `assistant-calm-mobile` (v0.145.0, with the service fixes B1–B4); what is left is below. Mocks: [`assistant-calm-mobile-mocks/`](assistant-calm-mobile-mocks/README.md). Frames that need new behaviour are listed below.

## Problem

The owner: "assistant is bit noisy especially in a mobile. we need better user experience." Measured on a 390 × 844 phone
(evidence file, Part A): 237 of 844 px of chrome (two header rows, a composer with a second row, the tab bar), a failed turn that says "Worked 1s" and folds its error away with no fix, a
chat that opens 334 px above its latest message, a send icon that reads as ⚠, and two different automation detail layouts.

## Proposal (frames in the mocks)

1. **Bugs first, no design decision needed:** open a chat at its latest message (U1); failed turn says it failed (U2 wording);
   the clipped model label (U3); the send icon (U4).
2. **Assistant on a phone (A–D):** one 52 px bar on the Assistant page; composer is one row with a **+** menu (Commands, Memory
   review, model); activity line names what was done and carries model · tokens · time; thoughts fold to one line; failed turn is a
   sentence, the fix as buttons, raw error behind Details (a shared `describeFailure` beside `services/failureReason.ts`);
   chats drawer has one ⋯ per row; tables say they scroll; tokens/s and CPU/MEM hidden while idle.
3. **Automations (E–H, J):** status-first list rows (last outcome, when, how long, Run); one detail layout for every automation:
   **Result · Runs · Setup**, with **Live** replacing Result while a run goes; network-audit's Simple/Advanced folds into Setup; the
   `last_output` log preamble is not shown above the report; Clear runs and Delete move into ⋯. This is the automation half of
   [`assistant-automation-workbench-layout.md`](assistant-automation-workbench-layout.md) (proposed); build it there, not twice.
4. **Desktop (I, J):** the same pieces with more room; the chat spans the full page width, no 52rem reading column (owner, 2026-11-26); turn meta on hover or focus; recent runs beside the result.

Every change is made in its shared component; the mocks' section 5 lists the other consumers to check (`ChatBubble` also draws
automation live runs, `DataTable` stacked rows have 7 consumers, `MarkdownViewer` 4, `SectionHeading` is left alone).

## Phases (each: `npm test` + `npm run build` + `npm run test:visual`, phone and desktop screenshots)

0. Owner picks from the mocks; SPEC-003 amendment for the Assistant phone bar and the automation detail tabs.
1. Bug fixes U1–U4 with regression tests (U1: a component test that mounts a loaded session and reads `scrollTop`; Playwright for geometry).
2. Turn: activity line, folded thoughts, failure sentence, table hint, memory sentence.
3. Phone chrome: one bar on Assistant, composer **+** menu, idle header stats, chats ⋯ menu.
4. Automations: list rows (DataTable `summary` slot), unified detail tabs.

## Needs new behaviour (not just layout)

Checked in the code before listing; the owner should know these are new work, not a re-layout:

- **Retry with another model** on a failed turn: the composer can pick a model (`RunSettings.vue`), but there is no one-tap retry with a different one.
- **Undo** on a memory save: the save is removed in the Memory panel today; Undo needs the saved fact's id on the turn (`TurnRun.memory_saved` holds text only).
- **"failed" in the chats list**: `SessionBrief` has `running` only; needs a `last_error` (or similar) from the sessions endpoint.
- **"step 4 of ~12"** on a running automation row: no step estimate exists; show "running · 3 min" instead unless one is added.
- **Starter prompts** on a new chat and **Select…** for chats: new UI only (no backend), but new.
- **Last outcome on each automation row**: the list endpoint already returns `history`; no backend change.

## As built (v0.145.0)

Frames A–H and J as mocked, with the desktop chat at full width; the tab bar hides while typing. Checked with
`e2e/assistant.visual.spec.ts` and `e2e/automations.visual.spec.ts` (geometry read in the browser), `npm test`,
`npm run build` and the whole visual suite. SPEC-003 2.78.

## Remaining Work

1. New behaviour from the list above, each needing a backend field: Retry with another model on a failed turn, Undo
   on a memory save, "failed" in the chats list.
2. The desktop's three doors to the assistant (review N8) and a one-bar header on the desktop Assistant page (frame I's
   header): not built, the desktop keeps today's header and Monitor.
3. Owner: on vertex, add the OpenCode key under **OpenCode Go** (Models › Providers) and re-add `glm-5.3-flash` and
   `deepseek-v4.1-flash` from it; remove the NVIDIA "opencode" key. Remove the six orphaned model overrides (review B5).
