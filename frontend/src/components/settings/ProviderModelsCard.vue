<script setup lang="ts">
import { ref, computed, watch, onMounted } from "vue";
import { useModels } from "../../composables/useModels";
import { useConfirm } from "../../composables/useConfirm";
import { formatBytes, formatParameters } from "../../utils/formatters";
import BaseButton from "../common/BaseButton.vue";
import { PROVIDER_STYLES } from "../../constants/providers";
import type { APIKeyItem, ProviderType } from "../../types/admin";
import type { Model, AvailableModel } from "../../types/model";
import {
  getDefaultModelSettings,
  deriveModelName,
  createEmptyModelForm,
  computeDefaultsFromContext,
} from "../../utils/modelUtils";
import type { ModelForm } from "../../utils/modelUtils";

const {
  state,
  addModel,
  updateModel,
  removeModel,
  removeAllModels,
  fetchProviderModels,
} = useModels();
const { confirm } = useConfirm();

const props = defineProps<{
  provider: ProviderType;
  apiKeys: APIKeyItem[];
  models: Model[];
  availableModels?: AvailableModel[];
}>();

const emit = defineEmits<{
  (e: "refresh"): void;
}>();

const agentDefaults = computed(() => {
  const pd = state.value?.config?.provider_defaults?.[props.provider];
  if (pd) return pd;
  return state.value?.config?.agent_defaults ?? {
    max_steps: 25,
    context_budget: 8000,
    max_tokens: 3072,
    reasoning_budget: 0,
    tool_call_format: '',
    prefill: false,
  };
});

const providerModels = ref<import('../../types/model').ProviderModelInfo[]>([]);
const isLoadingModels = ref(false);
const editingModel = ref<Partial<Model> | null>(null);
const isAddingNew = ref(false);
const modelForm = ref<ModelForm>(createEmptyModelForm(props.provider, props.models, agentDefaults.value));

const filterText = ref("");
const lastDerivedName = ref("");

const filteredProviderModels = computed(() => {
  if (!filterText.value) return providerModels.value;
  const q = filterText.value.toLowerCase();
  return providerModels.value.filter((m) => m.id.toLowerCase().includes(q));
});

const groupsByKey = computed(() => {
  const groups: { keyName: string; models: Model[] }[] = [];
  
  if (props.provider === 'local') {
    const localModels = props.models.filter(m => m.provider === 'local');
    if (localModels.length > 0) {
      groups.push({ keyName: "Local Models", models: localModels });
    }
    return groups;
  }
  
  // Create a group for every API key
  for (const key of props.apiKeys) {
    groups.push({
      keyName: key.name,
      models: props.models.filter(m => m.provider === props.provider && m.provider_config?.api_key_name === key.name)
    });
  }
  
  // Find models that have no matching API key
  const noKeyModels = props.models.filter(m => {
    if (m.provider !== props.provider) return false;
    const keyName = m.provider_config?.api_key_name;
    return !keyName || !props.apiKeys.some(k => k.name === keyName);
  });
  
  if (noKeyModels.length > 0) {
    groups.push({
      keyName: "", // "No key assigned"
      models: noKeyModels
    });
  }
  
  return groups;
});

watch(
  () => props.apiKeys,
  () => {
    if (modelForm.value.key) {
      const stillExists = props.apiKeys.some(
        (k) => k.id === modelForm.value.key || k.name === modelForm.value.key,
      );
      if (!stillExists) modelForm.value.key = "";
    }
  },
  { deep: true },
);

watch(() => modelForm.value.id, (id) => {
  if (!id || !isAddingNew.value) return;
  const derived = deriveModelName(id);
  if (!modelForm.value.name || modelForm.value.name === lastDerivedName.value) {
    modelForm.value.name = derived;
    lastDerivedName.value = derived;
  }
  const selected = providerModels.value.find(m => m.id === id);
  // Pre-fill context_budget and max_tokens from model metadata so the form
  // shows realistic values before the user clicks Save.  The backend also
  // runs ApplyMetadataDefaults, but by pre-filling here the user sees the
  // actual values and can adjust before submitting.
  const ctx = selected?.meta?.n_ctx_train || selected?.limits?.context;
  const defaults = computeDefaultsFromContext(ctx);
  if (defaults) {
    modelForm.value.context_budget = defaults.context_budget;
    modelForm.value.max_tokens = defaults.max_tokens;
  }
});

watch(() => modelForm.value.key, (keyName) => {
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
    const list = await fetchProviderModels(props.provider, apiKeyName || modelForm.value.key);
    providerModels.value = list;
  } finally {
    isLoadingModels.value = false;
  }
}

function startAdd() {
  const defaults = getDefaultModelSettings(props.provider, agentDefaults.value);
  modelForm.value = createEmptyModelForm(props.provider, props.models, agentDefaults.value);
  
  editingModel.value = {
    name: "",
    provider: props.provider,
    filename: "",
    model_id: "",
    args: [],
    prefill: defaults.prefill,
    provider_config: { api_key_name: "" },
  };
  
  lastDerivedName.value = "";
  filterText.value = "";
  isAddingNew.value = true;
  
  if (props.provider !== "local") {
    loadModels();
  }
}

function scanAndAdd(keyName: string) {
  startAdd();
  modelForm.value.key = keyName;
  // loadModels is automatically triggered by watch(modelForm.value.key)
  window.scrollTo({ top: 0, behavior: 'smooth' });
}

function cancelEdit() {
  editingModel.value = null;
  isAddingNew.value = false;
}

async function saveNewModel() {
  const { name, key, id, filename, port, args, ...tuning } = modelForm.value;
  const finalName = name || deriveModelName(id, filename);
  
  if (props.provider === "local") {
    if (!filename) return;
    await addModel({
      name: finalName,
      provider: "local",
      filename,
      port,
      args: args ? args.split(/\s+/).filter(Boolean) : [],
      ...tuning
    });
  } else {
    if (!id) return;
    if (!modelForm.value.key) return;
    const selected = providerModels.value.find(m => m.id === id);
    await addModel({
      name: finalName,
      provider: props.provider,
      model_id: id,
      provider_config: { api_key_name: key },
      ...tuning,
      ...(selected?.pricing ? { pricing: selected.pricing } : {}),
      ...(selected?.limits ? { limits: selected.limits } : {}),
      ...(selected?.meta ? { meta: selected.meta } : {}),
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

// Manual discovery triggered by user via button instead of watcher

onMounted(() => {
  if (props.provider === 'local') {
    emit("refresh");
  }
});

function addDiscoveredModel(m: AvailableModel) {
  if (alreadyConfiguredFilenames.value.has(m.filename)) return;
  
  isAddingNew.value = true;
  const name = m.metadata?.name || m.name;
  modelForm.value = createEmptyModelForm("local", props.models, agentDefaults.value);
  modelForm.value.name = name;
  modelForm.value.filename = m.filename;
  
  editingModel.value = {
    name,
    provider: "local",
    filename: m.filename,
    args: [],
    prefill: modelForm.value.prefill,
    provider_config: { api_key_name: "" },
  };
  
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
      return !modelForm.value.filename;
    }
    return !modelForm.value.id || !modelForm.value.key;
  }
  return !editingModel.value?.name;
});

// Redundant discovery logic removed in favor of native loadModels function
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
                v-model="modelForm.filename"
                type="text"
                class="form-input"
                placeholder="e.g. qwen2.5-7b-instruct-q4_k_m.gguf"
              />
            </div>
            <div class="form-group">
              <label class="form-label">Friendly Name</label>
              <input
                v-model="modelForm.name"
                type="text"
                class="form-input"
                placeholder="Auto-derived from filename if empty"
              />
            </div>
            <div class="form-group">
              <label class="form-label">Port</label>
              <input
                v-model.number="modelForm.port"
                type="number"
                class="form-input"
              />
            </div>
            <div class="form-group">
              <label class="form-label">Custom Arguments</label>
              <input
                v-model="modelForm.args"
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
                  v-model.number="modelForm.max_steps"
                  type="number"
                  class="form-input"
                  placeholder="25 (default)"
                  min="1" max="100"
                />
              </div>
              <div class="form-group">
                <label class="form-label">Context Budget (chars)</label>
                <input
                  v-model.number="modelForm.context_budget"
                  type="number"
                  class="form-input"
                  placeholder="8000"
                  min="1000" max="100000" step="1000"
                />
              </div>
              <div class="form-group">
                <label class="form-label">Tool Call Format</label>
                <select v-model="modelForm.tool_call_format" class="form-input">
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
                    v-model="modelForm.prefill"
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
              <select v-model="modelForm.key" class="form-input">
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
                  v-model="modelForm.id"
                  class="form-input model-filter-select"
                  size="5"
                >
                  <option
                    v-for="m in filteredProviderModels"
                    :key="m.id"
                    :value="m.id"
                  >
                    {{ m.id }}
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
                <div v-else class="flex flex-col gap-2">
                  <input
                    v-model="modelForm.id"
                    type="text"
                    class="form-input"
                    placeholder="e.g. gpt-4o"
                  />
                  <BaseButton
                    variant="secondary"
                    size="sm"
                    icon="spinner"
                    :loading="isLoadingModels"
                    @click="loadModels(modelForm.key)"
                    class="w-fit"
                  >
                    Scan Endpoint for Models
                  </BaseButton>
                </div>
              </div>
            <div class="form-group">
              <label class="form-label">Friendly Name</label>
              <input
                v-model="modelForm.name"
                type="text"
                class="form-input"
                placeholder="Auto-derived from model ID"
              />
            </div>
            <div class="form-section-divider">Agent Tuning (per-model overrides)</div>
            <div class="tuning-grid">
              <div class="form-group">
                <label class="form-label">Max Steps</label>
                <input
                  v-model.number="modelForm.max_steps"
                  type="number"
                  class="form-input"
                  placeholder="25 (default)"
                  min="1" max="100"
                />
              </div>
              <div class="form-group">
                <label class="form-label">Context Budget (chars)</label>
                <input
                  v-model.number="modelForm.context_budget"
                  type="number"
                  class="form-input"
                  placeholder="8000"
                  min="1000" max="100000" step="1000"
                />
              </div>
              <div class="form-group">
                <label class="form-label">Tool Call Format</label>
                <select v-model="modelForm.tool_call_format" class="form-input">
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
                    v-model="modelForm.prefill"
                    class="rounded border-gray-600 bg-gray-700 text-blue-600 focus:ring-blue-600"
                  />
                  <span class="text-sm text-gray-300">Prefill tool calls</span>
                </label>
              </div>
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
                  placeholder="8000"
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
              <div class="flex items-center justify-between mb-1">
                <label class="form-label mb-0">Model ID</label>
              </div>
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
                  placeholder="8000"
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



      <div
        v-for="group in groupsByKey"
        :key="group.keyName || 'no-key'"
        class="model-group"
      >
        <div class="group-header">
          <div class="flex items-center gap-3">
            <span class="group-key-label">
              {{ group.keyName || "No key assigned" }}
            </span>
            <span class="group-count">{{ group.models.length }} model(s)</span>
          </div>
          <BaseButton
            v-if="group.keyName"
            variant="ghost"
            size="sm"
            icon="search"
            @click="scanAndAdd(group.keyName)"
            title="Scan this endpoint and add a model"
          >
            Discover Models
          </BaseButton>
        </div>
        
        <div v-if="group.models.length === 0" class="py-3 px-4 text-xs text-gray-500 italic border border-dashed border-gray-700/50 rounded-lg mx-2 my-2 bg-gray-800/30">
          No models configured for this endpoint yet.
        </div>

        <div
          v-for="m in group.models"
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
