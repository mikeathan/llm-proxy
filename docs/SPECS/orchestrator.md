---
id: SPEC-005
title: Orchestrator / Budget
version: "1.1"
status: stable
last_updated: 2026-08-01
constitution_references: [VI]
related_specs: [SPEC-001]
supersedes:
---

# SPEC: Orchestrator / Budget

## Changelog

- **1.1 (2026-08-01)** — Cloud Provider Token Budgets amendment. The tier table
  now lives in `models/tuning.go` (leaf package) and the per-provider cloud
  output cap is the tier row (8192), not `ctxLen/3`. `max_tokens ← ctxLen/3` is
  **local-only** (LocalBudgetPolicy). Cloud workloads use the clamp-first
  CloudBudgetPolicy with published capabilities (top_provider/context_length)
  and a tier history budget (20K–50K chars). Vertex and mulerouter providers
  are removed; unknown providers fail closed (no local fallback). Workload
  classification (`WorkloadClass` local|cloud) is the single authority for
  budget, ICU, and reasoning-wire selection.


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

- `resolveContextLength()` determines the effective context size for a model.
  Resolution is **workload-scoped**, never provider-label-scoped:

  - **Local workloads** (`WorkloadClass == local`): `LocalBudgetPolicy.resolveContext`
    1. `Metadata.Nctx` (serving context from llama.cpp `/slots`) — highest priority.
    2. `Metadata.ContextLength` (training context from GGUF), capped by `defaultLocalContextMax`.
    3. `defaultLocalContextLength` (8192) — the universal numeric local fallback.
    The local path NEVER consults `providerCtxDefaults` — a local workload can
    never leak into a 128K/1M cloud calculation.
  - **Cloud workloads**: `PublishedContextSource` chain
    1. published `context_length` / `top_provider.context_length` from the live catalog.
    2. model `Metadata` context (n_ctx serving, then n_ctx_train).
    3. `knownCtx` (model name fragment lookup) — heuristic fallback.
    4. provider tier default context (`models/tuning.go`) — last resort.

- Forgetting to check `Nctx` first causes `max_tokens` and `reasoning_budget` to exceed
  actual server capacity.

### 3. Provider Tuning Defaults

- `models/tuning.go` defines per-provider numeric baselines (MaxSteps, history
  ContextBudget, MaxTokens cap, ToolCallFormat, Prefill, DefaultContext). Reasoning is a
  typed `ReasoningSpec` (mode + effort/flag/budget) in `assistant/reasoning_param.go`:

  | Provider  | MaxSteps | HistoryBudget | MaxTokens | ToolCallFormat | Prefill | Reasoning |
  |-----------|----------|---------------|-----------|----------------|---------|-----------|
  | local     | 25       | 8000          | 2048¹     | (xml)          | false   | ModeThinkTokens (derived) |
  | gemini    | 35       | 50000         | 8192      | native         | false   | ModeEffort (medium) |
  | openai    | 35       | 50000         | 8192      | native         | false   | ModeEffort (medium) |
  | openrouter| 30       | 30000         | 8192      | native         | false   | ModeObject (medium) |
  | nvidia    | 30       | 20000         | 8192      | native         | false   | ModeEnableThinking |

- The cloud `MaxTokens` is the **per-tier output cap**, not an allocation. It is the
  clamp target for the output-cap chain (`min(published, tier)`) — a provider known to
  support larger outputs raises its row independently; a published per-model cap still
  wins (is clamped to) the tier row.
- Per-model overrides in `settings.yml` under `model_overrides:` take priority. Explicit
  cloud `max_tokens`/`context_budget` edits that differ from the derived baseline are
  persisted (Phase 3); local workloads never persist budget fields (n_ctx-derived).
- `reasoning_enabled` is a nullable per-model override for provider-native reasoning
  modes. It applies to **all cloud workloads** (openai, gemini, openrouter, nvidia),
  not just NVIDIA/OpenRouter. Whether a provider is toggleable, its default, and its
  wire mechanism are declared in the `ReasoningCapability` table in
  `assistant/reasoning_param.go` and surfaced to the frontend via
  `provider_defaults[provider].reasoning` in `GET /admin/api/state` — the UI renders
  the toggle from that descriptor and never hardcodes provider names. Local workloads
  (`WorkloadLocal`, incl. an OpenAI slug pointed at a loopback URL) are non-toggleable:
  no toggle renders and no wire change occurs.
  - Unset preserves the provider's native default; `ReasoningCapability.DefaultEnabled`
    is `true` for every cloud provider today.
  - ModeEnableThinking (nvidia) → `chat_template_kwargs.enable_thinking`. An explicit
    `false` is serialized as `"enable_thinking": false` (the field has no `omitempty`),
    so a disabled override reaches the wire instead of being dropped.
  - ModeObject (openrouter) → `reasoning.enabled` (explicit `false` still serializes).
  - ModeEffort (openai/gemini) → `reasoning_effort`. **Behaviour change (2026-08-02):**
    a stored `reasoning_enabled:false` now maps to *omitting* `reasoning_effort` (provider
    default) instead of the explicit `medium` sent before; `true` maps to `medium`. Effort
    mode expresses "disabled" as omission (universally safe on Chat Completions) until
    `reasoning_effort:"none"` is verified end-to-end, at which point the resolver flips to
    send `"none"`.
  - **Unset is preserved.** The UI never coerces a model with no persisted
    `reasoning_enabled` into an explicit value on edit, and the persistence layer removes a
    stale override entry when no override remains — so clearing the field restores the
    provider default rather than leaving a frozen value behind.
  - The registration edge classifies workload with the fully-hydrated effective endpoint
    (per-credential `base_url` overrides applied), the same classification used at the
    runtime boundary, so an OpenAI slug whose credential points at a loopback URL is never
    registered as cloud.
  - OpenAI-compatible base URLs (Gemini's compat endpoint, self-hosted proxies) inherit the
    `openai` capability row automatically; a base URL reclassified `WorkloadLocal` is
    excluded by the workload classifier.
- `ApplyMetadataDefaults()` sets `ToolCallFormat` to `"native"` when empty for **cloud**
  workloads only; local/GGUF workloads stay empty (XML text mode).
- ¹ The local `MaxTokens` (2048) is a **display-only prefill** for the UI. At runtime the
  local budget derives from serving context (`LocalBudgetPolicy.Derive` = `ctxLen/3`, ~2730
  for the 8192 default) and ignores this row. It is intentionally distinct from
  `assistant.DefaultMaxTokens` (3072), the global agent-loop fallback for an unconfigured
  model.

### 4. Reasoning Budget

- Reasoning wire params are resolved per provider by `ReasoningSpec` +
  `ReasoningParamResolver` (strategy pattern in `assistant/reasoning_param.go`).
  Only the provider-appropriate field is serialized on the ChatRequest:
  - `thinking_budget_tokens` — llama.cpp/local (ModeThinkTokens).
  - `reasoning_effort` — openai/gemini (ModeEffort).
  - `reasoning` object (`effort`) — openrouter (ModeObject).
  - `chat_template_kwargs.enable_thinking` — nvidia (ModeEnableThinking).
- A workload classified `WorkloadLocal` (via the shared `WorkloadClassifier`, the same
  classifier that drives budget and ICU) always overrides to `thinking_budget_tokens`,
  preserving the working local path even for cloud-slugged configs pointed at a local URL.
- Local think-token budget is derived at agent-build time in `resolveReasoningSpec`:
  `DefaultReasoningBudget(maxTokens)` = `max_tokens / 3` when no explicit budget is
  configured. Because `max_tokens` itself derives from serving context (`ctxLen / 3`,
  **local-only**), the budget tracks the launched context. Explicit per-model
  `reasoning_budget` overrides the derived value. No model-name heuristics.
- Cloud providers never receive a numeric think-token budget.

### 5. ICU (Inter-Call Underwriting)

- Tracks total token spend per session.
- Refunds on stream failures to prevent double-charging.
- Prerequisites are checked via `doPreflightCheck()` before each LLM call.

## III. Error Handling

- Budget exceeded: return `ErrBudgetExceeded` — caller must pause or fail.
- Context resolution failure: local workloads always resolve a numeric fallback
  (`defaultLocalContextLength`) — never a typed error on the runtime path and never a
  cloud default. Cloud workloads with a published context too small for any viable prompt
  reserve (`publishedCtx ≤ minViablySmallContext`) return typed `ErrCapabilityImpossible`;
  all larger windows clamp and succeed.
- Output-cap 400s: converted to typed `OutputCapError` at the provider edge, never retried.
- Refund on already-refunded transaction: no-op (idempotent).

## IV. Configuration

- `ModelConfig.MaxTokens`, `ModelConfig.ContextBudget` in `registry.json`.
- `model_overrides` in `settings.yml`.
- No global defaults override per-model settings.
