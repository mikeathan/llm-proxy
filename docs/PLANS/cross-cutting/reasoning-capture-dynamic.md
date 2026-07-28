# Plan: Dynamic, provider-agnostic reasoning enable + capture (production-ready)

**Status:** `complete`  
**Created:** 2026-07-28  
**Last Updated:** 2026-07-28
**SPECs affected:** SPEC-003 (discovery/UI panels, assistant-ui), SPEC-005 (orchestrator budget)  
**Subsystems:** cross-cutting (proxy wire contract, assistant agent-loop, assistant-ui)  
**Related plans:** supersedes `.opencode/plans/reasoning-channel-generic.md` (early draft — per-provider param mapping not yet known)

## Context

A run with `poolside/laguna-xs-2.1` (NVIDIA) showed a "reasoning" panel full of
`Thinking...` spam and `reasoning_len 0`. Root cause (verified per provider):
our code sends a **flat top-level `reasoning_budget`** to every OpenAI-compatible
gateway, but **`reasoning_budget` is ignored by every major cloud provider**.
Each uses its own enable param:

| Provider | Correct enable param | Reasoning response field | Notes |
|---|---|---|---|
| openai / gpt-oss | `reasoning_effort` (string) | opaque (no readable text) | o-series/GPT-5 tokens unreadable |
| gemini / vertex | `reasoning_effort` (via OpenAI-compat endpoint) | `parts[].thought` (not used by our path) | proxied at `provider_gemini.go:34` |
| openrouter | `reasoning` object (`reasoning.effort`) | `reasoning` / `reasoning_details[]` | |
| mulerouter | `reasoning_effort` | `reasoning_content` (Qwen) | |
| nvidia | `chat_template_kwargs.enable_thinking` | `reasoning_content` | CONFIRMED forwarded |
| local llama.cpp | `thinking_budget_tokens` | `reasoning_content` | already correct |

So reasoning was never enabled on cloud providers; the field was silently
ignored. Goal: **fix ALL providers**, dynamic + agnostic, **typed (not
stringly-typed), SOLID + standard architecture patterns, single source of truth,
no duplication, clean code, production-ready** — and **must NOT break the
working local path** (an `openai`-slugged config pointed at a local llama.cpp
must keep using `thinking_budget_tokens` via `IsLocalModelURL`).

## Verified facts (HIGH confidence)

- laguna: `reasoning_content` field; thinking OFF by default, enabled via
  `chat_template_kwargs.enable_thinking` (Poolside card + NVIDIA NIM docs +
  OpenCode #24264 proving `integrate.api.nvidia.com` forwards it).
- OpenAI o-series/GPT-5 reasoning is **opaque** → show neutral status, never
  fabricate.
- Gemini reached via OpenAI-compat endpoint; correct param `reasoning_effort`.
- `Client.ReasoningField()` already does **host-based** detection
  (local→`thinking_budget_tokens`, cloud→`reasoning_budget`). Preserve + extend.

## Design (SOLID + patterns)

### Single Source of Truth — consolidated reasoning-wire knowledge
Today reasoning wire knowledge is split across `client.go` (`ReasoningField`),
`agent.go` (`providerTiers.ReasoningBudget`), `stream.go` (`SetReasoningBudget`),
`budget_squeezer.go` (name gate). **Consolidate into ONE table** +
**ONE resolver interface**. No duplicate per-provider logic elsewhere.

### Typed, not stringly-typed
No raw `string` for mode or effort. Define enumerated types with constants
(mirror the existing `ReasoningFieldBudget`/`ReasoningFieldThinkTokens`
pattern):

```go
type ReasoningMode int   // enumerated, not string
const (
    ModeThinkTokens ReasoningMode = iota   // local llama.cpp
    ModeEffort                           // openai/gemini/vertex/mulerouter
    ModeObject                           // openrouter
    ModeEnableThinking                   // nvidia
)

type ReasoningEffort int  // enumerated
const (
    EffortLow ReasoningEffort = iota
    EffortMedium
    EffortHigh
)
func (e ReasoningEffort) String() string // "low"|"medium"|"high" for the wire
```

`ReasoningSpec` (replaces `ProviderTuningDefaults.ReasoningBudget int`):
```go
type ReasoningSpec struct {
    Mode    ReasoningMode
    Effort  ReasoningEffort  // for ModeEffort
    Enabled bool             // for ModeEnableThinking
    Budget  int              // for ModeThinkTokens (post-squeeze amount)
}
```
Invalid states unrepresentable. `ReasoningSpec.Validate()` rejects
inconsistent combos (e.g. `ModeEffort` with `Enabled=true`).

### Strategy pattern — `ReasoningParamResolver` (Open/Closed, SRP, DIP)
`buildChatRequest` must not know per-provider wire details. Define:

```go
type ReasoningParamResolver interface {
    Apply(req *proxy.ChatRequest, spec ReasoningSpec)
}
```
One small implementation per mode:
- `effortResolver`    → `req.ReasoningEffort = spec.Effort.String()`
- `objectResolver`    → `req.Reasoning = &proxy.ReasoningObject{Effort: ...}`
- `enableThinkingResolver` → `req.ChatTemplateKwargs = &proxy.ChatTemplateKwargs{EnableThinking: true}`
- `thinkTokensResolver`    → `req.ThinkingBudgetTokens = spec.Budget`

`buildChatRequest` depends on the **interface**, never on concrete providers
(Dependency Inversion). Adding a provider = add a resolver + table entry; the
method stays closed for modification.

### Factory + single local-host override (no duplicated detection)
```go
func NewReasoningResolver(providerType string, client proxy.Client) ReasoningParamResolver
```
The factory applies the **local-host override ONCE**: if
`client.ReasoningField() == ReasoningFieldThinkTokens` (local host, via
`IsLocalModelURL`), it returns `thinkTokensResolver` regardless of cloud slug —
preserving the working local path. This is the only place host detection is
consulted; no re-checks at call sites. Unknown provider → `noopResolver`
(sends nothing).

## Changes

### 1. Types + table (SSOT)
Files: `backend/internal/core/assistant/agent.go`, `backend/models/llm_messages.go`.
- Add `ReasoningMode`, `ReasoningEffort` enums + `ReasoningSpec` (above).
- Replace `ProviderTuningDefaults.ReasoningBudget int` with
  `Reasoning ReasoningSpec`. New `providerTiers`:
  - `local` → `{ModeThinkTokens, Budget: configured}`
  - `openai`,`gemini`,`vertex`,`mulerouter` → `{ModeEffort, Effort: Medium}`
  - `openrouter` → `{ModeObject, Effort: Medium}`
  - `nvidia` → `{ModeEnableThinking, Enabled: true}` (drop unused 2048).
- `ChatRequest` additions (all `omitempty`/pointer → serialize ONLY for the
  providers that use them): `ReasoningEffort string`, `Reasoning *ReasoningObject`,
  `ChatTemplateKwargs *ChatTemplateKwargs`. Keep `ThinkingBudgetTokens` for local.

### 2. Resolver + factory
File: `backend/internal/core/assistant/reasoning_param.go` (NEW — single home
for reasoning-wire logic).
- `ReasoningParamResolver` interface + 4 impls + `noopResolver`.
- `NewReasoningResolver(providerType, client)` factory with the one local-host
  override. Pure functions, no allocation beyond the resolver pointer (shared
  singletons per mode — **no per-request allocation**).

### 3. Request build
File: `backend/internal/core/assistant/stream.go` (`buildChatRequest`).
- Replace `if a.config.ReasoningBudget > 0 { SetReasoningBudget(...) }` with:
  `NewReasoningResolver(ProviderType, deps.Client).Apply(&req, a.config.ReasoningSpec)`.
- `AgentConfig.ReasoningBudget int` becomes the **resolved post-squeeze** amount
  fed into `Spec.Budget` for `ModeThinkTokens` (local squeeze unchanged). Rename
  field to `ReasoningSpec ReasoningSpec` on `AgentConfig` for clarity; keep the
  squeeze math in `budget_squeezer.go` operating on `Spec.Budget`.
- Debug-log the resolved strategy once per request (no per-token cost) for the
  laguna verification.

### 4. Robust parser (zero-overhead)
Files: `backend/models/llm_messages.go`, `backend/internal/core/assistant/stream.go`.
- `Message` gains `ReasoningContent`,`Reasoning` (string),
  `ReasoningDetails []ReasoningDetail` (`summary/thinking/content/text`) —
  single unmarshal, `omitempty`, zero cost when absent.
- `extractReasoning(m Message) string` (single function, SSOT for precedence):
  `ReasoningContent` → `Reasoning` → `ReasoningDetails` joined → inline
  `<think>`/`<thinking>`/`<reasoning>`/`<REASONING_SCRATCHPAD>` (guarded by
  `strings.Contains(content,"<think")`). Returns `""` if none; never mutates
  `Content`.
- Wire into streaming (~`670`) + non-stream (~`710`); emit `EventReasoning`
  only when non-empty. Covers nvidia/openrouter/qwen/local. OpenAI→none→fallback.

### 5. Remove flaky name gate
File: `backend/internal/core/orchestrator/budget_squeezer.go:147-152`.
Delete `strings.Contains(name,"thinking|reason|r1|o3|o4")`. Reasoning now
driven by `ReasoningSpec` + explicit config. Update doc comment.

### 6. No-channel fallback (stop fabrication)
Files: `backend/internal/core/proxy/history.go:95`,
       `frontend/src/utils/message/turnGrouper.ts:88-96`,
       `backend/internal/core/assistant/agent_events.go` (+ `EventLifecycle`
       reason), `frontend` renderer.
- `history.go:95`: don't fill `"Thinking..."` → empty/non-display marker.
- `turnGrouper.ts:95`: delete content→reasoning lift when `reasoning_content`
  absent (Hermes: "return None, show nothing").
- `agent_events.go`: if `HasSeparateReasoning()` false →
  `EventLifecycle{reason:"reasoning_channel_none"}`; frontend shows neutral
  "Agent working…". Correctly handles OpenAI opaque case.

### 7. (Optional) inline-tag scrubbing
`stream.go`: when `extractReasoning` used inline tags, scrub them from stored
`Content` (Hermes `strip_think_blocks`); gated "only if tag present".

## Cleanup / removals (no duplication left)
- Remove `ProviderTuningDefaults.ReasoningBudget int` → `ReasoningSpec`.
- Remove `budget_squeezer.go:147-152` name heuristic.
- Remove `turnGrouper.ts:88-96` heuristic; `history.go:95` `"Thinking..."` fill.
- `SetReasoningBudget` dual-path: superseded by resolvers (keep only if still
  referenced by tests; otherwise remove to avoid two wire paths).
- Keep `ReasoningField()` host detection + `isUnsupportedParameterError` retry.

## Tests (TDD)

- `reasoning_param_test.go` (per Strategy, pure units):
  - `effortResolver` sets `ReasoningEffort`, clears others.
  - `objectResolver` sets `Reasoning` object.
  - `enableThinkingResolver` sets `ChatTemplateKwargs.EnableThinking`.
  - `thinkTokensResolver` sets `ThinkingBudgetTokens`.
  - `NewReasoningResolver`: local host (via mock client `ReasoningField()==
    ThinkTokens`) returns thinkTokens resolver even for `openai` slug
    (critical local-regression guard); cloud returns effort; unknown→noop.
  - `ReasoningSpec.Validate` rejects inconsistent modes.
- `llm_messages_test.go`: decode `reasoning_content`/`reasoning`/`reasoning_details`.
- `stream_test.go`: delta variants → `extractReasoning` captures; none → `""`
  and NO `EventReasoning`.
- `agent_test.go` (updated): per-tier request assertion (local→think_tokens;
  cloud openai/gemini/vertex/mulerouter→`ReasoningEffort`; openrouter→object;
  nvidia→`ChatTemplateKwargs`, `ReasoningBudget==0`). Keep existing
  `ZeroReasoningBudget`, XML-mode suppression (`:3167`),
  `TestProviderTuningDefaults_ReasoningBudgetField`,
  `TestClientReasoningField_ViaLocal`.
- Frontend unit: `turnGrouper` no longer lifts content→reasoning.

## Verification (must pass)
```
cd backend && go build ./... && go test ./...
go run ./tools/check-complexity/
cd frontend && npm run build
```
Re-run `list all files and report` per provider:
- **laguna (nvidia):** `ChatTemplateKwargs.EnableThinking=true` sent → expect
  `reasoning_len > 0`, real thinking, no `Thinking...` spam. Definitive proof.
- **local llama.cpp (openai-slug + local host):** identical to today
  (`thinking_budget_tokens`); assert `reasoning_effort`/`chat_template_kwargs`
  NOT serialized for local. Critical regression guard.
- **openai/gemini/openrouter (cloud):** correct param serialized, NO
  `reasoning_budget`. OpenAI panel = neutral "working" (opaque).

## Out of scope
- Per-model hand-maintained capability map (provider-tier + host detection only).
- Native Gemini `generationConfig.thinkingConfig` (our path is OpenAI-compat;
  `reasoning_effort` is correct there).
- Auto-detecting per-NVIDIA-model `reasoning_budget` vs `enable_thinking` (nvidia
  tier uses `enable_thinking` only — verified laguna needs it).

## Implementation Notes (done 2026-07-28)

All verification gates pass: `go build ./...`, `go test ./...`,
`go run ./tools/check-complexity/` (≤12), `npm run build`.

### Files changed
- `backend/models/llm_messages.go`: added `Reasoning`/`ReasoningDetails` to
  `Message`; `ReasoningEffort`, `*ReasoningObject`, `*ChatTemplateKwargs` to
  `ChatRequest`; `Message.ExtractReasoning()` (precedence: ReasoningContent →
  Reasoning → joined ReasoningDetails → inline `<think>`/`<thinking>`/`<reasoning>`/
  `<REASONING_SCRATCHPAD>`), `Message.HasSeparateReasoning()`, `extractInlineReasoning()`.
- `backend/internal/core/assistant/reasoning_param.go` (NEW): `ReasoningMode`,
  `ReasoningEffort` enums, `ReasoningSpec` + `Validate()`, `ReasoningParamResolver`
  interface + 4 mode resolvers + `noopResolver`, `providerReasoningTable` (SSOT),
  `NewReasoningResolver(providerType, client, configuredBudget)` with single
  local-host override, `localOverrideResolver`.
- `backend/internal/core/assistant/agent.go`: `ProviderTuningDefaults.ReasoningBudget
  int` → `Reasoning ReasoningSpec`; tier table uses `ReasoningSpec`; `AgentConfig`
  gains `ReasoningSpec` (kept `ReasoningBudget int` for local numeric budget used by
  preflight/interceptor/stuck-detection); `resolveReasoningSpec()` builds the spec
  from tier + configured budget.
- `backend/internal/core/assistant/stream.go`: `buildChatRequest` uses resolver
  instead of `SetReasoningBudget`; streaming accumulates `Reasoning`/`ReasoningDetails`;
  `EventReasoning` emitted from `ExtractReasoning()`; unsupported-param retry uses
  `proxy.ClearReasoningParams`.
- `backend/internal/core/proxy/client.go`: added `ClearReasoningParams`.
- `backend/internal/core/orchestrator/budget_squeezer.go`: removed the flaky
  name heuristic (`thinking|reason|r1|o3|o4`); reasoning now tier + config driven.
  The local think-token budget is **re-derived from context** (not the name):
  `resolveReasoningSpec` sets `Budget = DefaultReasoningBudget(maxTokens)` for
  `ModeThinkTokens` when no explicit budget is configured. `max_tokens` is itself
  `ctxLen/3` from the server's serving context, so the budget tracks the context
  the user launched the server with. (The local-budget derivation was finalized
  in this implementation; step 6 — the neutral "thinking/working" indicator —
  was completed in the merged section below.)
- `backend/internal/core/proxy/history.go`: empty assistant content stays empty
  (no `"Thinking..."` filler) to stop reasoning-panel spam.
- `backend/internal/transport/http/handlers/admin_handlers.go`: maps tier
  `Reasoning.Budget` into admin tuning defaults.

### Tests added/updated
- `reasoning_param_test.go` (NEW): enum/validate + 4 resolvers + local-host
  override guard + cloud/unknown noop.
- `models/llm_messages_test.go` (NEW): ExtractReasoning precedence + inline tags.
- `agent_test.go`: Nvidia → `ChatTemplateKwargs.EnableThinking`; cloud →
  `ReasoningEffort`/`Reasoning` object; local still `thinking_budget_tokens`.
- `history_test.go`: empty assistant stays empty.

### Deviations from plan
- Kept `AgentConfig.ReasoningBudget int` (not renamed to `ReasoningSpec`) to limit
  blast radius across admin/model/automation handlers and preserve preflight ICU +
  stuck-detection math; the typed `ReasoningSpec` drives wire params.
- Step 7 (inline-tag scrubbing of stored Content) not implemented — out of the
  core bug fix; extractReasoning covers display/event capture without mutating
  stored content.
- Frontend `turnGrouper.ts` content→reasoning lift retained: it only fires for
  tool-call assistant messages (planning text), not generic content; removing it
  would regress tool-call display. The `"Thinking..."` spam root cause (backend
  enabling reasoning correctly + history no longer fabricating placeholders) is
  addressed.

---

# Step 6 (merged from `reasoning-neutral-working-state.md`): Neutral "thinking / working…" indicator

**Completed 2026-07-28.** Step 6 of this plan was skipped during the core fix
and completed separately; content merged here. Covers BOTH the pre-response
compute wait (neutral "thinking…") AND the tool-execution wait (neutral
"working…"). This is a **status**, NOT fabricated reasoning text — the old
`"Thinking..."` string in `history.go` was wrong because it was injected as
message content and surfaced as reasoning-panel spam.

## Verified facts (read-only)
- Live chat path = `AgentIde/assistant/ChatMessages.vue` → `ChatBubble.vue`;
  `ChatBubble` receives `phase`/`thinking`/`liveReasoning` from `useMessageBuilder`
  via `ChatMessages.vue:122-126`.
- `ChatBubble.vue` already renders `phaseLabel = "Assistant — thinking..."` when
  `phase==='thinking'` and a spinner via `loading && isLastTurn`. Infra exists.
- `messageBuilder.ts` set `thinking=true`+`phase='thinking'` ONLY on a
  `reasoning`/`tool_stream` event — so an opaque model with no `reasoning` event
  kept `phase='idle'` → blank/"Assistant". Same for the pre-first-token wait.
- Tool-execution wait already covered: `tool_call` → `phase='working'`.
- Backend emits `EventLifecycle{session_started}` once per conversation, but
  nothing per LLM call. Three call sites emit the request:
  `computeNextResponse` (stream), `computeNextResponseNonStreaming`,
  `computeNextResponseStreamXML` (`stream.go`).
- OLD `AgentRun.vue` (with "Working…" header) is dead code — ignore it.

## Changes (done)
1. `backend/internal/core/assistant/agent_events.go`: added `PhaseAgentThinking = "agent_thinking"` (distinct from `session_started`).
2. `backend/internal/core/assistant/stream.go`: emit `notifyLifecycle(PhaseAgentThinking, {"step": …})` at start of each of the three compute functions. `notify` no-ops when observer nil. Emitted for ALL providers (opaque OpenAI included). Re-entrancy (XML fallback/prefill retry) may double-emit — harmless (frontend idempotent).
3. `frontend/src/utils/message/messageBuilder.ts`: extended the `lifecycle` branch — `p.phase === 'agent_thinking'` → if `phase.value === 'idle' || 'done'`, `setPhase('thinking')` + `thinking.value = true`. Real reasoning later arrives via a `reasoning` event and fills the inset; no regression. Tool execution keeps `working`.

## Tests (done)
- `reasoning_working_test.go` (NEW): opaque + reasoning providers emit
  `agent_thinking`; signal precedes first assistant content; payload carries no
  reasoning/content fields.
- Frontend: single idempotent branch; no frontend test runner configured (repo
  frontend verification is `npm run build` only), so no unit test added to avoid
  new test dependency; logic covered by existing `ChatBubble.vue`
  `phaseLabel`/"thinking…" path.

## Verification (done)
`go build ./...`, `go test ./...`, `check-complexity/` (≤12), `npm run build` — all pass.

## Out of scope
- Dead `AgentRun.vue` rewrite (separate cleanup).
- Auto-collapsing the thinking inset (handled by `autoCollapse`).
- Per-model capability map (provider-tier + host detection only).

