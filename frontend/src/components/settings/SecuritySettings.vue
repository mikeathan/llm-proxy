<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import BaseToggle from '../common/buttons/BaseToggle.vue'
import TerminalMonitor from './TerminalMonitor.vue'
import { AdminApiService } from '../../services/admin/adminService'
import { useToast } from '../../composables/useToast'
import { useConfirm } from '../../composables/ui/useConfirm'
import { useHostSandboxingEditor } from '../../composables/settings/useHostSandboxingEditor'
import { errorMessage } from '../../utils/errors'
import { formatSandboxSurface, sandboxSurfaceShort } from '../../utils/sandboxing'
import type { SandboxSurface } from '../../types/admin'

const toast = useToast()
const { confirm: confirmDialog } = useConfirm()
const isResetting = ref(false)

// Form state, dirty tracking and persistence live in the editor composable —
// the component only renders it and owns the page-level destructive actions.
const editor = useHostSandboxingEditor()
const { draft, effective, hasChanges, networkAllowed, networkDecided, filesystemOn, loading, saving } = editor

onMounted(async () => {
  await editor.load()
  if (editor.loadError.value) toast.error(`Failed to load security settings: ${editor.loadError.value}`)
})

// ---- Effective state readouts -------------------------------------------------
// Pills next to a toggle prefer the backend Effective projection (what this
// host actually enforces) and fall back to the configured posture, labelled as
// such, until the backend reports. Full rows in the Effective card always show
// the reason, so a downgrade is never silent.

const surfaceEnforced = (surface: SandboxSurface | undefined): boolean =>
  !!surface && surface.mechanism !== 'none'

const filesystemPill = computed(() =>
  effective.value
    ? sandboxSurfaceShort(effective.value.filesystem)
    : filesystemOn.value
      ? 'on (configured)'
      : 'off (configured)',
)
const networkPill = computed(() => {
  if (effective.value) return sandboxSurfaceShort(effective.value.network)
  if (networkAllowed.value) return networkDecided.value ? 'on (explicit)' : 'on (legacy, configured)'
  return 'off (configured)'
})
const filesystemTone = computed(() =>
  effective.value
    ? surfaceEnforced(effective.value.filesystem)
      ? 'text-on'
      : 'text-off'
    : filesystemOn.value
      ? 'text-on'
      : 'text-off',
)
const networkTone = computed(() => (networkAllowed.value ? 'text-on' : 'text-off'))
const filesystemEffectiveText = computed(() => formatSandboxSurface(effective.value?.filesystem))
const networkEffectiveText = computed(() => formatSandboxSurface(effective.value?.network))

// ---- Sandboxing switch actions ------------------------------------------------
const toggleMaster = (value: boolean) => {
  if (value) {
    editor.patch({ enabled: true })
    return
  }
  confirmDialog({
    title: 'Disable sandboxing?',
    message:
      'The backend requires terminal execution: with the master off it refuses to start after the next restart. The filesystem/network switches below stay configured (and keep gating what runs) — prefer individual switches over the master.',
    type: 'warning',
    confirmText: 'Disable',
  }).then((ok) => {
    if (ok) editor.patch({ enabled: false })
  })
}

const toggleFilesystem = (value: boolean) => {
  // Explicit true/false once the operator touches it (absent meant ON).
  editor.patch({ filesystem: value })
}

const setNetwork = (value: boolean) => {
  if (!value && (networkAllowed.value || !networkDecided.value)) {
    confirmDialog({
      title: 'Restrict agent network?',
      message:
        'Network off blocks agent network tools (fetch/scan/search/connector sends) everywhere; inbound webhook receipt is unaffected. Terminal processes keep direct network access until the OS network layer or the egress proxy is enabled. Per-run grants stay configured but are inert while the host switch is off.',
      type: 'error',
      confirmText: 'Restrict network',
    }).then((ok) => {
      if (ok) editor.patch({ network: false })
    })
    return
  }
  editor.patch({ network: value })
}

const restrictNetwork = () => editor.patch({ network: false })
const keepNetworkAllowed = () => editor.patch({ network: true })

const handleSave = async () => {
  const ok = await editor.save()
  if (ok) {
    toast.success('Security settings saved successfully')
  } else {
    toast.error(`Failed to save settings: ${editor.saveError.value}`)
  }
}

// ---- Page-level actions (destructive; backend service owns the calls) ---------
const handleRestartRuntime = async () => {
  const ok = await confirmDialog({
    title: 'Restart runtime',
    message: 'Restart the backend now? This will terminate all active terminal sessions.',
    type: 'warning',
    confirmText: 'Restart',
  })
  if (!ok) return
  try {
    await AdminApiService.restartSystem()
    toast.info('Restart requested. Reconnecting...')
    setTimeout(() => window.location.reload(), 5000)
  } catch (e) {
    toast.error(`Restart failed: ${errorMessage(e)}`)
  }
}

const handleClearRuntimeData = async () => {
  const ok = await confirmDialog({
    title: 'Clear runtime data',
    message:
      'Clear all runtime state (per-workspace sessions, process logs, locks, runs, and app logs)? Config, secrets, and database are untouched. Active sessions/runs are stopped.',
    type: 'warning',
    confirmText: 'Clear',
  })
  if (!ok) return
  isResetting.value = true
  try {
    await AdminApiService.clearRuntimeData()
    toast.success('Runtime data cleared')
  } catch (e) {
    toast.error(`Failed to clear runtime data: ${errorMessage(e)}`)
  } finally {
    isResetting.value = false
  }
}

const handleFactoryReset = async () => {
  const ok = await confirmDialog({
    title: 'Factory reset',
    message:
      'Reset settings.yml, registry, and secrets to defaults and generate a NEW master key? All stored API keys are unrecoverable. Orchestrator DB, templates, and workspaces are untouched.',
    type: 'error',
    confirmText: 'Reset',
  })
  if (!ok) return
  isResetting.value = true
  try {
    const res = await AdminApiService.factoryReset()
    toast.success(
      res.key_externally_managed
        ? 'Factory reset complete (master key externally managed, reused)'
        : 'Factory reset complete. New master key generated.',
    )
  } catch (e) {
    toast.error(`Factory reset failed: ${errorMessage(e)}`)
  } finally {
    isResetting.value = false
  }
}

const handleWipeout = async () => {
  const ok = await confirmDialog({
    title: 'Wipeout (Uninstall)',
    message:
      'This permanently deletes everything the service created: configuration, settings, API keys/secrets, the orchestrator database, templates, logs, runs, and the workspaces directory. The server will stop afterwards. This cannot be undone.',
    type: 'error',
    confirmText: 'Wipe everything',
    cancelText: 'Cancel',
  })
  if (!ok) return
  isResetting.value = true
  try {
    await AdminApiService.wipeout()
    toast.success('Service wiped. The server is stopping.')
  } catch (e) {
    toast.error(`Wipeout failed: ${errorMessage(e)}`)
    isResetting.value = false
  }
}
</script>

<template>
  <div class="security-card">
    <div class="settings-header">
      <h2 class="settings-title">Security &amp; Sandboxing</h2>
      <p class="settings-subtitle">
        Host-wide containment for everything the agent runs — terminal sessions, dev work, and network grants
      </p>
    </div>

    <div v-if="loading" class="loading-state">
      Loading security settings...
    </div>

    <div v-else class="settings-content">
      <!-- ===== Card 1: Sandboxing switches ===== -->
      <div class="setting-group advanced-card" :class="{
        'danger-zone': !draft.enabled,
        'functional-error': draft.enabled && !draft.functional,
      }">
        <div class="toggle-header">
          <div>
            <span class="setting-label">Sandboxing master</span>
            <span class="setting-description">
              Persistent-terminal runtime switch. Off = agent commands run one-shot (no session state) and startup logs a warning; the service still boots. The toggles below are the containment switches that gate agent work.
            </span>
          </div>
          <BaseToggle
            :model-value="draft.enabled"
            @update:model-value="toggleMaster"
          />
        </div>

        <div v-if="!draft.enabled" class="alert-box alert-danger">
          <span class="alert-icon">⚠️</span>
          <div class="alert-content">
            <strong>Persistent terminals disabled</strong>
            <p>Agent terminal commands fall back to one-shot execution without session state. The filesystem/network switches below still apply — re-enable the master for full agentic execution.</p>
          </div>
        </div>
        <div v-if="draft.enabled && !draft.functional" class="alert-box alert-functional">
          <span class="alert-icon">❌</span>
          <div class="alert-content">
            <strong>Terminal provider error</strong>
            <p>The terminal provider failed to initialize; single-shot execution is used. Check system logs.</p>
          </div>
        </div>

        <div class="divider" />

        <div class="toggle-header">
          <div>
            <span class="setting-label">Confine agent files to workspaces</span>
            <span class="setting-description">
              On (default): on Linux, agent processes are kernel-confined to their workspace files (workspace↔workspace isolation; host secrets unreadable). On macOS/dev no OS jail is active yet — production there uses the dedicated-user launchd deployment. Applies on restart.
              <strong :class="filesystemTone">Effective: {{ filesystemPill }}</strong>
            </span>
          </div>
          <BaseToggle
            :model-value="filesystemOn"
            @update:model-value="toggleFilesystem"
          />
        </div>

        <div class="divider" />

        <div class="toggle-header">
          <div>
            <span class="setting-label">Allow agent network <span class="badge-host">HOST MASTER</span></span>
            <span class="setting-description">
              Off = network-capable agent tools stop everywhere (fetch/scan/search/connector sends; inbound receipt unaffected). On = permitted; scope is granted per workspace / automation.
              Terminal processes are not yet OS-blocked on every host — enable the egress proxy for domain-level filtering and keep untrusted workspaces restricted.
              <strong :class="networkTone">
                Effective: {{ networkPill }}
              </strong>
            </span>
          </div>
          <BaseToggle
            :model-value="networkAllowed"
            @update:model-value="setNetwork"
          />
        </div>

        <!-- Migration banner: key absent = undecided (legacy allowed) -->
        <div v-if="draft.enabled && !networkDecided" class="alert-box alert-amber">
          <span class="alert-icon">⚠️</span>
          <div class="alert-content">
            <strong>Agent network is currently unrestricted (legacy default)</strong>
            <p>
              This install predates the network switch. Until you choose, any workspace/automation the guardrails permit can use the network.
              Restricting is recommended — you can re-enable the host switch from this page any time; per-workspace/automation grants only take effect while the host switch is on.
            </p>
            <div class="alert-actions">
              <button class="btn-primary btn-sm" @click="restrictNetwork">Restrict network</button>
              <button class="btn-secondary btn-sm" @click="keepNetworkAllowed">Keep allowed</button>
            </div>
          </div>
        </div>

        <div class="egress-row">
          <div>
            <span class="setting-label">Egress proxy port</span>
            <span class="setting-description">
              0 = off. When set, agent tool egress and network-on shell clients route through a loopback HTTP proxy
              that enforces host domain policy — allow/deny lists are read from settings.yml (egress_allow_domains / egress_deny_domains); an editing UI arrives later.
              Requires a restart to apply.
            </span>
          </div>
          <input
            v-model.number="draft.egress_proxy"
            type="number"
            min="0"
            max="65535"
            class="port-input"
            :disabled="!draft.enabled"
          />
        </div>
      </div>

      <!-- ===== Card 2: Resources (informational) ===== -->
      <div class="setting-group advanced-card">
        <div class="toggle-header">
          <div>
            <span class="setting-label">Resource limits</span>
            <span class="setting-description">
              Max memory {{ draft.max_memory_mb }} MB (informational) · max storage {{ draft.max_storage_gb }} GB per workspace.
              Storage is enforced as best-effort accounting before shell execution (not a hard quota). Memory is deployment-level: the Linux service unit caps the whole service (MemoryMax/MemoryHigh in docs/services); macOS cannot cap per-process memory (measured) — nothing in-process enforces it yet.
            </span>
          </div>
        </div>
      </div>

      <!-- ===== Card 3: Effective state (honest readout from the backend) ===== -->
      <div class="setting-group advanced-card">
        <div class="toggle-header">
          <div>
            <span class="setting-label">Effective enforcement</span>
            <span class="setting-description">
              What the running host actually enforces (backend Effective projection) vs the configured policy above. A surface reported "off" with a reason means the requested enforcement is not active on this host — downgrades are never silent.
            </span>
          </div>
        </div>
        <ul class="effective-list">
          <li><span class="eff-key">Filesystem</span><span>{{ filesystemEffectiveText }}</span></li>
          <li><span class="eff-key">Network (OS layer)</span><span>{{ networkEffectiveText }}</span></li>
          <li><span class="eff-key">Shell sessions</span><span>pooled per (workspace, network on/off)</span></li>
        </ul>
      </div>

      <!-- ===== Card 4: Reset controls ===== -->
      <div class="setting-group advanced-card">
        <div class="toggle-header">
          <div>
            <span class="setting-label">Reset controls</span>
            <span class="setting-description">
              Clear runtime state or reset configuration and secrets to factory defaults. Operates on a fixed allowlist of paths — orchestrator DB, templates, and workspaces are never touched.
            </span>
          </div>
        </div>
        <div class="flex gap-3 mt-3">
          <button class="btn-secondary" :disabled="isResetting" @click="handleClearRuntimeData">
            Clear Runtime Data
          </button>
          <button class="btn-danger" :disabled="isResetting" @click="handleFactoryReset">
            Factory Reset
          </button>
        </div>
        <div class="flex mt-3">
          <button class="btn-wipeout" :disabled="isResetting" @click="handleWipeout">
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
            Settings in effect
          </div>
        </div>
        <div class="flex gap-3">
          <button class="btn-secondary" @click="handleRestartRuntime">Restart Runtime</button>
          <button class="btn-primary" :disabled="!hasChanges || saving" @click="handleSave">
            {{ saving ? 'Saving...' : 'Apply & Save' }}
          </button>
        </div>
      </div>
    </div>

    <!-- Session Monitor -->
    <TerminalMonitor v-if="draft.enabled" />
  </div>
</template>

<style scoped lang="postcss">
.security-card { @apply bg-gray-900 border border-gray-800 rounded-xl overflow-hidden; }
.settings-header { @apply p-6 border-b border-gray-800 bg-gray-900/50; }
.settings-title { @apply text-xl font-medium text-gray-100; }
.settings-subtitle { @apply text-sm text-gray-400 mt-1; }
.settings-content { @apply p-6 space-y-6; }
.loading-state { @apply p-12 text-center text-gray-500 italic; }
.setting-group { @apply space-y-2; }
.setting-label { @apply block text-sm font-medium text-gray-200; }
.setting-description { @apply text-sm text-gray-400 block mt-1; }
.toggle-header { @apply flex items-start justify-between gap-4; }
.advanced-card { @apply p-4 rounded-lg border border-gray-800 bg-gray-900/50 transition-colors duration-300; }
.danger-zone { @apply border-yellow-500/30 bg-yellow-950/10; }
.functional-error { @apply border-red-600 bg-red-900/10; }
.divider { @apply border-t border-gray-800 my-4; }
.egress-row { @apply flex items-center justify-between gap-4 mt-4 pt-4 border-t border-gray-800; }
.port-input { @apply w-28 px-3 py-2 bg-gray-900 border border-gray-700 rounded-lg text-gray-200 text-sm font-mono disabled:opacity-40 disabled:cursor-not-allowed; }
.text-on { @apply text-emerald-400 font-medium; }
.text-off { @apply text-red-400 font-medium; }
.badge-host { @apply text-[10px] uppercase tracking-wider bg-amber-900/40 text-amber-300 border border-amber-700/50 rounded px-1.5 py-0.5 align-middle; }

.alert-box { @apply flex gap-3 p-4 rounded-lg mt-4 border; }
.alert-danger { @apply bg-yellow-950/30 border-yellow-500/30 text-yellow-200; }
.alert-functional { @apply bg-red-900/20 border-red-600/40 text-red-100; }
.alert-amber { @apply bg-amber-950/30 border-amber-600/40 text-amber-100; }
.alert-icon { @apply text-xl; }
.alert-content p { @apply text-sm text-gray-400 mt-1; }
.alert-content strong { @apply block text-sm; }
.alert-actions { @apply flex gap-3 mt-3; }
.btn-sm { @apply px-3 py-1.5 text-xs; }

.effective-list { @apply mt-3 space-y-1.5 list-none; }
.effective-list li { @apply flex items-center justify-between text-sm; }
.eff-key { @apply text-gray-400; }

.btn-primary { @apply px-4 py-2 bg-blue-600 hover:bg-blue-500 disabled:opacity-50 disabled:grayscale text-white text-sm font-bold rounded-lg transition-all active:scale-95 shadow-lg shadow-blue-900/20; }
.btn-secondary { @apply px-4 py-2 bg-gray-800 hover:bg-gray-700 text-gray-200 text-sm font-bold rounded-lg border border-gray-700 transition-all active:scale-95; }
.btn-danger { @apply px-4 py-2 bg-red-700/80 hover:bg-red-600 text-white text-sm font-bold rounded-lg border border-red-500 transition-all active:scale-95 disabled:opacity-50 disabled:grayscale; }
.btn-wipeout { @apply px-4 py-2 bg-red-900 hover:bg-red-800 text-white text-sm font-bold rounded-lg border-2 border-red-600 transition-all active:scale-95 disabled:opacity-50 disabled:grayscale; }
.action-bar { @apply mt-auto; }
</style>
