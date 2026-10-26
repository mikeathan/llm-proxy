# Assistant UI Chat Architecture — Segments, Turn Grouping and Completion

Reference for the [`assistant-ui-chat`](../SKILL.md) skill, loaded on demand. You are changing how a turn is split into segments, grouped, or finished.

## Segment System

The `AssistantMessage` type has an optional `segments` array:

```typescript
type Segment =
  | { kind: 'reasoning', text: string }
  | { kind: 'tool_call', name: string, args: string, status: 'running' | 'success' | 'error', result?: string, error?: string }
  | { kind: 'final', text: string }
```

- `reasoning` segments are pushed by `commitReasoning()` when a turn boundary is detected or a tool call occurs
- `tool_call` segments are pushed by `handleToolCall()` with status `'running'`, updated by `handleToolResult()` to `'success'`
- Status transition from `'running'` to `'success'` is synchronous (no setTimeout, no rAF). The real time gap between `tool_call` and `tool_result` events (the tool execution time) provides natural visibility for the spinner.

## Natural Completion Flow

The agent completes a task by producing a content-only assistant message (no further tool calls). The frontend handles this as a normal `message` event: `commitReasoning()` moves streamed text to a reasoning segment, and the final answer text becomes `turn.finalAnswer`. There is no special frontend code for completion — the `message` event is treated the same as any other turn boundary.

**Tool-call-markup guard (2026-08-28):** a content-only `message` whose content contains tool-call markup (`<tool_call`, `[TOOL_CALLS]`) is **NOT** treated as the final answer. Malformed/failed `<tool_call>` attempts are streamed as visible text inside the message payload (the backend cuts them from the live stream via `FilterStreamingMarkup`, but the payload carries the full content); finalizing on them rendered raw JSON as the result and suppressed the real answer (observed with Qwen3.6-35B-A3B). The guard lives in `messageBuilder.ts`'s `message` case.

The `message` event is sent after `processToolCalls` returns and must NOT push a tool-call segment — the tool calls are already represented by their own `tool_call`/`tool_result` events.

## Thinking-Gap Dots Indicator

The thinking-gap shows animated dots during inactivity pauses (no events for 200ms+). It is the ONLY thinking indicator.

- **During streaming**: events every 10-50ms → timer keeps resetting → dots hidden → correct (text is visible)
- **Between stream chunks during a pause**: events stop → 200ms → dots appear → correct
- **Between tool call and result**: no tool_stream events → 200ms → dots appear → correct
- **During final answer**: last tool_stream → 200ms → dots appear and stay through tool_call/tool_result/message → correct
- **Initial startup**: `builder.resetPauseTimer()` starts timer → 200ms → dots appear → correct

The dots NEVER appear while text is streaming. The user needs to see the text, not be distracted by animation.

## Turn Grouper (`turnGrouper.ts`)

The `groupTurns()` function converts the flat `messages[]` array into structured `Turn[]` objects for rendering. Each turn is anchored by a user-role message followed by zero or more assistant-role messages.

### Agent control messages (contract test)

The backend injects control messages as `role: "user"` (nags, format/retry feedback, the context sieve's `[System Note: …]` and `[Progress ledger …]`, length continuation) plus an empty `[stuck]` assistant placeholder. `isInternalMessage()` hides them and keeps them from starting a turn — when it missed the sieve note and ledger (2026-10-01) they showed as the operator's own messages and split one run into fake turns with a lone "Thought:" as the answer. `CONTROL_PREFIXES` mirrors `IsAgentControlMessage` (`backend/internal/core/assistant/session.go`); `__TESTS__/utils/message/controlMessages.contract.test.ts` reads that Go function and `prompts/templates.go` and fails for any control prompt the UI does not hide. **Adding a control prompt in Go → add its prefix here.**

### Single-message turn edge case

When `assistantMsgs.length === 1` (e.g., for webhook-triggered runs or direct text responses), `turn.finalAnswer` must be set explicitly from the single message's content. The `> 1` branch sets `finalAnswer = last.content` automatically, but the `=== 1` branch historically left it empty — causing the "Result" section to be hidden.

**Rule:** When exactly one assistant message exists, set `turn.finalAnswer = only.content` in the same block where `agentOutput` and `segments` are set. The `finalAnswer` content is still filtered by `buildSegmentsFromHistory()` (tool-call content is moved to reasoning segments, so only actual report text ends up in `finalAnswer`).

### `buildSegmentsFromHistory()` content-routing logic

When reconstructing segments from persisted history, assistant messages with tool calls have their `content` moved to a reasoning segment. This prevents intermediate planning text from appearing as raw output:

1. Messages with `reasoning_content` → reasoning segment
2. Messages with `tool_calls + content` → content moved to reasoning segment, message content cleared
3. Content-only messages (natural completion) → content preserved (it IS the report)
