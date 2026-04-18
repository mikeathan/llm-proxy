<script setup lang="ts">
import { ref, watch, computed, toRaw } from "vue";
import { listToString, stringToList } from "../../utils/config";
import type { GlobalConfig, AgentGuardrailsConfig } from "../../types/admin";

const props = defineProps<{
  config: GlobalConfig;
}>();

const emit = defineEmits<{
  (e: "update:config", config: GlobalConfig): void;
  (e: "save"): void;
}>();

const localGuardrails = ref<AgentGuardrailsConfig>(
  structuredClone(toRaw(props.config.guardrails)),
);

watch(
  () => props.config.guardrails,
  (newVal) => {
    if (newVal) {
      const current = JSON.stringify(toRaw(localGuardrails.value));
      const incoming = JSON.stringify(toRaw(newVal));
      if (current !== incoming) {
        localGuardrails.value = structuredClone(toRaw(newVal));
      }
    }
  },
  { deep: true },
);

watch(
  localGuardrails,
  (newVal) => {
    // Only emit if data actually changed compared to the prop to avoid re-render loops.
    // We use toRaw for stable comparison.
    const current = JSON.stringify(toRaw(newVal));
    const incoming = JSON.stringify(toRaw(props.config.guardrails));
    if (current !== incoming) {
      emit("update:config", { ...props.config, guardrails: newVal });
    }
  },
  { deep: true },
);

// Local state for raw strings to allow free typing
const terminalAllowedRaw = ref(
  listToString(localGuardrails.value.terminal.allowed_commands, "\n"),
);
const terminalBlockedRaw = ref(
  listToString(localGuardrails.value.terminal.blocked_patterns, "\n"),
);
const fsAllowedPathsRaw = ref(
  listToString(localGuardrails.value.filesystem.allowed_paths, "\n"),
);
const searchBlockedSitesRaw = ref(
  listToString(localGuardrails.value.search.blocked_sites, "\n"),
);
const globalBlockedRaw = ref(
  listToString(localGuardrails.value.global.user_blocked_patterns, "\n"),
);

watch(terminalAllowedRaw, (val) => {
  localGuardrails.value.terminal.allowed_commands = stringToList(val, "\n");
});
watch(terminalBlockedRaw, (val) => {
  localGuardrails.value.terminal.blocked_patterns = stringToList(val, "\n");
});
watch(fsAllowedPathsRaw, (val) => {
  localGuardrails.value.filesystem.allowed_paths = stringToList(val, "\n");
});
watch(searchBlockedSitesRaw, (val) => {
  localGuardrails.value.search.blocked_sites = stringToList(val, "\n");
});
watch(globalBlockedRaw, (val) => {
  localGuardrails.value.global.user_blocked_patterns = stringToList(val, "\n");
});

watch(
  localGuardrails,
  (newVal) => {
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
  },
  { deep: true },
);

function handleSave() {
  emit("save");
}
</script>

<template>
  <div class="guardrails-container">
    <div class="header-row">
      <h2 class="settings-title">Agent Guardrails</h2>
      <button @click="handleSave" class="btn-save">Save All Guardrails</button>
    </div>

    <div class="sections-grid">
      <!-- Global Guardrails -->
      <div class="config-card">
        <h3 class="card-title">Global Security</h3>
        <div class="form-group mb-4">
          <label class="switch-row">
            <input
              type="checkbox"
              v-model="localGuardrails.global.block_secrets"
              class="switch-input"
            />
            <span class="switch-label">Block Secrets (PII Redaction)</span>
          </label>
        </div>
        <div class="form-group border-t border-gray-700/50 pt-4 mt-2">
          <label class="form-label">Global Blocked Patterns</label>
          <div class="form-helper">
            Additional regex patterns to block globally across all tools.
          </div>
          <textarea
            v-model="globalBlockedRaw"
            class="form-input font-mono text-xs"
            rows="5"
            placeholder="e.g. [0-9]{3}-[0-9]{2}-[0-9]{4}"
          ></textarea>
        </div>
      </div>

      <!-- Terminal Guardrails -->
      <div class="config-card">
        <div class="card-header">
          <h3 class="card-title">Terminal Tool</h3>
          <label class="switch-row p-0">
            <input
              type="checkbox"
              v-model="localGuardrails.terminal.enabled"
              class="switch-input"
            />
            <span
              class="text-xs uppercase font-bold tracking-widest text-gray-500"
              >Enabled</span
            >
          </label>
        </div>

        <div v-if="localGuardrails.terminal.enabled" class="card-body">
          <div class="form-group">
            <label class="form-label">Allowed Commands</label>
            <div class="form-helper">
              One base command per line (e.g. ls, nmap, ping)
            </div>
            <textarea
              v-model="terminalAllowedRaw"
              class="form-input font-mono text-xs"
              rows="5"
              placeholder="e.g. ls, pwd, cat"
            ></textarea>
          </div>
          <div class="form-group">
            <label class="form-label">Blocked Patterns</label>
            <div class="form-helper">
              Regex patterns to block (e.g. rm\s+, sudo)
            </div>
            <textarea
              v-model="terminalBlockedRaw"
              class="form-input font-mono text-xs"
              rows="5"
              placeholder="e.g. rm -rf, sudo"
            ></textarea>
          </div>
          <div class="grid grid-cols-2 gap-4">
            <div class="form-group">
              <label class="form-label">Timeout (sec)</label>
              <input
                type="number"
                v-model.number="localGuardrails.terminal.timeout_seconds"
                class="form-input"
              />
            </div>
            <div class="form-group">
              <label class="form-label">Max Output (chars)</label>
              <input
                type="number"
                v-model.number="localGuardrails.terminal.max_output_size_chars"
                class="form-input"
              />
            </div>
          </div>
        </div>
        <div v-else class="card-disabled">Terminal execution is disabled.</div>
      </div>

      <!-- FileSystem Guardrails -->
      <div class="config-card">
        <div class="card-header">
          <h3 class="card-title">FileSystem Tool</h3>
          <label class="switch-row p-0">
            <input
              type="checkbox"
              v-model="localGuardrails.filesystem.enabled"
              class="switch-input"
            />
            <span
              class="text-xs uppercase font-bold tracking-widest text-gray-500"
              >Enabled</span
            >
          </label>
        </div>

        <div v-if="localGuardrails.filesystem.enabled" class="card-body">
          <div class="form-group">
            <label class="switch-row">
              <input
                type="checkbox"
                v-model="localGuardrails.filesystem.read_only"
                class="switch-input"
              />
              <span class="switch-label">Read Only Access</span>
            </label>
          </div>
          <div class="form-group">
            <label class="form-label">Allowed Paths</label>
            <div class="form-helper">
              One path per line. Defaults to workspace root.
            </div>
            <textarea
              v-model="fsAllowedPathsRaw"
              class="form-input font-mono text-xs"
              rows="3"
            ></textarea>
          </div>
          <div class="form-group">
            <label class="form-label">Max File Size (KB)</label>
            <input
              type="number"
              v-model.number="localGuardrails.filesystem.max_file_size_kb"
              class="form-input"
            />
          </div>
        </div>
        <div v-else class="card-disabled">FileSystem access is disabled.</div>
      </div>

      <!-- Search & Communication Grid -->
      <div
        class="grid grid-cols-1 md:grid-cols-2 gap-6 col-span-1 lg:col-span-2"
      >
        <!-- Search -->
        <div class="config-card h-full">
          <div class="card-header">
            <h3 class="card-title">Internet Search</h3>
            <label class="switch-row p-0">
              <input
                type="checkbox"
                v-model="localGuardrails.search.enabled"
                class="switch-input"
              />
              <span
                class="text-xs uppercase font-bold tracking-widest text-gray-500"
                >Enabled</span
              >
            </label>
          </div>
          <div v-if="localGuardrails.search.enabled" class="card-body">
            <div class="form-group">
              <label class="form-label">Max Query Length</label>
              <input
                type="number"
                v-model.number="localGuardrails.search.max_query_len"
                class="form-input"
              />
            </div>
            <div class="form-group">
              <label class="form-label">Blocked Sites</label>
              <textarea
                v-model="searchBlockedSitesRaw"
                class="form-input font-mono text-xs"
                rows="4"
              ></textarea>
            </div>
          </div>
        </div>

        <!-- Communication -->
        <div class="config-card h-full">
          <div class="card-header">
            <h3 class="card-title">Communication</h3>
            <label class="switch-row p-0">
              <input
                type="checkbox"
                v-model="localGuardrails.communication.enabled"
                class="switch-input"
              />
              <span
                class="text-xs uppercase font-bold tracking-widest text-gray-500"
                >Enabled</span
              >
            </label>
          </div>
          <div v-if="localGuardrails.communication.enabled" class="card-body">
            <div class="form-group">
              <label class="switch-row">
                <input
                  type="checkbox"
                  v-model="localGuardrails.communication.require_review"
                  class="switch-input"
                />
                <span class="switch-label">Require Human Review</span>
              </label>
            </div>
            <div class="form-group">
              <label class="form-label">Max Messages Per Task</label>
              <input
                type="number"
                v-model.number="
                  localGuardrails.communication.max_messages_per_task
                "
                class="form-input"
              />
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped lang="postcss">
.guardrails-container {
  @apply space-y-6 animate-in slide-in-from-right-4 duration-300;
}

.header-row {
  @apply flex items-center justify-between mb-2;
}

.settings-title {
  @apply text-xl font-bold text-white;
}

.btn-save {
  @apply bg-blue-600 hover:bg-blue-500 text-white px-4 py-2 rounded-md font-bold text-sm transition-all shadow-lg active:scale-95;
}

.sections-grid {
  @apply grid grid-cols-1 lg:grid-cols-2 gap-6;
}

.config-card {
  @apply bg-gray-800 rounded-xl border border-gray-700/50 p-5 shadow-xl flex flex-col;
}

.card-header {
  @apply flex items-center justify-between mb-4 pb-3 border-b border-gray-700/50;
}

.card-title {
  @apply text-sm font-black text-blue-400 uppercase tracking-widest;
}

.card-body {
  @apply space-y-4;
}

.card-disabled {
  @apply flex-1 flex items-center justify-center text-xs text-gray-600 italic py-10;
}

.form-group {
  @apply space-y-1.5;
}

.form-label {
  @apply block text-[11px] font-bold text-gray-400 uppercase tracking-wider;
}

.form-input {
  @apply w-full bg-gray-900 border border-gray-700 rounded-md px-3 py-2 text-sm text-white transition-all 
         focus:ring-2 focus:ring-blue-600/30 focus:border-blue-600 outline-none;
}

.form-helper {
  @apply text-[10px] text-gray-500 mb-1 leading-tight;
}

/* Switches */
.switch-row {
  @apply flex items-center gap-3 cursor-pointer select-none;
}

.switch-input {
  @apply w-4 h-4 rounded bg-gray-900 border-gray-700 text-blue-600 
         focus:ring-offset-gray-800 transition-all;
}

.switch-label {
  @apply text-sm font-medium text-gray-200;
}
</style>
