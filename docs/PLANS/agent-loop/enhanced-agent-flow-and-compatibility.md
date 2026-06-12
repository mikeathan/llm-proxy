---
status: partial
related_specs: [SPEC-001]
---
# Plan - Enhanced Agent Flow & Model Compatibility

**Status: PARTIAL**

Implemented: `toolErrorThisTurn` flag with relaxed stuck threshold, `ToolErrorNagPrompt` on tool failures, `normalizeKey` for terminal command dedup, sliding-window repetition detector, non-streaming stuck detector with progressive sieve recovery. Pending user review on parallel tool error handling and auto-prefill-disabling proposals.

This plan proposes key enhancements to the agenting loop, proxy, and parser subsystems to improve compatibility with 2025 LLMs (specifically reasoning models like DeepSeek R1, OpenAI o1/o3-mini, and smaller local models).

## User Review Required

> [!IMPORTANT]
> - **Parallel Tool Execution Error Handling**: When a tool call in a batch fails, the current loop exits immediately. This leaves subsequent tool calls without results, which causes strict cloud providers (like OpenAI and Gemini) to return `400 Bad Request` API errors. We propose to skip execution of subsequent tools on failure but still append a cancelled result for them to keep the history balanced.
> - **Auto-disabling Prefill on Failures**: If a model gets stuck in a formatting/parsing loop due to a static prefill, we dynamically disable prefill for that turn/retry.
> - **Inline Thinking Extraction**: Extract `<think>...</think>` tags from incoming stream content and route them to `ReasoningContent` rather than leaving them in conversational history.

## Proposed Changes

### Core Agent Loop & Parser

---

#### [MODIFY] [agent.go](file:///Users/mikeathan/dev/llm-proxy/backend/internal/core/assistant/agent.go)
- **Inline Thinking Extraction**: Parse and separate `<think>...</think>` tags in both streaming and non-streaming responses. Update `processStream` and `computeNextResponseNonStreaming` to use `ExtractInlineThinking` so that reasoning content is fully stripped from history and routed to the UI separately.
- **Dynamic Prefill Disable**: In `Execute()`, if `parseErrorStreak >= 2`, disable prefill for subsequent retries/turns.
- **Batch Tool Call Completion on Failure**: In `processToolCalls()`, if one tool in a batch fails, mark the batch as failed. For all remaining tool calls in the same batch, skip execution but append a cancelled result (`{"error": "Cancelled: previous tool in batch failed"}`) to satisfy API-level balance requirements.

---

#### [MODIFY] [tool_call_parser.go](file:///Users/mikeathan/dev/llm-proxy/backend/internal/core/proxy/tool_call_parser.go)
- **JSON Auto-Closing**: Add `autoCloseJSON()` helper to scan and auto-close unclosed string literals and open curly braces/brackets in truncated JSON outputs.
- **Flat JSON Support**: Enhance `parseSingleToolCall()` to handle flat JSON format. If the `"args"` key is missing or empty, but there are other keys at the root of the JSON (other than `"tool"`), group those keys under the `"args"` payload.
- **ExtractInlineThinking helper**: Implement `ExtractInlineThinking(raw string) (content string, reasoning string)` to extract `<think>...</think>` blocks.

---

#### [MODIFY] [budget_squeezer.go](file:///Users/mikeathan/dev/llm-proxy/backend/internal/core/orchestrator/budget_squeezer.go)
- **Local Model tool_call_format Default**: Update `ApplyMetadataDefaults` so it does not set `cfg.ToolCallFormat = "native"` for `"local"` models when empty, ensuring they default to XML text mode.

## Verification Plan

### Automated Tests
We will add new unit tests in the following files:
- `agent_test.go`:
  - `TestAgent_Execute_InlineThinkingExtraction` (verifies streaming/non-streaming extraction of `<think>` blocks).
  - `TestAgent_Execute_BatchToolExecutionCancellation` (verifies subsequent tools in a batch are cancelled but still have results appended).
  - `TestAgent_Execute_PrefillDisableOnParseErrorStreak` (verifies prefill is disabled after consecutive parse errors).
- `tool_call_parser_test.go`:
  - `TestParseSingleToolCall_AutoCloseJSON` (verifies auto-repair of unclosed JSON strings).
  - `TestParseSingleToolCall_FlatJSON` (verifies top-level arguments wrapping).

### Manual Verification
- We can inspect code consistency with existing test suites.
- The user can verify the fixes directly on their local models (e.g. Qwen 3.5 or DeepSeek R1).
