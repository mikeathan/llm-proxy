<template>
  <div class="settings-container">
    <h2 class="settings-title">Local Engine Configuration</h2>

    <form @submit.prevent="submitConfig" class="settings-form">
      <div class="form-group">
        <label class="form-label">Model Directory</label>
        <div class="form-helper">Absolute or relative path to scan for .gguf files</div>
        <input v-model="localProvider.model_dir" type="text" class="form-input" placeholder="/path/to/models">
      </div>

      <div class="form-group">
        <label class="form-label">Workspaces Directory</label>
        <div class="form-helper">Path for agent workspaces (automations, state). Defaults to &lt;repo&gt;/workspaces</div>
        <input v-model="localConfig.workspaces_dir" type="text" class="form-input">
      </div>

      <div class="form-group">
        <label class="form-label">Llama Server Binary</label>
        <div class="form-helper">Path to llama-server executable</div>
        <input v-model="localProvider.llama_server_binary" type="text" class="form-input" placeholder="/usr/local/bin/llama-server">
      </div>

      <div class="form-group">
        <label class="form-label">Model Host IP</label>
        <div class="form-helper">IP address the underlying server binds to (default: 127.0.0.1)</div>
        <input v-model="localConfig.model_host" type="text" class="form-input">
      </div>

      <div class="form-group">
        <label class="form-label">Default Arguments</label>
        <div class="form-helper">Space-separated arguments passed to all local models</div>
        <textarea v-model="defaultArgsStr" class="form-input font-mono text-xs" rows="2" placeholder="--ctx-size 4096"></textarea>
      </div>

      <div class="form-group">
        <label class="form-label">Environment Variables</label>
        <div class="form-helper">Line-separated KEY=VALUE pairs injected into the environment</div>
        <textarea v-model="environmentStr" class="form-input font-mono text-xs" rows="3" placeholder="HSA_OVERRIDE_GFX_VERSION=11.0.0&#10;AMD_SERIALIZE_KERNEL=1"></textarea>
      </div>

      <div class="form-section">
        <label class="form-label">System Log Level</label>
        <div class="form-helper mb-custom">Change the verbosity of proxy logging in the terminal</div>
        <LogLevelPanel :modelValue="logLevel" @update="$emit('updateLogLevel', $event)" />
      </div>

      <div class="form-actions">
        <button type="submit" class="btn-submit">Save Local Engine Settings</button>
      </div>
    </form>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import LogLevelPanel from './LogLevelPanel.vue'
import { argsToString, stringToArgs, envMapToString, stringToEnvMap } from '../utils/config'
import type { GlobalConfig } from '../types/admin'

const props = defineProps<{
  editConfig: GlobalConfig
  logLevel: string
}>()

const emit = defineEmits<{
  (e: 'update:editConfig', config: GlobalConfig): void
  (e: 'updateConfig'): void
  (e: 'updateLogLevel', level: string): void
}>()

const localConfig = ref({ ...props.editConfig })

// Dynamic access to providers.local
const localProvider = computed(() => {
  if (!localConfig.value.providers) localConfig.value.providers = {}
  if (!localConfig.value.providers.local) {
    localConfig.value.providers.local = {
      type: 'local',
      model_dir: '',
      llama_server_binary: '',
      default_args: [],
      environment: {}
    }
  }
  return localConfig.value.providers.local
})

const defaultArgsStr = computed({
  get: () => argsToString(localProvider.value.default_args),
  set: (val: string) => { localProvider.value.default_args = stringToArgs(val) }
})

const environmentStr = computed({
  get: () => envMapToString(localProvider.value.environment),
  set: (val: string) => { localProvider.value.environment = stringToEnvMap(val) }
})

watch(() => props.editConfig, (newVal) => {
  localConfig.value = JSON.parse(JSON.stringify(newVal))
}, { deep: true })

watch(localConfig, (newVal) => {
  emit('update:editConfig', newVal)
}, { deep: true })

function submitConfig() {
  emit('updateConfig')
}
</script>

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
  @apply w-full bg-gray-900 border border-gray-700 rounded-md px-3 py-2.5 text-white transition-all focus:ring-2 focus:ring-blue-600/30 focus:border-blue-600 outline-none;
}
.form-section {
  @apply pt-4 border-t border-gray-700 mt-4;
}
.form-actions {
  @apply pt-6 border-t border-gray-700 flex justify-end;
}
.btn-submit {
  @apply bg-blue-600 hover:bg-blue-500 text-white px-6 py-2.5 rounded-md font-bold transition-all shadow-lg hover:shadow-blue-600/20 active:scale-95;
}
</style>
