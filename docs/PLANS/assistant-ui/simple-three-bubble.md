# Simple Three-Bubble UI

**Status:** complete  
**Date:** 2026-06-17

## Problem

The assistant UI produced unstable transient bubbles during streaming.
`handleToolStream` **replaced** `reasoning_content` on each SSE event instead
of appending to `content`, causing the text to clear and re-fill. The
turnGrouper and template added unnecessary complexity (thinking panel, tool
steps, collapse/expand) that wasn't needed for the base three-bubble layout.

## Goal

Three and only three bubbles per turn:

1. User prompt
2. All agent output (accumulated streaming text)
3. Final result (the agent's final reply)

Tool calls, steps, and thinking panel are deferred for later.

## Changes

### `messageBuilder.ts`

**`handleToolStream(text)`** — append to the single assistant message's
`content` field instead of setting `reasoning_content`. This accumulates
all streaming text into one bubble.

**`handleToolCall` / `handleToolResult` / `handleMessage`** — removed. These
event handlers are no-ops; all content comes from `tool_stream` events and
`finalize`.

**`finalize(reply)`** — always pushes a new `{role:'assistant', content:
reply}` message as the result bubble. Resets `assistantIdx` so next turn
starts fresh.

### `turnGrouper.ts`

Simplified `Turn` to two fields: `agentOutput` and `finalAnswer`. After a
user message, collects all following assistant messages without tool_calls.
The **last** assistant message's content → `finalAnswer`. All preceding ones
→ `agentOutput` (joined). This avoids duplicate bubbles when the streamed
text and final answer are the same.

### `AssistantChat.vue`

Removed `AgentThinking`, `AgentStep`, steps-group card, and their CSS.
Template renders three items per turn:

1. `UserMessage`
2. Agent output bubble (v-if agentOutput)
3. Result bubble (v-if finalAnswer && finalAnswer !== agentOutput)
