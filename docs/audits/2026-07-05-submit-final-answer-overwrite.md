# Audit: submit_final_answer content overwrite

**Date**: 2026-07-05
**Severity**: medium
**Subsystem**: assistant backend (`session.go`)

## Symptom

User sends "tell me a joke" in an existing conversation after a "list all files and report" request. The assistant bubble's **Result** section displays the *previous* request's "Complete File Listing Report" instead of the joke. The reasoning section correctly shows the model thinking about telling a joke, confirming the model did generate the correct response.

Events.jsonl from the run (`conv_20260705201229/Qwen3.5-9B-UD-Q4_K_XL.gguf`) shows:
- **`tool_stream` events**: the joke text "Here's a joke for you: Why don't scientists trust atoms?…" (correct)
- **`tool_call` event**: `submit_final_answer` with `task_summary` = the full file listing report (incorrect — model included previous request's report under context pressure)
- **`message` event**: `content` = joke text (correct), `tool_calls` args contain the file listing report

## Root cause (primary)

The submit_final_answer tool call's **full content was stored TWICE** in the LLM conversation history:

1. **Line 218**: `turnMsg` — the model's original response (content + tool_calls including `submit_final_answer`)  
2. **Lines 234-237** (now removed): a separate message with `Role: AssistantRole` containing just the full content

When the next user message arrived in the same conversation, the model saw message #2 — a plain assistant message with the previous full report content. Under context pressure, the model **copied this content verbatim into the new `submit_final_answer` tool call's `task_summary` field**.

The `OR` condition in `checkSubmitFinalAnswer` then saw the meaningful summary and overwrote the new Content (the joke) with the stale report data.

The `checkSubmitFinalAnswer` condition was originally `OR` by design (system prompt instructs model to put answer in summary). The real issue was the **duplicate content in the LLM history** allowing the model to copy old data.

## Root cause (secondary — refactoring-introduced)

The refactoring in this branch (`task/communication_tool_calling`) changed `checkSubmitFinalAnswer` from `OR` to `AND` at line 141, which partially mitigated the symptom but broke the normal case (transition Content overwritten by real summary). This was reverted.

## Resolution

**Primary fix**: Removed the duplicate history append (`session.go:234-237`). The full final answer content is no longer stored in the LLM history. It still flows to the frontend via the `reply` HTTP response field and the session DB.

```go
// Removed (lines 234-237):
s.history = append(s.history, proxy.Message{
    Role:    proxy.AssistantRole,
    Content: content,
})
// The turnMsg (with content + tool calls) at line 218 already provides sufficient
// LLM context. The model knows submit_final_answer was called and the task is done.
```

**Secondary fix**: Aligned the system prompt with `checkSubmitFinalAnswer` logic to make Content the authoritative answer field:

- **`system_prompt.go:14`** — Changed from "answer MUST be in the summary argument, keep Content brief" to "write full answer in Content, summary should be a brief label only"
- **`templates.go:80, 98-101`** — Same update for non-native mode and tool references
- **`session.go:141`** — AND condition (prefer Content, fallback to summary when Content is empty)

This dual alignment (prompt + code) removes the root confusion:
- The model is told where to put the answer (Content), so it follows the expected pattern
- The code prefers Content, so if the model deviates, it still gets the best available text
- If Content is empty (model only called the tool), summary fills in as fallback

**Siege wording fix**: Changed `ContextSieveWarning` from "finalize when ready" to "call submit_final_answer when done" to reduce model confusion during context pressure.

## Tests added

| Test | What it verifies |
|------|-----------------|
| `TestAgent_SubmitFinalAnswer_NoDuplicateInHistory` | After agent execution, the LLM history contains the `turnMsg` (with ToolCalls), not a duplicate content-only message. The `reply` correctly contains the full answer. |
| `TestCheckSubmitFinalAnswer_ContentOverwrite` | `checkSubmitFinalAnswer` AND behavior for 6 combinations of empty/non-empty Content + meaningful/"Task complete."/empty Summary |
| `TestAgent_ToolCallParseErrorRetry` | After tool call parse error retry, Content is preserved over summary in the final answer |

## Lessons

- **Separate LLM context from session transcript.** The LLM history should contain only what the model needs for context continuity. Full response content belongs in the session transcript (for user display), not in the LLM context (where it becomes a template for copying).
- **Model behavior under context pressure**: Models may copy content from previous assistant messages into new tool call arguments. The defense is to keep previous full responses out of the LLM history.
- **Tool call arguments are the model's own data.** Do not store them as content-only assistant messages in the LLM history — store the original `turnMsg` with both content and tool calls.
- **Align prompt and code.** When the system prompt tells the model one thing ("put answer in summary") and the code does the opposite, you get unreliable behavior. Both must tell the same story.
- **AND condition is more forgiving.** When Content has the answer, keep it. When it's empty, fall back to summary. This avoids the OR condition's failure mode where metadata in the summary overwrites the real answer in Content.
- Refer to openclaw for the correct pattern: LLM history → model context only, Session transcript → full content for user.
