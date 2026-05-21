# Tool Choice & Temperature — Implementation Plan

## Problem

When native tools are active in automation mode, models that support thinking/reasoning
(e.g. Qwen 3.5 via llama.cpp) can produce reasoning-only responses that end with an
EOS token — no tool call, no content, causing "empty response with native tools"
fallbacks and eventual premature termination.

## Root Cause

Three gaps in the agent's LLM integration:

1. **`tool_choice`** defaults to `"auto"`, which lets the model choose zero or more
   tools. The model picks zero → EOS → empty response.
2. **`temperature`** defaults to 1.0 (creative), encouraging exploration.
   `temperature: 0.1` makes the model nearly deterministic while allowing enough
   variation to break out of "mirror loops" when a tool call fails.
3. **No client-side reasoning stuck detection.** The `reasoning_budget` parameter is
   sent to the server but some server versions don't enforce it as a hard cap,
   letting the model generate endless reasoning without producing a tool call.

## Phase 1 — Force Tool Call + Deterministic Temperature + Reasoning Budget + Stuck Detection

**Goal:** Guarantee the model always produces a tool call on every automation turn,
prevent the model from exhausting its token budget on thinking alone, and detect
when the model is stuck in an infinite thinking loop.

### Changes

#### 1. `models/llm_messages.go` — Add `Temperature` and `ReasoningBudget` fields

```go
type ChatRequest struct {
    Model           string     `json:"model"`
    Messages        []Message  `json:"messages"`
    MaxTokens       int        `json:"max_tokens,omitempty"`
    Temperature     float64    `json:"temperature,omitempty"`       // 0.1 = near-deterministic
    ReasoningBudget int        `json:"reasoning_budget,omitempty"`  // max thinking tokens
    Tools           []Tool     `json:"tools,omitempty"`
    ToolChoice      ToolChoice `json:"tool_choice,omitempty"`
    Stream          bool       `json:"stream,omitempty"`
}
```

#### 2. `internal/core/assistant/agent.go` — Both LLM call paths

Set `tool_choice: "required"`, `temperature: 0.1`, and `reasoning_budget` when
native tools + automation:

```go
if a.useNativeTools && isAutomationCtx {
    req.ToolChoice = proxy.ToolChoiceRequired
    req.Temperature = 0.1
    if a.reasoningBudget > 0 {
        req.ReasoningBudget = a.reasoningBudget
    } else {
        req.ReasoningBudget = a.maxTokens / 4  // default: quarter for thinking
    }
}
```

#### 3. `internal/core/assistant/agent.go` — `processStream` stuck detection

When the model generates more than 2000 chars of reasoning content without any
text output or tool call deltas, abort the stream early. This catches the case
where the server's `reasoning_budget` is not enforced as a hard cap:

```go
if len(fullMsg.ReasoningContent) > 2000 && len(fullMsg.Content) == 0 && len(fullMsg.ToolCalls) == 0 {
    a.logger.Warn("reasoning stuck detected, aborting stream")
    return nil
}
```

The aborted stream triggers the empty-response fallback in `computeNextResponse`,
which retries non-streaming with the full tools list — a fundamentally different
code path that often breaks the reasoning loop.

#### 4. `internal/core/assistant/agent.go` — `isPrematureTermination` fix

When the model produces reasoning content but no text and no tool calls (e.g.
from a non-streaming response), treat it as "needs a nag" rather than "premature
termination":

```go
if content == "" {
    if len(msg.ReasoningContent) > 0 {
        return false  // model was thinking but needs a nag
    }
    return len(msg.ToolCalls) == 0
}
```

#### 5. Progressive sieve recovery on consecutive stuck events

In the `Execute` loop, track consecutive context-size errors. On the 1st stuck:
apply the standard reactive sieve + a nag prompt telling the model to "Stop
analyzing, call a tool." On the 2nd stuck: apply an **aggressive sieve** that
keeps only first 2 + last 3 messages (vs. last 6 for the standard sieve) +
a stronger nag. On the 3rd stuck: return a fatal error instead of spinning.

See `agent.go` `Execute()` for the loop, `applyAggressiveSieve()` for the
aggressive sieve, and `prompts.ReasoningStuckNag` / `prompts.ReasoningStuckEscalatedNag`
for the nag messages.

### Tests

- `TestAgent_Execute_NativeToolsSetsToolChoice`: verifies `req.ToolChoice`
  is `"required"` when native tools + automation.
- `TestAgent_Execute_NativeToolsTemperatureSet`: verifies `req.Temperature`
  is `0.1` and `req.ReasoningBudget` is non-zero when native tools + automation.
- `TestAgent_Execute_XMLToolChoiceUnset`: verifies `ToolChoice` and
  `Temperature` are NOT set when native tools are disabled (XML mode).
- `TestAgent_Execute_ReasoningStuckFallback`: verifies that repeated stuck
  streams trigger progressive sieve and fail with a reasoning-loop error.
- `TestAgent_IsPrematureTermination/reasoning_only_(not_premature)`: verifies
  that `isPrematureTermination` returns false when reasoning content exists.
- `TestAgent_ContextSizeErrorRecovery`: verifies a single context-size error
  is recovered from via the reactive sieve.

## Phase 2 — Finish Reason & Usage Tracking

**Goal:** Detect truncated responses and track token usage accurately.

### Changes

#### 1. `models/llm_messages.go` — Add to `Choice` and `ChatResponse`

```go
type Choice struct {
    Message      Message `json:"message"`
    Delta        Message `json:"delta,omitempty"`
    FinishReason string  `json:"finish_reason,omitempty"`
}

type ChatResponse struct {
    Choices []Choice       `json:"choices"`
    Usage   *UsageInfo     `json:"usage,omitempty"`
    ID      string         `json:"id,omitempty"`
}

type UsageInfo struct {
    PromptTokens     int `json:"prompt_tokens"`
    CompletionTokens int `json:"completion_tokens"`
    TotalTokens      int `json:"total_tokens"`
}
```

#### 2. Detect `finish_reason: "length"` in stream

In `processStream` (or after it returns), check if `finish_reason` is
`"length"` (truncated) vs `"stop"` (natural) vs `"tool_calls"` (native call).

#### 3. Read `usage` from response

Pass token counts to the orchestrator for accurate budget tracking
(instead of client-side char estimation).

## Files Changed

| File | Phase |
|---|---|
| `models/llm_messages.go` | 1, 2 |
| `internal/core/assistant/agent.go` | 1 |
| `internal/core/assistant/agent_test.go` | 1, 2 |
| `docs/plans/tool-choice-temperature-implementation.md` | Current |
| `AGENTS.md` | 1 |
| `CLAUDE.md` | 1 |
