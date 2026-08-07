---
title: Provider-Agnostic Reasoning Enable — Capability-Driven Abstraction
status: proposed
date: 2026-08-02
related_specs: [SPEC-005]
---

# Provider-Agnostic Reasoning Enable

## Intent

Extend the per-model `reasoning_enabled` toggle from NVIDIA + OpenRouter to **every cloud
workload** (OpenAI, Gemini, NVIDIA, OpenRouter) while removing the scattered
`provider === "nvidia" || provider === "openrouter"` checks in the frontend. Replace them
with a single declarative capability descriptor owned by the backend and surfaced through
the admin API, plus the existing resolver strategy as the wire-mapping abstraction. Local
llama.cpp workloads (`WorkloadClass == local`, including OpenAI-slugged loopback URLs) are
explicitly excluded — no toggle, no wire change.

The system stays **LLM-agnostic**: the capability table is keyed by *provider type* (the
wire protocol), not by vendor. An OpenAI slug pointing at any OpenAI-compatible cloud
endpoint (including Gemini's compat endpoint) uses the `openai` row; an OpenAI slug
pointing at a local llama.cpp URL is reclassified `local` by the shared `WorkloadClassifier`
and follows the local path. Local numeric fields remain dynamically derived
(`useTuningFieldPolicy`, SPEC-005 §2.7); cloud numeric/tuning logic is unchanged except for
the R4 bug fix below.

This is a **review-driven follow-up** to
`per-model-reasoning-overrides-and-settings-layout.md` (status: complete, 2026-08-02).

## Review findings (what is wrong today)

| # | Location | Problem |
|---|---|---|
| R1 | `frontend/src/components/settings/ModelTuningFields.vue:101` | `v-if="provider === 'nvidia' \|\| provider === 'openrouter'"` — hardcoded provider names decide whether the toggle renders. Adding a provider means editing the component. |
| R2 | `frontend/src/composables/settings/useProviderModels.ts:245` | `handleEdit` only seeds `reasoning_enabled` for nvidia/openrouter — same hardcoded list, duplicated. |
| R3 | `frontend/src/utils/model/modelUtils.ts:39-41` | `defaultReasoningEnabled()` returns `provider === "nvidia" \|\| provider === "openrouter"` — third copy of the same list. |
| R4 | `backend/internal/core/assistant/agent.go:90-95` | `applyReasoningEnabledOverride` hardcodes `spec.Mode == ModeEnableThinking \|\| spec.Mode == ModeObject`. Mode-based, not capability-based, and it silently drops the override for `ModeEffort` (OpenAI/Gemini) — so per-model `reasoning_enabled` already does nothing for them today. |
| R5 | `backend/internal/core/assistant/reasoning_param.go:162-168` | `providerReasoningTable` rows carry `Enabled: true` for nvidia/openrouter but nothing for openai/gemini; "default enabled" is only encoded for two providers. |
| R6 | `adminTuningDefaults` / `provider_defaults` (admin_handlers.go:146-162, 385-397) | No reasoning capability exposed to the frontend — which is *why* the frontend hardcodes provider names. |
| R7 | Workload gating | The toggle is not gated on `workload_class` today. An OpenAI-slug pointing at a local llama.cpp URL (`provider: "openai"`, `workload_class: "local"`) currently renders the toggle — a meaningless wire change for a local server. |

**What is already good (keep):** `providerReasoningTable` + `ReasoningParamResolver`
(reasoning_param.go) is a real strategy abstraction — `buildChatRequest` (stream.go:76)
depends only on the resolver, and per-provider wire knowledge lives in one table. The
`WorkloadClassifier` (models/workload.go) is the single authority for local-vs-cloud and is
already used for budget/ICU/reasoning. The numeric tuning table
(`models/ProviderTuningDefaults`) deliberately excludes reasoning (tuning.go:11-13) — that
separation is preserved. The plan builds on all three; it does not re-architect them.

## Answering "how do we know the correct thinking/reasoning property per provider?"

The property name is **provider API documentation data**, not something derivable from the
slug. The wire mechanism differs in *kind*, not just name:

| Provider | Wire location (request JSON) | Mechanism | Disable expressible? |
|---|---|---|---|
| NVIDIA NIM | `chat_template_kwargs.enable_thinking` (bool) | flag | Yes — `false`. **Verified in repo tests** |
| OpenRouter | `reasoning.enabled` (bool) + `reasoning.effort` | object flag | Yes — `false`. **Verified in repo tests** |
| OpenAI (Chat Completions) | `reasoning_effort` (enum `low\|medium\|high`, `none` on newer GPT-5.x) | enum effort | Partial — `none`/omit. Needs live verification |
| Gemini (native API) | `generationConfig.thinkingConfig.thinkingBudget` (int, `0` = off) | token budget | Yes — `0`. Repo routes Gemini via the OpenAI-compat endpoint, so the effective field here is `reasoning_effort` |
| Anthropic (future) | `thinking.type` = `"enabled"`/`"disabled"` | object mode | Yes |
| llama.cpp (local — excluded) | `chat_template_kwargs.enable_thinking` / `thinking_budget_tokens` | flag/budget | Excluded by workload class |

OpenAI-compatible base URLs (Gemini compat endpoint, self-hosted proxies) all speak
`reasoning_effort`, so they inherit the `openai` row automatically. A base URL that routes
to local llama.cpp is reclassified `local` by the workload classifier and excluded.

**Implication:** the "right property" is captured as one declarative row per provider in a
single capability table. Adding a provider = one table row + (only if the wire mechanism is
new) one resolver. No UI change, no `provider ===` anywhere. The table is the contract;
"knowing the name" reduces to looking it up in the provider's API reference once, when the
row is added.

## Scope

### 1. Backend — capability descriptor (single source of truth)

Extend `reasoning_param.go` so the table row declares capability, not just the wire spec:

```go
type ReasoningCapability struct {
    Mode           ReasoningMode // existing
    Effort         ReasoningEffort
    DefaultEnabled bool          // provider's native default (nvidia/openrouter already true)
    Toggleable     bool          // a disabled state is expressible on the wire
}
```

- `providerReasoningTable` becomes `map[string]ReasoningCapability` (wire fields `Enabled`/
  `Budget` are still resolved into `ReasoningSpec` at compose time, so
  `resolveReasoningSpec` / `composeProviderTiers` keep working unchanged).
- Rows: `local` → `Toggleable:false`; `openai`/`gemini` → `Toggleable:true, DefaultEnabled:true`;
  `openrouter`/`nvidia` → `Toggleable:true, DefaultEnabled:true` (unchanged defaults).
- Add `EffortNone` to `ReasoningEffort`. Give it an explicit `String()` case returning `""`
  (the current default-case-returns-`"medium"` at `reasoning_param.go:53-54` would otherwise
  leak `"medium"`).
- `ReasoningSpec.Validate()` matrix: `ModeEffort` + `EffortNone` = **valid**; `ModeObject` +
  `EffortNone` = **invalid** (OpenRouter needs an effort string); `ModeEnableThinking` /
  `ModeThinkTokens` + `EffortNone` = **invalid**.
- `effortResolver.Apply` (`reasoning_param.go:99`) MUST short-circuit on `EffortNone` BEFORE
  any `String()` call: when `spec.Effort == EffortNone`, delegate to `noopResolver.Apply`
  (clears `ReasoningEffort`, `Reasoning`, `ChatTemplateKwargs`, budgets) and `return`. Do NOT
  rely on the `""` sentinel alone — the explicit branch makes the "send nothing" contract
  self-evident and removes the `String()` dependency. For all other efforts, behavior is
  unchanged.   Because `Validate()` forbids `EffortNone` outside `ModeEffort`, `effortResolver`
  only ever sees `EffortNone` in the effort path.
- **Runtime guard:** `buildChatRequest` (`stream.go:76`) MUST call `spec.Validate()` on the
  final `ReasoningSpec` (post-`applyReasoningEnabledOverride`, pre-`Apply`) and return a typed
  error on failure. Today `Validate` only guards test construction
  (`reasoning_param.go:59-60`); without a runtime call the §1 matrix cannot stop an invalid
  `ModeObject + EffortNone` spec reaching the wire.
- Rework `applyReasoningEnabledOverride` (R4) to be **capability- and workload-driven** and
  take the workload class (currently it only sees the spec — the openai-slug→local case has
  `spec.Mode == ModeEffort`, so a `ModeThinkTokens` guard would miss it):

  ```go
  func applyReasoningEnabledOverride(spec ReasoningSpec, enabled *bool, workload models.WorkloadClass) ReasoningSpec {
      if workload == models.WorkloadLocal || enabled == nil {
          return spec
      }
      if spec.Mode == ModeThinkTokens { // non-toggleable; guard for safety
          return spec
      }
      switch spec.Mode {
      case ModeObject, ModeEnableThinking:
          spec.Enabled = *enabled                      // today's behaviour
      case ModeEffort:
          if *enabled { spec.Effort = tierEffort }     // medium
          else        { spec.Effort = EffortNone }      // omit on the wire
      }
      return spec
  }
  ```
  `NewAgent` passes `opts.WorkloadClass` (agent.go:507).
  **Hard dependency:** `opts.WorkloadClass` MUST already be `WorkloadLocal` for an
  openai-slug→local loopback at `agent.go:507`. It is set by the caller that builds
  `AgentOptions` (that caller must run the shared `WorkloadClassifier` on the model's endpoint
  before construction — verify this in the AgentOptions builder; if it does not, classify
  there first). The request-time reclassification in `NewReasoningResolver`
  (`reasoning_param.go:178` / `stream.go:76`) is a SECOND guard, not a substitute: the
  override at `agent.go:507` runs against `resolveReasoningSpec(opts.ProviderType, …)` which
  keys on `ProviderType`, so an unclassified loopback would hit the `ModeEffort` branch and
  mutate the spec before the resolver later discards it. Local workloads — including an OpenAI
  slug classified `local` — never take the override; the `localOverrideResolver` already
  ignores the spec and uses only the configured budget.
  - Add test `TestApplyReasoningEnabledOverride_LocalLoopbackOpenaiSlug`: given
    `WorkloadLocal` + `ProviderType:"openai"` + `enabled:*false`, the returned spec is
    byte-identical to the input (no mutation), proving the gate holds regardless of slug.
- Add `ReasoningCapabilityFor(providerType)` exported from `assistant` so the admin handler
  builds the API payload from the same table.

**Design decision (flagged):** effort-mode "disabled" maps to *omitting* `reasoning_effort`
rather than sending `"none"`. Rationale: `none` is only valid on newer GPT-5.x and this repo
uses Chat Completions, not Responses; omission is universally safe and for reasoning-only
models degrades to provider-default effort (never a hard 400 — and the existing
retry-without-reasoning-params fallback at stream.go:802 already absorbs a rejected
parameter). If live testing confirms `reasoning_effort:"none"` works end-to-end on OpenAI
and Gemini's OpenAI-compat endpoint, flip `effortResolver` to send `"none"` instead — a
one-line change, no structural impact. Until verified, the toggle label for effort-mode
providers reflects this (see §3).

**Behavior change (intentional, must be documented in SPEC-005 §3):** the old UI already
persisted `reasoning_enabled: false` for every OpenAI/Gemini model added via the form
(`defaultReasoningEnabled` returned `false` for them, and `modelFormRequest` always carried
the bool). Today that stored value is inert (R4 drops effort overrides). After this change
it becomes active: `false` ⇒ `reasoning_effort` omitted (provider default) instead of the
explicit `medium` sent today. This is the intended fix, not a regression — but it affects
every existing OpenAI/Gemini model, so it is called out here and covered by a regression
test, not silently shipped.

### 2. Backend — expose capability via admin API

- `adminTuningDefaults` gains a `Reasoning` capability object:
  `{ supported, toggleable, default_enabled, mode }` (JSON snake_case).
- `convertProviderTiers` signature changes from `map[string]models.ProviderTuning` →
  `map[string]assistant.ProviderTuningDefaults`, and its BODY must read `Reasoning` out of the
  `assistant.ProviderTuningDefaults` (not just the numeric `models.ProviderTuning`) to
  populate `adminTuningDefaults.Reasoning` via `assistant.ReasoningCapabilityFor(k)`. The call
  site at `admin_handlers.go:315` changes from `models.ProviderTuningDefaults()` to
  `assistant.ProviderTiers()`. This introduces **no new dependency** — `admin_handlers.go`
  already imports `assistant` for the default constants (`baseAdminTuningDefaults`), and
  `models/tuning.go` deliberately excludes reasoning, so `assistant` is the correct composition
  point. Numeric fields read from the embedded `models.ProviderTuning`; `Reasoning` populated
  via `assistant.ReasoningCapabilityFor(k)`. Confirm no import cycle (`assistant → models`
  only; `handlers → assistant` is already exercised by `baseAdminTuningDefaults`).
- `adminModelView` keeps `reasoning_enabled` (the per-model override) and already carries
  `workload_class` (admin_view.go:101); the capability itself is provider-scoped so
  `provider_defaults` is the single channel. Per-model capability is unnecessary.
- `modelFormRequest` already accepts `reasoning_enabled` (model_handlers.go:139); persistence
  path (`writeModelOverrides` / `hasModelOverrides` / `ModelOverride`) is unchanged — it is
  already provider-agnostic.

### 3. Frontend — remove every `provider ===` reasoning check; capability-driven rendering

- `frontend/src/types/admin.ts`: new `ReasoningCapability` interface
  `{ supported: boolean; toggleable: boolean; default_enabled: boolean; mode: string }`;
  `AgentDefaults` gains `reasoning?: ReasoningCapability`.
- `frontend/src/utils/model/modelUtils.ts`: delete `defaultReasoningEnabled` (R3).
  `getDefaultModelSettings` sources `reasoning_enabled: defaults.reasoning?.default_enabled ?? false`.
- `frontend/src/composables/settings/useProviderModels.ts:245`: `handleEdit` seeds
  `reasoning_enabled` gated on **workload + capability**, not provider list (R2, R7):

  ```ts
  if (editingModel.value && model.workload_class === 'cloud') {
    editingModel.value.reasoning_enabled =
      model.reasoning_enabled ?? agentDefaults.value.reasoning?.default_enabled ?? false
  }
  ```
  `agentDefaults` is already computed in this composable and resolves to
  `provider_defaults[provider]` (useProviderModels.ts:34-38).
  **Precondition to verify:** list `Model` objects passed to `handleEdit` must carry
  `workload_class` (populated by `adminModelView`, admin_view.go:101). If the models list
  omits it, the `=== 'cloud'` gate silently falls through and `reasoning_enabled` is never
  seeded — add a guard or fall back to `policy.isCloud` (already computed in
  `ModelTuningFields` via `useTuningFieldPolicy`). Confirm `model.workload_class` is present
  in `useProviderModels.ts` before relying on it.
- `frontend/src/components/settings/ModelTuningFields.vue:101`: render the "Enable Thinking"
  block from capability + workload, not provider name (R1):

  ```html
  <div v-if="policy.isCloud && reasoning?.toggleable" class="form-group">
  ```

  New prop `reasoning?: ReasoningCapability`. Label/tooltip is **mode-aware**: for
  `mode === 'effort'` the tooltip reads "Send explicit reasoning effort (on) vs provider
  default (off)" — a true on/off only arrives once `reasoning_effort:"none"` is verified;
  for `enable_thinking`/`object` it reads "Enable provider-native reasoning".
- `frontend/src/components/settings/ProviderModelsCard.vue` (NEW to Expected Files): add
  `agentDefaults` to the `useProviderModels` destructure (ProviderModelsCard.vue:22-43) and
  pass `:reasoning="agentDefaults?.reasoning"` at all four `ModelTuningFields` call sites
  (:116, :200, :238, :275). The local sites pass `provider="local"`, whose
  `provider_defaults.local.reasoning.toggleable === false`, so the toggle stays hidden.

**UI editability matrix (workload × field) — mirrors SPEC-005 §2.7, unchanged for cloud:**

| Field | Cloud | Local (incl. openai-slug→local URL) |
|---|---|---|
| `max_tokens`, `context_budget` | editable (clamped to published cap) | **derived** (n_ctx math, read-only — existing policy) |
| `reasoning_budget` | editable (per-request) | editable → `thinking_budget_tokens` (per-request; supported by llama.cpp) |
| `reasoning_enabled` toggle | shown iff `reasoning.toggleable` | **never shown** |
| `temperature`, `tool_call_format`, `prefill`, timeouts | editable | editable |

Fields a local server cannot honor are disabled by the workload-class policy, never by
provider name. No field is newly enabled anywhere.

### 4. Docs

- Update SPEC-005 §3: `reasoning_enabled` now applies to all cloud workloads; the
  per-provider wire mapping is the capability table; local workloads remain excluded;
  effort-mode `false` = omitted `reasoning_effort` (documented behavior change, §1 above);
  OpenAI-compatible base URLs inherit the `openai` row.
- Add this plan to `docs/INDEX.md`.
- Note the "provider-name checks" pitfall and the capability-table pattern in
  `docs/architecture.md` if not already covered.

## Process (mandatory per AGENTS.md)

- **Before coding:** load `.agents/rules/go-staff-engineer.md` AND
  `.agents/rules/frontend-vue-engineer.md`; run `cd backend && go build ./... && go test ./...`
  as the baseline.
- **TDD:** write the failing tests in Expected Files first (red), then implement (green).
- After each backend edit: `go build ./...` from `backend/`; after each frontend edit:
  `npm run build` from `frontend/`.
- **Before finishing:** `go test ./...` + `go run ./tools/check-complexity/` (≤12) from
  `backend/`, and `npm run build` from `frontend/`.

## Expected Files

Backend:

- `backend/internal/core/assistant/reasoning_param.go` — `ReasoningCapability` struct,
  `EffortNone` (+ `String()` case), resolver/Validate updates, `ReasoningCapabilityFor`.
- `backend/internal/core/assistant/agent.go` — capability- + workload-driven
  `applyReasoningEnabledOverride`; `NewAgent` passes `opts.WorkloadClass` (agent.go:507).
- `backend/internal/transport/http/handlers/admin_handlers.go` — `adminTuningDefaults.Reasoning`
  + `convertProviderTiers(assistant.ProviderTiers())`.
- `backend/internal/transport/http/handlers/admin_handlers_test.go` — assert reasoning
  capability in `provider_defaults` for openai/gemini/openrouter/nvidia/local.
- `backend/internal/core/assistant/agent_test.go` — override matrix: effort enabled→medium,
  effort disabled→EffortNone, nil→tier default, local (incl. openai-slug)→untouched.
- `backend/internal/core/assistant/reasoning_param_test.go` — EffortNone string/Validate,
  effort-disabled wire output, capability defaults.

Frontend:

- `frontend/src/types/admin.ts`
- `frontend/src/utils/model/modelUtils.ts`
- `frontend/src/composables/settings/useProviderModels.ts`
- `frontend/src/components/settings/ModelTuningFields.vue`
- `frontend/src/components/settings/ProviderModelsCard.vue`

Docs:

- `docs/SPECS/orchestrator.md`
- `docs/INDEX.md`
- `docs/architecture.md`

## Test Plan

- `reasoning_enabled` unset / true / false for every cloud provider (openai, gemini,
  openrouter, nvidia) → correct wire per capability table.
- Effort-mode disabled ⇒ no `reasoning_effort` on the wire; effort-mode enabled ⇒
  `reasoning_effort: medium`; nil ⇒ tier default. (`agent_test.go` — override lives in
  `agent.go`.)
- **Behavior-change regression:** a persisted `reasoning_enabled:false` on an openai/gemini
  model now yields omitted `reasoning_effort` (previously inert). Asserted and documented.
- OpenRouter explicit `enabled:false` still serializes (regression guard,
  `TestObjectResolverSerializesDisabledReasoning`).
- NVIDIA `chat_template_kwargs.enable_thinking` true/false (regression).
- Local workload (incl. loopback OpenAI slug) ⇒ never sends/receives a toggle and never
  changes wire (`TestNewReasoningResolver_LocalHostOverride` regression).
- `provider_defaults[provider].reasoning` present and correct in the admin state JSON;
  `local.reasoning.toggleable === false`.
- `go build ./... && go test ./...` and `go run ./tools/check-complexity/` from `backend/`;
  `npm run build` from `frontend/`.
- Manual: settings form shows the toggle for all cloud providers (mode-aware label), hidden
  for local workloads (incl. an OpenAI model pointed at a loopback URL), default checked per
  `default_enabled`.

## Non-Goals

- No change to `reasoning_budget` (think-token budget) semantics.
- No new wire mechanism for Anthropic or other providers — the capability table is
  additive; only the four existing cloud providers get the toggle now.
- No change to local llama.cpp reasoning (workload-local wire path is untouched).
- No migration of existing settings.yml `reasoning_enabled` values — the documented
  behavior change in §1 applies to whatever is already persisted.
- No change to cloud token-budget calculation, credential provisioning, or the numeric
  per-provider tuning table (`models/tuning.go`).
