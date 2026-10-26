# Change Checklists

Ordered file lists for common change types. Follow the list for your change type before editing.

Part of the [architecture reference](../architecture.md).

## File Change Checklist

When adding a new model-level field, update these files:

1. `models/config.go` or `models/infrastructure.go` — type definition
2. `internal/transport/http/handlers/registry_handlers.go` — add/update request structs
3. `internal/transport/http/handlers/admin_handlers.go` — view struct
4. `internal/transport/http/handlers/admin_view.go` — view mapping
5. `internal/core/llm/manager.go` — if field affects runtime behavior
6. `internal/testing/mocks/manager.go` — if interface changed
7. Frontend component (if UI field)

When adding a new tool category (e.g. a new category like "communication"):

1. `models/tools.go` — add category constant (e.g. `CategoryCommunication`)
2. `internal/core/tools/manifests/{category}.json` — tool manifest (embedded)
3. `internal/core/tools/{category}.go` — implementation file with category struct + methods
4. `internal/core/toolregistry/registry.go` — registration wiring:
   - Add field to `LocalToolRegistry` struct
   - Add `init{Category}Tools()` helper function
   - Add `register{Category}Tools()` method on `LocalToolRegistry`
   - Call both from `InitializeAgentStack` and `registerAll()`
5. Frontend: add any category-specific UI (settings, status indicators)

When adding a single tool:

1. `models/tools.go` — constant
2. `internal/core/tools/manifests/{tool}.json` — manifest (embedded)
3. `internal/core/tools/{tool_category}.go` — implementation
4. `internal/core/toolregistry/registry.go` — registration (add field to `LocalToolRegistry`, add `register{Category}Tools()`, call from `registerAll()`, add `init{Category}Tools()` helper, wire in `InitializeAgentStack`)

When adding a communication connector:

1. `models/config.go` — `ConnectorConfig.Type` is the switch key (no struct change needed — generic map)
2. `internal/core/tools/communication.go` — implement `Connector` interface (Send + Name), use injected `*http.Client` from `NetworkTools.HTTPClient()`
3. `internal/core/tools/notifiers/` — add a self-registering `init()` calling `tools.RegisterConnectorFactory("your_type", factory)`. See `telegram.go` for a reference implementation. Do NOT edit `initCommunicationTools` or `registry.go` — the registry handles it dynamically.
4. Frontend `CommunicationSettings.vue` — add `<option value="your_type">` dropdown entry

When adding an internet-search provider: see the recipe in `.agents/skills/engineering-practices/SKILL.md` ("When adding a search provider"). The contract + `InternetTools` stay in `internal/core/tools/search.go`; providers and the explicit registration table live in `internal/core/tools/searchproviders/`.

When adding a prompt:

1. `internal/core/assistant/prompts/templates.go` — ONLY location for prompt text
2. Logic file (agent.go, tool_call_parser.go) — uses the template, never inlines strings

## Adding a Frontend Settings Tab Checklist

1. Add tab name to `SettingsTab` type in `frontend/src/types/admin.ts`
2. Add the label in `frontend/src/constants/providers.ts` (the tabs and section row show labels only)
3. If the tab is NOT a cloud provider, add exclusion to `isProviderTab()` in `frontend/src/domain/settings.ts`
4. Put the section under one of the six Settings tabs in `SETTINGS_GROUP_SECTIONS` (`frontend/src/constants/settings.ts`); cloud providers are the exception — they are discovered at runtime and `getSettingsTabGroups()` lists them under Models
5. Create the settings component in `frontend/src/components/settings/`, built from `Panel` sections and `FormField` controls
6. Import the component in `frontend/src/views/SettingsView.vue` and add a `v-show="activeTab === 'your-tab'"` block (sections stay mounted, so edits survive switching category)
7. If it edits the shared configuration, end the section with `SettingsActions` bound to `useConfig().isDirty` (Discard / Save). A draft of its own (secrets, host settings) emits `dirty-change` so the page's single unsaved-change guard covers it
8. Run `npm run build` — TS errors will catch any missing icon/label entries
9. The tab is a route section (`/settings/<tab>`) automatically — an unknown section renders the in-shell not-found page. Link to it with `toSettings('<tab>')` from `frontend/src/router/routes.ts`, never a hand-built path

## New Backend Endpoint Checklist

1. Handler in `internal/transport/http/{thing}_handlers.go`
2. Route registration (in `router.go` or `main.go` route setup)
3. Types/response structs in the same handler file
4. Frontend: endpoint constant in `api.ts`
5. Frontend: service method in `{thing}Service.ts`
6. Frontend: types in `types/{thing}.ts`
7. Frontend: composable in `composables/use{Thing}.ts`
8. Frontend: view component in `components/{thing}/{Thing}.vue`
