# Implementation Plans — live tracker

Plans are **future and in-flight work**: how a change will be built, phase by phase. This file lists only
plans that are not finished. Finished plans live in [`ARCHIVE/`](ARCHIVE/README.md) (load on demand).

Where other knowledge lives (see [`docs/INDEX.md`](../INDEX.md)): how the system works now → `docs/SPECS/` and
`docs/architecture/`; why it is this way → `docs/decisions/` (ADRs); what we tried and learned →
`docs/learnings/`; one page per subsystem tying these together → `docs/subsystems/`.

## Rules

- **Status** (first word of the plan's own `**Status:**` line, mirrored in the table below — the harness
  checks they agree): `proposed` (design pending) · `approved` · `active` (in flight) · `partial` (some
  phases merged) · `complete` · `superseded` · `reverted`.
- **Shape:** a front page — status, related SPECs/ADRs, phases each with an acceptance check, **Remaining
  Work** — kept under ~15 KB. Split long designs by phase.
- **Harvest, then archive.** When a phase merges, move what it settled out of the plan: as-built behavior →
  SPEC / `docs/architecture/`; decisions → an ADR in `docs/decisions/`; lessons and dead ends → a line in
  `docs/learnings/<subsystem>.md`. When the plan is `complete`/`superseded`, `git mv` it to `ARCHIVE/<folder>/`,
  add its row to [`ARCHIVE/README.md`](ARCHIVE/README.md), and delete its rows here. No `complete` row stays in
  this file (the harness fails on one).
- **New plan:** `docs/PLANS/<folder>/<kebab-name>.md` and a row in the table below.

Folders: `agent-loop/` (SPEC-001/010) · `assistant-ui/` (SPEC-003) · `automation/` (SPEC-007) · `autonomy/`
(always-on assistant; umbrella [`autonomy/autonomy-roadmap.md`](autonomy/autonomy-roadmap.md) owns the order) ·
`memory/` (SPEC-004) · `orchestrator/` (SPEC-005) · `cross-cutting/` (multi-SPEC or standalone).

## Open plans

One row per plan; **What's left** is the live "what's left" view (status stays on the plan itself — the
harness checks the plan's own status line matches this table).

| Plan | Status | Date | Related Specs | What's left |
|------|--------|------|---------------|-------------|
| [Agent Improvements](agent-loop/agent-improvements.md) | partial | — | SPEC-001 | re-scoped 2026-09-05: only Phase 4 (tool dedup), Phase 5 (UsageTracker), Phase 7 remainder (sub-agent wrap) still valid — see re-scope note in plan |
| [Large Result Storage](agent-loop/large-result-storage.md) | partial | 2026-10-19 | SPEC-001, SPEC-003, SPEC-004, SPEC-006, SPEC-007 | Phase 0 ranged `read_file` built (#99; live model check open); Phase 1 step 1 read-only checks next; §4 decisions made 2026-10-20 (chat reports in the workspace, clear-runtime-data keeps automation reports, no saving into read-only workspaces); then spill, purge, UI; Phase 6 (demote old results to recoverable stubs under pressure) after Phases 1–2 |
| [Multi-Agent Delegation](agent-loop/multi-agent-delegation.md) | active | 2026-10-22 | SPEC-001, SPEC-007, SPEC-012, SPEC-006, SPEC-004 | R0 + Part A done; Part B built (cloud web-research sub-agents: `delegate_task`, `delegation.Runner`, pool, profile `delegates`, Settings + chat UI) — B7 eval done 2026-10-23 (helpers kept); overflow count recorded (#126), Agents editor built (#128); Part C handler lifecycle + package moves done (#129, #130); left: reviewer, `on_finish`, files, pipelines (`agent-loop/multi-agent-delegation.md`) |
| [Run Efficiency and Convergence](agent-loop/run-efficiency-and-convergence.md) | partial | 2026-10-18 | SPEC-001, SPEC-006, SPEC-010 | see the plan's Remaining Work |
| [Assistant/Automation Workbench Layout](assistant-ui/assistant-automation-workbench-layout.md) | proposed | 2026-09-30 | SPEC-001, SPEC-003, SPEC-007 | Phase 0 wireframes + SPEC-003 amendment draft need sign-off → `context_usage` event + SSE-bleed backend fix → automation master-detail/one renderer → workspace workbench |
| [Automation Learning Journal](automation/automation-learning-journal.md) | partial | 2026-10-03 | SPEC-007, SPEC-004 | live-model checks only — Phase 3 (does a small model call `memory_search` first) + Phase 2 smoke run; Phases 1–4 are built (bullet dedup, journal + tool, memory-first prompt, heartbeat active hours) |
| [Chat → Automation](automation/chat-to-automation.md) | proposed | 2026-10-04 | SPEC-007, SPEC-001, SPEC-003, SPEC-006 | Phase 0 (`CreateAutomation` duplicate-name fix) done 2026-10-08 → trace extraction → draft endpoint → UI → docs |
| [Heartbeat Repeat Suppression](automation/heartbeat-seen-ledger.md) | proposed | 2026-10-12 | SPEC-007, SPEC-004 | contract text + docs → delivery counts in logs/run record → honest "no channel" heartbeat status → live three-run eval, then decide stale-prose and cross-source work |
| [Playbook Library Management](automation/playbook-library-management.md) | proposed | 2026-10-10 | SPEC-007, SPEC-003 | Phase 0 investigate (where operator playbooks live, override vs fork, upgrade hint, validation) → backend store + API → UI → docs; nothing built |
| [Agenda — Reminders and Follow-ups](autonomy/agenda-reminders-and-followups.md) | partial | 2026-10-04 | SPEC-007, SPEC-001, SPEC-009, SPEC-003 | reminders done (SPEC-011); left: `RunReason` → `schedule_followup` + `action: run` (D1, D4) → snooze → Agenda/Today UI |
| [Attention Policy](autonomy/attention-policy.md) | proposed | 2026-10-04 | SPEC-009, SPEC-001, SPEC-005, SPEC-003 | Phase 0 characterise → pure policy engine → wire `notify_user`/agenda/digest (D1, D2, D4) → spend cap (D3 + ledger) → UI/Telegram controls → docs |
| [Autonomy Roadmap (umbrella)](autonomy/autonomy-roadmap.md) | proposed | 2026-10-04 | SPEC-001, SPEC-003, SPEC-004, SPEC-006, SPEC-007, SPEC-009 | tracks M0–M5 across the child plans below; `complete` when they are |
| [Autonomy UX Design](autonomy/autonomy-ux-design.md) | proposed | 2026-10-04 | SPEC-003, SPEC-009 | Phase 0 inventory + IA (U1) → low-fi wireframes/storyboards → hi-fi on `/design` → SPEC-011/SPEC-003 drafts; blocks the UI phases of all autonomy plans |
| [Cross-Channel Conversations and Remote Approvals](autonomy/cross-channel-conversations-and-remote-approvals.md) | proposed | 2026-10-04 | SPEC-009, SPEC-006, SPEC-001, SPEC-003 | **Phase 0 security fix first** (inbound sender verification, ctx tethering, timezone) → owner linking → continuity (D1, D2) → interactive connectors + remote approvals (D3, D4) → UI → docs |
| [Event-Driven Automation Triggers](autonomy/event-driven-triggers.md) | proposed | 2026-10-04 | SPEC-007, SPEC-006, SPEC-003 | Phase 0 characterisation → Phase 1 `EventSource` + `after` (reuses `RunReason`) → Phase 2 `file` (needs D1) → Phase 3 `watch` (needs D2) → Phase 4 API/UI/SPEC-007 |
| [Standing Goals](autonomy/standing-goals.md) | proposed | 2026-10-04 | SPEC-001, SPEC-010, SPEC-007, SPEC-006, SPEC-004, SPEC-003 | Phase 0 tick-report contract → store + journal → tick execution (needs agenda P3) → finish/stuck/deadline → steering/proposals → UI → docs |
| [Assistant Memory Capture](memory/assistant-memory-capture.md) | active | 2026-10-05 | SPEC-004, SPEC-001, SPEC-003 | see the plan's Remaining Work |
| [Memory Consolidation](memory/memory-consolidation.md) | proposed | 2026-10-04 | SPEC-004, SPEC-001, SPEC-007, SPEC-003 | Phase 0 hygiene + proposal store → LLM extraction/merge → manual run → nightly schedule (needs agenda + attention) → review UI → docs |
| [Memory Gaps](memory/memory-gaps-plan.md) | active | 2026-10-16 | SPEC-004, SPEC-001, SPEC-005 | Phases 0–4 done (P2 and P3 with documented ceilings); P5 typed facts → P6 consolidation open; D2–D6 approved, D1 refined; P2 now led by the local model not saving statement + question — see the plan |
| [Memory Improvements](memory/memory-improvements-implementation-plan.md) | partial | — | SPEC-004 | Session Search tool (FTS5 `session_search`) + Skill System (procedural memory) — everything else done |
| [Auto Model Router](orchestrator/auto-model-router.md) | proposed | 2026-10-04 | SPEC-005, SPEC-007, SPEC-003 | Phase 0 characterise → pure router core → `/v1` wiring (needs D1, D2) → pre-first-byte failover → UI/docs → ledger prices |
| [Cloud Safety Threshold Decoupling](orchestrator/cloud-safety-threshold-decoupling.md) | proposed | 2026-10-11 | SPEC-005, SPEC-001 | Phase 0 measure (real published caps, reasoning-trace lengths, existing sanity bounds) → Phase 1 design decisions D1–D3 → Phase 2 implement (new safety ceiling, TDD, local-regression guard) → Phase 3 lift the cloud output-cap tier ceiling → Phase 4 docs; nothing built, cloud-only, local explicitly out of scope |
| [Cost and Savings Ledger](orchestrator/cost-and-savings-ledger.md) | proposed | 2026-10-04 | SPEC-005, SPEC-007, SPEC-003, SPEC-001 | Phase 0 measure provider usage reporting → automation token capture → store/API → inbound `/v1` capture → prices/UI → alerts (D4) |
| [Local/Cloud Inference Performance](orchestrator/local-and-cloud-inference-performance.md) | active | 2026-09-30 | SPEC-001, SPEC-005, SPEC-007 | Phase 0: cross-turn cache loss proved (chat template) and fixed with `preserve_thinking` (turn-2 prefill 17.3 s → 0.8 s, audit 2026-10-09); still to do: timings in run records, cloud TTFT, prefix-stability test, post-fix end-to-end numbers |
| [Admin / API Authentication](cross-cutting/admin-api-authentication.md) | proposed | 2026-09-30 | SPEC-003, SPEC-006, SPEC-007, SPEC-009 | Phase 0 (route-classification + exploit-shaped tests, prove/kill key-exfil via base URL) → Phase 1 zero-config hardening (Host/Origin, drop `ACAO: *`, PID-stop ownership, mandatory webhook secret) → Phases 2–4 need user decisions A1–A7 |
| [AGENTS.md Layering & Guardrails](cross-cutting/agents-md-layering-guardrails.md) | partial | 2026-08-04 | SPEC-001, CONSTITUTION II.13/II.10 | design acceptance + implementation |
| [Assistant Liveness Heartbeat & Package Restructure](cross-cutting/assistant-liveness-heartbeat-package-split.md) | partial | 2026-08-18 | SPEC-001, SPEC-010, SPEC-003 | §4.3/§4.4 package extractions deferred (§2, §3, §4.1 done; §4.2 rejected) |
| [Backend Hot Paths and Leaks](cross-cutting/backend-hot-paths-and-leak-hardening.md) | proposed | 2026-09-30 | SPEC-001, SPEC-007 | Phase 0 gauges + build-tagged soak + benchmarks → session-checkpoint decision (A/B/C) → list endpoints |
| [CI & Versioning](cross-cutting/ci-github-actions-and-versioning.md) | partial | 2026-08-30 | — | P2/P3 hygiene work + P6 (verify tag flow on a real merge); P5 build-release deliberately parked |
| [Cloud Privacy Firewall](cross-cutting/cloud-privacy-firewall.md) | proposed | 2026-10-04 | SPEC-006, SPEC-005, SPEC-001, SPEC-003 | Phase 0 detector corpus → pure engine → agent path (needs D1–D3) → `/v1` path → `ask` mode/UI/audit → router integration → docs |
| [Connector Auto-Reply](cross-cutting/connector-auto-reply.md) | active | 2026-06-28 | SPEC-009 | Phase 3 interactive gateway paths |
| [Frontend Redesign v2](cross-cutting/frontend-redesign-v2.md) | partial | 2026-10-14 | SPEC-003, SPEC-007 | Phases 0–5 delivered 2026-10-14; left: Automations/Reminders/Models/Activity on a phone, arrow-key handling + focus trap in the new sheets, "All workspaces" scope decision, recent-chats menu in the ask panel |
| [Network Audit](cross-cutting/network-audit.md) | partial | 2026-10-26 | SPEC-006, SPEC-007, SPEC-012, SPEC-001, SPEC-003, SPEC-009 | Phases 0–6, report UI, checks and per-audit scan settings merged (#137–#144, v0.12.0). 13.A small finishes (scan again, rescan a host, target warning, "Sets up" line, suggested fixes) merged (#146, v0.13.0, live on vertex). Left (plan §13): owner said yes 2026-10-27 to local vulnerability matching (NVD + KEV; design §13.G; phases 1–5 built 2026-10-28 including the Settings card; the file-import fallback waits for admin auth) and assisted fixes (Level 1 guided fixes accepted and built 2026-10-28, §13.H; Level 2 execution waits for admin auth), no to admin auth; nmap-under-Landlock verified by hand only (not yet done); guarded-programs Constitution text approved and added to II.3 |
| [Post-Implementation Cleanup](cross-cutting/post-implementation-cleanup.md) | active | 2026-08-01 | SPEC-005 | execute findings register (dead code/dup sweep) |
| [Prompt Profiles](cross-cutting/prompt-profiles.md) | active | 2026-10-17 | SPEC-012, SPEC-001, SPEC-007, SPEC-004, CONSTITUTION II.13 | built (Phases 0–6); left: eval the Research, Operator, Builder and Watcher profiles, fold in the layering plan's write guardrail, delete the dead `GetSystemPrompt` chain (`cross-cutting/prompt-profiles.md`) |
| [Sandbox and Egress Residuals](cross-cutting/sandbox-and-egress-residuals.md) | proposed | 2026-09-30 | SPEC-006, SPEC-008, SPEC-009 | Phase 1 (guard fix, proxy dial guard, `freePort` ownership) done 2026-10-08 → Phase 2 shell→loopback decision → Phase 3 optional platform items |
| [Search Tool Calling](cross-cutting/search-tool-calling.md) | active | 2026-09-12 | SPEC-001, SPEC-006 | implemented (Tavily/Brave/SerpAPI providers, Search settings tab, live key, hide-when-unconfigured gate; automated gates green). Open: manual end-to-end verification — Tavily was exercised live (auth fix, note 9); Brave and SerpAPI live calls and the "no key → tool hidden, no restart needed" steps are unconfirmed |
| [SQLite Session Storage](cross-cutting/sqlite-session-storage.md) | proposed | 2026-08-20 | SPEC-001 | JSON → SQLite migration (deliberately deferred, future work) |
| [Tool-Call Grammar Re-enable](cross-cutting/tool-call-grammar-reenable.md) | proposed | 2026-09-05 | SPEC-001, SPEC-002 | envelope-aware GBNF + opt-in per-model toggle, XML path only |
| [GPU Performance](gpu-performance.md) | active | 2026-08-06 | — | P0–P4 rendering/metrics (P5 unblocked — fix-final-report-realignment landed) |

## Follow-ups left by archived plans


- **Run pipeline cleanup and agent commands** ([`ARCHIVE/agent-loop/run-pipeline-and-agent-commands.md`](ARCHIVE/agent-loop/run-pipeline-and-agent-commands.md)): the Telegram live check, postponed.
- **Agent OS Sandboxing** ([`ARCHIVE/cross-cutting/agent-os-sandboxing.md`](ARCHIVE/cross-cutting/agent-os-sandboxing.md), `complete (pending Linux-CI)`): confirm the Landlock probe on Linux CI (the `Sandbox conformance (ubuntu-*)` jobs now run on every PR — check whether they cover it, then drop this line); optional macOS Seatbelt on framework-capable hardware.
- **Docs and Agent Harness Restructure** ([`ARCHIVE/cross-cutting/docs-and-agent-harness-restructure.md`](ARCHIVE/cross-cutting/docs-and-agent-harness-restructure.md) §11): harvest merged phases out of large `partial` plans as they land (`cross-cutting/network-audit.md` is 65 KB); optionally split `docs/architecture/pitfalls.md` (~44 KB) by subsystem; confirm Command Code reads `AGENTS.md` (ADR-0001).
