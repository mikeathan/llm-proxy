<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import BaseToggle from '../common/buttons/BaseToggle.vue'
import BaseButton from '../common/buttons/BaseButton.vue'
import Panel from '../common/layout/Panel.vue'
import FormField from '../common/forms/FormField.vue'
import StatusTag from '../common/display/StatusTag.vue'
import Callout from '../common/feedback/Callout.vue'
import LoadingState from '../common/feedback/LoadingState.vue'
import SettingsActions from './SettingsActions.vue'
import TerminalMonitor from './TerminalMonitor.vue'
import { AdminApiService } from '../../services/admin/adminService'
import { useToast } from '../../composables/useToast'
import { useConfirm } from '../../composables/ui/useConfirm'
import { useHostSandboxingEditor } from '../../composables/settings/useHostSandboxingEditor'
import { errorMessage } from '../../utils/errors'
import { formatSandboxSurface, sandboxSurfaceShort } from '../../utils/sandboxing'
import type { SandboxSurface } from '../../types/admin'
import type { StatusState } from '../../types/ui'

// Settings · Security & sandboxing: the host-wide containment policy (its own
// draft and save — host settings, not the shared configuration), what the host
// actually enforces, live terminal sessions and the reset controls.
// `active` is whether this section is showing: the session monitor polls only
// then. The page's unsaved-change guard learns about the draft via dirty-change.
const props = defineProps<{ active: boolean }>()
const emit = defineEmits<{ (e: 'dirty-change', dirty: boolean): void }>()

const toast = useToast()
const { confirm: confirmDialog } = useConfirm()
const isResetting = ref(false)

// Form state, dirty tracking and persistence live in the editor composable —
// the component only renders it and owns the page-level destructive actions.
const editor = useHostSandboxingEditor()
const { draft, effective, hasChanges, networkAllowed, networkDecided, filesystemOn, loading, saving, saveError } = editor

watch(hasChanges, (dirty) => emit('dirty-change', dirty), { immediate: true })

onMounted(async () => {
  await editor.load()
  if (editor.loadError.value) toast.error(`Could not load the security settings: ${editor.loadError.value}`)
})

// ---- Effective state readouts -------------------------------------------------
// Tags next to a switch prefer the backend Effective projection (what this
// host actually enforces) and fall back to the configured posture, labelled as
// such, until the backend reports. The Effective panel always shows the
// reason, so a downgrade is never silent.

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
const filesystemState = computed<StatusState>(() =>
  (effective.value ? surfaceEnforced(effective.value.filesystem) : filesystemOn.value) ? 'success' : 'error',
)
const networkState = computed<StatusState>(() => (networkAllowed.value ? 'success' : 'error'))
const effectiveRows = computed(() => [
  { key: 'Filesystem', value: formatSandboxSurface(effective.value?.filesystem) },
  { key: 'Network (OS layer)', value: formatSandboxSurface(effective.value?.network) },
  { key: 'Shell sessions', value: 'pooled per (workspace, network on/off)' },
])

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
  if (ok) toast.success('Security settings saved')
}

// ---- Page-level actions (destructive; backend service owns the calls) ---------
const handleClearRuntimeData = async () => {
  const ok = await confirmDialog({
    title: 'Clear runtime data?',
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
    toast.error(`Could not clear runtime data: ${errorMessage(e)}`)
  } finally {
    isResetting.value = false
  }
}

const handleFactoryReset = async () => {
  const ok = await confirmDialog({
    title: 'Factory reset?',
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
    title: 'Wipe out (uninstall)?',
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
  <div class="flex flex-col gap-4">
    <LoadingState v-if="loading" label="Loading security settings" />

    <template v-else>
      <Panel title="Sandboxing">
        <div class="flex flex-col divide-y divide-hairline">
          <div class="switch-row">
            <div class="min-w-0">
              <p class="switch-title">Sandboxing master</p>
              <p class="switch-copy">
                Persistent-terminal runtime switch. Off = agent commands run one-shot (no session state) and startup logs a warning; the service still boots. The switches below are the containment switches that gate agent work.
              </p>
            </div>
            <BaseToggle :model-value="draft.enabled" label="Sandboxing master" hide-label @update:model-value="toggleMaster" />
          </div>

          <div v-if="!draft.enabled || !draft.functional" class="py-3">
            <Callout v-if="!draft.enabled" tone="warning" title="Persistent terminals disabled">
              Agent terminal commands fall back to one-shot execution without session state. The filesystem and network switches below still apply — turn the master back on for full agentic execution.
            </Callout>
            <Callout v-else tone="error" title="Terminal provider error">
              The terminal provider failed to start, so single-shot execution is used. Check the app log in Activity.
            </Callout>
          </div>

          <div class="switch-row">
            <div class="min-w-0">
              <p class="switch-title">Confine agent files to workspaces</p>
              <p class="switch-copy">
                On (default): on Linux, agent processes are kernel-confined to their workspace files (workspace↔workspace isolation; host secrets unreadable). On macOS/dev no OS jail is active yet — production there uses the dedicated-user launchd deployment. Applies on restart.
              </p>
              <StatusTag class="mt-2" :state="filesystemState" :label="`Effective: ${filesystemPill}`" />
            </div>
            <BaseToggle :model-value="filesystemOn" label="Confine agent files to workspaces" hide-label @update:model-value="toggleFilesystem" />
          </div>

          <div class="switch-row">
            <div class="min-w-0">
              <p class="switch-title">Allow agent network <span class="ml-2 font-mono text-[length:var(--text-micro)] uppercase tracking-[var(--tracking-micro)] text-muted">Host master</span></p>
              <p class="switch-copy">
                Off = network-capable agent tools stop everywhere (fetch/scan/search/connector sends; inbound receipt unaffected). On = permitted; scope is granted per workspace / automation.
                Terminal processes are not yet OS-blocked on every host — enable the egress proxy for domain-level filtering and keep untrusted workspaces restricted.
              </p>
              <StatusTag class="mt-2" :state="networkState" :label="`Effective: ${networkPill}`" />
            </div>
            <BaseToggle :model-value="networkAllowed" label="Allow agent network" hide-label @update:model-value="setNetwork" />
          </div>

          <!-- Migration banner: key absent = undecided (legacy allowed) -->
          <div v-if="draft.enabled && !networkDecided" class="py-3">
            <Callout tone="warning" title="Agent network is unrestricted (legacy default)">
              This install predates the network switch. Until you choose, any workspace or automation the guardrails permit can use the network.
              Restricting is recommended — you can turn the host switch back on here at any time; per-workspace and per-automation grants only apply while it is on.
              <template #actions>
                <BaseButton variant="primary" size="sm" @click="restrictNetwork">Restrict network</BaseButton>
                <BaseButton variant="secondary" size="sm" @click="keepNetworkAllowed">Keep allowed</BaseButton>
              </template>
            </Callout>
          </div>

          <div class="pt-3">
            <FormField
              label="Egress proxy port"
              hint="0 = off. When set, agent tool egress and network-on shell clients go through a loopback HTTP proxy that enforces the host domain policy (egress_allow_domains / egress_deny_domains in settings.yml). Applies on restart."
            >
              <template #default="{ id, describedBy }">
                <input
                  :id="id"
                  v-model.number="draft.egress_proxy"
                  :aria-describedby="describedBy"
                  type="number"
                  min="0"
                  max="65535"
                  class="form-control w-28 font-mono tabular-nums"
                  :disabled="!draft.enabled"
                />
              </template>
            </FormField>
          </div>
        </div>
      </Panel>

      <SettingsActions :dirty="hasChanges" :saving="saving" :error="saveError ? `Could not save the security settings: ${saveError}` : ''" save-label="Save security settings" @save="handleSave" @discard="editor.discard" />

      <Panel title="Resource limits">
        <p class="m-0 max-w-[72ch] text-[length:var(--text-small)] text-secondary">
          Max memory <span class="font-mono tabular-nums text-primary">{{ draft.max_memory_mb }} MB</span> (informational) · max storage
          <span class="font-mono tabular-nums text-primary">{{ draft.max_storage_gb }} GB</span> per workspace.
          Storage is enforced as best-effort accounting before shell execution (not a hard quota). Memory is deployment-level: the Linux service unit caps the whole service (MemoryMax/MemoryHigh in docs/services); macOS cannot cap per-process memory (measured) — nothing in-process enforces it yet.
        </p>
      </Panel>

      <Panel title="Effective enforcement">
        <p class="mb-3 mt-0 max-w-[72ch] text-[length:var(--text-small)] text-muted">
          What the running host actually enforces, next to the policy above. A surface reported "off" with a reason means the requested enforcement is not active on this host — downgrades are never silent.
        </p>
        <dl class="m-0 grid grid-cols-[max-content_minmax(0,1fr)] gap-x-6 gap-y-1.5 text-[length:var(--text-small)]">
          <template v-for="row in effectiveRows" :key="row.key">
            <dt class="text-muted">{{ row.key }}</dt>
            <dd class="m-0 font-mono text-primary">{{ row.value }}</dd>
          </template>
        </dl>
      </Panel>

      <TerminalMonitor v-if="props.active && draft.enabled" />

      <Panel title="Reset controls">
        <p class="mb-3 mt-0 max-w-[72ch] text-[length:var(--text-small)] text-muted">
          Clear runtime state or reset configuration and secrets to factory defaults. Operates on a fixed allowlist of paths — the orchestrator database, templates and workspaces are never touched, except by a wipeout.
        </p>
        <div class="flex flex-wrap gap-2">
          <BaseButton variant="secondary" :disabled="isResetting" @click="handleClearRuntimeData">Clear runtime data</BaseButton>
          <BaseButton variant="danger" :disabled="isResetting" @click="handleFactoryReset">Factory reset</BaseButton>
          <BaseButton variant="danger" :disabled="isResetting" @click="handleWipeout">Wipeout (uninstall)</BaseButton>
        </div>
      </Panel>
    </template>
  </div>
</template>

<style scoped lang="postcss">
.switch-row {
  @apply flex items-start justify-between gap-4 py-3 first:pt-0;
}
.switch-title {
  @apply m-0 text-[length:var(--text-small)] font-medium text-primary;
}
.switch-copy {
  @apply mb-0 mt-1 max-w-[72ch] text-[length:var(--text-small)] text-muted;
}
</style>
