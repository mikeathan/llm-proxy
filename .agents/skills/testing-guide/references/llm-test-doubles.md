# Testing — Patterns, Tools & Strategies — LLM Test Doubles

Reference for the [`testing-guide`](../SKILL.md) skill, loaded on demand. You need a fake LLM in a Go test — pick the right one of the two mocks.

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
