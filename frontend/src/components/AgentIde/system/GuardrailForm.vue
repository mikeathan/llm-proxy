<script setup lang="ts">
import type { AgentGuardrailsConfig } from "../../../types/admin";
import { useGuardrailEditor } from "../../../composables/useGuardrailEditor";
import GuardrailSection from "./GuardrailSection.vue";
import BaseToggle from "../../common/buttons/BaseToggle.vue";

const props = defineProps<{
  modelValue: AgentGuardrailsConfig;
  isWorkspaceOverride?: boolean;
}>();

const emit = defineEmits<{
  (e: "update:modelValue", config: AgentGuardrailsConfig): void;
}>();

const {
  local,
  terminalAllowedRaw,
  terminalBlockedRaw,
  terminalEnvVarsRaw,
  terminalPathExtensionsRaw,
  terminalExternalPathsRaw,
  fsAllowedPathsRaw,
  fsAllowedExtensionsRaw,
  fsBlockedFilenamesRaw,
  networkBlockedDomainsRaw,
  networkBlockedIPsRaw,
  globalBlockedRaw,
  setupRawWatchers,
} = useGuardrailEditor(props.modelValue);

setupRawWatchers((val) => emit("update:modelValue", val));
</script>

<template>
  <div class="guardrail-form">
    <div class="sections-grid">
      <!-- Global Guardrails -->
      <div class="config-card">
        <h3 class="card-title">Global Security</h3>
        <div class="form-group mb-4">
          <BaseToggle
            v-model="local.global.block_secrets"
            label="Block Secrets (PII Redaction)"
          />
        </div>
        <div class="form-group border-t border-gray-700/50 pt-4 mt-2">
          <label class="form-label">Global Blocked Patterns</label>
          <textarea
            v-model="globalBlockedRaw"
            class="form-input font-mono text-xs"
            rows="3"
          ></textarea>
        </div>
      </div>

      <!-- Terminal Guardrails -->
      <GuardrailSection
        :title="'Terminal Tool'"
        :enabled="!!local.terminal?.enabled"
        @toggle="local.terminal!.enabled = $event"
      >
        <div class="form-group">
          <label class="form-label">Allowed Commands</label>
          <textarea
            v-model="terminalAllowedRaw"
            class="form-input font-mono text-xs"
            rows="4"
          ></textarea>
        </div>
        <div class="form-group">
          <label class="form-label">Blocked Patterns</label>
          <textarea
            v-model="terminalBlockedRaw"
            class="form-input font-mono text-xs"
            rows="4"
          ></textarea>
        </div>
        <div class="form-group">
          <label class="form-label">Allowed Environment Variables</label>
          <textarea
            v-model="terminalEnvVarsRaw"
            class="form-input font-mono text-xs"
            rows="4"
            placeholder="PATH, LANG, etc..."
          ></textarea>
        </div>
        <div class="form-group">
          <label class="form-label">Workspace Path Extensions</label>
          <textarea
            v-model="terminalPathExtensionsRaw"
            class="form-input font-mono text-xs"
            rows="4"
            placeholder="node_modules/.bin, .venv/bin, etc..."
          ></textarea>
        </div>
        <div class="form-group">
          <label class="form-label flex items-center gap-2">
            Allowed External Paths
            <span
              v-if="local.terminal?.allowed_external_paths?.length"
              class="external-warning-badge"
            >⚠️ External Access</span>
          </label>
          <textarea
            v-model="terminalExternalPathsRaw"
            class="form-input font-mono text-xs"
            :class="{ 'form-input--hazard': local.terminal?.allowed_external_paths?.length }"
            rows="3"
            placeholder="/home/user/projects, /mnt/data, etc..."
          ></textarea>
        </div>
        <div
          v-if="local.terminal?.allowed_external_paths?.length"
          class="external-warning-banner"
        >
          <span class="external-warning-icon">⚠️</span>
          <div>
            <p class="external-warning-title">External File System Access Enabled</p>
            <p class="external-warning-text">
              This agent can access paths outside its workspace jail. This
              reduces security isolation and should only be used for trusted
              workloads.
            </p>
          </div>
        </div>
        <div class="grid grid-cols-2 gap-4">
          <div class="form-group">
            <label class="form-label">Timeout (sec)</label>
            <input
              type="number"
              v-model.number="local.terminal!.timeout_seconds"
              class="form-input"
            />
          </div>
          <div class="form-group">
            <label class="form-label">Max Output (chars)</label>
            <input
              type="number"
              v-model.number="local.terminal!.max_output_size_chars"
              class="form-input"
            />
          </div>
        </div>
        <div class="form-group border-t border-gray-700/50 pt-4 mt-2">
          <div class="flex items-center justify-between mb-2">
            <label class="form-label">Session Idle Timeout</label>
            <BaseToggle
              :modelValue="(local.terminal?.session_idle_timeout_seconds ?? 0) > 0"
              @update:modelValue="
                (v: boolean) =>
                  (local.terminal!.session_idle_timeout_seconds = v ? 1800 : 0)
              "
            />
          </div>
          <div
            v-if="(local.terminal?.session_idle_timeout_seconds ?? 0) > 0"
            class="flex items-center gap-2"
          >
            <input
              type="number"
              v-model.number="local.terminal!.session_idle_timeout_seconds"
              class="form-input flex-1"
              placeholder="Idle seconds..."
            />
            <span class="text-[10px] text-gray-500 uppercase">sec</span>
          </div>
          <p v-else class="text-[10px] text-gray-500 italic">
            Sessions remain active indefinitely.
          </p>
        </div>
      </GuardrailSection>

      <!-- FileSystem Guardrails -->
      <GuardrailSection
        :title="'FileSystem Tool'"
        :enabled="!!local.filesystem?.enabled"
        @toggle="local.filesystem!.enabled = $event"
      >
        <div class="form-group">
          <BaseToggle
            v-model="local.filesystem!.read_only"
            label="Read Only Access"
          />
        </div>
        <div class="form-group">
          <label class="form-label">Allowed Paths</label>
          <textarea
            v-model="fsAllowedPathsRaw"
            class="form-input font-mono text-xs"
            rows="2"
          ></textarea>
        </div>
        <div class="grid grid-cols-2 gap-4">
          <div class="form-group">
            <label class="form-label">Allowed Extensions</label>
            <textarea
              v-model="fsAllowedExtensionsRaw"
              class="form-input font-mono text-xs"
              rows="3"
            ></textarea>
          </div>
          <div class="form-group">
            <label class="form-label">Blocked Filenames</label>
            <textarea
              v-model="fsBlockedFilenamesRaw"
              class="form-input font-mono text-xs"
              rows="3"
            ></textarea>
          </div>
        </div>
        <div class="form-group">
          <label class="form-label">Max File Size (KB)</label>
          <input
            type="number"
            v-model.number="local.filesystem!.max_file_size_kb"
            class="form-input"
          />
        </div>
      </GuardrailSection>

      <!-- Network Guardrails -->
      <GuardrailSection
        :title="'Network Tool (Native)'"
        :enabled="!!local.network?.enabled"
        @toggle="local.network!.enabled = $event"
      >
        <div class="grid grid-cols-2 gap-4">
          <BaseToggle
            v-model="local.network!.allow_lan_access"
            label="LAN Access"
          />
          <BaseToggle
            v-model="local.network!.allow_internet_access"
            label="Internet"
          />
        </div>
        <div class="form-group">
          <label class="form-label">Blocked Domains</label>
          <textarea
            v-model="networkBlockedDomainsRaw"
            class="form-input font-mono text-xs"
            rows="2"
          ></textarea>
        </div>
        <div class="form-group">
          <label class="form-label">Blocked IPs</label>
          <textarea
            v-model="networkBlockedIPsRaw"
            class="form-input font-mono text-xs"
            rows="2"
          ></textarea>
        </div>
        <div class="grid grid-cols-2 gap-4">
          <div class="form-group">
            <label class="form-label">Size Limit (KB)</label>
            <input
              type="number"
              v-model.number="local.network!.max_fetch_size_kb"
              class="form-input"
            />
          </div>
          <div class="form-group">
            <label class="form-label">Timeout (sec)</label>
            <input
              type="number"
              v-model.number="local.network!.timeout_seconds"
              class="form-input"
            />
          </div>
        </div>
      </GuardrailSection>
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

.form-input--hazard {
  @apply border-amber-600/50 focus:ring-amber-500/30 focus:border-amber-500;
}

.external-warning-badge {
  @apply inline-flex items-center px-2 py-0.5 rounded-md text-[9px] font-black uppercase tracking-wider
         bg-amber-500/15 text-amber-400 border border-amber-500/30;
}

.external-warning-banner {
  @apply flex items-start gap-3 bg-amber-500/10 border border-amber-500/30 rounded-lg p-3;
}

.external-warning-icon {
  @apply text-lg shrink-0;
}

.external-warning-title {
  @apply text-[11px] font-bold text-amber-400 uppercase tracking-wider;
}

.external-warning-text {
  @apply text-[10px] text-amber-300/70 leading-relaxed mt-0.5;
}

.switch-row {
  @apply flex items-center gap-3 cursor-pointer select-none;
}

.switch-input {
  @apply w-4 h-4 rounded bg-gray-900 border-gray-700 text-blue-600;
}
</style>
