---
status: reference
last_reviewed: 2026-07-11
---

# Audit: Assistant Debug Cycle — Tool Calling, History Leak & Emoji Loop

**Date**: 2026-07-06
**Duration**: ~7h across 50+ iterations
**Subsystems**: `session.go`, `stream.go`, `prompts/`, `messageBuilder.ts`, `turnGrouper.ts`, `useAssistant.ts`, `useAssistantSSE.ts`, `templates.go`, `system_prompt.go`, `rundir.go`

## Bugs Found & Fixed

### 1. Duplicate Tool Call Accumulation
**File**: `backend/internal/core/assistant/stream.go`
**Root cause**: Refactoring left the original tool-call-append block in place after adding a second identical block above it. Every stream chunk with tool calls got processed twice — same `tc.ID` appended twice, arguments concatenated twice.
**Symptom**: Model outputs tool calls with doubled arguments and same ID. llama.cpp returned 500: "Failed to parse tool call arguments as JSON."
**Fix**: Removed the duplicate block.

### 2. SSE URL Wrong After Refactoring
**File**: `frontend/src/composables/assistant/useAssistantSSE.ts`
**Root cause**: Refactoring changed SSE URL from `/admin/api/dispatcher/workspaces/{id}/live` to `/admin/api/conversation/sessions/{id}/live`. Backend only registers the `/dispatcher/workspaces/` route. Frontend silently connected to a non-existent endpoint — EventSource never raises 404.
**Symptom**: No SSE events received. "Thinking..." for full duration, then final result appeared via HTTP response.
**Fix**: Restored the correct URL.

### 3. Missing `builder.reset()` in `loadSession`
**File**: `frontend/src/composables/assistant/useAssistant.ts`
**Root cause**: When switching sessions by clicking history, `builder` was not reset. Stale `assistantIdx`, `reasoningBuffer`, `isFinalTurn` from previous conversation corrupted the new session's processing.
**Symptom**: Clicking a running conversation from history cleared the assistant bubble, flickered, or showed wrong content.
**Fix**: Added `builder.reset()` as first line of `loadSession`.

### 4. Duplicate History Append for Final Answer
**File**: `backend/internal/core/assistant/session.go`
**Root cause**: When `submit_final_answer` was detected, the `turnMsg` (with model response + tool calls) was already appended to `s.history`. Then a SECOND message was appended with just the final content. The model saw the previous full answer as a content-only assistant message in the next turn — which it copied verbatim into new tool call arguments.
**Symptom**: Second prompt in same conversation shows the first prompt's full output (e.g., "tell me a joke" shows file listing report). OR condition then uses the stale summary.
**Fix**: Removed the 4-line duplicate append at `session.go:234-237`. Only the `turnMsg` (with content + tool calls) is stored in LLM history.

### 5. Sieve Warning "finalize when ready" Confused Model
**File**: `backend/internal/core/assistant/prompts/templates.go`
**Root cause**: `ContextSieveWarning` said "finalize when ready" — model interpreted "finalize" as "produce a final text answer directly" instead of "call submit_final_answer tool."
**Symptom**: Under context pressure, model generated text directly without calling `submit_final_answer`.
**Fix**: Changed wording to "call `submit_final_answer` when done."

### 6. `checkSubmitFinalAnswer` Condition (Reverted 3×)
**File**: `backend/internal/core/assistant/session.go`
**History**:
- **origin/main**: `OR` — `if Content == "" || (summary != "Task complete." && summary != "")`
- **Refactoring changed to**: `AND` — `if Content == "" && summary != ""`
- **Our revert**: Back to `OR`

**Why**: The system prompt tells the model "put the answer in summary, keep Content brief." The `OR` condition is designed for this — extract the answer from summary, overwrite Content. The `AND` change broke the normal case where Content is a transition statement and summary has the real answer.

**The staleness problem was NOT caused by OR** — it was caused by the duplicate history append (fix #4). With the duplicate removed, `OR` is safe.

## Diagnostics & Logging (kept)

### GBNF Grammar Application Logging
**File**: `backend/internal/core/assistant/stream.go` `buildChatRequest()`

Added logging for provider-specific output constraints:
- `DEBUG` — grammar applied successfully for a provider type
- `WARN` — grammar could not be built (empty grammar)
- `DEBUG` — no constraint for provider type

Relevant when adding new provider types or debugging tool call enforcement.

## Key Findings (What Didn't Work)

### GBNF Grammar + `tools`/`tool_choice`
llama.cpp **does not support** combining `grammar` (GBNF) with `tools` in the same request. Returns 400: "Cannot use custom grammar constraints with tools." This means `providerConstraints` can only be applied to provider types where `UseNativeTools = false` (so `tools` isn't in the request). For "openai" provider with native tools, GBNF grammar cannot be used.

### Emoji Spam Loop
The model (Qwen3.5-9B) under long-context multi-turn sometimes generates content without calling `submit_final_answer` despite `tool_choice: required`. This is a llama.cpp multi-turn `tool_choice` enforcement limitation. The proxy sends correct constraints but llama.cpp doesn't enforce them in multi-turn.

**Safety net**: `processStream` terminates stream when content exceeds `maxTokens` chars with zero tool calls (`no_tool_content_cap` at `stream.go`). Slow but safe — doesn't cut legitimate long-content generation (which always has tool calls).

### Everything Reverted in `messageBuilder.ts` / `turnGrouper.ts`
Multiple rounds of frontend fixes were applied and reverted. The frontend's message processing was correct — the bugs were all backend (history management, SSE URL, duplicate history append). The lesson: frontend fixes were treating symptoms of backend bugs.

### `handleNoToolCalls` Tools-List Check
Removing the `len(toolsList) == 0` constraint was considered but rejected — it would make the agent accept incomplete mid-task responses as final answers.

### Content-Length Thresholds
All content-length thresholds (500, 1000, 2000, 2730 chars) were considered and rejected as too fragile. The existing `no_tool_content_cap` at `maxTokens` chars is the least-bad option.

### Time-Based Safeguards
30-second heartbeat termination was rejected — legitimate tasks like network scanning or long reports take longer than 30s.

## Sequence of Events (for time-travel debugging)

1. User sends first prompt → model calls tools → `checkSubmitFinalAnswer` with OR condition → reply from summary → duplicate appended to history
2. User sends second prompt in same conversation → context pressure activates sieve → model sees previous answer in history → model copies it into new `submit_final_answer` → OR uses stale summary → user sees wrong content
3. Various fix attempts → each fixed one case and broke another
4. Final solution: remove duplicate history append (stops staleness), keep OR condition (correct for prompt design), keep original prompt (model calls tools reliably)

## Audit Files Created/Updated This Session

| File | Content |
|------|---------|
| `docs/audits/2026-07-05-submit-final-answer-overwrite.md` | submit_final_answer content overwrite, AND/OR tradeoffs |
| `docs/audits/2026-07-06-assistant-debug-cycle.md` | This file — full session log, all bugs found |
