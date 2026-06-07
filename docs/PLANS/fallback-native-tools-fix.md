---
status: complete
related_specs: [SPEC-001]
---
# Fix: Reasoning-Stuck Fallback Actually Retries Without Native Tools

**Status: COMPLETE**

All three bugs fixed in `agent.go`: (1) retry sets `useNativeTools = false` (XML text mode), (2) `streamCancel()` prevents goroutine leak, (3) non-streaming stuck detector added to `computeNextResponseNonStreaming`.

## Problem

When native tools cause the model to enter a reasoning loop (5400+ chars of thinking
without tool calls), the stuck detector correctly aborts the stream (returns nil).
But the fallback has three bugs:

1. **Log lies**: says "retrying without them" but actually retries with the same
   native tools + `tool_choice: "required"`
2. **Goroutine leak**: the `processStream` progress ticker goroutine leaks because
   `ctx` is never cancelled on early return
3. **No stuck detector on non-streaming**: `computeNextResponseNonStreaming` calls
   `client.Chat` which blocks synchronously until max_tokens (up to 10 min)

## Root Cause

### Bug 1 — `agent.go:850`: fake retry

The fallback at line 849:
```go
if fullMsg.Content == "" && len(fullMsg.ToolCalls) == 0 && llmTools != nil {
    a.logger.Info("empty response with native tools, retrying without them")
    return a.computeNextResponseNonStreaming(ctx, history, tools)
}
```

Passes `tools` (= native schemas) and `a.useNativeTools` is still true. The
non-streaming call sends `tool_choice: "required"` again — identical payload.

### Bug 2 — `agent.go:~880`: goroutine leaks on early return

```go
go func() {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    for {
        select {
        case <-ticker.C:
            a.logger.Info("stream still generating", ...)
        case <-ctx.Done():
            return
        }
    }
}()
```

When stuck detector hits `return nil`, the goroutine's `ctx.Done()` never fires
because the turn context isn't cancelled. Leaks until global timeout (30 min).

### Bug 3 — `agent.go:computeNextResponseNonStreaming`: no stuck check

The non-streaming path has no reasoning-stuck detector. Model generates pure
reasoning → response has no tool calls → `handleNoToolCalls` injects nags →
each nag adds another 2-3 min synchronous wait.

## Fix 1: Actually retry without native tools

**`agent.go:~849`** — Fallback 1:

Change:
```go
return a.computeNextResponseNonStreaming(ctx, history, tools)
```

To:
```go
a.useNativeTools = false
return a.computeNextResponseNonStreaming(ctx, history, nil)
```

This strips native tools so the retry uses XML text mode — fundamentally
different format breaks the reasoning loop.

## Fix 2: Cancel turn context on stuck detection

**`agent.go:~936`** — In the stuck detector block:

Add:
```go
cancel()  // stops the progress goroutine
return nil
```

Requires making the cancel function available. Simplest: store on Agent struct
before calling `processStream` (`a.turnCancel = cancel`), or pass it in.

## Fix 3 (Optional): Timeout on non-streaming retry

**`agent.go:computeNextResponseNonStreaming`** — Wrap `client.Chat` with a
60-second timeout when in fallback mode to prevent 10-minute blocking.

---

## Files Changed

| File | Lines | Change |
|---|---|---|
| `agent.go` | ~850 | Fallback retry without native tools |
| `agent.go` | ~880–936 | Cancel turn context on stuck detection |
| `agent.go` | ~1080 | Optional: 60s timeout on non-streaming call |

## Test Changes

- Update `TestAgent_Execute_ReasoningStuckFallback` to assert `useNativeTools`
  becomes false after fallback
- Add `TestProcessStream_GoroutineCleanup` — verify ticker exits on stuck detection
