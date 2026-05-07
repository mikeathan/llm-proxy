# ARCHITECTURAL BLUEPRINT: Universal LLM-Agnostic Agent Loop ()

## 1. The Core Problem: Why the Previous Refactors Failed
The "whack-a-mole" bugs (500 Server Errors, infinite **SYSTEM ERROR** loops, and token bleed like `<|tool_call|>`) stem from treating the symptoms rather than the root cause. 

### The Root Causes
*   **The 500 Server Error**: Caused by passing a `tools` or `functions` array in the JSON payload to the LLM API. Engines like `llama.cpp` intercept the model's output, attempt to natively validate it against the schema, and crash if the model hallucinates or outputs conversational text before the JSON.
*   **Token Bleed**: Models fine-tuned for tool calling (like Gemma) try to use their special fine-tuning tokens (`<|tool_call|>`, `<|"|>`) when prompted with tool-like structures. Regex-based replacement of these tokens is endlessly fragile.
*   **Over-engineered Parsing**: Using `jsonrepair` and complex backward-scanning makes the system unpredictable. If the LLM produces garbage, the proxy should not try to guess what it meant; it should cleanly force the LLM to correct itself.

## 2. The Universal Goal
Convert the entire agent architecture into a **pure text-in/text-out interface** using standard Markdown code blocks. The LLM inference engine must have **zero knowledge** that tools exist at the API level. Only the Go proxy manages the tool lifecycle.

---

## 3. Implementation Details

### Phase 1: Strip Native Tool Schemas from API Payload
*   **Action**: Remove the `Tools` and `ToolChoice` array entirely from the request body sent to the LLM provider in `backend/internal/core/proxy/client.go`.
*   **Impact**: Stops LLM engines from attempting to validate schemas and crashing.

### Phase 2: The "Dumb" Markdown Parser
*   **Action**: Rewrite `ParseContentToolCalls` to strictly find the first ` ```json ` block and extract the JSON.
*   **Standard**: Use standard Go `json.Unmarshal`. No more `jsonrepair` or backward-scanning.
*   **Error Handling**: If parsing fails, return a `system_error` tool call that instructs the model on the correct format.

### Phase 3: Universal System Prompt & Finality
*   **Action**: Dynamically serialize available tools into the system prompt.
*   **Format**: Enforce a strict `Thought: ... Action: ` + ` ```json ` format.
*   **Finality**: Define `submit_final_answer` as the mandatory exit tool.

### Phase 4: Guardrailed Agent Loop & Explicit Error Recovery
*   **Duplicate Detection**: Track the last 3 calls. Intercept consecutive identical calls with a specific warning.
*   **Max Turns**: Enforce a hard cap of **15 turns**.
*   **Premature Exit Guard**: If generation stops without an action, nag the model with a specific error message.

### Phase 5: Context Window Protection
*   **Truncation**: Implement 4000-character limits and head-and-tail truncation in `ReadFile` and `FetchURL` tools.
*   **Sliding Window**: Keep only the system prompt and the most recent 10 messages in the conversation history.

---

## 4. Acceptance Criteria
1. The proxy passes **pure text** to the LLM engine, omitting all native `tools`/`functions` arrays.
2. The parser relies strictly on standard ` ```json ` markdown fences.
3. Large file reads do not cause context exhaustion.
4. The agent correctly terminates via `submit_final_answer`.