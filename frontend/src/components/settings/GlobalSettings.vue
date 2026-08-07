<script setup lang="ts">
import { computed, ref, watch } from "vue";
import LogLevelPanel from "./LogLevelPanel.vue";
import BaseButton from "../common/buttons/BaseButton.vue";
import InfoTooltip from "../common/display/InfoTooltip.vue";
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
  (e: "restartBackend"): void;
}>();

// Writable local copy of editConfig — emits on every change so parent
// stores the canonical value and this component stays reactive.
const cfg = computed({
  get: () => props.editConfig,
  set: (val) => emit("update:editConfig", val),
})

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
    const clone = JSON.parse(JSON.stringify(cfg.value));
    if (!clone.providers) clone.providers = {};
    clone.providers.local = val;
    cfg.value = clone;
  }
});

const defaultArgsStr = computed({
  get: () => argsToString(localProvider.value.default_args),
  set: (val: string) => {
    const provider = { ...localProvider.value, default_args: stringToArgs(val) };
    localProvider.value = provider;
  },
});

const environmentStr = ref(envMapToString(localProvider.value.environment));

watch(() => localProvider.value.environment, (env) => {
  const serialized = envMapToString(env);
  if (environmentStr.value !== serialized) {
    environmentStr.value = serialized;
  }
}, { deep: true });

function commitEnvironment() {
  const parsed = stringToEnvMap(environmentStr.value);
  const serialized = envMapToString(parsed);
  if (serialized !== envMapToString(localProvider.value.environment ?? {})) {
    const provider = { ...localProvider.value, environment: parsed };
    localProvider.value = provider;
  }
}

const runLoggingEnabled = computed({
  get: () => props.editConfig.run_logging?.enabled ?? false,
  set: (val: boolean) => {
    const clone = JSON.parse(JSON.stringify(props.editConfig));
    if (!clone.run_logging) {
      clone.run_logging = { enabled: false };
    }
    clone.run_logging.enabled = val;
    emit("update:editConfig", clone);
  }
});

function submitConfig() {
  emit("updateConfig");
}

function handleRestart() {
  if (window.confirm("Are you sure you want to restart the backend? This will terminate any active model sessions and automations.")) {
    emit("restartBackend");
  }
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
          v-model="cfg.primary_model"
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
          v-model="cfg.fallback_model"
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
          Where agent workspace files are stored. When empty, defaults to
          ./workspaces relative to the running binary (or the launch directory).
        </div>
        <input
          v-model="cfg.workspaces_dir"
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
          v-model="cfg.model_host"
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
          @blur="commitEnvironment"
        ></textarea>
      </div>

      <div class="form-section">
        <h3 class="section-title">GPU Status Configuration</h3>
        <div class="form-helper mb-4">
          Configure how the system retrieves GPU utilization and memory metrics.
        </div>
        
        <div class="form-grid">
          <div class="form-group">
            <label class="form-label">GPU Provider</label>
            <div class="form-helper">Method used to poll metrics</div>
            <select v-model="cfg.gpu_provider" class="form-input">
              <option value="">(None: Not setup)</option>
              <option value="auto">Auto-detect (Recommended)</option>
              <option value="nvidia">NVIDIA (nvidia-smi)</option>
              <option value="rocm">AMD ROCm (rocm-smi)</option>
              <option value="macos">macOS (Metal/Apple Silicon)</option>
              <option value="amdgpu_top">AMD (amdgpu_top)</option>
              <option value="sysfs">Linux (Direct Sysfs)</option>
            </select>
          </div>

          <div class="form-group" v-if="editConfig.gpu_provider && editConfig.gpu_provider !== 'macos'">
            <label class="form-label">GPU Index</label>
            <div class="form-helper">The device ID (usually 0)</div>
            <input
              v-model.number="editConfig.gpu_index"
              type="number"
              class="form-input"
              placeholder="0"
            />
          </div>
        </div>

        <div class="form-group mt-4" v-if="['nvidia', 'rocm', 'amdgpu_top'].includes(editConfig.gpu_provider || '')">
          <label class="form-label">Custom Tool Binary</label>
          <div class="form-helper">Override path to tool binary (Optional)</div>
          <input
            v-model="cfg.gpu_binary"
            type="text"
            class="form-input"
            placeholder="e.g. /opt/rocm/bin/rocm-smi"
          />
        </div>

        <div class="form-group mt-4" v-if="editConfig.gpu_provider === 'sysfs'">
          <label class="form-label">Sysfs Device Path</label>
          <div class="form-helper">Path to the GPU device in /sys (Optional)</div>
          <input
            v-model="cfg.gpu_sysfs_path"
            type="text"
            class="form-input"
            placeholder="/sys/class/drm/card0/device"
          />
        </div>

        <div class="form-group mt-4">
          <label class="form-label">GPU Sample Interval (seconds)
            <InfoTooltip text="How often the backend polls the GPU for utilization. Higher = less CPU but older readings; lower = fresher but more frequent sampling. Requires a backend restart to take effect." />
          </label>
          <div class="form-helper">Background polling period for GPU metrics</div>
          <input
            v-model.number="cfg.gpu_sample_interval_seconds"
            type="number"
            class="form-input"
            placeholder="10"
            min="1" max="300" step="1"
          />
        </div>

        <div class="form-group mt-4">
          <label class="form-label">GPU Smoothing (0–1)
            <InfoTooltip text="Exponential-moving-average factor for the displayed GPU utilization. Lower = calmer, flatter gauge that dampens spikes more; higher = tracks raw changes (1.0 = off). Applies immediately without a restart." />
          </label>
          <div class="form-helper">EMA smoothing factor for the GPU utilization gauge</div>
          <input
            v-model.number="cfg.gpu_smoothing_alpha"
            type="number"
            class="form-input"
            placeholder="0.3"
            min="0.05" max="1" step="0.05"
          />
        </div>
      </div>

      <div class="form-section">
        <h3 class="section-title">Logging Settings</h3>
        <div class="form-helper mb-4">
          Configure system and execution logging.
        </div>

        <div class="space-y-4">
          <div class="form-group">
            <label class="form-label">System Log Level</label>
            <div class="form-helper">
              Change the verbosity of proxy logging in the terminal
            </div>
            <LogLevelPanel
              :modelValue="logLevel"
              @update="$emit('updateLogLevel', $event)"
            />
          </div>

          <div class="form-group">
            <label class="form-label">Workspace Run Logging</label>
            <div class="form-helper">
              Enable per-run logs, execution history, and event streams inside workspace directories
            </div>
            <label class="flex items-center gap-2 cursor-pointer mt-2 w-fit">
              <input
                type="checkbox"
                v-model="runLoggingEnabled"
                class="rounded border-gray-600 bg-gray-700 text-blue-600 focus:ring-blue-600 w-4 h-4"
              />
              <span class="text-sm text-gray-300">Enable Run Logging</span>
            </label>
          </div>
        </div>
      </div>

      <div class="form-actions gap-3">
        <BaseButton 
          type="button" 
          variant="secondary" 
          icon="power"
          @click="handleRestart"
        >
          Restart Backend
        </BaseButton>
        <BaseButton type="submit" variant="primary" icon="play">
          Save Local Engine Settings
        </BaseButton>
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
.section-title {
  @apply text-base font-bold text-gray-300 mb-2;
}
.settings-form {
  @apply space-y-4;
}
.form-grid {
  @apply grid grid-cols-1 md:grid-cols-2 gap-4;
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
</style>
