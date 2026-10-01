---
status: proposed
date: 2026-09-30
last_reviewed: 2026-09-30
related_specs: [SPEC-001, SPEC-003, SPEC-007]
constitution_references: [V.2, V.3, IV.4]
related_plans: [assistant-ui/overhaul-chat-history-layout.md, cross-cutting/frontend-redesign-retro.md, cross-cutting/admin-api-authentication.md, memory/small-context-memory.md]
evidence: docs/audits/2026-09-30-platform-scan.md (U1–U6, U7)
---

# Assistant and Automation — Workbench Layout

**Status:** proposed. **Gate:** Phase 0 (wireframes + SPEC-003 amendment draft) needs the user's sign-off before any
Vue change — the same gate the retro redesign used. Backend changes are small and listed in Phase 1.

## The problem, in the user's terms

The redesign (SPEC-003 v2.x, complete 2026-09-30) fixed the shell, tokens, routes and primitives. The two screens
where work actually happens still behave like the old tab layout:

- **Assistant** (`/workspaces/:ws/assistant`) is a fixed-height box (`h-[calc(100vh-10rem)]`) that *replaces*
  the file view (`views/WorkspacesView.vue:349-394`). You cannot watch the editor or tree while the agent edits
  those very files; you go back and forth between two sections.
- **Automation** detail is one long column: Configuration → Last result → Console (capped at 60 vh) → Runs
  (`AutomationDetails.vue:131-220`). The live console and the run history are never on screen together, a past run
  opens in a drawer, and there is no comparison with the previous run.
- A **finished** run is drawn by a different renderer than a **live** one: live runs use `ChatMessages`
  (`mode="automation"`); finished runs use `ExecutionAuditTrail`, a "Terminal Log" box with emoji glyphs
  (`HistoricalRunDetails.vue:86`, `ExecutionAuditTrail.vue`). The same run looks different before and after it ends.
- Run state (status, model, lane, step, elapsed) is shown in different places in each view, and the Monitor drawer is
  wired separately in both (`WorkspacesView.vue:418-429`, `AutomationsView.vue:356-364`).
- For small local models the most useful number is missing: **how full the context is and whether history was pruned**.

## Keep (do not redo)

`ChatMessages` shared by chat and automation; `useMessageBuilder` as the single event consumer (pitfall 25);
session list with search/pins/groups; status strip with real run state; mobile drawer at 390 px; the
guardrail banner (`submit` prop); typed route builders; `usePolling` ownership rules; `useRunningActivity()`
called by exactly one view (pitfall 37).

## Design rules that bind this plan (from SPEC-003 §IV and the user's review history)

- Tokens only; no raw palette classes (`npm run lint` enforces). **No shadows** (only the primary-button brand offset).
  No glass/blur.
- Nothing may trace to a reference product: no product names in artefacts, no copied glyphs/motifs/palette. Rejected and
  not to return: dotted-relay mark, square-in-square/hollow-square cells, LED segment bars, `//` markers, outlined pills,
  inverted cream chips, corner crosshairs, a `⌘K` hint without a spec'd palette feature.
- Hamburger is mobile-only. Destructive actions use `ConfirmDialog`. Every data view has loading / empty / error states.
- No new polling source. Anything live rides SSE or the existing global poll.
- Adding a destination, route or poll is a SPEC-003 change (spec-first, Constitution §V.2).

## Proposal

### A. Workspace workbench (Workspaces destination, `lg` and up)

```
┌ workspace ▾  Files · Assistant · Memory · Playbooks · Settings ──────────── [run pill] ┐
├──────────────┬──────────────────────────────────────────┬──────────────────────────────┤
│ Files | Chats│  Chat  (centre, full height)             │  Inspector (collapsible)     │
│ tree /       │  ─ run header: status · model · lane ·   │  Changes   files touched     │
│ sessions     │    step · elapsed · context 71% ▮▮▮▯     │  Run       step/tool summary │
│              │  ─ turns                                 │  Memory    injected block    │
│              │  ─ composer                              │  Approvals pending decisions │
│              │                          ┌ editor split ┐ │                              │
│              │                          │ file ▸ diff  │ │                              │
└──────────────┴──────────────────────────┴──────────────┴──────────────────────────────┘
```

- **Section stays a route** (`/workspaces/:ws/assistant/:conversationId?`); the editor split is view state plus an
  optional `?file=` query so a split layout is shareable — no new destination.
- **Files touched by the agent** come from existing `tool_result` events for file-writing tools (*ASSUMPTION: the
  payload carries the path — Phase 0 confirms*). A chip in the turn opens the file in the split pane; the pane shows
  it read-only while the run is live (avoid edit conflicts) and editable after.
- **Context meter** in the run header: prepared prompt chars vs `context_budget`, with "pruned ×N" after a sieve.
  Needs one backend event (Phase 1). This is the small-model feature: it explains slow turns, repeated work and
  forgotten facts.
- **Inspector** replaces the Monitor drawer *for this page*; the drawer's content (running chats, history, dispatcher
  metrics) moves into it as tabs so there is one component (`RunInspector`), not two wirings.
- Below `lg`: sections remain tabs; Inspector becomes a bottom sheet (reuse `PopoverPanel` mechanics, already used by
  the run pill and host stats); editor split becomes a full-screen pane with "← Chat".

### B. Automation master–detail (Automations destination)

```
┌ Automations ────────────────────────────────────────── [Monitor] [+ New] ┐
├───────────────────┬──────────────────────────────────────────────────────┤
│ status filter     │ smoke-test · workspace-1     ● running   ETA ~4 min   │
│ ● smoke-test      │ [Run now|Stop] [Edit] [⋯]                              │
│ ○ nightly-audit   │ ┌ Live | Runs | Config ┐                              │
│ ⧗ report (queued) │ │ Live: transcript (same renderer as chat), full height│
│                   │ │ Runs: table, row → split pane, Δ vs previous run     │
│                   │ │ Config: read-only summary + Edit                     │
└───────────────────┴──────────────────────────────────────────────────────┘
```

- `/automations` keeps list-only on narrow screens and master–detail from `lg`; `/automations/:id` remains the
  addressable detail; tabs are a query (`?tab=live|runs|config`) so each is linkable and back/forward works.
- **One renderer for live and finished runs.** A finished run's events go through `useMessageBuilder` +
  `ChatMessages mode="automation"`. `ExecutionAuditTrail` becomes a **Raw events** view inside the Runs tab (or is
  deleted if nobody wants it — Constitution §IV.4) and loses its emoji glyphs for the icon set.
- **When idle, Live shows the last run's transcript** instead of "Not running".
- **Runs table** gains duration/step/status deltas against the previous run, and opens a run beside the table instead of
  in a drawer. (Drawer stays for Activity and Monitor where there is no table context.)
- **ETA** from the median of the last five completed runs (same data as the performance plan Phase 3.1).

### C. Shared pieces

- `RunHeader` — one component for status · model · lane · step · elapsed · context meter, used by chat and automation.
- `RunInspector` — replaces the two Monitor wirings inside these pages; the standalone Monitor button remains on the
  list/landing pages and in the header.
- `ContextMeter` — a tokens-only primitive (continuous fill over a track, per the existing "ruler meter" motif; not a
  new motif). Added to `/design` with Playwright baselines.

### D. Explicitly not in this plan

Composer model/effort selectors (V6: the send API has no per-turn fields); per-request usage ledger (V1); a
command palette; new theme presets; any change to colours or typography.

## Phases

### Phase 0 — Decide and draw (no Vue changes)

1. Confirm the assumptions: which tool events carry file paths; whether `AutomationInfo` has next-run/ETA inputs;
   what `HistoricalRunDetails` receives (`run.events` shape) so the unified renderer can be fed. Put the answers in this doc.
   - Verify: read `frontend/src/composables/assistant/useMessageBuilder*`, `frontend/src/types/dispatcher.ts`, backend
     `assistant/agent_events.go`; record `file:line`.
2. Wireframes as static markup in the existing `/design` gallery route (dev-only), desktop / tablet / 390 px, using only
   tokens and current primitives — the same "show, then sign off" loop as the redesign's Phase 0, without
   reviving the deleted `docs/design/` folder.
3. Draft the SPEC-003 amendment (version bump, §II.2 destinations text, §III.2 route/query table, §II.5 context meter,
   §V budgets). Do not merge the amendment until sign-off.
4. User sign-off on the decisions below.

Acceptance: wireframes reviewed at the three widths; decisions answered; amendment draft reviewed.

### Phase 1 — Small backend additions (TDD)

1. **`context_usage` lifecycle event**: `{chars, budget, pruned}` emitted once per turn from the point that already
   measures it (`preparedOverContextBudget`, `assistant/sieve.go:159-181`), for both channels. Additive; old clients ignore it.
   SPEC-001 event list updated. Verify: `cd backend && go test ./internal/core/assistant/ -run ContextUsage -count=1`.
2. **Backend half of the SSE-bleed fix** (overhaul plan Phase 5, still open): terminate the event stream for a cancelled
   run so no cancelled-turn events reach a new turn. The split view makes two live consumers more likely, so close this first.
   Verify: `cd backend && go test ./internal/core/assistant/ ./internal/transport/http/handlers/ -run 'Cancel|Bleed' -count=1 -race`.
3. **Next-run / ETA fields** on `AutomationInfo` only if Phase 0 shows the frontend cannot derive them (cron parse + run history).

### Phase 2 — Automation master–detail and one renderer

1. `RunHeader`, `ContextMeter` (+ `/design` entries and visual baselines).
2. Feed finished runs through `useMessageBuilder`; `ExecutionAuditTrail` → "Raw events" or removed.
3. Master–detail at `lg`, tabs as query, Runs table deltas, split run pane, idle-shows-last-run.
4. Keep-alive and unsaved-change guard behaviour unchanged (`useAutomationForm` populate watch stays keyed on `editAutomation.value?.id`, pitfall 38).

Verify: `cd frontend && npm test && npm run lint && npm run build && npm run test:visual`.

Acceptance: a finished run and the same run while live render identically (screenshot diff ≤ threshold on the transcript
region); the list → detail → edit → back path works by URL at 1440 and 390 px; no new `setInterval` (test asserts pollers unchanged).

### Phase 3 — Workspace workbench

1. Chat + editor split with `?file=`; files-touched chips; read-only while live.
2. `RunInspector` (Changes / Run / Memory / Approvals); Memory tab uses the injection preview endpoint from
   `memory/small-context-memory.md` Phase 4.1 when available, else is omitted (no placeholder).
3. Workspaces keep-alive preserved; `useRunningActivity()` still called only by `WorkspacesView`.
4. Bottom-sheet inspector and full-screen editor below `lg`.

Verify: same four frontend commands + `cd backend && go test ./...`.

Acceptance: with a run active you can see the agent's chat, the file it just wrote, and the context meter in one viewport at
1440×900; at 390 px every pane is reachable by keyboard and touch; the inspector adds ≤ 25 kB gzip to the Workspaces chunk.

### Phase 4 — Hardening and close-out

Accessibility sweep (Tab order, focus visible, `aria-live` only for run state, reduced motion); contrast gate;
route chunk sizes recorded; docs: SPEC-003 bumped, `docs/architecture.md` (entry points, route/query table),
`.agents/skills/assistant-ui-chat` and `assistant-ui-patterns` updated, plan statuses and `docs/PLANS/README.md` Remaining Work,
`./scripts/check-agent-harness.sh` green.

## Decisions for the user

| # | Question | Recommendation |
|---|---|---|
| D-U1 | Split chat + editor by default, or opt-in per workspace? | Default **off** below `xl`, on at `xl`+; remembered via `usePersistedState`. |
| D-U2 | Let the Inspector replace the Monitor drawer on these two pages? | Yes — one component, fewer concepts; the drawer stays elsewhere. |
| D-U3 | Backend `context_usage` event (Phase 1.1) | Yes — it is the feature that makes small-model behaviour legible. |
| D-U4 | Keep the terminal-style raw event log? | Keep as a "Raw events" view inside Runs; delete if unused after a month (Constitution §IV.4). |
| D-U5 | Run comparison scope | Start with duration/steps/status deltas vs the previous run; no text diff of reports. |

## Risks

| Risk | Mitigation |
|---|---|
| Two live consumers of one event stream regress the SSE-bleed fix | Phase 1.2 before the split; `-race` test |
| Layout churn breaks the visual baselines | baselines updated per phase, reviewed in the PR; the preset contrast gate stays in `lint` |
| Workspaces chunk grows | budget in the acceptance criteria; lazy-load the inspector tabs |
| Reading `file:` query on the editor creates a second source of truth with the route path | one helper in `router/routes.ts`; the route path stays canonical, query is additive |
| Scope creep into selectors/ledger/palette | listed under D; each needs its own plan |

## Remaining Work

Everything; Phase 0 step 1 (confirm the event/payload assumptions) is a half-day read and unblocks the wireframes.
