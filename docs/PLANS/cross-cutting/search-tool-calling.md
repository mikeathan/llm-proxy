# Wire up the `internet_search` tool (pluggable multi-provider)

**Status:** active — implemented; automated gates green (backend build/vet/race/complexity, frontend test/build); manual end-to-end verification pending
**Date:** 2026-09-12
**Related Specs:** SPEC-001 (Agent Loop), SPEC-006 (Guardrail Engine)

## Goal
Make the existing `internet_search` agent tool usable end-to-end: the operator picks a search
provider and enters its API key in Settings, the tool becomes visible to the model **live (no
restart)**, and calls return real results. Providers are pluggable.

## Skills applied in this review
Loaded per `AGENTS.md` → Skills (Design/plan + Implement):
`.agents/skills/clean-code/SKILL.md`, `.agents/skills/engineering-practices/SKILL.md`,
`.agents/skills/tdd-guide/SKILL.md`, `.agents/skills/testing-guide/SKILL.md`,
`.agents/skills/connector-patterns/SKILL.md` (analogous registration recipe),
`.agents/skills/documentation-stewardship/SKILL.md`, plus the mandatory rule layers
`.agents/rules/go-staff-engineer.md` and `.agents/rules/frontend-vue-engineer.md`,
and `CONSTITUTION.md` (I.1/I.2 guarded network, II.13 prompts, V.2 spec-first).

## Corrections from this pass (vs the previous revision)
1. **Dropped the `validateSearch` "not configured" backstop.** A guardrail denial routes through the
   approval flow; a tool that cannot work must not raise an approval prompt. Schema-hide is the single
   gate; a residual call now reaches the tool and returns its error, which the loop records as a tool
   result (record-and-continue, non-approvable). Fewer code paths (clean-code: no speculative error
   handling).
2. **Removed the unused `defaultSearchTimeout` const** (dead code — `go-staff-engineer` "no hardcoded
   values" does not mean invent an unused one; Constitution IV.4 no dead code). The executor's
   per-tool timeout already bounds the call — one source, not two.
3. **`InternetTools.Search` is nil-safe** (nil receiver or nil resolver → `ErrSearchNotConfigured`) —
   Null Object over nil checks, which removes the registry-side `r.Search == nil` branch too.
4. **`SearchProvider.Valid()` treats `""` as valid** (unset → default), and `Validate()` rejects only
   a non-empty unknown value and out-of-range `MaxResults`. The previous draft would have rejected the
   default empty config on every save.
5. **One shared `searchTarget()` helper** feeds both `resolve` and `available` (DRY — no predicate
   drift between the tool and the schema gate).
6. **One `searchDisabled(cfg)` guardrail helper** used by both `DisabledToolNames` and
   `scopeTierDisabled` (no duplicated OR expression).
7. **Provider HTTP hygiene:** build GET params with `url.Values`, bound the error body read, validate
   result URLs with `net/url`, and never log the API key or the query.
8. **Staged provider rollout:** Tavily first (existing, lowest risk), then Brave and SerpAPI each gated
   on verifying the live API shape — drop a provider rather than ship an unverified one.
9. **Docs target the agent-facing home:** the "add a search provider" recipe goes in
   `.agents/skills/engineering-practices/SKILL.md`, with `docs/architecture.md` holding only a pointer
   (avoid the duplication the old draft implied).
10. **Test plan mirrors each source file** and reuses existing tables where one exists (tdd-guide
    "keep the suite fast").

## Implementation notes (2026-09-12)
Recorded while implementing; where these differ from the text above, these are the shipped choices.
1. **Layout is Option B (user-directed, supersedes "no new subpackage" above):** `tools/search.go`
   keeps the contract + `InternetTools`; `tools/searchproviders/` holds the three providers and the
   explicit table + shared provider plumbing. This mirrors `tools/communication.go` + `tools/notifiers/`
   and keeps each package under the ~15-file soft cap (`tools/` = 15, `searchproviders/` = 8). The
   subpackage imports `tools` for the contract; `tools` never imports the subpackage. The shared
   provider helpers (`normalizedMaxResults`, `validResultURL`, `errorBodySnippet`, body bound) live in
   `searchproviders` — not exported from `tools`.
2. **Two sentinels + `models.IsSearchConfigError`:** the plan named only `ErrInvalidSearchProvider`.
   Implemented as `ErrInvalidSearchProvider` + `ErrSearchMaxResultsOutOfRange`, classified by
   `IsSearchConfigError(err)`, so the handler maps both save-boundary failures to 400 in one check.
3. **Test mirroring fixed:** `search_registry.go` previously had its test inside `search_tavily_test.go`;
   it now has its own `search_registry_test.go`. Shared HTTP test helpers live in the test file
   mirroring the shared-plumbing source. No `_test.go` without a mirroring source was added.
4. **Frontend empty state dropped:** the provider list is always non-empty (backend list with a typed
   local fallback), so an empty-list branch would be dead code. States covered: Loading / Success /
   Error (save).
5. **Frontend tests assert listener callbacks, not VTU `emitted()`:** `emitted()` does not record
   `<script-setup>` emits in this harness (the same reason three pre-existing component tests fail).
   The component test passes `onUpdate:editConfig` / `onUpdateConfig` spies and asserts those.
6. **400 mapping only on `AdminConfigUpdateHandler`** (the endpoint the frontend uses), per the plan;
   `AdminSystemPutHandler` (PUT `/admin/api/system`) was left unchanged.
7. **`RegisterSearchProvider` removed (post-implementation review).** It was a public setter whose only
   caller was its own test — dead code under Constitution IV.4 and a speculative seam against the
   "fixed typed set" decision. `GetSearchProvider` (used by the wiring) remains; providers are added as
   a table row. The general rule is now in `.agents/skills/clean-code/SKILL.md` (§7 + smell T10).
8. **Named consumer-side interfaces replace the inline `interface { ... }` params.** The wiring now
   uses `assistant.AgentStackContext` (for `InitializeAgentStack`) and `assistant.configSecretsReader`
   (shared by `initSearchTools`/`initCommunicationTools`), defined in `assistant` so `*app.AppContext`
   satisfies them implicitly. The inline interfaces also carried `GetSystem()`, which no call site
   used — dropped (and from its mocks). Rule recorded in `engineering-practices` (Go Idioms).
9. **Tavily auth corrected to `Authorization: Bearer` (found in manual E2E).** Tavily's current
   quickstart requires the key in the `Authorization: Bearer tvly-…` header; the legacy JSON-body
   `api_key` field now returns `401 {"detail":{"error":"Unauthorized: missing or invalid API key."}}`.
   `searchproviders/search_tavily.go` now sends the Bearer header and omits `api_key` from the body
   (covered by `search_tavily_test.go`). All three provider constructors also `TrimSpace` the key to
   survive paste artifacts.

## Guiding principle — reuse the existing flow (no parallel systems)
- **Secrets stay in the existing `secrets.json`** (single root, AES-256-GCM via `master.key`). The
  key is written with the existing `SecretStore.SetSecret(models.CategorySearch, <provider>, key)`
  → entry ID `search:<provider>` in the existing `"tools"` provider group
  (`secrets_store.go:228-266`). Same file, masking, and endpoint connectors use. **No new secret
  store.**
- **Config stays in `registry.json` → `RegistryData.Search`.** Already threaded through GET
  `/admin/api/state`, GET `/admin/api/config`, PUT `/admin/api/config`
  (`SystemUpdatePayload.Search`, `app_context_system.go:199/225`). We only add fields.
- **Provider code lives in the `tools/searchproviders` subpackage** — the parent `tools` package owns
  the contract + `InternetTools` facade in `tools/search.go`, and the subpackage owns the concrete
  providers and the explicit registration table (mirrors the existing `tools/communication.go` +
  `tools/notifiers/` split; also keeps `tools/` under the ~15-file package soft cap). There is
  deliberately **no `init()` self-registration** (the connector pattern's `init()` linkage is fragile;
  an explicit table is the repo rule: `engineering-practices` "No `init()`"). The subpackage imports
  the parent for the contract; the parent never imports the subpackage.
- **Settings is one new component + one tab**, mirroring `CommunicationSettings.vue`.

## What already exists (do not rebuild)
- `backend/internal/core/tools/search.go` — `SearchProvider` interface, `TavilyProvider`,
  `InternetTools` (uses the guardrailed `NetworkTools.HTTPClient()`).
- `backend/internal/core/tools/manifests/search.json` — tool schema + guardrails (**unchanged**).
- Registry wiring: `registerSearchTools` (`registry.go:494`), `initSearchTools` (`registry.go:271`).
- Guardrails: `validateSearch` (Enabled / max_query_len / blocked_sites — **unchanged**),
  `DisabledToolNames`, `scopeTierDisabled`, host network switch.
- `models.SearchConfig` is an empty struct (`config.go:424`) whose plumbing is complete.
- Frontend: `AdminApiService.fetchToolSecret/saveToolSecret/deleteToolSecret`.

## Decisions
- **Providers:** Tavily (keep), Brave, SerpAPI — a fixed typed set, not an open list.
- **Unconfigured:** `internet_search` is hidden from the schema via a live predicate on the guardrail
  engine (same mechanism as `SetHostNetworkAllowed`). **No execution-time guardrail backstop** — a
  residual call returns the tool's own `ErrSearchNotConfigured` (record-and-continue).
- **Live, no restart:** resolve provider+key lazily per call from live `appCtx.GetRegistry()` /
  `appCtx.Secrets()` (both verified live in-memory reads). Schema visibility updates on the next run.
- **Registration:** an explicit package-level table read by `GetSearchProvider` — no runtime registration
  seam (the provider set is a fixed typed enum; adding one is a table row).
- **Availability is one source:** a provider is "available" iff the selected provider is registered
  and (it does not require a key OR its `search:<provider>` secret is non-empty). One helper feeds both
  the tool resolver and the guardrail schema gate.
- **No new timeout:** the existing per-tool timeout bounds a search call; providers do not add their own.

## Backend changes

### 1. `backend/models/config.go` — typed enum + config (leaf, no imports)
```go
type SearchProvider string
const (
    SearchProviderTavily  SearchProvider = "tavily"
    SearchProviderBrave   SearchProvider = "brave"
    SearchProviderSerpAPI SearchProvider = "serpapi"
)
func SearchProviderIDs() []SearchProvider          // canonical, ordered (Tavily first)
func (p SearchProvider) Valid() bool               // "" is valid (unset → default)

var ErrInvalidSearchProvider = errors.New("invalid search provider")

type SearchConfig struct {
    Provider   SearchProvider `json:"provider,omitempty"`
    MaxResults int            `json:"max_results,omitempty"`
}
func (c SearchConfig) Validate() error             // "" provider OK; unknown → %w ErrInvalidSearchProvider;
                                                   // MaxResults 0 OK, else 1..MaxSearchMaxResults
func DefaultSearchConfig() SearchConfig            // Provider: Tavily, MaxResults: DefaultSearchMaxResults

const (
    DefaultSearchMaxResults = 5
    MaxSearchMaxResults     = 20
)
```
- JSON tags only (registry.json is JSON; no `yaml` noise). `DefaultSearchConfig` is **used** by the
  resolver's default path (not dead code).
- Tests appended to the existing **`backend/models/config_test.go`** (mirrors `config.go`): empty
  config valid; unknown provider rejected; `MaxResults` bounds (0 / 1 / max / max+1).

### 2. `backend/internal/core/tools/search.go` — contract + lazy resolver
- Keep `SearchProvider` interface, `SearchResult`.
```go
type SearchProviderConfig struct {          // groups the inputs — ≤3 params
    APIKey     string
    Client     *http.Client                 // injected guardrailed client (Constitution I.2)
    MaxResults int
}

type SearchProviderSpec struct {
    RequiresKey bool
    New         func(cfg SearchProviderConfig) (SearchProvider, error)   // constructors never take ctx
}
// The provider table + GetSearchProvider live in tools/searchproviders (see #3).

var ErrSearchNotConfigured = errors.New("internet search is not configured")

type ProviderResolver func(ctx context.Context) (SearchProvider, error)
type InternetTools struct{ resolve ProviderResolver }
func NewInternetTools(resolve ProviderResolver) *InternetTools

// nil-safe (Null Object): nil receiver or nil resolver → ErrSearchNotConfigured.
func (i *InternetTools) Search(ctx context.Context, query string) ([]SearchResult, error)
```
- `Search`: reject empty/whitespace query; resolve; wrap provider errors with `%w`. No
  `http.DefaultClient` fallback (Constitution I.1). **No provider-level timeout const** — the tool
  executor's timeout applies.
- `TavilyProvider` moves out of this file (see #3).

### 3. One file per provider (in `tools/searchproviders`) + a central explicit table
- `backend/internal/core/tools/searchproviders/search_tavily.go` — `TavilyProvider` (POST, JSON body; moved from `search.go`).
- `backend/internal/core/tools/searchproviders/search_brave.go` — Brave (`GET .../res/v1/web/search`, header `X-Subscription-Token`).
- `backend/internal/core/tools/searchproviders/search_serpapi.go` — SerpAPI (`GET https://serpapi.com/search.json`).
- Endpoints/param names are named consts per file. **Implement Tavily first; add Brave/SerpAPI only
  after verifying each request/response shape against its official docs — drop any provider that
  cannot be verified rather than ship it untested.** (All three were verified against the live docs.)
- Per-provider contract: reject a nil injected client; reject a missing key when `RequiresKey`; build
  GET params with `url.Values`; bound bodies with `io.LimitReader` (success **and** error paths);
  validate each result URL with `net/url` (require `http`/`https`) and skip malformed entries; wrap
  non-2xx as `fmt.Errorf("... (status %d): %w", ...)`; never log the key or the query; no retries.
- `backend/internal/core/tools/searchproviders/search_registry.go` — one explicit table, no `init()`,
  no `sync.Once` (also the home of the shared provider plumbing: response-body bound, `url.Values`
  normalisation, URL validation, error-body snippet — those are provider-only and must stay in the
  subpackage so `tools` keeps no leaky exported helpers):
```go
var searchProviderSpecs = map[models.SearchProvider]tools.SearchProviderSpec{
    models.SearchProviderTavily:  {RequiresKey: true, New: newTavilyProvider},
    models.SearchProviderBrave:   {RequiresKey: true, New: newBraveProvider},
    models.SearchProviderSerpAPI: {RequiresKey: true, New: newSerpAPIProvider},
}
```
  `GetSearchProvider` (exported from `searchproviders`) is the read seam the wiring uses. There is
  deliberately no registration setter — the set is fixed, and adding a provider is a table row.

### 4. `backend/internal/core/assistant/registry.go` — live resolver + availability
- Widen the `appCtx` interface with `GetRegistry() models.RegistryData`.
- **One helper feeds both** the tool and the predicate (no drift):
```go
// searchSelection is the live resolution (provider, spec, key, max, ok) — one
// cohesive struct instead of a 4-value return.
type searchSelection struct {
    provider models.SearchProvider
    spec     tools.SearchProviderSpec
    key      string
    max      int
    ok       bool
}
func searchTarget(registry func() models.RegistryData, secrets models.SecretsStore) searchSelection

func initSearchTools(appCtx configSecretsReader, network *tools.NetworkTools) (*tools.InternetTools, func() bool)
```
  (`configSecretsReader` is a named consumer-side interface — `{Secrets(); GetRegistry()}` — sharing
  the narrow read surface with `initCommunicationTools`.)
  - `resolve(ctx)`: `searchTarget(...)` → `sel.spec.New(tools.SearchProviderConfig{APIKey: sel.key, Client: network.HTTPClient(), MaxResults: sel.max})`. The spec comes from `searchproviders.GetSearchProvider`.
  - `available()`: `searchTarget(...)` → the `ok` result (no I/O, no ctx — safe from the guardrail gate).
- `registerSearchTools` (`registry.go:494`): call `r.Search.Search(...)` directly — `InternetTools` is
  nil-safe, so the `r.Search == nil` branch is deleted (no null check needed).
- Log resolution failures with `logging.Warn` (provider name + error class; never the key or query).

### 5. `backend/internal/core/assistant/guardrails/guardrails.go` — hide when unconfigured
- Add `searchAvailable func() bool` + `SetSearchAvailable(func() bool)` (mirror of
  `SetHostNetworkAllowed`), plus nil-safe `searchUnavailable() bool` (nil ⇒ no gate, so existing
  behaviour/tests are unchanged).
- Add one helper and use it in both places (DRY):
```go
func (e *GuardrailEngine) searchDisabled(cfg models.AgentGuardrailsConfig) bool {
    return !cfg.Search.Enabled || e.searchUnavailable()
}
```
  - `DisabledToolNames`: `add(models.ToolInternetSearch, e.searchDisabled(cfg))`.
  - `scopeTierDisabled`: `add(models.ToolInternetSearch, lanScope || e.searchDisabled(cfg))`.
- **No change to `validateSearch`** (see correction #1).
- Tests in the existing `guardrails_test.go`: hidden when unavailable; visible when available and
  enabled; still hidden when `Search.Enabled == false`; scope behaviour unchanged (nil predicate ⇒
  old behaviour).

### 6. `InitializeAgentStack` (`registry.go:306`) — single wiring point
- `search, searchAvailable := initSearchTools(appCtx, network)`; pass `search` to
  `NewLocalToolRegistry` (signature unchanged); call
  `grEngine.SetSearchAvailable(searchAvailable)` immediately after `SetHostNetworkAllowed`. One
  composition root, no scattered wiring.

### 7. Save validation (boundary) + surface the option list
- Validate once in `backend/internal/app/app_context_system.go` before the branch (one helper shared
  by both branches):
  `if req.Search != nil { if err := req.Search.Validate(); err != nil { return err } }`.
- Map the typed error to 400 in `system_handlers.go` `AdminConfigUpdateHandler` (alongside the
  existing `ModelNotFoundError` check: `errors.Is(err, models.ErrInvalidSearchProvider)`).
- `backend/internal/transport/http/handlers/admin_handlers.go`: add
  `SearchProviders []string \`json:"search_providers"\`` to `adminConfigView` (next to
  `LoopStrategyOptions`, `:204`), populated from `models.SearchProviderIDs()` in the state handler
  (`:337-351`). This is the `loop_strategy_options` precedent — the frontend never hardcodes the list.
  (The FE reads state, so the `/admin/api/config` view needs no change.)

## Frontend changes

1. `frontend/src/types/admin.ts`
   - `SettingsTab` (`:5`) += `'search'`.
   - `SearchProvider` union (`'tavily' | 'brave' | 'serpapi'`) and `SearchConfig`
     (`{ provider?: SearchProvider; max_results?: number }`).
   - `search?: SearchConfig` on `GlobalConfig` (`:148-169`) and `search_providers?: string[]`
     (mirrors `loop_strategy_options`, `:167`).
   - **Types only** — no const arrays here.
2. `frontend/src/constants/search.ts` (new) — `SEARCH_PROVIDER_IDS` + `SEARCH_PROVIDER_LABELS`
   (values live in `constants/`; comment notes the backend mirror `models.SearchProviderIDs()`).
   The const array is a **fallback/typing aid only** — the dropdown is driven by the backend list.
3. `frontend/src/constants/providers.ts` — add `PROVIDER_META.search` (`:23-35`); the
   `Record<SettingsTab, …>` type forces it (vue-tsc safety net). Follow the existing `PROVIDER_META`
   convention for the tab glyph (that record is the established home for tab icons; the
   `constants/icons.ts` rule governs non-tab symbols).
4. `frontend/src/domain/settings.ts` — exclude `'search'` in `isProviderTab` (`:11`); add it to the
   Extensions group (`:32`).
5. `frontend/src/composables/models/useProviders.ts` — add `'search'` to the `base` tab list (`:40`).
6. `frontend/src/composables/useConnectorTokens.ts` → **`useToolSecrets(category)`** (rename +
   category param, keeping `{masked, dirty}` + `load`/`saveDirty`). Update the single existing caller
   `CommunicationSettings.vue` to `useToolSecrets("connector")`. Rationale: extending an existing
   shared abstraction; the same non-trivial masked/dirty/save flow must not be copy-pasted.
7. **New** `frontend/src/components/settings/SearchSettings.vue` — same props/emits as
   `CommunicationSettings.vue` (`editConfig` / `update:editConfig` / `updateConfig`). Provider
   `<select>` driven by `config.search_providers` (fallback `SEARCH_PROVIDER_IDS`), key field (masked
   load + dirty save via the composable), `max_results` input (bounds mirrored from the backend),
   helper text stating the tool is hidden until configured. Cover Loading / Empty / Success / Error
   states; label every control (a11y). No API calls in the component beyond the composable/service.
8. `frontend/src/components/settings/Settings.vue` — import `SearchSettings` and add
   `v-show="activeTab === 'search'"` next to the Communication block (`:264-271`).
9. Tests: `frontend/src/__TESTS__/components/settings/SearchSettings.component.test.ts` (happy path +
   error path + provider switch) and `frontend/src/__TESTS__/composables/useToolSecrets.test.ts`
   (mirroring the source tree), reusing the existing `GlobalConfig` fixture factory.

## Docs (Constitution V.2 — spec-first)
- `docs/SPECS/guardrails.md` §6 — `internet_search` is now also schema-hidden when no provider is
  configured (not only when `Search.Enabled == false` / network off); record the availability predicate.
- `.agents/skills/engineering-practices/SKILL.md` — add a short "When adding a search provider" recipe
  (const + enum + one table row + the `tools`-package drift test; the `search:<provider>` secret path).
- `docs/architecture.md` — one-line pointer to that recipe (no duplicated checklist).
- `docs/INDEX.md` + `docs/PLANS/README.md` — already registered; keep in sync if scope shifts.

## Implementation order (TDD — Red → Green → Refactor, `.agents/skills/tdd-guide/SKILL.md`)
0. Baseline: `cd backend && go build ./... && go test ./...`;
   `cd frontend && npm test && npm run build`.
1. `models`: enum + `SearchConfig` + `Valid`/`Validate` + sentinel; extend `config_test.go`.
2. `tools`: `InternetTools` nil-safe lazy resolver + `ErrSearchNotConfigured`; `search_test.go`.
3. `tools`: Tavily moved to `searchproviders/search_tavily.go` + the explicit table and shared provider
   plumbing in `searchproviders/search_registry.go`; `searchproviders/search_tavily_test.go`
   (httptest: request shape/status/parse, missing key, nil client, malformed URL skipped, result cap),
   plus `searchproviders/search_registry_test.go` (table drift + helper tests). Keep
   `manifest_sync_test.go` green.
4. `searchproviders`: add Brave, then SerpAPI — each only after doc verification; mirror-named `_test.go` each.
5. `guardrails`: `SetSearchAvailable` + `searchDisabled` helper; extend `guardrails_test.go`.
6. `assistant`: `searchTarget` + `initSearchTools` + `registerSearchTools` + `InitializeAgentStack`;
   update `registry_test.go` (nil-search registry still works via the nil-safe tool).
7. `app` + handlers: save validation, 400 mapping, `search_providers` surfacing; handler tests.
8. Frontend: types/constants/domain/composable + tests.
9. `SearchSettings.vue` + `Settings.vue` + component test.
10. Docs sync (`.agents/skills/documentation-stewardship/SKILL.md`).

## Verification
Backend (from `backend/`):
```bash
go build ./...
go test ./... -race -coverprofile=coverage.out -covermode=atomic
go vet ./...
go run ./tools/check-complexity/          # complexity <= 12
```
Also assert: functions < 80 lines, complexity < 10, no magic values, **no unused consts**, no
`init()`, signatures <= 4 params (grouped structs otherwise), every error wrapped with `%w` and
matched via `errors.Is`/`errors.As`, no goroutines added. `manifest_sync_test.go` stays green.

Frontend (from `frontend/`):
```bash
npm test && npm run build
```

Manual end-to-end:
1. Start backend (`cd backend && go run main.go`), open Settings → Search.
2. No key: the model tool list does **not** include `internet_search`; a forced residual call returns a
   clear tool error (no approval prompt, no crash).
3. Choose a provider, enter its key, Save → `PUT /admin/api/secrets/tools?category=search&provider=<p>`;
   `secrets.json` holds `search:<p>` in the `"tools"` group (same file as connectors).
4. Start a new chat run → `internet_search` appears; a call returns real results (rendered generically
   by `ToolCallSegment.vue` / `toolLabel` via the `query` arg — no renderer change).
5. Confirm no restart was needed between steps 3 and 4; confirm an unknown provider and an
   out-of-range `max_results` are rejected with 400 on save.
