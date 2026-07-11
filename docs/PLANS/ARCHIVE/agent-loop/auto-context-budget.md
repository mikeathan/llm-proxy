---
status: complete
related_specs: [SPEC-001]
---
# Auto-Compute context_budget (and prefill) for Local LLMs

**Status: COMPLETE**

`resolveContextLength` in `budget_squeezer.go` auto-computes context window from model metadata (GGUF scan) with fallback chain. `max_tokens` and `reasoning_budget` derived from actual `ctxLen`.

## Goal

Auto-calculate `context_budget` (and optionally `prefill`) for local GGUF models
using the model's true context window, removing the need for manual per-model
tuning of these fields.  Computed values are persisted the same way as manual
overrides today (settings.yml `model_overrides`), so they survive restarts and
can be overridden by an explicit user setting.

---

## 1.  Data Model — Persist ContextLength in registry.json

### Problem

`ModelRegistryEntry` (models/registry.go:20-29) has **no** field for the
model's context length.  The GGUF scan produces `ContextLength` at runtime
but it is silently dropped when converting `ModelConfig` → `ModelRegistryEntry`
in `PersistModel`/`PersistReplaceModel` (app_context.go).  On restart, no
metadata is available.

### Change

Add a field to `ModelRegistryEntry`:

```go
type ModelRegistryEntry struct {
    // …existing fields…
    Prefill      bool     `json:"prefill,omitempty"`

    // Persisted metadata for auto-computation.
    ContextLength int    `json:"context_length,omitempty"`   // n_ctx from GGUF or --ctx-size
    Parameters    int64  `json:"parameters,omitempty"`       // model param count (for prefill heuristic)
}
```

**Files to modify:**

| File | What |
|------|------|
| `models/registry.go` | Add `ContextLength int` and `Parameters int64` to `ModelRegistryEntry` |
| `internal/core/llm/manager.go:114-126` | After loading from registry catalogue, populate `ModelConfig.Metadata` from the new fields: `Metadata = &ModelMetadata{ContextLength: entry.ContextLength, Parameters: entry.Parameters}` |
| `internal/app/app_context.go` (PersistModel / PersistReplaceModel) | Copy `mc.Metadata.ContextLength` and `mc.Metadata.Parameters` into the registry entry before writing |
| `models/registry.go` entry -> cfg mapping (manager.go) | Also needs to be done for the _read_ path in `NewManagerFromRegistry` |

---

## 2.  Determine the Effective Context Window

Two possible sources, with precedence:

| Source | Priority | Where to read |
|--------|----------|---------------|
| `--ctx-size N` in model `Args` | Highest | Parse `Args` for `--ctx-size` or `-c` flag |
| GGUF header `MaximumContextLength` | Fallback | From `ModelMetadata.ContextLength` (scanned at add time or page load) |

### Parsing `--ctx-size` from Args

The args list typically looks like:

```
["--ctx-size", "8196", "--parallel", "1", "-ngl", "99", ...]
```

Write a helper:

```go
// ctxSizeFromArgs extracts the effective context size from model launch args.
// Returns 0 if not specified.
func ctxSizeFromArgs(args []string) int {
    for i, a := range args {
        if a == "--ctx-size" || a == "-c" {
            if i+1 < len(args) {
                if n, err := strconv.Atoi(args[i+1]); err == nil {
                    return n
                }
            }
        }
    }
    return 0
}
```

Location: new file `models/model_args.go` or inline in the auto-compute module.

**Effective Ctx = `ctxSizeFromArgs(args)` or `metadata.ContextLength` or 0.**

---

## 3.  Computation Logic

### context_budget Formula

```go
// ComputeContextBudget derives a safe character-based sieve threshold
// from the model's token context window.
func ComputeContextBudget(effectiveCtx int) int {
    if effectiveCtx <= 0 {
        return 0 // caller falls back to DefaultContextBudget (15000)
    }

    // Estimated chars per token for code/JSON content: ~3.5
    // Safety margin: reserve 20% of context for model response + overhead
    const charsPerToken = 3.5
    const safetyFactor = 0.80
    budget := int(float64(effectiveCtx) * charsPerToken * safetyFactor)

    // Floor — a budget below 6000 chars means the sieve fires before
    // any meaningful work can happen. Also cap so we never exceed what
    // the physical sieve can reasonably handle.
    const minBudget = 6000
    const maxBudget = 120000 // 32k ctx * 3.5 ≈ 112000 chars
    if budget < minBudget {
        budget = minBudget
    }
    if budget > maxBudget {
        budget = maxBudget
    }
    return budget
}
```

### prefill Auto-Setting

```go
// ComputePrefill decides whether prefill should be on by default.
// Smaller models (< 7B params) tend to struggle with XML tool call format.
// At or above 7B they usually handle it well enough without prefill.
//
// When tool_call_format is "native", prefill is irrelevant (the agent code
// skips it entirely), but we still compute for consistency — the handler
// can ignore it when native is set.
func ComputePrefill(params int64, toolCallFormat string) bool {
    if toolCallFormat == "native" {
        return false // native tools don't use prefill
    }
    if params > 0 && params < 7_000_000_000 {
        return true // < 7B: prefill helps
    }
    return false // >= 7B or unknown: no prefill
}
```

---

## 4.  Where the Computation Runs

### 4a.  Handler — On Model Add / Update

`registry_handlers.go` — in both `handleAddModel` and `handleUpdateModel`,
after the GGUF scan already happens (when building `runtimeCfg`), add a new
block that:

1. Determines the effective context window:
   - Parse `runtimeCfg.Args` for `--ctx-size` — if found, use it
   - Else use `runtimeCfg.Metadata.ContextLength` — if non-zero, use it
   - Else skip auto-computation (rely on defaults)

2. Compute context_budget:
   ```go
   computedBudget := ComputeContextBudget(effectiveCtx)
   ```
   Only write it if `req.ContextBudget == 0` (user didn't set it explicitly).

3. Compute prefill:
   ```go
   computedPrefill := ComputePrefill(runtimeCfg.Metadata.Parameters, req.ToolCallFormat)
   ```
   Only if the user didn't explicitly set `req.Prefill` (need a nullable bool for "not set" — see Section 6).

4. Write the computed values to `settings.yml` model_overrides (same block at
   lines 278-291 / 416-429).

**Pseudo-code for the auto-compute block (placed after building `runtimeCfg`, before calling `PersistModel`):**

```go
// Auto-compute context_budget and prefill for local models when not user-overridden.
if req.Provider == "local" {
    effectiveCtx := ctxSizeFromArgs(runtimeCfg.Args)
    if effectiveCtx == 0 && runtimeCfg.Metadata != nil {
        effectiveCtx = runtimeCfg.Metadata.ContextLength
    }
    if effectiveCtx > 0 {
        computedBudget := ComputeContextBudget(effectiveCtx)
        if req.ContextBudget == 0 {
            runtimeCfg.ContextBudget = computedBudget
            persistCfg.ContextBudget = computedBudget
        }
        if !req.PrefillSet { // user did not explicitly set prefill
            computedPrefill := ComputePrefill(runtimeCfg.Metadata.Parameters, req.ToolCallFormat)
            runtimeCfg.Prefill = computedPrefill
            persistCfg.Prefill = computedPrefill
        }
    }
}
```

### 4b.  Manager — On Restart (Fallback)

When loading from `registry.json` on startup, the manager may have the
`ContextLength` from the persisted field (Section 1).  Add a fallback in
`NewManagerFromRegistry`:

After calling `ApplyModelOverrides(settings.ModelOverrides)`, iterate the
model map.  For any model where `ContextBudget == 0` and `ContextLength > 0`,
auto-compute:

```go
// After ApplyModelOverrides — fill in computed defaults for local models.
for name, cfg := range m.models {
    if cfg.ContextBudget > 0 || cfg.Provider != "local" {
        continue
    }
    effectiveCtx := ctxSizeFromArgs(cfg.Args)
    if effectiveCtx == 0 && cfg.Metadata != nil {
        effectiveCtx = cfg.Metadata.ContextLength
    }
    if effectiveCtx > 0 {
        cfg.ContextBudget = ComputeContextBudget(effectiveCtx)
        if cfg.Prefill == false {
            params := int64(0)
            if cfg.Metadata != nil {
                params = cfg.Metadata.Parameters
            }
            cfg.Prefill = ComputePrefill(params, cfg.ToolCallFormat)
        }
        m.models[name] = cfg
    }
}
```

This ensures models added by a different process (or in an older version
before auto-computation) get a reasonable budget on the next restart.

---

## 5.  Settings.yml Interaction

Current behavior: the handler writes to `settings.yml` `model_overrides` only
when at least one override field is non-zero:

```go
if req.MaxSteps > 0 || req.ContextBudget > 0 || req.ToolCallFormat != "" || req.Prefill {
    // write override entry
}
```

With auto-computation, computed values ARE written to settings.yml the same
way.  This means:

- **First add**: handler computes, writes to settings.yml → survives restart.
- **User changes `--ctx-size` via update**: handler recomputes, overwrites.
- **User manually sets `context_budget` in settings.yml**: the set condition
  (`req.ContextBudget == 0`) prevents auto-computation from overriding it.
- **User removes the override from settings.yml**: on restart, manager
  fallback (4b) re-computes from `registry.json` context_length.

This also means the Display Bug in `admin_view.go:55-58` (where
`PersistReplaceModel` silently drops metadata) must be fixed (covered in
Section 1).

---

## 6.  Nullable Prefill for "Not Set" Detection

Today `Prefill bool` can only be `true` or `false` — there is no way to
distinguish "user explicitly set false" from "user didn't set it."  For
auto-computation we need a third state.

### Option A — `*bool` in the handler request

In `handleAddModel` and `handleUpdateModel` request structs, change:

```go
Prefill bool `json:"prefill"`
```

to:

```go
Prefill *bool `json:"prefill"`
```

Then check `req.Prefill == nil` before auto-computing.  If non-nil, use the
user's value.  This is a **backward-compatible** JSON change — `false` and
`null` both decode as `nil` in JSON, but Go distinguishes `*bool` nil from
`*bool` pointing to `false`.  However, the frontend sends `false` as a
boolean which unmarshals to a `*bool` pointing to `false`, not `nil`.
So the frontend must also be updated to either omit the field or send `null`.
Alternatively, the frontend can use `"prefill": null` when the user hasn't
touched it.

### Option B — Separate `prefill_set` flag

Add a separate `prefill_set bool` to the request struct.  The frontend sets
it to `true` only when the user has explicitly toggled prefill.

### Recommendation: Option A with frontend opt-in

The API already uses JSON.  Wire up the frontend to send `"prefill": null`
(omitting the field) for "not set."  The Go handler checks `req.Prefill == nil`.

**Files to modify:**

| File | What |
|------|------|
| `internal/transport/http/registry_handlers.go` | Change `Prefill bool` to `Prefill *bool` in both add and update request structs. Adjust `req.PrefillSet` logic. |
| `internal/transport/http/admin_handlers.go` | Possibly add `prefill` to `adminModelView` if not already there |
| `frontend/src/...` | Send `null` when prefill is not user-configured |

**Alternative (simpler):** Skip the nullability issue entirely.  Use the same
approach as `ContextBudget` — if `Prefill == false`, auto-compute.  The user
can only set prefill to `true` to override.  This means `prefill: false`
cannot be manually set, but that's acceptable because:
- If the model is ≥7B and auto-compute says `false`, the user doesn't need to
  change it.
- If the user wants prefill off for some other reason, they can set
  `tool_call_format: native` (which disables prefill anyway).
- Users rarely override prefill to `false` for small models.

**Decision for v1: Skip nullability.  Treat `prefill: false` as "auto" for
local models.**

---

## 7.  Files to Modify — Summary

### Models Layer

| File | Change |
|------|--------|
| `models/registry.go` | Add `ContextLength int` and `Parameters int64` to `ModelRegistryEntry` |
| `models/model_args.go` | NEW — helper `ctxSizeFromArgs` and `ComputeContextBudget`, `ComputePrefill` |

### Core / Runtime

| File | Change |
|------|--------|
| `internal/core/llm/manager.go:114-126` | Populate `ModelConfig.Metadata` from `ModelRegistryEntry.ContextLength` / `.Parameters` on registry load |
| `internal/core/llm/manager.go` (after `ApplyModelOverrides`) | Add fallback auto-computation loop for local models with `ContextBudget == 0` |

### Transport / API

| File | Change |
|------|--------|
| `internal/transport/http/registry_handlers.go` | In `handleAddModel` and `handleUpdateModel`: insert auto-compute block after `runtimeCfg` is built, before `PersistModel`/`UpdateSettings` |
| `internal/transport/http/registry_handlers.go` | Optionally change `Prefill bool` to `*bool` in request structs |

### App Context (Persistence)

| File | Change |
|------|--------|
| `internal/app/app_context.go` | `PersistModel` and `PersistReplaceModel`: copy `mc.Metadata.ContextLength` and `mc.Metadata.Parameters` into the registry entry |

### Frontend (Optional, for nullable prefill)

| File | Change |
|------|--------|
| `frontend/src/...` model-edit component | Send `"prefill": null` when unset (if using *bool) |

---

## 8.  Test Plan

| Scenario | Expectation |
|----------|-------------|
| Add local model with no args, GGUF n_ctx=8192 | context_budget ≈ 22937, prefill=false (>= 7B) or prefill=true (< 7B) |
| Add local model with `--ctx-size 4096` | context_budget ≈ 11468, uses args value not GGUF |
| Add local model with user-set context_budget=30000 | User's value preserved, auto-compute skipped |
| Add local model with user-set prefill=true | User's value preserved, auto-compute skipped |
| Add cloud model (provider != "local") | No auto-computation applied |
| Restart after auto-computed values were persisted | Values loaded from settings.yml via ApplyModelOverrides |
| Restart with settings.yml override removed | Fallback in manager re-computes from registry.json |
| User updates args to change `--ctx-size` | Handler recomputes on update |
| model has n_ctx=0 (scan failed) | Fallback to DefaultContextBudget=15000 |
| Parameters unknown (params=0), tool_call_format native | prefill=false, context_budget computed from ctx only |

---

## 9.  Edge Cases

- **GGUF scan failed at add time**: `ContextLength` is 0, `Parameters` is 0.
  Handler skips auto-computation.  Manager fallback on restart won't have data
  either.  User must set manually.

- **Args contain `-c` instead of `--ctx-size`**: `ctxSizeFromArgs` handles both
  flags.

- **Args contain `--ctx-size 0`**: Parsed as 0, ignored, falls through to GGUF
  metadata.

- **Multiple `--ctx-size` flags**: The last one wins (or first — either is fine
  as this is a user error; document that it takes the first).

- **User sets `tool_call_format: native`**: `ComputePrefill` returns false
  because native skips prefill anyway.  `context_budget` is still computed
  the same way (the sieve is format-independent).

- **Existing models already in registry.json without context_length field**:
  The field is `omitempty`, so existing entries load with `ContextLength = 0`.
  The manager fallback has no data to compute from — user must either
  re-add the model or set the values manually.  This is acceptable for a new
  feature; existing models remain on defaults.
