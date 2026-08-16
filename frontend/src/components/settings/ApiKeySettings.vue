<script setup lang="ts">
import { ref } from "vue";
import type { APIKeyItem } from "../../types/admin";
import InlineConfirm from "../ui/InlineConfirm.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import Icon from "../icons/Icon.vue"

const props = defineProps<{
  apiKeys: APIKeyItem[];
  title: string;
  helperText: string;
  testLoading: boolean;
  testSuccess?: string;
  testError?: string;
  showBaseUrl?: boolean;
  modelCounts?: Record<string, number>;
}>();

const emit = defineEmits<{
  (e: "update:apiKeys", keys: APIKeyItem[]): void;
  (e: "testKey", payload: { key: string; name: string; id: string; base_url?: string }): void;
  (e: "clearTest"): void;
  (e: "clearAll"): void;
}>();

import { watch, computed } from "vue";

const removeConfirmMessage = computed(() => {
  const count = selectedId.value ? (props.modelCounts?.[editName.value] || 0) : 0;
  const base = `Remove '${editName.value}'?`;
  if (count > 0) {
    return `${base} ${count} model(s) reference this key and will also be removed.`;
  }
  return base;
});

watch(
  () => props.apiKeys,
  (newKeys) => {
    if (selectedId.value && !newKeys.find((k) => k.id === selectedId.value)) {
      closePanel();
    }
  },
  { deep: true },
);

const selectedId = ref<string | null>(null);
const editName = ref("");
const editValue = ref("");
const editBaseUrl = ref("");
const showValue = ref(false);
const confirmRemove = ref(false); // inline confirm — never use window.confirm
const confirmClearAll = ref(false);

function openPanel(id: string) {
  const item = props.apiKeys.find((k) => k.id === id);
  if (!item) return;
  selectedId.value = id;
  editName.value = item.name;
  editValue.value = item.key;
  editBaseUrl.value = item.base_url || "";
  showValue.value = false;
  confirmRemove.value = false;
  emit("clearTest");
}

function closePanel() {
  selectedId.value = null;
  confirmRemove.value = false;
  emit("clearTest");
}

function onItemClick(id: string) {
  selectedId.value === id ? closePanel() : openPanel(id);
}

function saveEdit() {
  const currentId = selectedId.value;
  if (!currentId) return;
  const newName = editName.value.trim();
  const newKey = editValue.value.trim();
  if (!newName || !newKey) return;

  const updated = props.apiKeys.map((k) =>
    k.id === currentId
      ? {
          ...k,
          name: newName,
          key: newKey,
          base_url: editBaseUrl.value.trim() || "",
        }
      : k,
  );
  emit("update:apiKeys", updated);
}

function confirmAndRemove() {
  const id = selectedId.value;
  if (!id) return;
  const updated = props.apiKeys.filter((k) => k.id !== id);
  closePanel();
  emit("update:apiKeys", updated);
}

function clearAllKeys() {
  confirmClearAll.value = false;
  emit("clearAll");
}

function testSelected() {
  if (!selectedId.value) return;
  emit("testKey", {
    key: editValue.value,
    name: editName.value,
    id: selectedId.value,
    base_url: editBaseUrl.value.trim() || undefined,
  });
}

// ─────────────────────────────────────────────────────────────
// Add new key
// ─────────────────────────────────────────────────────────────
const newKeyName = ref("");
const newKeyValue = ref("");
const newKeyBaseUrl = ref("");

function addKey() {
  const name = newKeyName.value.trim() || `Key ${props.apiKeys.length + 1}`;
  const key = newKeyValue.value.trim();
  if (!key) return;

  const id =
    typeof crypto !== "undefined" && crypto.randomUUID
      ? crypto.randomUUID()
      : Math.random().toString(36).substring(2, 11);
  const newItem: APIKeyItem = {
    id,
    name,
    key,
    base_url: newKeyBaseUrl.value.trim() || undefined,
  };

  emit("update:apiKeys", [...props.apiKeys, newItem]);

  selectedId.value = id;
  editName.value = name;
  editValue.value = key;
  editBaseUrl.value = newItem.base_url || "";
  showValue.value = false;
  confirmRemove.value = false;

  newKeyName.value = "";
  newKeyValue.value = "";
  newKeyBaseUrl.value = "";
}
</script>

<template>
  <div class="api-key-manager">
    <!-- ── Header ── -->
    <div class="section-header">
      <label class="form-label">{{ title }}</label>
      <div class="form-helper">{{ helperText }}</div>
    </div>

    <!-- ── Key list ── -->
    <div class="keys-list">
      <div v-if="apiKeys.length === 0" class="empty-keys">
        No API keys configured yet. Add one below.
      </div>

      <button
        v-for="item in apiKeys"
        :key="item.id"
        type="button"
        class="key-item"
        :class="{ 'key-item--selected': selectedId === item.id }"
        @click="onItemClick(item.id)"
      >
        <span
          class="key-dot"
          :class="{ 'key-dot--active': selectedId === item.id }"
        ></span>
        <div class="key-info">
          <div class="key-name">{{ item.name }}</div>
          <div class="key-detail">
            <span class="key-preview">••••••••{{ item.key.slice(-4) }}</span>
            <span v-if="item.base_url" class="key-url">{{ item.base_url }}</span>
          </div>
        </div>
        <span v-if="selectedId === item.id" class="key-badge">Selected</span>
        <span
          v-if="modelCounts?.[item.name]"
          class="model-count-badge"
          :title="`${modelCounts[item.name]} model(s) use this key`"
        >
          {{ modelCounts[item.name] }}
        </span>
      </button>
    </div>

    <!-- ── Edit panel ── -->
    <div v-show="selectedId !== null" class="edit-panel">
      <div class="edit-panel-header">
        <span class="edit-panel-title">Edit: {{ editName }}</span>
        <BaseButton
          variant="ghost"
          size="sm"
          icon="close"
          iconOnly
          @click.stop="closePanel"
          title="Close"
        />
      </div>

      <div class="edit-panel-body">
        <!-- Fields -->
        <div class="field">
          <label class="field-label">Key Name</label>
          <input
            v-model="editName"
            type="text"
            class="form-input"
            placeholder="e.g. Personal"
            autocomplete="off"
          />
        </div>

        <div class="field">
          <label class="field-label">API Key Value</label>
          <div class="input-row">
            <input
              v-model="editValue"
              :type="showValue ? 'text' : 'password'"
              class="form-input"
              placeholder="Paste new key to update"
              autocomplete="new-password"
              data-1p-ignore
              data-lpignore="true"
            />
            <BaseButton
              variant="secondary"
              size="sm"
              :icon="showValue ? 'spinner' : 'document'"
              iconOnly
              @click.stop="showValue = !showValue"
              title="Toggle Visibility"
            />
          </div>
        </div>

        <div v-if="showBaseUrl" class="field">
          <label class="field-label">Base URL</label>
          <div class="form-helper">API endpoint for this key</div>
          <input
            v-model="editBaseUrl"
            type="text"
            class="form-input"
            placeholder="https://api.openai.com/v1"
            autocomplete="off"
          />
        </div>

        <!-- Test feedback -->
        <div
          v-if="testLoading || testSuccess || testError"
          class="test-feedback"
        >
          <div v-if="testLoading" class="test-loading">
            <Icon name="spinner" size="xs" /> Verifying…
          </div>
          <div v-else-if="testSuccess" class="test-success">
            <Icon name="check" size="xs" class="inline" /> {{ testSuccess }}
          </div>
          <div v-else-if="testError" class="test-error">
            <Icon name="close" size="xs" class="inline" /> {{ testError }}
          </div>
        </div>

        <!-- Inline remove confirmation — no window.confirm, no dialog, no event issues -->
        <InlineConfirm
          v-if="confirmRemove"
          :message="removeConfirmMessage"
          @confirm="confirmAndRemove"
          @cancel="confirmRemove = false"
        />

        <!-- Action bar -->
        <div class="panel-actions">
          <BaseButton
            variant="secondary"
            size="sm"
            icon="search"
            :loading="testLoading"
            @click.stop="testSelected"
          >
            Test Connection
          </BaseButton>
          <div class="action-row">
            <BaseButton
              variant="danger"
              size="sm"
              icon="trash"
              @click.stop="confirmRemove = true"
            >
              Remove
            </BaseButton>
            <BaseButton
              variant="primary"
              size="sm"
              icon="check"
              @click.stop="saveEdit"
            >
              Save Changes
            </BaseButton>
          </div>
        </div>
      </div>
    </div>

    <!-- ── Add new key ── -->
    <div class="add-section">
      <div class="add-header">
        <span>Add New API Key</span>
        <div class="add-header-actions">
          <!-- Inline clear-all confirmation -->
          <InlineConfirm
            v-if="confirmClearAll && apiKeys.length > 0"
            :message="`Remove all ${apiKeys.length} key(s)?`"
            @confirm="clearAllKeys"
            @cancel="confirmClearAll = false"
          />
          <BaseButton
            v-if="apiKeys.length > 0 && !confirmClearAll"
            variant="ghost"
            size="sm"
            icon="trash"
            @click.stop="confirmClearAll = true"
            title="Clear All Keys"
          >
            Clear All
          </BaseButton>
        </div>
      </div>
      <div class="add-body">
        <input
          v-model="newKeyName"
          type="text"
          class="form-input"
          placeholder="Key name (e.g. Personal)"
          autocomplete="off"
        />
        <div class="input-row">
          <input
            v-model="newKeyValue"
            type="password"
            class="form-input"
            placeholder="API Key value"
            autocomplete="new-password"
            data-1p-ignore
            data-lpignore="true"
          />
          <BaseButton
            variant="primary"
            size="sm"
            icon="plus"
            :disabled="!newKeyValue"
            @click.stop="addKey"
          >
            Add
          </BaseButton>
        </div>
        <div v-if="showBaseUrl" class="add-base-url">
          <input
            v-model="newKeyBaseUrl"
            type="text"
            class="form-input"
            placeholder="Base URL (e.g. https://api.openai.com/v1)"
            autocomplete="off"
          />
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped lang="postcss">
/* ── Typography & inputs ── */
.form-label {
  @apply block text-sm font-semibold text-gray-200;
}
.form-helper {
  @apply text-xs text-gray-500 mb-3;
}
.form-input {
  @apply w-full bg-gray-900 border border-gray-700 rounded-md px-3 py-2 text-white text-sm
         transition-all focus:ring-2 focus:ring-blue-600/30 focus:border-blue-600 outline-none;
}
.input-row {
  @apply flex gap-2;
}
.input-row .form-input {
  @apply flex-1;
}

/* ── Key list ── */
.keys-list {
  @apply border border-gray-700 rounded-lg overflow-hidden bg-gray-900/50 mb-3;
}
.key-item {
  @apply w-full flex items-center gap-3 px-3 py-3
         border-b border-gray-700 last:border-0
         cursor-pointer transition-colors hover:bg-gray-700/40 text-left;
}
.key-item--selected {
  @apply bg-blue-600/10;
}
.key-dot {
  @apply w-2 h-2 rounded-full bg-gray-600 shrink-0 transition-colors;
}
.key-dot--active {
  @apply bg-blue-400;
}
.key-info {
  @apply flex-1 min-w-0;
}
.key-name {
  @apply text-xs font-bold text-gray-200 truncate;
}
.key-detail {
  @apply flex items-center gap-2;
}
.key-preview {
  @apply text-[10px] text-gray-500 font-mono;
}
.key-url {
  @apply text-[10px] text-blue-400/60 font-mono truncate;
}
.key-badge {
  @apply text-[10px] font-bold text-blue-400 bg-blue-600/15
         border border-blue-600/30 px-2 py-0.5 rounded-full shrink-0;
}
.empty-keys {
  @apply p-4 text-center text-xs text-gray-500 italic;
}

/* ── Edit panel ── */
.edit-panel {
  @apply border border-blue-600/30 rounded-lg bg-blue-950/10 mb-4 overflow-hidden;
}
.edit-panel-header {
  @apply flex items-center justify-between px-4 py-2.5
         bg-blue-600/10 border-b border-blue-600/20;
}
.edit-panel-title {
  @apply text-xs font-bold text-blue-300 tracking-wide flex-1 min-w-0 truncate;
}
.btn-close {
  @apply text-gray-400 hover:text-white text-xs px-1.5 py-0.5 rounded transition-colors shrink-0 ml-2;
}
.edit-panel-body {
  @apply p-4 space-y-3;
}
.field {
  @apply space-y-1;
}
.field-label {
  @apply block text-xs font-semibold text-gray-400;
}

/* ── Inline remove confirmation ── */
.confirm-remove {
  @apply flex items-center justify-between gap-3 bg-red-950/30
         border border-red-500/30 rounded-md px-3 py-2;
}
.confirm-text {
  @apply text-xs text-red-300 font-medium flex-1 min-w-0 truncate;
}
.confirm-actions {
  @apply flex gap-2 shrink-0;
}
.btn-cancel {
  @apply text-gray-300 bg-gray-700 hover:bg-gray-600 text-xs px-3 py-1.5 rounded-md transition-all;
}
.btn-confirm {
  @apply text-red-200 bg-red-700 hover:bg-red-600 text-xs px-3 py-1.5 rounded-md font-bold transition-all;
}

/* ── Action bar ── */
.panel-actions {
  @apply flex items-center justify-between pt-3 border-t border-blue-600/15 mt-1;
}
.action-row {
  @apply flex gap-2;
}
.btn-test {
  @apply flex items-center gap-2 bg-gray-700 hover:bg-gray-600 disabled:opacity-50
         text-gray-200 text-xs px-3 py-2 rounded-md font-medium transition-all;
}
.btn-save {
  @apply bg-blue-600 hover:bg-blue-500 text-white text-xs px-3 py-2 rounded-md font-bold transition-all;
}
.btn-danger {
  @apply bg-red-900/40 hover:bg-red-900/70 text-red-300 text-xs px-3 py-2 rounded-md transition-all;
}
.btn-icon {
  @apply p-2 rounded-md bg-gray-700 hover:bg-gray-600 transition-colors text-xs shrink-0;
}

/* ── Add section ── */
.add-section {
  @apply border border-dashed border-gray-600 rounded-lg overflow-hidden;
}
.add-header {
  @apply text-xs font-bold text-gray-400 px-3 py-2 bg-gray-800/50
         border-b border-gray-700/50 uppercase tracking-wider
         flex items-center justify-between;
}
.add-header-actions {
  @apply flex items-center gap-2;
}
.add-body {
  @apply p-3 space-y-2;
}
.add-base-url {
  @apply pt-1;
}
.btn-add {
  @apply bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white
         px-4 py-2 rounded-md text-xs font-bold transition-all shrink-0;
}

/* ── Test feedback ── */
.test-feedback {
  @apply text-xs rounded-md;
}
.test-loading {
  @apply text-blue-400 flex items-center gap-2 px-2 py-1;
}
.test-success {
  @apply text-green-400 bg-green-950/20 border border-green-500/30 px-3 py-2 rounded;
}
.test-error {
  @apply text-red-400   bg-red-950/20   border border-red-500/30   px-3 py-2 rounded break-words;
}

/* ── Spinners ── */
.spinner {
  @apply inline-block w-3 h-3 border-2 border-blue-400 border-t-transparent rounded-full animate-spin;
}
.spinner-sm {
  @apply inline-block w-3 h-3 border border-current border-t-transparent rounded-full animate-spin;
}

.model-count-badge {
  @apply text-[9px] font-bold text-blue-400 bg-blue-600/15 border border-blue-600/30
         px-1.5 py-0.5 rounded-full shrink-0 ml-1 cursor-default;
}
</style>
