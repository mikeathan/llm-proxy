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
| `codebase-audit-report.md` | Codebase Audit Report — 88 findings (bugs, architecture/duplication, docs) + resolved backend-duplication appendix | active |

Related cross-listed audit: [`codebase-audit-report.md`](codebase-audit-report.md)
— comprehensive 88-finding audit (bugs, architecture/duplication, docs) with a
resolved backend-duplication appendix.
