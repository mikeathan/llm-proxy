---
status: reference
last_reviewed: 2026-07-11
---

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
- `recording.jsonl` — full request/response pairs (for replay)
- `events.jsonl` — SSE events (lifecycle, stuck, fallback)
- `final-report.md` — agent's final output

**Checklist for a successful run:**
- `run-meta.json` shows `error null` or missing error field
- Tool calls match task steps (no missed steps, no extras)
- No stuck-detection or spiral-detection events
- Reasoning budget exceeded warnings are warn-only (expected)
- Final report is coherent and covers all required topics

## MockClient Patterns (Agent Tests)

Agent tests in `internal/core/assistant/agent_test.go` use `MockClient` to simulate
LLM responses. There are **three patterns** for controlling what the mock returns:

### Pattern 1: Single fixed response (`client.Response`)

Simplest — the mock returns the same response on every `Chat()` call.
Use for single-turn tests where the model only needs to respond once:

```go
client := &MockClient{
    Response: proxy.ChatResponse{
        Choices: []proxy.Choice{
            {Message: proxy.Message{Role: "assistant", Content: "# Summary\nHello world"}},
        },
    },
}
```

Every call to `client.Chat()` returns the same `Response`.

### Pattern 2: Response sequence (`client.Responses` array)

The mock cycles through responses in order. Use for multi-turn tests where
the model needs to call a tool, then respond:

```go
clientMock.Responses = []proxy.ChatResponse{
    // 1. First LLM response: Call Tool
    {Choices: []proxy.Choice{{
        Message: proxy.Message{
            Role: "assistant",
            ToolCalls: []proxy.ToolCall{{...}},
        },
    }}},
    // 2. Second LLM response: Final text
    {Choices: []proxy.Choice{{
        Message: proxy.Message{
            Role:    "assistant",
            Content: "# Summary\nThe result is done.",
        },
    }}},
}
```

The mock returns `Responses[0]` on first call, `Responses[1]` on second, etc.
If calls exceed the array length, the last response is reused.

### Pattern 3: Custom logic (`client.ChatFunc`)

For complex scenarios that need per-call logic. `client.Calls` is post-incremented
(already incremented when `ChatFunc` runs):

```go
client.ChatFunc = func(ctx context.Context, req proxy.ChatRequest) (*proxy.ChatResponse, error) {
    if client.Calls == 1 {
        return toolCallResponse, nil
    }
    return finalResponse, nil
}
```

### Critical: Streaming is disabled by default

`MockClient.Stream()` returns `error "streaming not implemented in mock"` by default.
This means agent tests **silently fall through to `Chat()`** — they never test the
streaming code path. To test streaming, set `client.StreamFunc`.

### Response handling order

```
client.Chat()
  → client.Calls++
  → if client.ChatFunc != nil → use ChatFunc
  → if len(client.Responses) > 0 → cycle through Responses by index
  → fall back to client.Response (single shared value)
```

### Common mistakes

- **Off-by-one on `client.Calls`** — Post-incremented: first call has `Calls == 1`, not 0.
- **Setting both `Response` AND `Responses`** — `Responses` takes priority if non-empty.
- **Forgetting `StreamFunc`** — Tests silently test the non-streaming fallback, not streaming.
- **Shared `Response` pointer** — `return &m.Response` returns the same pointer every time.
  The response struct is reused, not copied. Make a copy inside `ChatFunc` if needed.

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

Fixture `.jsonl` files go in `internal/core/assistant/testdata/recordings/`.

See `docs/PLANS/ARCHIVE/cross-cutting/record-replay-test-framework.md` for the full design.

## Common Pitfalls

- **Workspace pollution** — Previous runs leave files behind. Clean workspace before tests: `rm -rf workspace-1/{dev-test,smoke-test-dir,node_modules,*.txt,*.json,*.ts,*.js}`
- **Temperature too low** — 0.1 is default. For Gemma 4, raise to 0.3-0.4 if looping. Set via model settings.yml override.
- **llama.cpp args** — Must include `--repeat-penalty 1.12 --repeat-last-n 256 --frequency-penalty 0.5 --presence-penalty 0.5` to prevent token-level repetition.
- **Cache cold starts** — First request after server start is slow (~6-7s prompt eval). Subsequent requests use prompt cache (~0.3-0.6s).
- **Recording files accumulate** — `--record` writes every interaction. Clean old recordings periodically.
