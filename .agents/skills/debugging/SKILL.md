---
name: debugging
description: "Root-cause a failing test, build, agent run, or tool call: reproduce, capture the exact error, locate the code path, isolate one hypothesis, fix the smallest cause, add a regression test, run the gate. Use when something fails or misbehaves."
when_to_use: "A test/build/run/tool call fails, an agent run loops or stalls, or behavior is wrong and the cause is unknown."
status: reference
last_reviewed: 2026-09-26
---

# Debugging — Root-Cause Workflow

**Trigger:** something fails or misbehaves — a test, build, `go vet`, an agent run (loop/stall/empty), a tool call, or the UI.
**Skip:** a one-line fix with an obvious error (just fix it + `tdd-guide`), and pure feature work (`task-planning`).

**Rule: read the error before changing code.** Never edit from a guess about the cause.
Work the loop top to bottom; if a step disproves your hypothesis, go back one step.

## 1. Reproduce and freeze the baseline

Get a deterministic failure before theorising.

```bash
# Backend — one failing test, verbose, no cache
cd backend && go test ./internal/<pkg>/ -run TestFoo -count=1 -v
# Frontend — one file
cd frontend && npx vitest run src/__TESTS__/path/to/file.test.ts
# Build/vet
cd backend && go build ./... && go vet ./...
```

If it does not reproduce, say so before "fixing" anything — a non-reproduced fix is a guess. For flaky/race symptoms run `go test ./<pkg>/ -race -count=5` (race bugs are real bugs; never dismiss them).

## 2. Capture the exact failure (not the model's next action)

Read the actual error text, stack, exit code, HTTP status, or SSE event — not the agent's retry of it. Extract the first failing layer and the input that triggered it.

## 3. Locate the code path

Use `docs/architecture.md` (directory map + Common Pitfalls) to name the package, then grep for the seam:

| Symptom | Start here |
|---|---|
| HTTP route / handler | `internal/app/routes.go` → `internal/transport/http/handlers/` |
| Agent loop / completion / sieve | `internal/core/assistant/` (`session.go`, `stream.go`, `agent.go`) |
| Tool executes wrong / not at all | `models/tools.go` const → `internal/core/tools/manifests/<tool>.json` → `core/tools/<category>.go` → `core/assistant/registry.go` |
| Prompt text wrong | `internal/core/assistant/prompts/templates.go` (the ONLY home — Constitution II.13) |
| Model field ignored | `models/{config,infrastructure}.go` → `transport/http/handlers/{registry_handlers,admin_handlers,admin_view}.go` → `core/llm/manager.go` → `internal/testing/mocks/manager.go` |
| Event not reaching the UI | `assistant/agent_events.go` producer → `core/eventbus/` → `/live` SSE → `frontend/.../useAssistantSSE.ts` → `utils/message/messageBuilder.ts` |
| Frontend behavior | `composables/` → `services/` → `types/` (see the affected UI skill) |

Grep by symbol, not by file: `rg -n "func .*Foo|FooBar"`, `rg -n "EventX|notify.*X"`, `rg -n "route|HandleFunc|\.Get\(|\.Post\("`.

## 4. Form ONE hypothesis, then prove or disprove it

Write the hypothesis as a sentence ("X returns Y because Z is not set"). Instrument the cheapest way that confirms/refutes it:

- Add a temporary `logging.Info("dbg", "v", v)` / `console.log` and re-run the single test.
- Copy the failure into a focused `t.Run` case and iterate there.
- For agent runs, read the recorded evidence (below) instead of re-running blind.

**Agent-run / automation evidence** lives under the run dir (`{root}/runs/<ws>/<task>/<model>/<ts>_<uuid>/`):
`run-meta.json` (duration, LLM/tool counts, error), `events.jsonl` (lifecycle: `stuck_detected`, `fallback_*`, `session_*`), `recording.jsonl` (full request/response pairs), `final-report.md`.
Replay offline without a live model: `go test -tags recordreplay ./internal/core/assistant/ -run TestAgent_Execute_AgainstRecordings -v`.
Server/model logs: `GET /admin/api/logs`, `{root}/logs/`, `journalctl -u llm-proxy` (Linux). Record new evidence with `go run main.go --record`.

Delete temporary instrumentation before finishing.

## 5. Fix the smallest cause

Fix the root cause at the layer that owns it — not a symptom, not a caller, not the test. Do not widen scope while debugging; if you find unrelated breakage, note it and keep going.

## 6. Add a regression test

Turn the reproduction into a permanent test — a new `t.Run` case in the existing table, mirroring the source file (`foo.go → foo_test.go`). The fix is not done until the failing case passes and stays. See `tdd-guide` and `testing-guide`.

## 7. Verify and gate

```bash
cd backend && go test ./... && go build ./... && go vet ./... && go run ./tools/check-complexity/
cd frontend && npm test && npm run build          # frontend-touching changes
```

Then run the Pre-Completion Review (`AGENTS.md`) and review your own diff (`git diff HEAD`).

## Tool-call failures (error → handler → prompt → retry)

When a tool call keeps failing and the model loops, the gap is usually in the recovery path, not the model:

1. Read the error at the point of failure.
2. Find **every** layer that handles it: the tool handler, `tool_exec.go`, and the session loop that routes parse errors to prompts.
3. **Check handler scope** — the most common bug: a handler branches on `toolName == "write_file"` while the failing tool is `edit_file_block`, so the model gets a generic (or misleading) prompt.
4. Check the **recovery prompt** matches the real problem (a "use write_file" hint is useless for a JSON-parse failure of the same tool).
5. If the right answer is a different tool, the handler should guide the model to switch, not retry the same call.

## Common failure modes

- **Fixing the test to match the bug** — change production code; a weakened test hides the regression.
- **Symptom patch** — clamping the value that overflowed instead of the arithmetic that produced it.
- **Stale baseline** — a "passing" suite on a tree that does not build; run build before test.
- **Silent catch** — `_ = err`/empty `catch` swallowed the real cause; remove it to see the error (Constitution IV.2/IV.4).
- **Wrong layer** — "fixing" a transport handler for a domain bug; confirm the owner from `architecture.md`.
- **Environment masquerade** — a missing model/server/timeout looks like a logic bug; check the classified transport error (Pitfall #27/#33) before touching logic.

## Completion criteria

- The original reproduction no longer fails, deterministically.
- A regression test covers it and passes.
- The relevant gate is green (commands above) and reported exactly.
- The diff is scoped, temporaries removed, and reviewed with `git diff HEAD`.

## Related

- `testing-guide` — run analysis, `MockClient` patterns, record-replay.
- `agent-loop` — sieve, stuck/spiral detection, fallback chain, key constants.
- `tdd-guide` — Red/Green/Refactor for the fix.
- `clean-code` §6/§7 — error handling and test smells.
- `docs/architecture.md` Common Pitfalls · `docs/audits/` — post-mortems for known incidents.
