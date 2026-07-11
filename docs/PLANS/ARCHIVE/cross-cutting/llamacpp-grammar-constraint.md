---
status: complete
date: 2026-06-12
related_specs: [SPEC-001, SPEC-003]
supersedes: llamacpp-grammar-constraint.md (2026-06-09 v1)
---

# Plan: Provider-Agnostic Output Constraints for Tool Calls

**Status:** complete
**Date:** 2026-06-12
**Related specs:** SPEC-001 (Agent Loop), SPEC-003 (Discovery Panel)

## Problem

Small local models (Gemma 4 4B, GPT-OSS-20B) frequently emit structurally broken
JSON in tool call arguments — mismatched brackets, wrong types, truncated objects.
Each broken call wastes a full turn (~5-10s) and degrades automation reliability.

Existing defences in `tool_call_parser.go` (`sanitizeJSON`, Python-boolean
replacement, trailing-commentary stripping) handle surface-level issues but cannot
fix structurally invalid JSON.

## Root Cause

Local inference engines (llama.cpp, etc.) generate free-form text. The tool
manifest JSON schema embedded in the system prompt is only a guideline — there is
no enforcement at the token level. The model must "guess" the correct JSON
structure, and small models lack the structural reasoning capacity to do so
reliably.

For cloud providers (OpenAI, Gemini) with native tool calling, this specific
problem doesn't exist — arguments arrive as structured API JSON.

## Solution: Provider-Agnostic Output Constraint System

Define a `RequestConstraint` interface — each provider registers an
implementation that modifies the `ChatRequest` with its server-specific
constraint fields. The agent selects the right implementation based on
provider type at `buildChatRequest` time.

### Provider Support Matrix

| Provider | Constraint Mechanism | ChatRequest Field | Prevents | Value |
|---|---|---|---|---|
| **llama.cpp (local)** | GBNF grammar | `grammar` | Invalid JSON in tool args | **High** — token-level enforcement |
| **OpenAI** | Structured Outputs | `response_format` (json_schema) | Malformed args | **Medium** — native tools already validate |
| **OpenRouter** | Pass-through to upstream | `response_format` (json_schema) | Depends on upstream | Medium |
| **NVIDIA** | OpenAI-compatible | `response_format` (json_schema) | Malformed args | Medium |
| **MuleRouter** | OpenAI-compatible | `response_format` (json_schema) | Malformed args | Medium |
| **Gemini** | Response schema | `response_format` (json_schema) | Malformed text | Low |
| **Vertex AI** | Native REST API | `response_format` (json_schema) | Malformed text | Low |
| **TGI** | JSON Schema grammar | `grammar` | Invalid JSON in args | High (when provider type exists) |
| **vLLM** | Guided JSON / GBNF | `guided_json` / `guided_grammar` | Invalid JSON in args | High (when provider type exists) |

**When constraint applies:** Constraints are only applied in **native tool mode**
(`useNativeTools = true`, `tool_choice: "required"`) because this is when the
model outputs structured tool calls (no free-text wrapper). In XML text mode,
the model generates free text with embedded `<tool_call>` blocks — constraining
the full output would block necessary "Thought:" prefixes. The existing XML
parser handles this path.

## Implementation

### Phase 1: Add Constraint Fields to ChatRequest

**File:** `models/llm_messages.go`

```go
type ChatRequest struct {
    // ... existing fields ...
    ResponseFormat *ResponseFormat `json:"response_format,omitempty"`
    Grammar        *string         `json:"grammar,omitempty"`          // llama.cpp / TGI: GBNF grammar string
    GuidedJSON     *string         `json:"guided_json,omitempty"`      // vLLM: JSON schema for guided decoding
    GuidedGrammar  *string         `json:"guided_grammar,omitempty"`   // vLLM: GBNF grammar alternative
}
```

`omitempty` ensures all constraint fields are absent by default — existing
cloud APIs are unaffected until a constraint is explicitly applied.

### Phase 2: Define RequestConstraint Interface

**New file:** `internal/core/proxy/tool_constraint.go`

```go
// RequestConstraint applies a provider-specific output constraint to a
// ChatRequest to prevent malformed tool call output at the generation layer.
type RequestConstraint interface {
    Apply(req *ChatRequest, tools []Tool) bool
}
```

### Phase 3: Implement GBNFConstraint (llama.cpp Local)

**Also in `internal/core/proxy/tool_constraint.go`**

Converts tool manifest JSON schema parameters into llama.cpp GBNF format.

#### Schema Type → GBNF Rule Mapping

The generator walks a `map[string]any` in OpenAPI-schema shape — the exact
format returned by `LoadManifestAsTool()` and stored in `FunctionSchema.Parameters`:

| Manifest `type` | GBNF Rule |
|---|---|
| `"string"` | `string-lit ::= "\"" [^"]* "\""` |
| `"integer"` | `int-lit ::= [0-9]+` |
| `"number"` | `num-lit ::= [0-9]+ "."? [0-9]*` |
| `"boolean"` | `bool-lit ::= "true" \| "false"` |
| `"array"` + `items: {type: "string"}` | `arr-lit ::= "[" string-lit ("," string-lit)* "]"` |
| `"array"` + `items: {type: "integer"}` | `arr-lit ::= "[" int-lit ("," int-lit)* "]"` |
| `{"enum": ["a","b"]}` | `enum-lit ::= "\"a\"" \| "\"b\""` |
| **Unsupported / missing type** | Return false — caller skips constraint |

#### Generated Grammar Example

For `memory_search` (fields: `query` string optional, `limit` integer optional,
`tags` array optional, `scope` enum optional):

```gbnf
root ::= "{" (pair ("," pair)*)? "}"
pair ::= query-field | tags-field | limit-field | scope-field

query-field ::= "\"query\":" string-lit
tags-field  ::= "\"tags\":" "[" string-lit ("," string-lit)* "]"
limit-field ::= "\"limit\":" int-lit
scope-field ::= "\"scope\":" scope-enum

scope-enum  ::= "\"user\"" | "\"workspace\""
string-lit  ::= "\"" [^"]* "\""
int-lit     ::= [0-9]+
```

**Required-field handling:** The grammar does NOT enforce required fields.
Missing required fields trigger the existing `system_error` tool recovery.
GBNF prevents structural errors (invalid JSON) only — the parser handles
semantic validation.

#### Multi-Tool Disjunctive Grammar

When multiple tools are available, the grammar covers all of them via an
alternation at the top level (`root ::= tool1-schema | tool2-schema | ...`).

#### Special Cases

| Case | Behaviour |
|---|---|
| **Empty schema** (no parameters) | Return false — no constraint needed |
| **Unsupported type** (e.g. `"null"`) | Return false — let natural parsing handle it |
| **Nested objects** | Recurse, cap at 3 levels |

### Phase 4: Implement ResponseFormatConstraint (Cloud Providers)

**Also in `internal/core/proxy/tool_constraint.go`** (planned but not v1)

Handles OpenAI-compatible providers (OpenAI, OpenRouter, NVIDIA, MuleRouter)
and Gemini (via its OpenAI-compatible proxy endpoint). Wraps tool argument
schemas into a `json_schema`-type `response_format`:

```go
type ResponseFormatConstraint struct{}

func (c *ResponseFormatConstraint) Apply(req *ChatRequest, tools []Tool) bool {
    if len(tools) == 0 { return false }
    schema := buildOneOfSchema(tools)
    req.ResponseFormat = &ResponseFormat{
        Type: "json_schema",
        JSONSchema: map[string]interface{}{
            "name":   "tool_call",
            "strict": true,
            "schema": schema,
        },
    }
    return true
}
```

**VM: v1 ships GBNFConstraint only.** ResponseFormatConstraint is designed
but not implemented — cloud providers' native tool APIs already enforce
argument validity. Implement when a cloud provider is used in XML mode or
when TGI/vLLM provider types are added.

### Phase 5: Provider Constraint Registration

**File:** `internal/core/assistant/stream.go`

```go
var providerConstraints = map[string]proxy.RequestConstraint{
    "local": &proxy.GBNFConstraint{},
}
```

Only local providers get GBNF grammar in v1. Cloud providers do not need it
because their native tool APIs already return structured, valid JSON.

### Phase 6: Wire in buildChatRequest

**File:** `internal/core/assistant/stream.go`

```go
func (a *Agent) buildChatRequest(prepared []proxy.Message, llmTools []proxy.Tool, isAutomationCtx bool) proxy.ChatRequest {
    req := proxy.ChatRequest{...}
    // ... existing tool_choice, temperature, reasoningBudget setup ...

    // Apply output constraint when native tools are active.
    // Local providers get GBNF grammar to prevent invalid JSON at
    // the token generation level.  Cloud providers are skipped —
    // their native tool API already returns structured valid JSON.
    if a.useNativeTools && len(llmTools) > 0 {
        if constraint, ok := providerConstraints[a.providerType]; ok {
            constraint.Apply(&req, llmTools)
        }
    }
    return req
}
```

The `providerConstraints` map and `Agent.providerType` (already existed at
`agent.go:102`) enable constraint selection.

## Files Changed

| File | Change | Est. Lines |
|---|---|---|
| `models/llm_messages.go` | Add `Grammar`, `GuidedJSON`, `GuidedGrammar` to `ChatRequest` | 3 |
| `internal/core/proxy/tool_constraint.go` | **New:** `RequestConstraint` interface + `GBNFConstraint` + full GBNF generator | ~170 |
| `internal/core/proxy/tool_constraint_test.go` | **New:** 10 tests covering all paths | ~200 |
| `internal/core/assistant/stream.go` | Add `providerConstraints` map + wire in `buildChatRequest()` | 15 |
| `docs/PLANS/cross-cutting/llamacpp-grammar-constraint.md` | This plan | — |

## Test Strategy

### Unit Tests (`tool_constraint_test.go`)

| Test | Covers |
|---|---|
| `TestGBNFConstraint_EmptyTools` | Nil/empty tools → false, Grammar stays nil |
| `TestGBNFConstraint_NoSchema` | Tool without parameters → false |
| `TestGBNFConstraint_StringParam` | String parameter → correct GBNF |
| `TestGBNFConstraint_AllTypes` | string, integer, boolean, array, enum → all present in grammar |
| `TestGBNFConstraint_MultiTool` | Multiple tools → disjunctive `\|` in root |
| `TestGBNFConstraint_OptionalField` | Required + optional mix → correct |
| `TestGBNFConstraint_AllOptional` | All fields optional → no leading comma bug |
| `TestGBNFConstraint_ApplyIdempotent` | Same tools → identical grammar |
| `TestClientChat_GrammarFieldInBody` | Grammar appears in serialized JSON |
| `TestClientChat_GrammarFieldNotSet` | Grammar absent when not set |

## Serialization Architecture

```
Agent.buildChatRequest()
  → ChatRequest{ Grammar: &str, GuidedJSON: nil, ... }
    → json.Marshal(req)
      → HTTP payload:
        {
          "model": "...",
          "messages": [...],
          "tools": [...],
          "tool_choice": "required",
          "grammar": "root ::= ..."    // only set for local llama.cpp
        }
          → provider endpoint
            → llama.cpp: enforces grammar
            → OpenAI/Gemini: ignores unknown field (grammar absent due to omitempty)
```

`LLMClient` in `client.go` does no request transformation — just sends
`json.Marshal(req)`. Fields set on the struct appear in the payload;
providers ignore unrecognized fields.

## Constraint Selection Flow

```
buildChatRequest()
  → a.useNativeTools?             (native mode only)
    → len(llmTools) > 0?           (tools available)
      → isAutomationCtx?           (tool_choice: "required" active)
        → providerConstraints[a.providerType]
          → "local"  → GBNFConstraint  (sets req.Grammar)
          → nil      → unregistered    (no constraint)
```

For cloud providers (openai, gemini, vertex, etc.), no constraint is
registered in v1 — their native tool APIs already return valid JSON.

## Risks & Mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| **5-10% generation slowdown** | Higher first-token latency | 2-3 field schemas have negligible overhead. Only applied in automation context. |
| **Constraint too restrictive** | Model can't express valid variations | `string-lit` allows any non-quote chars. Optional fields use `(...)?`. |
| **Schema mismatch** | Constrains wrong shape | Generator reads the exact `Parameters` map from tool registration. Single source of truth. |
| **Parse error in Apply** | Returns false → no constraint → existing behaviour | Never fatal. Constraint is optional enhancement. |
| **Server rejects unknown field** | 400 error | `omitempty` on all constraint fields. Only set when local provider. |

## Interaction with XML Text Mode for Local Models

In automation, `buildAgentOptions` sets `opts.UseNativeTools = true` when
`ToolCallFormat` is `"native"` or `""` (which `ApplyMetadataDefaults` fills
as `"native"`). **Local models in automation use native tools by default**
via this opts override, overriding `LocalToolRegistry.UseNativeTools()`
(which returns `false`). See `agent.go` `NewAgent()` lines 244-247.

GBNF constraint applies in this native tool path — it prevents malformed
JSON at the token level, reducing fallbacks to slower XML mode.

When `useNativeTools` is `false` or `llmTools` is `nil` (XML text mode),
the constraint is never called. XML parser + `sanitizeJSON()` handle tool
call extraction for those paths.

## Hermes Agent Comparison

Hermes Agent solves tool-call-format errors via a reactive
`ToolCallGuardrailController` (post-hoc validation + correction prompts).
This plan's constraint system is **proactive** (prevent at generation). Both
can coexist: constraints prevent structural errors, guardrails catch semantic
errors (wrong tool for the task, blocked domains, etc.).

## Alternatives Considered

1. **Post-hoc JSON repair** (current state): `sanitizeJSON()` + `interface{}`
   handlers. Catches surface issues but not structural invalidity.

2. **GBNF-only, no abstraction**: Harder to add new backends. The interface
   approach adds minimal abstraction for future extensibility.

3. **Enforcing required fields in GBNF**: Quickly becomes complex with
   multi-required-field alternations. The parser already validates required
   fields via `system_error` tool. GBNF focuses on structural validity.

4. **Per-tool constraint regeneration**: Rebuilding the constraint each time
   the tool list changes. The disjunctive approach covers all tools at once.

5. **XML text-mode constraint**: Constraining the `<tool_call>` wrapper in
   non-native mode. Too restrictive — model needs thought prefixes + XML +
   JSON, exceeding practical grammar expressiveness.

## Future Work (Out of Scope)

1. **Lexicon constraints**: Constraining vocabulary within string fields
   (e.g., only valid file paths, only known tool names). GBNF supports this
   via explicit alternation but requires per-field rules based on domain
   logic, not just schema types.

2. **ResponseFormatConstraint for cloud providers**: Implement when a cloud
   provider is used in XML mode, or when TGI/vLLM provider types are added.

3. **vLLM / TGI provider constraints**: Wire `GuidedJSON` and `GuidedGrammar`
   when these provider types are added. The `RequestConstraint` interface
   is already defined — only implementation + registration needed.
