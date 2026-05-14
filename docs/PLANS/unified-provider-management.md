# Unified Provider & Model Management

## Problem Summary

Today, users manage cloud provider configuration across **three separate places**:

| Task | Location | Component |
|------|----------|-----------|
| Add/edit API keys | Settings → [Provider] tab | `ApiKeySettings.vue` |
| Add/edit models | Dashboard → Cloud tab | `ModelManager.vue` + `CloudFields.vue` |
| Edit/delete models | Settings → Model Catalogue | `ModelCatalogue.vue` |

This creates a confusing UX: after adding an API key in Settings, the user must navigate to the Dashboard to add a model that uses that key. The link between a model and its API key is a loose `CredentialID` string match — easily orphaned when keys are deleted. For local OpenAI-compatible endpoints, the user must configure a `base_url` per key AND add a model entry in two separate forms.

Additionally, several concrete bugs were discovered during investigation:

- **Fixed**: Per-key `base_url` was shadowed by provider default `BaseURL` (`registrar.go` Build step order)
- **Fixed**: Clearing all API keys leaves orphaned model configs in the registry
- **Fixed**: Deleting a single API key also orphans models referencing that key
- **Fixed**: Two redundant model management UIs (`ModelManager` on Dashboard + `ModelCatalogue` in Settings)
- **Fixed**: No proactive notification when secrets change (no `OnChange` subscriber for secrets store)
- **Fixed**: `SecretStore.mu` field declared but never used

---

## Design Goals

1. **One place to rule them all** — All provider configuration (keys, models, provider settings) lives under a single provider tab in Settings
2. **Explicit key↔model binding** — Models are created *under* a specific API key; deleting a key cascades to its models
3. **Streamlined local-endpoint flow** — Adding an OpenAI-compatible local server is one workflow, not three separate forms
4. **Consistent delete semantics** — Both single-key and clear-all-key deletions clean up orphaned models
5. **Dashboard becomes monitoring-only** — Model management moves to Settings; Dashboard shows status/metrics/runtime

---

## Proposed Architecture

### Data Model Changes

**New field on `ModelRegistryEntry`:**
```go
type ModelRegistryEntry struct {
    // ... existing fields ...
    CredentialID string `json:"credential_id,omitempty"` // existing — becomes foreign-key
    // No new fields needed — the CredentialID link is sufficient
}
```

The `CredentialID` already links a model to a key. The issue is not the data model — it's the UX and the cleanup logic.

**Cascade rule:** When a key is deleted, any model whose `CredentialID` matches that key's `Name` or `ID` (or is empty and the key was the only/last key for that provider) is also removed from the registry catalogue.

### UI Architecture

```
Settings → [Provider] tab
├── Provider Configuration Card
│   ├── Base URL (provider default)
│   ├── Project ID / Region (Gemini/Vertex only)
│   └── [Save Provider Config]
│
├── API Keys Card
│   ├── Key list (name, masked value, base_url badge, model count)
│   ├── Add/edit/delete key (inline panel — existing)
│   ├── "Clear All Keys" button (existing)
│   └── Test Connection button (existing)
│
└── Models Card  ← NEW — moves here from Dashboard + ModelCatalogue
    ├── Models grouped by API key (or "No key" bucket)
    ├── Model list (name, model_id, status)
    ├── "Add Model" form (auto-discovers models from the endpoint)
    ├── Edit/delete model (inline)
    └── Prefill / tuning toggles
```

**Dashboard after migration:**
- Local tab: unchanged (local model management stays)
- Cloud tab: becomes a **read-only status view** — shows cloud model health, recent activity, token usage. "Add Model" link redirects to Settings.

### Cascade Delete Semantics

| Action | Effect on models |
|--------|-----------------|
| Delete single key | Remove all models with `CredentialID` matching that key's `Name` or `ID` |
| Clear all keys for provider | Remove ALL models for that provider (already implemented) |
| Delete last remaining key | Same as "Clear all" — remove all models for that provider |

**No-CredentialID models (empty string):** These use the first available key at runtime. If no keys remain for the provider, these models are also removed.

---

## Implementation Plan

### Phase 1 — Backend: Cascade Delete + Bug Fixes ✅

#### Step 1.1: Single-key delete cascades to orphaned models ✅
**File:** `internal/transport/http/secrets_handlers.go` — `AdminProviderKeyDeleteHandler`

Before deleting the key, look up the key's `Name` and `ID`. After deleting, remove all models from the registry whose `CredentialID` matches either.

```go
// Before deleting, capture the key identity for cascade
keys := h.admin.Secrets().GetProviderKeys(provider)
var targetName, targetID string
for _, k := range keys {
    if k.ID == keyID {
        targetName = k.Name
        targetID = k.ID
        break
    }
}

// Delete the key
if err := h.admin.Secrets().DeleteProviderKey(provider, keyID); err != nil { ... }

// Cascade: remove models that reference this key
h.admin.UpdateRegistry(func(reg *models.RegistryData) {
    out := reg.Catalogue[:0]
    for _, m := range reg.Catalogue {
        if m.ProviderID != provider {
            out = append(out, m)
            continue
        }
        if m.CredentialID != "" && m.CredentialID != targetName && m.CredentialID != targetID {
            out = append(out, m)
        }
        // Also keep models that have a different key to fall back to
        if m.CredentialID == "" && len(remainingKeysAfterDelete) > 0 {
            out = append(out, m)
        }
    }
    reg.Catalogue = out
})
```

**Files:** `secrets_handlers.go`

#### Step 1.2: Add secrets OnChange subscriber ✅
**File:** `internal/app/app_context.go` — `registerSubscribers`

Register an `OnChange` callback on the encrypted secrets store that calls `manager.Sync()` (or at minimum logs the change). This ensures the runtime is aware of credential changes without requiring a restart.

```go
m.dataMgr.EncryptedSecretStore().OnChange(func(data models.EncryptedSecretData) {
    if m, ok := mgr.(*llm.LLMRuntimeManager); ok {
        m.Sync()
    }
})
```

**Files:** `manager.go` (expose `EncryptedSecretStore()`), `app_context.go`

#### Step 1.3: Remove dead `mu` field from `SecretStore` ✅
**File:** `internal/platform/storage/secrets_store.go`

Remove the unused `mu sync.RWMutex` field — the underlying `Store` provides its own thread safety.

#### Step 1.4: Add `DeleteAllProviderKeys` to `AdminService` interface ✅
**File:** `internal/transport/http/services.go`

Add a dedicated method to the `AdminService` interface so the cascade logic is centralized, not duplicated in the handler:

```go
// AdminService interface addition:
DeleteProviderWithCleanup(provider string) error
```

Implemented in `AppContext` — deletes all keys + cascades to models. The handler calls this single method.

**Files:** `services.go`, `app_context.go`, `secrets_handlers.go`

---

### Phase 2 — Frontend: Unified Provider Settings ✅

#### Step 2.1: Create `ProviderModelsCard.vue` ✅
**New file:** `frontend/src/components/settings/ProviderModelsCard.vue`

A new component that replaces the cloud model management currently split across `ModelManager.vue` and `ModelCatalogue.vue`. Features:

- **Props:** `provider` (ProviderType), `apiKeys` (APIKeyItem[])
- **Model list:** Grouped by API key. Each model shows: name, model_id, key it's bound to, status indicator (configured/active)
- **"Add Model" inline form:**
  - Select API key (dropdown of available keys for this provider)
  - Model ID (text input or select from auto-discovered list)
  - Friendly name (auto-derived, editable)
  - Prefill toggle
  - "Add" button
- **Auto-discover:** When a key is selected, calls `GET /admin/api/providers/models?provider=X&api_key_name=Y` to list available models from the endpoint
- **Edit/delete:** Inline edit panel (reuse `ModelItem.vue` patterns)
- **Empty state:** "No models configured for this provider. Add an API key first, then add a model."

#### Step 2.2: Integrate `ProviderModelsCard` into Settings ✅
**File:** `frontend/src/components/settings/Settings.vue`

Add the `<ProviderModelsCard>` component to each cloud provider tab, below the API keys card:

```html
<ApiKeySettings ... />
<div class="form-divider"></div>
<ProviderModelsCard
  :provider="provider"
  :apiKeys="providerKeys[provider] || []"
/>
```

The `ProviderModelsCard` uses `useModels()` composable for add/update/delete operations (same API, same backend).

#### Step 2.3: Simplify Dashboard cloud tab to read-only ✅
**File:** `frontend/src/components/dashboard/Dashboard.vue`

The Cloud tab in the dashboard changes from model management to a read-only status view:
- Cloud model health cards (model name, provider, key used, status)
- Link to Settings: "Manage cloud models in Settings →"
- Remove `ModelManager` from the cloud tab

**File:** `frontend/src/components/ModelManager.vue`

This component stays but is only used for the Dashboard local tab. The cloud-specific logic (`CloudFields.vue` usage) becomes dead code and can be cleaned up later.

#### Step 2.4: Remove `ModelCatalogue.vue` (or repurpose) ✅
**File:** `frontend/src/components/settings/ModelCatalogue.vue`

Once `ProviderModelsCard` is in place, the Model Catalogue tab is redundant for cloud providers. Options:
1. **Remove entirely** — cleanest
2. **Keep for local models only** — renamed to "Local Models"
3. **Keep as read-only overview** — all models across all providers

Recommended: **Option 1** — remove the catalogue tab from the sidebar, delete the component. The functionality is now in each provider tab.

#### Step 2.5: Update sidebar navigation ✅
**File:** `frontend/src/components/settings/Settings.vue` — `settingsGroups` computed

Remove the "Model Catalogue" tab from the sidebar groups. All model management is now per-provider.

**File:** `frontend/src/domain/settings.ts` — `getSettingsGroups`

Remove the catalogue entry.

---

### Phase 3 — UX Improvements ✅

#### Step 3.1: Key↔model count badge ✅
Show a badge on each API key in `ApiKeySettings.vue` indicating how many models reference it. Clicking the badge scrolls to the models section.

#### Step 3.2: Delete key confirmation warns about models ✅
When deleting a single key that has models referencing it, show: "This key is used by N model(s). Deleting it will also remove those models."

#### Step 3.3: Endpoint URL preview
In the model form, show the resolved endpoint URL (composed from key `base_url` + provider default or manifest default). This helps users verify the correct endpoint before adding a model.

#### Step 3.4: Test connection lists models
The existing "Test Connection" button in `ApiKeySettings.vue` already tests connectivity. Enhance it to also show the available models from the endpoint, so the user can see what models are available before adding them.

---

### Phase 4 — Optional: Local Endpoint Quick-Start

#### Step 4.1: One-step "Add Local Endpoint" flow
For OpenAI-compatible local servers (like llama.cpp's `/v1` endpoint), provide a quick-start form that:
1. Asks for the endpoint URL + API key (if any)
2. Auto-discovers models from that endpoint
3. Lets the user pick which models to register
4. Creates the API key + model entries in one atomic operation

This replaces the current three-step flow: add provider config → add API key → add model.

---

## Files Changed (Complete List)

### Backend
| File | Change |
|------|--------|
| `internal/transport/http/secrets_handlers.go` | Cascade delete for single key; extract `DeleteProviderWithCleanup` |
| `internal/transport/http/services.go` | Add `DeleteProviderWithCleanup` to `AdminService` |
| `internal/app/app_context.go` | Implement `DeleteProviderWithCleanup`; add secrets `OnChange` subscriber |
| `internal/platform/storage/secrets_store.go` | Remove dead `mu` field |
| `internal/platform/storage/manager.go` | Expose `EncryptedSecretStore()` for OnChange registration |
| `internal/core/llm/manager.go` | (minor) Accept `Sync()` trigger from secrets change |

### Frontend
| File | Change |
|------|--------|
| `src/components/settings/ProviderModelsCard.vue` | **New** — unified model management per provider |
| `src/components/settings/Settings.vue` | Add `ProviderModelsCard` to each provider tab; remove catalogue from sidebar |
| `src/components/dashboard/Dashboard.vue` | Cloud tab becomes read-only status |
| `src/components/ModelManager.vue` | Cloud-specific logic removed (local-only) |
| `src/components/models/CloudFields.vue` | (may become dead code, remove or archive) |
| `src/components/settings/ModelCatalogue.vue` | Removed |
| `src/components/settings/ApiKeySettings.vue` | Add model count badge; enhance delete confirmation |
| `src/domain/settings.ts` | Remove catalogue from settings groups |
| `src/composables/useModels.ts` | Ensure add/update/delete works from Settings context |
| `src/services/adminService.ts` | (no changes — uses existing endpoints) |

### Removed
| File | Reason |
|------|--------|
| `src/components/settings/ModelCatalogue.vue` | Superseded by per-provider model cards |

---

## Undiscovered Bug Fixes Included

1. **Single-key delete orphans models** — ✅ Phase 1 Step 1.1
2. **No secrets change notification** — ✅ Phase 1 Step 1.2
3. **Dead `mu` field in SecretStore** — ✅ Phase 1 Step 1.3
4. **Two redundant model UIs** — ✅ Phase 2 consolidates into one
5. **`SecretData.Version` inconsistent serialization** — The Version field is stored in the encrypted wrapper (`EncryptedSecretData.Version`) but never inside the encrypted payload. This is not a functional bug (Version is unused) but is misleading. Not addressed in this pass.

---

## Rollout Strategy

1. **Phase 1 first** — Backend cascade delete + bug fixes ✅
2. **Phase 2 next** — Frontend unification ✅
3. **Phase 3** — Polish ✅
4. **Phase 4** — Optional quick-start flow. Requires the most new code. Not yet implemented.
