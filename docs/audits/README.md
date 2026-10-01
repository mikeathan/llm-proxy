# Audits

Audits are **post-hoc analysis documents** that examine system behavior against specifications.
They identify gaps, regressions, and bugs found during testing or production use.

| File | Title | Status |
|------|-------|--------|
| `2026-06-26-stale-turn-bleed.md` | Stale turn bleed on cancel + new message | complete |
| `2026-07-06-assistant-debug-cycle.md` | Full debug cycle: tool calls, history leak, emoji loop, GBNF limitations | complete |
| `gpu-performance-audit.md` | **GPU Performance — consolidated audit** (all knowledge, fixes, lessons, measurements) | reference |
| `known-performance-findings.md` | **Known Performance Findings** — provider TTFT vs local logic, SSE reader fix | reference |
| `agent-stability-report.md` | Agent Stability Audit (13 issues) | complete |
| `backend-audit-report.md` | Backend Audit Report (bugs, leaks, bottlenecks) | reference |
| `ephemeral-turn-context-failed-run.md` | Ephemeral Turn Context — Failed Run Analysis | complete |
| `memory-injection-investigation.md` | Memory Injection + Automation Limitations | reference |
| `remove-memory-rewriter.md` | Remove Memory Rewriter + FTS5 Fix | complete |
| `write-file-truncation-cycles.md` | write_file Truncation Cycles, Block Editing & Early Reasoning Stuck Detection | reference |
| `degenerate-stream-repetition-guard.md` | Degenerate stream repetition loop — content guard & per-stream duration cap | complete |
| `hermes-write-file-guardrail.md` | Hermes Agent does not structurally block report file writes | reference |
| `terminal-vs-filesystem-guardrail-asymmetry.md` | Audit: Terminal vs Filesystem Guardrail Asymmetry — `.sandbox` readable via terminal | complete |
| `2026-08-28-ops-performance-review.md` | Ops & Backend Performance Review — findings + fixes (log rotation, tail reads, host-metrics cache, EventBus byte budget, compact session marshal) | reference |
| `2026-08-30-llm-smoke-test-incomplete-run.md` | llm-smoke-test Incomplete Run — terminal newline collapse, premature finalization on truncated ReAct scaffold, local native-tools auto-detection | complete |
| `2026-09-30-platform-scan.md` | Platform Scan — security (no auth, guard gaps), memory, performance/leaks, assistant/automation UI; evidence ledger behind six proposed plans | reference |
| `codebase-audit-report.md` | Codebase Audit Report — 88 findings (bugs, architecture/duplication, docs) + resolved backend-duplication appendix | active |
