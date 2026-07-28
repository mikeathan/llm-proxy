---
status: proposed
last_reviewed: 2026-08-01
---

# Cloud Provider Token Budgets + Provider Set Reduction

**Status:** 📋 Proposed — design revised, implementation not started
**Related SPECs:** SPEC-005 (Orchestrator / Budget), SPEC-003 (Discovery Panel UI)
**Constitution:** I.1–I.2 (network clients), II.11 (per-model config flow), III.5 (two-tier model persistence), VI (budget)
**Rules:** `.agents/rules/go-staff-engineer.md`, `.agents/rules/frontend-vue-engineer.md`

**Origin:** investigation of a `laguna-xs-2.1` (NVIDIA) run that took 8 minutes and was
cancelled by the user — `backend/data/runs/workspace-test/laguna-xs-2.1/20260801T080104Z_89e1194c3e8bcf8f/`.
Every request in that log carried `max_tokens 42666`.

---

## 0. NON-NEGOTIABLE: the local invariant

> **Local models and `openai`-provider models pointed at a local LLM must not change
> behaviour. Not their token math, not their context resolution, not their reasoning
> budget, not their tool-call format. Zero risk. This overrides every other goal in
> this document.**

What "local" means here is broader than the current code assumes — see §2.2. It covers:

| Case | Example | Today |
|---|---|---|
| `provider: local` | any GGUF launched by us | `ctx/3` |
| `provider: openai` + `.gguf` name | `registry.json:12-110` (all 6 entries) | `ctx/3` |
| `provider: openai` + local URL + non-`.gguf` name | LM Studio / vLLM / `llama-server --alias foo` | **`ctx/3` today only by luck — see M8** |

The third row is the fragile one. It is *not* currently detected by the budget path's
local test, and a naive implementation of this plan would silently move it onto the
cloud branch. §2.2 fixes that; §3 proves it stayed fixed.

**Enforcement mechanism (§3):** a committed golden snapshot captured *before* any change.
Every local entry in that snapshot must remain byte-identical through every phase. The
snapshot diff is the audit — a single changed local line fails review.

---

## 1. Problem

### 1.1 Cloud models get a local-model formula (root cause)

`ApplyMetadataDefaults` (`backend/internal/core/orchestrator/budget_squeezer.go:136-153`)
applies one formula to every provider:

```
max_tokens     = ctx / 3
context_budget = (ctx - max_tokens) * 2
```

This is a **llama.cpp** rule and it is correct there: one KV cache holds prompt *and*
response, so carving the serving window into thirds is right. Cloud APIs do not work that
way — the context window and the per-request output cap are independent quantities.

`laguna-xs-2.1` has no metadata (`backend/data/registry.json:112-119`), so
`resolveContextLength` falls to `providerCtxDefaults["nvidia"] = 128_000`
(`budget_squeezer.go:186`):

```
max_tokens     = 128000 / 3           = 42666
context_budget = (128000 - 42666) * 2 = 170668
```

### 1.2 The provider tier is never applied at runtime

`assistant/agent.go:114` declares `nvidia → {MaxTokens: 2048, ContextBudget: 20000}`.
That table is consumed only for form prefill and admin defaults
(`admin_handlers.go:322`). At load, `manager.go:528-536` zeroes the fields and calls
`ApplyMetadataDefaults`, which knows nothing about tiers:

```go
cfg.MaxTokens = 0
cfg.ContextBudget = 0
cfg.ReasoningBudget = 0
orchestrator.ApplyMetadataDefaults(&cfg)
```

The declared 2048 is silently replaced by 42666 on every sync.

### 1.3 The UI's `max_tokens` / `context_budget` inputs are write-only theatre

`ModelOverride` carries both fields (`models/infrastructure.go:35-36`) and
`ApplyModelOverrides` applies them (`manager.go:556-562`) — but `hasModelOverrides`
excludes them (`model_handlers.go:252-259`) and `writeModelOverrides` never writes them
(`:272-287`). So a UI edit applies in memory for one request, is never persisted, and
`Sync()` restores 42666. No working escape hatch exists short of hand-editing
`settings.yml` (which does not exist yet on this install).

### 1.4 Frontend and backend disagree on the formula

`frontend/src/utils/model/modelUtils.ts:106` uses `ctx / 4`; the backend uses `ctx / 3`.
`computeDefaultsFromContext` fires only on **cloud add** (`frontend/src/composables/settings/useProviderModels.ts:107-121`)
— never on edit, never for local.

### 1.5 Cloud metadata never arrives — verified against live APIs

`ListModels` (`provider_openai_compatible.go:85-92`) decodes only `pricing`,
`limits.context`, and `meta.{n_ctx_train,n_ctx,n_params}`.

**NVIDIA** `GET https://integrate.api.nvidia.com/v1/models` (2026-08-01, unauthenticated):

```
102 models — keys: id, object, created, owned_by.  Nothing else.
```

**OpenRouter** `GET https://openrouter.ai/api/v1/models` (2026-08-01):

```
336 models
context_length ................................ 336/336
top_provider.max_completion_tokens ............ 294/336
"limits" key .................................. 0/336
"meta" key .................................... 0/336
```

The parser matches **nothing** OpenRouter publishes, despite the file header claiming
OpenRouter support (`provider_openai_compatible.go:1-4`). The `meta` shape is llama.cpp's
extension; `limits` matches no provider in use. OpenRouter falls back to 42666 like NVIDIA.

Real OpenRouter shape:

```json
{ "id": "deepseek/deepseek-v4-flash-0731",
  "context_length": 1048576,
  "top_provider": { "context_length": 1048576, "max_completion_tokens": 384000 },
  "pricing": { "prompt": "0.00000014", "completion": "0.00000028" } }
```

### 1.6 Wasted network calls; Constitution I.1 violations

`fetchSlotsContext` (`provider_openai_compatible.go:113,130-137`) runs unconditionally,
firing two futile HTTPS requests to `integrate.api.nvidia.com/slots` and `/v1/slots` on
every cloud listing. `/slots` is a llama.cpp root endpoint.

Three un-timeouted clients violate Constitution I.1 (`http.DefaultClient` /
`http.Get` are explicitly prohibited):

| Location | Call |
|---|---|
| `provider_openai_compatible.go:75` | `http.DefaultClient.Do` |
| `provider_openai_compatible.go:153` | `http.DefaultClient.Do` |
| `provider_gemini.go:47` | `http.Get` |

### 1.7 ICU accounting is inflated by the same bug

`budget_manager.go:128` reserves ICU from `MaxTokens`:

```go
allocatedICU = int64(float64(req.MaxTokens+req.ReasoningBudget+req.ContextChars/2) * req.ICUWeight)
```

and `:139` records `ResponseTokens: req.MaxTokens`. At 42666 every cloud request
over-reserves ~5×. Fixing §1.1 therefore **changes ledger behaviour** — reservations
shrink, effective throughput per ICU window rises. A correction, but a visible one.

### 1.8 `max_tokens` silently controls three agent-loop safety mechanisms

Not just a wire parameter:

| Mechanism | Formula | At 42666 | At 8192 | At 2048 |
|---|---|---|---|---|
| Stuck detection (`stream.go:430`) | `reasoning > max_tokens/1` | 42,666 chars | 8,192 | 2,048 |
| Stream char cap (`stream.go:921`) | `content > max_tokens*4` | 170,664 chars | 32,768 | 8,192 |
| Native-tools content cap (`stream.go:684`) | `content > max_tokens` | 42,666 chars | 8,192 | 2,048 |

Two consequences:

1. **At 42666 all three are effectively dormant.** The origin run spun 20 steps with a
   stuck threshold of 42,666 characters against actual reasoning of 55–559 characters per
   step. Independent second reason the current value is wrong.
2. **2048 would be actively harmful** — the char cap would truncate any report over
   ~8,192 characters (~2,000 words) mid-stream.

`stream.go:430` has **no zero-guard** (unlike `:921`, which checks `MaxTokens > 0`), so
`MaxTokens == 0` sets the threshold to 0 and *any* reasoning content trips stuck
detection instantly. This blocks any future "omit `max_tokens`" strategy until the
consumers are decoupled. See M10.

---

## 2. Design

### 2.1 Precedent to follow

`assistant/reasoning_param.go` already solved this exact class of problem in this
codebase. Its header:

> Consolidates what was previously split across `client.go` (ReasoningField), `agent.go`
> (providerTiers.ReasoningBudget), `stream.go` (SetReasoningBudget) and
> **`budget_squeezer.go` (name gate)** into ONE typed table plus ONE resolver strategy.
> No per-provider wire logic lives anywhere else.

Reasoning was consolidated; budgeting was left behind. That is precisely why three
disagreeing "is it local?" predicates now exist (M8). This plan gives budgeting the same
treatment, using the same in-repo conventions: typed enums so invalid combinations are
unrepresentable, one table, one resolver, typed errors.

Second in-repo precedent: `automation/strategies.go:11` — `ExecutionStrategy` interface
plus a `NewStrategy` factory.

### 2.2 `WorkloadClass` — one classifier, three signals

Today three predicates disagree about what "local" means:

| # | Predicate | Location | Signal |
|---|---|---|---|
| 1 | `isLocalWorkload(cfg)` | `budget_squeezer.go:169-177` | `provider=="local"` OR `.gguf` suffix |
| 2 | `IsLocalModelURL(baseURL, modelHost)` | `proxy/client.go:109-129` | URL match; used by `bootstrap.go:90` + reasoning path |
| 3 | `cfg.Provider == "local"` | `budget_squeezer.go` `ResolveICUWeight` | exact string only |

Unified value object in the leaf `models` package — typed, not a bool
(rule: *no boolean state flags*):

```go
type WorkloadClass string

const (
    WorkloadLocal WorkloadClass = "local"
    WorkloadCloud WorkloadClass = "cloud"
)
```

```go
type WorkloadClassifier struct {
    modelHost          string
    localInterfaceIPs  []net.IP   // enumerated once at bootstrap (net.Interfaces); no network at classify time
}

func NewWorkloadClassifier(modelHost string, localInterfaceIPs []net.IP) WorkloadClassifier
func (c WorkloadClassifier) Classify(cfg models.ModelConfig) models.WorkloadClass
func (c WorkloadClassifier) ClassifyEndpoint(rawURL string) bool   // reusable from bootstrap + reasoning path
```

```
Local ⟺ provider == "local"
      ∨ .gguf artifact (Filename | Path | Name)
      ∨ host(ProviderConfig.BaseURL) ∈ {localhost, 127.0.0.1, ::1, 0.0.0.0}
      ∨ host(ProviderConfig.BaseURL) matches modelHost        // existing IsLocalModelURL semantics
      ∨ host(ProviderConfig.BaseURL) ∈ localInterfaceIPs
      (no DNS / no network calls at classify time)
```

**Single authority — budget, ICU, and reasoning wire all key on `WorkloadClass`.** The
reasoning-wire local override (`assistant/reasoning_param.go:177-184`) currently sniffs
`client.ReasoningField()` set from `proxy.IsLocalModelURL` (`bootstrap.go:90`); that sniff is
replaced by `cfg.WorkloadClass == WorkloadLocal`. A loopback-URL `openai` model can no longer
pick the local budget policy while sending `reasoning_effort` (cloud) — the classifier drives
both. `proxy.IsLocalModelURL` becomes a thin delegate to `ClassifyEndpoint` so
`proxy/client_test.go:437-479` keeps passing.

**Local-first invariant.** Any workload that reaches a local serving endpoint must use
the local policy, including `provider: openai` with a local URL and a non-`.gguf` model
name. This is validated by the characterization matrix in §3.1. The effective endpoint
must be resolved before policy selection; a missing endpoint must not silently select a
cloud policy.

**Storage:** resolved at the runtime boundary after provider configuration and secret
hydration, then stored as a computed typed field on `models.ModelConfig`. The resolver
must also be callable for add/update/admin-view paths, because those paths calculate
defaults before the model is inserted into the runtime manager. No caller may infer
workload class from `Provider` alone.

The effective endpoint is an input to classification. It must come from the same
hydrated source used for inference, including provider configuration and per-credential
base-URL overrides. Reading only the persisted `ModelConfig.ProviderConfig.BaseURL` is
insufficient.

**Persistence:** computed-only. Must carry `json:"-"` on the persistence path — a stored
value goes stale the moment a base URL changes. The split already exists because the
catalogue entry type (`models/registry.go:30`) is distinct from `ModelConfig`; confirm at
implementation time.

**Exposed on the read DTO** (`workload_class`) so the frontend field policy (§2.7) reads
the same server-computed value. New-model forms additionally use a pure draft
classifier over the entered effective URL; they must not invent budget values. The
backend remains authoritative after save.

### 2.3 `BudgetPolicy` — strategy, selected by class

```go
// Immutable value object.
type Budget struct{ MaxTokens, ContextBudget int }

// Small interface, per go-staff-engineer rules.
type BudgetPolicy interface {
    Derive(cfg models.ModelConfig, context ContextResolution) (Budget, error)
}
```

| Implementation | Content |
|---|---|
| `LocalBudgetPolicy` | **verbatim copy of today's body** — `ctx/3`, `(ctx-mt)*2`; requires a resolved serving context. No cloud fallback. |
| `CloudBudgetPolicy` | new — §2.4; uses provider defaults and published capability limits. |

Selected via a map keyed on `WorkloadClass`. A third class later is one table entry, not
a growing `if` chain.

### 2.4 Cloud policy — clamp-first, data-driven

Two parallel capability chains (see §2.10), first `known` wins each:

```go
OutputCapSource:    top_provider.max_completion_tokens
                    → max_completion_tokens
                    → provider tier MaxTokens   (data, from tuning.go; no global constant)
PublishedContextSource: top_provider.context_length
                    → context_length
                    → knownCtx heuristic
                    → provider tier default context
```

`CloudBudgetPolicy.Derive`:

```
maxTokens    = OutputCapSource.Get(cfg)              // published per-model cap, else tier row
publishedCtx = PublishedContextSource.Get(cfg)

if publishedCtx > 0:
    reserve = max(maxTokens, promptReserveGuess)
    if publishedCtx <= minViablySmallContext:
        return ErrCapabilityImpossible                // contradictory config — typed, actionable
    maxTokens = min(maxTokens, max(1, publishedCtx - reserve))   // CLAMP, do not fail
    contextBudget = max(minHistoryBudgetChars,
                        min(tierHistoryBudget, publishedCtx - maxTokens))
else:
    contextBudget = tierHistoryBudget                 // capless: nothing to clamp against
```

**Clamp-first, fail-only-when-impossible** (Hermes-adopted behaviour §9 #4/#7): a 4K-context
capless model **clamps and succeeds**, never bricks the agent. The typed
`ErrCapabilityImpossible` fires only on contradictory config (`publishedCtx ≤
minViablySmallContext`, i.e. the window cannot fit any viable prompt reserve). This
reconciles §2.4 (typed error), §5 ("constrained below capacity, succeeds"), and §9 #4 (clamp).

`max_tokens` is a **ceiling, not an allocation** — the model stops at its own stop token.
The per-tier row lives in `models/tuning.go` (§2.10), **not a global constant**, so a
provider known to support larger outputs raises its row independently. The chain still
clamps to `min(published_cap, tier_value)`, so a per-tier raise never exceeds what a model
publishes. Local workloads never use any of this — they use `LocalBudgetPolicy` (§3.3).

New file-top constants in `budget_policy.go`: `promptReserveGuess`,
`minHistoryBudgetChars`, `minViablySmallContext`. No hardcoded per-model values anywhere —
models read their numbers from the live catalog (§2.10).

Resulting defaults:

| Provider | Today | After | Cap source |
|---|---|---|---|
| nvidia | 42,666 | tier row (8192) | tier — nothing published |
| openrouter | 42,666 | `min(published, tier row)` | live catalog (§2.10) |
| gemini | 349,525 | tier row (8192) | tier — see below |
| openai (cloud) | 42,666 | tier row (8192) | tier |
| **openai (`.gguf`)** | **ctx/3** | **ctx/3 — UNCHANGED** | local |
| **openai (local URL)** | **ctx/3** | **ctx/3 — UNCHANGED** | local |
| **local** | **ctx/3** | **ctx/3 — UNCHANGED** | local |

**Gemini is not an openai-compatible archetype.** `definitions/gemini.json` declares
`"archetype": "gemini"`, served by `provider_gemini.go`, whose `ListModels` (`:41-66`)
decodes only `models[].name`. §2.6 parsing does not reach it; it lands permanently on the
8192 default. Intended.

### 2.5 Why 8192, and the clamp

`max_tokens` is a **ceiling, not an allocation** — the model stops at its own stop token.

Measured risk, from the live OpenRouter catalogue (294 models publish a cap):

```
cap < 2048 ..........   1   0.3%
2048–4095 ...........   8   2.7%
4096–8191 ...........  17   5.8%
exactly 8192 ........  10   3.4%
> 8192 .............. 258  87.8%

would 400 on a flat max_tokens=8192:  26 models (8.8%)
would 400 on a flat max_tokens=4096:   9 models (3.1%)
```

Not obscure models: `openai/gpt-4-turbo` (4096), `openai/gpt-4o-2024-05-13` (4096),
`google/gemma-2-27b-it` (2048), the `cohere/command-r` family (4000).

**The clamp is what makes 8192 safe**, not the constant itself. Where a cap is published
we never exceed it. 8192 is chosen because it is the value best matched to the coupled
safety constants of §1.8 — a 32,768-char content cap (~8,000 words) and stuck detection
that actually fires. 4096 halves the report ceiling for a marginal risk reduction on the
unknown set only.

Residual exposure after clamping: NVIDIA (102 models, nothing published), Gemini, OpenAI
cloud, and OpenRouter's 42 capless models. Handled by §2.6.

### 2.6 `OutputCapSource` — chain of responsibility

```go
type OutputCapSource interface {
    OutputCap(cfg models.ModelConfig) (tokens int, known bool)
}
```

Ordered chain, first `known` wins:

Ordered chain, first `known` wins:

1. `PublishedMetadataCap` — from `top_provider.max_completion_tokens` / `max_completion_tokens`
   (extracted via the alias list, §2.10)
2. `ProviderTierCap` — the per-provider row in `tuning.go` (data, not a global constant)
3. *(extensibility)* `LearnedCapFrom400` — a cap recovered from a parsed 400
   (`output_cap_error.go`), cached per provider/model so subsequent calls clamp without
   retrying (Hermes §9 #5/#6; slot in without touching `CloudBudgetPolicy`)

A future probe or a cap learned from a 400 slots in as another element. This is the
designed extensibility point — `CloudBudgetPolicy` never grows a provider switch.

**Typed error at the edge:**

```go
type OutputCapError struct{ Requested, Available int }
func (e *OutputCapError) Error() string
var ErrOutputCapExceeded = errors.New("output cap exceeded")
```

On the rule *"never parse error strings when structured data exists"*: for output-cap
400s no structured data exists — providers return prose. Parsing is therefore
unavoidable, but it is confined to **one infrastructure-edge file**, converts to a typed
error immediately, and the domain never sees a string. It is also the *last* resort,
behind the published-cap path.

Note our current behaviour differs from Hermes': `IsRetryableHTTPStatus`
(`proxy/client.go:187-199`) does not retry 400 at all, so we hard-fail rather than
death-loop. Safer than their pre-fix state, but with an opaque message.

### 2.7 Frontend mirror

Same shape so the UI does not sprawl either: one `ModelTuningFields.vue` plus a
`useTuningFieldPolicy(workloadClass)` composable returning field descriptors
(`editable` | `derived`). Collapses both the 4× duplication and the local/cloud branching
into one policy lookup, driven by the server-computed `workload_class`.

### 2.8 Constitution I.1 / I.2 compliance

I.1 forbids `http.DefaultClient`; agent-tool traffic uses the `NetworkTools` abstraction
(`internal/core/tools/network.go:22-29`). Provider infrastructure is a separate traffic
class: it uses the dedicated pooled client below and is covered by the Phase 2 Constitution
I.2 amendment. `network.HTTPClient()` must not be reused for providers because it blocks
loopback and applies agent-tool guardrails to provider calls.

`providers` must not take a lateral dependency on `internal/core/tools`. Consumer-defined
small interface instead:

```go
// providers package
type HTTPDoer interface{ Do(*http.Request) (*http.Response, error) }
```

Bootstrap injects `&http.Client{Transport: proxy.SharedTransport, Timeout: 45 * time.Second}`
through `ProviderRegistrar` into the factories. This keeps providers unit-testable with a
stub and avoids agent-tool guardrails. Threads one parameter through `factories.go` —
contained but real.

### 2.9 Verified non-impact: reasoning budget

`DefaultReasoningBudget(maxTokens) = maxTokens / 3` (`agent.go:47-52`) appears to couple
reasoning to the value being changed. It does not, for cloud:

```go
// resolveReasoningSpec, agent.go:66-79
spec := tier.Reasoning
if spec.Mode == ModeThinkTokens {   // local only
    spec.Budget = DefaultReasoningBudget(maxTokens)
}
```

Only `ModeThinkTokens` (local) consumes it. Cloud tiers use `ModeEffort`, `ModeObject`, or
`ModeEnableThinking` and never receive a numeric budget — asserted by
`agent_test.go:4055-4060`. Local `max_tokens` is unchanged, therefore **no reasoning
budget changes anywhere**.

`docs/SPECS/orchestrator.md` §4 and `docs/architecture.md:264` describe this coupling
without the local-only qualifier — imprecise today, wrong after this change. Phase 6.

### 2.10 Data-driven provider setup (no per-model hardcoding)

**Goal:** OpenRouter's 336 models, NVIDIA's 102, OpenAI's catalogue — none are hardcoded
in the proxy. Adding a cloud provider or a model on an existing provider requires zero
code branches. This is the production pattern verified at
`/Users/mikeathan/dev/hermes-agent` (`agent/model_metadata.py`, `providers/base.py`) and
adapted to this codebase's existing manifest system.

Five mechanisms, all data-driven:

**1. Declarative provider manifests (already in repo).**
`internal/core/llm/providers/definitions/{provider}.json` carries `id`, `archetype`,
`default_base_url`, `endpoints.{models,chat}`, `auth`. Adding a cloud provider = adding one
JSON file + (only if a new transport archetype) a factory row in `factories.go`. **No
per-model entries anywhere.** Phase 0's vertex/mulerouter removal is manifest deletion, not
code surgery.

**2. Capability extraction via key-alias lists, not hardcoded field names.**
Hermes (`model_metadata.py:518-537`, `_extract_first_int` + `_iter_nested_dicts`) walks
nested JSON trying an ordered alias tuple, so one parser works across OpenRouter's
`top_provider.max_completion_tokens`, OpenAI's `max_completion_tokens`, NVIDIA's
`limits.context`, llama.cpp's `meta.n_ctx`, LM Studio's
`loaded_instances[].config.context_length`, DeepInfra's `metadata.pricing.input_tokens`.

```go
// internal/core/llm/providers/capability_keys.go (new)
var ContextLengthKeys = []string{
    "context_length", "context_window", "context_size", "max_context_length",
    "max_position_embeddings", "max_model_len", "max_input_tokens",
    "max_sequence_length", "n_ctx_train", "n_ctx", "ctx_size",
}
var OutputCapKeys = []string{"max_completion_tokens", "max_output_tokens", "max_tokens"}

// extractCapability(payload, keys) walks nested dicts, lowercases keys,
// coerces with a reasonable-int range, returns first known.
```

New provider JSON shape = add an alias to the slice, not a parser branch. `ListModels`
becomes "fetch + extract via aliases," replacing the bespoke `provider_openai_compatible.go:85-92` decoder.

**3. Live catalog fetch + cache.**
`ListModels` already exists per provider. Extend it to extract capabilities via the alias
lists and **cache** the result (in-memory TTL ~1h + on-disk under the data dir). OpenRouter's
full catalog is read once and cached; the proxy never hardcodes any OpenRouter/NVIDIA model.
The pinned 8192 lives in the **provider tier row** (`tuning.go`), not per-model.

**4. Local server probes for M8 (not filename guessing).**
For endpoints classified `WorkloadLocal` whose model name is not `.gguf`, recover the real
serving context by probing the server — Hermes `detect_local_server_type` (`:836-936`):
llama.cpp `/v1/props`→`/props` (`default_generation_settings.n_ctx`), LM Studio
`/api/v1/models` (`loaded_instances[].config.context_length`), vLLM `/version`. This is the
data-driven counterpart to the §3.4 `/slots` gate: a non-`.gguf` local model gets its `n_ctx`
from the server, never from a name heuristic and never from `providerCtxDefaults`. Reuses the
injected `HTTPDoer` (§2.8) with a short child context.

**5. WorkloadClass by effective endpoint URL (not config slug).**
`WorkloadClassifier.ClassifyEndpoint` host-matches against `modelHost` + loopback + cached
local-interface IPs. Provider slug is irrelevant to budget/reasoning/context selection — an
`openai`-slugged model on a local URL is `WorkloadLocal`, full stop. This mirrors Hermes'
`_URL_TO_PROVIDER` host→provider detection (`model_metadata.py:569-611`) but generalised to
local-vs-cloud.

**Tier table is per-provider (≈5 rows), not per-model.** `models/tuning.go` carries
provider-level defaults: `MaxTokens` (the per-tier output cap, e.g. 8192 for cloud tiers;
raise per provider when known), `ContextBudget` (history budget), `MaxSteps`, reasoning mode.
Models get their real numbers from the live catalog (`OutputCapSource`/`PublishedContextSource`
chains) when published; the tier row is the fallback when not. DeepSeek-v4-flash's 384K cap is
read from the OpenRouter catalog, not hardcoded. A provider known to support larger outputs
raises its row; the chain still clamps to `min(published, tier)` so a raise never exceeds what
a model publishes.

**Net:** adding a model on OpenRouter/NVIDIA = nothing in the proxy (the catalog provides it).
Adding a cloud provider = one manifest JSON + one tier row + (optionally) alias additions. No
growing per-model switch anywhere.

---

## 3. Local-safety strategy

This section is the mechanism behind §0. It is not optional and it is not a formality.

### 3.1 Characterization goldens (Phase A, lands first)

A committed golden **snapshot file** — not inline assertions — capturing today's
`ApplyMetadataDefaults` output for:

- every model in `backend/data/registry.json` (6 local `.gguf` under `openai`, 3 nvidia)
- `provider: local` with and without `Metadata.Nctx`
- `provider: openai` + `.gguf` name + `Nctx` present
- `provider: openai` + local base URL + **non-`.gguf`** name ← the M8 case
- `provider: openai` + remote base URL + non-`.gguf` name (genuine cloud)
- each cloud provider with metadata absent / present / inflated (>128K)

Recorded fields: `{MaxTokens, ContextBudget, ToolCallFormat, ResolveICUWeight}`.
The harness calls `orchestrator.ResolveICUWeight(cfg)` explicitly (it is not set by
`ApplyMetadataDefaults`), so M9's ledger change is captured in the committed snapshot. The
"without `Metadata.Nctx`" local row carries `ContextLength` (priority-2), so it resolves to a
**numeric** local default and is byte-identical — never a typed error.

**Why a file and not assertions:** the snapshot's own diff becomes the behaviour-change
audit. A changed local line is a visible diff line in review. This is what permits Phase
B and C to be merged without losing the guarantee.

### 3.2 Local-context safety property test

```
∀ cfg:  effectiveEndpoint(cfg) is local  ⟹  Classify(cfg) == WorkloadLocal
                                            ∧ ReasoningWire(cfg) == thinking_budget_tokens   // Fix 1

∀ local cfg:  context resolution fails  ⟹  no cloud provider default is applied
                                      ∧  LocalBudgetPolicy returns the numeric local default
                                         (defaultLocalContextLength), never a typed error on
                                         the runtime path, never providerCtxDefaults[provider]
```

Proves that local endpoint detection always wins over the persisted provider label, that the
reasoning wire follows the same classifier (Fix 1), and that an unavailable local serving
context cannot leak into a 128K/1M cloud calculation (Fix 2 — no `providerCtxDefaults` lookup
in the local path; `defaultLocalContextLength` is the universal local fallback, keyed on
workload not provider).

### 3.3 Verbatim local move — numeric, workload-scoped

`LocalBudgetPolicy.Derive` is a copy-paste of the existing local math (`ctx/3`,
`(ctx-mt)*2`). Its context resolution is **workload-scoped, not provider-scoped** — it must
NOT consult `providerCtxDefaults[cfg.Provider]`:

```
LocalBudgetPolicy.resolveContext(cfg):           /* never providerCtxDefaults */
  1. Metadata.Nctx                               (serving context, /slots or /v1/props)
  2. Metadata.ContextLength   capped by defaultLocalContextMax (1_048_576)
  3. defaultLocalContextLength (8192)            /* the universal local fallback */
```

For an `openai`+local-URL+no-metadata model this yields 8192 → ctx/3 = 2730 — byte-identical
to today's `provider: local` golden, **never** the M8 128K leak. `Derive` returns the
**numeric** local default when nothing resolves — so the golden stays bytewise intact. A local
workload is never sent with a guessed cloud number, and the runtime path never returns a typed
unresolved-context error.

The typed **unresolved-context error is registration-edge only** (`handleAddModel` for a
newly added local model with no metadata and no server probe yet): emit an admin warning and
kick a `/slots` (or `/v1/props`, §2.10 #4) probe — never hard-reject across the golden. This
reconciles "verbatim move" with "typed error": the error lives at the edge, `Derive` is
numeric. A model that resolves via priority 2 or 3 never reaches the error.

`WorkloadClass` is the budget/ICU classification. It must not silently change unrelated wire
behavior. Tool-call format remains governed by its existing explicit/default resolution until
a separate, characterized protocol change is approved. This prevents the newly recognized
`openai` + local-URL case from changing native/XML behavior as an incidental consequence of
budget refactoring.

### 3.4 The `/slots` gate must key on URL, not filename

The original draft of this plan proposed gating `fetchSlotsContext` on `isLocalWorkload`.
**That would have broken local metadata discovery** for any local model whose name is not
`.gguf`: no `/slots` probe → no `n_ctx` → `resolveContextLength` falls to
`providerCtxDefaults["openai"] = 128_000`.

Correct gate:

```
probe /slots (or /v1/props for llama.cpp, /api/v1/models for LM Studio)  ⟺  effectiveEndpoint is local
```

Probes any local llama.cpp/LM Studio regardless of model name; still skips cloud URLs,
preserving the wasted-calls fix. For non-`.gguf` local models (M8), the server probe — not
`providerCtxDefaults` — recovers the real serving `n_ctx` (§2.10 #4). The probe reuses the
injected `HTTPDoer` with a 5s child context.

Context on how current local models obtained `n_ctx: 8192`: they are added through the
**cloud** UI path (provider ≠ `local` tab), which forwards `meta`
(`frontend/src/composables/settings/useProviderModels.ts:199-201`) after `/slots` populated it. That path must keep working.

### 3.5 Merged Phase B+C — conditions

Merging the refactor with the rule change is permitted **only** under all three:

1. Phase A lands and passes on its own, first.
2. Goldens are a committed snapshot file (§3.1).
3. `LocalBudgetPolicy` is a verbatim move (§3.3).

Without condition 2 the merge is unsafe — a local regression would hide inside a large
mechanical diff.

---

## 4. Phases

| # | Phase | Behaviour change | Gate |
|---|---|---|---|
| A | Characterization goldens | none | must pass before anything moves |
| 0 | Deprecate or safely remove vertex + mulerouter | none for supported providers | unknown-provider safety tests |
| B+C | effective endpoint resolution, `WorkloadClass`, classifier, `BudgetPolicy` strategies, cloud policy, workload-aware metadata enrichment, admin DTOs | **local serving context and local math unchanged; cloud behavior changes** | snapshot + local-context safety tests |
| 2 | Provider schema parsing, `/slots` effective-URL gate, dedicated infrastructure `HTTPDoer`, 5s probe timeout, Constitution I.2 amendment | removes cloud probes and restores local discovery | infrastructure-client tests + Constitution review |
| 3 | Persist explicit overrides | fixes silent discard | round-trip test |
| 4 | Frontend extract + field policy | UI only | `npm run build` |
| 5 | Loose ends X2 / X3 / X4 | cleanup | — |
| 6 | SPEC-005 amendment + doc corrections | docs | stewardship |

### Phase A — Characterization goldens

New test + committed snapshot per §3.1. No production code touched.

### Phase 0 — Deprecate or safely remove `vertex` and `mulerouter`

The repository fixture does not use either provider, but installed user registries may.
Do not infer migration safety from the fixture. First make unknown providers fail closed;
then either retain deprecated manifests for loading existing entries or add an explicit
registry migration that reports affected models and credentials. Never allow removal to
trigger the current unknown-provider fallback to a local provider.

**Backend**

| File | Change |
|---|---|
| `internal/core/llm/providers/provider_vertex.go` | delete only after deprecated-provider migration/retention decision |
| `internal/core/llm/providers/definitions/vertex.json` | retain as deprecated manifest or delete after migration |
| `internal/core/llm/providers/definitions/mulerouter.json` | retain as deprecated manifest or delete after migration |
| `models/provider_manifest.go:20` | remove `ArchetypeVertex` |
| `models/provider_manifest.go:6-7` | comment refs |
| `internal/core/llm/providers/factories.go:16-17` | remove vertex factory |
| `internal/core/assistant/agent.go:110,113` | remove tiers |
| `internal/core/assistant/reasoning_param.go:163,166` | remove tier entries |
| `internal/core/assistant/reasoning_param.go:28` | comment ref |
| `internal/core/orchestrator/budget_squeezer.go:187,189` | remove ctx defaults |
| `models/llm_messages.go:52` | comment ref |
| `internal/core/orchestrator/budget_squeezer_test.go:507-513` | remove cases |
| `internal/core/assistant/agent_test.go:3986,4055` | drop from lists |
| `internal/core/llm/providers/provider_cloud_test.go:16-40` | rewrite against `nvidia`/`openrouter` |

**Frontend**

| File | Change |
|---|---|
| `src/constants/providers.ts:7,9,23,25,39,41,55,57,67,69` | remove from all 5 maps |
| `src/types/admin.ts:2` | `ProviderType` union |
| `src/composables/models/useConfig.ts:68,71` | remove shadow tier entries |
| `src/composables/models/useProviders.ts:32,35` | remove `staticProviders` vertex |
| `src/components/settings/Settings.vue:134` | `openAICompatibleProviders` |
| `src/components/settings/Settings.vue:300-355` | Vertex project_id / region panel |
| `src/components/models/CloudFields.vue:42` | transitively dead through dead `ModelManager.vue` — Phase 5 |
| `src/components/settings/ModelCatalogue.vue:135` | dead file — Phase 5 |

**Leave alone:** `internal/platform/network/address_test.go:18` — `vertex.local` is an
unrelated hostname fixture.

**M7:** `secrets.json` is AES-256-GCM encrypted (Constitution III.6), so we cannot confirm
whether vertex/mulerouter credentials exist. Removal must tolerate orphaned credentials,
preserve them until explicit user deletion, and report orphaned models/credentials. Note
III.7 cascades key deletion to models.

### Phase B+C — Classification and budget policies

| File | Change |
|---|---|
| `models/workload.go` (new) | `WorkloadClass`; `WorkloadClassifier{modelHost, localInterfaceIPs}` + `Classify` + `ClassifyEndpoint`; pure, no network |
| `models/config.go` | computed `WorkloadClass` and published capability fields, excluded from persistence where derived (`json:"-"`, `yaml:"-"`) |
| `models/tuning.go` (new) | immutable **per-provider** tuning defaults (MaxTokens cap, history budget, MaxSteps, reasoning spec); leaf package, no core imports; the only place a default cloud output cap lives (data, not a global constant) |
| `internal/core/llm/providers/capability_keys.go` (new) | key-alias slices (`ContextLengthKeys`, `OutputCapKeys`) + `extractCapability(payload, keys)` walker (§2.10 #2) |
| `internal/core/orchestrator/context_resolution.go` | `ContextResolution`; `PublishedContextSource` chain (top_provider → top-level → knownCtx → tier default); LocalBudgetPolicy uses priorities 1/2/3 + `defaultLocalContextLength`/`…Max`, **never** `providerCtxDefaults` (Fix 2) |
| `internal/core/orchestrator/workload_classifier.go` (new) | pure classifier over provider, artifact, effective endpoint host, modelHost, local-interface IPs |
| `internal/core/orchestrator/budget_policy.go` (new) | `Budget`, `BudgetPolicy`; `LocalBudgetPolicy` (verbatim local math, numeric-only — no typed error on the runtime path); `CloudBudgetPolicy` clamp-first + `ErrCapabilityImpossible` only when `publishedCtx ≤ minViablySmallContext`; `minHistoryBudgetChars` floor |
| `internal/core/orchestrator/output_cap.go` (new) | `OutputCapSource` chain (published → tier → learned-from-400); no HTTP/error-string parsing |
| `internal/core/orchestrator/budget_squeezer.go` | thin compatibility entry point; resolves effective workload/context, selects policy, applies result; no provider branches; deletes `providerCtxDefaults`/`knownCtx` (moved to `context_resolution.go` + `tuning.go`) |
| `internal/core/llm/manager.go` | resolve hydrated workload/context during `Sync()` on a deep-copied `cfg`; resolver accesses registrar/secrets only, never re-enters manager locks |
| `internal/core/llm/providers/registrar.go` | expose one effective provider endpoint resolver (mirror of `Build()` secret-hydration steps 1–2, no provider constructed) for runtime and classification |
| `internal/core/assistant/reasoning_param.go:170-184` | **Fix 1** — `NewReasoningResolver(workload models.WorkloadClass, providerType string, configuredBudget int)`; local override ⟺ `workload == WorkloadLocal`, drop the `client.ReasoningField()` sniff |
| `internal/core/assistant/stream.go:76` | pass `a.config.WorkloadClass` to `NewReasoningResolver` |
| `internal/core/assistant/agent.go` | `AgentConfig.WorkloadClass` + `AgentOptions.WorkloadClass`; `ApplyModelConfig` propagates `cfg.WorkloadClass`; compose shared tuning defaults, do not duplicate budget formulas |
| `internal/core/proxy/client.go:103-129` | `IsLocalModelURL` → thin delegate to `models.ClassifyEndpoint` (keeps `client_test.go:437-479` green); `ReasoningField()` becomes a derived diagnostic only |
| `internal/app/bootstrap.go:85-103` | build `WorkloadClassifier` once (modelHost + cached `net.Interfaces()` IPs); replace `proxy.IsLocalModelURL(baseURL, …)` with `classifier.ClassifyEndpoint(baseURL)` in the client factory (Fix 1 unification) |
| `internal/transport/http/handlers/model_handlers.go` | make metadata enrichment workload-aware; local workloads never receive cloud default zeroing or pricing-derived tuning |
| `internal/transport/http/handlers/admin_view.go` | populate computed `workload_class`; preserve the GGUF scan call site (3rd `ApplyMetadataDefaults` caller) |
| `internal/transport/http/handlers/admin_handlers.go` | expose `workload_class` on the read DTO (`adminModelView`); migrate `convertProviderTiers`/`ProviderTiers()` to the leaf table |
| `internal/core/assistant/reasoning_param_test.go:107-146` | update `NewReasoningResolver` signature; add `WorkloadLocal`+`"openai"`+loopback-URL → `thinking_budget_tokens` case |

Each new production file must have one primary responsibility and remain below roughly
300 lines. Prefer extending `context_resolution.go` and `budget_policy.go` over creating
single-function files. Keep transport DTOs in handlers and domain values in `models`.

`enrichMetadataFromProviders` must classify before applying metadata behavior. Local workloads
preserve serving `n_ctx`, skip pricing-derived ICU changes, and ignore submitted derived
budget fields. Cloud workloads retain provider metadata enrichment and published-cap parsing.
`adminModelView` is the read DTO for `workload_class`; the computed field on `ModelConfig`
remains non-persistent (`json:"-"`, `yaml:"-"`).

**Import-cycle constraint:** `assistant` imports `orchestrator` (`agent_builder.go`,
`conversation_service.go`, `stream.go`, `agent.go`) — verified one-way. `orchestrator`
cannot import `assistant`, which is why the numeric tier table moves to leaf `models`.

**Complexity gate:** split into helpers so each stays under 12
(`go run ./tools/check-complexity/`). The local helper must be a verbatim move.

**Named constants** (rule: no hardcoded values in logic), file-top:
`defaultLocalContextLength = 8192`, `defaultLocalContextMax = 1_048_576` (in
`budget_policy.go`, used only by `LocalBudgetPolicy`); `promptReserveGuess`,
`minHistoryBudgetChars`, `minViablySmallContext` (in `budget_policy.go`, used only by
`CloudBudgetPolicy`). Existing local divisors (the `ctx/3` and `(ctx-mt)*2`) move verbatim into
`LocalBudgetPolicy.Derive`. **No global `defaultCloudOutputCap`** — the cloud output cap lives
as the per-provider `MaxTokens` row in `models/tuning.go` (data-driven, §2.10 #5). Cloud tier
`MaxTokens` for new-model prefill matches the policy value; local prefill remains 2048.

### Phase 2 — Parse what providers publish

| File | Change |
|---|---|
| `providers/provider_openai_compatible.go:85-92` | add `context_length`, `top_provider.{context_length,max_completion_tokens}` |
| `models/llm.go:32-43` | add explicit `ContextLength` and `MaxOutputTokens` capability fields; do not overload llama-specific `ModelMeta` names |
| `models/config.go:397-401` | carry `MaxOutputTokens` into `ModelMetadata` |
| `handlers/model_handlers.go:163-232` | propagate published cap through `enrichMetadataFromProviders` |
| `providers/provider_openai_compatible.go:113` | **gate `/slots` on the effective local endpoint** (§3.4); use a 5-second child context |
| `providers/provider_openai_compatible.go:75,153` | injected `HTTPDoer` (§2.8) |
| `providers/provider_gemini.go:47` | injected `HTTPDoer` |
| `providers/factories.go`, `registrar.go`, `app/bootstrap.go` | thread the dedicated `proxy.SharedTransport`-based doer; do not use `networkTools.HTTPClient()` |
| `providers/output_cap_error.go` (new) | provider-edge classifier/parser → typed capability error (§2.6), kept below 300 lines |
| `internal/core/proxy/client.go` | convert recognized output-cap responses into the typed error; never retry them and never parse them outside the edge helper |

Gemini gets only the HTTP-client fix; parsing does not reach it (§2.4).

### Phase 3 — Make overrides persist

| File | Change |
|---|---|
| `handlers/model_handlers.go:252-259` | distinguish explicit cloud values from derived values using the submitted form plus resolved baseline |
| `handlers/model_handlers.go:272-287` | persist explicit cloud overrides only; never persist local derived values |
| `handlers/model_handlers.go` add/update | for `WorkloadLocal`, reject/ignore submitted budget fields and remove stale persisted budget overrides |
| `internal/core/llm/manager.go` | prevent stale local `MaxTokens`/`ContextBudget` overrides from being reapplied |
| `models/infrastructure.go` | document zero-value semantics; use explicit map-entry deletion for reset, not zero-value guessing |

Resetting a field must delete that field's persisted override without deleting unrelated
model tuning. If the current flat `ModelOverride` shape cannot represent field deletion,
add a targeted clear operation in the handler rather than introducing nullable fields for
every tuning value.

The "differs from baseline" condition preserves the original intent of the exclusion
comment (don't freeze stale derived values) while making deliberate edits stick.
Constitution III.5 keeps these in `settings.yml`, never `registry.json`.

### Phase 4 — UI

- **4a** Extract `ModelTuningFields.vue` **first** — block duplicated 4× in
  `ProviderModelsCard.vue` (`:118-254`, `:341-477`, `:518-654`, `:694-830`).
- **4b** `useTuningFieldPolicy(workloadClass)` returning `editable` | `derived`
  descriptors, driven by the server-computed `workload_class` (§2.2). For unsaved models,
  classify only the draft endpoint and show unresolved state when endpoint/context is
  unknown.
- **4c** Local → readonly with `.form-input--readonly` (`CommunicationSettings.vue:194-195`,
  style `:273-275`), `InfoTooltip` explaining `n_ctx` derivation, "derived" pill reusing
  `.pill` (`ProviderModelsCard.vue:1190-1208`).
- **4d** Cloud → editable, prefilled from backend, helper text distinguishing output cap
  from context window; reject values above a published cap instead of only flagging them.
- **4e** Delete `computeDefaultsFromContext` (`modelUtils.ts:104-113`) and its caller
  (`useProviderModels.ts:107-121`) — kills the `/3` vs `/4` mismatch.
- **4f** Preserve the cloud-tab metadata forwarding path (`useProviderModels.ts:199-201`).
  The local-add path (`:224-240`) does not currently forward provider metadata. *Scope
  (M6):* every local model in `registry.json:12-110` already carries `metadata.n_ctx`, so
  **existing models are unaffected**; this only changes models added from now on. Because local
  `max_tokens` feeds `DefaultReasoningBudget` (§2.9), a newly added local model gets a
  proportionally larger reasoning budget — intended.
- **4g** Drop the shadow provider tier table (`useConfig.ts:65-73`); keep the
  `provider_defaults` fetch. `useModels.ts:15-30` is a flat `agent_defaults` fallback, not
  a provider tier table. Cloud prefill must receive the policy-aligned 8192 default.

### Phase 5 — Loose ends

| ID | Issue | Location |
|---|---|---|
| X2 | `slot_timeout` hardcoded to `0` on create, no live UI, absent from `AgentDefaults` | `modelUtils.ts:136`, `types/admin.ts:113-128` |
| X3 | Frontend hardcodes `tool_call_format: "xml"` for local; backend expects `""` | `modelUtils.ts:32-37` vs `budget_squeezer.go:162-164` |
| X4 | Dead model forms; `ModelManager.vue` transitively imports `CloudFields.vue` and other dead model components; these hold the only `vertex`/`slot_timeout` UI | `components/ModelManager.vue`, `components/models/*`, `components/settings/ModelCatalogue.vue` |

### Phase 6 — SPEC and docs

**SPEC-005 is `stable`.** Both the tier table and the derivation rule are contract, so
this amends it. Per `docs/SPEC-change-management.md` § "Changing a Stable SPEC": increment
`version`, add a changelog entry at the top, update `last_updated`. Corrective, not
breaking — no superseding SPEC needed.

| File | Change |
|---|---|
| `docs/SPECS/orchestrator.md:42-52` | tier table: drop vertex/mulerouter; update MaxTokens column |
| `docs/SPECS/orchestrator.md:53-54` | document the local/cloud policy split |
| `docs/SPECS/orchestrator.md:63` | reasoning wire list |
| `docs/SPECS/orchestrator.md:68-70` | qualify `max_tokens ← ctxLen/3` as **local-only** |
| `docs/SPECS/orchestrator.md` frontmatter | version bump + changelog + `last_updated` |
| `docs/architecture.md:262` | reasoning param list |
| `docs/architecture.md:264` | correct the `ctxLen/3` claim to local-only |
| `docs/SPECS/agent-loop.md:171` | **stale ref** — cites `assistant/tiers.go`, which does not exist; table is `agent.go:107-115`, moving to `models/` |
| `README.md:20` | provider list mentions "Vertex AI" |
| `CONSTITUTION.md:I.2` | add provider-infrastructure carve-out: shared pooled client for inference, catalogue listing, `/slots`, and connection tests; agent-tool guardrails remain unchanged |
| `docs/INDEX.md:59`, `docs/PLANS/README.md:37` | already registered |

---

## 5. Tests

**Local (must pass unchanged — the §0 contract):**

- every existing local/GGUF case in `budget_squeezer_test.go`
- the Phase A golden snapshot, local entries byte-identical; fields include
  `MaxTokens`, `ContextBudget`, `ToolCallFormat`, and `ResolveICUWeight`
- local-context safety property (§3.2)
- `openai` provider + local URL + non-`.gguf` name → `WorkloadLocal`, `/slots`-derived `ctx/3` path
- local URL with unavailable `/slots` and no serving metadata → no cloud default and typed unresolved-context result
- `/slots` probed for that model; `n_ctx` reaches `ModelMetadata`
- local submission with bogus `max_tokens` → ignored, `n_ctx` math wins
- local URL supplied through a credential/provider base-URL override → same classification as inference
- `openai` local URL using `localhost`, `127.0.0.1`, `::1`, `0.0.0.0`, and a cached local-interface IP → `WorkloadLocal`
- **`WorkloadLocal` ⇒ reasoning wire `thinking_budget_tokens`** (V1 — budget class and reasoning wire share the classifier; never `reasoning_effort` for a loopback-URL `openai` model)
- **local no-metadata, no name heuristic → numeric `defaultLocalContextLength` budget (8192 → ctx/3 = 2730)**, never a typed error on the runtime path, never `providerCtxDefaults[provider]` (V2)
- **local with `ContextLength` only (no `Nctx`)** → priority-2 resolves, golden byte-identical
- local tool-call and reasoning behavior remains unchanged

**Cloud (new behaviour):**

| Test | Assertion |
|---|---|
| nvidia, no metadata | `max_tokens == 8192`, not 42666 |
| gemini, no metadata | `max_tokens == 8192`, not 349,525 |
| openrouter with published cap | `min(cap, 8192)` |
| openrouter cap < 8192 (e.g. 4096) | clamped to 4096, not 8192 |
| openrouter decode fixture | real payload shape parses |
| cloud override round-trip | explicit `max_tokens` survives `Sync()` |
| output-cap 400 | classified, typed `OutputCapError`, actionable message |
| output-cap 400 through `proxy/client.go` | typed error, no retry, no death-loop |
| published 4K context | cloud history budget is constrained below provider capacity |
| cloud context-budget defaults | policy uses the intended 20K–50K history tiers, not the old context/3-derived hundreds-of-thousands budget |
| explicit cloud output above published cap | rejected or clamped before request |
| small-context capless model (4K ctx, capless) | clamps to `max(1, ctx − reserve)` and **succeeds** (V3) |
| contradictory cap (`publishedCtx ≤ minViablySmallContext`) | typed `ErrCapabilityImpossible`, actionable message (V3) |
| override reset | only selected override is deleted; unrelated tuning remains |
| provider set | no `vertex` / `mulerouter` symbols remain |
| removed/unknown provider | typed failure; never local fallback |
| provider infrastructure client | uses pooled client, 45s overall timeout, and never inherits agent LAN/internet guardrails |
| `/slots` timeout/cancellation | probe exits within child deadline and does not leak goroutines |
| provider-default prefill | cloud new-model prefill matches the tier-row `MaxTokens` value |
| alias-list extraction (V5) | one `extractCapability` parses OpenRouter `top_provider.max_completion_tokens`, OpenAI `max_completion_tokens`, NVIDIA `limits.context`, llama.cpp `meta.n_ctx`, LM Studio `loaded_instances[].config.context_length` |
| live catalog cache (V5) | OpenRouter/NVIDIA catalog fetched once, cached (TTL), no per-model hardcoding |
| local server probe (M8 data-driven) | non-`.gguf` local model gets `n_ctx` from `/v1/props` (llama.cpp) or `/api/v1/models` (LM Studio), not `providerCtxDefaults` |
| tier MaxTokens raise (V4) | a tier row > 8192 is respected; published cap still wins (`min(published, tier)`) |

Table-driven throughout (rule). Gates:

```bash
cd backend && go build ./... && go test ./...
go run ./tools/check-complexity/          # ≤12
cd frontend && npm run build
```

Baseline recorded 2026-08-01: build OK, `internal/core/orchestrator` tests ok,
complexity clean.

## 5A. Implementation Quality Rules

These rules govern implementation of this plan:

- Keep domain values and capability structs in `models`; keep URL probing, HTTP, and
  provider response parsing at infrastructure edges.
- Keep `ApplyMetadataDefaults` thin. It resolves effective workload/context, selects a
  policy, and applies the result. It must not contain provider branches or duplicate
  formulas.
- Use small interfaces at boundaries: `EffectiveEndpointResolver`, `BudgetPolicy`,
  `OutputCapSource`, `PublishedContextSource`, and provider `HTTPDoer`. Consumers accept
  interfaces; constructors return concrete types.
- Prefer strategy maps keyed by typed `WorkloadClass` over growing provider switches.
  Provider-specific data belongs in tables (`models/tuning.go`), not scattered conditionals.
  Capabilities are read from the live catalog via alias lists (§2.10); no per-model entries
  hardcoded anywhere for providers with a `/models` endpoint (OpenRouter, NVIDIA, OpenAI).
- Use typed errors with `errors.Is`/`errors.As` for unresolved context, invalid
  capabilities, output-cap responses, and unknown providers. Preserve upstream errors with
  `%w`.
- Do not make network calls from pure classifiers or budget policies. The classifier takes
  cached inputs (modelHost, `localInterfaceIPs`); capability chains read cached catalog /
  model metadata. All network calls (catalog fetch, `/slots`, `/v1/props`, `/api/v1/models`)
  accept `context.Context`, use the injected `HTTPDoer`, and have cancellation/timeout tests.
- Keep one primary type/responsibility per file. Target under 300 lines per production
  file; split by domain concept, not arbitrary line ranges.
- Do not add compatibility shims or duplicate old/new formulas. Move the local formula
  once, preserve it verbatim, and delete the old implementation after tests pass.
- Do not log API keys, prompts, request bodies, or provider response payloads by default.
  Capability errors may include numeric limits and provider/model identifiers only.
- Update backend DTOs, frontend API types, composables, and components together when
  adding `workload_class`, context capability, or output-cap fields.
- Run backend build after each meaningful backend phase; run frontend build after each
  frontend phase; finish with tests, complexity checks, and documentation stewardship.

---

## 6. Findings register

| ID | Finding | Phase |
|---|---|---|
| P1 | Cloud models get the local `ctx/3` formula | B+C |
| P2 | Provider tier never applied at runtime | B+C |
| P3 | UI `max_tokens`/`context_budget` edits never persist | 3 |
| P4 | Frontend `ctx/4` vs backend `ctx/3` | 4e |
| P5 | OpenRouter's real schema not parsed (0/336 fields match) | 2 |
| P6 | `/slots` probed for cloud; 3 × Constitution I.1 violations | 2 |
| P7 | ICU reservations inflated ~5× | B+C (consequence) |
| P8 | `max_tokens` silently gates 3 agent-loop safety mechanisms | §1.8 |
| X1 | Frontend drops local GGUF metadata — **new models only** | 4f |
| X2 | Orphan `slot_timeout` | 5 |
| X3 | `tool_call_format: "xml"` mismatch | 5 |
| X4 | Dead model forms | 5 |
| D1 | Tier table duplicated 3× | B+C + 4g |
| D2 | Tuning form block duplicated 4× | 4a |

### Second-pass re-assessment

| ID | Finding | Phase |
|---|---|---|
| M1 | SPEC-005 + 4 docs encode old behaviour; SPEC is `stable`, needs versioned amendment | 6 |
| M2 | `DefaultReasoningBudget` coupling verified **local-only** — no cloud impact | §2.9 |
| M3 | Gemini is archetype `gemini`, not openai-compatible — parsing does not reach it | §2.4 |
| M4 | `provider_gemini.go:47` bare `http.Get` | 2 |
| M5 | `docs/SPECS/agent-loop.md:171` cites non-existent `assistant/tiers.go` | 6 |
| M6 | X1 scope overstated — existing registry models already carry `n_ctx` | 4f |
| M7 | `secrets.json` encrypted; removal must tolerate orphaned credentials | 0 |

### Third-pass — local-safety review

| ID | Finding | Phase |
|---|---|---|
| **M8** | **Three disagreeing local predicates.** `isLocalWorkload` (filename) vs `IsLocalModelURL` (URL) vs `Provider=="local"` (string). An `openai` model on a local URL with a non-`.gguf` name is invisible to the budget path. | B+C |
| **M9** | `ResolveICUWeight` gates on `Provider == "local"` only, so `openai`-provider GGUFs never get parameter-derived ICU weight (silently 1.0). Pre-existing. | B+C |
| **M10** | `stream.go:430` has no zero-guard (unlike `:921`), so `MaxTokens == 0` trips stuck detection on any reasoning content. Blocks any future "omit `max_tokens`" strategy until consumers are decoupled. | documented; not changed |
| **M11** | Original `/slots` gate proposal (filename-based) would have **broken local metadata discovery**. Corrected to URL-based. | §3.4 |

### Fourth-pass — solution review

| ID | Finding | Resolution / Phase |
|---|---|---|
| **C1** | `network.HTTPClient()` uses agent guardrails and unconditionally blocks loopback; injecting it into providers would break local `/slots` and couple cloud listing to agent internet settings. | Dedicated `proxy.SharedTransport`-based provider client with 45s timeout; 5s `/slots` child context; Constitution I.2 amendment. Phase 2 |
| **C2** | Exact `modelHost` matching misses `localhost`, loopback, `0.0.0.0`, and local LAN-interface addresses. | Classifier accepts loopback/unspecified hosts, configured model host, and cached local interface IPs. Reuse predicate for budget, ICU, and reasoning wire selection. B+C |
| **H1** | `enrichMetadataFromProviders` runs before policy selection and can apply cloud zeroing/prefill behavior to local URLs. | Make enrichment workload-aware; local path preserves serving `n_ctx`, ignores submitted budget fields, and skips pricing-derived tuning. B+C |
| **H2** | `adminModelView`, `getModelsView`, and `convertProviderTiers` were omitted from the implementation surface. | Add `workload_class` to `adminModelView`, preserve the GGUF scan call site, and migrate provider-default conversion. B+C |
| **H3** | Cloud runtime cap changes to 8192 while provider-default form prefill remains 2048/4096. | Set cloud tuning-table prefill `MaxTokens` to the policy value; test API prefill against policy. B+C |
| **H4** | Cloud `context_budget` drops from hundreds of thousands/millions of chars to 20K–50K, changing sieve behavior. | Treat as an explicit behavior change; add golden coverage and SPEC-005 documentation. B+C + 6 |
| **S1** | Golden snapshot omitted ICU weight, hiding the M9 ledger change. | Add `ResolveICUWeight` to golden fields; harness calls it explicitly. Phase A |
| **S2** | `/slots` probes had no short timeout. | 5-second child context and cancellation test. Phase 2 |
| **S3** | Classification during `Sync()` could create lock re-entry risk. | Deep-copy config; resolver may access registrar/secrets only and never manager. B+C |
| **S4** | `json:"-"` computed field conflicts with exposing `workload_class`. | Keep runtime field non-persistent; expose only through `adminModelView`. B+C |

### Fifth-pass — verify review (solution correctness)

| ID | Finding | Resolution / Phase |
|---|---|---|
| **V1** | Reasoning-wire path (`reasoning_param.go:177-184`) sniffs `client.ReasoningField()` set from `IsLocalModelURL` (`bootstrap.go:90`); C2's expanded classifier never reached it. A loopback-URL `openai` model would pick local budget but send `reasoning_effort` (cloud) → silent reasoning-budget drop. | `NewReasoningResolver(workload, providerType, configuredBudget)`; local override on `WorkloadClass` only. `bootstrap.go:90` uses `classifier.ClassifyEndpoint`. §2.2 + B+C file table |
| **V2** | "Verbatim copy" of `resolveContextLength` kept priority-4 `providerCtxDefaults[cfg.Provider]` → `openai`+local-URL+no-metadata → 42666 under the local policy = M8 reintroduced; also contradicted golden. | `LocalBudgetPolicy` is workload-scoped (priorities 1/2/3 + `defaultLocalContextLength`), never `providerCtxDefaults`; runtime path numeric-only; typed error registration-edge only. §3.3 |
| **V3** | §2.4 (typed error), §5 ("constrained, succeeds"), §9 #4 (clamp) disagreed on small-context capless models. | Clamp-first `CloudBudgetPolicy`: `ErrCapabilityImpossible` only when `publishedCtx ≤ minViablySmallContext`; otherwise clamp-and-succeed. §2.4 + §5 + §9 |
| **V4** | Output cap was a single global constant `defaultCloudOutputCap`; not agnostic to per-provider differences. | Per-tier `MaxTokens` row in `models/tuning.go`; chain clamps to `min(published, tier)`. §2.10 #5 |
| **V5** | OpenRouter/NVIDIA models would be hardcoded per-model. | Data-driven setup (§2.10): alias-list extraction + live catalog cache + server probes + URL-based class. No per-model entries. Phase 2 |
| **V6** | §7 rules row still cited `NetworkTools.HTTPClient()` after C1 changed the client. | §7 row updated to `proxy.SharedTransport`-based client. §7 |

---

## 7. Rules compliance

Per `.agents/rules/go-staff-engineer.md`:

| Rule | How |
|---|---|
| Small interfaces; concrete constructors | `BudgetPolicy`, `OutputCapSource`, `HTTPDoer` — one method each |
| Business rules in domain, infra at edges | Policies in `models`/orchestrator; HTTP + error parsing at provider edge |
| No boolean state flags | `WorkloadClass` typed enum |
| Value objects immutable | `Budget` returned by value |
| Composition over inheritance | Policy map + source chain |
| Typed errors, `errors.Is`/`As` | `OutputCapError`, `ErrOutputCapExceeded`, `%w` wrapping |
| Never parse error strings when structured data exists | None exists for output-cap 400s; parsing isolated at one edge file, typed immediately, last resort |
| No hardcoded values in logic | `defaultLocalContextLength`/`…Max` + `promptReserveGuess`/`minHistoryBudgetChars`/`minViablySmallContext` as file-top consts; cloud cap per-tier in `tuning.go` |
| Fail fast on invalid configuration | Unparseable output-cap 400 → typed error + actionable message |
| Table-driven tests; contract tests | Characterization matrix, local-context safety property |
| Correctness before cleverness | Verbatim local move; behaviour change isolated to one substitution |
| Constitution I.1 / I.2 | Injected `proxy.SharedTransport`-based client via `HTTPDoer`; Phase 2 amends I.2 with the provider-infrastructure carve-out (§2.8) |
| Constitution II.11 | Per-model values still flow runtime → agent without global override |
| Constitution III.5 | Tuning overrides stay in `settings.yml` |
| Constitution V.3 (no half-finished) | No feature flags; each phase complete and green |

---

## 8. Out of scope

### 8.1 The 8-minute run was not caused by these bugs

Measured from `backend/data/runs/poolside/laguna-xs-2.1/conv_20260801090104/`:

```
step  1   prompt  7,036 chars (~1.7k tokens)
step 20   prompt 32,685 chars (~8.2k tokens)
```

Peak ~8k tokens against a 170,668-char budget — trimming never engaged. Multi-minute
steps emitted 55–559 characters of reasoning (`run.log:111-115`, `:188-193`). **Those
stalls were NVIDIA-side latency.** These fixes are correctness and safety, not a speed
cure.

The one genuine connection: at `max_tokens = 42666` the three safety mechanisms of §1.8
were dormant, so the loop had no working stuck detection across its 20 steps.

### 8.2 Agent step efficiency

20 steps to list files and write a report: repeated `list_directory` on the same path,
exploratory `read_file`, `npx ts-node` / `node` / `cat` / `find`, a `.sandbox` guardrail
violation (`events.jsonl:103`), no final answer before cancellation. Separate concern —
SPEC-001 / `docs/skills/agent-loop.md`.

### 8.3 Decoupling `max_tokens` from the safety constants

Hermes omits `max_tokens` entirely and lets the provider decide (§9). We cannot, because
of M10 and because `budget_manager.go:128` needs a number for ICU. A follow-up could
introduce a separate "expected output tokens" for the stuck / char-cap / ICU consumers,
after which the wire value could be omitted. Not in this plan.

---

## 9. Reference: how Hermes Agent handles this

Verified 2026-08-01 by cloning `github.com/NousResearch/hermes-agent` (an earlier note in
this plan said "unverified" — superseded).

| # | Behaviour | Location |
|---|---|---|
| 1 | **Default is to omit `max_tokens`** — `None` unless user-configured or `HERMES_MAX_TOKENS`. On OpenAI-compat endpoints an omitted limit means full budget. | `cli.py:4366-4377` |
| 2 | Per-provider hook when omitting is wrong | `providers/base.py:161-172` `get_max_tokens(model)` |
| 3 | **Gemini native is the documented exception** — omitting makes Gemini apply a low internal default and truncate tool calls mid-stream, so they hardcode 65535 | `gemini_native_adapter.py:44-49`, `:505-517` |
| 4 | Clamp `input + max_tokens ≤ context_length`, to `context_length − 1` on small endpoints | `anthropic_adapter.py:2702-2757` |
| 5 | Parse the 400 across five provider phrasings (Anthropic, OpenRouter/Nous, LM Studio/llama.cpp, DashScope, vLLM) to recover the real available output | `model_metadata.py:1427-1547` |
| 6 | Classify output-cap errors separately from context overflow — misclassification **death-loops** the compressor (their issue #55546) | `model_metadata.py:1550-1612` |
| 7 | Unparseable → fail fast with an actionable message | `conversation_loop.py:4704-4736` |
| 8 | Context length via a 10-level ladder with persistent caching, including an OpenRouter probe | `model_metadata.py:2314-2336` |
| 9 | **Key-alias extraction** (`_CONTEXT_LENGTH_KEYS`/`_MAX_COMPLETION_KEYS` + `_iter_nested_dicts`/`_extract_first_int`) — one parser across every provider's JSON shape, no per-model hardcoding | `model_metadata.py:518-537, 939-972` |
| 10 | **Live catalog fetch + cache** — OpenRouter's full catalog fetched once, TTL-cached in-memory + on-disk; the proxy never hardcodes OpenRouter/NVIDIA models | `model_metadata.py:1041-1099` |
| 11 | **Local server probes** recover real `n_ctx` — LM Studio `/api/v1/models`, llama.cpp `/v1/props`→`/props`, vLLM `/version`; not filename guessing | `model_metadata.py:836-936, 1102-1230` |
| 12 | **Provider detection by URL hostname** (`_URL_TO_PROVIDER`, auto-extended from profiles) — not config slug | `model_metadata.py:569-611`, `providers/base.py:98-109` |

**What we adopt:** 4, 5, 6, 7 — clamp, parse, classify, fail actionably (§2.5, §2.6); **9, 10,
11, 12** — key-alias extraction, live catalog cache, local server probes, URL-based class
(§2.10, the data-driven cloud-provider setup). Hermes #8 OpenRouter probe validates Phase 2.

**What we cannot adopt:** 1. Omitting requires `MaxTokens == 0`, which trips
`stream.go:430` instantly (M10) and zeroes ICU reservations. Blocked until §8.3.
