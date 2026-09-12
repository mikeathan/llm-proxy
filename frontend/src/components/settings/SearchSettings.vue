<script setup lang="ts">
import { computed, ref, watch } from "vue"
import type { GlobalConfig, SearchConfig, SearchProvider } from "../../types/admin"
import { useToolSecrets } from "../../composables/useToolSecrets"
import {
  SEARCH_PROVIDER_IDS,
  SEARCH_MAX_RESULTS_MIN,
  SEARCH_MAX_RESULTS_MAX,
  SEARCH_MAX_RESULTS_DEFAULT,
  searchProviderLabel,
} from "../../constants/search"
import BaseButton from "../common/buttons/BaseButton.vue"

const props = defineProps<{
  editConfig: GlobalConfig
}>()

const emit = defineEmits<{
  (e: "update:editConfig", config: GlobalConfig): void
  (e: "updateConfig"): void
}>()

const secrets = useToolSecrets("search")
const saveError = ref("")
const loading = ref(true)
const keyValue = ref("")
const showKey = ref(false)

// Masked preview in the same shape cloud provider keys use: bullets + last 4.
// keyValue is already the backend mask ("abcd...wxyz"), so its tail identifies
// the stored key without revealing it.
const keyTail = computed<string>(() => (keyValue.value.length > 4 ? keyValue.value.slice(-4) : ""))

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
  const clone = JSON.parse(JSON.stringify(props.editConfig)) as GlobalConfig
  clone.search = { ...clone.search, ...patch }
  emit("update:editConfig", clone)
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
  try {
    await secrets.load(name)
    keyValue.value = secrets.tokens.value[name]?.masked ?? ""
  } finally {
    loading.value = false
  }
}

// Load the selected provider's masked key whenever the selection changes.
watch(provider, (name) => void loadKey(name), { immediate: true })

function onKeyInput(value: string) {
  keyValue.value = value
  const name = provider.value
  if (!name) return
  secrets.ensureTracked(name)
  const entry = secrets.tokens.value[name]!
  entry.dirty = value && value !== entry.masked ? value : null
}

async function save() {
  saveError.value = ""
  if (await secrets.saveDirty(saveError)) {
    const name = provider.value
    keyValue.value = name ? (secrets.tokens.value[name]?.masked ?? keyValue.value) : keyValue.value
    emit("updateConfig")
  }
}
</script>

<template>
  <div class="settings-container">
    <h2 class="settings-title">Internet Search</h2>
    <div class="form-helper">
      Choose a search provider and enter its API key. The agent's internet_search
      tool stays hidden until a provider is configured. Changes apply to the next
      run — no restart needed.
    </div>

    <div v-if="loading" class="loading-state">Loading search settings…</div>

    <div v-else class="form-section">
      <div class="form-group">
        <label class="form-label" for="search-provider">Provider</label>
        <select
          id="search-provider"
          :value="provider"
          class="form-input"
          @change="onProviderChange(($event.target as HTMLSelectElement).value)"
        >
          <option v-for="id in providerOptions" :key="id" :value="id">
            {{ searchProviderLabel(id) }}
          </option>
        </select>
      </div>

      <div class="form-group">
        <label class="form-label" for="search-key">API key</label>
        <div class="form-helper">Stored encrypted. Leave unchanged to keep the existing key.</div>
        <div v-if="keyTail" class="key-preview">••••••••{{ keyTail }}</div>
        <div class="input-row">
          <input
            id="search-key"
            :value="keyValue"
            :type="showKey ? 'text' : 'password'"
            autocomplete="off"
            class="form-input"
            :placeholder="keyValue ? 'Leave blank to keep the existing key' : 'Enter API key'"
            @input="onKeyInput(($event.target as HTMLInputElement).value)"
          />
          <BaseButton
            variant="secondary"
            size="sm"
            :icon="showKey ? 'spinner' : 'document'"
            iconOnly
            title="Toggle Visibility"
            @click="showKey = !showKey"
          />
        </div>
      </div>

      <div class="form-group">
        <label class="form-label" for="search-max">Max results</label>
        <div class="form-helper">
          How many results each search returns ({{ SEARCH_MAX_RESULTS_MIN }}–{{ SEARCH_MAX_RESULTS_MAX }}).
        </div>
        <input
          id="search-max"
          :value="maxResults"
          type="number"
          :min="SEARCH_MAX_RESULTS_MIN"
          :max="SEARCH_MAX_RESULTS_MAX"
          step="1"
          class="form-input"
          @input="onMaxResultsInput(($event.target as HTMLInputElement).value)"
        />
      </div>

      <div class="save-bar">
        <BaseButton variant="primary" icon="play" className="save-search" @click="save">Save Search Settings</BaseButton>
      </div>
      <p v-if="saveError" class="save-error">{{ saveError }}</p>
    </div>
  </div>
</template>

<style scoped lang="postcss">
.settings-container {
  @apply bg-gray-800 rounded-lg shadow-xl border border-gray-700 p-6 space-y-4;
}
.settings-title {
  @apply text-xl font-bold text-white mb-2;
}
.form-helper {
  @apply text-xs text-gray-500;
}
.loading-state {
  @apply text-sm text-gray-400 py-4 text-center;
}
.form-section {
  @apply space-y-4;
}
.form-group {
  @apply space-y-1.5;
}
.form-label {
  @apply block text-sm font-semibold text-gray-200;
}
.form-input {
  @apply w-full bg-gray-900 border border-gray-700 rounded px-3 py-2 text-sm text-white;
}
.input-row {
  @apply flex gap-2;
}
.input-row .form-input {
  @apply flex-1;
}
.key-preview {
  @apply text-[10px] text-gray-500 font-mono mb-1;
}
.save-bar {
  @apply pt-4 border-t border-gray-700 flex justify-end items-center gap-3;
}
.save-error {
  @apply text-xs text-red-400;
}
</style>
