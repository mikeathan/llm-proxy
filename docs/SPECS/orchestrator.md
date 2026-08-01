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

- `ProviderTiers()` in `assistant/agent.go` defines per-provider baselines. Reasoning is a
  typed `ReasoningSpec` (mode + effort/flag/budget), not a raw int:

  | Provider  | MaxSteps | ContextBudget | MaxTokens | ToolCallFormat | Prefill | Reasoning |
  |-----------|----------|---------------|-----------|----------------|---------|-----------|
  | local     | 25       | 8000          | 2048      | (xml)          | false   | ModeThinkTokens (derived) |
  | gemini    | 35       | 50000         | 4096      | native         | false   | ModeEffort (medium) |
  | vertex    | 35       | 50000         | 4096      | native         | false   | ModeEffort (medium) |
  | openai    | 35       | 50000         | 4096      | native         | false   | ModeEffort (medium) |
  | openrouter| 30       | 30000         | 2048      | native         | false   | ModeObject (medium) |
  | mulerouter| 30       | 30000         | 2048      | native         | false   | ModeEffort (medium) |
  | nvidia    | 30       | 20000         | 2048      | native         | false   | ModeEnableThinking |

- Per-model overrides in `settings.yml` under `model_overrides:` take priority.
- `ApplyMetadataDefaults()` sets `ToolCallFormat` to `"native"` when empty (cloud tiers).

### 4. Reasoning Budget

- Reasoning wire params are resolved per provider by `ReasoningSpec` +
  `ReasoningParamResolver` (strategy pattern in `assistant/reasoning_param.go`).
  Only the provider-appropriate field is serialized on the ChatRequest:
  - `thinking_budget_tokens` — llama.cpp/local (ModeThinkTokens).
  - `reasoning_effort` — openai/gemini/vertex/mulerouter (ModeEffort).
  - `reasoning` object (`effort`) — openrouter (ModeObject).
  - `chat_template_kwargs.enable_thinking` — nvidia (ModeEnableThinking).
- A local host (via `IsLocalModelURL`) always overrides to `thinking_budget_tokens`,
  preserving the working local path even for cloud-slugged configs.
- Local think-token budget is derived at agent-build time in `resolveReasoningSpec`:
  `DefaultReasoningBudget(maxTokens)` = `max_tokens / 3` when no explicit budget is
  configured. Because `max_tokens` itself derives from serving context (`ctxLen / 3`),
  the budget tracks the launched context. Explicit per-model `reasoning_budget` overrides
  the derived value. No model-name heuristics.
- The legacy `streamReasoningBudgetDivisor` name-gate and `ProviderTuningDefaults.ReasoningField`
  were removed; `budget_squeezer.go` operates on `ReasoningSpec.Budget` (ModeThinkTokens only).

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
