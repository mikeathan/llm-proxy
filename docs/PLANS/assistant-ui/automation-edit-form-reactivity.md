# Fix Automation Edit Form — reactive populate via derive-don't-sync refactor

**Status:** `proposed`
**Created:** 2026-08-01
**Subsystems:** assistant-ui (SPEC-003), automation (SPEC-007)
**Regressions from:** commit `c140c0e` ("Story/assistant redesign") — extraction of
form logic from `AutomationForm.vue` into the `useAutomationForm` composable.

---

## 0. Goal

Clicking **Edit** on an automation in the Automations tab must populate every
form field (name, task file, trigger, connection, model) on the **first click**,
every time — regardless of page-load timing or tab navigation. The fix removes
the fragile two-way model/provider sync entirely, not just patches it.

Design principle: **single source of truth** (`form.model`), **derive, don't
sync**. Delete interlocking watchers instead of adding more guards.

---

## 1. Root cause (verified)

### Bug 1 — editAutomation captured by value (all fields blank)

`AutomationForm.vue:39-44` passes `props.editAutomation ?? null` — a **plain
value snapshot** taken once at setup — into `useAutomationForm`. The composable's
populate watch (`useAutomationForm.ts:75`, `watch(() => editAutomation?.id, …)`)
tracks a closure variable frozen at mount time (`null`). It fires once
(`immediate: true`, null → reset), then **never again** when the prop changes.
Clicking Edit updates the prop; the form never repopulates.

`AutomationForm` is mounted/unmounted with `leftTab === 'automations'`
(`AgentIde.vue:371`). It only populates when it happens to **remount while
`editAutomation` is already set** (navigated tabs away and back) — the
"works after a few tries" symptom.

### Bug 2 — models/providers also captured by value (why past "simple fixes" failed)

`useAutomationForm.ts:16-17` captures `props.models` and `props.providers` by
value too. Both are computeds over `adminState` in `AgentIde.vue:149-150`, which
loads **asynchronously** (`refreshModels()` in `onMounted`). If the form mounts
before that resolves:

- `filteredModels` (`:106`) is stuck on the stale `[]` → Model dropdown empty.
- `cloudProvidersWithKeys` (`:122`) is stuck on the stale `{}` → Connection
  dropdown has no cloud options.
- The `watch(() => models, { deep: true })` getter (`:96`) returns the **same
  stale array reference** → never fires when the prop is *replaced* → the
  `syncModelSource` late-load re-sync is dead.

So fixing only Bug 1 still leaves blank Connection/Model dropdowns on
fresh-load-→-edit, and late model-load re-sync never runs.

### Trap for naive fixes

Changing only the call site to `toRef(props, 'editAutomation')` breaks everything
— `editAutomation.model` / `.workspace` on a Ref are `undefined`. Composable body
must switch to `.value` too, and `models`/`providers` need the same treatment.

### Dead code (delete during refactor)

- `useAutomationForm.ts:170-214` — `cronType`, `cronEvery`, `cronUnit`,
  `cronDescription` + their watchers. **Duplicated dead logic**: `CronEditor.vue`
  owns its own copies (`CronEditor.vue:15-16,42`) and the form template only binds
  `CronEditor`. The composable's copies are never bound.
- `useAutomationForm.ts:39,53` — `modelSource` ref, set but never read.
- `useAutomationForm.ts:56-73` — `syncModelSource` (replaced by a computed).
- `useAutomationForm.ts:149-162` — `selectedProviderKey` reset watcher with the
  `isStillValid` guard (only existed to protect populate from the two-way sync).
- `useAutomationForm.ts:96-104` — models deep watch (made redundant).
- `cronstrue` import in the composable (only used by deleted cron code).

---

## 2. Refactor design

### 2a. Composable — `frontend/src/composables/automation/useAutomationForm.ts`

`form.model` is the single source of truth; the provider key is a **derived**
`computed` with getter/setter. `models`/`providers` are read **directly from the
`useModels()` store singleton** — not passed in — so there is no value snapshot
to go stale (see "Why this is the better engineered solution" below). Only
`editAutomation` (genuinely parent-owned selection state) and the file-fetch
callback cross the boundary, as a `Ref` and a callback respectively.

```ts
import { ref, computed, watch, type Ref } from "vue"
import { useModels } from "../models/useModels"
import type { Model } from "../../types/model"
import type { Automation } from "../../types/dispatcher"

export type TriggerType = "cron" | "interval" | "manual"

export interface AutomationFormData {
  name: string
  triggerType: TriggerType
  triggerValue: string
  taskFile: string
  strategy: string
  model: string
}

export function useAutomationForm(
  editAutomation: Ref<Automation | null>,
  onFetchFiles: (workspace: string) => void,
) {
  // App-wide admin store singleton: live computeds, so a late adminState load
  // just recomputes the derivations below. No watches.
  const { state } = useModels()
  const models = computed(() => state.value?.models ?? [])
  const providers = computed(() => state.value?.config.providers ?? {})

  const selectedWorkspace = ref("")
  const form = ref<AutomationFormData>(emptyForm())

  // ---- derived model routing ---------------------------------------------
  function modelsForKey(key: string): Model[] {
    if (key === "local") return models.value.filter((m) => m.provider === "local")
    if (!key) return []
    const [provider, keyName] = key.split("/")
    return models.value.filter(
      (m) => m.provider === provider
        && (m.provider_config?.api_key_name || "") === (keyName || ""),
    )
  }

  const selectedProviderKey = computed({
    get: () => {
      const model = models.value.find((m) => m.name === form.value.model)
      if (!model) return ""
      return model.provider === "local"
        ? "local"
        : `${model.provider}/${model.provider_config?.api_key_name || ""}`
    },
    set: (key: string) => {
      form.value.model = modelsForKey(key)[0]?.name ?? ""
    },
  })

  const filteredModels = computed(() => modelsForKey(selectedProviderKey.value))

  const cloudProvidersWithKeys = computed(() => {
    const result: { providerName: string; keys: { name: string; id: string; keyVal: string }[] }[] = []
    for (const [name, p] of Object.entries(providers.value)) {
      if (name === "local") continue
      const keys = (p.api_keys ?? []).map((k) => ({ name: k.name, id: k.id, keyVal: k.name }))
      if (keys.length === 0) continue
      result.push({ providerName: name, keys })
    }
    return result
  })

  // ---- workspace ---------------------------------------------------------
  watch(selectedWorkspace, (ws) => {
    if (ws) onFetchFiles(ws)
    if (!editAutomation.value) form.value.taskFile = ""
  })

  // ---- populate / reset --------------------------------------------------
  watch(editAutomation, (target) => {
    if (!target) {
      resetForm()
      return
    }
    selectedWorkspace.value = target.workspace
    form.value = {
      name: target.name,
      triggerType: (target.trigger as TriggerType) || "cron",
      triggerValue: target.trigger_value || "",
      taskFile: target.task_file,
      strategy: target.strategy,
      model: target.model || "",
    }
  }, { immediate: true })

  function emptyForm(): AutomationFormData {
    return { name: "", triggerType: "cron", triggerValue: "", taskFile: "", strategy: "persistent", model: "" }
  }

  function resetForm() {
    form.value = emptyForm()
    selectedWorkspace.value = ""
  }

  // ---- trigger behaviour -------------------------------------------------
  watch(
    () => form.value.triggerType,
    (newVal, oldVal) => {
      if (oldVal !== undefined && !editAutomation.value) form.value.triggerValue = ""
    },
  )

  const handleSubmit = (): AutomationFormData | null => {
    if (!selectedWorkspace.value || !form.value.name) return null
    return { ...form.value }
  }

  return {
    selectedWorkspace,
    form,
    selectedProviderKey,
    filteredModels,
    cloudProvidersWithKeys,
    handleSubmit,
    resetForm,
  }
}
```

**Why this fixes both bugs:**

- Populate only sets `form.model`; the Connection dropdown populates
  **derivatively** via the `get`. No `syncModelSource` call.
- `models`/`providers` come from the `useModels()` store singleton → their
  computeds recompute when adminState loads late, and `filteredModels`,
  `cloudProvidersWithKeys`, and the derived key all follow. No watch needed.
- The `set` only fires on **user interaction** with the Connection dropdown, so
  it can never clobber a programmatic populate. The `isStillValid` guard dies.
- Bug 2 is eliminated **structurally**: the composable holds no `models`/
  `providers` value to capture. There is no snapshot, so a late adminState load
  cannot leave the form on stale `[]`.

#### Why this is the better engineered solution (vs. props + `toRef`)

The original plan passed `models`/`providers` in as `Ref`s and `toRef`'d them at
the call site. That also fixes the bug (Vue tracks the prop reassignment), but it
keeps three layers of redundant indirection:

1. `AgentIde.vue:149-150` re-derives `models`/`providers` computeds that simply
   wrap what `useModels()` already exposes.
2. The form receives them as props (`:models`/`:providers`, lines 375-376) that
   exist **only** to feed this one component.
3. The composable then `toRef`'s them back into refs — the snapshot class of bug
   can silently recur if a future call site passes a plain value again.

`useModels()` is a module-level singleton (`useModels.ts:9`) that every consumer
already reads directly (AgentIde, useTemplates, useRunningActivity). The form
needs the global model catalog, period — it is not per-call-site injectable
state. Reading it inside the composable:

- removes props 375-376 and the now-dead computeds 149-150;
- kills the stale-snapshot bug at the root, not at each call site;
- matches the codebase's established direct-store-consumption pattern.

`editAutomation` stays a prop: it is parent-owned selection state, and
`toRef(props, "editAutomation")` is the correct minimal bridge.

**Fallback (conservative, if store coupling is rejected):** keep the props-based
signature — `useAutomationForm(toRef(props, "models"), toRef(props, "providers"),
toRef(props, "editAutomation"), cb)`. It is equally correct and testable via
injected refs (no `vi.mock` needed), at the cost of the indirection above. The
rest of the design (derived key, single source of truth) is identical either way.

### 2b. Component — `frontend/src/components/AgentIde/automation/AutomationForm.vue`

- Import `toRef` from `vue`.
- Drop the `models`/`providers` props (the form reads the `useModels()` store):
  ```ts
  const props = defineProps<{
    workspaces: { id: string }[];
    workspaceFiles: Record<string, string[]>;
    hasAutomations: boolean;
    editAutomation: Automation | null;
  }>();
  ```
- Call the composable with just the selection ref and the file-fetch callback:
  ```ts
  const { ..., resetForm } = useAutomationForm(
    toRef(props, "editAutomation"),
    (ws) => emit("fetch-files", ws),
  );
  ```
- Tighten prop typing: `editAutomation?: Automation | null` →
  `editAutomation: Automation | null` (parent always passes it; keeps `toRef`
  type clean as `Ref<Automation | null>`).
- Replace the inline create-path form reset (current `AutomationForm.vue:66-74`)
  with `resetForm()` — one reset implementation, not two.
- Template bindings are unchanged — `v-model="selectedProviderKey"` works with
  the computed getter/setter; `:disabled="!selectedProviderKey"` reads the
  computed value.

---

## 3. Implementation steps (exact)

1. Rewrite `frontend/src/composables/automation/useAutomationForm.ts` per §2a.
   - Delete: `modelSource`, `syncModelSource`, models deep watch,
     `selectedProviderKey` reset watcher, all cron refs/watches, `cronstrue`
     import.
   - Merge the two `selectedWorkspace` watches into one.
   - Read `models`/`providers` from the `useModels()` store singleton.
   - `handleSubmit` → `{ ...form.value }`.
2. Update `frontend/src/components/AgentIde/automation/AutomationForm.vue` per §2b:
   remove `models`/`providers` props, `toRef(props, "editAutomation")`, replace
   the inline create-path reset with `resetForm()`.
3. Update `frontend/src/components/AgentIde/AgentIde.vue`: drop `:models` and
   `:providers` bindings (lines 375-376) and the now-unused `models`/`providers`
   computeds (lines 149-150).
4. Verify (see §5). No backend changes.

`frontend/src/composables/automation/index.ts` export is unchanged
(`useAutomationForm` still exported, same name).

---

## 4. Edge cases / risks

- **Stored model removed/renamed**: `get` returns `""` → Connection shows the
  disabled placeholder, Model dropdown empty. Same as today; not made worse.
  Optional future improvement: a "missing model" placeholder (out of scope).
- **`Automation.trigger` is the trigger-type string** (backend
  `dispatcher_handlers.go:86`: `Trigger string json:"trigger"`, values
  `"cron"|"interval"|"manual"`). The `target.trigger as TriggerType` cast is
  sound. Optional cleanup: type `Automation.trigger` as the union in
  `types/dispatcher.ts`.
- **Task File dropdown populates asynchronously** — the populate watch sets
  `selectedWorkspace`, which emits `onFetchFiles`; the parent's `workspaceFiles`
  prop lands later. The `<select>` is reactive, so once files arrive the option
  matching `form.taskFile` appears (self-healing). Caveats: (a) if the form's
  `selectedWorkspace` is already the target workspace, the watch won't refire and
  files won't be re-fetched — but they are already in `workspaceFiles` from the
  earlier load; (b) if the stored `task_file` is no longer in the workspace, the
  select shows blank (same as today). Verify both in §5 manual testing.
- **Default-key cloud models** (`api_key_name` empty): derived key is
  `provider/` (trailing slash); `modelsForKey` still resolves them but the
  Connection `<select>` has no matching `<option>` (`options` are
  `provider/keyName`). Same as today; not made worse.
- **Re-clicking Edit on the same row without Cancel**: `editAutomation.value` set
  to the same object reference → Vue ref setter skips → watch doesn't re-fire.
  Form is already populated, so no visible issue (and Cancel/Update both null it
  first). Equivalent to current behavior.
- **CronEditor untouched**: its internal cron state/description logic is
  self-contained and remains the single cron implementation.
- **Create path**: Connection change fires the `set` → picks first model for the
  key; Create validates as before. `selectedProviderKey` reset on Cancel happens
  implicitly because `form.model` is cleared by `resetForm`.

---

## 5. Verification

- `cd frontend && npm run build` → eslint + vue-tsc + vite must be clean.
- `cd backend && go build ./... && go test ./...` → green baseline (no backend
  change, but confirm).
- `go run ./tools/check-complexity/` → ≤ 12.
- Manual (no frontend test suite installed — see §7):
  - Fresh load → click Automations immediately → Edit → **all** fields populate
    on first click (name, task file, trigger, connection, model).
  - Edit a second, different automation directly (no Cancel in between).
  - Cancel clears the form; re-Edit populates again.
  - Create path: pick Connection → model auto-selects first model; save works.
  - Models-loading-late case: Connection/Model populate once models arrive.
  - Task-file late-arrival case: Edit an automation whose workspace files have
    not loaded yet → Task File option appears once `workspaceFiles` arrive.

---

## 6. Docs to update (after implementation)

- `docs/architecture.md` → Common Pitfalls: add "do not pass props as plain
  value snapshots into composables; use `toRef` — or consume the store singleton
  directly for global catalog data. Model routing in the automation form is
  derived from `form.model` — do not reintroduce a two-way sync."
- `docs/skills/assistant-ui-patterns.md` → note the derive-don't-sync pattern for
  dependent `<select>`s (connection → model) and the prop-snapshot gotcha.

---

## 7. Notes

- **No frontend test suite** (vitest not installed). AGENTS.md TDD + frontend
  rules recommend a unit test for the composable (populate-on-edit, reset-on-null,
  derive-key, late-load). Adding vitest is a dev-dependency change needing
  approval — deferred by user decision; will be done later.

---

## 8. Files touched (summary)

| File | Change |
|------|--------|
| `frontend/src/composables/automation/useAutomationForm.ts` | Rewrite: consume `useModels()` store, `Ref` input for `editAutomation`, derived `selectedProviderKey` computed, delete sync watchers + dead cron code. |
| `frontend/src/components/AgentIde/automation/AutomationForm.vue` | Drop `models`/`providers` props; `toRef(props, "editAutomation")`; tighten `editAutomation` prop type; use `resetForm()`. Template unchanged. |
| `frontend/src/components/AgentIde/AgentIde.vue` | Remove `:models`/`:providers` bindings (375-376) and the now-unused `models`/`providers` computeds (149-150). |
