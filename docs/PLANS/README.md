# Implementation Plans

Plans document **implementation strategies** — how a feature was (or will be) built.

Each plan has a status field. Use the status to determine relevance:
- **complete** — Implementation is done. Plan kept for historical reference.
- **partial** — Partially implemented. Remaining work is tracked in the plan.
- **proposed** — Not yet started. Design is being evaluated.
- **draft** — Early-stage design, not yet scoped.
- **superseded** — Replaced by a newer design or specification. Kept for historical reference.
- **reference** — Architectural reference document, not an implementation plan.

Organized by subsystem:
- `agent-loop/` — SPEC-001: agent loop, sieves, stuck detection, fallbacks, refactoring
- `memory/` — SPEC-004: memory storage, tags, injection, dedup
- `orchestrator/` — SPEC-005: budget management, token allocation
- `automation/` — SPEC-007: dispatcher, triggers
- `discovery/` — SPEC-003: model catalog, UI panels
- `assistant-ui/` — SPEC-003: chat UI, bubbles, layout
- `cross-cutting/` — Multi-SPEC or standalone: connectors, user input, lifecycle, testing, agent instructions
- `autonomy/` — Autonomy program (always-on assistant): umbrella [`autonomy/autonomy-roadmap.md`](autonomy/autonomy-roadmap.md) owns the order of work; agenda, attention policy, cross-channel + remote approvals, standing goals, event triggers (memory consolidation lives in `memory/`)

> The **Codebase Audit Report** (88 findings) was an audit, not a plan — it now lives at
> [`docs/audits/codebase-audit-report.md`](../audits/codebase-audit-report.md).

## Active Plans

> Status legend: `active` (in flight) · `approved` (accepted, in progress) · `partial` (some phases done)
> · `proposed` (design pending) · `complete` (done, kept for reference) · `reverted`/`superseded` (archived).

| File | Title | Status | Date | Related Specs |
|------|-------|--------|------|---------------|
| [`unattended-run-safety-hardening.md`](unattended-run-safety-hardening.md) | Unattended Run Safety Hardening (13 gaps, 7 leaks, 5 optimizations) | complete | 2026-07-22 | SPEC-001, SPEC-006, SPEC-007 |
| [`gpu-performance.md`](gpu-performance.md) | GPU Performance (consolidated: completed + next steps) | active | 2026-08-06 | — |
| [`primary-model-warning-banner.md`](primary-model-warning-banner.md) | Remove model auto-bootstrap; explicit primary/fallback selection + banners | complete | 2026-08-14 | SPEC-003, CONSTITUTION III.4 |
| [`agent-loop/agent-improvements.md`](agent-loop/agent-improvements.md) | Agent Improvements (7-phase; re-scoped 2026-09-05 — Phases 3/6 superseded by SPEC-010, Phase 4/5/7-remainder open) | partial | — | SPEC-001 |
| [`agent-loop/agent-loop-strategies.md`](agent-loop/agent-loop-strategies.md) | Agent Loop Strategies (pluggable loop-strategy engine) | complete | 2026-08-16 | SPEC-001, SPEC-010 |
| [`agent-loop/strategy-agnostic-completion-and-tool-schema.md`](agent-loop/strategy-agnostic-completion-and-tool-schema.md) | Strategy-Agnostic Completion + Tool-Schema/Policy Consistency | complete | 2026-08-18 | SPEC-010, SPEC-006, SPEC-001 |
| [`agent-loop/surface-planning-reasoning.md`](agent-loop/surface-planning-reasoning.md) | Surface Plan-Generation Reasoning (stream the planner) | complete | 2026-08-18 | SPEC-010, SPEC-001, SPEC-003 |
| [`agent-loop/fix-final-report-realignment.md`](agent-loop/fix-final-report-realignment.md) | Fix automation "Final Report" regression | complete | 2026-08-07 | SPEC-001 |
| [`assistant-ui/overhaul-chat-history-layout.md`](assistant-ui/overhaul-chat-history-layout.md) | Assistant UI Overhaul | complete | 2026-09-30 | SPEC-003 |
| [`assistant-ui/automation-renderer-unify-consumption.md`](assistant-ui/automation-renderer-unify-consumption.md) | Unify Automation + Assistant Event Consumption | complete | 2026-07-18 | SPEC-003, SPEC-007 |
| [`assistant-ui/automation-edit-form-reactivity.md`](assistant-ui/automation-edit-form-reactivity.md) | Fix Automation Edit Form — reactive populate | complete (absorbed by frontend-redesign-retro, Phase 5) | 2026-08-01 | SPEC-003, SPEC-007 |
| [`assistant-ui/consolidate-app-banner.md`](assistant-ui/consolidate-app-banner.md) | Consolidate banner logic into a single event-driven `AppBanner` | complete | 2026-08-14 | SPEC-003 |
| [`assistant-ui/reasoning-inset-auto-collapse.md`](assistant-ui/reasoning-inset-auto-collapse.md) | Reasoning inset auto-expand while running, collapse on done | complete | 2026-08-14 | SPEC-003 |
| [`cross-cutting/frontend-redesign-retro.md`](cross-cutting/frontend-redesign-retro.md) | Frontend Redesign — Retro Theme System, Shell, Tree & Notifications (absorbs `automation-edit-form-reactivity` at Phase 5) — Phases 0–7 complete; SPEC-003 v2.0 | complete | 2026-09-30 | SPEC-003, SPEC-007 |
| [`cross-cutting/ci-github-actions-and-versioning.md`](cross-cutting/ci-github-actions-and-versioning.md) | CI, GitHub Actions Integration & Versioning Flow (P1 CI + P4 tag implemented; P2/P3 + P6 pending, P5 parked) | partial | 2026-08-30 | — |
| [`cross-cutting/connector-inbound-webhook.md`](cross-cutting/connector-inbound-webhook.md) | Communication Connector Inbound Webhook | complete | 2026-06-28 | SPEC-009 |
| [`cross-cutting/connector-auto-reply.md`](cross-cutting/connector-auto-reply.md) | Communication Connector Auto-Reply & Automation Trigger | active | 2026-06-28 | SPEC-009 |
| [`cross-cutting/webhook-fresh-sessions.md`](cross-cutting/webhook-fresh-sessions.md) | Fresh Webhook Sessions + Source Grouping | complete | 2026-07-09 | SPEC-009 |
| [`cross-cutting/session-source-backend-driven.md`](cross-cutting/session-source-backend-driven.md) | Session `source` derived from backend (single source of truth) | complete | 2026-07-09 | SPEC-009 |
| [`cross-cutting/reasoning-capture-dynamic.md`](cross-cutting/reasoning-capture-dynamic.md) | Dynamic, provider-agnostic reasoning enable + capture + neutral indicator (typed, SOLID) | complete | 2026-07-28 | SPEC-003, SPEC-005 |
| [`cross-cutting/per-model-reasoning-overrides-and-settings-layout.md`](cross-cutting/per-model-reasoning-overrides-and-settings-layout.md) | Per-Model Reasoning Overrides and Settings Layout Repair | complete | 2026-08-02 | SPEC-005 |
| [`cross-cutting/post-implementation-cleanup.md`](cross-cutting/post-implementation-cleanup.md) | Post-Implementation Cleanup: Duplication & Dead Code | active | 2026-08-01 | SPEC-005 |
| [`cross-cutting/agent-os-sandboxing.md`](cross-cutting/agent-os-sandboxing.md) | Agent OS Sandboxing (rev 2 — network-first, uid-first; one action pipeline with OS jail + egress proxy; Phases 0–4 implemented + post-review hardening) | complete (pending Linux-CI) | 2026-09-06 | SPEC-006, SPEC-009 |
| [`cross-cutting/sandbox-runtime-invisibility.md`](cross-cutting/sandbox-runtime-invisibility.md) | Sandbox Runtime Invisibility (`.sandbox` hidden from filesystem listings + terminal output) | complete | 2026-08-25 | SPEC-006, CONSTITUTION II.3 |
| [`cross-cutting/assistant-liveness-heartbeat-package-split.md`](cross-cutting/assistant-liveness-heartbeat-package-split.md) | Assistant Liveness Heartbeat & Package Restructure | partial | 2026-08-18 | SPEC-001, SPEC-010, SPEC-003 |
| [`cross-cutting/agents-md-layering-guardrails.md`](cross-cutting/agents-md-layering-guardrails.md) | AGENTS.md Layering, Override-ability & Write Guardrails | proposed | 2026-08-04 | SPEC-001, CONSTITUTION II.13/II.10 |
| [`cross-cutting/tool-call-grammar-reenable.md`](cross-cutting/tool-call-grammar-reenable.md) | Re-enable Tool-Call Grammar Constraint (opt-in, llama.cpp-safe) | proposed | 2026-09-05 | SPEC-001, SPEC-002 |
| [`cross-cutting/search-tool-calling.md`](cross-cutting/search-tool-calling.md) | Wire up `internet_search` tool calling (pluggable multi-provider, live key, hide-when-unconfigured) | active | 2026-09-12 | SPEC-001, SPEC-006 |
| [`cross-cutting/tool-error-classification.md`](cross-cutting/tool-error-classification.md) | Tool Error Classification & Run-Fatality Policy (terminal tool errors, delivery-vs-essential, failure bound) | complete | 2026-09-12 | SPEC-001, SPEC-010, SPEC-006 |
| [`cross-cutting/assistant-conversation-package.md`](cross-cutting/assistant-conversation-package.md) | Assistant Conversation Package (deferred extraction; Step 0 consolidates the LLM/tool test doubles) | proposed | 2026-09-12 | SPEC-001 |
| [`cross-cutting/persist-assistant-run-state-for-reload.md`](cross-cutting/persist-assistant-run-state-for-reload.md) | Persist assistant run state (errors/cancels/running) for reliable reload | complete | 2026-08-20 | SPEC-001, SPEC-003 |
| [`cross-cutting/sqlite-session-storage.md`](cross-cutting/sqlite-session-storage.md) | SQLite session storage (future work, proposed) | proposed | 2026-08-20 | SPEC-001 |
| [`cross-cutting/xdg-config-data-relocation.md`](cross-cutting/xdg-config-data-relocation.md) | XDG Config/Data Relocation + Storage Cleanup + Reset Controls (Phases 0–7, 9–12 complete; Phase 8 removed; reset/clear-runtime-data hardened) | complete | 2026-08-07 | CONSTITUTION III.2/III.4/III.6 |
| [`memory/memory-improvements-implementation-plan.md`](memory/memory-improvements-implementation-plan.md) | Memory Improvements | partial | — | SPEC-004 |
| [`automation/automation-learning-journal.md`](automation/automation-learning-journal.md) | Automation Learning Journal (bullet dedup, per-automation journal, memory-first, active hours) | partial | 2026-10-03 | SPEC-007, SPEC-004 |
| [`automation/memory-and-heartbeat-simplification.md`](automation/memory-and-heartbeat-simplification.md) | Memory defaults + overrides (assistant/automation) and a simple per-workspace Heartbeat section | complete | 2026-10-04 | SPEC-004, SPEC-007, SPEC-003 |
| [`cross-cutting/admin-api-authentication.md`](cross-cutting/admin-api-authentication.md) | Admin / API Authentication and Control-Plane Hardening (no auth today; Host/Origin checks, operator token, `/v1` keys, safer bind) | proposed | 2026-09-30 | SPEC-003, SPEC-006, SPEC-007, SPEC-009 |
| [`cross-cutting/sandbox-and-egress-residuals.md`](cross-cutting/sandbox-and-egress-residuals.md) | Sandbox and Egress Residual Hardening (`0.0.0.0` guard gap, port-kill ownership, shell→loopback, platform residuals) | proposed | 2026-09-30 | SPEC-006, SPEC-008, SPEC-009 |
| [`memory/small-context-memory.md`](memory/small-context-memory.md) | Memory for Small-Context Local Models (per-run injection, window-scaled budget, progress ledger, step-aware hints, operator UX) | complete | 2026-10-01 | SPEC-001, SPEC-004, SPEC-005, SPEC-007 |
| [`memory/assistant-memory-capture.md`](memory/assistant-memory-capture.md) | Assistant Memory Capture (A: explicit capture, C: per-chat review, D: gated guidance) | active | 2026-10-05 | SPEC-004, SPEC-001, SPEC-003 |
| [`orchestrator/local-and-cloud-inference-performance.md`](orchestrator/local-and-cloud-inference-performance.md) | Local and Cloud Inference Performance (measure first; KV-cache/prefix stability, timings capture, cloud TTFT) | active | 2026-09-30 | SPEC-001, SPEC-005, SPEC-007 |
| [`cross-cutting/backend-hot-paths-and-leak-hardening.md`](cross-cutting/backend-hot-paths-and-leak-hardening.md) | Backend Hot Paths, Memory Growth and Leak Hardening (gauges, soak test, session checkpoint, list endpoints) | proposed | 2026-09-30 | SPEC-001, SPEC-007 |
| [`assistant-ui/assistant-automation-workbench-layout.md`](assistant-ui/assistant-automation-workbench-layout.md) | Assistant and Automation Workbench Layout (chat+editor split, automation master-detail, one run renderer, context meter) | proposed | 2026-09-30 | SPEC-001, SPEC-003, SPEC-007 |
| [`autonomy/autonomy-roadmap.md`](autonomy/autonomy-roadmap.md) | Autonomous Assistant — Roadmap (umbrella: vision, shared building blocks, milestones M0–M5, cross-plan decisions) | proposed | 2026-10-04 | SPEC-001, SPEC-003, SPEC-004, SPEC-006, SPEC-007, SPEC-009 |
| [`autonomy/autonomy-ux-design.md`](autonomy/autonomy-ux-design.md) | Autonomy UX Design (M0, design-only: Today view, agenda, goals, attention settings, channels, consolidation review, Telegram formats) | proposed | 2026-10-04 | SPEC-003, SPEC-009 |
| [`autonomy/agenda-reminders-and-followups.md`](autonomy/agenda-reminders-and-followups.md) | Agenda — Reminders and Self-Scheduled Follow-ups (M1; introduces shared `RunReason`) | proposed | 2026-10-04 | SPEC-007, SPEC-001, SPEC-009, SPEC-003 |
| [`autonomy/attention-policy.md`](autonomy/attention-policy.md) | Attention Policy — Quiet Hours, Interruption Budget, Digest, Autonomy Spend Cap (M2) | proposed | 2026-10-04 | SPEC-009, SPEC-001, SPEC-005, SPEC-003 |
| [`autonomy/cross-channel-conversations-and-remote-approvals.md`](autonomy/cross-channel-conversations-and-remote-approvals.md) | Cross-Channel Conversations and Remote Approvals (M1 Phase 0 inbound sender-verification fix; M3 continuity + Telegram approve/deny) | proposed | 2026-10-04 | SPEC-009, SPEC-006, SPEC-001, SPEC-003 |
| [`autonomy/standing-goals.md`](autonomy/standing-goals.md) | Standing Goals — outcome-driven, self-scheduling goal ticks with journal, limits, stuck detection (M4) | proposed | 2026-10-04 | SPEC-001, SPEC-010, SPEC-007, SPEC-006, SPEC-004, SPEC-003 |
| [`autonomy/event-driven-triggers.md`](autonomy/event-driven-triggers.md) | Event-Driven Automation Triggers (`file` drop, `watch` for change, `after` run chaining; moved from `automation/`) | proposed | 2026-10-04 | SPEC-007, SPEC-006, SPEC-003 |
| [`memory/memory-consolidation.md`](memory/memory-consolidation.md) | Memory Consolidation — nightly "sleep": extract, merge, demote; reviewable proposals (autonomy M5) | proposed | 2026-10-04 | SPEC-004, SPEC-001, SPEC-007, SPEC-003 |
| [`automation/chat-to-automation.md`](automation/chat-to-automation.md) | "Turn This Chat Into an Automation" (trace → least-privilege draft; Phase 0 fixes `CreateAutomation` duplicate-name bug) | proposed | 2026-10-04 | SPEC-007, SPEC-001, SPEC-003, SPEC-006 |
| [`orchestrator/cost-and-savings-ledger.md`](orchestrator/cost-and-savings-ledger.md) | Usage, Cost and "Saved by Running Locally" Ledger (chat + automation + inbound `/v1`) | proposed | 2026-10-04 | SPEC-005, SPEC-007, SPEC-003, SPEC-001 |
| [`orchestrator/auto-model-router.md`](orchestrator/auto-model-router.md) | `model: "auto"` — Runtime-Aware Model Router for `/v1` | proposed | 2026-10-04 | SPEC-005, SPEC-007, SPEC-003 |
| [`cross-cutting/cloud-privacy-firewall.md`](cross-cutting/cloud-privacy-firewall.md) | Cloud Privacy Firewall (reversible redaction of secrets/PII on cloud egress, or keep it local) | proposed | 2026-10-04 | SPEC-006, SPEC-005, SPEC-001, SPEC-003 |

Completed, superseded, and not-implemented plans live in [`ARCHIVE/`](ARCHIVE/) — loaded only when their specific topic is relevant. The detailed plan archive/consolidation changelog (previously in `docs/INDEX.md`) is retained in that file's git history.

## Remaining Work (non-complete plans)

Filtered view of everything not `complete`. Use this as the live "what's left" tracker — **no separate todo folder**, status stays on the plan itself.

| Status | Plan | Open scope |
|--------|-------|-----------|
| partial | Automation Learning Journal | live-model checks only — Phase 3 (does a small model call `memory_search` first) + Phase 2 smoke run; Phases 1–4 are built (bullet dedup, journal + tool, memory-first prompt, heartbeat active hours) |
| active | GPU Performance | P0–P4 rendering/metrics (P5 unblocked — fix-final-report-realignment landed) |
| active | Connector Auto-Reply | Phase 3 interactive gateway paths |
| active | Post-Implementation Cleanup | execute findings register (dead code/dup sweep) |
| partial | CI & Versioning | P2/P3 hygiene work + P6 (verify tag flow on a real merge); P5 build-release deliberately parked |
| partial | Agent Improvements | re-scoped 2026-09-05: only Phase 4 (tool dedup), Phase 5 (UsageTracker), Phase 7 remainder (sub-agent wrap) still valid — see re-scope note in plan |
| partial | Memory Improvements | Session Search tool (FTS5 `session_search`) + Skill System (procedural memory) — everything else done |
| partial | Assistant Liveness Heartbeat & Package Restructure | §4.3/§4.4 package extractions deferred (§2, §3, §4.1 done; §4.2 rejected) |
| proposed | AGENTS.md Layering & Guardrails | design acceptance + implementation |
| proposed | Agent OS Sandboxing | Phases 1–6 (rlimits → FS jail → network switch → OS network deny → egress proxy → deployment hardening) |
| proposed | Tool-Call Grammar Re-enable | envelope-aware GBNF + opt-in per-model toggle, XML path only |
| active | Search Tool Calling | implemented (Tavily/Brave/SerpAPI providers, Search settings tab, live key, hide-when-unconfigured gate; automated gates green). Open: manual end-to-end verification — Tavily was exercised live (auth fix, note 9); Brave and SerpAPI live calls and the "no key → tool hidden, no restart needed" steps are unconfirmed |
| proposed | Assistant Conversation Package | Step 0 consolidate LLM/tool test doubles, then extract `conversation` (loop core/strategies stay — need a session facade) |
| proposed | SQLite Session Storage | JSON → SQLite migration (deliberately deferred, future work) |
| proposed | Admin / API Authentication | Phase 0 (route-classification + exploit-shaped tests, prove/kill key-exfil via base URL) → Phase 1 zero-config hardening (Host/Origin, drop `ACAO: *`, PID-stop ownership, mandatory webhook secret) → Phases 2–4 need user decisions A1–A7 |
| proposed | Sandbox and Egress Residuals | Phase 1 (guard fix, proxy dial guard, `freePort` ownership) done 2026-10-08 → Phase 2 shell→loopback decision → Phase 3 optional platform items |
| active | Local/Cloud Inference Performance | Phase 0: cross-turn cache loss proved (chat template) and fixed with `preserve_thinking` (turn-2 prefill 17.3 s → 0.8 s, audit 2026-10-09); still to do: timings in run records, cloud TTFT, prefix-stability test, post-fix end-to-end numbers |
| proposed | Backend Hot Paths and Leaks | Phase 0 gauges + build-tagged soak + benchmarks → session-checkpoint decision (A/B/C) → list endpoints |
| proposed | Assistant/Automation Workbench Layout | Phase 0 wireframes + SPEC-003 amendment draft need sign-off → `context_usage` event + SSE-bleed backend fix → automation master-detail/one renderer → workspace workbench |
| proposed | Autonomy Roadmap (umbrella) | tracks M0–M5 across the child plans below; `complete` when they are |
| proposed | Autonomy UX Design | Phase 0 inventory + IA (U1) → low-fi wireframes/storyboards → hi-fi on `/design` → SPEC-011/SPEC-003 drafts; blocks the UI phases of all autonomy plans |
| proposed | Agenda — Reminders and Follow-ups | Phase 0 characterise → `RunReason` → store/scheduler/reminders (D2, D3) → agent tools + follow-ups (D1, D4) → UI → docs |
| proposed | Attention Policy | Phase 0 characterise → pure policy engine → wire `notify_user`/agenda/digest (D1, D2, D4) → spend cap (D3 + ledger) → UI/Telegram controls → docs |
| proposed | Cross-Channel Conversations and Remote Approvals | **Phase 0 security fix first** (inbound sender verification, ctx tethering, timezone) → owner linking → continuity (D1, D2) → interactive connectors + remote approvals (D3, D4) → UI → docs |
| proposed | Standing Goals | Phase 0 tick-report contract → store + journal → tick execution (needs agenda P3) → finish/stuck/deadline → steering/proposals → UI → docs |
| proposed | Event-Driven Automation Triggers | Phase 0 characterisation → Phase 1 `EventSource` + `after` (reuses `RunReason`) → Phase 2 `file` (needs D1) → Phase 3 `watch` (needs D2) → Phase 4 API/UI/SPEC-007 |
| proposed | Memory Consolidation | Phase 0 hygiene + proposal store → LLM extraction/merge → manual run → nightly schedule (needs agenda + attention) → review UI → docs |
| proposed | Chat → Automation | Phase 0 (`CreateAutomation` duplicate-name fix) done 2026-10-08 → trace extraction → draft endpoint → UI → docs |
| proposed | Cost and Savings Ledger | Phase 0 measure provider usage reporting → automation token capture → store/API → inbound `/v1` capture → prices/UI → alerts (D4) |
| proposed | Auto Model Router | Phase 0 characterise → pure router core → `/v1` wiring (needs D1, D2) → pre-first-byte failover → UI/docs → ledger prices |
| proposed | Cloud Privacy Firewall | Phase 0 detector corpus → pure engine → agent path (needs D1–D3) → `/v1` path → `ask` mode/UI/audit → router integration → docs |

> **2026-09-05 hygiene pass:** archived as complete — `knight-rider-arc-bubble.md` (extraction to `ArcOrbitLoader` verified in code), `cloud-provider-token-budgets.md` (all phases incl. merged Phase 7 + M8 probe verified). Archived as merged — `cancel-stale-turn-bleed.md` (backend SSE bleed now homed in overhaul Phase 5). `xdg-config-data-relocation.md` removed from this table (already complete, row was stale).
>
> **2026-09-25 consolidation:** the completed `cross-cutting/global-run-lane-scheduler.md` and `cross-cutting/inbound-request-admission.md` were folded into **SPEC-007 §V/§V.1** (run scheduler + model-residency contract) plus `docs/architecture.md` (package layout, pitfalls #34–#36) and the `automation` skill, then removed from `docs/PLANS/` — their design rationale remains in git history.

See [`docs/INDEX.md`](../INDEX.md) for the catalog router (SPECs → `docs/SPECS/README.md`, audits → `docs/audits/README.md`, skills → `.agents/skills/`).
