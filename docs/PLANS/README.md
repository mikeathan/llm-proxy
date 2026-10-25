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
| [`cross-cutting/frontend-redesign-v2.md`](cross-cutting/frontend-redesign-v2.md) | Frontend Redesign v2 — assistant-first shell + ask panel, Settings tabs, calmer look, more themes, phone bottom tabs | partial | 2026-10-14 | SPEC-003, SPEC-007 |
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
| [`cross-cutting/run-recordings.md`](cross-cutting/run-recordings.md) | Make run recordings readable and safe (run logging already records every request; masked on write, listed without `--record`, content endpoint) | complete | 2026-10-20 | SPEC-001, SPEC-007 |
| [`cross-cutting/agents-md-layering-guardrails.md`](cross-cutting/agents-md-layering-guardrails.md) | AGENTS.md Layering, Override-ability & Write Guardrails | partial | 2026-08-04 | SPEC-001, CONSTITUTION II.13/II.10 |
| [`assistant-ui/composer-queue-and-reminder-alerts.md`](assistant-ui/composer-queue-and-reminder-alerts.md) | Composer message queue while a run is live, and live web UI alerts when a reminder fires (owner phone testing, 2026-10-18) | complete | 2026-10-18 | SPEC-003, SPEC-011, SPEC-001 |
| [`agent-loop/run-efficiency-and-convergence.md`](agent-loop/run-efficiency-and-convergence.md) | Run Efficiency and Convergence (pair-atomic pruning, result shaping, result-level progress monitor, batching; from a measured 141 s run; pairing fix + observability done) | partial | 2026-10-18 | SPEC-001, SPEC-006, SPEC-010 |
| [`agent-loop/large-result-storage.md`](agent-loop/large-result-storage.md) | Keep large tool results instead of cutting them (ranged `read_file` built in #99; automation/chat reports saved under the workspace `reports/`; operator-only purge; link + side sheet) | partial | 2026-10-19 | SPEC-001, SPEC-003, SPEC-004, SPEC-006, SPEC-007 |
| [`agent-loop/request-size-estimator.md`](agent-loop/request-size-estimator.md) | Request size estimator — one request measure scaled by the server's own token counts, typed overflow error, one-step recovery (vertex 16k overflow, 2026-10-17) | complete | 2026-10-18 | SPEC-001, SPEC-005, SPEC-004 |
| [`agent-loop/multi-agent-delegation.md`](agent-loop/multi-agent-delegation.md) | Multi-Agent Delegation — one run pipeline for chat and automation (characterisation tests, one resolver, shared run helpers), then cloud web-research sub-agents (capability table, profile `delegates`, `SubRunner` port, sub-agent pool, eval); R0 category drift fixed | active | 2026-10-22 | SPEC-001, SPEC-007, SPEC-012, SPEC-006, SPEC-004 |
| [`agent-loop/run-pipeline-and-agent-commands.md`](agent-loop/run-pipeline-and-agent-commands.md) | Run pipeline cleanup and agent commands — Part C of delegation: dead code, one chat service (lifecycle out of the handler, ≤4 parameters), `/research` and `/agent` turn commands for every channel, merged duplicates, package moves | complete | 2026-10-24 | SPEC-001, SPEC-007, SPEC-009, SPEC-012, SPEC-003 |
| [`agent-loop/agents-editor.md`](agent-loop/agents-editor.md) | Agents editor — a Settings section to create, duplicate, edit and delete profiles and helpers (same `profiles/*.md` files; `GET/PUT/DELETE /admin/api/profiles/{id}`, schema endpoint) | complete | 2026-10-23 | SPEC-012, SPEC-001, SPEC-003 |
| [`agent-loop/use-helpers-control.md`](agent-loop/use-helpers-control.md) | Use Helpers — a composer switch that makes a research chat delegate (a local model never does on its own: 0/5 unprompted, 5/5 when asked); `use_helpers` note on the message, `GET /admin/api/helpers`, third row in Run settings | complete | 2026-10-22 | SPEC-001, SPEC-003, SPEC-012 |
| [`cross-cutting/prompt-profiles.md`](cross-cutting/prompt-profiles.md) | Prompt Profiles — selectable instruction bundles for assistant and automation (generic default, loop policy flags, per-turn prompt rebuild) | active | 2026-10-17 | SPEC-012, SPEC-001, SPEC-007, SPEC-004, CONSTITUTION II.13 |
| [`cross-cutting/tool-call-grammar-reenable.md`](cross-cutting/tool-call-grammar-reenable.md) | Re-enable Tool-Call Grammar Constraint (opt-in, llama.cpp-safe) | proposed | 2026-09-05 | SPEC-001, SPEC-002 |
| [`cross-cutting/search-tool-calling.md`](cross-cutting/search-tool-calling.md) | Wire up `internet_search` tool calling (pluggable multi-provider, live key, hide-when-unconfigured) | active | 2026-09-12 | SPEC-001, SPEC-006 |
| [`cross-cutting/tool-error-classification.md`](cross-cutting/tool-error-classification.md) | Tool Error Classification & Run-Fatality Policy (terminal tool errors, delivery-vs-essential, failure bound) | complete | 2026-09-12 | SPEC-001, SPEC-010, SPEC-006 |
| [`cross-cutting/assistant-conversation-package.md`](cross-cutting/assistant-conversation-package.md) | Assistant Conversation Package (deferred extraction; done instead as C5 of the run-pipeline plan, into `core/chat`) | superseded | 2026-10-23 | SPEC-001 |
| [`cross-cutting/persist-assistant-run-state-for-reload.md`](cross-cutting/persist-assistant-run-state-for-reload.md) | Persist assistant run state (errors/cancels/running) for reliable reload | complete | 2026-08-20 | SPEC-001, SPEC-003 |
| [`cross-cutting/sqlite-session-storage.md`](cross-cutting/sqlite-session-storage.md) | SQLite session storage (future work, proposed) | proposed | 2026-08-20 | SPEC-001 |
| [`cross-cutting/xdg-config-data-relocation.md`](cross-cutting/xdg-config-data-relocation.md) | XDG Config/Data Relocation + Storage Cleanup + Reset Controls (Phases 0–7, 9–12 complete; Phase 8 removed; reset/clear-runtime-data hardened) | complete | 2026-08-07 | CONSTITUTION III.2/III.4/III.6 |
| [`memory/memory-improvements-implementation-plan.md`](memory/memory-improvements-implementation-plan.md) | Memory Improvements | partial | — | SPEC-004 |
| [`automation/automation-learning-journal.md`](automation/automation-learning-journal.md) | Automation Learning Journal (bullet dedup, per-automation journal, memory-first, active hours) | partial | 2026-10-03 | SPEC-007, SPEC-004 |
| [`automation/memory-and-heartbeat-simplification.md`](automation/memory-and-heartbeat-simplification.md) | Memory defaults + overrides (assistant/automation) and a simple per-workspace Heartbeat section | complete | 2026-10-04 | SPEC-004, SPEC-007, SPEC-003 |
| [`cross-cutting/admin-api-authentication.md`](cross-cutting/admin-api-authentication.md) | Admin / API Authentication and Control-Plane Hardening (no auth today; Host/Origin checks, operator token, `/v1` keys, safer bind) | proposed | 2026-09-30 | SPEC-003, SPEC-006, SPEC-007, SPEC-009 |
| [`cross-cutting/network-audit.md`](cross-cutting/network-audit.md) | Network audit (guarded programs, an nmap `network_scan` tool in a new `netaudit` category, one tool catalog, scan-scope list, nightly Security auditor agent with acknowledgements; auth work parked in the plan above) | proposed | 2026-10-24 | SPEC-006, SPEC-007, SPEC-012, SPEC-001, SPEC-003, SPEC-009 |
| [`cross-cutting/sandbox-and-egress-residuals.md`](cross-cutting/sandbox-and-egress-residuals.md) | Sandbox and Egress Residual Hardening (`0.0.0.0` guard gap, port-kill ownership, shell→loopback, platform residuals) | proposed | 2026-09-30 | SPEC-006, SPEC-008, SPEC-009 |
| [`memory/small-context-memory.md`](memory/small-context-memory.md) | Memory for Small-Context Local Models (per-run injection, window-scaled budget, progress ledger, step-aware hints, operator UX) | complete | 2026-10-01 | SPEC-001, SPEC-004, SPEC-005, SPEC-007 |
| [`memory/assistant-memory-capture.md`](memory/assistant-memory-capture.md) | Assistant Memory Capture (A: explicit capture, C: per-chat review, D: gated guidance) | active | 2026-10-05 | SPEC-004, SPEC-001, SPEC-003 |
| [`orchestrator/local-and-cloud-inference-performance.md`](orchestrator/local-and-cloud-inference-performance.md) | Local and Cloud Inference Performance (measure first; KV-cache/prefix stability, timings capture, cloud TTFT) | active | 2026-09-30 | SPEC-001, SPEC-005, SPEC-007 |
| [`cross-cutting/backend-hot-paths-and-leak-hardening.md`](cross-cutting/backend-hot-paths-and-leak-hardening.md) | Backend Hot Paths, Memory Growth and Leak Hardening (gauges, soak test, session checkpoint, list endpoints) | proposed | 2026-09-30 | SPEC-001, SPEC-007 |
| [`assistant-ui/assistant-automation-workbench-layout.md`](assistant-ui/assistant-automation-workbench-layout.md) | Assistant and Automation Workbench Layout (chat+editor split, automation master-detail, one run renderer, context meter) | proposed | 2026-09-30 | SPEC-001, SPEC-003, SPEC-007 |
| [`autonomy/autonomy-roadmap.md`](autonomy/autonomy-roadmap.md) | Autonomous Assistant — Roadmap (umbrella: vision, shared building blocks, milestones M0–M5, cross-plan decisions) | proposed | 2026-10-04 | SPEC-001, SPEC-003, SPEC-004, SPEC-006, SPEC-007, SPEC-009 |
| [`autonomy/autonomy-ux-design.md`](autonomy/autonomy-ux-design.md) | Autonomy UX Design (M0, design-only: Today view, agenda, goals, attention settings, channels, consolidation review, Telegram formats) | proposed | 2026-10-04 | SPEC-003, SPEC-009 |
| [`autonomy/agenda-reminders-and-followups.md`](autonomy/agenda-reminders-and-followups.md) | Agenda — Reminders and Self-Scheduled Follow-ups (M1; introduces shared `RunReason`; reminders built, SPEC-011) | partial | 2026-10-04 | SPEC-007, SPEC-001, SPEC-009, SPEC-003 |
| [`autonomy/attention-policy.md`](autonomy/attention-policy.md) | Attention Policy — Quiet Hours, Interruption Budget, Digest, Autonomy Spend Cap (M2) | proposed | 2026-10-04 | SPEC-009, SPEC-001, SPEC-005, SPEC-003 |
| [`autonomy/cross-channel-conversations-and-remote-approvals.md`](autonomy/cross-channel-conversations-and-remote-approvals.md) | Cross-Channel Conversations and Remote Approvals (M1 Phase 0 inbound sender-verification fix; M3 continuity + Telegram approve/deny) | proposed | 2026-10-04 | SPEC-009, SPEC-006, SPEC-001, SPEC-003 |
| [`autonomy/standing-goals.md`](autonomy/standing-goals.md) | Standing Goals — outcome-driven, self-scheduling goal ticks with journal, limits, stuck detection (M4) | proposed | 2026-10-04 | SPEC-001, SPEC-010, SPEC-007, SPEC-006, SPEC-004, SPEC-003 |
| [`autonomy/event-driven-triggers.md`](autonomy/event-driven-triggers.md) | Event-Driven Automation Triggers (`file` drop, `watch` for change, `after` run chaining; moved from `automation/`) | proposed | 2026-10-04 | SPEC-007, SPEC-006, SPEC-003 |
| [`memory/memory-consolidation.md`](memory/memory-consolidation.md) | Memory Consolidation — nightly "sleep": extract, merge, demote; reviewable proposals (autonomy M5) | proposed | 2026-10-04 | SPEC-004, SPEC-001, SPEC-007, SPEC-003 |
| [`memory/memory-gaps-plan.md`](memory/memory-gaps-plan.md) | Memory Gaps — close the "knows me" loop: evaluation, tool contract, save guidance, recall, hot budget, typed facts, consolidation | active | 2026-10-16 | SPEC-004, SPEC-001, SPEC-005 |
| [`automation/chat-to-automation.md`](automation/chat-to-automation.md) | "Turn This Chat Into an Automation" (trace → least-privilege draft; Phase 0 fixes `CreateAutomation` duplicate-name bug) | proposed | 2026-10-04 | SPEC-007, SPEC-001, SPEC-003, SPEC-006 |
| [`orchestrator/cost-and-savings-ledger.md`](orchestrator/cost-and-savings-ledger.md) | Usage, Cost and "Saved by Running Locally" Ledger (chat + automation + inbound `/v1`) | proposed | 2026-10-04 | SPEC-005, SPEC-007, SPEC-003, SPEC-001 |
| [`orchestrator/auto-model-router.md`](orchestrator/auto-model-router.md) | `model: "auto"` — Runtime-Aware Model Router for `/v1` | proposed | 2026-10-04 | SPEC-005, SPEC-007, SPEC-003 |
| [`orchestrator/cloud-safety-threshold-decoupling.md`](orchestrator/cloud-safety-threshold-decoupling.md) | Decouple Cloud Safety Thresholds From `max_tokens` (so a real published output cap can be trusted without loosening stuck detection) — cloud only, local explicitly out of scope | proposed | 2026-10-11 | SPEC-005, SPEC-001 |
| [`cross-cutting/cloud-privacy-firewall.md`](cross-cutting/cloud-privacy-firewall.md) | Cloud Privacy Firewall (reversible redaction of secrets/PII on cloud egress, or keep it local) | proposed | 2026-10-04 | SPEC-006, SPEC-005, SPEC-001, SPEC-003 |
| [`automation/playbook-library-management.md`](automation/playbook-library-management.md) | Playbook Library Management (add, remove, edit operator playbooks; shipped set stays read-only and embedded) | proposed | 2026-10-10 | SPEC-007, SPEC-003 |
| [`automation/heartbeat-seen-ledger.md`](automation/heartbeat-seen-ledger.md) | Heartbeat Repeat Suppression (opt-in seen ledger instead of model-written memory; contract, delivery counts, honest status, live eval before cross-source work) | proposed | 2026-10-12 | SPEC-007, SPEC-004 |
| [`automation/automation-authoring-ux.md`](automation/automation-authoring-ux.md) | Automation Authoring UX (schedule picker so no cron knowledge is needed, server schedule preview + timezone, write/pick the task in the form, next run + test run, create-from-playbook) | complete | 2026-10-11 | SPEC-007, SPEC-003 |

Completed, superseded, and not-implemented plans live in [`ARCHIVE/`](ARCHIVE/) — loaded only when their specific topic is relevant. The detailed plan archive/consolidation changelog (previously in `docs/INDEX.md`) is retained in that file's git history.

## Remaining Work (non-complete plans)

Filtered view of everything not `complete`. Use this as the live "what's left" tracker — **no separate todo folder**, status stays on the plan itself.

| Status | Plan | Open scope |
|--------|-------|-----------|
| complete | Run pipeline cleanup and agent commands | All phases merged and checked live (#129–#134): one chat service in `core/chat`, `run.ResolveOptions`, `/research`, `/agent` and `/run` in the command registry, the 12 KB history cut fixed; left: the Telegram live check only, postponed (`agent-loop/run-pipeline-and-agent-commands.md`) |
| active | Multi-Agent Delegation | R0 + Part A done; Part B built (cloud web-research sub-agents: `delegate_task`, `delegation.Runner`, pool, profile `delegates`, Settings + chat UI) — B7 eval done 2026-10-23 (helpers kept); overflow count recorded (#126), Agents editor built (#128); Part C handler lifecycle + package moves done (#129, #130); left: reviewer, `on_finish`, files, pipelines (`agent-loop/multi-agent-delegation.md`) |
| active | Prompt Profiles | built (Phases 0–6); left: eval the Research, Operator, Builder and Watcher profiles, fold in the layering plan's write guardrail, delete the dead `GetSystemPrompt` chain (`cross-cutting/prompt-profiles.md`) |
| partial | Automation Learning Journal | live-model checks only — Phase 3 (does a small model call `memory_search` first) + Phase 2 smoke run; Phases 1–4 are built (bullet dedup, journal + tool, memory-first prompt, heartbeat active hours) |
| partial | Frontend Redesign v2 | Phases 0–5 delivered 2026-10-14; left: Automations/Reminders/Models/Activity on a phone, arrow-key handling + focus trap in the new sheets, "All workspaces" scope decision, recent-chats menu in the ask panel |
| active | GPU Performance | P0–P4 rendering/metrics (P5 unblocked — fix-final-report-realignment landed) |
| active | Connector Auto-Reply | Phase 3 interactive gateway paths |
| active | Post-Implementation Cleanup | execute findings register (dead code/dup sweep) |
| partial | CI & Versioning | P2/P3 hygiene work + P6 (verify tag flow on a real merge); P5 build-release deliberately parked |
| partial | Agent Improvements | re-scoped 2026-09-05: only Phase 4 (tool dedup), Phase 5 (UsageTracker), Phase 7 remainder (sub-agent wrap) still valid — see re-scope note in plan |
| partial | Memory Improvements | Session Search tool (FTS5 `session_search`) + Skill System (procedural memory) — everything else done |
| partial | Assistant Liveness Heartbeat & Package Restructure | §4.3/§4.4 package extractions deferred (§2, §3, §4.1 done; §4.2 rejected) |
| partial | Large Result Storage | Phase 0 ranged `read_file` built (#99; live model check open); Phase 1 step 1 read-only checks next; §4 decisions made 2026-10-20 (chat reports in the workspace, clear-runtime-data keeps automation reports, no saving into read-only workspaces); then spill, purge, UI; Phase 6 (demote old results to recoverable stubs under pressure) after Phases 1–2 |
| proposed | AGENTS.md Layering & Guardrails | design acceptance + implementation |
| complete (pending Linux-CI) | Agent OS Sandboxing | Phases 0–4 shipped (plan §10); left: Linux-CI confirmation of the Landlock probe, optional macOS Seatbelt on framework-capable hardware |
| proposed | Tool-Call Grammar Re-enable | envelope-aware GBNF + opt-in per-model toggle, XML path only |
| active | Search Tool Calling | implemented (Tavily/Brave/SerpAPI providers, Search settings tab, live key, hide-when-unconfigured gate; automated gates green). Open: manual end-to-end verification — Tavily was exercised live (auth fix, note 9); Brave and SerpAPI live calls and the "no key → tool hidden, no restart needed" steps are unconfirmed |
| proposed | SQLite Session Storage | JSON → SQLite migration (deliberately deferred, future work) |
| proposed | Admin / API Authentication | Phase 0 (route-classification + exploit-shaped tests, prove/kill key-exfil via base URL) → Phase 1 zero-config hardening (Host/Origin, drop `ACAO: *`, PID-stop ownership, mandatory webhook secret) → Phases 2–4 need user decisions A1–A7 |
| proposed | Sandbox and Egress Residuals | Phase 1 (guard fix, proxy dial guard, `freePort` ownership) done 2026-10-08 → Phase 2 shell→loopback decision → Phase 3 optional platform items |
| active | Local/Cloud Inference Performance | Phase 0: cross-turn cache loss proved (chat template) and fixed with `preserve_thinking` (turn-2 prefill 17.3 s → 0.8 s, audit 2026-10-09); still to do: timings in run records, cloud TTFT, prefix-stability test, post-fix end-to-end numbers |
| proposed | Heartbeat Repeat Suppression | contract text + docs → delivery counts in logs/run record → honest "no channel" heartbeat status → live three-run eval, then decide stale-prose and cross-source work |
| proposed | Network Audit | Phase 0 characterise + verify (gating table test, executor failure path, nmap under Landlock) → merge the tool catalog → seams (workspace document, `ToolModule`) → safety gates (amendment, scan scope, `local_only`) → nmap engine + `network_scan` (delete `scan_local_network`) → auditor + nightly run → acknowledgements + report UI → deeper modes → docs; nothing built |
| proposed | Backend Hot Paths and Leaks | Phase 0 gauges + build-tagged soak + benchmarks → session-checkpoint decision (A/B/C) → list endpoints |
| proposed | Assistant/Automation Workbench Layout | Phase 0 wireframes + SPEC-003 amendment draft need sign-off → `context_usage` event + SSE-bleed backend fix → automation master-detail/one renderer → workspace workbench |
| proposed | Autonomy Roadmap (umbrella) | tracks M0–M5 across the child plans below; `complete` when they are |
| proposed | Autonomy UX Design | Phase 0 inventory + IA (U1) → low-fi wireframes/storyboards → hi-fi on `/design` → SPEC-011/SPEC-003 drafts; blocks the UI phases of all autonomy plans |
| partial | Agenda — Reminders and Follow-ups | reminders done (SPEC-011); left: `RunReason` → `schedule_followup` + `action: run` (D1, D4) → snooze → Agenda/Today UI |
| proposed | Attention Policy | Phase 0 characterise → pure policy engine → wire `notify_user`/agenda/digest (D1, D2, D4) → spend cap (D3 + ledger) → UI/Telegram controls → docs |
| proposed | Cross-Channel Conversations and Remote Approvals | **Phase 0 security fix first** (inbound sender verification, ctx tethering, timezone) → owner linking → continuity (D1, D2) → interactive connectors + remote approvals (D3, D4) → UI → docs |
| proposed | Standing Goals | Phase 0 tick-report contract → store + journal → tick execution (needs agenda P3) → finish/stuck/deadline → steering/proposals → UI → docs |
| proposed | Event-Driven Automation Triggers | Phase 0 characterisation → Phase 1 `EventSource` + `after` (reuses `RunReason`) → Phase 2 `file` (needs D1) → Phase 3 `watch` (needs D2) → Phase 4 API/UI/SPEC-007 |
| proposed | Memory Consolidation | Phase 0 hygiene + proposal store → LLM extraction/merge → manual run → nightly schedule (needs agenda + attention) → review UI → docs |
| active | Memory Gaps | Phases 0–4 done (P2 and P3 with documented ceilings); P5 typed facts → P6 consolidation open; D2–D6 approved, D1 refined; P2 now led by the local model not saving statement + question — see the plan |
| proposed | Chat → Automation | Phase 0 (`CreateAutomation` duplicate-name fix) done 2026-10-08 → trace extraction → draft endpoint → UI → docs |
| proposed | Cost and Savings Ledger | Phase 0 measure provider usage reporting → automation token capture → store/API → inbound `/v1` capture → prices/UI → alerts (D4) |
| proposed | Auto Model Router | Phase 0 characterise → pure router core → `/v1` wiring (needs D1, D2) → pre-first-byte failover → UI/docs → ledger prices |
| proposed | Cloud Privacy Firewall | Phase 0 detector corpus → pure engine → agent path (needs D1–D3) → `/v1` path → `ask` mode/UI/audit → router integration → docs |
| proposed | Playbook Library Management | Phase 0 investigate (where operator playbooks live, override vs fork, upgrade hint, validation) → backend store + API → UI → docs; nothing built |
| proposed | Cloud Safety Threshold Decoupling | Phase 0 measure (real published caps, reasoning-trace lengths, existing sanity bounds) → Phase 1 design decisions D1–D3 → Phase 2 implement (new safety ceiling, TDD, local-regression guard) → Phase 3 lift the cloud output-cap tier ceiling → Phase 4 docs; nothing built, cloud-only, local explicitly out of scope |

> **2026-09-05 hygiene pass:** archived as complete — `knight-rider-arc-bubble.md` (extraction to `ArcOrbitLoader` verified in code), `cloud-provider-token-budgets.md` (all phases incl. merged Phase 7 + M8 probe verified). Archived as merged — `cancel-stale-turn-bleed.md` (backend SSE bleed now homed in overhaul Phase 5). `xdg-config-data-relocation.md` removed from this table (already complete, row was stale).
>
> **2026-09-25 consolidation:** the completed `cross-cutting/global-run-lane-scheduler.md` and `cross-cutting/inbound-request-admission.md` were folded into **SPEC-007 §V/§V.1** (run scheduler + model-residency contract) plus `docs/architecture.md` (package layout, pitfalls #34–#36) and the `automation` skill, then removed from `docs/PLANS/` — their design rationale remains in git history.

See [`docs/INDEX.md`](../INDEX.md) for the catalog router (SPECs → `docs/SPECS/README.md`, audits → `docs/audits/README.md`, skills → `.agents/skills/`).
