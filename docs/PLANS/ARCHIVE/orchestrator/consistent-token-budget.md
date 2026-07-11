---
status: complete
related_specs: [SPEC-005]
---
# Consistent Token Budget Computation

**Status: COMPLETE**

`budget_squeezer.go` implemented with `resolveContextLength()` that follows a strict priority order (Nctx → ContextLength → knownCtx → providerCtxDefaults). `max_tokens = ctxLen / 3` and `reasoning_budget = ctxLen / 8` applied consistently across enrichment and Sync paths.

## Problem

`addModel` API enrichment and `Sync()` startup use different `ctxLen` values:

| Path | `ctxLen` used | `max_tokens` | `budget` |
|---|---|---|---|
| `addModel` enrichment | 8192 (serving ctx overwrites training ctx) | 2730 | 10924 |
| `Sync()` startup (no metadata) | 128000 (provider default) | 42666 | 170668 |

The computed UI value is never persisted unless the user manually clicks Save.

## Root Cause

Two bugs, same root: `Nctx` (serving context from llama.cpp `/slots`) was either
overwriting or being ignored by `ContextLength` (training context from GGUF).

### Bug 1 — `registry_handlers.go:241`

`enrichMetadataFromProviders` overwrites `ContextLength` (n_ctx_train) with
`metaCtx` (n_ctx, serving context):

```go
r.Metadata.ContextLength = metaCtx  // 8192 overwrites 131072
```

### Bug 2 — `budget_squeezer.go:resolveContextLength`

`resolveContextLength` only checked `Metadata.ContextLength`. When Bug 1 was
fixed (storing serving context in `Nctx` instead), `resolveContextLength` saw
zero in `ContextLength` and fell through to the provider default (128K) —
**ignoring the detected 8192 serving context** from `/slots`.

This caused `max_tokens = 42666` for a server that only had 8192 total context.
Models that needed more than a few reasoning tokens (Qwen 3.5 4B) would hit the
wall; frugal models (GPT-OSS 20B) survived by luck.

## Fix: Two Changes

### 1. `budget_squeezer.go:resolveContextLength` — Check Nctx first

Add a new priority-1 check for `cfg.Metadata.Nctx` before the existing
`ContextLength` check. The full resolution order is now:

1. **`cfg.Metadata.Nctx`** — serving context from llama.cpp `/slots`
   (discovered by `OpenAICompatibleProvider.fetchSlotsContext`)
2. **`cfg.Metadata.ContextLength`** — training context from GGUF
3. **`knownCtx`** — exceptional models by name fragment (e.g. deepseek-v3)
4. **`providerCtxDefaults`** — per-provider fallback (e.g. openai=128K)
5. **`0`** — unknown (no budget applied)

Both Nctx and ContextLength are capped by `providerCtxDefaults[provider]` to
prevent inflated values (e.g. 262K n_ctx_train capped to 128K for openai).

### 2. `registry_handlers.go:241` — Store in the correct field

```go
// Before:
r.Metadata.ContextLength = metaCtx

// After:
r.Metadata.Nctx = metaCtx
```

## Files Changed

| File | Change |
|---|---|
| `internal/core/orchestrator/budget_squeezer.go` | `resolveContextLength` checks `Nctx` first (priority 1) |
| `internal/transport/http/registry_handlers.go` | `.ContextLength = metaCtx` → `.Nctx = metaCtx` |

## Result

- Models with a `/slots`-detected serving context (e.g. 8192) get
  `max_tokens = Nctx / 3`, `budget = (Nctx - maxTokens) * 2`.
- Models without metadata fall through to the provider default as before.
- The `registry_handlers.go` fix prevents serving-context from overwriting
  training-context in the persistence layer.

No settings.yml changes. The computed values match the actual server capacity.

---

## Postscript: ToolCallFormat Default Removal

### Problem

`ApplyMetadataDefaults` unconditionally set `ToolCallFormat = "native"` when empty
(`budget_squeezer.go:152-154`). This overrode the constitutional default (XML text
mode for local models — see Constitution II.5) and the `tiers.go` default
(`local` → `ToolCallFormat: ""`). Models like Gemma 4 whose llama.cpp chat
template doesn't enforce `tool_choice: "required"` would generate excessive
reasoning without tool calls, triggering the stuck detector.

### Fix

Removed the line `cfg.ToolCallFormat = "native"` from `ApplyMetadataDefaults`.
`ToolCallFormat` is now determined by:

1. `settings.yml model_overrides` (highest priority — user explicitly sets xml/native)
2. Registry entry (from UI add flow, which reflects the provider tier default)
3. `""` (empty) → XML text mode (default for non-native tool calling)

### Result

- Models with `ToolCallFormat = ""` now correctly use XML text mode.
- Models with `ToolCallFormat = "native"` explicitly set (via UI or settings.yml)
  continue using native function calling unchanged.
- Gemma 4 configured with `tool_call_format: xml` in settings.yml avoids the
  reasoning-stuck loop because XML tool calls appear in `content`, so the stuck
  detector condition (`content == 0 && tool_calls == 0`) never matches.
