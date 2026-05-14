<script setup lang="ts">
import { ref, onMounted, watch, computed } from "vue";
import { DispatcherService } from "../../../services/dispatcherService";
import GuardrailForm from "../system/GuardrailForm.vue";
import { useToast } from "../../../composables/useToast";
import type { AgentGuardrailsConfig } from "../../../types/admin";

const props = defineProps<{
  workspaceId: string;
  globalGuardrails: AgentGuardrailsConfig;
}>();

const hasExternalAccess = computed(() => {
  const paths = config.value?.guardrails?.terminal?.allowed_external_paths;
  return Array.isArray(paths) && paths.length > 0;
});

const emit = defineEmits<{
  (e: "close"): void;
}>();

const toast = useToast();
const config = ref<any>(null);
const loading = ref(true);
const saving = ref(false);

const loadConfig = async () => {
  loading.value = true;
  try {
    config.value = await DispatcherService.getWorkspaceConfig(
      props.workspaceId,
    );
    // Initialize guardrails if missing
    if (!config.value.guardrails) {
      config.value.guardrails = JSON.parse(
        JSON.stringify(props.globalGuardrails),
      );
    }
  } catch (err) {
    console.error("Failed to load workspace config", err);
    toast.error("Failed to load workspace configuration");
  } finally {
    loading.value = false;
  }
};

const handleSave = async () => {
  if (!config.value) return;
  saving.value = true;
  try {
    await DispatcherService.updateWorkspaceConfig(
      props.workspaceId,
      config.value,
    );
    toast.success("Workspace security updated successfully");
    // We don't close, just signal success
  } catch (err) {
    toast.error("Failed to save security settings");
  } finally {
    saving.value = false;
  }
};

const handleReset = () => {
  if (!config.value) return;
  // Populate UI with a fresh copy of global guardrails
  config.value.guardrails = JSON.parse(JSON.stringify(props.globalGuardrails));
  toast.info("Security form reset to system baseline. Click 'Save' to apply.");
};

onMounted(loadConfig);
watch(() => props.workspaceId, loadConfig);
</script>

<template>
  <div class="ws-settings-shell">
    <div class="settings-header">
      <div class="title-group">
        <span class="shield-icon">🛡️</span>
        <div>
          <h2 class="settings-title">Security Guardrails</h2>
          <p class="settings-subtitle">
            Workspace: <code class="ws-id">{{ workspaceId }}</code>
          </p>
        </div>
      </div>

      <div class="actions">
        <button @click="handleReset" class="btn-secondary" title="Reset to global defaults">
          Reset to Baseline
        </button>
        <button @click="handleSave" :disabled="saving" class="btn-primary">
          {{ saving ? "Saving..." : "Save Overrides" }}
        </button>
        <button @click="emit('close')" class="btn-ghost">Close</button>
      </div>
    </div>

    <div v-if="loading" class="loading-state">
      <div class="spinner"></div>
      <span>Loading workspace policy...</span>
    </div>

    <div v-else-if="config" class="settings-scroll-area">
      <div class="info-banner">
        <span class="info-icon">ℹ️</span>
        <p>
          Changes here override the global security policy for
          <strong>{{ workspaceId }}</strong> only. If a field is left empty, the
          system-wide baseline is typically inherited.
        </p>
      </div>

      <div
        v-if="hasExternalAccess"
        class="external-access-alert"
      >
        <span class="alert-icon">⚠️</span>
        <div>
          <p class="alert-title">External File System Access Enabled</p>
          <p class="alert-body">
            This workspace can access paths outside its jail:
            <code class="alert-code">{{ config.guardrails.terminal.allowed_external_paths.join(', ') }}</code>
          </p>
          <p class="alert-footer">
            Reduce scope when the task no longer requires external access.
          </p>
        </div>
      </div>

      <GuardrailForm v-model="config.guardrails" :isWorkspaceOverride="true" />
    </div>
  </div>
</template>

<style scoped lang="postcss">
.ws-settings-shell {
  @apply flex flex-col h-full bg-gray-900/40 animate-in fade-in duration-300;
}

.settings-header {
  @apply px-6 py-5 border-b border-gray-700/50 bg-gray-800/20 flex items-center justify-between shrink-0;
}

.title-group {
  @apply flex items-center gap-4;
}

.shield-icon {
  @apply text-3xl;
}

.settings-title {
  @apply text-lg font-black text-white leading-tight;
}

.settings-subtitle {
  @apply text-[11px] text-gray-500 font-medium uppercase tracking-wider;
}

.ws-id {
  @apply text-blue-400 font-mono normal-case;
}

.actions {
  @apply flex items-center gap-3;
}

.btn-primary {
  @apply bg-blue-600 hover:bg-blue-500 text-white px-5 py-2 rounded-md font-bold text-xs shadow-lg 
         active:scale-95 transition-all disabled:opacity-50 disabled:cursor-not-allowed;
}

.btn-secondary {
  @apply bg-gray-700 hover:bg-gray-600 text-gray-200 px-4 py-2 rounded-md font-bold text-xs 
         active:scale-95 transition-all mr-2;
}

.btn-ghost {
  @apply text-gray-400 hover:text-white px-3 py-2 text-xs font-medium transition-colors;
}

.loading-state {
  @apply flex-1 flex flex-col items-center justify-center gap-4 text-gray-500;
}

.spinner {
  @apply w-6 h-6 border-2 border-blue-500 border-t-transparent rounded-full animate-spin;
}

.settings-scroll-area {
  @apply flex-1 overflow-y-auto p-6 space-y-6;
}

.info-banner {
  @apply bg-blue-900/20 border border-blue-800/30 rounded-lg p-4 flex items-start gap-3 
         text-blue-200/80 text-xs leading-relaxed;
}

.info-icon {
  @apply text-base grayscale;
}

.external-access-alert {
  @apply flex items-start gap-3 bg-amber-500/10 border border-amber-500/40 rounded-lg p-4;
}

.alert-icon {
  @apply text-xl shrink-0;
}

.alert-title {
  @apply text-xs font-black text-amber-400 uppercase tracking-widest;
}

.alert-body {
  @apply text-[11px] text-amber-200/80 leading-relaxed mt-1;
}

.alert-code {
  @apply bg-amber-500/10 border border-amber-500/20 rounded px-1.5 py-0.5 text-[10px] text-amber-300 font-mono;
}

.alert-footer {
  @apply text-[10px] text-amber-400/50 italic mt-2;
}
</style>
