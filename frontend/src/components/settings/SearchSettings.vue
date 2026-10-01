<script setup lang="ts">
import { computed, ref, watch } from "vue"
import type { GlobalConfig, SearchConfig, SearchProvider } from "../../types/admin"
import { useToolSecrets } from "../../composables/useToolSecrets"
import { useConfirm } from "../../composables/ui/useConfirm"
import {
  SEARCH_PROVIDER_IDS,
  SEARCH_MAX_RESULTS_MIN,
  SEARCH_MAX_RESULTS_MAX,
  SEARCH_MAX_RESULTS_DEFAULT,
  searchProviderLabel,
} from "../../constants/search"
import BaseButton from "../common/buttons/BaseButton.vue"
import Panel from "../common/layout/Panel.vue"
import FormField from "../common/forms/FormField.vue"
import LoadingState from "../common/feedback/LoadingState.vue"
import SettingsActions from "./SettingsActions.vue"

// Settings · Search: the provider and result count live in the shared
// configuration; the provider's API key is a stored secret, saved first by
// this section's Save. `configDirty` / `saving` / `configError` are the page's
// save state for the shared configuration; the typed key is reported to the
// page's unsaved-change guard via dirty-change.
const props = defineProps<{
  editConfig: GlobalConfig
  configDirty?: boolean
  saving?: boolean
  configError?: string
}>()

const emit = defineEmits<{
  (e: "update:editConfig", config: GlobalConfig): void
  (e: "updateConfig"): void
  (e: "discard"): void
  (e: "dirty-change", dirty: boolean): void
}>()

const secrets = useToolSecrets("search")
const { confirm } = useConfirm()
const saveError = ref("")
const loading = ref(true)
// keyValue holds the backend mask (e.g. "tvly-...9f2c") for a stored key, or the
// text being typed while replacing one.
const keyValue = ref("")
const showKey = ref(false)
const replacing = ref(false)

// hasKey mirrors the other provider key cards: a stored credential is shown as
// bullets + last 4, never as an editable field.
const hasKey = computed<boolean>(() => keyValue.value !== "" && !replacing.value)

// Masked preview in the same shape cloud provider keys use: bullets + last 4.
// keyValue is already the backend mask ("abcd...wxyz"), so its tail identifies
// the stored key without revealing it.
const keyTail = computed<string>(() => (keyValue.value.length > 4 ? keyValue.value.slice(-4) : ""))
const keyMask = computed<string>(() => (keyTail.value ? `••••••••${keyTail.value}` : ""))

watch(secrets.isDirty, (dirty) => emit("dirty-change", dirty), { immediate: true })
const dirty = computed(() => !!props.configDirty || secrets.isDirty.value)

// Backend-driven option list, with the local typed set as an offline fallback.
const providerOptions = computed<string[]>(() => {
  const fromBackend = props.editConfig.search_providers
  return fromBackend && fromBackend.length > 0 ? fromBackend : [...SEARCH_PROVIDER_IDS]
})

const provider = computed<string>(
  () => props.editConfig.search?.provider || providerOptions.value[0] || "",
)

const maxResults = computed<number>(
  () => props.editConfig.search?.max_results ?? SEARCH_MAX_RESULTS_DEFAULT,
)

function updateSearch(patch: Partial<SearchConfig>) {
  emit("update:editConfig", { ...props.editConfig, search: { ...props.editConfig.search, ...patch } })
}

function onProviderChange(value: string) {
  updateSearch({ provider: value as SearchProvider })
}

function onMaxResultsInput(raw: string) {
  const n = Number(raw)
  updateSearch({ max_results: Number.isFinite(n) && n > 0 ? n : undefined })
}

async function loadKey(name: string) {
  if (!name) {
    loading.value = false
    return
  }
  loading.value = true
  replacing.value = false
  try {
    await secrets.load(name)
    keyValue.value = secrets.tokens.value[name]?.masked ?? ""
  } finally {
    loading.value = false
  }
}

// Replace mode starts from a blank field: the stored mask is never editable
// text, so it can never be submitted back as the credential.
function startReplace() {
  replacing.value = true
  showKey.value = false
  keyValue.value = ""
  const name = provider.value
  if (name) {
    secrets.ensureTracked(name)
    secrets.tokens.value[name]!.dirty = null
  }
}

function cancelReplace() {
  replacing.value = false
  const name = provider.value
  if (name && secrets.tokens.value[name]) secrets.tokens.value[name]!.dirty = null
  keyValue.value = name ? (secrets.tokens.value[name]?.masked ?? "") : ""
}

// Load the selected provider's masked key whenever the selection changes.
watch(provider, (name) => void loadKey(name), { immediate: true })

function onKeyInput(value: string) {
  keyValue.value = value
  const name = provider.value
  if (!name) return
  secrets.ensureTracked(name)
  // Only typed text is dirty — never the stored mask, which is not editable.
  secrets.tokens.value[name]!.dirty = value !== "" ? value : null
}

async function save() {
  saveError.value = ""
  if (await secrets.saveDirty(saveError)) {
    const name = provider.value
    replacing.value = false
    keyValue.value = name ? (secrets.tokens.value[name]?.masked ?? keyValue.value) : keyValue.value
    emit("updateConfig")
  }
}

function discard() {
  saveError.value = ""
  secrets.discard()
  cancelReplace()
  emit("discard")
}

// Remove the stored key for the selected provider. Destructive, so confirm
// first; the tool reverts to unconfigured (and is hidden from the agent).
async function clearKey() {
  const name = provider.value
  if (!name) return
  const ok = await confirm({
    title: `Remove the ${searchProviderLabel(name)} API key?`,
    message: "The stored key is deleted now. The agent's internet_search tool stays hidden until a key is saved again.",
    type: "warning",
    confirmText: "Remove key",
  })
  if (!ok) return
  saveError.value = ""
  const err = await secrets.clear(name)
  if (err) {
    saveError.value = err
    return
  }
  replacing.value = false
  keyValue.value = secrets.tokens.value[name]?.masked ?? ""
  emit("updateConfig")
}
</script>

<template>
  <div class="flex flex-col gap-4">
    <Panel title="Internet search">
      <p class="mb-4 mt-0 max-w-[72ch] text-[length:var(--text-small)] text-muted">
        Choose a search provider and store its API key. The agent's internet_search tool stays hidden until a
        provider is configured. Changes apply to the next run — no restart needed.
      </p>

      <LoadingState v-if="loading" label="Loading search settings" :rows="3" />

      <div v-else class="grid grid-cols-[repeat(auto-fit,minmax(min(100%,240px),1fr))] gap-4">
        <FormField label="Provider">
          <template #default="{ id }">
            <select :id="id" :value="provider" class="form-control" @change="onProviderChange(($event.target as HTMLSelectElement).value)">
              <option v-for="option in providerOptions" :key="option" :value="option">{{ searchProviderLabel(option) }}</option>
            </select>
          </template>
        </FormField>

        <FormField label="API key" hint="Stored encrypted.">
          <template #default="{ id, describedBy }">
            <div class="flex min-w-0 items-center gap-2">
              <!-- Stored key: shown exactly like every other provider key (bullets +
                   last 4). The mask is never editable text, so it can never be
                   sent back as the credential. -->
              <span
                v-if="hasKey"
                data-test="key-mask"
                :aria-describedby="describedBy"
                class="flex h-8 min-w-0 flex-1 items-center rounded-[var(--radius-md)] border border-hairline bg-surface-raised px-2.5 font-mono text-[length:var(--text-small)] text-muted"
              >{{ keyMask }}</span>
              <input
                v-else
                :id="id"
                :value="keyValue"
                :type="showKey ? 'text' : 'password'"
                :aria-describedby="describedBy"
                autocomplete="off"
                spellcheck="false"
                class="form-control flex-1 font-mono"
                :placeholder="replacing ? 'Enter the new API key' : 'Enter an API key'"
                @input="onKeyInput(($event.target as HTMLInputElement).value)"
              />
              <BaseButton v-if="!hasKey" variant="ghost" size="sm" :aria-pressed="showKey" @click="showKey = !showKey">{{ showKey ? "Hide" : "Show" }}</BaseButton>
              <BaseButton v-if="hasKey" variant="secondary" size="sm" @click="startReplace">Replace key</BaseButton>
              <BaseButton v-if="replacing" variant="ghost" size="sm" @click="cancelReplace">Cancel</BaseButton>
              <BaseButton v-if="hasKey || replacing" variant="ghost" size="sm" icon="trash" icon-only label="Remove stored key" @click="clearKey" />
            </div>
          </template>
        </FormField>

        <FormField label="Max results" :hint="`Results per search, ${SEARCH_MAX_RESULTS_MIN}–${SEARCH_MAX_RESULTS_MAX}.`">
          <template #default="{ id, describedBy }">
            <input
              :id="id"
              :value="maxResults"
              :aria-describedby="describedBy"
              type="number"
              :min="SEARCH_MAX_RESULTS_MIN"
              :max="SEARCH_MAX_RESULTS_MAX"
              step="1"
              class="form-control font-mono tabular-nums"
              @input="onMaxResultsInput(($event.target as HTMLInputElement).value)"
            />
          </template>
        </FormField>
      </div>
    </Panel>

    <SettingsActions :dirty="dirty" :saving="saving" :error="saveError || configError" save-label="Save search settings" @save="save" @discard="discard" />
  </div>
</template>
