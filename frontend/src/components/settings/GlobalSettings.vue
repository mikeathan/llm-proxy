<script setup lang="ts">
import { computed, ref, watch } from "vue";
import Panel from "../common/layout/Panel.vue";
import FormField from "../common/forms/FormField.vue";
import SegmentedControl from "../common/forms/SegmentedControl.vue";
import BaseToggle from "../common/buttons/BaseToggle.vue";
import BaseButton from "../common/buttons/BaseButton.vue";
import ToggleField from "../common/forms/ToggleField.vue";
import {
  argsToString,
  stringToArgs,
  envMapToString,
  stringToEnvMap,
} from "../../utils/config";
import { LOG_LEVELS } from "../../constants/api";
import type { GlobalConfig, MemoryHotDefaults, ProviderItem, SchedulerConfig, SettingsTab } from "../../types/admin";
import type { GeneralPanel } from "../../types/admin";
import { GENERAL_PANEL_SECTION, SUBAGENT_DEFAULT_CONCURRENCY, SUBAGENT_MAX_CONCURRENCY } from "../../constants/settings";
import { HELPER_MODEL_UNAVAILABLE_SUFFIX } from "../../constants/labels";
import { SHIPPED_MEMORY_DEFAULTS } from "../../constants/memory";
import { DELIVER_TO_HINT, TIMEZONE_HINT, UNKNOWN_TIMEZONE_ERROR, WEB_UI_LABEL } from "../../constants/reminders";
import { defaultConnectorTypes, deliveryLabel, enabledConnectorTypes, withConnectorSkipped } from "../../utils/reminders/channels";
import { browserTimeZone, isKnownTimeZone, knownTimeZones } from "../../utils/format/timezone";
import type { Model } from "../../types/model";
import type { ChoiceOption } from "../../types/ui";

// Settings · Local engine: the global configuration as numbered panels. Every
// field writes a copy of the config through `update:editConfig` (the page owns
// it and its save); only the log level applies on the spot.

const props = defineProps<{
  /** Which General section is open: only its panels show (all stay mounted). */
  section: SettingsTab;
  editConfig: GlobalConfig;
  logLevel: string;
  models: Model[];
}>();

const show = (panel: GeneralPanel) => GENERAL_PANEL_SECTION[panel] === props.section;

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

/** A global hot-memory default as a v-model target; the pair is always written whole. */
function memoryField(key: keyof MemoryHotDefaults) {
  return computed<boolean>({
    get: () => props.editConfig.memory?.[key] ?? SHIPPED_MEMORY_DEFAULTS[key],
    set: (value) => replaceConfig({ memory: { ...SHIPPED_MEMORY_DEFAULTS, ...props.editConfig.memory, [key]: value } }),
  });
}
const assistantHot = memoryField("assistant_hot");
const automationHot = memoryField("automation_hot");

/** The timezone as a v-model target; the reminders section is written whole. */
const reminderTimezone = computed<string>({
  get: () => props.editConfig.reminders?.timezone ?? "",
  set: (value) => replaceConfig({ reminders: { ...props.editConfig.reminders, timezone: value } }),
});
// Only a connector that is switched on can carry a reminder; it does until it is ticked off.
const reminderConnectors = computed(() => enabledConnectorTypes(props.editConfig.communication?.connectors));
const receivesReminders = (type: string) => !(props.editConfig.reminders?.skip_connectors ?? []).includes(type);
const setReceivesReminders = (type: string, receives: boolean) =>
  replaceConfig({ reminders: { ...props.editConfig.reminders, skip_connectors: withConnectorSkipped(props.editConfig.reminders?.skip_connectors, type, receives) } });
const reminderDelivery = computed(() => deliveryLabel(defaultConnectorTypes(reminderConnectors.value, props.editConfig.reminders?.skip_connectors)));
const timezoneError = computed(() => (isKnownTimeZone(reminderTimezone.value) ? undefined : UNKNOWN_TIMEZONE_ERROR));
const TIMEZONE_LIST_ID = "reminder-timezones";
const timezoneChoices = knownTimeZones();

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

// The adaptive result cap is on unless the saved config says otherwise; turning it off is saved as an explicit false.
const adaptiveResultCap = computed({
  get: () => props.editConfig.agent?.adaptive_result_cap ?? true,
  set: (adaptive_result_cap: boolean) => replaceConfig({ agent: { ...props.editConfig.agent, adaptive_result_cap } }),
})

// Helpers (delegated sub-agents) run on a cloud model only: a local one would take the GPU from the parent run.
const cloudModels = computed(() => props.models.filter((m) => m.workload_class === 'cloud'))
const subagentModel = computed({
  get: () => props.editConfig.agent?.subagent_model ?? '',
  set: (subagent_model: string) => replaceConfig({ agent: { ...props.editConfig.agent, subagent_model } }),
})
// A helper model that was removed from the models (or is not a cloud model) is still what is saved: it shows, flagged,
// so the user sees what is set and can change it, not a blank box.
const subagentModelUnavailable = computed(() => !!subagentModel.value && !cloudModels.value.some((m) => m.name === subagentModel.value))
const subagentConcurrency = computed({
  get: () => props.editConfig.agent?.subagent_concurrency ?? SUBAGENT_DEFAULT_CONCURRENCY,
  set: (subagent_concurrency: number) => replaceConfig({ agent: { ...props.editConfig.agent, subagent_concurrency } }),
})

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
    <Panel v-show="show('routing')" title="Routing">
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

    <Panel v-show="show('engine')" title="Local engine">
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

    <Panel v-show="show('storage')" title="Workspace storage">
      <div class="form-grid">
        <FormField
          label="Workspaces directory"
          hint="Empty: workspaces/ in the repository during development, otherwise in the data root (LLM_PROXY_HOME / --data). Relative paths resolve against the data root."
        >
          <template #default="{ id, describedBy }">
            <input :id="id" v-model="workspacesDir" :aria-describedby="describedBy" type="text" class="form-control font-mono" autocomplete="off" />
          </template>
        </FormField>
        <FormField
          label="Run logging"
          hint="Each run gets a folder with its event stream and execution history, plus recording.jsonl: every request sent to the model and every reply, with credential-shaped values masked. Runs are kept 30 days. Read a recording at /admin/api/recordings."
        >
          <template #default="{ id, describedBy }">
            <BaseToggle :id="id" v-model="runLoggingEnabled" :described-by="describedBy" label="Record run logs and model requests" />
          </template>
        </FormField>
      </div>
    </Panel>

    <Panel v-show="show('lifecycle')" title="Model lifecycle">
      <div class="form-grid">
        <FormField label="Idle timeout (seconds)" hint="A local model with no requests for this long is stopped to free memory. -1 keeps it loaded.">
          <template #default="{ id, describedBy }">
            <input :id="id" v-model.number="idleTimeout" :aria-describedby="describedBy" type="number" min="-1" step="1" class="form-control font-mono tabular-nums" />
          </template>
        </FormField>
      </div>
    </Panel>

    <Panel v-show="show('gpu')" title="GPU metrics">
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

    <Panel v-show="show('scheduler')" title="Run scheduler">
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
        <FormField
          label="Tool results"
          hint="A file, a page or search hits keep more of their text when the model's window has room for it, and never less than 3000 characters. Off keeps a fixed 3000 characters for every result. Applies to the next run."
        >
          <template #default="{ id, describedBy }">
            <BaseToggle :id="id" v-model="adaptiveResultCap" :described-by="describedBy" label="Size tool results to the room in the window" />
          </template>
        </FormField>
        <FormField
          label="Helper model"
          hint="A research chat can hand questions to helpers that search the web in parallel and send back a short answer. They run on this cloud model; Off turns helpers off."
        >
          <template #default="{ id, describedBy }">
            <select :id="id" v-model="subagentModel" :aria-describedby="describedBy" class="form-control">
              <option value="">Off — no helpers</option>
              <option v-if="subagentModelUnavailable" :value="subagentModel">{{ subagentModel }}{{ HELPER_MODEL_UNAVAILABLE_SUFFIX }}</option>
              <option v-for="m in cloudModels" :key="m.name" :value="m.name">{{ m.name }} ({{ m.provider }})</option>
            </select>
          </template>
        </FormField>
        <FormField label="Helpers at once" hint="Across the app, beside the run lanes. 1 to 10.">
          <template #default="{ id, describedBy }">
            <input :id="id" v-model.number="subagentConcurrency" :aria-describedby="describedBy" type="number" :min="1" :max="SUBAGENT_MAX_CONCURRENCY" step="1" class="form-control font-mono tabular-nums" />
          </template>
        </FormField>
      </div>
    </Panel>

    <Panel v-show="show('external')" title="External requests">
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

    <Panel v-show="show('memory')" title="Memory">
      <p class="mb-4 mt-0 max-w-[64ch] text-[length:var(--text-small)] text-muted">
        Hot memory puts the facts you marked "always" in front of the model at the start of a run. It uses a small
        share of the context window, and only when such facts exist. These are the defaults: a workspace's assistant
        and each automation can override them.
      </p>
      <div class="form-grid">
        <FormField label="Assistant" hint="Chats in every workspace unless the workspace overrides it.">
          <template #default="{ id, describedBy }">
            <BaseToggle :id="id" v-model="assistantHot" :described-by="describedBy" label="Assistant remembers" />
          </template>
        </FormField>
        <FormField label="Automations" hint="Scheduled and manual runs. Off by default: each run spends part of its context on it.">
          <template #default="{ id, describedBy }">
            <BaseToggle :id="id" v-model="automationHot" :described-by="describedBy" label="Automations remember" />
          </template>
        </FormField>
      </div>
    </Panel>

    <Panel v-show="show('reminders')" title="Reminders">
      <p class="mb-4 mt-0 max-w-[64ch] text-[length:var(--text-small)] text-muted">
        Ask the assistant to remind you of something and it is delivered word for word at the time you gave, with no
        model involved.
      </p>
      <div class="form-grid">
        <FormField label="Deliver reminders to" :hint="DELIVER_TO_HINT">
          <template #default>
            <div class="flex flex-col gap-1" data-test="reminder-delivery">
              <ToggleField :model-value="true" disabled :label="WEB_UI_LABEL" hint="Always: every reminder is listed on the Reminders page." />
              <ToggleField
                v-for="type in reminderConnectors"
                :key="type"
                :model-value="receivesReminders(type)"
                :label="type"
                @update:model-value="setReceivesReminders(type, $event)"
              />
              <p v-if="!reminderConnectors.length" class="m-0 text-[length:var(--text-small)] text-faint">No connector is switched on, so reminders stay in the web UI.</p>
              <p class="m-0 text-[length:var(--text-small)] text-muted" data-test="reminder-delivery-summary">Reminders go to: {{ reminderDelivery }}</p>
            </div>
          </template>
        </FormField>
        <FormField label="Timezone" :hint="TIMEZONE_HINT" :error="timezoneError">
          <template #default="{ id, describedBy, invalid }">
            <div class="flex flex-wrap items-center gap-2">
              <input :id="id" v-model.trim="reminderTimezone" :aria-describedby="describedBy" :aria-invalid="invalid" :list="TIMEZONE_LIST_ID" type="text" placeholder="Europe/London" autocomplete="off" class="form-control font-mono" />
              <BaseButton variant="secondary" size="sm" @click="reminderTimezone = browserTimeZone()">Use this browser's timezone</BaseButton>
            </div>
            <datalist :id="TIMEZONE_LIST_ID">
              <option v-for="zone in timezoneChoices" :key="zone" :value="zone"></option>
            </datalist>
          </template>
        </FormField>
      </div>
    </Panel>

    <Panel v-show="show('logging')" title="Logging">
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
