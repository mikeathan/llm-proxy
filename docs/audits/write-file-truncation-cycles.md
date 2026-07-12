---
status: reference
last_reviewed: 2026-07-11
---

# write_file Truncation Cycles & Block Editing

## Root Cause

The `write_file` tool had `"maxLength": 800` in the `filesystem.json` manifest. OpenAI-compatible
servers (llama.cpp, vLLM) enforce `maxLength` as a **grammar constraint**, truncating the content
string at exactly 800 characters and then continuing to serialize the next JSON key. This creates a
structurally valid JSON with silently truncated content. The model writes what it thinks is a
complete file, reads it back, sees truncated code, and enters an infinite rewrite loop.

### Timeline

1. **Handler enforcement** (f4f3e2d1): Added `maxWriteFileContent = 800` check in `WriteFile`
   that returned an error when content exceeded 800 chars. This caused stuck loops where the
   model retried `write_file` instead of switching to `append_file`.

2. **Partial write on overflow** (a1b2c3d4): Changed to write first 800 chars + return warning
   with `append_file` suggestion. Split at line boundaries via `splitContent`. Still caused
   loops — the model didn't understand how to use `append_file` to continue.

3. **Removed handler enforcement** (e5f6g7h8): Handler writes all content without checking
   length. But the manifest `maxLength: 800` was still present, so the server-side grammar
   engine silently truncated content at 800 chars. Cycle continued.

4. **Removed manifest maxLength** (i9j0k1l2): `maxLength` removed entirely from both
   `write_file.content` and `append_file.content`. The model's own `max_tokens` is now the
   only output cap. Server-side grammar truncation eliminated.

### The EditFileBlock Tool

Added `edit_file_block` tool (`filesystem.go`, `filesystem.json`) to replace exact blocks of
text in existing files without rewriting the entire file. Uses whitespace normalization
(trim trailing spaces per line, normalize `\r\n`) so the model doesn't need to match
indentation precisely.

**Normalization scope:** trailing whitespace per line, carriage-return stripping only.
Leading whitespace/indentation is NOT normalized — it's semantically meaningful in code.

**Key design decisions:**
- No `maxLength` on `edit_file_block` parameters to avoid grammar truncation
- Error on empty `old_block`
- Error on multiple matches (with instructions to add context)
- Error on no match (with instructions to verify content)
- Empty `new_block` allowed (deletes the matched block)
- File size quota checked on resulting content

## Affected Code

| File | Change |
|------|--------|
| `filesystem.go` | Removed `maxWriteFileContent`, `splitContent`. Added `EditFileBlock`, `normalizeBlock`, `buildReplacement` |
| `filesystem.json` | Removed `maxLength` from `write_file` and `append_file`. Added `edit_file_block` manifest |
| `models/tools.go` | Added `ToolEditFileBlock = "edit_file_block"` |
| `registry.go` | Registered `edit_file_block` handler |
| `templates.go` | Updated `AutomationContentTooLongPrompt` (removed "800 chars" reference) |
| `CLAUDE.md` | Updated tool descriptions |
| `AGENTS.md` | Updated pitfall #21 |

## Early Reasoning Stuck Detection

Models with `reasoningBudget == 0` (no server-side thinking enforcement, e.g. local GGUF
models) sometimes generate pure reasoning content without text or tool calls — a sign of being
stuck. Previously the stuck detector waited for the full `maxTokens * 2` threshold (4096+ chars,
~30s) regardless of model type.

Changed `checkStreamStuck` in `stream.go` to add an early check: when `reasoningBudget == 0`,
stuck fires at `maxTokens / stuckNonReasoningDivisor` chars instead of the full threshold.

### Divisor Tuning History

- **Divisor=2 (original fix):** Threshold = `maxTokens / 2` (e.g. 1024 chars for 2048-token
  models). Was meant to catch stuck GPT-OSS-20B faster. Claimed "false positive is impossible"
  — but this was wrong. Gemma 4 (a local GGUF model) produces ~1371 chars of legitimate
  `<think>` reasoning before generating output. The content/tool call guard at the top of
  `checkStreamStuck` doesn't help because the output hasn't arrived yet — it comes *after* the
  thinking block. Result: **false stuck detection on Gemma 4**.
- **Divisor=1 (current fix):** Threshold = `maxTokens` (e.g. 2048 chars for 2048-token models).
  Gives reasoning-capable models (Gemma 4, GPT-OSS-20B when not stuck) enough room to finish
  thinking while still catching stuck models 2x faster than the pre-change `maxTokens * 2`
  baseline. Qwen is unaffected — its `<tool_call>` blocks inside `<think>` are extracted by
  `tryExtractToolCallFromReasoning()` before the stuck check runs.

**Tradeoff:** Divisor=1 catches stuck models at 2048 chars instead of 1024 — ~1s slower — but
eliminates false positives on models that legitimately produce reasoning content without an
explicit `ReasoningBudget`. For models that need even more thinking headroom, set an explicit
`ReasoningBudget` in their config so they use the normal `stuckThreshold()` path entirely.

See `stream.go` `stuckNonReasoningDivisor` constant and `checkStreamStuck()`.

## Memory Search Format — Entry N/M

The search result format used `---` as an entry separator:

```
**Title**\nContent\n---\n**Title2**\nContent2
```

The 20B model sometimes counted `---` separators as entries, leading to inflated
counts (e.g. reporting 5 results for 3 entries). Changed to `Entry N/M` prefix:

```
Entry 1/3 — **Title**\nContent\nEntry 2/3 — **Title2**\nContent2
```

This gives the model an explicit count in every entry. The format works for all
model sizes — no separator parsing needed. See `memory_tools.go` `Search()`.

## Empty Query Returns All Entries

`memory_search` with `query:""` previously returned an error:

```
Empty query. Search for keywords separated by spaces...
```

Models naturally expected `""` to mean "return everything," wasting 3-4 turns
trying empty queries before switching to keyword chaining. Changed empty-query
+ no-tags to return all entries via `store.List()` instead of erroring.

Two new helpers in `memory_tools.go`:
- `listAllMemories(ctx, wsID, scope, limit)` — routes to `store.List` by scope
- `mergeAndCap(a, b, limit)` — merges workspace + user entries, deduplicates by ID

## Status
reference
