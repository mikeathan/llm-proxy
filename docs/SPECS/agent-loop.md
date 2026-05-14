# SPEC: Agent Loop

## I. Intent
The agent loop (`assistant/agent.go`) executes multi-turn tool-augmented conversations. It orchestrates model inference, tool call parsing, tool execution, and history management. It must be model-agnostic — supporting both local models (via text-based XML tool calls) and cloud models (via native tool schemas) — and resilient to model-specific failures.

## II. Functional Requirements

### 1. Main Execute Loop
- Accept a conversation `history` and iterate up to `maxSteps` turns (default 25, overridable per-model).
- Each turn: list tools, check context budget, call the LLM, parse tool calls, validate against registry, execute, append results.
- Global timeout: 30 minutes. Per-turn timeout: 10 minutes.

### 2. Context Budget & Physical Sieve (Constitution II.6)
- Default budget: 15,000 characters (overridable via `ModelConfig.ContextBudget`).
- When exceeded: keep system message + first user message (Locked Head), insert sieve marker, keep last 10 messages (Priority Tail).
- **Critical**: `recentCalls` (repetition detector) MUST survive the sieve boundary.

### 3. Tool Call Extraction (Constitution II.4)
- XML-only: `<tool_call>{"tool":"...","args":{...}}</tool_call>`.
- Parsed by `proxy.ParseContentToolCalls()`.
- Failed parses return a structured `ParseError` with `XMLFound`, `JSONError`, and `ToolName` fields.
- Valid tool calls are validated against the provider's tool list via `proxy.ValidateToolCall()`.

### 4. Repetition & Loop Detection
- Track up to 3 recent `(name, args)` pairs.
- Consecutive identical calls increment `duplicateStreak`.
- At 3 duplicates: hard error ("infinite loop detected").
- `submit_final_answer` and `system_error` tools are exempt.

### 5. Exit Heuristics

**Automation mode:**
- Canonical path: `submit_final_answer` tool call (extracts summary from args).
- Premature termination guard: empty output or 3 consecutive identical assistant messages without tool calls.

**Chat mode:**
- Step 1: single-turn response with no tool calls → exit.
- 2 consecutive assistant messages with no tool calls → exit.
- Assistant reply immediately follows a tool result (`precededByToolResult`) → exit.
- Premature termination → exit.

### 6. Error Recovery
- Parse errors: inject `ParseError.Feedback(availableTools)` — specific format guidance with valid tool names.
- No parse error but no tool calls: inject `AutomationNagPrompt`.
- Tool validation failure: treat as parse error, clear invalid tool calls, inject feedback.

### 7. Native Tool Support (Constitution II.5)
- Controlled by `Agent.useNativeTools` (resolved from `AgentOptions.UseNativeTools` > `ToolProvider.UseNativeTools()`).
- When true: native tool schemas passed in `req.Tools`, LLM server sends tool-call deltas natively.
- When false: tools stripped, tool manual injected into system prompt as text.
- The HTTP client (`client.go`) does NOT strip tools — the agent controls this.

### 9. Guardrail Decision Flow (Constitution II.10)
- When `guardrails.ValidateToolCall()` rejects a tool call, the agent invokes `onGuardrail(ctx, payload)` if set.
- The payload includes: `DecisionID`, `Tool`, `Args`, `Reason`, `Category` (terminal/filesystem/network/search/communication).
- The callback blocks until: user approves (`Allow: true`), user rejects (`Allow: false`), timeout fires (60s default), or context is cancelled.
- If approved with `Persist: true`: the guardrail engine saves an override to the workspace config (`config.yaml`) so future matching calls pass without blocking.
- In automation mode (no user present), the callback is nil and guardrail violations fail immediately with an error tool result.
- The decision store (`assistant/guardrail_decision.go`) provides concurrent-safe Register/Resolve/Remove operations.

### 8. History Normalization (Constitution II.8)
- `NormalizeHistory()`: strips `ToolCalls` when `useNativeTools=false`. Converts `tool` role → `user` role with `tool_call_id` embedded in content (`Tool result [call_N]: <json>`) to avoid Jinja template errors in llama.cpp while preserving call/result association. No auto-nags. Consolidates consecutive same-role messages.
- `SanitizeHistory()`: preserves `role`, `content`, `tool_calls`, `tool_call_id`.

## III. Technical Architecture

### Component Map
```
automation/executor.go          — task dispatch, per-model config → AgentOptions
assistant/agent.go              — Execute loop, sieve, tool processing
assistant/registry.go           — wires LocalToolRegistry + MCP → provider/engine
assistant/tool_provider.go      — MultiToolProvider, CompositeEngine, ToolProvider interface
assistant/content_filter.go     — streaming markup filter
assistant/guardrails/           — tool-call guardrail validation
assistant/prompts/              — system prompts, nag prompts, tool manual builder
proxy/client.go                 — HTTP pipe to LLM servers (Chat + Stream)
proxy/history.go                — NormalizeHistory, SanitizeHistory, NormalizeContextSieve
proxy/tool_call_parser.go       — XML-only tool call extraction, ParseError, validation
models/config.go                — ModelConfig with per-model agent tuning fields
```

### Data Flow (single turn)
```
Executor → Agent.Execute()
  → provider.ListTools()
  → check context budget (sieve if needed)
  → computeNextResponse()
    → prepareMessages() → NormalizeHistory()
    → client.Stream() / client.Chat()
    → processStream() or computeNextResponseNonStreaming()
  → handleContentToolCalls() → ParseContentToolCalls()
  → ValidateToolCall() against tool list
  → deduplicate + repetition check
  → processToolCalls() → guardrails.ValidateToolCall() → onGuardrail() (if blocked) → engine.ExecuteTool()
  → append tool results to history
  → check exit conditions
```

### Per-Model Configuration
| Field | Default | Purpose |
|---|---|---|
| `ModelConfig.MaxSteps` | 25 | Max agent loop iterations |
| `ModelConfig.ContextBudget` | 15000 | Char count triggering sieve |
| `ModelConfig.ToolCallFormat` | (empty) | `"native"` to force native tools |

## IV. Testing Strategy
- Unit tests: `agent_test.go` covers simple execution, tool calls, loop detection, streaming, premature termination, `precededByToolResult`.
- Parser tests: `tool_call_parser_test.go` covers XML format, malformed JSON, missing tool fields, unsupported tags, tool validation, `ParseError.Feedback`.
- Content filter tests: `content_filter_test.go` covers markup detection edge cases.

## V. Constitutional References
- II.4: Unambiguous Tool Boundaries (XML-only)
- II.5: Text-First Tool Interface (native tools for cloud)
- II.6: Token Budgeting & Structural Sieve
- II.7: Explicit Task Completion (submit_final_answer)
- II.8: Context-Preserving Normalization
- II.10: Guardrail Decision Flow (user approval for blocked tool calls)
