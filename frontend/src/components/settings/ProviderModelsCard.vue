<script setup lang="ts">
import { ref, computed, watch, onMounted } from "vue";
import { useModels } from "../../composables/useModels";
import { useConfirm } from "../../composables/useConfirm";
import { formatBytes, formatParameters } from "../../utils/formatters";
import BaseButton from "../common/BaseButton.vue";
import { PROVIDER_STYLES } from "../../constants/providers";
import type { APIKeyItem, ProviderType } from "../../types/admin";
import type { Model, AvailableModel } from "../../types/model";

const props = defineProps<{
  provider: ProviderType;
  apiKeys: APIKeyItem[];
  models: Model[];
  availableModels?: AvailableModel[];
}>();

const emit = defineEmits<{
  (e: "refresh"): void;
}>();

const { addModel, updateModel, removeModel, removeAllModels, fetchProviderModels } = useModels();
const { confirm } = useConfirm();

const providerModels = ref<string[]>([]);
const isLoadingModels = ref(false);
const editingModel = ref<Partial<Model> | null>(null);
const isAddingNew = ref(false);
const newModelKey = ref("");
const newModelId = ref("");
const newModelName = ref("");
const newFilename = ref("");
const newPort = ref(8081);
const newArgs = ref("");
const newMaxSteps = ref(0);
const newContextBudget = ref(0);
const newToolCallFormat = ref("");
const newPrefill = ref(false);
const filterText = ref("");
const lastDerivedName = ref("");

const filteredProviderModels = computed(() => {
  if (!filterText.value) return providerModels.value;
  const q = filterText.value.toLowerCase();
  return providerModels.value.filter((m) => m.toLowerCase().includes(q));
});

const modelsByKey = computed(() => {
  const grouped: Record<string, Model[]> = {};
  for (const m of props.models) {
    if (m.provider !== props.provider) continue;
    const key = m.provider_config?.api_key_name || "";
    if (!grouped[key]) grouped[key] = [];
    grouped[key].push(m);
  }
  return grouped;
});

watch(
  () => props.apiKeys,
  () => {
    if (newModelKey.value) {
      const stillExists = props.apiKeys.some(
        (k) => k.id === newModelKey.value || k.name === newModelKey.value,
      );
      if (!stillExists) newModelKey.value = "";
    }
  },
  { deep: true },
);

watch(newModelId, (id) => {
  if (!id || !isAddingNew.value) return;
  const parts = id.split("/");
  const derived = parts[parts.length - 1] || "";
  if (!newModelName.value || newModelName.value === lastDerivedName.value) {
    newModelName.value = derived;
    lastDerivedName.value = derived;
  }
});

watch(newModelKey, (keyName) => {
  if (isAddingNew.value && props.provider !== "local") {
    loadModels(keyName);
  }
});

async function loadModels(apiKeyName?: string) {
  if (props.provider === "local") return;
  isLoadingModels.value = true;
  providerModels.value = [];
  filterText.value = "";
  try {
    const list = await fetchProviderModels(props.provider, apiKeyName || newModelKey.value);
    providerModels.value = list;
  } finally {
    isLoadingModels.value = false;
  }
}

function nextLocalPort(): number {
  const localModels = props.models.filter((m) => m.provider === "local");
  let port = 8081;
  for (const m of localModels) {
    if (m.port && m.port >= port) port = m.port + 1;
  }
  return port;
}

function startAdd() {
  editingModel.value = {
    name: "",
    provider: props.provider,
    filename: "",
    model_id: "",
    args: [],
    prefill: false,
    provider_config: { api_key_name: "" },
  };
  newModelKey.value = "";
  newModelId.value = "";
  newModelName.value = "";
  newFilename.value = "";
  newPort.value = nextLocalPort();
  newArgs.value = "";
  newMaxSteps.value = 0;
  newContextBudget.value = 0;
  newToolCallFormat.value = "";
  newPrefill.value = false;
  lastDerivedName.value = "";
  filterText.value = "";
  isAddingNew.value = true;
  if (props.provider !== "local") {
    loadModels();
  }
}

function cancelEdit() {
  editingModel.value = null;
  isAddingNew.value = false;
}

async function saveNewModel() {
  if (props.provider === "local") {
    if (!newFilename.value) return;
    const name = newModelName.value || newFilename.value.replace(/\.gguf$/i, "").split("/").pop() || newFilename.value;
    await addModel({
      name,
      provider: "local",
      filename: newFilename.value,
      port: newPort.value,
      args: newArgs.value ? newArgs.value.split(/\s+/).filter(Boolean) : [],
      prefill: newPrefill.value,
      max_steps: newMaxSteps.value || undefined,
      context_budget: newContextBudget.value || undefined,
      tool_call_format: newToolCallFormat.value || undefined,
    });
  } else {
    if (!newModelId.value) return;
    const name = newModelName.value || newModelId.value.split("/").pop() || newModelId.value;
    await addModel({
      name,
      provider: props.provider,
      model_id: newModelId.value,
      provider_config: { api_key_name: newModelKey.value },
    });
  }
  cancelEdit();
  emit("refresh");
}

const alreadyConfiguredFilenames = computed(() => {
  if (props.provider !== "local") return new Set<string>();
  return new Set(
    props.models
      .filter((m) => m.provider === "local")
      .map((m) => m.filename)
      .filter(Boolean) as string[],
  );
});

watch(() => props.apiKeys, (newKeys, oldKeys) => {
  if (props.provider === 'local') return;
  // Only auto-discover when keys are ADDED
  if ((newKeys?.length || 0) > (oldKeys?.length || 0)) {
    discoverAllEndpointModels();
  }
}, { deep: true });

onMounted(() => {
  if (props.provider === 'local') {
    emit("refresh");
  }
});

function addDiscoveredModel(m: AvailableModel) {
  if (alreadyConfiguredFilenames.value.has(m.filename)) return;
  
  isAddingNew.value = true;
  editingModel.value = {
    name: m.metadata?.name || m.name,
    provider: "local",
    filename: m.filename,
    args: [],
    prefill: false,
    provider_config: { api_key_name: "" },
  };
  newFilename.value = m.filename;
  newModelName.value = m.metadata?.name || m.name;
  newPort.value = nextLocalPort();
  newArgs.value = "";
  newMaxSteps.value = 0;
  newContextBudget.value = 0;
  newToolCallFormat.value = "";
  newPrefill.value = false;
  
  // Scroll to form
  window.scrollTo({ top: 0, behavior: 'smooth' });
}

async function handleClearAll() {
  const confirmed = await confirm({
    title: "Clear All Models",
    message: `Are you sure you want to remove ALL models for ${props.provider}? This cannot be undone.`,
    type: "error",
    confirmText: "Clear All",
    cancelText: "Cancel",
  });
  if (!confirmed) return;
  await removeAllModels(props.provider);
  emit("refresh");
}

const editingArgsStr = computed({
  get: () => (editingModel.value?.args || []).join(" "),
  set: (val: string) => {
    if (editingModel.value) {
      editingModel.value.args = val.split(/\s+/).filter(Boolean);
    }
  },
});

function handleEdit(model: Model) {
  editingModel.value = JSON.parse(JSON.stringify(model));
  isAddingNew.value = false;
}

async function saveEdit() {
  if (!editingModel.value?.name) return;
  await updateModel(editingModel.value);
  editingModel.value = null;
  emit("refresh");
}

async function handleRemove(name: string) {
  const confirmed = await confirm({
    title: "Remove Model",
    message: `Remove model "${name}"?`,
    type: "error",
    confirmText: "Remove",
    cancelText: "Cancel",
  });
  if (!confirmed) return;
  await removeModel(name);
  emit("refresh");
}

const isSubmitDisabled = computed(() => {
  if (isAddingNew.value) {
    if (props.provider === "local") {
      return !newFilename.value;
    }
    return !newModelId.value;
  }
  return !editingModel.value?.name;
});

const discoveredModels = ref<{ modelId: string; keyName: string }[]>([]);
const isDiscovering = ref(false);

async function discoverAllEndpointModels() {
  if (props.provider === 'local') return;
  
  const keys = props.apiKeys || [];
  if (keys.length === 0) {
    discoveredModels.value = [];
    return;
  }
  
  isDiscovering.value = true;
  try {
    const allDiscovered: { modelId: string; keyName: string }[] = [];
    for (const key of keys) {
      const models = await fetchProviderModels(props.provider, key.name);
      const mapped = models.map(m => ({ modelId: m, keyName: key.name }));
      allDiscovered.push(...mapped);
    }
    discoveredModels.value = allDiscovered;
  } finally {
    isDiscovering.value = false;
  }
}

</script>

<template>
  <div class="provider-models-card">
    <div class="card-header">
      <h3 class="card-title">Models</h3>
      <div class="header-actions">
        <BaseButton
          v-if="!editingModel && props.models.length > 0"
          variant="secondary"
          size="sm"
          icon="trash"
          className="mr-2 text-red-400 border-red-400/30 hover:bg-red-500/10"
          @click="handleClearAll"
        >
          Clear All
        </BaseButton>
        <BaseButton
          v-if="!editingModel"
          variant="primary"
          size="sm"
          icon="plus"
          @click="startAdd"
        >
          Add Model
        </BaseButton>
      </div>
    </div>

    <!-- Add/Edit Form -->
    <div v-if="editingModel" class="form-panel">
      <h4 class="form-title">
        {{ isAddingNew ? "Add Model" : "Edit Model" }}
      </h4>
      <div class="form-body">
        <template v-if="isAddingNew">
          <template v-if="provider === 'local'">
            <div class="form-group">
              <label class="form-label">Model Filename</label>
              <input
                v-model="newFilename"
                type="text"
                class="form-input"
                placeholder="e.g. qwen2.5-7b-instruct-q4_k_m.gguf"
              />
            </div>
            <div class="form-group">
              <label class="form-label">Friendly Name</label>
              <input
                v-model="newModelName"
                type="text"
                class="form-input"
                placeholder="Auto-derived from filename if empty"
              />
            </div>
            <div class="form-group">
              <label class="form-label">Port</label>
              <input
                v-model.number="newPort"
                type="number"
                class="form-input"
              />
            </div>
            <div class="form-group">
              <label class="form-label">Custom Arguments</label>
              <input
                v-model="newArgs"
                type="text"
                class="form-input"
                placeholder="--ctx-size 8192 --gpu-layers 32"
              />
            </div>
            <div class="form-section-divider">Agent Tuning (per-model overrides)</div>
            <div class="tuning-grid">
              <div class="form-group">
                <label class="form-label">Max Steps</label>
                <input
                  v-model.number="newMaxSteps"
                  type="number"
                  class="form-input"
                  placeholder="25 (default)"
                  min="1" max="100"
                />
              </div>
              <div class="form-group">
                <label class="form-label">Context Budget (chars)</label>
                <input
                  v-model.number="newContextBudget"
                  type="number"
                  class="form-input"
                  placeholder="15000 (default)"
                  min="1000" max="100000" step="1000"
                />
              </div>
              <div class="form-group">
                <label class="form-label">Tool Call Format</label>
                <select v-model="newToolCallFormat" class="form-input">
                  <option value="">Default (native)</option>
                  <option value="native">Native Tools</option>
                  <option value="xml">XML Text</option>
                </select>
              </div>
              <div class="form-group">
                <label class="form-label">Prefill</label>
                <label class="flex items-center gap-2 cursor-pointer mt-2">
                  <input
                    type="checkbox"
                    v-model="newPrefill"
                    class="rounded border-gray-600 bg-gray-700 text-blue-600 focus:ring-blue-600"
                  />
                  <span class="text-sm text-gray-300">Prefill tool calls</span>
                </label>
              </div>
            </div>
          </template>
          <template v-else>
            <div class="form-group">
              <label class="form-label">API Key</label>
              <select v-model="newModelKey" class="form-input">
                <option value="">No key (use default)</option>
                <option
                  v-for="key in apiKeys"
                  :key="key.id"
                  :value="key.name"
                >
                  {{ key.name }}
                </option>
              </select>
            </div>
            <div class="form-group">
              <label class="form-label">Model ID</label>
              <div v-if="providerModels.length > 0" class="model-select-area">
                <input
                  v-model="filterText"
                  type="text"
                  class="form-input model-filter-input"
                  placeholder="Type to filter models..."
                />
                <select
                  v-model="newModelId"
                  class="form-input model-filter-select"
                  size="5"
                >
                  <option
                    v-for="m in filteredProviderModels"
                    :key="m"
                    :value="m"
                  >
                    {{ m }}
                  </option>
                </select>
                <div v-if="filteredProviderModels.length === 0 && filterText" class="filter-no-results">
                  No models match "{{ filterText }}"
                </div>
                <div class="model-select-toolbar">
                  <span class="model-count-hint">
                    {{ filteredProviderModels.length }} of {{ providerModels.length }} models
                  </span>
                  <BaseButton
                    variant="ghost"
                    size="sm"
                    icon="spinner"
                    iconOnly
                    :loading="isLoadingModels"
                    @click="loadModels"
                    title="Refresh model list"
                  />
                </div>
              </div>
              <input
                v-else
                v-model="newModelId"
                type="text"
                class="form-input"
                placeholder="e.g. gpt-4o"
              />
            </div>
            <div class="form-group">
              <label class="form-label">Friendly Name</label>
              <input
                v-model="newModelName"
                type="text"
                class="form-input"
                placeholder="Auto-derived from model ID"
              />
            </div>
          </template>
        </template>
        <template v-else>
          <template v-if="provider === 'local'">
            <div class="form-group">
              <label class="form-label">Name</label>
              <input
                v-model="editingModel.name"
                type="text"
                class="form-input"
              />
            </div>
            <div class="form-group">
              <label class="form-label">Filename</label>
              <input
                v-model="editingModel.filename"
                type="text"
                class="form-input"
              />
            </div>
            <div class="form-group">
              <label class="form-label">Port</label>
              <input
                v-model.number="editingModel.port"
                type="number"
                class="form-input"
              />
            </div>
            <div class="form-group">
              <label class="form-label">Custom Arguments</label>
              <input
                v-model="editingArgsStr"
                type="text"
                class="form-input"
                placeholder="--ctx-size 8192 --gpu-layers 32"
              />
            </div>
            <div class="form-section-divider">Agent Tuning (per-model overrides)</div>
            <div class="tuning-grid">
              <div class="form-group">
                <label class="form-label">Max Steps</label>
                <input
                  v-model.number="editingModel.max_steps"
                  type="number"
                  class="form-input"
                  placeholder="25 (default)"
                  min="1" max="100"
                />
              </div>
              <div class="form-group">
                <label class="form-label">Context Budget (chars)</label>
                <input
                  v-model.number="editingModel.context_budget"
                  type="number"
                  class="form-input"
                  placeholder="15000 (default)"
                  min="1000" max="100000" step="1000"
                />
              </div>
              <div class="form-group">
                <label class="form-label">Tool Call Format</label>
                <select v-model="editingModel.tool_call_format" class="form-input">
                  <option value="">Default (native)</option>
                  <option value="native">Native Tools</option>
                  <option value="xml">XML Text</option>
                </select>
              </div>
              <div class="form-group">
                <label class="form-label">Prefill</label>
                <label class="flex items-center gap-2 cursor-pointer mt-2">
                  <input
                    type="checkbox"
                    v-model="editingModel.prefill"
                    class="rounded border-gray-600 bg-gray-700 text-blue-600 focus:ring-blue-600"
                  />
                  <span class="text-sm text-gray-300">Prefill tool calls</span>
                </label>
              </div>
            </div>
          </template>
          <template v-else>
            <div class="form-group">
              <label class="form-label">Name</label>
              <input
                v-model="editingModel.name"
                type="text"
                class="form-input"
              />
            </div>
            <div class="form-group">
              <label class="form-label">Model ID</label>
              <input
                v-model="editingModel.model_id"
                type="text"
                class="form-input"
              />
            </div>
            <div class="form-group">
              <label class="form-label">API Key</label>
              <select
                v-model="editingModel.provider_config!.api_key_name"
                class="form-input"
              >
                <option value="">No key (use default)</option>
                <option
                  v-for="key in apiKeys"
                  :key="key.id"
                  :value="key.name"
                >
                  {{ key.name }}
                </option>
              </select>
            </div>
            <div class="form-section-divider">Agent Tuning (per-model overrides)</div>
            <div class="tuning-grid">
              <div class="form-group">
                <label class="form-label">Max Steps</label>
                <input
                  v-model.number="editingModel.max_steps"
                  type="number"
                  class="form-input"
                  placeholder="25 (default)"
                  min="1" max="100"
                />
              </div>
              <div class="form-group">
                <label class="form-label">Context Budget (chars)</label>
                <input
                  v-model.number="editingModel.context_budget"
                  type="number"
                  class="form-input"
                  placeholder="15000 (default)"
                  min="1000" max="100000" step="1000"
                />
              </div>
              <div class="form-group">
                <label class="form-label">Tool Call Format</label>
                <select v-model="editingModel.tool_call_format" class="form-input">
                  <option value="">Default (native)</option>
                  <option value="native">Native Tools</option>
                  <option value="xml">XML Text</option>
                </select>
              </div>
              <div class="form-group">
                <label class="form-label">Prefill</label>
                <label class="flex items-center gap-2 cursor-pointer mt-2">
                  <input
                    type="checkbox"
                    v-model="editingModel.prefill"
                    class="rounded border-gray-600 bg-gray-700 text-blue-600 focus:ring-blue-600"
                  />
                  <span class="text-sm text-gray-300">Prefill tool calls</span>
                </label>
              </div>
            </div>
          </template>
        </template>
      </div>
      <div class="form-actions">
        <BaseButton variant="secondary" size="sm" @click="cancelEdit">
          Cancel
        </BaseButton>
        <BaseButton
          variant="primary"
          size="sm"
          icon="check"
          :disabled="isSubmitDisabled"
          @click="isAddingNew ? saveNewModel() : saveEdit()"
        >
          {{ isAddingNew ? "Add" : "Save" }}
        </BaseButton>
      </div>
    </div>

    <!-- Model list -->
    <div v-if="!editingModel" class="models-list">
      <div v-if="props.models.length === 0" class="models-empty">
        No models configured for {{ provider }}.
        <button class="link-btn" @click="startAdd">Add one now.</button>
      </div>

      <!-- Discovered GGUF (local only) -->
      <div
        v-if="provider === 'local' && availableModels && availableModels.length > 0"
        class="discovered-section"
      >
        <div class="discovered-header">
          <div class="flex items-center gap-2">
            <span class="discovered-title">Discovered on Disk</span>
            <button 
              class="refresh-btn" 
              @click="emit('refresh')"
              title="Rescan model directory"
            >
              <svg xmlns="http://www.w3.org/2000/svg" width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12a9 9 0 1 1-9-9c2.52 0 4.93 1 6.74 2.74L21 8"/><path d="M21 3v5h-5"/></svg>
            </button>
          </div>
          <span class="discovered-count">{{ availableModels.length }} models</span>
        </div>
        <div class="discovered-list">
          <div
            v-for="m in availableModels"
            :key="m.filename"
            class="discovered-row"
          >
            <div class="discovered-info">
              <div class="discovered-primary">
                <span class="discovered-name">{{ m.metadata?.name || m.name }}</span>
                <div v-if="m.metadata" class="meta-pills">
                  <span v-if="m.metadata.architecture" class="pill pill--blue">{{ m.metadata.architecture }}</span>
                  <span v-if="m.metadata.quantization" class="pill pill--green">{{ m.metadata.quantization }}</span>
                </div>
              </div>
              <div class="discovered-secondary">
                <span class="discovered-filename">{{ m.filename }}</span>
                <span class="discovered-detail-dot">•</span>
                <span class="discovered-detail-text">{{ formatBytes(m.size_bytes) }}</span>
                <template v-if="m.metadata">
                  <span v-if="m.metadata.parameters" class="discovered-detail-dot">•</span>
                  <span v-if="m.metadata.parameters" class="discovered-detail-text">{{ formatParameters(m.metadata.parameters) }} params</span>
                  <span v-if="m.metadata.context_length" class="discovered-detail-dot">•</span>
                  <span v-if="m.metadata.context_length" class="discovered-detail-text">{{ m.metadata.context_length }} ctx</span>
                </template>
              </div>
            </div>
            <BaseButton
              variant="secondary"
              size="sm"
              @click="addDiscoveredModel(m)"
            >
              Add
            </BaseButton>
          </div>
        </div>
      </div>

      <!-- Discovered via API (cloud only) - Removed in favor of dropdown autocomplete -->
      <div v-if="isDiscovering" class="px-4 py-3 text-center">
        <span class="text-[10px] text-gray-500 flex items-center justify-center gap-2">
          <svg class="animate-spin" xmlns="http://www.w3.org/2000/svg" width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12a9 9 0 1 1-9-9c2.52 0 4.93 1 6.74 2.74L21 8"/><path d="M21 3v5h-5"/></svg>
          Scanning endpoint for models...
        </span>
      </div>

      <div
        v-for="(group, keyName) in modelsByKey"
        :key="keyName"
        class="model-group"
      >
        <div class="group-header">
          <span class="group-key-label">
            {{ keyName || "No key assigned" }}
          </span>
          <span class="group-count">{{ group.length }} model(s)</span>
        </div>
        <div
          v-for="m in group"
          :key="m.name"
          class="model-row"
        >
          <div class="model-info">
            <span
              :class="[
                'provider-badge',
                PROVIDER_STYLES[m.provider as keyof typeof PROVIDER_STYLES],
              ]"
            >
              {{ m.provider }}
            </span>
            <span class="model-name">{{ m.name }}</span>
            <span class="model-id">{{ m.model_id || m.filename }}</span>
          </div>
          <div class="model-actions">
            <button
              @click="handleEdit(m)"
              class="action-btn"
              title="Edit"
            >
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" class="w-3.5 h-3.5">
                <path d="M11 4H4a2 2 0 00-2 2v14a2 2 0 002 2h14a2 2 0 002-2v-7M18.5 2.5a2.121 2.121 0 013 3L12 15l-4 1 1-4 9.5-9.5z"/>
              </svg>
            </button>
            <button
              @click="handleRemove(m.name)"
              class="action-btn action-btn-remove"
              title="Remove"
            >
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" class="w-3.5 h-3.5">
                <path d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"/>
              </svg>
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped lang="postcss">
.provider-models-card {
  @apply bg-gray-800/50 border border-gray-700 rounded-lg p-4 space-y-3;
}

.card-header {
  @apply flex items-center justify-between;
}

.card-title {
  @apply text-sm font-bold text-white;
}

.form-panel {
  @apply bg-gray-900/60 border border-blue-600/30 rounded-lg p-4 space-y-3;
}

.form-title {
  @apply text-xs font-bold text-blue-400 uppercase tracking-wide;
}

.form-body {
  @apply space-y-3;
}

.form-group {
  @apply space-y-1;
}

.form-label {
  @apply block text-xs font-semibold text-gray-400;
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

.model-select-area {
  @apply space-y-1;
}

.model-filter-input {
  @apply text-xs;
}

.model-filter-select {
  @apply h-auto min-h-[100px] max-h-[200px] appearance-none bg-gray-900 border-gray-700
         overflow-y-auto text-xs font-mono pr-3;
}

.model-filter-select option {
  @apply px-2 py-1;
}

.model-filter-select option:checked {
  @apply bg-blue-600/30;
}

.filter-no-results {
  @apply text-xs text-gray-500 italic px-1;
}

.model-select-toolbar {
  @apply flex items-center justify-between;
}

.model-count-hint {
  @apply text-[10px] text-gray-600;
}

.option-muted {
  @apply opacity-50;
}

.form-actions {
  @apply flex justify-end gap-2 pt-2 border-t border-gray-700/50;
}

.models-list {
  @apply space-y-3;
}

.models-empty {
  @apply text-center text-xs text-gray-500 italic py-6;
}

.link-btn {
  @apply text-blue-400 hover:text-blue-300 underline;
}

.model-group {
  @apply space-y-1;
}

.group-header {
  @apply flex items-center justify-between px-2 py-1;
}

.group-key-label {
  @apply text-xs font-bold text-gray-400;
}

.group-count {
  @apply text-[10px] text-gray-600;
}

.model-row {
  @apply flex items-center justify-between px-3 py-2 rounded-md
         bg-gray-900/40 border border-gray-700/50 hover:border-gray-600 transition-all;
}

.model-info {
  @apply flex items-center gap-2 min-w-0 flex-1;
}

.provider-badge {
  @apply px-1.5 py-0.5 rounded text-[8px] uppercase font-black tracking-widest border shrink-0;
}

.model-name {
  @apply text-sm font-bold text-white truncate;
}

.model-id {
  @apply text-[10px] text-gray-500 font-mono truncate;
}

.model-actions {
  @apply flex gap-1 shrink-0;
}

.action-btn {
  @apply p-1.5 rounded text-gray-600 hover:text-white hover:bg-gray-800/50 transition-all;
}

.action-btn-remove:hover {
  @apply text-red-500 bg-red-500/10;
}

.form-section-divider {
  @apply text-xs font-bold text-blue-400 uppercase tracking-wide border-t border-gray-700 pt-3 mt-2;
}

.tuning-grid {
  @apply grid grid-cols-1 sm:grid-cols-2 gap-3;
}

.discovered-section {
  @apply bg-gray-900/40 border border-gray-700/50 rounded-lg overflow-hidden;
}

.discovered-header {
  @apply flex items-center justify-between px-4 py-2 bg-gray-800/50 border-b border-gray-700/50;
}

.discovered-title {
  @apply text-[10px] font-bold text-gray-400 uppercase tracking-widest;
}

.discovered-count {
  @apply text-[10px] text-gray-600 font-mono;
}

.discovered-list {
  @apply divide-y divide-gray-700/30;
}

.discovered-row {
  @apply flex items-center justify-between px-4 py-3
         hover:bg-gray-800/20 transition-all;
}

.discovered-info {
  @apply flex-1 min-w-0 flex flex-col gap-1;
}

.discovered-primary {
  @apply flex items-center gap-3;
}

.discovered-name {
  @apply text-[13px] font-bold text-white truncate;
}

.discovered-secondary {
  @apply flex items-center gap-2 min-w-0;
}

.discovered-filename {
  @apply text-[10px] text-gray-500 font-mono truncate max-w-[180px];
}

.discovered-detail-text {
  @apply text-[10px] text-gray-500 font-medium whitespace-nowrap;
}

.discovered-detail-dot {
  @apply text-gray-700 text-[10px] select-none;
}

.meta-pills {
  @apply flex items-center gap-1.5 flex-wrap;
}

.pill {
  @apply text-[9px] font-black uppercase tracking-wider px-1.5 py-0.5 rounded-[4px] border leading-none;
}

.pill--blue {
  @apply bg-blue-500/10 text-blue-400 border-blue-500/20;
}

.pill--green {
  @apply bg-emerald-500/10 text-emerald-400 border-emerald-500/20;
}

.pill--gray {
  @apply bg-gray-700/30 text-gray-400 border-gray-600/20;
}

.pill--purple {
  @apply bg-indigo-500/10 text-indigo-400 border-indigo-500/20;
}

.discovered-row {
  @apply flex items-center justify-between px-4 py-3
         hover:bg-gray-800/20 transition-all border-b border-gray-700/30 last:border-0;
}

.discovered-header {
  @apply flex items-center justify-between px-4 py-2.5 bg-gray-800/40 border-b border-gray-700/50;
}

.discovered-title {
  @apply text-[10px] font-bold text-gray-400 uppercase tracking-widest;
}

.discovered-count {
  @apply text-[10px] text-gray-600 font-mono;
}

.refresh-btn {
  @apply p-1 rounded-full text-gray-600 hover:text-blue-400 hover:bg-blue-400/10 transition-all;
}

.animate-spin {
  animation: spin 1s linear infinite;
}

@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}
</style>
