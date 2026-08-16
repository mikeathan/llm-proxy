# Plan: Remove model auto-bootstrap; explicit primary/fallback selection + banners

> **Frontend redesign (supersedes "Frontend changes" §1–6 below):**
> The main banner is now a **generic, domain-agnostic sink** (`composables/ui/useAppBanner.ts`:
> `show`/`clear`/`active`, `AppBannerMessage` carries only `severity`/`message`/`html?`/`persistent?`/`action?`).
> The model-configuration warning logic lives in a dedicated, self-starting composable
> `composables/models/useModelBanner.ts` that owns the derivation (`computeModelBanner`
> in `composables/models/modelBanner.ts`) and emits a complete message to the bus. It
> watches `useModels().state` so the warning appears/disappears reactively on **any**
> state change (Settings save, model add/remove) without a page reload, and it backs
> off / re-asserts when a transient (non-persistent) error banner is shown and then
> cleared (precedence is component-level, not a priority field on the message).
> App.vue calls `useModelBanner().start()` once centrally; no component emits the
> banner directly except via this composable or the generic `show` (transient errors
> in `useDispatcher`/`useAssistant`). `AppBanner.vue` is unchanged (renders `active`,
> dismiss gated on `!persistent`, supports `html` + `action` deep-linking to Settings).
> Tests: `useModelBanner.test.ts` (re-assert behavior) + `useAppBanner.test.ts` (dumb
> sink). Built + tested green.

## Goal
1. Delete the entire auto-bootstrap path that creates a default model and sets
   `PrimaryModel` when credentials are first saved.
2. Guarantee `PrimaryModel`/`FallbackModel` can never dangle: if the model they
   reference is removed from the catalogue, the reference is auto-cleared; and a
   user cannot *set* a primary/fallback that does not exist.
3. Change model selection semantics (no arbitrary fallback):
   - Primary set -> use primary.
   - Primary unset, fallback set -> use fallback.
   - Both unset -> error (request fails with a clear message).
4. Surface a **global, persistent** banner:
   - **Critical (amber/red, sticky):** no primary AND no fallback set.
   - **Notice (blue/info, sticky):** primary unset but fallback set ("using
     fallback X"). No banner for fallback alone.
   Banner stays until resolved (re-evaluated on each state refresh).

## Design decisions (confirmed)
- Banner: global persistent, two-tier (critical + notice).
- Fallback auto-cleared when its model is removed, but never warned on its own.
- No silent `Catalogue[0]` fallback when primary is empty.

---

## Backend changes

### 1. Delete `internal/transport/http/handlers/secrets_bootstrap.go`
Remove the whole file: `bootstrapProviderDefaultModel`, `selectDefaultModelID`,
`matchesModelToken`, `defaultModelPreference`, `sanitizeModelName`,
`bootstrapModelName`, and `defaultModelNameFallback` const if unused elsewhere
(grep first). Remove its test file `secrets_bootstrap_test.go`.

### 2. Stop invoking bootstrap — `secrets_handlers.go`
- Remove the `firstActivation` bootstrap block at `secrets_handlers.go:94-108`.
- Keep the orphaned-model cleanup (`reg.Catalogue = out`) and add the
  dangling-clear call (step 4) there.

### 3. New model-selection semantics — `app_context.go:116-129`
Replace `SelectModels()`:
```go
func (a *AppContext) SelectModels() (string, string) {
    reg := a.dataMgr.Registry().Get()
    p := reg.PrimaryModel
    f := reg.FallbackModel
    if p == "" {
        p = f // fall back to fallback only
    }
    return p, f
}
```
Drops the `Catalogue[0]` auto-pick. Empty primary now stays empty unless
fallback covers it; both empty -> callers already error on empty primary.

### 4. Auto-clear dangling primary/fallback on model removal
Add helper (in `app_context_models.go` or new file), called inside each Catalogue
rebuild `Update` closure AFTER `reg.Catalogue = out`:
```go
func clearDanglingModelRefs(reg *models.RegistryData) {
    names := make(map[string]bool, len(reg.Catalogue))
    for _, m := range reg.Catalogue {
        names[m.Name] = true
    }
    if reg.PrimaryModel != "" && !names[reg.PrimaryModel] {
        reg.PrimaryModel = ""
    }
    if reg.FallbackModel != "" && !names[reg.FallbackModel] {
        reg.FallbackModel = ""
    }
}
```
Call sites (all rebuild `reg.Catalogue`):
- `AppContext.PersistDeleteModel` (`app_context_models.go:157`) — single delete.
- `AppContext.DeleteProviderWithCleanup` (`app_context_models.go:~193`) — bulk
  provider delete; add the call inside its `Update` closure AFTER
  `reg.Catalogue = out` and before `delete(reg.Providers, ...)`.
- `secrets_handlers.go` key-save cleanup (`secrets_handlers.go:71-86`) — call
  after `reg.Catalogue = out`.
- `model_handlers.go` `handleDeleteAllModels` (`model_handlers.go:799-815`) —
  currently calls `PersistDeleteModel` per model, which already clears; verify
  no separate Catalogue rebuild bypasses it. If it rebuilds Catalogue directly,
  add the call.
- Any other `reg.Catalogue = out` rebuild found by grep.

### 5. Validate primary/fallback on set — `ApplySystemUpdate` (`app_context_system.go:157`)  [GAP 4 — MUST ADD]
Currently any non-empty `req.PrimaryModel`/`req.FallbackModel` string is accepted
blindly (no existence check). Add validation inside the `Update` closure:
for each of `req.PrimaryModel`/`req.FallbackModel` that is non-empty, confirm the
name exists in `reg.Catalogue`; if not, return an error that surfaces as HTTP 400
("primary model X does not exist"). This closes the hole auto-clear cannot:
auto-clear only fixes *post-hoc* removal, not a user *setting* a non-existent
model. The Settings UI already sources the dropdown from `props.models`
(existing catalogue), so normally only valid names are submitted; this is defense
in depth.

### 6. Verify error paths on empty selection
Callers already handle empty primary; confirm behavior:
- `proxy_handlers.go:60` -> `http.Error(..., "missing model name and no default configured", 400)`.
- `core/proxy/provider.go:49` -> `fmt.Errorf("no target model available")`. Note:
  with new semantics, when primary unset + fallback set, `SelectModels` returns
  `primary == fallback`; the `fallback != primary` guard at `provider.go` then
  correctly SKIPS redundant failover (only one model to try). No bug.
- `assistant_handlers.go:252` `getLLMClient` -> 500 "No primary model is
  configured...". This is the real user-facing guard for the assistant path.
- `conversation_service.go:127` `resolveModelConfig` calls `SelectModels()` then
  `ModelConfig(modelName)`. When both unset, `modelName == ""` and
  `ModelConfig("")` returns `(empty, false)` (bootstrap.go:231) — harmless; the
  actual error is raised at `getLLMClient`. Note this so the implementer is not
  surprised; no code change needed.
- `automation/executor.go:149` sets `req.Model = primary` (may be "") then relies
  on later client error. Acceptable; no change.

### 7. Tests
Delete: `secrets_bootstrap_test.go`; `TestAdminProviderKeysPutHandler_FirstActivationBootstrap`;
any bootstrap cases in `secrets_handlers_test.go`.
Add:
- `app_context_models_test.go`: delete primary model -> `PrimaryModel` cleared;
  delete fallback model -> `FallbackModel` cleared; both removed -> both empty;
  `DeleteProviderWithCleanup` clears refs for the removed provider's models.
- `app_context_test.go`: `SelectModels` returns fallback when primary empty;
  returns ("","") when both empty (callers error).
- `app_context_system_test.go` (or existing system test): `ApplySystemUpdate`
  rejects a non-existent `PrimaryModel`/`FallbackModel` with an error; accepts a
  valid one.
- `secrets_handlers_test.go`: saving/rotating keys and removing a provider no
  longer bootstraps a model and does not set `PrimaryModel`.

---

## Frontend changes

### 1. New global banner component
Create `src/components/common/WarningBanner.vue` modeled on
`src/components/AgentIde/common/ErrorBanner.vue` (top absolute banner, dismiss
button) but supporting a `severity: 'critical' | 'notice'` prop controlling
color (amber/red vs blue). Non-dismissable-until-resolved (or re-appears on next
state refresh). Props: `{ severity: 'critical'|'notice', message: string }`.

### 2. Mount in `App.vue`
Add `<WarningBanner v-if="banner" :severity="banner.severity" :message="banner.message" />`
above `<main>`, imported. Inside the `v-if="state"` region so it shows once state
loads. Keep `<Toast />` as-is.

### 3. Compute banner from existing `useModels` state
`useModels` already fetches `/admin/api/state` -> `state.value.config` (has
`primary_model`, `fallback_model`) and `state.value.models` (each item has
`.name`, see `types/model.ts:3`). Derive:
```ts
const banner = computed(() => {
  const cfg = state.value?.config
  if (!cfg) return null
  const names = new Set((state.value?.models ?? []).map(m => m.name))
  const primaryOk = !!cfg.primary_model && names.has(cfg.primary_model)
  const fallbackOk = !!cfg.fallback_model && names.has(cfg.fallback_model)
  if (!primaryOk && !fallbackOk) {
    return { severity: 'critical', message: 'No primary or fallback model set. Requests will fail. Open Settings -> Global to choose a model.' }
  }
  if (!primaryOk && fallbackOk) {
    return { severity: 'notice', message: `Primary model not set — using fallback "${cfg.fallback_model}". Set a primary in Settings -> Global.` }
  }
  return null
})
```
Only primary drives banners; fallback alone -> no banner.

### 4. State refresh wiring [GAP 7 — confirmed OK]
`useModels.removeModel` (useModels.ts:70-74) calls `refresh()` after delete, so
the `banner` computed re-evaluates automatically. No extra wiring needed.
Similarly model add/start/stop all call `refresh()`. Confirmed.

### 5. Settings UI [GAP 5 — note, no change required]
`GlobalSettings.vue` primary/fallback `<select>` already sources options from
`props.models` (existing catalogue), so users can only pick valid models. Combined
with the backend validation in step 5, the dangling-set path is fully closed.
No separate UI change required; documented for completeness.

### 6. Frontend tests (if `npm test` runs)
Unit-test the `banner` computed: both unset -> critical; primary unset + fallback
ok -> notice; primary ok -> null; primary set-but-not-in-catalogue (should not
happen post auto-clear, but guard) -> treated per primaryOk=false.

---

## Verification (per AGENTS.md workflow)
Backend:
- `cd backend && go build ./...`
- `go test ./...` (0 FAIL; bootstrap tests removed)
- `go run ./tools/check-complexity/`
- `go vet ./...` ; `gofmt -l` on touched files
- `-race` on `internal/app` and `internal/transport/http/handlers`

Frontend:
- `cd frontend && npm test` ; `npm run build`

## Risks / notes
- Removing the `Catalogue[0]` fallback means a fresh install (keys saved, no
  model added, nothing set) now ERRORS on requests instead of silently serving a
  model. Intended — the banner communicates the missing config.
- `SelectModels` returning empty primary is already handled at all known callers
  (proxy 400, provider error, assistant 500). `conversation_service` tolerates
  empty via `ModelConfig("")` returning false; the real guard is `getLLMClient`.
- Ensure no other caller references deleted symbols before deleting
  (`bootstrapProviderDefaultModel`, `bootstrapModelName`, `selectDefaultModelID`,
  `matchesModelToken`, `defaultModelPreference`, `sanitizeModelName`,
  `defaultModelNameFallback`) — grep first.
