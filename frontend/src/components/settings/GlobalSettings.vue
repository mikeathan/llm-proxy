<script setup lang="ts">
import { computed, ref, watch } from "vue";
import Panel from "../common/layout/Panel.vue";
import FormField from "../common/forms/FormField.vue";
import SegmentedControl from "../common/forms/SegmentedControl.vue";
import BaseToggle from "../common/buttons/BaseToggle.vue";
import {
  argsToString,
  stringToArgs,
  envMapToString,
  stringToEnvMap,
} from "../../utils/config";
import { LOG_LEVELS } from "../../constants/api";
import type { GlobalConfig, ProviderItem, SchedulerConfig } from "../../types/admin";
import type { Model } from "../../types/model";
import type { ChoiceOption } from "../../types/ui";

// Settings · Local engine: the global configuration as numbered panels. Every
// field writes a copy of the config through `update:editConfig` (the page owns
// it and its save); only the log level applies on the spot.

const props = defineProps<{
  editConfig: GlobalConfig;
  logLevel: string;
  models: Model[];
}>();

const emit = defineEmits<{
  (e: "update:editConfig", config: GlobalConfig): void;
  (e: "updateConfig"): void;
  (e: "updateLogLevel", level: string): void;
}>();

const GPU_PROVIDERS: ChoiceOption[] = [
  { value: "", label: "None — no GPU metrics" },
  { value: "auto", label: "Auto-detect (recommended)" },
  { value: "nvidia", label: "NVIDIA (nvidia-smi)" },
  { value: "rocm", label: "AMD ROCm (rocm-smi)" },
  { value: "macos", label: "macOS (Metal / Apple silicon)" },
  { value: "amdgpu_top", label: "AMD (amdgpu_top)" },
  { value: "sysfs", label: "Linux (direct sysfs)" },
];
// Providers that poll through a CLI tool whose path can be overridden.
const GPU_TOOL_PROVIDERS = ["nvidia", "rocm", "amdgpu_top"];
const GPU_NO_INDEX = "macos";
const GPU_SYSFS = "sysfs";
const LOG_LEVEL_OPTIONS: ChoiceOption[] = LOG_LEVELS.map((level) => ({ value: level, label: level }));

const replaceConfig = (patch: Partial<GlobalConfig>) => emit("update:editConfig", { ...props.editConfig, ...patch });

/** A top-level config field as a v-model target that writes a copy. */
const configField = <K extends keyof GlobalConfig>(key: K) =>
  computed<GlobalConfig[K]>({
    get: () => props.editConfig[key],
    set: (value) => replaceConfig({ [key]: value } as Partial<GlobalConfig>),
  });

const primaryModel = configField("primary_model");
const fallbackModel = configField("fallback_model");
const workspacesDir = configField("workspaces_dir");
const modelHost = configField("model_host");
const idleTimeout = configField("idle_timeout_seconds");
const gpuProvider = configField("gpu_provider");
const gpuIndex = configField("gpu_index");
const gpuBinary = configField("gpu_binary");
const gpuSysfsPath = configField("gpu_sysfs_path");

const EMPTY_LOCAL: ProviderItem = { type: "local", model_dir: "", llama_server_binary: "", default_args: [], environment: {} };
const localProvider = computed<ProviderItem>(() => props.editConfig.providers?.local ?? EMPTY_LOCAL);
const replaceLocal = (patch: Partial<ProviderItem>) =>
  replaceConfig({ providers: { ...props.editConfig.providers, local: { ...localProvider.value, ...patch } } });

/** A local-provider field as a v-model target that writes a copy. */
const localField = <K extends keyof ProviderItem>(key: K) =>
  computed<ProviderItem[K]>({
    get: () => localProvider.value[key],
    set: (value) => replaceLocal({ [key]: value } as Partial<ProviderItem>),
  });

const modelDir = localField("model_dir");
const llamaBinary = localField("llama_server_binary");

const defaultArgsStr = computed({
  get: () => argsToString(localProvider.value.default_args),
  set: (val: string) => replaceLocal({ default_args: stringToArgs(val) }),
});

// Environment is parsed only when the field loses focus, so typing a line
// is never reformatted under the cursor.
const environmentStr = ref(envMapToString(localProvider.value.environment));
watch(() => localProvider.value.environment, (env) => {
  const serialized = envMapToString(env);
  if (environmentStr.value !== serialized) environmentStr.value = serialized;
}, { deep: true });

function commitEnvironment() {
  const parsed = stringToEnvMap(environmentStr.value);
  if (envMapToString(parsed) !== envMapToString(localProvider.value.environment ?? {})) {
    replaceLocal({ environment: parsed });
  }
}

// The scheduler section is always written whole, seeded with the shipped
// defaults: the backend treats an absent field as its default, so a partial
// object must never be persisted.
const SCHEDULER_DEFAULTS: Required<SchedulerConfig> = {
  local_concurrency: 1,
  cloud_concurrency: 3,
  preempt_automations: true,
  inbound_wait_seconds: 60,
  inbound_wait_by_default: false,
  inbound_max_queued: 32,
  inbound_preempt: false,
};

/** A scheduler field as a v-model target; `normalise` cleans typed numbers. */
function schedulerField<K extends keyof SchedulerConfig>(
  key: K,
  normalise: (value: Required<SchedulerConfig>[K]) => Required<SchedulerConfig>[K] = (value) => value,
) {
  return computed<Required<SchedulerConfig>[K]>({
    // A present field is never undefined, so the fallback only fills gaps.
    get: () => (props.editConfig.scheduler?.[key] ?? SCHEDULER_DEFAULTS[key]) as Required<SchedulerConfig>[K],
    set: (value) => replaceConfig({ scheduler: { ...SCHEDULER_DEFAULTS, ...props.editConfig.scheduler, [key]: normalise(value) } }),
  });
}
const wholeAtLeast = (min: number, fallback: number) => (value: number) => Math.max(min, Math.floor(value) || fallback);

const localConcurrency = schedulerField("local_concurrency", wholeAtLeast(1, 1));
const cloudConcurrency = schedulerField("cloud_concurrency", wholeAtLeast(1, 1));
const preemptAutomations = schedulerField("preempt_automations");
// Inbound admission for external /v1 callers. Seconds: 0 refuses immediately,
// -1 waits indefinitely (still bounded by the queue depth). A caller that asks
// with X-Queue-Wait is capped at this; a caller that doesn't ask is only parked
// when the parking toggle below is on.
const inboundWaitSeconds = schedulerField("inbound_wait_seconds", wholeAtLeast(-1, 0));
const inboundWaitByDefault = schedulerField("inbound_wait_by_default");
const inboundMaxQueued = schedulerField("inbound_max_queued", wholeAtLeast(1, 1));
const inboundPreempt = schedulerField("inbound_preempt");

const runLoggingEnabled = computed({
  get: () => props.editConfig.run_logging?.enabled ?? false,
  set: (enabled: boolean) => replaceConfig({ run_logging: { ...props.editConfig.run_logging, enabled } }),
});

const showGpuIndex = computed(() => !!gpuProvider.value && gpuProvider.value !== GPU_NO_INDEX);
const showGpuBinary = computed(() => GPU_TOOL_PROVIDERS.includes(gpuProvider.value ?? ""));
const showSysfsPath = computed(() => gpuProvider.value === GPU_SYSFS);
</script>

<template>
  <form class="flex flex-col gap-4" @submit.prevent="$emit('updateConfig')">
    <Panel title="Routing">
      <div class="form-grid">
        <FormField label="Primary model" hint="Serves requests that name no model.">
          <template #default="{ id, describedBy }">
            <select :id="id" v-model="primaryModel" :aria-describedby="describedBy" class="form-control">
              <option value="">Auto — first available</option>
              <option v-for="m in models" :key="m.name" :value="m.name">{{ m.name }} ({{ m.provider }})</option>
            </select>
          </template>
        </FormField>
        <FormField label="Fallback model" hint="Takes over when the primary is offline or fails.">
          <template #default="{ id, describedBy }">
            <select :id="id" v-model="fallbackModel" :aria-describedby="describedBy" class="form-control">
              <option value="">None — no fallback</option>
              <option v-for="m in models" :key="m.name" :value="m.name">{{ m.name }} ({{ m.provider }})</option>
            </select>
          </template>
        </FormField>
      </div>
    </Panel>

    <Panel title="Local engine">
      <div class="form-grid">
        <FormField label="Model directory" hint="Scanned for .gguf files. Absolute, or relative to the data root.">
          <template #default="{ id, describedBy }">
            <input :id="id" v-model="modelDir" :aria-describedby="describedBy" type="text" class="form-control font-mono" placeholder="/path/to/models" autocomplete="off" />
          </template>
        </FormField>
        <FormField label="llama-server binary" hint="Path to the llama-server executable.">
          <template #default="{ id, describedBy }">
            <input :id="id" v-model="llamaBinary" :aria-describedby="describedBy" type="text" class="form-control font-mono" placeholder="/usr/local/bin/llama-server" autocomplete="off" />
          </template>
        </FormField>
        <FormField label="Model host IP" hint="The address local model servers bind to. Default 127.0.0.1.">
          <template #default="{ id, describedBy }">
            <input :id="id" v-model="modelHost" :aria-describedby="describedBy" type="text" class="form-control font-mono" autocomplete="off" />
          </template>
        </FormField>
      </div>
      <div class="form-grid mt-4">
        <FormField label="Default arguments" hint="Space-separated, passed to every local model.">
          <template #default="{ id, describedBy }">
            <textarea :id="id" v-model="defaultArgsStr" :aria-describedby="describedBy" rows="2" class="form-control font-mono" placeholder="--ctx-size 4096" spellcheck="false"></textarea>
          </template>
        </FormField>
        <FormField label="Environment variables" hint="One KEY=VALUE per line, set for every local model.">
          <template #default="{ id, describedBy }">
            <textarea
              :id="id"
              v-model="environmentStr"
              :aria-describedby="describedBy"
              rows="3"
              class="form-control font-mono"
              placeholder="HSA_OVERRIDE_GFX_VERSION=11.0.0&#10;AMD_SERIALIZE_KERNEL=1"
              spellcheck="false"
              @blur="commitEnvironment"
            ></textarea>
          </template>
        </FormField>
      </div>
    </Panel>

    <Panel title="Workspace storage">
      <div class="form-grid">
        <FormField
          label="Workspaces directory"
          hint="Empty: workspaces/ in the repository during development, otherwise in the data root (LLM_PROXY_HOME / --data). Relative paths resolve against the data root."
        >
          <template #default="{ id, describedBy }">
            <input :id="id" v-model="workspacesDir" :aria-describedby="describedBy" type="text" class="form-control font-mono" autocomplete="off" />
          </template>
        </FormField>
        <FormField label="Run logging" hint="Per-run logs, execution history and event streams inside each workspace.">
          <template #default="{ id, describedBy }">
            <BaseToggle :id="id" v-model="runLoggingEnabled" :described-by="describedBy" label="Record workspace run logs" />
          </template>
        </FormField>
      </div>
    </Panel>

    <Panel title="Model lifecycle">
      <div class="form-grid">
        <FormField label="Idle timeout (seconds)" hint="A local model with no requests for this long is stopped to free memory. -1 keeps it loaded.">
          <template #default="{ id, describedBy }">
            <input :id="id" v-model.number="idleTimeout" :aria-describedby="describedBy" type="number" min="-1" step="1" class="form-control font-mono tabular-nums" />
          </template>
        </FormField>
      </div>
    </Panel>

    <Panel title="GPU metrics">
      <div class="form-grid">
        <FormField label="GPU provider" hint="How utilisation and memory are read.">
          <template #default="{ id, describedBy }">
            <select :id="id" v-model="gpuProvider" :aria-describedby="describedBy" class="form-control">
              <option v-for="option in GPU_PROVIDERS" :key="option.value" :value="option.value">{{ option.label }}</option>
            </select>
          </template>
        </FormField>
        <FormField v-if="showGpuIndex" label="GPU index" hint="The device ID, usually 0.">
          <template #default="{ id, describedBy }">
            <input :id="id" v-model.number="gpuIndex" :aria-describedby="describedBy" type="number" min="0" class="form-control font-mono tabular-nums" placeholder="0" />
          </template>
        </FormField>
        <FormField v-if="showGpuBinary" label="Tool binary" hint="Optional. Overrides the path to the provider's tool.">
          <template #default="{ id, describedBy }">
            <input :id="id" v-model="gpuBinary" :aria-describedby="describedBy" type="text" class="form-control font-mono" placeholder="/opt/rocm/bin/rocm-smi" autocomplete="off" />
          </template>
        </FormField>
        <FormField v-if="showSysfsPath" label="Sysfs device path" hint="Optional. The GPU device under /sys.">
          <template #default="{ id, describedBy }">
            <input :id="id" v-model="gpuSysfsPath" :aria-describedby="describedBy" type="text" class="form-control font-mono" placeholder="/sys/class/drm/card0/device" autocomplete="off" />
          </template>
        </FormField>
      </div>
    </Panel>

    <Panel title="Run scheduler">
      <p class="mb-4 mt-0 max-w-[64ch] text-[length:var(--text-small)] text-muted">
        How many agent runs execute at once per workload class. Local runs share one GPU; cloud runs run in parallel.
      </p>
      <div class="form-grid">
        <FormField label="Local runs at once">
          <template #default="{ id }">
            <input :id="id" v-model.number="localConcurrency" type="number" min="1" step="1" class="form-control font-mono tabular-nums" />
          </template>
        </FormField>
        <FormField label="Cloud runs at once">
          <template #default="{ id }">
            <input :id="id" v-model.number="cloudConcurrency" type="number" min="1" step="1" class="form-control font-mono tabular-nums" />
          </template>
        </FormField>
        <FormField label="Chat priority" hint="A chat message pauses a running automation instead of waiting behind it.">
          <template #default="{ id, describedBy }">
            <BaseToggle :id="id" v-model="preemptAutomations" :described-by="describedBy" label="Chat preempts running automations" />
          </template>
        </FormField>
      </div>
    </Panel>

    <Panel title="External requests">
      <p class="mb-4 mt-0 max-w-[64ch] text-[length:var(--text-small)] text-muted">
        An external client (another proxy, a tool) asking for a different local model would otherwise stop the
        running one. It is held or told to retry instead — a run is only interrupted when you serve the request from
        the run indicator, where queued requests appear and can be dismissed.
      </p>
      <div class="form-grid">
        <FormField label="Wait up to (seconds)" hint="0 refuses at once; -1 waits with no limit.">
          <template #default="{ id, describedBy }">
            <input :id="id" v-model.number="inboundWaitSeconds" :aria-describedby="describedBy" type="number" min="-1" step="1" class="form-control font-mono tabular-nums" />
          </template>
        </FormField>
        <FormField label="Queue depth" hint="Requests held at once; more are refused.">
          <template #default="{ id, describedBy }">
            <input :id="id" v-model.number="inboundMaxQueued" :aria-describedby="describedBy" type="number" min="1" step="1" class="form-control font-mono tabular-nums" />
          </template>
        </FormField>
        <FormField label="Requests that don't ask to wait">
          <template #default="{ id }">
            <BaseToggle :id="id" v-model="inboundWaitByDefault" label="Queue them too" />
          </template>
        </FormField>
        <FormField label="Interrupting">
          <template #default="{ id }">
            <BaseToggle :id="id" v-model="inboundPreempt" label="A waiting request may interrupt the running job" />
          </template>
        </FormField>
      </div>
    </Panel>

    <Panel title="Logging">
      <FormField label="System log level" hint="Applies immediately — no save needed.">
        <template #default>
          <SegmentedControl :model-value="logLevel" :options="LOG_LEVEL_OPTIONS" label="System log level" class="w-fit" @update:model-value="$emit('updateLogLevel', $event)" />
        </template>
      </FormField>
    </Panel>
  </form>
</template>

<style scoped lang="postcss">
.form-grid {
  @apply grid grid-cols-[repeat(auto-fit,minmax(min(100%,240px),1fr))] gap-4;
}
</style>
