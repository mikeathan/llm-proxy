<template>
  <div class="settings-container">
    <h2 class="settings-title">Global Settings</h2>

    <form @submit.prevent="submitConfig" class="settings-form">
      <div class="form-group">
        <label class="form-label">Model Directory</label>
        <div class="form-helper">Absolute or relative path to scan for .gguf files</div>
        <input v-model="localConfig.model_dir" type="text" class="form-input">
      </div>

      <div class="form-group">
        <label class="form-label">Llama Binary Path</label>
        <div class="form-helper">Path to llama-server executable</div>
        <input v-model="localConfig.llama_binary" type="text" class="form-input">
      </div>

      <div class="form-group">
        <label class="form-label">Model Host IP</label>
        <div class="form-helper">IP address the underlying server binds to (default: 127.0.0.1)</div>
        <input v-model="localConfig.model_host" type="text" class="form-input">
      </div>

      <div class="form-group">
        <label class="form-label">Global Default Arguments</label>
        <div class="form-helper">Space-separated arguments passed to all models (e.g. --ctx-size 8192)</div>
        <textarea v-model="defaultArgsStr" class="form-input font-mono text-xs" rows="2" placeholder="--ctx-size 4096"></textarea>
      </div>

      <div class="form-group">
        <label class="form-label">Global Environment Variables</label>
        <div class="form-helper">Line-separated KEY=VALUE pairs injected into the environment</div>
        <textarea v-model="environmentStr" class="form-input font-mono text-xs" rows="3" placeholder="HSA_OVERRIDE_GFX_VERSION=11.0.0&#10;AMD_SERIALIZE_KERNEL=1"></textarea>
      </div>

      <div class="form-section">
        <label class="form-label">System Log Level</label>
        <div class="form-helper mb-custom">Change the verbosity of proxy logging in the terminal</div>
        <LogLevelPanel :modelValue="logLevel" @update="$emit('updateLogLevel', $event)" />
      </div>

      <div class="form-actions">
        <button type="submit" class="btn-submit">Save Settings</button>
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
  (e: 'update:editConfig', config: any): void
  (e: 'updateConfig'): void
  (e: 'updateLogLevel', level: string): void
}>()

const localConfig = ref({ ...props.editConfig })

const defaultArgsStr = computed({
  get: () => argsToString(localConfig.value.default_args),
  set: (val: string) => { localConfig.value.default_args = stringToArgs(val) }
})

const environmentStr = computed({
  get: () => envMapToString(localConfig.value.environment),
  set: (val: string) => { localConfig.value.environment = stringToEnvMap(val) }
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
  @apply bg-gray-800 rounded-lg shadow border border-gray-700 p-6 space-y-4;
}
.settings-title {
  @apply text-lg font-semibold text-white mb-4 border-b border-gray-700 pb-2;
}
.settings-form {
  @apply space-y-4;
}
.form-label {
  @apply block text-sm font-medium text-gray-300 mb-1;
}
.form-helper {
  @apply text-xs text-gray-500 mb-1;
}
.mb-custom {
  @apply mb-2;
}
.form-input {
  @apply w-full bg-gray-900 border border-gray-600 rounded px-3 py-2 text-white focus:border-blue-500 focus:outline-none;
}
.form-section {
  @apply pt-2 border-t border-gray-700 mt-2;
}
.log-level-buttons {
  @apply flex gap-2;
}
.btn-log {
  @apply px-3 py-1.5 rounded text-xs font-medium transition-colors;
}
.btn-log-active {
  @apply bg-blue-600 text-white;
}
.btn-log-inactive {
  @apply bg-gray-700 text-gray-300 hover:bg-gray-600;
}
.form-actions {
  @apply pt-4 border-t border-gray-700 mt-4;
}
.btn-submit {
  @apply bg-blue-600 hover:bg-blue-500 text-white px-4 py-2 rounded font-medium transition-colors;
}
</style>
