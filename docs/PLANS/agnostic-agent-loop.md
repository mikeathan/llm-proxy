# ARCHITECTURAL BLUEPRINT: Universal LLM-Agnostic Agent Loop (The "Open Claw" Standard)

## 1. Problem Statement & Context
The current agent loop implementation relies heavily on native, provider-specific tool-calling mechanisms (e.g., OpenAI/Gemini JSON schemas sent via the API payload). While highly capable models like Gemma 4 can strictly adhere to these invisible schemas, smaller or differently tuned open-source models (like 20B GGUF models) struggle. 

When running these models locally via llama.cpp, the current architecture fails due to:
1.  **Schema Fragility:** The OSS model wraps JSON in markdown, adds conversational filler, or forgets closing brackets, causing the Go parser to crash or silently fail.
2.  **Context Window Exhaustion:** Unbounded returns from `filesystem` or `network` tools dump massive text blobs into the context window. The model loses its system prompt instructions, leading to "context errors" or amnesia.
3.  **Infinite Loops & Hallucinations:** The agent lacks state awareness. If a tool call fails, the LLM often repeats the exact same failing command infinitely. If it thinks it is done, it simply stops generating without triggering a completion state.
4.  **Implicit Provider Coupling:** The system is not truly "generic" if it relies on the LLM API to format and enforce tool calls.

## 2. The Goal
To decouple the agent's reasoning loop from the LLM provider's API features. We will implement a text-based **ReAct (Reason + Act)** architecture. The agent will interact with tools using highly forgiving, regex-parseable XML tags embedded directly in standard text completions. This ensures the loop runs reliably regardless of the host environment or underlying LLM engine.

---

## 3. Implementation Plan & File Directives

This section contains exact instructions for the AI assistant modifying the codebase.

### Phase 1: The Resilient Tool Parser
**Objective:** Replace strict JSON unmarshalling with regex-based extraction. The parser must find the tool call even if the LLM surrounds it with prose.
**Target File:** `backend/internal/core/proxy/tool_call_parser.go`

**Instructions:**
1.  Refactor `ParseToolCall` (or equivalent method) to process raw string responses.
2.  Implement a cascading fallback parsing strategy:
    * **Priority 1 (Regex XML):** Search the text for a block matching `<tool_call>...</tool_call>`. Extract the interior string and parse it as JSON.
    * **Priority 2 (Regex Markdown):** Search for ` ```json ... ``` ` blocks if the XML tags are missing.
    * **Priority 3 (Greedy JSON):** Attempt to find the first `{` and last `}` and parse the block.
3.  **Crucial:** If parsing completely fails, **DO NOT PANIC OR CRASH**. Return a structured error string: `"SYSTEM ERROR: Malformed tool call. You must wrap your JSON in <tool_call> tags. Example: <tool_call>{\"name\": \"tool\", \"args\": {}}</tool_call>"`. The agent loop must send this string back to the LLM as its next observation.

### Phase 2: The Universal System Prompt
**Objective:** Force the model into a strict text-based reasoning structure and inject the tool manifests into the prompt.
**Target Files:** `backend/internal/core/assistant/prompts/system_prompt.go` (and associated template files)

**Instructions:**
1.  Strip out any logic that attaches the `Tools` array to the LLM API request object.
2.  Instead, serialize the available tools from `backend/internal/core/tools/manifests/` into a structured Markdown list and inject it into the `SystemPrompt` string.
3.  Add the following explicit enforcement block to the system prompt:
    ```
    You operate in a strict Reason -> Act -> Observe loop. 
    You must ALWAYS output your response in the following format:
    
    Thought: [Your detailed reasoning about what to do next based on observations]
    Action: <tool_call>{"name": "tool_name", "args": {"key": "value"}}</tool_call>
    ```
4.  **Define Finality:** Explicitly define a mandatory tool named `SubmitFinalAnswer`. The prompt must state: *"You are not finished until you successfully call the SubmitFinalAnswer tool."*

### Phase 3: The Guardrailed Agent Loop
**Objective:** Give the agent memory of its immediate past to detect infinite loops and enforce termination.
**Target File:** `backend/internal/core/assistant/agent.go`

**Instructions:**
1.  **State Tracking:** Inside the main execution loop, create a history array to track the last 3 `Action` calls (tool name + args hash).
2.  **Duplicate Detection:** Before executing a tool, check if it matches the exact previous call. If it has been called twice in a row, DO NOT execute it. Instead, immediately return an observation to the LLM: `"SYSTEM WARNING: You just repeated an action. It is failing or stuck. Analyze the error and try a completely different approach."`
3.  **Max Iterations Cap:** Implement a hard limit (e.g., `MaxTurns = 15`). If reached, the loop breaks and returns a timeout error.
4.  **Premature Exit Guard:** If the LLM generates a response that contains NO `<tool_call>` tag, do not exit. Inject this observation: `"SYSTEM ERROR: You stopped generating without taking an action or calling SubmitFinalAnswer. Continue your reasoning."`

### Phase 4: Context Window Protection
**Objective:** Prevent large file/network operations from destroying the context window.
**Target Files:** * `backend/internal/core/tools/filesystem.go`
* `backend/internal/core/tools/network.go`
* `backend/internal/core/llm/manager.go`

**Instructions:**
1.  In `filesystem.go` (e.g., `ReadFile` function) and `network.go` (e.g., `HTTPRequest`), implement a character limit check on the return string (e.g., `MaxReturnChars = 4000`).
2.  If the result exceeds the limit, implement a "head and tail" truncation:
    ```go
    // Pseudo-code
    if len(content) > 4000 {
        return content[:1500] + 
               "\n\n... [SYSTEM TRUNCATED " + str(len-3000) + " CHARACTERS TO SAVE CONTEXT] ...\n\n" + 
               content[len(content)-1500:] + 
               "\nSYSTEM NOTE: File too large. Use search/grep tools to target specific lines."
    }
    ```
3.  In `manager.go`, ensure the sliding window drops the oldest `Thought/Action/Observation` turns once the conversation exceeds a predefined token threshold, preserving only the system prompt and the most recent 5-7 turns.

---

## 4. Acceptance Criteria
1. The AI completes a multi-step filesystem modification without the `llm-proxy` passing any native JSON schemas to the provider API.
2. The agent correctly recovers from a hallucinated or misformatted JSON block without terminating the application.
3. Reading a 500KB log file via the agent results in a truncated response, and the loop continues functioning normally.