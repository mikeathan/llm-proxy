# Agent Loop — Execution, Sieve, Stuck Detection & Fallback — Tool-Call Data Flow, Batches and GBNF

Reference for the [`agent-loop`](../SKILL.md) skill, loaded on demand. You are changing how tool calls are parsed, batched, constrained by grammar, or executed.

Contents: Output Constraint (GBNF Grammar) · Tool-Call Batches · Tool Call Data Flow

## Output Constraint (GBNF Grammar)

The agent does **not** send a GBNF grammar on native-tools requests to local
llama.cpp. llama.cpp — the only server that accepted GBNF — rejects a custom
grammar combined with native `tools` in one request (HTTP 400 "Cannot use
custom grammar constraints with tools."), and every local llama.cpp is reached
over the OpenAI-compatible wire. Attaching the grammar made each tool-using
turn fail before generation (2026-09-05: assistant execution failed on the
locally-managed llama.cpp path while the same model via an OpenAI-style
registration worked). Native tool-call arguments are already constrained by the
server's own tool template, and llm-proxy validates results post-hoc
(`handleContentToolCalls` + `ValidateToolCall`), so no grammar is attached on
any path today:

- **Native tools (local or cloud):** no grammar — `buildChatRequest` never sets
  `req.Grammar`; the server returns structured JSON that the parser validates.
- **XML text mode:** no grammar either — the model writes
  `<tool_call>{"tool":…}</tool_call>` free-text and parse failures feed the
  format-error feedback loop (SPEC-002), never a generation-level constraint.
- The bare-JSON GBNF builder remains available as `proxy.GBNFConstraint`
  (unit-tested) for a future non-tools/JSON-mode use; it is deliberately not
  wired to native-tools requests.
- **Re-enabling grammar is tracked, not forgotten:** an opt-in, llama.cpp-safe
  re-enablement (envelope-aware grammar for XML text mode, never with native
  tools, per-model toggle) is planned in
  `docs/PLANS/cross-cutting/tool-call-grammar-reenable.md`.

## Tool-Call Batches

All calls of one assistant turn are emitted before any result, so a call depends only on
another's side effects. `toolpolicy.EffectFor(name)` classifies each tool:

| Effect | Tools | Batch after a non-fatal failure |
|---|---|---|
| `EffectReadOnly` | `fetch_url`, `internet_search`, `read_file`, `list_directory`, `memory_search`, `get_network_info` | continues |
| `EffectHousekeeping` | `memory_update`, `automation_journal` | stops |
| `EffectMutating` (default: every unlisted and MCP tool) | `write_file`, terminal, `notify_user`, … | stops |

Invalid arguments, a guardrail denial and a run-fatal error always stop the batch. Every early stop
answers the unrun calls with `prompts.ToolCallNotExecuted` (`appendSkippedToolResults`, under `mu`,
bypassing `appendToolResult` so the guardrail streak and the ledger are untouched) — an unanswered
tool_call id was resent on every later request and is rejected by strict APIs. Failures inside one
batch count once toward `toolFailureStreakLimit` (`toolFailureState.beginBatch`); plan steps and
lone `executeSingleToolStep` calls count one by one.

## Tool Call Data Flow

### History Normalization (`NormalizeHistory` in `history.go`)

Before each LLM call, history is normalized based on `useNativeTools`:

**When `useNativeTools=true` (native mode):**
- `ToolRole` messages kept as-is with `Content` = raw JSON result
- `ToolCalls` arrays preserved on assistant messages
- Standard OpenAI-style `tools` + `tool_choice` format

**When `useNativeTools=false` (XML mode):**
- `ToolRole` messages **converted to `UserRole`** with `"Tool result [callID]: ..."` prefix
- `ToolCalls` arrays **emptied** and serialized to XML in `Content`:
  ```
  <tool_call>
  {"tool": "memory_search", "args": {"query": "foo"}}
  </tool_call>
  ```
- Consecutive same-role messages are merged (except messages with `ToolCallID`)
- Empty content is filled with placeholders (`"Thinking..."`, `"Tool result: ..."`)
- `ReasoningContent`, `Name`, and non-standard fields are stripped by `SanitizeHistory`

**Effect on debugging:** A history dump with `useNativeTools=false` looks fundamentally
different from one with `true`. There will be no `ToolRole` messages or `ToolCalls` arrays.
The model sees `UserRole` messages with `"Tool result"` text prefixes instead.

### `useNativeTools` Resolution (Three-State `*bool`)

`AgentOptions.UseNativeTools` is a `*bool` — nil, `&false`, or `&true`:

| Value | Behaviour |
|-------|-----------|
| `nil` (unset) | Use `provider.UseNativeTools()` — `LocalToolRegistry` returns `false`, `MockNodeHerder` returns `true` |
| `&true` | Force native mode — `tool_choice: "required"` in automation, `NormalizeHistory` keeps ToolRole |
| `&false` | Force XML mode — `tool_choice` not set, `NormalizeHistory` converts ToolRole → UserRole |

**Important:** `&false` is NOT the same as `nil`. Setting `UseNativeTools: &false`
silently overrides the provider's `UseNativeTools()` return value. This changed the
tool role format in the assistant handler and broke the agnostic flow test.

### `appendToolResult` Double-JSON-Marshaling

In `processToolCalls`, the tool result is marshaled to JSON TWICE:
1. In the caller (`processToolCalls` lines 159-170) to create `finalResult`
2. In `appendToolResult` (line 443) via `json.Marshal(result)`

This is wasteful and can produce different output if `result` is a pointer type
modified between the two calls. When adding new tool execution paths, prefer
passing pre-serialized strings to `appendToolResult`: `string(json.Marshal(result))`.

```go
// Correct — single marshal in caller:
raw, _ := json.Marshal(result)
a.appendToolResult(history, tc, string(raw))
```
