<script setup lang="ts">
import { ref, watch, onMounted } from 'vue'
import BaseToggle from '../common/BaseToggle.vue'
import SandboxMonitor from './SandboxMonitor.vue'
import { AdminApiService } from '../../services/adminService'
import { useToast } from '../../composables/useToast'

const toast = useToast()
const isLoading = ref(true)
const isSaving = ref(false)

const settings = ref({
  sandboxing: {
    enabled: true,
    max_storage_gb: 2,
    max_memory_mb: 256,
    functional: true
  }
})

const originalSettings = ref<string>('')
const hasChanges = ref(false)

const fetchSettings = async () => {
  isLoading.value = true
  try {
    const data = await AdminApiService.fetchHostSettings()
    if (data && data.sandboxing) {
      settings.value = data
      originalSettings.value = JSON.stringify(data)
    }
  } catch (e: any) {
    if (e.message !== "Not Found") { // Ignore 404 for existing defaults
      toast.error(`Failed to load security settings: ${e.message}`)
    }
  } finally {
    isLoading.value = false
  }
}

const handleToggle = (val: boolean) => {
  if (!val) {
    if (!confirm("Are you sure you want to DISABLE sandboxing? This allows the agent to execute raw shell commands directly on your host machine.")) {
      // Revert if cancelled
      settings.value.sandboxing.enabled = true
      return
    }
  }
  settings.value.sandboxing.enabled = val
}

const saveSettings = async () => {
  isSaving.value = true
  try {
    await AdminApiService.updateHostSettings(settings.value)
    originalSettings.value = JSON.stringify(settings.value)
    hasChanges.value = false
    toast.success('Security settings saved successfully')
  } catch (e: any) {
    toast.error(`Failed to save settings: ${e.message}`)
  } finally {
    isSaving.value = false
  }
}

const handleRestart = async () => {
    if (!confirm("Restart the backend now? This will terminate all active agent sessions.")) return
    
    try {
        await AdminApiService.restartSystem()
        toast.info("Restart requested. Reconnecting...")
        setTimeout(() => window.location.reload(), 5000)
    } catch (e: any) {
        toast.error(`Restart failed: ${e.message}`)
    }
}

// Track changes manually instead of auto-saving
watch(settings, (newVal) => {
    if (!isLoading.value) {
        hasChanges.value = JSON.stringify(newVal) !== originalSettings.value
    }
}, { deep: true })

onMounted(() => {
  fetchSettings()
})
</script>

<template>
  <div class="security-card">
    <div class="settings-header">
      <h2 class="settings-title">WASM Virtual Machine</h2>
      <p class="settings-subtitle">
        High-performance WebAssembly sandboxing for Agent tool execution
      </p>
    </div>

    <div v-if="isLoading" class="loading-state">
      Initializing WASM runtime...
    </div>
    
    <div v-else class="settings-content">
        <!-- Sandboxing Toggle -->
        <div class="setting-group advanced-card" :class="{ 
            'danger-zone': !settings.sandboxing.enabled,
            'functional-error': settings.sandboxing.enabled && !settings.sandboxing.functional 
        }">
          <div class="toggle-header">
            <div>
              <span class="setting-label">Enable Global Sandboxing</span>
              <span class="setting-description">
                Isolates all terminal tool executions inside a lightweight WebAssembly virtual jail (Wazero). 
              </span>
            </div>
            <BaseToggle 
                :model-value="settings.sandboxing.enabled" 
                @update:model-value="handleToggle" 
            />
          </div>
          
          <!-- Case 1: Manually Disabled -->
          <div v-if="!settings.sandboxing.enabled" class="alert-box alert-danger">
            <span class="alert-icon">⚠️</span>
            <div class="alert-content">
              <strong>High-Risk Warning:</strong>
              <p>Disabling sandboxing allows the LLM agent to execute raw shell commands directly on your host machine. This disables zero-trust isolation and allows full file system modifications.</p>
            </div>
          </div>

          <!-- Case 2: Enabled but Unreachable (WASM crash) -->
          <div v-if="settings.sandboxing.enabled && !settings.sandboxing.functional" class="alert-box alert-functional">
            <span class="alert-icon">❌</span>
            <div class="alert-content">
              <strong>Runtime Error: WASM Engine Failed</strong>
              <p>WASM sandboxing is enabled but the runtime failed to initialize. The agent is currently falling back to **native host execution**. Check logs for Wazero initialization errors.</p>
            </div>
          </div>
        </div>

        <div class="grid grid-cols-2 gap-4">
          <div class="setting-group" :class="{ 'disabled-state': !settings.sandboxing.enabled || !settings.sandboxing.functional }">
              <label class="setting-label">Memory Limit (MB)</label>
              <input 
                  v-model.number="settings.sandboxing.max_memory_mb" 
                  type="number" 
                  class="base-input" 
                  min="64" 
                  max="1024"
                  :disabled="!settings.sandboxing.enabled || !settings.sandboxing.functional"
              />
              <p class="setting-description mt-1">Maximum heap allocated to the WASM virtual machine.</p>
          </div>

          <div class="setting-group" :class="{ 'disabled-state': !settings.sandboxing.enabled || !settings.sandboxing.functional }">
              <label class="setting-label">Storage Quota (GB)</label>
              <input 
                  v-model.number="settings.sandboxing.max_storage_gb" 
                  type="number" 
                  class="base-input" 
                  min="1" 
                  max="50"
                  :disabled="!settings.sandboxing.enabled || !settings.sandboxing.functional"
              />
              <p class="setting-description mt-1">Strict jailing for workspace file system operations.</p>
          </div>
        </div>

        <!-- Action Bar -->
        <div class="action-bar pt-6 border-t border-gray-800 flex items-center justify-between">
           <div class="status-info flex items-center gap-2">
               <div v-if="hasChanges" class="flex items-center gap-2 text-yellow-500 text-xs font-bold uppercase tracking-wider animate-pulse">
                   <span class="w-2 h-2 rounded-full bg-yellow-500"></span>
                   Unsaved Changes
               </div>
               <div v-else class="text-gray-500 text-xs font-bold uppercase tracking-wider">
                   WASI Architecture Active
               </div>
           </div>

           <div class="flex gap-3">
               <button 
                   @click="handleRestart" 
                   class="btn-secondary"
               >
                   Restart Runtime
               </button>
               <button 
                   @click="saveSettings" 
                   class="btn-primary" 
                   :disabled="!hasChanges || isSaving"
               >
                   {{ isSaving ? 'Saving...' : 'Apply & Save' }}
               </button>
           </div>
        </div>
        
        <div class="restart-warning text-xs text-gray-500 mt-4 italic">
           * Note: Transitioning from legacy Docker to WASM requires a full backend restart to re-initialize the Wazero memory space.
        </div>

        <!-- Session Monitor -->
        <SandboxMonitor v-if="settings.sandboxing.enabled" />
    </div>
  </div>
</template>

<style scoped lang="postcss">
.security-card {
  @apply bg-gray-900 border border-gray-800 rounded-xl overflow-hidden;
}

.settings-header {
  @apply p-6 border-b border-gray-800 bg-gray-900/50;
}

.settings-title {
  @apply text-xl font-medium text-gray-100;
}

.settings-subtitle {
  @apply text-sm text-gray-400 mt-1;
}

.settings-content {
  @apply p-6 space-y-6;
}

.setting-group {
  @apply space-y-2;
}

.setting-label {
  @apply block text-sm font-medium text-gray-200;
}

.setting-description {
  @apply text-sm text-gray-400 block;
}

.base-input {
  @apply w-full bg-gray-950 border border-gray-700 rounded-lg px-4 py-2.5 text-gray-200 focus:outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500 transition-colors;
}

.toggle-header {
  @apply flex items-center justify-between;
}

.advanced-card {
  @apply p-4 rounded-lg border border-gray-800 bg-gray-900/50 transition-colors duration-300;
}

.danger-zone {
  @apply border-red-500/50 bg-red-950/20 shadow-[0_0_15px_rgba(239,68,68,0.1)];
}

.functional-error {
  @apply border-red-600 bg-red-900/10 shadow-[0_0_20px_rgba(220,38,38,0.15)];
  animation: pulse-border 2s infinite;
}

@keyframes pulse-border {
  0% { @apply border-red-600/60; }
  50% { @apply border-red-600; }
  100% { @apply border-red-600/60; }
}

.alert-box {
  @apply flex gap-3 p-4 rounded-lg mt-4 border;
}

.alert-danger {
  @apply bg-red-950/30 border-red-500/30 text-red-200;
}

.alert-functional {
  @apply bg-red-900/20 border-red-600/40 text-red-100;
}

.alert-icon {
  @apply text-xl;
}

.alert-content p {
  @apply text-sm text-red-300/80 mt-1;
}

.action-bar {
  @apply mt-auto;
}

.btn-primary {
  @apply px-4 py-2 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 disabled:grayscale text-white text-sm font-bold rounded-lg transition-all active:scale-95 shadow-lg shadow-blue-900/20;
}

.btn-secondary {
  @apply px-4 py-2 bg-gray-800 hover:bg-gray-700 text-gray-200 text-sm font-bold rounded-lg border border-gray-700 transition-all active:scale-95;
}

.disabled-state {
  @apply opacity-50 cursor-not-allowed grayscale pointer-events-none transition-all duration-300;
}
</style>
