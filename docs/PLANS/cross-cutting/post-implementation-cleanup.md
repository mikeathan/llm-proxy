---
status: active
last_reviewed: 2026-08-01
---

# Post-Implementation Cleanup: Duplication & Dead Code

**Status:** 🚧 Draft plan — derived from the audit of the
`cloud-provider-token-budgets` implementation (see
`docs/PLANS/cross-cutting/cloud-provider-token-budgets.md`).
**Related:** `cloud-provider-token-budgets.md`, SPEC-005.
**Rules:** `.agents/rules/go-staff-engineer.md`, `.agents/rules/frontend-vue-engineer.md`
**Constitution:** I.1 (no `http.DefaultClient`), III.5 (settings persistence).

## 0. Origin & scope

After the budget refactor landed, a full duplication/dead-code sweep of the touched
surface (`orchestrator`, `llm/providers`, `assistant`, `models`, `handlers`, `frontend`)
found the plan's single-source-of-truth (SSOT) largely held — no `ctx/4`, no
`providerCtxDefaults`, no `computeDefaultsFromContext`, all Phase-5 dead components
already deleted. Residual residue falls into three buckets:

- **A. Dead code** — symbols with zero callers anywhere (prod or tests).
- **B. Duplicated code** — same logic in 2+ places that should collapse to the SSOT.
- **C. Drift / stale** — values or comments that disagree and need a decision, not a
  mechanical edit.

This plan does **not** alter runtime behaviour for supported providers except where a
consolidation fixes a latent divergence (notably `model_handlers.go` context cap, §B5).

**Non-negotiable:** the `cloud-provider-token-budgets` §0 local invariant stays green.
Every phase below must keep `TestCharacterizationGolden` and `TestLocalContext*`
byte-identical. No phase may change a local `max_tokens`/`ContextBudget` value.

---

## 1. Findings register

### Bucket A — Dead code (safe to delete)

| ID | Location | Dead symbol | Evidence |
|---|---|---|---|
| A1 | `backend/internal/core/assistant/agent.go:137-142` | `TierForProvider` | zero callers (prod + tests); superseded by `models.TuningFor` / `ProviderTiers()` |
| A2 | `backend/internal/core/assistant/agent_events.go:191-197` | `notifyMemoryRecall`, `notifyMemoryFlush` | zero callers; constants `EventMemoryRecall`/`EventMemoryFlush` (`:34-35`) unused elsewhere; no frontend handler |
| A3 | `backend/internal/core/proxy/client.go:107-109` | `IsLocalModelURL` | sole caller is `client_test.go:439`; it is the only caller of `IsLocalModelURLStrict` |
| A4 | `backend/internal/core/models/workload.go:154-156` | `IsLocalModelURLStrict` | dead once A3 removed; `hostMatches` (`:126`) must stay — used by `ClassifyEndpoint` |
| A5 | `backend/internal/core/llm/providers/launch_config.go:12-14` | `ResolveHost` | pass-through over `network.ResolveHost`; single in-package caller (`registrar.go:225`) |
| A6 | `frontend/src/utils/model/models.ts` (whole file) | — | zero importers; duplicates `modelUtils.ts` (`deriveFriendlyName`, `makeEmptyForm`, `isLocalProvider`) |
| A7 | `frontend/src/utils/model/model-discovery.ts` (whole file) | — | zero importers; `formatFileSize`/`formatParameters` are byte-clones of `utils/format/formatters.ts` |
| A8 | `frontend/src/components/common/agent/AgentRun.vue` | — | zero references |
| A9 | `frontend/src/components/common/chat/ToolCallBlock.vue` | — | zero references |
| A10 | `frontend/src/components/common/chat/ToolResultBlock.vue` | — | zero references |
| A11 | `frontend/src/components/common/chat/LifecycleMessage.vue` | — | zero references |
| A12 | `frontend/src/components/common/layout/CollapsiblePanel.vue` | — | zero references |
| A13 | `frontend/src/constants/providers.ts:3-24` | `CLOUD_PROVIDERS`, `ALL_PROVIDERS`, `SETTINGS_TABS` | zero importers; superseded by manifest-driven `useProviders.ts` (`PROVIDER_ICONS/LABELS/STYLES` in same file ARE used — keep) |

**A-flag (do NOT delete blindly):** `orchestrator.ResolveICUWeight`
(`budget_squeezer.go:195-218`) has no production caller — nothing sets
`AgentOptions.ICUWeight` (defaults to 1.0 at `agent.go:369`), so per-model ICU weighting
is inert. It lives only in `budget_squeezer_test.go` / `local_safety_test.go` (12 refs).
Decision required (see §4): either (a) wire it into agent construction so it is live, or
(b) delete the function + its test assertions together. Not a mechanical deletion.

### Bucket B — Duplicated code (consolidate)

| ID | Location | Duplication | Fix |
|---|---|---|---|
| B1 | `backend/internal/core/orchestrator/budget_squeezer.go:67` | `contextLen = 8192` literal duplicates `defaultLocalContextLength` (`budget_policy.go:19`, same package) | swap literal → constant (zero risk) |
| B2 | `frontend/src/composables/models/useConfig.ts:49-64` vs `frontend/src/composables/settings/useProviderModels.ts:34-49` | byte-identical 14-field `agent_defaults` literal | `useProviderModels` imports the defaults from `useConfig.ts` (export them) |
| B3 | `backend/internal/core/llm/manager.go:560-562` | `localWorkload := cfg.WorkloadClass == WorkloadLocal \|\| HasGGUFArtifact(...) ×3` — partial re-impl of `WorkloadClassifier.Classify` (`models/workload.go:51-62`), drops provider + endpoint branches | replace with `m.registrar.Classify(cfg) == models.WorkloadLocal` (already used at `manager.go:536`; lock-safe per comment `:533-535`) |
| B4 | `backend/internal/transport/http/handlers/admin_handlers.go:307-322` vs `:374-395` | near-identical `adminTuningDefaults` literals (10/14 fields same `assistant.Default*` consts) | factor constant tail into one builder; `admin_handlers_test.go:521-565` asserts JSON output, not internals |
| B5 | `backend/internal/transport/http/handlers/model_handlers.go:212-240` | second context-resolution impl — prefers `Meta.Nctx`, else `Meta.ContextLength` capped at `128_000`, else tier defaults. Duplicates `orchestrator.resolveLocalContext` (`:45`) / `publishedServingMetadataContext` (`:83`) but with a **different cap** (`128_000` vs `defaultLocalContextMax = 1_048_576`) | **highest-value fix.** Export a resolver from `orchestrator` (handlers already import it — no cycle). TDD-first: behaviour differs at boundary, so a test must pin the intended cap before merging |
| B6 | `backend/internal/core/assistant/agent.go:93-100` `ProviderTuningDefaults` vs `backend/models/tuning.go:14-24` `ProviderTuning` | two structs, 5 identical fields; assistant one only adds `Reasoning` | embed `models.ProviderTuning` (models is leaf, no cycle) — low priority |
| B7 | `frontend/src/components/settings/ProviderModelsCard.vue:275` | divider `<div class="form-section-divider">Agent Tuning…</div>` rendered twice (own + inside `ModelTuningFields.vue:20`) in cloud-edit branch only | delete the redundant `:275` divider; local/add branches don't have it |

### Bucket C — Drift / stale (needs a decision)

| ID | Location | Issue | Required decision |
|---|---|---|---|
| C1 | `models/tuning.go:29` `local` MaxTokens **2048** vs `assistant/agent.go:26` `DefaultMaxTokens` **3072** vs `frontend useConfig.ts:52` / `useProviderModels.ts:37` **3072** | same conceptual default, 3 tables, already drifted | choose ONE authoritative source (recommend `models/tuning.go` as leaf SSOT; frontend pulls from API `provider_defaults`) |
| C2 | `backend/internal/core/orchestrator/budget_squeezer.go:162-167` | `ErrCapabilityImpossible` (V3 contract) returned by `CloudBudgetPolicy` but **swallowed** with bare `return` | surface the error (admin warning / log) — do not remove; per `cloud-provider-token-budgets` §2.4 it is actionable |
| C3 | `frontend/src/components/settings/ModelTuningFields.vue:81` | UI text `"Output cap is set by the server (ctx/3)"` is a 4th copy of the local formula as prose | reword to "derived from serving context" to avoid drift |
| C4 | `frontend/src/utils/model/modelUtils.ts:34-39` `providerTuningHints` | both branches return `tool_call_format: ""`; comment claims "Cloud models default to native" but code does not do it | collapse to a `prefill` boolean; correct the comment |
| C5 | `frontend/src/utils/model/modelUtils.ts:112-115` | `max_tokens`, `temperature`, `reasoning_budget`, `timeout_minutes` re-assigned to the exact values from `...tuning` on `:111` — pure no-ops | delete the redundant re-assignments |

**Checked clean (no action):** `providerCtxDefaults` (comments only), `knownCtx`
(`context_resolution.go` only), `slot_timeout` (live end-to-end: `config.go:397` →
`manager.go:576` → `admin_view.go:38` → `types/model.ts:46`), `vertex`/`mulerouter`
(comments only + deliberate unknown-provider fixture `provider_unknown_test.go:20`),
duplicate function names (only legit `setPdeathsig` build-tag variants).

---

## 2. Phases

| # | Phase | Behaviour change | Gate |
|---|---|---|---|
| A | Dead-code deletions (A1–A13, defer A-flag) | none | `go build ./... && go test ./...`; frontend `npm run build` |
| B1 | Literal→constant (B1) + context-resolver dedup (B5, TDD) | fixes `128_000` vs `1M` cap divergence on local context | golden snapshot byte-identical; `go test ./...` |
| B2 | Classifier dedup (B3) + admin defaults dedup (B4) + frontend defaults dedup (B2) + UI divider (B7) + struct embed (B6) | none | `go test ./...`; `npm run build` |
| C | Drift decisions (C1–C5) | C1 may change a default constant; C2 surfaces an error | golden snapshot + `npm run build`; doc stewardship if C1 touches SPEC |
| X | A-flag resolution (ResolveICUWeight) | only if we choose to wire it live (C-or-b decision) | — |

### Phase A — Dead-code deletions

**Backend**

| File | Change |
|---|---|
| `internal/core/assistant/agent.go` | delete `TierForProvider` (A1) |
| `internal/core/assistant/agent_events.go` | delete `notifyMemoryRecall`, `notifyMemoryFlush`, `EventMemoryRecall`, `EventMemoryFlush` (A2) |
| `internal/core/proxy/client.go` | delete `IsLocalModelURL` (A3) + its test `client_test.go:437-481` `TestIsLocalModelURL` |
| `internal/core/models/workload.go` | delete `IsLocalModelURLStrict` (A4); keep `hostMatches` |
| `internal/core/llm/providers/launch_config.go` | inline `ResolveHost` at `registrar.go:225` (A5) |
| `internal/transport/http/handlers/admin_handlers.go` | drop `convertProviderTiers` legacy if orphaned after B4 (verify) |

**Frontend**

| File | Change |
|---|---|
| `src/utils/model/models.ts` | delete file (A6) |
| `src/utils/model/model-discovery.ts` | delete file (A7) |
| `src/components/common/agent/AgentRun.vue` | delete file (A8) |
| `src/components/common/chat/ToolCallBlock.vue` | delete file (A9) |
| `src/components/common/chat/ToolResultBlock.vue` | delete file (A10) |
| `src/components/common/chat/LifecycleMessage.vue` | delete file (A11) |
| `src/components/common/layout/CollapsiblePanel.vue` | delete file (A12) |
| `src/constants/providers.ts` | delete `CLOUD_PROVIDERS`, `ALL_PROVIDERS`, `SETTINGS_TABS` (A13); keep `PROVIDER_ICONS/LABELS/STYLES` |

Verify zero importers with a grep before each deletion. Confirm `providerCtxDefaults`
/ `vertex` / `mulerouter` remain comment-only (no resurrection).

### Phase B1 — SSOT constant + context-resolver dedup

| File | Change |
|---|---|
| `orchestrator/budget_squeezer.go:67` | `contextLen = defaultLocalContextLength` (B1) |
| `orchestrator/context_resolution.go` | export `ResolveLocalContext(cfg)` (workload-scoped, cap `defaultLocalContextMax`) for handler reuse |
| `internal/transport/http/handlers/model_handlers.go:212-240` | replace bespoke walk with `orchestrator.ResolveLocalContext` (B5) |

**TDD first (B5):** add a test pinning the intended cap. Current handler caps at
`128_000`; `LocalBudgetPolicy` path caps at `1_048_576`. Decide which is correct (recommend
`defaultLocalContextMax` to match the local path) and assert before merge. This is the one
phase that can change a derived value, so the golden snapshot must stay byte-identical for
the local rows — if the chosen cap alters an existing golden line, that is a behaviour
change requiring explicit sign-off, not a silent edit.

### Phase B2 — Remaining dedup

| File | Change |
|---|---|
| `internal/core/llm/manager.go:560-562` | `m.registrar.Classify(cfg) == models.WorkloadLocal` (B3) |
| `internal/transport/http/handlers/admin_handlers.go:307-322,374-395` | single `adminTuningDefaults` builder (B4) |
| `frontend/src/composables/settings/useProviderModels.ts:34-49` | import `agent_defaults` from `useConfig.ts` (export it) (B2) |
| `frontend/src/components/settings/ProviderModelsCard.vue:275` | delete redundant divider (B7) |
| `internal/core/assistant/agent.go:93-100` | embed `models.ProviderTuning` in `ProviderTuningDefaults` (B6) |

### Phase C — Drift decisions

| Decision | Recommended action |
|---|---|
| C1 | Make `models/tuning.go` the leaf SSOT for `local` `MaxTokens`. Frontend already fetches `provider_defaults` from API — remove the hardcoded `3072` from `useConfig.ts`/`useProviderModels.ts`. No golden impact (local prefill only). |
| C2 | At `budget_squeezer.go:162`, replace bare `return` with an admin warning + structured log carrying provider/model + numeric limit (no payloads, per Constitution). `ErrCapabilityImpossible` stays. |
| C3 | `ModelTuningFields.vue:81` → "derived from serving context". |
| C4 | `modelUtils.ts:34-39` collapse to prefill boolean; fix comment. |
| C5 | `modelUtils.ts:112-115` delete no-op re-assignments. |

If C1 touches the `local` `MaxTokens` row used for prefill, confirm SPEC-005 still matches
(Phase 6-style doc stewardship). Otherwise docs need no change.

### Phase X — `ResolveICUWeight` (A-flag)

Two options, pick one:
- **X-a (wire live):** set `AgentOptions.ICUWeight` from model config during
  `ApplyModelConfig` so the per-model ICU weight actually applies (closes the M9 gap from
  the budget plan). Add a test.
- **X-b (delete):** remove `ResolveICUWeight` + its 12 test assertions; document that ICU
  weight is fixed at 1.0.

Until decided, **leave it** — do not delete silently.

---

## 3. Tests & gates

Per `AGENTS.md`: build + test after each backend edit; `npm run build` after each frontend
edit; finish with `go test ./...` + `go run ./tools/check-complexity/`.

**Must stay green (§0 contract):**
- `TestCharacterizationGolden` — local rows byte-identical (especially B5; see Phase B1 note).
- `TestLocalContextNeverLeaksCloudDefault`, `TestClassifyLocalURLMatrix` — local safety.
- `client_test.go` `TestIsLocalModelURL` removed with A3.
- `admin_handlers_test.go:521-565` — JSON output shape unchanged by B4.

**New tests:**
- B5: handler context-resolution test pinning the cap (`128_000` vs `1_048_576` decision).
- X-a (if chosen): ICU-weight-applied test.

**Complexity:** every new/changed function ≤12 (`go run ./tools/check-complexity/`).

**Constitution:**
- I.1: no `http.DefaultClient` reintroduction during A5 inline.
- III.5: C1 default moves stay in code/API, not `registry.json`.

---

## 4. Out of scope

- Behaviour changes for supported providers beyond B5's cap correction.
- The `cloud-provider-token-budgets` §0 local invariant (must remain untouched).
- Any new feature; this is cleanup only.
- Frontend component rewrites beyond the listed deletions/dedup.

---

## 5. Recommendations (sequencing)

1. **PR 1 (low risk):** Phase A (A1–A13) — pure deletions, all verified zero-importer.
2. **PR 2:** Phase B1 (B1 constant + B5 resolver dedup, TDD-first) — highest value, fixes
   latent cap divergence.
3. **PR 3:** Phase B2 (B2/B3/B4/B6/B7) — remaining dedup.
4. **PR 4:** Phase C (C1–C5) — drift decisions, mostly trivial.
5. **Deferred:** Phase X (`ResolveICUWeight`) — needs the M9 decision; out of this cleanup's
   critical path.
