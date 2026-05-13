
# Agent Stability Analysis Report

## Implementation Status (2026-05-09)

| # | Issue | Severity | Status |
|---|---|---|---|
| 1 | Tool Call Parser | CRITICAL | ✅ Fixed — XML-only, structured ParseError |
| 2 | History Normalization | CRITICAL | ✅ Fixed — tool role preserved, auto-nag removed |
| 3 | Streaming Tools Stripped | CRITICAL | ✅ Fixed — moved to agent level |
| 4 | Error Recovery Too Generic | HIGH | ✅ Fixed — ParseError.Feedback() |
| 5 | Soft Exit Heuristic | HIGH | ✅ Fixed — replaced with explicit exit rules |
| 6 | No Per-Model Calibration | HIGH | ✅ Fixed — ModelConfig fields wired to AgentOptions |
| 7 | Physical Sieve | HIGH | ✅ Fixed — recentCalls survives sieve |
| 8 | Dead Code: isPrematureTermination | MEDIUM | ✅ Fixed — wired into Execute loop |
| 9 | Stream Fallback Latency | MEDIUM | ⏸ Deferred — requires streaming architecture change |
| 10 | CompositeEngine Fallback | MEDIUM | ✅ Verified — engine handles fallback internally |
| 11 | FilterStreamingMarkup Tests | MEDIUM | ✅ Fixed — comprehensive edge case tests added |
| 12 | Stream Tool Call Accumulation | MEDIUM | 📝 Documented — dormant path when useNativeTools=false |
| 13 | Retry Dead Code | LOW | 📝 Documented — safety net for native-tool flows |

Refer to `docs/SPECS/agent-loop.md` for the current architecture specification.
Refer to `docs/SPECS/tool-call-parser.md` for the parser behavior specification.
Refer to `CONSTITUTION.md` for the amended Constitutional laws (II.5–II.9).
Refer to `docs/PLANS/agnostic-agent-loop.md` for the original design plan and implementation divergences.

## Summary

The agent loop has **13 distinct issues** that combine to create fragile,
model-dependent behavior. The root cause is not one bug but a system that relies
on heuristic text parsing for tool calls while lacking per-model calibration and
specific error recovery. Below is a detailed breakdown of every issue ranked by
severity, with concrete fixes.

---

## CRITICAL: Issues That Cause Hard Failures

### 1. Tool Call Parser Is a House of Cards

**File:** `backend/internal/core/proxy/tool_call_parser.go`

The parser has 3 phases, each increasingly desperate:

**Phase 1 — XML tags:** Uses fuzzy regex `<tool(?:_call)?>.*?</?tool(?:_call)?>`.
OK in principle, but `sanitizeJSON()` calls `jsonrepair.Repair()` which can
"fix" garbage into valid-but-wrong JSON, producing phantom tool calls with
nonsensical arguments.

**Phase 2 — Naked JSON (THE WORST OFFENDER):**
```go
start := strings.Index(content, "{")
end := strings.LastIndex(content, "}")
jsonStr := sanitizeJSON(content[start : end+1])
```
This grabs from the **first** `{` to the **last** `}` in the entire response.
If a model outputs:
```
I checked the {config file} and here is the result.
{"tool": "write_file", "args": {"path": "out.txt", "content": "done"}}
```
It extracts `{config file} ... {"tool": "write_file"...}`, feeds that to
jsonrepair, and gets either garbage or a hallucinated tool call.

**Phase 3 — Ultra-Greedy Key-Value:** Wraps arbitrary content in `{...}` if
it contains `"tool":` and `"args":` anywhere. This triggers on conversational
text that mentions tools.

**Fix:**
- Delete Phase 2 and Phase 3 entirely. They cause more false positives than
  true recoveries.
- Phase 1 (XML) is the Constitution-mandated format. If the model doesn't
  produce valid `<tool_call>` XML, give it **specific** feedback about what
  failed instead of trying to guess.
- After extracting JSON from XML, validate `tool` field against the actual
  available tools list before accepting it.

### 2. History Normalization Destroys Context for Local Models

**File:** `backend/internal/core/proxy/history.go`, lines 23-28

```go
if !useNativeTools {
    newMsg.ToolCalls = nil
    if msg.Role == ToolRole {
        newMsg.Role = UserRole
        newMsg.Content = fmt.Sprintf("Observation: %s", msg.Content)
    }
}
```

When `useNativeTools` is false (which it always is — `MultiToolProvider` hardcodes
`return false`), every `tool` role message becomes a `user` role message prefixed
with "Observation:". Then the consolidation pass (lines 41-56) merges consecutive
same-role messages. This means:

- Two consecutive tool results get smushed into one "Observation: ... Observation: ..."
- The association between a specific tool call and its result is lost
- The model can't tell which result corresponds to which tool call

Additionally, lines 69-73 auto-append a nag:
```go
if !useNativeTools && len(merged) > 0 && merged[len(merged)-1].Role == AssistantRole {
    merged = append(merged, Message{
        Role:    UserRole,
        Content: "Observation: No action taken in last turn. You MUST provide a <tool_call> tag to proceed.",
    })
}
```
This fires on **every** call, even when the assistant just gave a valid final
answer on step 1 of a non-automation conversation.

**Fix:**
- Keep `tool` role messages as a distinct role. Most local LLM servers (llama.cpp,
  ollama) support the `tool` role natively even if they don't support the `tools`
  schema.
- Remove the auto-nag from NormalizeHistory. It doesn't belong in a normalization
  function — nags should only happen in the agent loop when a turn actually
  produced no action.
- If you must convert tool roles, use a format that preserves tool_call_id:
  `Observation [call_123]: <result>`

### 3. Streaming Tools Are Always Stripped, Even for Cloud Models

**File:** `backend/internal/core/proxy/client.go`, lines 72-73 and 111-112

```go
// Chat():
req.Tools = nil
req.ToolChoice = ""

// Stream():
req.Tools = nil
req.ToolChoice = ""
```

The Constitution mandates "Pure Text Interface" for tool calling. But this
blanket-strip happens at the HTTP client level, making it impossible to use
native tool calling even for cloud models (OpenAI, Gemini) that handle it
reliably. The system forces ALL models through the fragile XML text parser.

**Fix:**
- Move the tools-stripping decision to the agent level, where `ToolProvider.UseNativeTools()`
  already exists as a decision point.
- Let the HTTP client pass through whatever the agent gives it.
- For cloud models that support native tools reliably, `UseNativeTools()` should
  return `true` and the agent should pass native tool schemas.
- For local models, continue with pure text tool calling.

---

## HIGH: Issues That Cause Flaky Behavior

### 4. Error Recovery Is Too Generic

**File:** `backend/internal/core/assistant/agent.go`

When `ParseContentToolCalls()` fails, the model gets:
```
SYSTEM ERROR: NO ACTION DETECTED. You are monologuing.
YOU MUST START YOUR RESPONSE WITH THE CHARACTER '<'. NO PREAMBLE.
```

This doesn't tell the model WHAT went wrong. The model may have tried to produce
a tool call but got the format slightly wrong. Without specific feedback, it will
either repeat the same mistake or try a different format that also fails.

**Fix:**
- When XML tags are found but JSON parsing fails inside them, say: "I saw your
  `<tool_call>` tags but couldn't parse the JSON inside. Ensure you use double
  quotes for keys and values. The 'tool' field must match an available tool name."
- When no XML tags are found at all, say: "Wrap your tool call in `<tool_call>`
  tags. Example: `<tool_call>{"tool":"read_file","args":{"path":"file.txt"}}</tool_call>`"
- Show the model the list of valid tool names in the error message.

### 5. The Soft Exit Heuristic Is Unreliable

**File:** `backend/internal/core/assistant/agent.go`, lines 218-242

The "Agnostic Sentence Guard" checks for:
- Final punctuation (`.`, `!`, `?`, `}`, ` ``` `)
- NOT containing `<tool` or `"tool":`
- Containing `task complete`, `summary`, or `final report`
- Passing `isFinalReport()` which checks for markdown headers and backtick balance

Problems:
- **False positives:** A model saying "Let me check the config file and then
  I'll write a summary" triggers the exit because it contains "summary" and ends
  with punctuation.
- **False negatives:** GPT-OSS 20B or Gemma models often phrase completions
  differently ("Done.", "Here you go:", "All finished!") and won't match these
  specific keywords.
- Models that output markdown reports with code blocks (```) may fail the
  backtick-balance check if a code block is incomplete in the stream.

**Fix:**
- The `submit_final_answer` tool should be the **only** path to task completion
  in automation mode.
- Remove the soft exit heuristic entirely for automation tasks.
- Instead, if a model produces text without tool calls for 2 consecutive turns
  in automation mode, inject a message saying: "You have not called any tools
  for 2 turns. If you are done, call `submit_final_answer`. Otherwise, continue
  with your next tool call inside `<tool_call>` tags."
- For chat mode (non-automation), keep the simple step-1 or 2-consecutive-chat
  exit rule but remove the heuristic keyword matching.

### 6. No Per-Model Calibration

The following thresholds are global constants:
- `MaxSteps = 25` — Some models need more turns because they're verbose
- `totalChars > 15000` for context sieving — Many local models support 128K+
  context; sieving at 15K throws away useful context
- `MaxReturnChars = 3000` for tool results — Too aggressive for large models
- Soft exit keyword matching — Language-specific; fails on non-English or
  differently-trained models

**Fix:**
- Make these per-model configuration options in `ModelConfig`.
- Default to conservative values for unknown models.
- Auto-detect context length from GGUF metadata (you already have GGUF scanning
  in `gguf_scanner.go`) and set sieving threshold as a fraction of max context.

### 7. The Physical Sieve Corrupts Automation State

**File:** `backend/internal/core/assistant/agent.go`, lines 104-129

When the 15K char limit is hit:
1. History is pruned to system + first user + recent 10 messages
2. "Memory Sieve" system note is injected
3. `recentCalls` repetition detector is reset

The problem: `recentCalls` reset means the model can repeat the same tool call
that was just made before the sieve, creating a loop that spans the sieve boundary.

**Fix:**
- Don't reset `recentCalls` on sieve. The repetition state should survive pruning.
- Consider tracking tool names (not full args) for repetition detection across
  the sieve boundary.

---

## MEDIUM: Issues That Degrade Experience

### 8. Dead Code: `isPrematureTermination` Never Called

**File:** `backend/internal/core/assistant/agent.go`, lines 432-447

`isPrematureTermination()` is defined and tested but never invoked in the main
`Execute` loop. Either wire it in or remove it.

### 9. Stream Fallback Doubles Latency on Empty Responses

**File:** `backend/internal/core/assistant/agent.go`, lines 336-339

When streaming produces empty content (common with local models during long
prefill), the agent falls back to a non-streaming call. This means the user
waits for the stream timeout PLUS the non-streaming call. Total: up to
`StreamChunkTimeout (5 min) + AgentTurnTimeout (10 min) = 15 min`.

**Fix:**
- Start both stream and non-stream in parallel, take the first successful response.
- Or: if streaming yields content but no tool calls, don't retry — just use the
  content.

### 10. Unused `compositeEngine.Secondary` in Tool Execution

**File:** `backend/internal/core/assistant/tool_provider.go`, lines 72-86

The `CompositeEngine` tries the primary engine first and falls back to secondary
only on `ErrToolNotInternal`. But the agent's `processToolCalls` method returns
`nil` for errors (line 517), meaning even if a tool fails, processing continues
without trying the secondary engine.

**Fix:**
- Errors from primary engine should propagate so CompositeEngine can try the
  secondary. Currently errors are handled at the agent level with generic
  "tool execution failed - stopping batch" behavior.

### 11. `FilterStreamingMarkup` Has No Unit Tests

**File:** `backend/internal/core/assistant/content_filter.go`

The streaming content filter strips content when it detects tool-call-like
patterns. If the cutoff patterns are too aggressive, legitimate content gets
hidden from the UI. If too lenient, raw JSON/XML flashes to the user.

**Fix:**
- Add table-driven tests for various streaming content prefixes.
- Ensure the filter doesn't truncate normal text that happens to contain
  characters like `{` or `<` that aren't tool calls.

### 12. Stream Tool Call Accumulation Is Dead Code Path

**File:** `backend/internal/core/assistant/agent.go`, lines 374-382

The native streaming tool call accumulation (`tc.ID` matching, argument
concatenation) never runs because `Stream()` strips `req.Tools = nil`, so
the LLM server never sends native tool call deltas.

**Fix:**
- If native tool calling is restored for cloud models, this code path needs
  testing. Currently it's untested dead code.

---

## LOW: Code Quality Issues

### 13. `computeNextResponseNonStreaming` Retry Logic Is Mostly Dead

**File:** `backend/internal/core/assistant/agent.go`, lines 389-430

The `isToolSupportError()` check on line 413 catches errors like "tools is not
currently supported". But since tools are always stripped by `Chat()`, the
LLM server never sees tools and never returns these errors. The entire retry
block (lines 413-418) is dead code.

---

## Concrete Action Plan

### Step 1: Fix the HTTP Client (1-2 hours)

Move tools-stripping out of `client.go` and into the agent. The client should
be a dumb pipe:

```go
// client.go - REMOVE these lines from both Chat() and Stream():
// req.Tools = nil
// req.ToolChoice = ""
```

Instead, let `agent.go` control whether tools are included based on
`UseNativeTools()` (which already exists and already works correctly).

### Step 2: Harden the Parser (2-3 hours)

1. Delete Phase 2 and Phase 3 from `ParseContentToolCalls()`.
2. Keep only Phase 1 (XML tags) as the Constitution mandates.
3. After extracting JSON from XML, validate the `tool` field against known tools.
4. Return a specific, descriptive parse error that the agent can feed back to
   the model.

### Step 3: Fix History Normalization (1-2 hours)

1. Stop converting `tool` roles to `user` roles. Pass `tool` role messages
   through — llama.cpp and ollama support them.
2. Remove the auto-nag from `NormalizeHistory`.
3. If `tool` role is truly unsupported by a backend, handle it in the provider
   adapter, not in the shared normalization path.

### Step 4: Replace Generic Nags with Specific Error Feedback (2-3 hours)

When a turn produces no valid tool calls, analyze WHY:
- No `<tool_call>` tags found → tell model the required XML format
- Tags found but JSON parse failed → show the parse error and expected format
- Tool name not recognized → list valid tool names
- Missing required args → show the required parameters

### Step 5: Remove Soft Exit, Rely on `submit_final_answer` (1 hour)

The soft exit heuristic is a band-aid. Remove it. If models struggle to call
`submit_final_answer`, the fix is better prompting, not heuristic guesswork.

### Step 6: Per-Model Configuration (2-3 hours)

Add to `ModelConfig`:
```go
type ModelConfig struct {
    // ... existing fields ...
    MaxSteps       int `json:"max_steps,omitempty"`        // default 25
    ContextBudget  int `json:"context_budget,omitempty"`   // default 15000
    ToolCallFormat string `json:"tool_call_format,omitempty"` // "xml" or "native"
}
```

Read context window size from GGUF metadata where available and auto-set
`ContextBudget` as a fraction (e.g., 80%) of the total.

### Step 7: Add Streaming Tests (1-2 hours)

The mock client doesn't test streaming. Add:
- Test: stream returns partial content across multiple chunks
- Test: stream with embedded XML tool calls in content
- Test: stream empty, fallback to non-streaming
- Test: stream with both content AND tool calls interleaved

---

## Why Models Like GPT-OSS 20B Fail Most

GPT-OSS 20B is a smaller model (20B parameters) that was likely not fine-tuned
for structured tool calling. The system demands it:

1. Follow a specific XML format (`<tool_call>` + JSON)
2. Understand a complex system prompt with 4+ sections (rules, tool manual,
   tool interface, workspace rules)
3. Never output conversational text without tool calls in automation mode
4. Know to call `submit_final_answer` to exit
5. Produce JSON with correct quoting and no trailing commas

A 20B model has limited instruction-following precision. It will frequently:
- Start tool calls but produce malformed JSON
- Forget the XML wrapper and output naked JSON
- Drift into conversational text
- Generate JSON with trailing commas or unquoted keys
- Mix conversational text with tool calls

The current parser tries to compensate for all these failures with regexes and
jsonrepair, but this creates false positives. Then the generic nags confuse the
model further, creating a downward spiral.

**The fix for small/local models specifically:**
- Shorter system prompts with fewer sections
- Simpler tool format (consider a flat key=value format as an alternative to
  JSON for simple tools)
- Fewer tools presented at once (only show relevant tools based on task type)
- More patient error recovery with specific format examples in error messages
