---
id: SPEC-005
title: Orchestrator / Budget
version: "1.5"
status: stable
last_updated: 2026-10-21
constitution_references: [VI]
related_specs: [SPEC-001]
supersedes:
---

# SPEC: Orchestrator / Budget

## Changelog

- **1.5 (2026-10-21)** — A local model is handed out only once its server has answered its window. llama-server opens its
  port while it is still reading the weights and answers 503 until it has, so the once-per-start serving-context probe failed
  on a cold start and the run that triggered the start used the stored metadata (the training window, 262144 tokens, where the
  server serves 16384) for all its turns: a 699,052-character budget, a result cap at its ceiling, and a request that overflowed
  the real window. `ReconcileLocalServingContext` now reports whether the model may be used; while the server is silent
  `GetInstance` returns `ErrModelStarting` (callers already wait on it, chat with a "model is starting" notice), for at most
  `llm.ServingContextGrace` (2 minutes, from the first silent probe since the start), after which the model is used with the
  metadata it has. A context overflow that reports the server's window also lowers the run's output cap and context budget to
  what that window allows (`adoptReportedWindow`; only ever lowers). SPEC-001 §2 unchanged.

- **1.4 (2026-10-18)** — `TokenRatios` (on `Orchestrator.Ratios`) learns, per model, how many characters of a request cost
  one prompt token, from the server's own count (default 3.0, blended 50/50, clamped 1.5 to 6.0, at most 256 models,
  nil-safe, `sync.Mutex`). A local run's character limit is its persisted `ContextBudget` times learned ÷ 4.0
  (`LocalCharsPerToken`, now exported): the budget stays stated at the nominal 4 chars a token and holds the run to the
  token reserve whatever it contains. A cloud `ContextBudget` is a history allowance for cost and is not scaled. Nothing
  persisted or shown changed unit. SPEC-001 1.7.

- **1.3 (2026-10-05)** — Remote llama.cpp behind an OpenAI-style URL is classified **local** from the
  model's own listing entry. `ModelMeta.Serving` / `ModelMetadata.Serving` (`"llamacpp"`) is set per entry
  from `owned_by "llamacpp"`, `meta.n_ctx_train` or a `.gguf` id, persisted with the model, and read by
  `WorkloadClassifier.Classify`; a published context alone never makes a model local. Models saved earlier
  are marked at startup (`RefreshServingFingerprints`, runtime-only). The proxy's own `/v1/models` lists only
  models it serves with llama.cpp as `owned_by "llamacpp"` with `meta.n_ctx_train` (cloud entries carry their
  provider). `ModelInstance.Local` makes the client follow the class (transport and reasoning field).
  Deployment note: an older proxy stamps every model it fronts as llama.cpp, so a cloud model behind it
  would be read as local until that proxy is updated.

- **1.2 (2026-09-29)** — Reference correction (no behavior change): budget-exceeded is signalled by
  `PreFlightResult{Allowed: false, Reason: …}` (surfaced as a `budget exceeded: <reason>` error),
  not a removed `ErrBudgetExceeded` sentinel.

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

    The serving context probe (`/slots`) is listing-driven, not host-only: it runs
    for effective-local endpoints **or** any `/v1/models` listing with an entry carrying a
    llama.cpp/local-workload fingerprint (`owned_by "llamacpp"`, `meta.n_ctx_train`,
    or a `.gguf` artifact id). The fingerprint is read **per entry** and persisted as
    `Metadata.Serving`; the probed `n_ctx` is written only to the fingerprinted entries. A **remote**
    llama.cpp host (including one behind an OpenAI-style slug) is therefore a local workload and
    resolves its real serving `n_ctx` — never the training `n_ctx_train` — and the probe result
    overrides any training-derived `ContextLength` (see `docs/PLANS/ARCHIVE/cross-cutting/cloud-provider-token-budgets.md` §3.4).
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
  wins (is clamped to) the tier row. This ceiling is deliberate even when a model
  publishes a larger cap: `max_tokens` also sizes stuck detection and the stream
  content cap (`internal/core/assistant/stream.go`), both tuned against the tier value
  (`docs/PLANS/ARCHIVE/cross-cutting/cloud-provider-token-budgets.md` §1.8/§2.5 measured
  stuck detection going dormant at an uncapped max_tokens). Lifting it for a specific
  model requires decoupling those thresholds from `max_tokens` first, not just raising
  the tier row.
- The cloud `ContextBudget` (history budget) is different: the tier row is a **no-data
  fallback**, not a ceiling. `CloudBudgetPolicy.Derive` only clamps the history budget to
  the tier row when nothing model-specific is known (`ContextResolution.PublishedContextKnown`
  false — i.e. the published-context chain fell through to the tier's own `DefaultContext`
  guess). A verified real published context (live catalog, metadata, or a `knownCtx`
  fragment match) is never clamped down to the generic provider-type guess, because
  nothing downstream of `ContextBudget` couples it to the output-cap safety mechanisms
  above — `hotMemoryCharBudget`/`recallBudget`/`ledgerCharBudget` each carry their own
  independent char ceiling regardless of how large `ContextBudget` is.
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
  local budget derives from serving context (`LocalBudgetPolicy.Derive` = `ctxLen/3`;
  `context_budget = (ctxLen − maxTokens) × 4 chars/token` — the real tokenizer ratio,
  ≈ 21848 chars for the 8192 default, vs the old `× 2` ≈ 10924 that under-provisioned the
  sieve ~2×). It is intentionally distinct from `assistant.DefaultMaxTokens` (3072), the
  global agent-loop fallback for an unconfigured model.

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

- Budget exceeded: `PreFlightCheck` returns `PreFlightResult{Allowed: false, Reason: …}`; the loop
  surfaces it as a `budget exceeded: <reason>` error — caller must pause or fail. (There is no
  `ErrBudgetExceeded` sentinel.)
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
