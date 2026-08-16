<script setup lang="ts">
import { ref, watch, onMounted } from 'vue'
import BaseToggle from '../common/buttons/BaseToggle.vue'
import TerminalMonitor from './TerminalMonitor.vue'
import { AdminApiService } from '../../services/admin/adminService'
import { useToast } from '../../composables/useToast'
import { useConfirm } from '../../composables/ui/useConfirm'

const toast = useToast()
const { confirm: confirmDialog } = useConfirm()
const isLoading = ref(true)
const isSaving = ref(false)

const settings = ref({
  sandboxing: {
    enabled: true,
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
      toast.error(`Failed to load terminal settings: ${e.message}`)
    }
  } finally {
    isLoading.value = false
  }
}

const handleToggle = (val: boolean) => {
  if (!val) {
    if (!confirm("Are you sure you want to DISABLE persistent terminals? This may cause the agent to lose state between tool executions.")) {
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
    toast.success('Terminal settings saved successfully')
  } catch (e: any) {
    toast.error(`Failed to save settings: ${e.message}`)
  } finally {
    isSaving.value = false
  }
}

const handleRestart = async () => {
    if (!confirm("Restart the backend now? This will terminate all active terminal sessions.")) return
    
    try {
        await AdminApiService.restartSystem()
        toast.info("Restart requested. Reconnecting...")
        setTimeout(() => window.location.reload(), 5000)
    } catch (e: any) {
        toast.error(`Restart failed: ${e.message}`)
    }
}

const isResetting = ref(false)

const handleClearRuntimeData = async () => {
    if (!confirm("Clear all runtime state (per-workspace sessions, process logs, locks, runs, and app logs)? Config, secrets, and database are untouched. Active sessions/runs are stopped.")) return
    isResetting.value = true
    try {
        await AdminApiService.clearRuntimeData()
        toast.success("Runtime data cleared")
    } catch (e: any) {
        toast.error(`Failed to clear runtime data: ${e.message}`)
    } finally {
        isResetting.value = false
    }
}

const handleFactoryReset = async () => {
    if (!confirm("FACTORY RESET: reset settings.yml, registry, and secrets to defaults and generate a NEW master key. All stored API keys are unrecoverable. Orchestrator DB, templates, and workspaces are untouched. Restart recommended afterwards.")) return
    isResetting.value = true
    try {
        const res = await AdminApiService.factoryReset()
        toast.success(res.key_externally_managed
            ? "Factory reset complete (master key externally managed, reused)"
            : "Factory reset complete. New master key generated.")
    } catch (e: any) {
        toast.error(`Factory reset failed: ${e.message}`)
    } finally {
        isResetting.value = false
    }
}

const handleWipeout = async () => {
    const ok = await confirmDialog({
        title: 'Wipeout (Uninstall)',
        message: 'This permanently deletes everything the service created: configuration, settings, API keys/secrets, the orchestrator database, templates, logs, runs, and the workspaces directory. The server will stop afterwards. This cannot be undone.',
        type: 'error',
        confirmText: 'Wipe everything',
        cancelText: 'Cancel',
    })
    if (!ok) return
    isResetting.value = true
    try {
        await AdminApiService.wipeout()
        toast.success('Service wiped. The server is stopping.')
    } catch (e: any) {
        toast.error(`Wipeout failed: ${e.message}`)
        isResetting.value = false
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
      <h2 class="settings-title">Local Host Terminal</h2>
      <p class="settings-subtitle">
        Managed persistent shell sessions for Agent tool execution
      </p>
    </div>

    <div v-if="isLoading" class="loading-state">
      Initializing terminal provider...
    </div>
    
    <div v-else class="settings-content">
        <!-- Terminal Toggle -->
        <div class="setting-group advanced-card" :class="{ 
            'danger-zone': !settings.sandboxing.enabled,
            'functional-error': settings.sandboxing.enabled && !settings.sandboxing.functional 
        }">
          <div class="toggle-header">
            <div>
              <span class="setting-label">Enable Persistent Terminals</span>
              <span class="setting-description">
                Maintains a long-running bash session for each workspace, allowing the agent to preserve environment variables and state between commands.
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
              <strong>State Loss Warning:</strong>
              <p>Disabling persistent terminals means each command runs in a fresh, temporary shell. Environment variables (like export PATH) or directory changes (cd) will not persist between tool calls.</p>
            </div>
          </div>

          <!-- Case 2: Enabled but Unreachable -->
          <div v-if="settings.sandboxing.enabled && !settings.sandboxing.functional" class="alert-box alert-functional">
            <span class="alert-icon">❌</span>
            <div class="alert-content">
              <strong>Initialization Error</strong>
              <p>The terminal provider failed to initialize. The agent will fall back to single-shot command execution. Check system logs for details.</p>
            </div>
          </div>
        </div>

        <!-- Reset Controls (Phase 10) -->
        <div class="setting-group advanced-card">
          <div class="toggle-header">
            <div>
              <span class="setting-label">Reset Controls</span>
              <span class="setting-description">
                Clear runtime state or reset configuration and secrets to factory defaults. Both operate on a fixed allowlist of paths — orchestrator DB, templates, and workspaces are never touched.
              </span>
            </div>
          </div>
          <div class="flex gap-3 mt-3">
            <button
                @click="handleClearRuntimeData"
                class="btn-secondary"
                :disabled="isResetting"
            >
                Clear Runtime Data
            </button>
            <button
                @click="handleFactoryReset"
                class="btn-danger"
                :disabled="isResetting"
            >
                Factory Reset
            </button>
          </div>
          <div class="flex mt-3">
            <button
                @click="handleWipeout"
                class="btn-wipeout"
                :disabled="isResetting"
            >
                Wipeout (Uninstall)
            </button>
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
                   Native Host Shell Active
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
           * Note: Terminal sessions are isolated by workspace and use the host's native tools (node, python, go, etc.).
        </div>

        <!-- Session Monitor -->
        <TerminalMonitor v-if="settings.sandboxing.enabled" />
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

.loading-state {
  @apply p-12 text-center text-gray-500 italic;
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

.toggle-header {
  @apply flex items-center justify-between;
}

.advanced-card {
  @apply p-4 rounded-lg border border-gray-800 bg-gray-900/50 transition-colors duration-300;
}

.danger-zone {
  @apply border-yellow-500/30 bg-yellow-950/10;
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
  @apply bg-yellow-950/30 border-yellow-500/30 text-yellow-200;
}

.alert-functional {
  @apply bg-red-900/20 border-red-600/40 text-red-100;
}

.alert-icon {
  @apply text-xl;
}

.alert-content p {
  @apply text-sm text-gray-400 mt-1;
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

.btn-danger {
  @apply px-4 py-2 bg-red-700/80 hover:bg-red-600 text-white text-sm font-bold rounded-lg border border-red-500 transition-all active:scale-95 disabled:opacity-50 disabled:grayscale;
}

.btn-wipeout {
  @apply px-4 py-2 bg-red-900 hover:bg-red-800 text-white text-sm font-bold rounded-lg border-2 border-red-600 transition-all active:scale-95 disabled:opacity-50 disabled:grayscale;
}
</style>
