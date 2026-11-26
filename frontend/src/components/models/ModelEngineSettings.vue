<script setup lang="ts">
import { computed, ref, watch } from "vue";
import Panel from "../common/layout/Panel.vue";
import FormField from "../common/forms/FormField.vue";
import { argsToString, stringToArgs, envMapToString, stringToEnvMap } from "../../utils/config";
import { useConfigDraftFields } from "../../composables/settings/useConfigDraftFields";
import type { GlobalConfig, ProviderItem } from "../../types/admin";
import type { ChoiceOption } from "../../types/ui";

// Models · Engine: how local models are served (llama.cpp). The fields edit the shared configuration draft the page owns and saves
// (moved from Settings · General, ADR-0090).

const props = defineProps<{ editConfig: GlobalConfig }>();
const emit = defineEmits<{ (e: "update:editConfig", config: GlobalConfig): void }>();

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
const EMPTY_LOCAL: ProviderItem = { type: "local", model_dir: "", llama_server_binary: "", default_args: [], environment: {} };

const { replaceConfig, configField } = useConfigDraftFields(() => props.editConfig, (next) => emit("update:editConfig", next));

const modelHost = configField("model_host");
const idleTimeout = configField("idle_timeout_seconds");
// Unset is "" on the server (no GPU metrics), so it shows as the "None" option rather than a blank select.
const gpuProvider = computed<string>({
  get: () => props.editConfig.gpu_provider ?? "",
  set: (gpu_provider) => replaceConfig({ gpu_provider }),
});
const gpuIndex = configField("gpu_index");
const gpuBinary = configField("gpu_binary");
const gpuSysfsPath = configField("gpu_sysfs_path");

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

const showGpuIndex = computed(() => !!gpuProvider.value && gpuProvider.value !== GPU_NO_INDEX);
const showGpuBinary = computed(() => GPU_TOOL_PROVIDERS.includes(gpuProvider.value ?? ""));
const showSysfsPath = computed(() => gpuProvider.value === GPU_SYSFS);
</script>

<template>
  <div class="flex flex-col gap-4">
    <Panel title="Where models come from">
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
    </Panel>

    <Panel title="Every local model">
      <div class="form-grid">
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
  </div>
</template>

<style scoped lang="postcss">
.form-grid {
  @apply grid grid-cols-[repeat(auto-fit,minmax(min(100%,240px),1fr))] gap-4;
}
</style>
