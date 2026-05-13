# ARCHITECTURAL BLUEPRINT: Universal LLM-Agnostic Agent Loop

**Status: COMPLETE (2026-05-09)**

The agent loop has been hardened per the 13-issue audit in `docs/audits/agent-stability-report.md`. The implementation has diverged from this original plan in several ways based on real-world model testing. See `docs/SPECS/agent-loop.md` for the current specification.

## 1. The Core Problem: Why the Previous Refactors Failed
The "whack-a-mole" bugs (500 Server Errors, infinite **SYSTEM ERROR** loops, and token bleed like `<|tool_call|>`) stem from treating the symptoms rather than the root cause.

### The Root Causes
- **The 500 Server Error**: Caused by passing a `tools` or `functions` array in the JSON payload to the LLM API. Engines like `llama.cpp` intercept the model's output, attempt to natively validate it against the schema, and crash if the model hallucinates or outputs conversational text before the JSON.
- **Token Bleed**: Models fine-tuned for tool calling (like Gemma) try to use their special fine-tuning tokens (`<|tool_call|>`, `<|"|>`) when prompted with tool-like structures. Regex-based replacement of these tokens is endlessly fragile.
- **Over-engineered Parsing**: Using `jsonrepair` and complex backward-scanning makes the system unpredictable. If the LLM produces garbage, the proxy should not try to guess what it meant; it should cleanly force the LLM to correct itself.

## 2. The Universal Goal
Convert the entire agent architecture into a **pure text-in/text-out interface** using standard Markdown code blocks. The LLM inference engine must have **zero knowledge** that tools exist at the API level. Only the Go proxy manages the tool lifecycle.

---

## 3. Implementation Details

### Phase 1: Strip Native Tool Schemas from API Payload
- **Action**: Remove the `Tools` and `ToolChoice` array entirely from the request body sent to the LLM provider in `backend/internal/core/proxy/client.go`.
- **Status**: ✅ DONE — tools stripping removed from client.go. Decision moved to agent level via `UseNativeTools()`.
- **Divergence**: Cloud models may now use native tools when `UseNativeTools()` returns true (Constitution II.5 amendment).

### Phase 2: The "Dumb" Markdown Parser
- **Action (original)**: Rewrite `ParseContentToolCalls` to strictly find the first ` ```json ` block.
- **Status**: ✅ DONE — but with XML tags (`<tool_call>`), NOT markdown fences.
- **Divergence**: XML was chosen over markdown fences because models more reliably produce XML tags and markdown fences appear in normal conversation. The parser is strictly XML-only with no greedy fallbacks. See `docs/SPECS/tool-call-parser.md`.

### Phase 3: Universal System Prompt & Finality
- **Action**: Dynamically serialize available tools into the system prompt.
- **Status**: ✅ DONE — `prompts.BuildToolManual()` injects tools into the system prompt as text.
- **Divergence**: Exit is now explicit via `submit_final_answer` only. The heuristic soft exit (keyword matching) was removed (Constitution II.7).

### Phase 4: Guardrailed Agent Loop & Explicit Error Recovery
- **Duplicate Detection**: ✅ Track last 3 calls, block duplicates after 3-streak.
- **Max Turns**: ✅ Default 25, configurable per-model.
- **Premature Exit Guard**: ✅ `isPrematureTermination()` wired into both automation and chat paths.

### Phase 5: Context Window Protection
- **Truncation**: ✅ 4000-character limits and head-and-tail truncation.
- **Sliding Window**: ✅ Physical sieve keeps Locked Head + last 10 messages.

---

## 4. Acceptance Criteria (Post-Implementation)

1. ✅ HTTP client does not strip tools — the agent controls tool inclusion.
2. ✅ Parser relies strictly on `<tool_call>` XML tags — no markdown, no naked JSON, no jsonrepair.
3. ✅ Large outputs are truncated before entering the context window.
4. ✅ Agent exits via `submit_final_answer` (automation) or explicit chat-mode heuristics.
5. ✅ Per-model configuration (MaxSteps, ContextBudget, ToolCallFormat) flows from ModelConfig to AgentOptions.
6. ✅ Parse errors generate specific, actionable feedback instead of generic nags.
7. ✅ Tool role messages are converted to user role with tool_call_id embedded in content when native tools disabled — avoids Jinja template errors.
8. ✅ Repetition detection survives context sieve boundaries.

## 5. What Changed From This Plan

| Original Plan | Final Implementation | Reason |
|---|---|---|
| Markdown ` ```json ` fences | XML `<tool_call>` tags | Models more reliable with XML; markdown fences ambiguous |
| Pure text-only (no native tools ever) | Native tools allowed for cloud models | Constitution II.5 amended; cloud models handle native tools reliably |
| Heuristic soft exit (keyword matching) | Explicit `submit_final_answer` only | Keywords caused false positives/negatives across model families |
| Max 15 turns | Default 25, per-model configurable | Complex tasks need more turns |
| jsonrepair for malformed JSON | No repair — specific error feedback instead | Repair created phantom tool calls from garbage input |
| Tool→user role conversion | Tool roles converted to user with call_id in content when native=false | Jinja templates in some llama.cpp backends reject orphan tool roles |
| Tool roles preserved | Tool roles preserved when native=true, converted when native=false | Mixed — see above |
