<script setup lang="ts">
import { ref, computed, toRef } from "vue";
import type { Automation } from "../../../types/dispatcher";
import { useAutomationForm } from "../../../composables/automation/useAutomationForm";
import CronEditor from "./CronEditor.vue";
import Icon from "../../icons/Icon.vue";

const props = defineProps<{
  workspaces: { id: string }[];
  workspaceFiles: Record<string, string[]>;
  hasAutomations: boolean;
  editAutomation: Automation | null;
}>();

const emit = defineEmits<{
  (e: "create-automation", workspace: string, data: any): void;
  (e: "update-automation", workspace: string, oldName: string, data: any): void;
  (e: "fetch-files", workspace: string): void;
  (e: "cancel-edit"): void;
}>();

const userCollapsed = ref(false);
const isCollapsed = computed(() => {
  if (props.editAutomation) return false;
  return props.hasAutomations && !userCollapsed.value;
});

const {
  selectedWorkspace,
  form,
  selectedProviderKey,
  filteredModels,
  cloudProvidersWithKeys,
  handleSubmit: validateSubmit,
  resetForm,
} = useAutomationForm(
  toRef(props, "editAutomation"),
  (ws) => emit("fetch-files", ws),
);

const handleSubmit = () => {
  const data = validateSubmit();
  if (!data) return;

  const payload = {
    name: data.name,
    trigger: {
      type: data.triggerType,
      value: data.triggerValue,
    },
    task_file: data.taskFile,
    strategy: data.strategy,
    model: data.model,
  };

  if (props.editAutomation) {
    emit("update-automation", selectedWorkspace.value, props.editAutomation.name, payload);
  } else {
    emit("create-automation", selectedWorkspace.value, payload);
    resetForm();
  }
};

const handleCancel = () => {
  if (props.editAutomation) {
    emit("cancel-edit");
  } else {
    userCollapsed.value = true;
  }
};
</script>

<template>
  <div class="form-container">
    <div
      class="form-header"
      @click="userCollapsed = !userCollapsed"
    >
      <div class="header-title">
        {{ editAutomation ? "Edit Automation" : "Create Automation" }}
      </div>
      <div class="header-actions">
        <button
          v-if="editAutomation"
          @click.stop="emit('cancel-edit')"
          class="btn-cancel-small"
        >
          Cancel
        </button>
        <div class="text-gray-400">
          <Icon name="chevron-up" size="sm" class="header-arrow" :class="{ 'header-arrow--collapsed': isCollapsed }" />
        </div>
      </div>
    </div>

    <div v-show="!isCollapsed" class="form-body">
      <!-- Workspace Selection (Disabled if editing) -->
      <div class="field-group">
        <label class="field-label">Workspace</label>
        <select
          v-model="selectedWorkspace"
          :disabled="!!editAutomation"
          class="select-input"
        >
          <option value="" disabled>Select Workspace...</option>
          <option v-for="ws in workspaces" :key="ws.id" :value="ws.id">
            {{ ws.id }}
          </option>
        </select>
      </div>

      <!-- Container for rest of form, disabled if no workspace -->
      <div
        :class="{ 'form-section-group--disabled': !selectedWorkspace }"
        class="form-section-group"
      >
        <!-- Name -->
        <div class="field-group">
          <label class="field-label">Automation Name</label>
          <input
            v-model="form.name"
            placeholder="e.g. daily-sync"
            class="text-input"
          />
        </div>

        <!-- Model Selection Section -->
        <div class="nested-config-section">
          <div class="flex items-center justify-between mb-1">
            <label class="section-header-label">Model Routing</label>
          </div>
          
          <div class="config-grid">
             <!-- Connection Selector -->
             <div class="config-field">
               <label class="field-label-tiny">Connection Source</label>
               <select 
                 v-model="selectedProviderKey"
                 class="select-input select-input--nested"
               >
                 <option value="" disabled>Select Provider/Location...</option>
                 <option value="local">Local AI Instance</option>
                 <optgroup v-for="p in cloudProvidersWithKeys" :key="p.providerName" :label="p.providerName.toUpperCase()">
                   <option v-for="k in p.keys" :key="k.id" :value="`${p.providerName}/${k.keyVal}`">
                     {{ p.providerName }} - {{ k.name }}
                   </option>
                 </optgroup>
               </select>
             </div>

             <!-- Model Selector -->
             <div class="config-field">
               <label class="field-label-tiny">Specific Model</label>
               <select 
                 v-model="form.model"
                 :disabled="!selectedProviderKey"
                 class="select-input select-input--nested"
               >
                 <option value="" disabled>{{ selectedProviderKey ? 'Choose a Model...' : 'Select Connection First' }}</option>
                 <option v-for="m in filteredModels" :key="m.name" :value="m.name">
                   {{ m.name }}
                 </option>
               </select>
             </div>
          </div>
        </div>

        <!-- Task File -->
        <div class="field-group">
          <label class="field-label">Task File</label>
          <select 
            v-model="form.taskFile" 
            class="select-input"
          >
            <option value="" disabled>Select File...</option>
            <option 
              v-for="file in (selectedWorkspace && workspaceFiles[selectedWorkspace] ? workspaceFiles[selectedWorkspace] : [])" 
              :key="file" 
              :value="file"
            >
              {{ file }}
            </option>
          </select>
        </div>

        <!-- Trigger Configuration -->
        <div class="trigger-config">
          <div class="trigger-header">
            <label class="field-label">Trigger Setup</label>
            <select
              v-model="form.triggerType"
              class="trigger-type-select"
            >
              <option value="cron">Schedule (Cron)</option>
              <option value="interval">Interval</option>
              <option value="manual">Manual Only</option>
            </select>
          </div>

          <CronEditor
            v-if="form.triggerType === 'cron'"
            :modelValue="form.triggerValue"
            :triggerType="form.triggerType"
            @update:modelValue="form.triggerValue = $event"
          />

          <div v-else-if="form.triggerType === 'interval'" class="trigger-content">
            <input
              v-model="form.triggerValue"
              placeholder="e.g. 5m, 1h, 24h"
              class="text-input"
            />
            <div class="manual-helper">
              Go duration format (m = minutes, h = hours)
            </div>
          </div>

          <div v-else class="manual-helper py-2">
            This automation will only run when triggered manually via the UI or
            API.
          </div>
        </div>

        <div class="form-footer-actions">
          <button
            v-if="editAutomation"
            @click="handleCancel"
            class="btn-cancel-wide"
          >
            Cancel
          </button>
          <button
            @click="handleSubmit"
            :disabled="
              !selectedWorkspace ||
              !form.name ||
              !form.taskFile ||
              (form.triggerType !== 'manual' && !form.triggerValue)
            "
            class="btn-submit-wide"
          >
            {{ editAutomation ? "Update Automation" : "Create Automation" }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped lang="postcss">
.form-container {
  @apply border-b border-gray-700 bg-gray-800;
}

.form-header {
  @apply p-4 py-3 flex items-center justify-between cursor-pointer select-none hover:bg-gray-700 transition-colors;
}

.header-title {
  @apply text-sm font-semibold text-gray-200;
}

.header-actions {
  @apply flex items-center gap-2;
}

.btn-cancel-small {
  @apply text-[10px] uppercase font-bold text-gray-400 hover:text-white px-2 py-0.5 border border-gray-600 rounded;
}

.header-arrow {
  @apply h-4 w-4 transform transition-transform duration-200;
}

.header-arrow--collapsed {
  @apply rotate-180;
}

.form-body {
  @apply p-4 pt-0 space-y-3;
}

.field-group {
  @apply space-y-1;
}

.field-label {
  @apply block text-xs font-medium text-gray-400;
}

.field-label-tiny {
  @apply block text-[10px] text-gray-500 font-medium ml-1;
}

.select-input {
  @apply w-full bg-gray-900 border border-gray-700 rounded px-3 py-2 text-sm text-white 
         focus:border-blue-500 focus:ring-1 focus:ring-blue-500 transition-colors disabled:opacity-50;
}

.select-input--nested {
  @apply bg-gray-800 border-gray-700 focus:border-blue-500/50 focus:ring-blue-500/20 font-medium;
}

.text-input {
  @apply w-full bg-gray-900 border border-gray-700 rounded px-3 py-2 text-sm text-white 
         focus:border-blue-500 focus:ring-1 focus:ring-blue-500 transition-colors;
}

.form-section-group {
  @apply space-y-3 transition-opacity duration-200;
}

.form-section-group--disabled {
  @apply opacity-50 pointer-events-none;
}

.nested-config-section {
  @apply space-y-3 p-3 bg-gray-900 shadow-inner rounded-lg border border-gray-700/60 ring-1 ring-white/5;
}

.section-header-label {
  @apply text-[11px] font-bold text-gray-400 uppercase tracking-wider;
}

.config-grid {
  @apply space-y-3;
}

.config-field {
  @apply space-y-1;
}

.trigger-config {
  @apply bg-gray-900/50 p-3 rounded-lg border border-gray-700/50;
}

.trigger-header {
  @apply flex items-center justify-between mb-3;
}

.trigger-type-select {
  @apply bg-gray-800 text-xs text-white px-2 py-1 rounded border border-gray-700 w-32;
}

.trigger-content {
  @apply space-y-3;
}

.trigger-type-select {
  @apply bg-gray-800 text-xs text-white px-2 py-1 rounded border border-gray-700 w-32;
}

.cron-simple-row {
  @apply flex items-center gap-2;
}

.cron-simple-label {
  @apply text-sm text-gray-400;
}

.cron-number-input {
  @apply w-20 bg-gray-900 text-sm text-white px-3 py-2 rounded border border-gray-700 text-center;
}

.cron-unit-select {
  @apply bg-gray-900 text-sm text-white px-3 py-2 rounded border border-gray-700 w-32;
}

.cron-input-group {
  @apply space-y-1;
}

.cron-preview {
  @apply mt-1 text-xs text-blue-400 min-h-[16px];
}

.manual-helper {
  @apply text-xs text-gray-500;
}

.form-footer-actions {
  @apply flex gap-3 mt-4;
}

.btn-cancel-wide {
  @apply flex-1 bg-gray-700 hover:bg-gray-600 text-white py-2 rounded font-medium transition-colors;
}

.btn-submit-wide {
  @apply flex-[2] bg-blue-600 hover:bg-blue-700 disabled:bg-gray-700 disabled:text-gray-500 
         disabled:cursor-not-allowed text-white py-2 rounded font-medium transition-colors;
}
</style>
