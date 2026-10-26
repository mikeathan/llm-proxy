---
name: testing-guide
description: "Testing guide: authoring/running Go + Vitest/Playwright tests, smoke tasks, record-replay, MockClient patterns, and run-artifact analysis. Use when writing or running tests; for a failing test/run start with debugging."
metadata:
  last_reviewed: "2026-10-26"
---

# Testing — Patterns, Tools & Strategies

**Source docs:** `docs/PLANS/ARCHIVE/cross-cutting/record-replay-test-framework.md`, `backend/data/templates/`, `docs/architecture/conventions.md` → Test Patterns

> A failing test or run? Start with [`debugging`](../debugging/SKILL.md). This skill is for authoring/running tests and reading run artifacts.

---

## Reference files

Open only the one your task needs:

| File | Open it when |
|------|--------------|
| [`references/llm-test-doubles.md`](references/llm-test-doubles.md) | You need a fake LLM in a Go test — pick the right one of the two mocks. |
| [`references/record-replay.md`](references/record-replay.md) | You are recording a real run or replaying one in a test. |
| [`references/frontend-testing.md`](references/frontend-testing.md) | You are writing or running frontend unit or visual tests. |

## Test Types

| Type | When to use | Command |
|------|-------------|---------|
| **Go unit test** | Parser, store, tool logic | `go test ./...` |
| **Agent integration** | Agent loop behaviour | `go test ./internal/core/assistant/` |
| **Frontend unit / component** | Composables, utils, domain, theme (`*.test.ts`); anything mounting a `.vue` or routing (`*.component.test.ts`) | `cd frontend && npm test` |
| **Frontend visual** | `/design` reference screenshots at 1440 / 390 px | `cd frontend && npm run test:visual` |
| **Record-replay** | LLM interaction without live model | `go test -tags recordreplay ./internal/core/assistant/ -run TestAgent_Execute_AgainstRecordings -v` |

## Automation Task Templates

Template files live in `backend/data/templates/` and are copied to the workspace when an automation runs. After changing a template, the workspace copy must also be updated.

| Template | Purpose |
|----------|---------|
| `smoke-test.md` | LLM smoke test — multi-tool coverage (filesystem, terminal, npm, TypeScript, network, final report) |
| `memory-cascade-test.md` | Memory cascade — persona recall & cross-reference |
| `memory-three-tier-test.md` | Memory three-tier test (scope/mode/keep routing) |
| `memory-store-test.md` | Memory verification step 1: the agent saves one made-up fact with explicit routing (chat or automation) |
| `memory-recall-test.md` | Memory verification step 2: three questions answerable only from the injected memory block, tools forbidden, one unsaved control (see `docs/guides/memory-testing.md` Part A) |
| `memory-ab-test.md` | Memory A/B comparison — goal-phrased task that fills an 8K window; run with `memory_mode` off vs on (see `docs/guides/memory-testing.md` Part B) |
| `sandbox-conformance-probe.md` | Sandbox conformance probe (OS-jail / run capability) |
| `ts-logic-interface-test.md` | TypeScript type system, interfaces, generics |
| `network_audit.md` | Network audit: scan, compare with the last scan, assess (declares the Security auditor, local-network access, journal off, a 02:00 schedule) |
| `workspace_health_audit.md` | Workspace storage & health audit |
| `web_discovery_fast.md` | Fast AI/LLM news discovery — token-efficient search digest |
| `llm_ai_release_brief.md` | LLM & AI release news brief |

## Running a Smoke Test

```bash
# Start the proxy with recording enabled
go run main.go --record

# The automation dispatcher runs the smoke test on schedule or via the UI.
# Results go to: backend/data/runs/workspace-1/smoke-test/<model>/<timestamp>/
```

## Analysing a Run

Each run produces:
- `run-meta.json` — duration, LLM calls, tool calls, result
- `recording.jsonl` — full request/response pairs (for replay)
- `events.jsonl` — SSE events (lifecycle, stuck, fallback)
- `final-report.md` — agent's final output

**Checklist for a successful run:**
- `run-meta.json` shows `error null` or missing error field
- Tool calls match task steps (no missed steps, no extras)
- No stuck-detection or spiral-detection events
- Reasoning budget exceeded warnings are warn-only (expected)
- Final report is coherent and covers all required topics

## Common Pitfalls

- **Workspace pollution** — Previous runs leave files behind. Clean workspace before tests: `rm -rf workspace-1/{dev-test,smoke-test-dir,node_modules,*.txt,*.json,*.ts,*.js}`
- **Temperature too low** — 0.1 is default. For Gemma 4, raise to 0.3-0.4 if looping. Set via model settings.yml override.
- **llama.cpp args** — Must include `--repeat-penalty 1.12 --repeat-last-n 256 --frequency-penalty 0.5 --presence-penalty 0.5` to prevent token-level repetition.
- **Cache cold starts** — First request after server start is slow (~6-7s prompt eval). Subsequent requests use prompt cache (~0.3-0.6s).
- **Recording files accumulate** — `--record` writes every interaction. Clean old recordings periodically.
