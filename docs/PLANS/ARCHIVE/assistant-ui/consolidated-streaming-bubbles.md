# Consolidated Streaming Bubbles

**Status:** complete  
**Date:** 2026-06-17  
**Related:** SPEC-003 (Discovery Panel UI)

## Problem

The assistant chat UI produced unstable transient bubbles during streaming.
`messageBuilder.ts` created 4-5 separate `AssistantMessage` entries per agent
turn (one each for `tool_stream`, `tool_call`, `tool_result`, `message`,
`finalize`). The `turnGrouper` computed property re-evaluated on every change
causing visible flicker — bubbles appearing, disappearing, content jumping
between slots. At stream end, all intermediate bubbles were cleared, leaving
only the user prompt and final result.

## Desired Behavior

Per-turn rendering:
1. User prompt bubble
2. "Agent Run" card containing ALL model output (reasoning + tool steps),
   scrollable (`max-height: 400px; overflow-y: auto`), auto-collapsed on
   completion
3. Final answer bubble (rendered markdown)

Historical sessions: same collapsed-by-default behavior.

## Solution

### Files Changed

| File | Change |
|------|--------|
| `frontend/src/utils/messageBuilder.ts` | Rewrote to consolidate into ONE assistant message per turn. Removed `activeIdx` pattern. `handleToolStream`/`handleToolCall` update a single message; `handleToolResult` pushes `{role:'tool'}` for format compat; `handleMessage` distinguishes intermediate (has tool_calls) vs final (no tool_calls); `finalize` is a safe no-op if content already set. |
| `frontend/src/utils/turnGrouper.ts` | Added consolidated-format detection (single assistant message after user with reasoning/tool_calls/content). Pairs tool_calls with subsequent `{role:'tool'}` messages by name. Legacy flat-format fallback preserved for loaded sessions. |
| `frontend/src/components/AgentIde/assistant/AssistantChat.vue` | Steps group body: `max-height: 400px; overflow-y: auto`. `AgentThinking` auto-collapse bound to `!loading`: during streaming (`loading=true`) thinking stays expanded; after completion (`loading=false`) auto-collapses. |
| `frontend/src/composables/useAssistant.ts` | Added `builder.reset()` calls before `sse.connect()` and after `finalize`/error paths to clear internal `assistantIdx` for next turn. |

### Key Design Decision

The `handleMessage` event from the backend includes both intermediate turns
(with `tool_calls`) and the final answer (no `tool_calls`). The builder
skips setting `content` on intermediate messages — only the final
no-tool-call message populates `content`. This prevents intermediate text
from appearing as a premature "final answer" bubble.
