# Plan: GBNF Grammar Constraints for Tool Calls

**Status:** proposed
**Date:** 2026-06-09
**Related specs:** SPEC-001, SPEC-003

## Problem

The model occasionally generates malformed JSON in tool call arguments
(e.g., `query: {}` where a string is expected, or mismatched brackets).
This causes JSON deserialization errors that waste a turn or crash the tool.

## Root Cause

llama.cpp generates free-form text. The tool manifest schema is only a
guideline for the model — there's no enforcement at the token level.
The model must "guess" the correct JSON structure, and small models
(Gemma 4 4B) frequently guess wrong.

## Solution: GBNF Grammar

llama.cpp supports GBNF (Grammar-Based Neural Format) — a BNF-like
grammar language that constrains which tokens the model can output.
If we generate a GBNF grammar from the tool call JSON schema and pass
it alongside `tool_choice`, the server physically prevents the model
from generating invalid JSON.

## Implementation

### Backend

**1. Add `Grammar` field to `ChatRequest`**

```go
type ChatRequest struct {
    ...
    Grammar *string `json:"grammar,omitempty"` // GBNF grammar string
}
```

The `omitempty` ensures it's only sent when present, so cloud providers
that don't support grammars are unaffected.

**2. GBNF generator** — new file `internal/core/proxy/tool_grammar.go`

Converts a tool manifest's JSON schema parameters into llama.cpp GBNF
format. Example output for `memory_search`:

```
root ::= "{"
  pair ("," pair)*
"}"

pair  ::= query-field | tags-field | limit-field
query-field ::= "\"query\":" string-value
tags-field  ::= "\"tags\":" "[" string-value ("," string-value)* "]"
limit-field ::= "\"limit\":" integer-value

string-value ::= "\"" [^"]* "\""
integer-value ::= [0-9]+
```

The generator handles:
- `string` → `string-value` rule
- `integer` → `integer-value` rule
- `array` → `[value ("," value)*]` rule
- `optional` fields → wrapped in `(...)?`
- `enum` values → explicit alternation

**3. Wire in `stream.go` `buildChatRequest`**

When `tool_choice` is `"required"` and the provider supports grammars
(checked via a `SupportsGrammars` capability flag), generate the GBNF
for the requested tool and attach it to `req.Grammar`.

**4. Provider capability flag**

Add a `SupportsGrammars` field to provider config. Only set to true
for local llama.cpp providers (not OpenAI/Gemini/Vertex etc.).

**5. Proxy passthrough**

The `grammar` field is already accepted by llama.cpp's
`/v1/chat/completions` endpoint natively — no proxy-side translation
needed beyond passing the field through.

### Files to change

| File | Change |
|------|--------|
| `models/llm_messages.go` | Add `Grammar *string` to `ChatRequest` |
| `models/config.go` | Add `SupportsGrammars bool` to provider config |
| `internal/core/proxy/tool_grammar.go` | New file: GBNF grammar generator |
| `internal/core/proxy/tool_grammar_test.go` | Tests for grammar generation |
| `internal/core/assistant/stream.go` | Wire grammar into `buildChatRequest()` |
| `internal/core/llm/providers/local_provider.go` | Set `SupportsGrammars = true` |

## Risks

- GBNF grammar can slow generation slightly (5-10%) on complex schemas
- Simple tool schemas (our use case — 2-3 fields) have negligible overhead
- Only works with llama.cpp servers (not OpenAI/Gemini API)
- Must be gated behind `SupportsGrammars` flag to avoid breaking cloud providers

## Alternative Considered

Using `interface{}` at tool handler boundaries (the current workaround)
handles the symptom but not the root cause. Grammar constraints are the
proper fix — they prevent bad JSON at the generation layer rather than
tolerating it after the fact.
