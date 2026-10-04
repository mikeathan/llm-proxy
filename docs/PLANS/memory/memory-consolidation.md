---
status: proposed
date: 2026-10-04
last_reviewed: 2026-10-04
related_specs: [SPEC-004, SPEC-001, SPEC-007, SPEC-003]
constitution_references: [II.2, II.12, II.13, II.14, III.2, V.2, V.3]
related_plans: [../autonomy/autonomy-roadmap.md, ../autonomy/attention-policy.md, ../autonomy/agenda-reminders-and-followups.md, memory-improvements-implementation-plan.md, small-context-memory.md]
---

# Memory Consolidation — "Sleep" for the Assistant (autonomy milestone M5)

**Status:** proposed — part of the autonomy program (`../autonomy/autonomy-roadmap.md`). Phases 0–2 are
independent of the other autonomy plans; Phase 3 uses the agenda and attention policy.

## Why this plan exists

Memory today grows only when the agent decides, mid-task, to call `memory_update`, or when you add facts
by hand. Nothing ever steps back and asks: *what did I learn about this person today? Which facts are
duplicates, outdated, or never used?* So useful preferences said in passing are lost, and the hot set
slowly fills with stale facts that eat a small local model's context budget.

| User benefit | Service benefit |
|---|---|
| The assistant gets noticeably better week by week: preferences and facts from the day's conversations are captured. | A smaller, cleaner hot set → more useful context per token, especially for small local models (`small-context-memory.md`). |
| A short "what I learned today" you can review — accept, edit or reject. Nothing about you is stored silently. | Usage data that is already collected (`injected_count`, `searched_count`, `last_used_at`) finally drives cleanup. |
| Fewer wrong answers caused by outdated facts. | Runs at night on the local model, when the GPU is idle. |

## Ground truth (verified in code, 2026-10-04)

| Fact | Source |
|---|---|
| Memory types: `long_term`, `daily`, `session`, `user_profile` | `backend/internal/platform/memory/types.go:16-19` |
| Store operations exist for insert, update, priority, delete, list, exact-duplicate check, find by title/substring | `backend/internal/platform/memory/store.go:182-348` |
| Usage tracking: injected/searched counts and last-used time; `ListUnused` returns facts never used | `backend/internal/platform/memory/usage.go:104-160`; `docs/api-reference.md` (Memory API `?unused=true`) |
| A reaper already deletes only `session` memories past retention; long-term and profile facts are "operator data regardless of age" | `backend/internal/platform/memory/reaper.go` |
| Hot set = facts every run carries; inspectable via `injection-preview` | `docs/api-reference.md` (Memory API) |
| Conversation sessions are stored per workspace as JSON | `docs/data-layout.md` (meta/ section) |
| The memory-improvements plan's open items are session search (FTS5) and the skill system — not consolidation | `memory-improvements-implementation-plan.md` (status note at top) |
| Memory system is constitutional | Constitution II.12; SPEC-004 |

## Design

### Inputs (per workspace + global)
- The day's conversation sessions and automation/goal run outputs (bounded, newest first).
- Current facts with usage stats; the hot set.
- Operator notes (`MEMORY.md`) — read-only context, never edited.

### Steps
1. **Deterministic hygiene (no LLM):** exact duplicates, facts never used in N days that are in the hot
   set (→ propose *demote*, never delete), expired `daily` entries.
2. **Extraction (LLM, local model by default):** from the day's sessions, propose new
   `user_profile`/`long_term` facts (preferences, recurring people/places/projects, corrections you made).
   One template in `prompts/templates.go`; each proposal cites the session it came from.
3. **Merge proposals (LLM):** near-duplicate facts → one merged fact, shown as a diff.
4. **Contradictions:** a new fact conflicting with an old one → propose *replace*, with both shown.

### Output = proposals, not edits
Every change is a `ConsolidationProposal{kind: add|merge|demote|replace|delete, before, after, evidence,
risk: low|high}` stored for review. **Apply modes** (D1): `review` (default — nothing changes until you
accept), `auto-low-risk` (exact duplicates and demotions apply automatically; everything else waits),
`off`.

### Delivery
- Scheduled nightly through the agenda (`../autonomy/agenda-reminders-and-followups.md`) as an `agenda`
  run in each workspace with recent activity.
- One `low` notice "Learned 4 things, 2 cleanups to review" → attention policy → morning digest.
- Review in the web UI inbox; quick accept/reject from Telegram for single items (cross-channel plan).

### Safety
- Never touches operator notes; never deletes `user_profile` facts without explicit acceptance.
- Secrets are never proposed as facts (`tools.RedactSecrets` on all inputs —
  `backend/internal/core/tools/security.go:22`; later the privacy engine).
- Accepted changes keep provenance (`source: consolidation`) so they can be found and reverted.

## Decisions for the user

| # | Decision | Recommendation |
|---|---|---|
| D1 | Default apply mode | `review` for the first weeks; offer `auto-low-risk` after 20 accepted proposals |
| D2 | Run time | 03:00 owner time, only if the local lane is idle; skip and retry next night otherwise |
| D3 | "Unused" threshold for demotion proposals | 30 days not injected or searched |
| D4 | Cloud models allowed for consolidation? | No by default (it reads all of the day's conversations); opt-in per workspace |

## Phases

### Phase 0 — Deterministic hygiene + proposal store
- Proposal table, hygiene rules, accept/reject API.
- **Acceptance:** store/rule tests; accept applies via existing store functions; reject leaves memory
  unchanged. `go test ./internal/platform/memory/ -count=1`.

### Phase 1 — Extraction and merge (LLM)
- Templates, extraction and merge passes, evidence links; record-replay tests on sample sessions.
- **Acceptance:** recorded run produces stable proposals; no proposal contains a secret pattern; facts
  already present are not re-proposed.

### Phase 2 — Manual run
- "Consolidate now" in the Memory UI and `POST /admin/api/memory/{ws}/consolidate`.
- **Acceptance:** handler test; run admitted through the run lane (not run inline in the request).

### Phase 3 — Nightly schedule + notices (needs the agenda and attention plans; D2)
- **Acceptance:** fake-clock test — runs at 03:00 when idle, skips when busy, one digest notice.

### Phase 4 — Review UI + apply modes (after `../autonomy/autonomy-ux-design.md` Phase 2; D1, D3)
- Review inbox with diffs, accept-all-low-risk, Telegram quick actions.
- **Acceptance:** `npm test && npm run build && npm run test:visual`.

### Phase 5 — Docs
- SPEC-004 (consolidation, proposals, provenance), `docs/api-reference.md`, memory skill update.
- **Acceptance:** `./scripts/check-agent-harness.sh` green.

## Risks and non-goals
- **Wrong facts learned** — review mode by default, evidence on every proposal, provenance for revert.
- **Privacy** — local model by default (D4); inputs redacted.
- **Non-goal:** the skill system / procedural memory and session search (owned by
  `memory-improvements-implementation-plan.md`).

## Remaining Work
All phases (0–5). Phase 3 blocked on the agenda and attention plans; Phase 4 on the UX design.
