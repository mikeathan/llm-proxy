---
name: testing-guide
description: "Testing guide: authoring/running Go + Vitest/Playwright tests, smoke tasks, record-replay, MockClient patterns, and run-artifact analysis. Use when writing or running tests; for a failing test/run start with debugging."
last_reviewed: 2026-09-30
---

# Testing — Patterns, Tools & Strategies

**Source docs:** `docs/PLANS/ARCHIVE/cross-cutting/record-replay-test-framework.md`, `backend/data/templates/`, `docs/architecture.md` → Test Patterns

> A failing test or run? Start with [`debugging`](../debugging/SKILL.md). This skill is for authoring/running tests and reading run artifacts.

---

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
| `memory-ab-test.md` | Memory A/B comparison — goal-phrased task that fills an 8K window; run with `memory_mode` off vs hot (see `docs/guides/memory-testing.md` Part B) |
| `sandbox-conformance-probe.md` | Sandbox conformance probe (OS-jail / run capability) |
| `ts-logic-interface-test.md` | TypeScript type system, interfaces, generics |
| `compliance_check_internal.md` | Compliance & high-risk port audit |
| `network_recon_unprivileged.md` | Unprivileged network reconnaissance |
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

## LLM test doubles (two distinct mocks — use the right one)

There are **two** LLM mocks with different fields; mixing them is a compile error.

| Mock | File | Use for | Fields |
|------|------|---------|--------|
| `assistant.MockClient` | `internal/core/assistant/agent_test.go` | agent-loop tests | `Response` (single), `Err`, `Calls`, `ChatFunc`, `StreamFunc`, `ReasoningFieldOverride` |
| `mocks.MockLLMClient` | `internal/testing/mocks/provider.go` | handler / service tests | `Response`, `Responses[]`, `Err`, `LastReq`, `Requests`, `Calls` |

### `assistant.MockClient` (agent tests)

- **One fixed reply** — set `Response`; every `Chat()` returns `&m.Response` (the same pointer each call — copy inside `ChatFunc` if you mutate it).
- **Per-call logic** — set `ChatFunc`; `Calls` is post-incremented (already incremented when `ChatFunc` runs, so the first call sees `Calls == 1`).

```go
client.ChatFunc = func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
    if client.Calls == 1 {
        return toolCallResponse, nil // first turn: call a tool
    }
    return finalResponse, nil        // second turn: final text
}
```

- **Streaming is off by default** — `Stream()` returns `"streaming not implemented in mock"` unless you set `StreamFunc`, so agent tests silently exercise the non-streaming `Chat()` fallback and never the streaming path. Set `StreamFunc` to cover streaming.
- It has **no `Responses` field**.

### `mocks.MockLLMClient` (handler / service tests)

- Set `Responses` to script a sequence: it returns `Responses[Calls-1]`, and once the calls exceed the slice it reuses the last entry. If `Responses` is empty it falls back to `Response`. `Responses` takes priority over `Response`.
- It has **no `ChatFunc`/`StreamFunc`** — `Stream()` always errors.

### Common mistakes

- **Off-by-one on `Calls`** — post-incremented: first call has `Calls == 1`, not 0.
- **Using `Responses` on `assistant.MockClient`** — the field does not exist (that is `mocks.MockLLMClient`).
- **Forgetting `StreamFunc` on `assistant.MockClient`** — the test silently tests the non-streaming fallback, not streaming.
- **Setting both `Response` and `Responses` on `MockLLMClient`** — `Responses` wins when non-empty.

## Record-Replay Testing

### Recording

Every LLM call (Chat or Stream) is written to `data/runs/<model>/<task>/<timestamp>_<session>.jsonl`:

```bash
go run main.go --record
```

Hit different LLMs with different prompts through the proxy or agent API — each model gets its own subdirectory.

### JSONL Fixture Format

One JSON object per line, with these event types:

| Type | When | Fields |
|------|------|--------|
| `request` | Before LLM call | `model`, `messages[]`, `tools[]` |
| `response` | Non-streaming response | `choices[{message}]` |
| `chunk` | Stream delta | `choices[{delta: {content, tool_calls, reasoning}}]` |
| `error` | HTTP/connection error | Error details |
| `done` | Stream completion | `total_chunks` |

```json
{"type":"request","model":"gemma4","messages":[...],"tools":[...]}
{"type":"response","choices":[{"message":{"role":"assistant","content":"answer"}}]}
{"type":"done","total_chunks":1}
```

For streaming sessions, lines alternate `chunk`/`response`/`done` following the initial `request` line.

### Replay

Replay tests are opt-in via the `recordreplay` build tag. The test runner (`llmprofiles.RunAgainstFixtures`) loads all `.jsonl` files, wraps each in a `FixtureClient` that implements `proxy.Client`, and runs the agent against it — identical to a live run:

```bash
go test -tags recordreplay ./internal/core/assistant/ -run TestAgent_Execute_AgainstRecordings -v
```

Fixture `.jsonl` files live in `internal/core/assistant/testdata/recordings/`, **but**
`TestAgent_Execute_AgainstRecordings` reads `../../../testdata/recordings` (→ backend/testdata/recordings,
which does not exist) and `t.Skip`s when it finds no `.jsonl` files there. The replay test therefore
**silently skips** today — do not trust a green result; confirm it actually ran, or point the test at
the fixtures directory.

See `docs/PLANS/ARCHIVE/cross-cutting/record-replay-test-framework.md` for the full design.

## Frontend testing (Vitest + Playwright)

Tests mirror the source tree in `frontend/src/__TESTS__/`. `frontend/vitest.config.ts` defines two projects:
**`unit`** (node, `*.test.ts`) and **`component`** (happy-dom + Vue plugin, `*.component.test.ts`) — a test that
mounts or imports a `.vue` file, or navigates the router (lazy route chunks), must be a `.component.test.ts`.
`npm run build` runs `npm run lint` (ESLint + palette ratchet + contrast gate) → `vue-tsc -b` → `vite build`.
`npm run test:visual` drives installed Chrome against the Vite dev server (`frontend/playwright.config.ts`);
add `-- --update-snapshots` only to accept an intended change.

Gotchas (each cost a debugging round):
- **Vitest stubs CSS imports, even `?raw`** — `tokens.css` is let through per project via `css.include`.
- The node project has no `localStorage` — use `__TESTS__/helpers/memoryStorage.ts` + `vi.stubGlobal`. Other
  helpers: `fieldByLabel.ts` (find a control by label; survives markup changes), `fakeMatchMedia.ts`.
- ES2020 target (`tsconfig.app.json`) + `noUncheckedIndexedAccess` (inherited from `@vue/tsconfig`): no `.at()` / `Object.hasOwn`; indexed access is `T | undefined`.
- happy-dom does not submit a form when its submit button is clicked — `form.trigger('submit')`.
- `onBeforeRouteLeave` registers only inside a `RouterView` — mount a wrapper rendering one, then `vi.waitFor`
  the lazy page. A route navigation settles after its chunk loads (`vi.waitFor`, not `flushPromises`);
  `router.resolve()` does not follow redirects (`router.push` + `currentRoute`).
- Route views read `composables/ui/useDestinationRoute.ts`, not `useRoute()` — with `out-in` transitions and
  keep-alive, a leaving view otherwise sees the *next* route.
- `structuredClone` throws on reactive props — copy plain prop data with JSON.

**Checking the real app (isolated backend):** the binary embeds `frontend_dist` at compile time, so run
`npm run build` and rebuild the binary before looking at UI changes. Build into a scratch dir and start it
**with cwd = that dir**: `cd <scratch> && ./lp-bin --data <scratch>/home`. Never start it with cwd `backend/` —
it then loads `backend/.env.development` (`internal/platform/env/env.go`) and points at the user's real
data. Confirm `GET /admin/api/dispatcher/workspaces` returns your fixtures before any write. To stop it, read
the PID's command line from `lsof -tiTCP:4001 -sTCP:LISTEN` first and kill only what you started. Ad-hoc
Playwright scripts use `channel: 'chrome'`; inject a theme with localStorage `admin-ui:theme-selection` =
`{"version":1,"data":{"kind":"preset","id":"retro-dark"}}` (plus `admin-ui:theme-applied`, see `e2e/design.visual.spec.ts`).

---

## Common Pitfalls

- **Workspace pollution** — Previous runs leave files behind. Clean workspace before tests: `rm -rf workspace-1/{dev-test,smoke-test-dir,node_modules,*.txt,*.json,*.ts,*.js}`
- **Temperature too low** — 0.1 is default. For Gemma 4, raise to 0.3-0.4 if looping. Set via model settings.yml override.
- **llama.cpp args** — Must include `--repeat-penalty 1.12 --repeat-last-n 256 --frequency-penalty 0.5 --presence-penalty 0.5` to prevent token-level repetition.
- **Cache cold starts** — First request after server start is slow (~6-7s prompt eval). Subsequent requests use prompt cache (~0.3-0.6s).
- **Recording files accumulate** — `--record` writes every interaction. Clean old recordings periodically.
