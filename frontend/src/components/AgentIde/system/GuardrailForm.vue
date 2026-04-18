<script setup lang="ts">
import { ref, watch, toRaw } from "vue";
import { listToString, stringToList } from "../../../utils/config";
import type { AgentGuardrailsConfig } from "../../../types/admin";

const props = defineProps<{
  modelValue: AgentGuardrailsConfig;
  isWorkspaceOverride?: boolean;
}>();

const emit = defineEmits<{
  (e: "update:modelValue", config: AgentGuardrailsConfig): void;
}>();

const local = ref<AgentGuardrailsConfig>(structuredClone(toRaw(props.modelValue)));

watch(
  () => props.modelValue,
  (newVal) => {
    const current = JSON.stringify(toRaw(local.value));
    const incoming = JSON.stringify(toRaw(newVal));
    if (current !== incoming) {
      local.value = structuredClone(toRaw(newVal));
    }
  },
  { deep: true },
);

watch(
  local,
  (newVal) => {
    emit("update:modelValue", structuredClone(toRaw(newVal)));
  },
  { deep: true },
);

// Raw string helpers for textareas
const terminalAllowedRaw = ref(listToString(local.value.terminal.allowed_commands, "\n"));
const terminalBlockedRaw = ref(listToString(local.value.terminal.blocked_patterns, "\n"));
const fsAllowedPathsRaw = ref(listToString(local.value.filesystem.allowed_paths, "\n"));
const searchBlockedSitesRaw = ref(listToString(local.value.search.blocked_sites, "\n"));
const globalBlockedRaw = ref(listToString(local.value.global.user_blocked_patterns, "\n"));

watch(terminalAllowedRaw, (val) => { local.value.terminal.allowed_commands = stringToList(val, "\n"); });
watch(terminalBlockedRaw, (val) => { local.value.terminal.blocked_patterns = stringToList(val, "\n"); });
watch(fsAllowedPathsRaw, (val) => { local.value.filesystem.allowed_paths = stringToList(val, "\n"); });
watch(searchBlockedSitesRaw, (val) => { local.value.search.blocked_sites = stringToList(val, "\n"); });
watch(globalBlockedRaw, (val) => { local.value.global.user_blocked_patterns = stringToList(val, "\n"); });

watch(local, (newVal) => {
  const sync = (rawRef: any, list: string[] | undefined) => {
    const clean = listToString(list, "\n");
    if (listToString(stringToList(rawRef.value, "\n"), "\n") !== clean) {
       rawRef.value = clean;
    }
  };
  sync(terminalAllowedRaw, newVal.terminal.allowed_commands);
  sync(terminalBlockedRaw, newVal.terminal.blocked_patterns);
  sync(fsAllowedPathsRaw, newVal.filesystem.allowed_paths);
  sync(searchBlockedSitesRaw, newVal.search.blocked_sites);
  sync(globalBlockedRaw, newVal.global.user_blocked_patterns);
}, { deep: true });
</script>

<template>
  <div class="guardrail-form">
    <div class="sections-grid">
      <!-- Global Guardrails -->
      <div class="config-card">
        <h3 class="card-title">Global Security</h3>
        <div class="form-group mb-4">
          <label class="switch-row">
            <input type="checkbox" v-model="local.global.block_secrets" class="switch-input" />
            <span class="switch-label">Block Secrets (PII Redaction)</span>
          </label>
        </div>
        <div class="form-group border-t border-gray-700/50 pt-4 mt-2">
          <label class="form-label">Global Blocked Patterns</label>
          <textarea v-model="globalBlockedRaw" class="form-input font-mono text-xs" rows="3"></textarea>
        </div>
      </div>

      <!-- Terminal Guardrails -->
      <div class="config-card">
        <div class="card-header">
          <h3 class="card-title">Terminal Tool</h3>
          <label class="switch-row p-0">
            <input type="checkbox" v-model="local.terminal.enabled" class="switch-input" />
            <span class="text-xs uppercase font-bold tracking-widest text-gray-500">Enabled</span>
          </label>
        </div>
        <div v-if="local.terminal.enabled" class="card-body">
          <div class="form-group">
            <label class="form-label">Allowed Commands</label>
            <textarea v-model="terminalAllowedRaw" class="form-input font-mono text-xs" rows="4"></textarea>
          </div>
          <div class="form-group">
            <label class="form-label">Blocked Patterns</label>
            <textarea v-model="terminalBlockedRaw" class="form-input font-mono text-xs" rows="4"></textarea>
          </div>
          <div class="grid grid-cols-2 gap-4">
            <div class="form-group">
              <label class="form-label">Timeout (sec)</label>
              <input type="number" v-model.number="local.terminal.timeout_seconds" class="form-input" />
            </div>
          </div>
        </div>
      </div>

      <!-- FileSystem Guardrails -->
      <div class="config-card">
        <div class="card-header">
          <h3 class="card-title">FileSystem Tool</h3>
          <label class="switch-row p-0">
            <input type="checkbox" v-model="local.filesystem.enabled" class="switch-input" />
            <span class="text-xs uppercase font-bold tracking-widest text-gray-500">Enabled</span>
          </label>
        </div>
        <div v-if="local.filesystem.enabled" class="card-body">
          <div class="form-group">
            <label class="switch-row">
              <input type="checkbox" v-model="local.filesystem.read_only" class="switch-input" />
              <span class="switch-label">Read Only Access</span>
            </label>
          </div>
          <div class="form-group">
            <label class="form-label">Allowed Paths</label>
            <textarea v-model="fsAllowedPathsRaw" class="form-input font-mono text-xs" rows="3"></textarea>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped lang="postcss">
.guardrail-form {
  @apply space-y-6 container mx-auto;
}

.sections-grid {
  @apply grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6;
}

.config-card {
  @apply bg-gray-800/50 rounded-xl border border-gray-700/50 p-5 shadow-xl flex flex-col;
}

.card-header {
  @apply flex items-center justify-between mb-4 pb-3 border-b border-gray-700/50;
}

.card-title {
  @apply text-[10px] font-black text-blue-400 uppercase tracking-widest;
}

.card-body {
  @apply space-y-4;
}

.form-group {
  @apply space-y-1.5;
}

.form-label {
  @apply block text-[10px] font-bold text-gray-500 uppercase tracking-wider;
}

.form-input {
  @apply w-full bg-black/40 border border-gray-700 rounded px-3 py-2 text-xs text-white transition-all 
         focus:ring-2 focus:ring-blue-600/30 focus:border-blue-600 outline-none;
}

.switch-row {
  @apply flex items-center gap-3 cursor-pointer select-none;
}

.switch-input {
  @apply w-4 h-4 rounded bg-gray-900 border-gray-700 text-blue-600;
}

.switch-label {
  @apply text-xs font-medium text-gray-200;
}
</style>
