<script setup lang="ts">
import { ref, computed, watch, onMounted } from "vue";
import ModelItem from "./models/ModelItem.vue";
import LocalFields from "./models/LocalFields.vue";
import CloudFields from "./models/CloudFields.vue";
import DiscoveredList from "./models/DiscoveredList.vue";
import { useProviders } from "../composables/useProviders";
import {
  makeEmptyForm,
  tabToDefaultProvider,
  filterModelsByTab,
  deriveFriendlyName,
} from "../utils/models";
import type { AdminState, AvailableModel, NewModelForm, Model } from "../types";

const props = defineProps<{
  state: AdminState | null;
  availableModels: AvailableModel[];
  filterProvider: "local" | "cloud";
}>();

const emit = defineEmits<{
  (e: "startModel", name: string): void;
  (e: "stopModel"): void;
  (e: "removeModel", name: string): void;
  (e: "updateModel", model: Model): void;
  (e: "addModel", model: NewModelForm): void;
}>();

const { cloudProviders, getLabel, fetchManifests } = useProviders();

// Form state lives entirely here — no prop sync, no watcher loop.
const form = ref<NewModelForm>(
  makeEmptyForm(tabToDefaultProvider(props.filterProvider)),
);

watch(
  () => props.filterProvider,
  (tab) => {
    form.value = makeEmptyForm(tabToDefaultProvider(tab));
  },
  { immediate: true },
);

// Auto-default model name from ID when selected
const lastDerivedName = ref("");
watch(
  () => form.value.model_id,
  (newId) => {
    if (!newId || form.value.provider === "local") return;

    const derived = deriveFriendlyName(newId);

    // Update if empty OR if the user hasn't changed it since the last auto-default
    if (!form.value.name || form.value.name === lastDerivedName.value) {
      form.value.name = derived;
      lastDerivedName.value = derived;
    }
  },
);

const filteredModels = computed(() =>
  filterModelsByTab(props.state?.models ?? [], props.filterProvider),
);

function submitModel() {
  emit("addModel", form.value);
  form.value = makeEmptyForm(form.value.provider);
}

function selectAvailableModel(model: AvailableModel) {
  form.value = {
    ...form.value,
    provider: "local",
    name: model.name,
    filename: model.filename,
    port: props.state?.next_port || 8000,
    args: "",
  };
}

const editingModelName = ref<string | null>(null);
function handleStartEdit(model: Model) {
  editingModelName.value = model.name;
}
function handleCancelEdit() {
  editingModelName.value = null;
}
function handleUpdateModel(model: Model) {
  emit("updateModel", model);
  editingModelName.value = null;
}

onMounted(() => {
  fetchManifests();
});
</script>

<template>
  <div class="model-manager-container">
    <!-- Configured Models List -->
    <div class="models-box">
      <header class="models-header">
        <h2 class="models-title">
          {{ filterProvider === "local" ? "Local Instances" : "Cloud Models" }}
          <span class="models-count">{{ filteredModels.length }} Models</span>
        </h2>
      </header>

      <div class="models-list-wrapper">
        <div v-if="filteredModels.length === 0" class="models-empty">
          No {{ filterProvider }} models configured.
        </div>
        <div class="models-list" v-else>
          <ModelItem
            v-for="model in filteredModels"
            :key="model.name"
            :model="model"
            :state="state"
            :is-editing="editingModelName === model.name"
            @start-model="$emit('startModel', $event)"
            @stop-model="$emit('stopModel')"
            @remove-model="$emit('removeModel', $event)"
            @start-edit="handleStartEdit"
            @cancel-edit="handleCancelEdit"
            @update-model="handleUpdateModel"
          />
        </div>
      </div>
    </div>

    <!-- Add Model Form + Discovery -->
    <div class="models-box">
      <h2 class="add-title">Add New Model</h2>

      <form @submit.prevent="submitModel" class="add-form">
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-3 mb-4">
          <div>
            <label class="form-label">Provider</label>
            <select v-model="form.provider" class="form-input">
              <template v-if="filterProvider === 'local'">
                <option value="local">Local Engine (Llama.cpp)</option>
              </template>
              <template v-else>
                <option v-for="p in cloudProviders" :key="p" :value="p">
                  {{ getLabel(p) }}
                </option>
              </template>
            </select>
          </div>
          <div>
            <label class="form-label">Friendly Name</label>
            <input
              v-model="form.name"
              type="text"
              required
              placeholder="e.g. My-Model"
              class="form-input"
            />
          </div>
        </div>

        <div class="provider-fields-container">
          <LocalFields v-if="form.provider === 'local'" v-model="form" />
          <CloudFields
            v-else
            :provider="form.provider"
            v-model:model-id="form.model_id!"
            v-model:api-key-name="form.provider_config!.api_key_name!"
            :state="state"
          />
        </div>

        <button
          type="submit"
          class="btn-add"
          :disabled="form.provider !== 'local' && !form.model_id"
        >
          Add to Configuration
        </button>
      </form>

      <DiscoveredList
        v-if="filterProvider === 'local'"
        :available-models="availableModels"
        @select="selectAvailableModel"
      />
    </div>
  </div>
</template>

<style scoped lang="postcss">
.model-manager-container {
  @apply grid grid-cols-1 lg:grid-cols-2 gap-6;
}
.models-box {
  @apply bg-gray-800 rounded-lg shadow-xl border border-gray-700 p-5 flex flex-col min-h-[300px] h-[500px];
}
.models-header {
  @apply mb-4 flex justify-between items-center;
}
.models-title {
  @apply text-lg font-semibold text-white flex items-center gap-3;
}
.models-count {
  @apply text-[10px] bg-gray-700 px-2 py-0.5 rounded text-gray-400;
}
.models-list-wrapper {
  @apply overflow-y-auto flex-1 pr-2;
}
.models-empty {
  @apply text-center text-gray-500 py-10 italic text-sm;
}
.models-list {
  @apply space-y-3;
}
.add-title {
  @apply text-lg font-semibold text-white mb-4;
}
.add-form {
  @apply mb-6 p-5 bg-gray-900 border border-gray-700 rounded-lg shadow-inner;
}
.form-label {
  @apply block text-[10px] uppercase font-bold text-gray-500 mb-1.5 tracking-wider;
}
.form-input {
  @apply w-full bg-gray-800 border border-gray-700 rounded-md px-3 py-2 text-sm text-white
         focus:border-blue-600 focus:ring-1 focus:ring-blue-600 outline-none transition-all;
}
select.form-input {
  @apply appearance-none
         bg-[url('data:image/svg+xml;charset=utf-8,%3Csvg%20xmlns%3D%22http%3A%2F%2Fwww.w3.org%2F2000%2Fsvg%22%20fill%3D%22none%22%20viewBox%3D%220%200%2020%2020%22%3E%3Cpath%20stroke%3D%22%236b7280%22%20stroke-linecap%3D%22round%22%20stroke-linejoin%3D%22round%22%20stroke-width%3D%221.5%22%20d%3D%22m6%208%204%204%204-4%22%2F%3E%3C%2Fsvg%3E')]
         bg-[position:right_0.5rem_center] bg-[length:1.25rem_1.25rem] bg-no-repeat pr-10;
}
.btn-add {
  @apply w-full bg-blue-700 hover:bg-blue-600 disabled:opacity-30 disabled:grayscale
         text-white py-2.5 rounded-md text-xs font-black uppercase tracking-widest
         transition-all mt-4 shadow-lg hover:shadow-blue-600/20 active:scale-[0.98];
}
.provider-fields-container {
  @apply min-h-[80px];
}
</style>
