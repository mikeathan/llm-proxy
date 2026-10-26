# Test Patterns and Coding Standards

Repo-specific test patterns and coding standards. Language rules live in `.agents/rules/`; test procedure in the `testing-guide` skill.

Part of the [architecture reference](../architecture.md).

## Test Patterns

See `.agents/skills/testing-guide/SKILL.md` for the full test patterns guide (smoke tests, record-replay, run analysis).

- Mock the LLM client, use real tool providers where possible
- Agent tests at `internal/core/assistant/agent_test.go`
- Parser tests at `internal/core/proxy/tool_call_parser_test.go`
- Recorder tests at `internal/core/proxy/recorder/recorder_test.go`
- FixtureClient tests at `internal/testing/llmprofiles/profiles_test.go`
- Record-replay integration tests at `internal/core/assistant/agent_recording_test.go` (build tag: `recordreplay`)
- Use `mocks.NewMockManager()` for manager-related tests
- Test guardrail decisions with `GuardrailDecisionStore`
- Always assert both success AND error paths

### Record-Replay Testing

Record live LLM interactions by starting the server with `--record`:

```bash
go run main.go --record
```

This wraps every LLM client in a `RecordingClient` that writes JSONL transcripts to `data/runs/<model>/<task>/<timestamp>_<session>.jsonl` and enables the replay/fixture store. Each run also gets a per-run folder under `data/runs/` containing `events.jsonl` and `recording.jsonl` whenever run logging is enabled (config `run_logging.enabled`, `--enable-runs`, or `--record`).

Run replay tests offline (no LLM required):

```bash
go test -tags recordreplay ./internal/core/assistant/ -run TestAgent_Execute_AgainstRecordings -v
```

The `recordreplay` build tag ensures these tests are excluded from `go test ./...` — they only run when explicitly invoked. Fixture `.jsonl` files go in `internal/core/assistant/testdata/recordings/` or any path passed to `RunAgainstFixtures`.

## Coding Standards & Architecture

### Principles

Coding principles, Go idioms, error handling, and Vue conventions are owned by the mandatory rule
files and skills — reference them, don't restate them here:

- **Mandatory Go / Vue mechanics:** [`.agents/rules/`](../../.agents/rules/).
- **Language-agnostic principles + smells checklist:** [`clean-code`](../../.agents/skills/clean-code/SKILL.md).
- **Repo patterns, limits, file checklists:** [`engineering-practices`](../../.agents/skills/engineering-practices/SKILL.md).

Repo-specific facts worth keeping here:

- **Dependency direction:** `internal/core/` → `internal/platform/` → `models/`; transport depends on core, never the reverse. `models/` has zero imports from the rest of the codebase.
- **No `init()` functions** in backend code — explicit construction via `New*` / `Initialize*` (the tool-registry connector factories are the documented exception).
- **Production readiness:** graceful shutdown via `signal.NotifyContext` in `main.go`; every long-lived operation takes a `context.Context`; structured `logging.*` with key-value pairs; no secrets in logs.
- **Frontend:** composables are module-level singletons; `npm run build` runs `vue-tsc -b` then `vite build` (TS errors fail the build). The admin UI's contract is SPEC-003 (`docs/SPECS/discovery-panel.md`).
- **Polling through `usePolling`** for new code — pauses when a keep-alive view deactivates, stops on unmount. The older shared pollers own their timers and are ref-counted (`useProcesses`, and `useMetrics`, whose consumers also release on keep-alive deactivate; the header's `HostStats` keeps it running app-wide and it pauses on a hidden tab); `useGlobalRunActivity` runs app-wide and pauses on a hidden tab. Every timer needs a named owner and teardown (Phase 6 leak audit).
- **Admin UI conventions** (tokens only, `utils/format/`, typed route builders, `usePersistedState`, `ConfirmDialog`, `renderMarkdown`) — `.agents/rules/frontend-vue-engineer.md` → Admin UI conventions.
