---
id: SPEC-005
title: Orchestrator / Budget
version: "1.0"
status: stable
last_updated: 2026-05-28
constitution_references: [VI]
related_specs: [SPEC-001]
supersedes:
---

# SPEC: Orchestrator / Budget

## I. Intent

The orchestrator manages token budgets across concurrent agent runs and model inference requests.
It prevents individual runs from consuming disproportionate resources, resolves context lengths
from model metadata, and applies provider-tier tuning defaults.

## II. Functional Requirements

### 1. Budget Management

- `Budget` tracks token consumption per time window.
- Each LLM call deducts from the budget (`Spend()`).
- `Refund()` returns tokens on stream failure.
- Runs that exceed their budget are paused or throttled.

### 2. Context Resolution

- `resolveContextLength()` determines the effective context size for a model:
  1. `Metadata.Nctx` (serving context from llama.cpp `/slots`) — highest priority.
  2. `Metadata.ContextLength` (training context from GGUF) — fallback.
  3. `knownCtx` (model name fragment lookup) — heuristic fallback.
  4. `providerCtxDefaults` (per-provider default) — last resort.
- Forgetting to check `Nctx` first causes `max_tokens` and `reasoning_budget` to exceed
  actual server capacity.

### 3. Provider Tuning Defaults

- `ProviderTiers()` in `assistant/tiers.go` defines per-provider baselines:

  | Provider  | MaxSteps | ContextBudget | MaxTokens | ToolCallFormat | Prefill | ReasoningBudget |
  |-----------|----------|---------------|-----------|----------------|---------|-----------------|
  | local     | 25       | 8000          | 2048      | (xml)          | false   | 0               |
  | gemini    | 35       | 50000         | 4096      | native         | false   | 8192            |
  | vertex    | 35       | 50000         | 4096      | native         | false   | 8192            |
  | openai    | 35       | 50000         | 4096      | native         | false   | 8192            |
  | openrouter| 30       | 30000         | 2048      | native         | false   | 4096            |

- Per-model overrides in `settings.yml` under `model_overrides:` take priority.
- `ApplyMetadataDefaults()` sets `ToolCallFormat` to `"native"` when empty (cloud tiers).

### 4. Reasoning Budget

- `reasoning_budget = max_tokens / 4` (divisor in `streamReasoningBudgetDivisor`).
- Budget is sent on the ChatRequest. Field selection is provider-aware:
  - `reasoning_budget` — OpenAI-compatible field. Sent to cloud providers.
  - `thinking_budget_tokens` — llama.cpp field. Sent to local providers.
- `ProviderTuningDefaults.ReasoningField` declares which field each provider uses.
  Sending unknown fields to cloud providers that reject them (e.g. Nvidia NIM)
  causes HTTP 400 errors, so the agent selects fields per provider type.

### 5. ICU (Inter-Call Underwriting)

- Tracks total token spend per session.
- Refunds on stream failures to prevent double-charging.
- Prerequisites are checked via `doPreflightCheck()` before each LLM call.

## III. Error Handling

- Budget exceeded: return `ErrBudgetExceeded` — caller must pause or fail.
- Context resolution failure: log warning, use provider default.
- Refund on already-refunded transaction: no-op (idempotent).

## IV. Configuration

- `ModelConfig.MaxTokens`, `ModelConfig.ContextBudget` in `registry.json`.
- `model_overrides` in `settings.yml`.
- No global defaults override per-model settings.
