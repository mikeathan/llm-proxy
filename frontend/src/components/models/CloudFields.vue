<script setup lang="ts">
import { ref, computed, watch } from "vue";
import { useModels } from "../../composables/useModels";
import { useProviders } from "../../composables/useProviders";
import type { AdminState } from "../../types";

const props = defineProps<{
  provider: string;
  modelId: string;
  apiKeyName: string;
  state: AdminState | null;
}>();

const emit = defineEmits<{
  (e: "update:modelId", value: string): void;
  (e: "update:apiKeyName", value: string): void;
}>();

const { cloudProviders } = useProviders();
const { fetchProviderModels } = useModels();
const providerModels = ref<string[]>([]);
const isLoadingModels = ref(false);
const filterText = ref("");

const filteredProviderModels = computed(() => {
  if (!filterText.value) return providerModels.value;
  const search = filterText.value.toLowerCase();
  return providerModels.value.filter((m) => m.toLowerCase().includes(search));
});

const isProviderConfigured = computed(() => {
  if (props.provider === "local") return true;
  const cfg = props.state?.config?.providers?.[props.provider];
  if (!cfg) return false;

  if (props.provider === "vertex") return !!cfg.project_id;
  return !!cfg.api_key || (cfg.api_keys && cfg.api_keys.length > 0);
});

const availableKeys = computed(() => {
  const cfg = props.state?.config?.providers?.[props.provider];
  return cfg?.api_keys || [];
});

async function loadProviderModels() {
  if (!props.provider || props.provider === "local") return;
 
  isLoadingModels.value = true;
  try {
    const list = await fetchProviderModels(props.provider, props.apiKeyName);
    providerModels.value = list;
    if (list.length > 0 && !props.modelId) {
      const firstModel = list[0];
      if (firstModel) {
        emit("update:modelId", firstModel);
      }
    }
  } finally {
    isLoadingModels.value = false;
  }
}

watch(
  () => props.provider,
  (newProv, oldProv) => {
    if (newProv !== oldProv) {
      providerModels.value = [];
      filterText.value = "";

      // Auto-select first key if none selected and keys exist
      if (availableKeys.value.length > 0 && !props.apiKeyName) {
        emit("update:apiKeyName", availableKeys.value[0]?.name || "");
      }

      if (newProv !== "local" && isProviderConfigured.value) {
        loadProviderModels();
      }
    }
  },
  { immediate: true },
);

// Reload models when API key changes
watch(
  () => props.apiKeyName,
  (newKey, oldKey) => {
    if (newKey !== oldKey && props.provider !== "local") {
      loadProviderModels();
    }
  }
);

// Trigger load when provider becomes configured (e.g. after state refresh)
watch(isProviderConfigured, (configured) => {
  if (configured && props.provider !== "local" && providerModels.value.length === 0) {
    loadProviderModels();
  }
});
</script>

<template>
  <div class="grid grid-cols-1 gap-3">
    <div v-if="!isProviderConfigured" class="config-warning">
      <span class="config-warning-text">
        <span class="text-base">⚠️</span> Configuration Required
      </span>
      <router-link to="/settings" class="btn-settings-link"
        >Settings</router-link
      >
    </div>

    <div v-if="availableKeys.length > 0" class="mb-3">
      <label class="form-label">API Key Name</label>
      <select
        :value="apiKeyName"
        @change="emit('update:apiKeyName', ($event.target as HTMLSelectElement).value)"
        class="form-input"
        required
      >
        <option v-if="availableKeys.length === 0" value="">Default Provider Key</option>
        <option v-for="k in availableKeys" :key="k.name" :value="k.name">
          {{ k.name }}
        </option>
      </select>
    </div>

    <div class="field-header">
      <label
        class="form-label mb-0"
        :class="!isProviderConfigured ? 'opacity-50' : ''"
        >Model ID</label
      >
      <button
        v-if="cloudProviders.includes(provider as any)"
        type="button"
        @click="loadProviderModels"
        class="btn-refresh"
        :disabled="isLoadingModels || !isProviderConfigured"
      >
        <span
          v-if="isLoadingModels"
          class="animate-spin h-2 w-2 border-b border-current rounded-full"
        ></span>
        {{ isLoadingModels ? "Loading..." : "Refresh List" }}
      </button>
    </div>

    <div v-if="providerModels.length > 0" class="search-wrapper">
      <input
        v-model="filterText"
        type="text"
        placeholder="Search models..."
        class="form-input form-input--search"
        :disabled="!isProviderConfigured"
      />
      <span class="search-icon">
        <svg
          xmlns="http://www.w3.org/2000/svg"
          width="12"
          height="12"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2.5"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          <circle cx="11" cy="11" r="8" />
          <path d="m21 21-4.3-4.3" />
        </svg>
      </span>
    </div>

    <template v-if="providerModels.length > 0">
      <select
        :value="modelId"
        @change="
          emit('update:modelId', ($event.target as HTMLSelectElement).value)
        "
        class="form-input"
        required
        :disabled="!isProviderConfigured"
      >
        <option value="" disabled>Select a model...</option>
        <option v-for="m in filteredProviderModels" :key="m" :value="m">
          {{ m }}
        </option>
      </select>
      <div v-if="filteredProviderModels.length === 0" class="helper-text">
        No models match "{{ filterText }}"
      </div>
    </template>
    <input
      v-else
      :value="modelId"
      @input="emit('update:modelId', ($event.target as HTMLInputElement).value)"
      type="text"
      required
      placeholder="e.g. gpt-4o or gemini-1.5-pro"
      class="form-input"
      :disabled="!isProviderConfigured"
    />
  </div>
</template>

<style scoped lang="postcss">
.form-label {
  @apply block text-[10px] uppercase font-bold text-gray-500 mb-1.5 tracking-wider;
}

.form-input {
  @apply w-full bg-gray-800 border border-gray-700 rounded-md px-3 py-2 text-sm text-white 
         focus:border-blue-600 focus:ring-1 focus:ring-blue-600 outline-none transition-all;
}

select.form-input {
  @apply appearance-none bg-no-repeat pr-10
         bg-[url('data:image/svg+xml;charset=utf-8,%3Csvg%20xmlns%3D%22http%3A%2F%2Fwww.w3.org%2F2000%2Fsvg%22%20fill%3D%22none%22%20viewBox%3D%220%200%2020%2020%22%3E%3Cpath%20stroke%3D%22%236b7280%22%20stroke-linecap%3D%22round%22%20stroke-linejoin%3D%22round%22%20stroke-width%3D%221.5%22%20d%3D%22m6%208%204%204%204-4%22%2F%3E%3C%2Fsvg%3E')] 
         bg-[position:right_0.5rem_center] bg-[length:1.25rem_1.25rem];
}

.config-warning {
  @apply mb-3 p-2.5 bg-yellow-900/20 border border-yellow-700/50 rounded-md flex justify-between items-center gap-2;
}

.config-warning-text {
  @apply text-[11px] text-yellow-500 font-bold uppercase tracking-tight flex items-center gap-1.5;
}

.btn-settings-link {
  @apply text-[10px] bg-yellow-600/20 hover:bg-yellow-600/30 text-yellow-400 px-2 py-1 rounded border border-yellow-600/30 font-bold transition-all;
}

.field-header {
  @apply flex justify-between items-center mb-1.5;
}

.btn-refresh {
  @apply text-[10px] text-blue-400 hover:text-blue-300 transition-colors uppercase font-bold 
         tracking-tighter flex items-center gap-1 disabled:opacity-20;
}

.search-wrapper {
  @apply mb-2 relative;
}

.form-input--search {
  @apply text-xs py-1.5 pl-8 bg-gray-900/50;
}

.search-icon {
  @apply absolute left-2.5 top-1/2 -translate-y-1/2 text-gray-600 pointer-events-none;
}

.helper-text {
  @apply mt-1 text-[10px] text-gray-500 italic;
}
</style>
