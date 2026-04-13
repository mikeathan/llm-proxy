<script setup lang="ts">
import { computed } from "vue";
import LogLevelPanel from "./LogLevelPanel.vue";
import {
  argsToString,
  stringToArgs,
  envMapToString,
  stringToEnvMap,
} from "../../utils/config";
import type { GlobalConfig } from "../../types/admin";

const props = defineProps<{
  editConfig: GlobalConfig;
  logLevel: string;
  models: any[];
}>();

const emit = defineEmits<{
  (e: "update:editConfig", config: GlobalConfig): void;
  (e: "updateConfig"): void;
  (e: "updateLogLevel", level: string): void;
}>();

// Dynamic access to local provider. 
// Note: We use a computed for read/write so it stays in sync with the prop
const localProvider = computed({
  get: () => {
    if (!props.editConfig.providers?.local) {
      return {
        type: "local" as const,
        model_dir: "",
        llama_server_binary: "",
        default_args: [],
        environment: {},
      };
    }
    return props.editConfig.providers.local;
  },
  set: (val) => {
    const clone = JSON.parse(JSON.stringify(props.editConfig));
    if (!clone.providers) clone.providers = {};
    clone.providers.local = val;
    emit("update:editConfig", clone);
  }
});

const defaultArgsStr = computed({
  get: () => argsToString(localProvider.value.default_args),
  set: (val: string) => {
    const provider = { ...localProvider.value, default_args: stringToArgs(val) };
    localProvider.value = provider;
  },
});

const environmentStr = computed({
  get: () => envMapToString(localProvider.value.environment),
  set: (val: string) => {
    const provider = { ...localProvider.value, environment: stringToEnvMap(val) };
    localProvider.value = provider;
  },
});

function submitConfig() {
  emit("updateConfig");
}
</script>
<template>
  <div class="settings-container">
    <h2 class="settings-title">Local Engine Configuration</h2>

    <form @submit.prevent="submitConfig" class="settings-form">
      <div class="form-group">
        <label class="form-label">Primary System Model</label>
        <div class="form-helper">
          The default model to use for the proxy and general requests if not specified.
        </div>
        <select
          v-model="editConfig.primary_model"
          class="form-input"
        >
          <option value="">(Auto: First available)</option>
          <option v-for="m in models" :key="m.name" :value="m.name">
            {{ m.name }} ({{ m.provider }})
          </option>
        </select>
      </div>

      <div class="form-group">
        <label class="form-label">Fallback System Model</label>
        <div class="form-helper">
          The fallback model to use if the primary model goes offline or throws an error.
        </div>
        <select
          v-model="editConfig.fallback_model"
          class="form-input"
        >
          <option value="">(None: No fallback)</option>
          <option v-for="m in models" :key="m.name" :value="m.name">
            {{ m.name }} ({{ m.provider }})
          </option>
        </select>
      </div>

      <div class="form-group">
        <label class="form-label">Model Directory</label>
        <div class="form-helper">
          Absolute or relative path to scan for .gguf files
        </div>
        <input
          v-model="localProvider.model_dir"
          type="text"
          class="form-input"
          placeholder="/path/to/models"
        />
      </div>

      <div class="form-group">
        <label class="form-label">Workspaces Directory</label>
        <div class="form-helper">
          Path for agent workspaces (automations, state). Defaults to
          &lt;repo&gt;/workspaces
        </div>
        <input
          v-model="editConfig.workspaces_dir"
          type="text"
          class="form-input"
        />
      </div>

      <div class="form-group">
        <label class="form-label">Llama Server Binary</label>
        <div class="form-helper">Path to llama-server executable</div>
        <input
          v-model="localProvider.llama_server_binary"
          type="text"
          class="form-input"
          placeholder="/usr/local/bin/llama-server"
        />
      </div>

      <div class="form-group">
        <label class="form-label">Model Host IP</label>
        <div class="form-helper">
          IP address the underlying server binds to (default: 127.0.0.1)
        </div>
        <input
          v-model="editConfig.model_host"
          type="text"
          class="form-input"
        />
      </div>

      <div class="form-group">
        <label class="form-label">Default Arguments</label>
        <div class="form-helper">
          Space-separated arguments passed to all local models
        </div>
        <textarea
          v-model="defaultArgsStr"
          class="form-input form-input--mono"
          rows="2"
          placeholder="--ctx-size 4096"
        ></textarea>
      </div>

      <div class="form-group">
        <label class="form-label">Environment Variables</label>
        <div class="form-helper">
          Line-separated KEY=VALUE pairs injected into the environment
        </div>
        <textarea
          v-model="environmentStr"
          class="form-input form-input--mono"
          rows="3"
          placeholder="HSA_OVERRIDE_GFX_VERSION=11.0.0&#10;AMD_SERIALIZE_KERNEL=1"
        ></textarea>
      </div>

      <div class="form-section">
        <label class="form-label">System Log Level</label>
        <div class="form-helper mb-custom">
          Change the verbosity of proxy logging in the terminal
        </div>
        <LogLevelPanel
          :modelValue="logLevel"
          @update="$emit('updateLogLevel', $event)"
        />
      </div>

      <div class="form-actions">
        <button type="submit" class="btn-submit">
          Save Local Engine Settings
        </button>
      </div>
    </form>
  </div>
</template>

<style scoped lang="postcss">
.settings-container {
  @apply bg-gray-800 rounded-lg shadow-xl border border-gray-700 p-6 space-y-4 animate-in slide-in-from-right-4 duration-300;
}
.settings-title {
  @apply text-xl font-bold text-white mb-6 border-b border-gray-700 pb-3;
}
.settings-form {
  @apply space-y-4;
}
.form-group {
  @apply space-y-1.5;
}
.form-label {
  @apply block text-sm font-semibold text-gray-200;
}
.form-helper {
  @apply text-xs text-gray-500 mb-2;
}
.mb-custom {
  @apply mb-2;
}
.form-input {
  @apply w-full bg-gray-900 border border-gray-700 rounded-md px-3 py-2.5 text-white transition-all 
         focus:ring-2 focus:ring-blue-600/30 focus:border-blue-600 outline-none;
}
.form-input--mono {
  @apply font-mono text-xs;
}
.form-section {
  @apply pt-4 border-t border-gray-700 mt-4;
}
.form-actions {
  @apply pt-6 border-t border-gray-700 flex justify-end;
}
.btn-submit {
  @apply bg-blue-600 hover:bg-blue-500 text-white px-6 py-2.5 rounded-md font-bold transition-all shadow-lg 
         hover:shadow-blue-600/20 active:scale-95;
}
</style>
