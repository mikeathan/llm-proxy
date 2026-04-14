<script setup lang="ts">
import { ref, watch, computed } from "vue";
import cronstrue from "cronstrue";
import type { Model } from "../../../types/model";
import type { ProviderItem } from "../../../types/admin";
import type { Automation } from "../../../types/dispatcher";

const props = defineProps<{
  workspaces: { id: string }[];
  workspaceFiles: Record<string, string[]>;
  models: Model[];
  providers: Record<string, ProviderItem>;
  hasAutomations: boolean;
  editAutomation?: Automation | null;
}>();

const emit = defineEmits<{
  (e: "create-automation", workspace: string, data: any): void;
  (e: "update-automation", workspace: string, oldName: string, data: any): void;
  (e: "fetch-files", workspace: string): void;
  (e: "cancel-edit"): void;
}>();

const isCollapsed = ref(props.hasAutomations && !props.editAutomation);

watch(
  () => props.hasAutomations,
  (val) => {
    if (!props.editAutomation) {
      isCollapsed.value = val;
    }
  },
);

const selectedWorkspace = ref("");

watch(selectedWorkspace, (newVal) => {
  if (newVal) {
    emit("fetch-files", newVal);
  }
});

const form = ref({
  name: "",
  triggerType: "cron",
  triggerValue: "",
  taskFile: "",
  strategy: "persistent",
  model: "",
});

// Model Selection Strategy state
const modelSource = ref<"local" | "cloud">("local");
const selectedProviderKey = ref(""); // "providerName/keyName"

const resetForm = () => {
  form.value = {
    name: "",
    triggerType: "cron",
    triggerValue: "",
    taskFile: "",
    strategy: "persistent",
    model: "",
  };
  selectedWorkspace.value = "";
  selectedProviderKey.value = "";
  modelSource.value = "local";
};

// Logic to derive model source and key from initial model name if editing
const syncModelSource = () => {
  if (props.editAutomation?.model) {
    const modelName = props.editAutomation.model;
    const modelObj = props.models.find((m) => m.name === modelName);
    if (modelObj) {
      let newKey = "";
      if (modelObj.provider === "local") {
        newKey = "local";
      } else {
        // For cloud models, reconstruct the key (empty string for default key)
        const keyName = modelObj.provider_config?.api_key_name || "";
        newKey = `${modelObj.provider}/${keyName}`;
      }
      if (selectedProviderKey.value !== newKey) {
        selectedProviderKey.value = newKey;
      }
    }
  }
};

watch(
  () => props.editAutomation,
  (newVal) => {
    if (newVal) {
      selectedWorkspace.value = newVal.workspace;
      // Sync provider key first so filteredModels is ready
      syncModelSource();
      // Set form values
      form.value = {
        name: newVal.name,
        triggerType: newVal.trigger || "cron",
        triggerValue: newVal.trigger_value || "",

        taskFile: newVal.task_file,
        strategy: newVal.strategy,
        model: newVal.model || "",
      };
      isCollapsed.value = false;
    } else {
      resetForm();
      // Collapse if we have other automations
      if (props.hasAutomations) {
        isCollapsed.value = true;
      }
    }
  },
  { immediate: true },
);

// Re-sync model source if models list was empty but now loaded
watch(
  () => props.models,
  () => {
    if (props.editAutomation) {
      syncModelSource();
    }
  },
  { deep: true },
);

const filteredModels = computed(() => {
  if (selectedProviderKey.value === "local") {
    return props.models.filter((m) => m.provider === "local");
  }

  if (!selectedProviderKey.value) return [];
  const parts = selectedProviderKey.value.split("/");
  const provider = parts[0];
  const keyName = parts[1] || ""; // Handle both "provider/key" and "provider/"
  
  return props.models.filter(
    (m) =>
      m.provider === provider && (m.provider_config?.api_key_name || "") === keyName,
  );
});

const cloudProvidersWithKeys = computed(() => {
  const result: {
    providerName: string;
    keys: { name: string; id: string; keyVal: string }[];
  }[] = [];
  
  for (const [name, p] of Object.entries(props.providers)) {
    if (name === "local") continue;
    
    const keys: { name: string; id: string; keyVal: string }[] = [];
    
    // Add named keys if they exist
    if (p.api_keys && p.api_keys.length > 0) {
      p.api_keys.forEach(k => {
        keys.push({ name: k.name, id: k.id, keyVal: k.name });
      });
    } else if (p.api_key) {
      // Only show Default if there's actually a key value in the legacy field
      keys.push({ name: "Default Provider Key", id: "default", keyVal: "" });
    }
    
    // Skip providers with no credentials at all
    if (keys.length === 0) continue;

    result.push({
      providerName: name,
      keys: keys.map(k => ({ name: k.name, id: k.id, keyVal: k.keyVal }))
    });
  }
  return result;
});

watch(selectedProviderKey, (_, oldVal) => {
  // Reset model selection when connection changes
  if (oldVal !== undefined) {
    // If we're editing and the current model is already valid for the new provider/key,
    // don't reset it. This prevents syncModelSource from overwriting the model
    // during initial load of the edit form.
    if (props.editAutomation && form.value.model) {
      const isStillValid = filteredModels.value.some(m => m.name === form.value.model);
      if (isStillValid) return;
    }

    if (filteredModels.value.length > 0 && filteredModels.value[0]) {
      form.value.model = filteredModels.value[0].name;
    } else {
      form.value.model = "";
    }
  }
});

watch(selectedWorkspace, () => {
  if (!props.editAutomation) {
    form.value.taskFile = "";
  }
});

const cronType = ref("custom");
const cronEvery = ref(1);
const cronUnit = ref("hours");

watch([cronType, cronEvery, cronUnit], () => {
  if (cronType.value === "custom") return;

  if (cronType.value === "every") {
    if (cronUnit.value === "minutes") {
      form.value.triggerValue = `*/${cronEvery.value} * * * *`;
    } else if (cronUnit.value === "hours") {
      form.value.triggerValue = `0 */${cronEvery.value} * * *`;
    } else if (cronUnit.value === "days") {
      form.value.triggerValue = `0 0 */${cronEvery.value} * *`;
    }
  }
});

watch(
  () => form.value.triggerType,
  (newVal, oldVal) => {
    if (oldVal !== undefined && !props.editAutomation) {
      form.value.triggerValue = "";
    }
    if (newVal === "cron") {
      cronType.value = "custom";
    }
  },
);

const cronDescription = ref("");
watch(
  () => form.value.triggerValue,
  (newVal) => {
    if (form.value.triggerType === "cron" && newVal) {
      try {
        cronDescription.value = cronstrue.toString(newVal);
      } catch {
        cronDescription.value = "Invalid cron expression";
      }
    } else {
      cronDescription.value = "";
    }
  },
);

const handleSubmit = () => {
  if (!selectedWorkspace.value || !form.value.name) return;

  const data = {
    name: form.value.name,
    trigger: {
      type: form.value.triggerType,
      value: form.value.triggerValue,
    },
    task_file: form.value.taskFile,
    strategy: form.value.strategy,
    model: form.value.model,
  };

  if (props.editAutomation) {
    emit(
      "update-automation",
      selectedWorkspace.value,
      props.editAutomation.name,
      data,
    );
  } else {
    emit("create-automation", selectedWorkspace.value, data);
    resetForm();
  }
};

const handleCancel = () => {
  if (props.editAutomation) {
    emit("cancel-edit");
  } else {
    isCollapsed.value = true;
  }
};
</script>

<template>
  <div class="form-container">
    <div
      class="form-header"
      @click="isCollapsed = !isCollapsed"
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
          <svg
            xmlns="http://www.w3.org/2000/svg"
            class="header-arrow"
            :class="{ 'header-arrow--collapsed': isCollapsed }"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M5 15l7-7 7 7"
            />
          </svg>
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

          <div v-if="form.triggerType === 'cron'" class="trigger-content">
            <div class="flex gap-2">
              <select
                v-model="cronType"
                class="select-input"
              >
                <option value="every">Simple Frequency</option>
                <option value="custom">Custom Expression</option>
              </select>
            </div>

            <div v-if="cronType === 'every'" class="cron-simple-row">
              <span class="cron-simple-label">Run every</span>
              <input
                type="number"
                v-model="cronEvery"
                min="1"
                class="cron-number-input"
              />
              <select
                v-model="cronUnit"
                class="cron-unit-select"
              >
                <option value="minutes">Minutes</option>
                <option value="hours">Hours</option>
                <option value="days">Days</option>
              </select>
            </div>

            <div class="cron-input-group">
              <input
                v-model="form.triggerValue"
                placeholder="* * * * *"
                :readonly="cronType !== 'custom'"
                class="text-input font-mono"
              />
              <div class="cron-preview">
                {{ cronDescription }}
              </div>
            </div>
          </div>

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
