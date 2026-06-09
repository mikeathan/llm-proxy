# Testing — Patterns, Tools & Strategies

**Source docs:** `docs/PLANS/cross-cutting/record-replay-test-framework.md`, `backend/data/templates/`, `AGENTS.md` Test Patterns

---

## Test Types

| Type | When to use | Command |
|------|-------------|---------|
| **Go unit test** | Parser, store, tool logic | `go test ./...` |
| **Agent integration** | Agent loop behaviour | `go test ./internal/core/assistant/` |
| **Record-replay** | LLM interaction without live model | `go test -tags recordreplay ./internal/core/assistant/ -run TestAgent_Execute_AgainstRecordings -v` |

## Automation Task Templates

Template files live in `backend/data/templates/` and are copied to the workspace when an automation runs. After changing a template, the workspace copy must also be updated.

| Template | Purpose |
|----------|---------|
| `smoke-test.md` | 10-step smoke test: filesystem, terminal, npm, TypeScript, network, final report |
| `memory-cascade-test.md` | Save persona facts, search once, answer questions, write biography |
| `memory-tags-test.md` | Save facts with tags, verify tag-filtered search returns correct subsets |
| `sandbox-fs-hierarchy-test.md` | Create directory structure, write/compile/run TypeScript, verify hierarchy |
| `ts-logic-interface-test.md` | TypeScript type system, interfaces, generics |
| `ts-runtime-sanity-check.md` | TypeScript runtime behaviour, Node.js interop |
| `compliance_check_internal.md` | Security compliance audit |
| `network_recon_unprivileged.md` | Unprivileged network reconnaissance |
| `workspace_health_audit.md` | Workspace health check |
| `web_discovery_fast.md` | Fast web discovery |

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
- `run.log` — chronological agent events (turn timing, tool calls, reasoning length)
- `recording.jsonl` — full request/response pairs (for replay)
- `events.jsonl` — SSE events (lifecycle, stuck, fallback)
- `final-report.md` — agent's final output

**Checklist for a successful run:**
- `run-meta.json` shows `error null` or missing error field
- Tool calls match task steps (no missed steps, no extras)
- No stuck-detection or spiral-detection events
- Reasoning budget exceeded warnings are warn-only (expected)
- Final report is coherent and covers all required topics

## Record-Replay Testing

Record live interactions:
```bash
go run main.go --data ./data --record-dir=testdata/recordings
```

Replay offline (no LLM model needed):
```bash
go test -tags recordreplay ./internal/core/assistant/ -run TestAgent_Execute_AgainstRecordings -v
```

Fixture `.jsonl` files go in `internal/core/assistant/testdata/recordings/`.

## Common Pitfalls

- **Workspace pollution** — Previous runs leave files behind. Clean workspace before tests: `rm -rf workspace-1/{dev-test,smoke-test-dir,node_modules,*.txt,*.json,*.ts,*.js}`
- **Temperature too low** — 0.1 is default. For Gemma 4, raise to 0.3-0.4 if looping. Set via model settings.yml override.
- **llama.cpp args** — Must include `--repeat-penalty 1.12 --repeat-last-n 256 --frequency-penalty 0.5 --presence-penalty 0.5` to prevent token-level repetition.
- **Cache cold starts** — First request after server start is slow (~6-7s prompt eval). Subsequent requests use prompt cache (~0.3-0.6s).
- **Recording files accumulate** — `--record-dir` writes every interaction. Clean old recordings periodically.
