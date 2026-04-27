<script setup lang="ts">
import { ref, watch, toRaw } from "vue";
import { listToString, stringToList } from "../../../utils/config";
import type { AgentGuardrailsConfig } from "../../../types/admin";
import BaseToggle from "../../common/BaseToggle.vue";

const props = defineProps<{
  modelValue: AgentGuardrailsConfig;
  isWorkspaceOverride?: boolean;
}>();

const emit = defineEmits<{
  (e: "update:modelValue", config: AgentGuardrailsConfig): void;
}>();

const ensureStructure = (cfg: any) => {
  if (!cfg) cfg = {};
  if (!cfg.global)
    cfg.global = { block_secrets: true, user_blocked_patterns: [] };
  if (!cfg.terminal) {
    cfg.terminal = {
      enabled: false,
      allowed_commands: [],
      allowed_env_vars: [],
      path_extensions: [],
      timeout_seconds: 30,
      session_idle_timeout_seconds: 1800,
      max_output_size_chars: 5000,
    };
  }
  if (!cfg.filesystem)
    cfg.filesystem = {
      enabled: false,
      allowed_paths: [],
      read_only: true,
      max_file_size_kb: 512,
    };
  if (!cfg.search)
    cfg.search = { enabled: false, max_query_len: 100, blocked_sites: [] };
  if (!cfg.communication)
    cfg.communication = {
      enabled: false,
      require_review: true,
      max_messages_per_task: 10,
    };
  if (!cfg.network)
    cfg.network = {
      enabled: false,
      allow_lan_access: false,
      allow_internet_access: false,
      max_fetch_size_kb: 512,
      timeout_seconds: 30,
    };
  return cfg;
};

const local = ref<AgentGuardrailsConfig>(
  ensureStructure(structuredClone(toRaw(props.modelValue))),
);

watch(
  () => props.modelValue,
  (newVal) => {
    const current = JSON.stringify(toRaw(local.value));
    const incoming = JSON.stringify(toRaw(newVal));
    if (current !== incoming) {
      local.value = ensureStructure(structuredClone(toRaw(newVal)));
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

// Raw string helpers for textareas with defensive null-checks
const terminalAllowedRaw = ref(
  listToString(local.value.terminal?.allowed_commands, "\n"),
);
const terminalBlockedRaw = ref(
  listToString(local.value.terminal?.blocked_patterns, "\n"),
);
const terminalEnvVarsRaw = ref(
  listToString(local.value.terminal?.allowed_env_vars, "\n"),
);
const terminalPathExtensionsRaw = ref(
  listToString(local.value.terminal?.path_extensions, "\n"),
);
const fsAllowedPathsRaw = ref(
  listToString(local.value.filesystem?.allowed_paths, "\n"),
);
const fsAllowedExtensionsRaw = ref(
  listToString(local.value.filesystem?.allowed_extensions, "\n"),
);
const fsBlockedFilenamesRaw = ref(
  listToString(local.value.filesystem?.blocked_filenames, "\n"),
);
const searchBlockedSitesRaw = ref(
  listToString(local.value.search?.blocked_sites, "\n"),
);
const networkBlockedDomainsRaw = ref(
  listToString(local.value.network?.blocked_domains, "\n"),
);
const networkBlockedIPsRaw = ref(
  listToString(local.value.network?.blocked_ips, "\n"),
);
const globalBlockedRaw = ref(
  listToString(local.value.global?.user_blocked_patterns, "\n"),
);

watch(terminalAllowedRaw, (val) => {
  if (!local.value.terminal)
    local.value.terminal = {
      enabled: false,
      allowed_commands: [],
      timeout_seconds: 30,
      session_idle_timeout_seconds: 1800,
      max_output_size_chars: 5000,
    };
  local.value.terminal.allowed_commands = stringToList(val, "\n");
});
watch(terminalBlockedRaw, (val) => {
  if (!local.value.terminal)
    local.value.terminal = {
      enabled: false,
      allowed_commands: [],
      timeout_seconds: 30,
      session_idle_timeout_seconds: 1800,
      max_output_size_chars: 5000,
    };
  local.value.terminal.blocked_patterns = stringToList(val, "\n");
});
watch(terminalEnvVarsRaw, (val) => {
  if (!local.value.terminal)
    local.value.terminal = {
      enabled: false,
      allowed_commands: [],
      allowed_env_vars: [],
      timeout_seconds: 30,
      session_idle_timeout_seconds: 1800,
      max_output_size_chars: 5000,
    };
  local.value.terminal.allowed_env_vars = stringToList(val, "\n");
});
watch(terminalPathExtensionsRaw, (val) => {
  if (!local.value.terminal)
    local.value.terminal = {
      enabled: false,
      allowed_commands: [],
      allowed_env_vars: [],
      path_extensions: [],
      timeout_seconds: 30,
      session_idle_timeout_seconds: 1800,
      max_output_size_chars: 5000,
    };
  local.value.terminal.path_extensions = stringToList(val, "\n");
});
watch(fsAllowedPathsRaw, (val) => {
  if (!local.value.filesystem)
    local.value.filesystem = {
      enabled: false,
      allowed_paths: [],
      read_only: true,
      max_file_size_kb: 512,
    };
  local.value.filesystem.allowed_paths = stringToList(val, "\n");
});
watch(fsAllowedExtensionsRaw, (val) => {
  if (!local.value.filesystem)
    local.value.filesystem = {
      enabled: false,
      allowed_paths: [],
      read_only: true,
      max_file_size_kb: 512,
    };
  local.value.filesystem.allowed_extensions = stringToList(val, "\n");
});
watch(fsBlockedFilenamesRaw, (val) => {
  if (!local.value.filesystem)
    local.value.filesystem = {
      enabled: false,
      allowed_paths: [],
      read_only: true,
      max_file_size_kb: 512,
    };
  local.value.filesystem.blocked_filenames = stringToList(val, "\n");
});
watch(searchBlockedSitesRaw, (val) => {
  if (!local.value.search)
    local.value.search = {
      enabled: false,
      max_query_len: 100,
      blocked_sites: [],
    };
  local.value.search.blocked_sites = stringToList(val, "\n");
});
watch(networkBlockedDomainsRaw, (val) => {
  if (!local.value.network)
    local.value.network = {
      enabled: false,
      allow_lan_access: false,
      allow_internet_access: false,
      max_fetch_size_kb: 512,
      timeout_seconds: 30,
    };
  local.value.network.blocked_domains = stringToList(val, "\n");
});
watch(networkBlockedIPsRaw, (val) => {
  if (!local.value.network)
    local.value.network = {
      enabled: false,
      allow_lan_access: false,
      allow_internet_access: false,
      max_fetch_size_kb: 512,
      timeout_seconds: 30,
    };
  local.value.network.blocked_ips = stringToList(val, "\n");
});
watch(globalBlockedRaw, (val) => {
  if (!local.value.global)
    local.value.global = { block_secrets: true, user_blocked_patterns: [] };
  local.value.global.user_blocked_patterns = stringToList(val, "\n");
});

watch(
  local,
  (newVal) => {
    const sync = (rawRef: any, list: string[] | undefined) => {
      const clean = listToString(list, "\n");
      if (listToString(stringToList(rawRef.value, "\n"), "\n") !== clean) {
        rawRef.value = clean;
      }
    };
    sync(terminalAllowedRaw, newVal.terminal?.allowed_commands);
    sync(terminalBlockedRaw, newVal.terminal?.blocked_patterns);
    sync(terminalEnvVarsRaw, newVal.terminal?.allowed_env_vars);
    sync(terminalPathExtensionsRaw, newVal.terminal?.path_extensions);
    sync(fsAllowedPathsRaw, newVal.filesystem?.allowed_paths);
    sync(fsAllowedExtensionsRaw, newVal.filesystem?.allowed_extensions);
    sync(fsBlockedFilenamesRaw, newVal.filesystem?.blocked_filenames);
    sync(searchBlockedSitesRaw, newVal.search?.blocked_sites);
    sync(networkBlockedDomainsRaw, newVal.network?.blocked_domains);
    sync(networkBlockedIPsRaw, newVal.network?.blocked_ips);
    sync(globalBlockedRaw, newVal.global?.user_blocked_patterns);
  },
  { deep: true },
);
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
      <div class="config-card">
        <div class="card-header">
          <h3 class="card-title">Terminal Tool</h3>
          <BaseToggle v-model="local.terminal.enabled" />
        </div>
        <div v-if="local.terminal.enabled" class="card-body">
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
          <div class="grid grid-cols-2 gap-4">
            <div class="form-group">
              <label class="form-label">Timeout (sec)</label>
              <input
                type="number"
                v-model.number="local.terminal.timeout_seconds"
                class="form-input"
              />
            </div>
            <div class="form-group">
              <label class="form-label">Max Output (chars)</label>
              <input
                type="number"
                v-model.number="local.terminal.max_output_size_chars"
                class="form-input"
              />
            </div>
          </div>
          <div class="form-group border-t border-gray-700/50 pt-4 mt-2">
            <div class="flex items-center justify-between mb-2">
              <label class="form-label">Session Idle Timeout</label>
              <BaseToggle
                :modelValue="local.terminal.session_idle_timeout_seconds > 0"
                @update:modelValue="
                  (v) =>
                    (local.terminal.session_idle_timeout_seconds = v ? 1800 : 0)
                "
              />
            </div>
            <div
              v-if="local.terminal.session_idle_timeout_seconds > 0"
              class="flex items-center gap-2"
            >
              <input
                type="number"
                v-model.number="local.terminal.session_idle_timeout_seconds"
                class="form-input flex-1"
                placeholder="Idle seconds..."
              />
              <span class="text-[10px] text-gray-500 uppercase">sec</span>
            </div>
            <p v-else class="text-[10px] text-gray-500 italic">
              Sessions remain active indefinitely.
            </p>
          </div>
        </div>
      </div>

      <!-- FileSystem Guardrails -->
      <div class="config-card">
        <div class="card-header">
          <h3 class="card-title">FileSystem Tool</h3>
          <BaseToggle v-model="local.filesystem.enabled" />
        </div>
        <div v-if="local.filesystem.enabled" class="card-body">
          <div class="form-group">
            <BaseToggle
              v-model="local.filesystem.read_only"
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
              v-model.number="local.filesystem.max_file_size_kb"
              class="form-input"
            />
          </div>
        </div>
      </div>

      <!-- Network Guardrails -->
      <div class="config-card">
        <div class="card-header">
          <h3 class="card-title">Network Tool (Native)</h3>
          <BaseToggle v-model="local.network.enabled" />
        </div>
        <div v-if="local.network.enabled" class="card-body">
          <div class="grid grid-cols-2 gap-4">
            <BaseToggle
              v-model="local.network.allow_lan_access"
              label="LAN Access"
            />
            <BaseToggle
              v-model="local.network.allow_internet_access"
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
                v-model.number="local.network.max_fetch_size_kb"
                class="form-input"
              />
            </div>
            <div class="form-group">
              <label class="form-label">Timeout (sec)</label>
              <input
                type="number"
                v-model.number="local.network.timeout_seconds"
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
</style>
