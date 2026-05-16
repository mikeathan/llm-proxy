<script setup lang="ts">
import { ref } from "vue";
import { useModels } from "../../composables/useModels";
import { getDefaultModelSettings } from "../../utils/modelUtils";
import BaseButton from "../common/BaseButton.vue";
import type { Model, AvailableModel } from "../../types/model";

const {
  models,
  availableModels,
  addModel,
  updateModel,
  removeModel,
  nextPort,
  agentDefaults,
} = useModels();

const editingModel = ref<Partial<Model> | null>(null);
const isAddingNew = ref(false);

const argsToString = (args?: string[]) => args?.join(" ") || "";
const stringToArgs = (str: string) =>
  str.split(/\s+/).filter((s) => s.length > 0);

const rawArgs = ref("");

function handleEdit(model: Model) {
  editingModel.value = JSON.parse(JSON.stringify(model));
  rawArgs.value = argsToString(editingModel.value?.args);
  isAddingNew.value = false;
}

function handleAddNew(available?: AvailableModel) {
  const tuning = getDefaultModelSettings("local", agentDefaults.value);
  if (available) {
    editingModel.value = {
      name: available.name,
      provider: "local",
      filename: available.filename,
      port: nextPort.value,
      args: [],
      metadata: available.metadata,
      ...tuning,
      max_tokens: tuning.max_tokens,
      reasoning_budget: tuning.reasoning_budget,
      slot_timeout: 0,
    };
  } else {
    editingModel.value = {
      name: "",
      provider: "local",
      filename: "",
      port: nextPort.value,
      args: [],
      ...tuning,
      max_tokens: tuning.max_tokens,
      reasoning_budget: tuning.reasoning_budget,
      slot_timeout: 0,
    };
  }
  rawArgs.value = "";
  isAddingNew.value = true;
}

async function saveModel() {
  if (!editingModel.value) return;

  const payload = {
    ...editingModel.value,
    args: stringToArgs(rawArgs.value),
  };

  if (isAddingNew.value) {
    await addModel(payload);
  } else {
    await updateModel(payload);
  }
  editingModel.value = null;
}

function cancelEdit() {
  editingModel.value = null;
}

const formatSize = (bytes: number) => {
  if (bytes === 0) return "0 B";
  const k = 1024;
  const sizes = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + " " + sizes[i];
};
</script>

<template>
  <div class="catalogue-container">
    <div class="header-row">
      <h2 class="title">Model Catalogue</h2>
      <BaseButton
        v-if="!editingModel"
        variant="primary"
        icon="plus"
        @click="handleAddNew()"
      >
        Manual Add
      </BaseButton>
    </div>

    <!-- Edit/Add Form -->
    <div
      v-if="editingModel"
      class="form-card animate-in zoom-in-95 duration-200"
    >
      <h3 class="form-title">
        {{ isAddingNew ? "Register New Model" : "Edit Model Settings" }}
      </h3>

      <div class="form-grid">
        <div class="form-group">
          <label>Display Name</label>
          <input
            v-model="editingModel.name"
            type="text"
            class="form-input"
            placeholder="e.g. My Llama"
          />
        </div>

        <div class="form-group">
          <label>Provider</label>
          <select v-model="editingModel.provider" class="form-input">
            <option value="local">Local (llama.cpp)</option>
            <option value="gemini">Google Gemini</option>
            <option value="openai">OpenAI Compatible</option>
            <option value="nvidia">NVIDIA NIM</option>
            <option value="vertex">Google Vertex</option>
          </select>
        </div>

        <div class="form-group">
          <label>Model ID / Filename</label>
          <input
            v-model="editingModel.filename"
            type="text"
            class="form-input"
            placeholder="e.g. llama3.gguf"
          />
        </div>

        <div class="form-group" v-if="editingModel.provider === 'local'">
          <label>Port</label>
          <input
            v-model.number="editingModel.port"
            type="number"
            class="form-input"
          />
        </div>

        <div class="form-group col-span-2">
          <label>Custom Arguments</label>
          <textarea
            v-model="rawArgs"
            class="form-input font-mono text-xs"
            rows="2"
            placeholder="--ctx-size 8192 --n-gpu-layers 32"
          ></textarea>
          <p class="helper-text">
            These arguments override global defaults for this specific model.
          </p>
        </div>

        <!-- Agent Tuning -->
        <div class="col-span-2 mt-4">
          <h4 class="form-section-title">Agent Tuning (per-model overrides)</h4>
        </div>

        <div class="form-group">
          <label>Max Steps</label>
          <input
            v-model.number="editingModel.max_steps"
            type="number"
            class="form-input"
            placeholder="25 (default)"
            min="1"
            max="100"
          />
          <p class="helper-text">Max agent loop iterations before forced exit.</p>
        </div>

        <div class="form-group">
          <label>Context Budget (chars)</label>
          <input
            v-model.number="editingModel.context_budget"
            type="number"
            class="form-input"
            placeholder="8000"
            min="1000"
            max="100000"
            step="1000"
          />
          <p class="helper-text">Character count that triggers context pruning.</p>
        </div>

        <div class="form-group">
          <label>Max Tokens (output)</label>
          <input
            v-model.number="editingModel.max_tokens"
            type="number"
            class="form-input"
            placeholder="2048"
            min="64"
            max="32768"
            step="256"
          />
          <p class="helper-text">Limits LLM response length per turn.</p>
        </div>

        <div class="form-group">
          <label>Reasoning Budget</label>
          <input
            v-model.number="editingModel.reasoning_budget"
            type="number"
            class="form-input"
            placeholder="0 (provider default)"
            min="0"
            max="32768"
            step="512"
          />
          <p class="helper-text">Max thinking tokens for reasoning models. 0 = unlimited.</p>
        </div>

        <div class="form-group" v-if="editingModel.provider === 'local'">
          <label>Slot Timeout (seconds)</label>
          <input
            v-model.number="editingModel.slot_timeout"
            type="number"
            class="form-input"
            placeholder="0 (disabled)"
            min="0"
            max="86400"
            step="60"
          />
          <p class="helper-text">How long to keep the llama.cpp KV cache slot alive between requests.</p>
        </div>

        <div class="form-group">
          <label>Tool Call Format</label>
          <select v-model="editingModel.tool_call_format" class="form-input">
            <option value="">Default (native)</option>
            <option value="native">Native Tools (OpenAI function calling)</option>
            <option value="xml">XML Text (model writes &lt;tool_call&gt; tags)</option>
          </select>
          <p class="helper-text">How tools are presented to the model.</p>
        </div>

        <div class="form-group">
          <label>Prefill</label>
          <div class="toggle-row">
            <label class="toggle">
              <input
                v-model="editingModel.prefill"
                type="checkbox"
                class="toggle-input"
              />
              <span class="toggle-slider"></span>
            </label>
            <span class="toggle-label">{{
              editingModel.prefill ? "Enabled" : "Disabled"
            }}</span>
          </div>
          <p class="helper-text">Prefill assistant response with &lt;tool_call&gt; opener in automation mode.</p>
        </div>
      </div>

      <div class="form-actions">
        <BaseButton variant="secondary" @click="cancelEdit">Cancel</BaseButton>
        <BaseButton variant="primary" @click="saveModel"
          >Save Configuration</BaseButton
        >
      </div>
    </div>

    <!-- Active Models List -->
    <div v-else class="space-y-6">
      <section class="section">
        <h3 class="section-subtitle">Configured Models</h3>
        <div class="model-grid">
          <div v-for="m in models" :key="m.name" class="model-card">
            <div class="model-info">
              <div class="model-main">
                <span class="model-name">{{ m.name }}</span>
                <span
                  :class="[
                    'badge',
                    m.provider === 'local' ? 'badge-blue' : 'badge-purple',
                  ]"
                >
                  {{ m.provider }}
                </span>
              </div>
              <div class="model-details">
                <span class="detail-item" v-if="m.filename"
                  >📄 {{ m.filename }}</span
                >
                <span class="detail-item" v-if="m.port">🔌 :{{ m.port }}</span>
                <span class="detail-item" v-if="m.args && m.args.length"
                  >⚙️ {{ m.args.length }} args</span
                >
              </div>
            </div>
            <div class="model-actions">
              <button @click="handleEdit(m)" class="action-btn" title="Edit">
                ✏️
              </button>
              <button
                @click="removeModel(m.name)"
                class="action-btn text-red-400"
                title="Delete"
              >
                🗑️
              </button>
            </div>
          </div>
        </div>
      </section>

      <!-- Available on Disk (Discovery) -->
      <section v-if="availableModels.length > 0" class="section">
        <h3 class="section-subtitle">Discovered on Disk (.gguf)</h3>
        <div class="available-list">
          <div
            v-for="a in availableModels"
            :key="a.filename"
            class="available-item"
          >
            <div class="available-info">
              <span class="available-name">{{ a.name }}</span>
              <span class="available-meta"
                >{{ a.filename }} • {{ formatSize(a.size_bytes) }}</span
              >
            </div>
            <BaseButton variant="secondary" size="sm" @click="handleAddNew(a)">
              Add to Catalogue
            </BaseButton>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>

<style scoped lang="postcss">
.catalogue-container {
  @apply space-y-6 animate-in fade-in duration-500;
}

.header-row {
  @apply flex justify-between items-center mb-4;
}

.title {
  @apply text-xl font-bold text-white;
}

.section {
  @apply space-y-3;
}

.section-subtitle {
  @apply text-xs font-bold text-gray-500 uppercase tracking-widest px-1;
}

.form-card {
  @apply bg-gray-800/50 backdrop-blur-md border border-gray-700 rounded-xl p-6 space-y-6 shadow-2xl;
}

.form-title {
  @apply text-lg font-bold text-white border-b border-gray-700 pb-3;
}

.form-grid {
  @apply grid grid-cols-1 md:grid-cols-2 gap-4;
}

.form-group {
  @apply space-y-1.5;
}

.form-group label {
  @apply block text-sm font-semibold text-gray-300;
}

.form-input {
  @apply w-full bg-gray-900/50 border border-gray-700 rounded-lg px-3 py-2 text-white focus:ring-2 focus:ring-blue-500/50 outline-none transition-all;
}

.helper-text {
  @apply text-[10px] text-gray-500 mt-1;
}

.form-section-title {
  @apply text-sm font-bold text-blue-400 uppercase tracking-wide border-t border-gray-700 pt-3;
}

.toggle-row {
  @apply flex items-center gap-3 mt-1;
}

.toggle {
  @apply relative inline-block w-9 h-5 cursor-pointer;
}

.toggle-input {
  @apply sr-only;
}

.toggle-input:checked + .toggle-slider {
  @apply bg-emerald-500;
}

.toggle-slider {
  @apply absolute inset-0 rounded-full bg-gray-600 transition-colors;
}

.toggle-slider::after {
  content: '';
  @apply absolute top-0.5 left-0.5 w-4 h-4 rounded-full bg-white transition-transform;
}

.toggle-input:checked + .toggle-slider::after {
  transform: translateX(1rem);
}

.toggle-label {
  @apply text-sm text-gray-300 font-medium;
}

.form-actions {
  @apply flex justify-end gap-3 pt-4 border-t border-gray-700;
}

.model-grid {
  @apply grid grid-cols-1 gap-3;
}

.model-card {
  @apply bg-gray-800/30 hover:bg-gray-800/50 border border-gray-700/50 rounded-xl p-4 flex justify-between items-center transition-all;
}

.model-main {
  @apply flex items-center gap-3 mb-1;
}

.model-name {
  @apply font-bold text-white text-base;
}

.badge {
  @apply text-[10px] px-2 py-0.5 rounded-full font-bold uppercase;
}

.badge-blue {
  @apply bg-blue-500/10 text-blue-400 border border-blue-500/20;
}
.badge-purple {
  @apply bg-purple-500/10 text-purple-400 border border-purple-500/20;
}

.model-details {
  @apply flex flex-wrap gap-x-4 gap-y-1;
}

.detail-item {
  @apply text-xs text-gray-500;
}

.model-actions {
  @apply flex gap-2;
}

.action-btn {
  @apply p-2 rounded-lg hover:bg-gray-700 transition-all filter grayscale hover:grayscale-0;
}

.available-list {
  @apply bg-gray-900/30 rounded-xl border border-gray-800 divide-y divide-gray-800;
}

.available-item {
  @apply p-4 flex justify-between items-center hover:bg-gray-800/20 transition-all;
}

.available-info {
  @apply flex flex-col;
}

.available-name {
  @apply text-sm font-bold text-gray-200;
}

.available-meta {
  @apply text-xs text-gray-500;
}
</style>
