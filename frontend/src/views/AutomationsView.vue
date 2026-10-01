<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue"
import { useRouter } from "vue-router"
import { useDispatcher } from "../composables/automation/useDispatcher"
import { useAutomationRunner } from "../composables/automation/useAutomationRunner"
import { useWorkspaceHistory } from "../composables/automation/useWorkspaceHistory"
import { useDestinationRoute } from "../composables/ui/useDestinationRoute"
import { usePolling } from "../composables/ui/usePolling"
import { useConfirm } from "../composables/ui/useConfirm"
import { useToast } from "../composables/useToast"
import { DispatcherService } from "../services/automation/dispatcherService"
import { toAutomation, toAutomationEdit, toAutomationNew, toAutomationRecordings, toAutomations } from "../router/routes"
import { automationStatus, triggerLabel } from "../utils/automation/automationDisplay"
import { ROUTE_NAMES } from "../types/routes"
import type { Automation, AutomationRun, RecordingMeta } from "../types/dispatcher"
import type { AutomationPayload } from "../types/automation"

import AutomationList from "../components/AgentIde/automation/AutomationList.vue"
import AutomationForm from "../components/AgentIde/automation/AutomationForm.vue"
import AutomationDetails from "../components/AgentIde/automation/AutomationDetails.vue"
import HistoricalRunDetails from "../components/AgentIde/automation/HistoricalRunDetails.vue"
import RecordingsPanel from "../components/AgentIde/recordings/RecordingsPanel.vue"
import MonitorPanel from "../components/AgentIde/common/MonitorPanel.vue"
import ContextDrawer from "../components/layout/ContextDrawer.vue"
import PageHeader from "../components/common/layout/PageHeader.vue"
import Panel from "../components/common/layout/Panel.vue"
import StatusTag from "../components/common/display/StatusTag.vue"
import BaseButton from "../components/common/buttons/BaseButton.vue"
import ErrorState from "../components/common/feedback/ErrorState.vue"
import LoadingState from "../components/common/feedback/LoadingState.vue"

// Automations (plan D9, D18, Phase 5): managed as a fleet across workspaces.
// One page per route — the list, one automation, new / edit, recordings. A
// run opened from a list shows in a drawer, so the page underneath stays put.

const REFRESH_INTERVAL_MS = 10_000

const router = useRouter()
const current = useDestinationRoute("automations")
const routeName = computed(() => current.value.name)
const automationId = computed(() => {
  const id = current.value.params.id
  return typeof id === "string" ? id : null
})

const {
  automations,
  metrics,
  workspaces,
  workspaceFiles,
  loading,
  fetchAutomations,
  fetchMetrics,
  triggerAutomation,
  fetchWorkspaces,
  fetchWorkspaceTree,
  fetchWorkspaceState,
  fetchGlobalActivity,
  createAutomation,
  updateAutomation,
  deleteAutomation,
  stopAutomation,
  cancelQueued,
  deleteRun,
  deleteAutomationRuns,
} = useDispatcher()
const toast = useToast()
const { confirm } = useConfirm()
const { workspaceHistory, refreshHistory } = useWorkspaceHistory()
const refreshGlobalHistory = () => refreshHistory(null, fetchWorkspaceState, fetchGlobalActivity)

const {
  selectedAutomationId,
  triggering,
  lastTriggerResult,
  selectedAutomation,
  anyRunningInSelectedWorkspace,
  handleTrigger,
  handleReplayRecording,
  handleStopRecording,
  handleStop,
} = useAutomationRunner(automations, triggerAutomation, stopAutomation, refreshGlobalHistory, fetchAutomations)

// Route → selection. Another automation (or none) resets the trigger message.
watch(
  automationId,
  (id) => {
    if (id === selectedAutomationId.value) return
    selectedAutomationId.value = id
    lastTriggerResult.value = null
  },
  { immediate: true },
)

const automationsLoaded = ref(false)
type Page = "list" | "new" | "edit" | "detail" | "recordings" | "missing" | "loading"
const page = computed<Page>(() => {
  switch (routeName.value) {
    case ROUTE_NAMES.automationNew:
      return "new"
    case ROUTE_NAMES.automationRecordings:
      return "recordings"
    case ROUTE_NAMES.automation:
    case ROUTE_NAMES.automationEdit:
      if (selectedAutomation.value) return routeName.value === ROUTE_NAMES.automationEdit ? "edit" : "detail"
      return automationsLoaded.value ? "missing" : "loading"
    default:
      return "list"
  }
})

const monitorOpen = ref(false)
const recordingsEnabled = ref(false)

// ── Runs (drawer) ────────────────────────────────────────────────────────
const openRun = ref<AutomationRun | null>(null)
const runOpen = computed({
  get: () => openRun.value !== null,
  set: (open: boolean) => {
    if (!open) openRun.value = null
  },
})

function showRun(run: AutomationRun) {
  monitorOpen.value = false
  openRun.value = run
}

// The delete composables already surface errors via a banner, so a thrown
// error means "did not happen": no refresh and no view change.
async function handleDeleteRun(run: AutomationRun) {
  try {
    await deleteRun(run)
  } catch {
    return
  }
  if (openRun.value?.id === run.id) openRun.value = null
  void refreshGlobalHistory()
  void fetchAutomations(true)
}

async function handleClearAutomationRuns(auto: { workspace: string; name: string }) {
  if (!auto.workspace || !auto.name) return
  try {
    await deleteAutomationRuns(auto.workspace, auto.name)
  } catch {
    return
  }
  openRun.value = null
  void refreshGlobalHistory()
  void fetchAutomations(true)
}

// ── Automations ──────────────────────────────────────────────────────────
const openByName = (workspace: string, name: string) => {
  const saved = automations.value.find((a) => a.workspace === workspace && a.name === name)
  return router.push(saved ? toAutomation(saved.id) : toAutomations())
}

async function handleCreateAutomation(workspace: string, data: AutomationPayload) {
  try {
    await createAutomation(workspace, data)
  } catch (err) {
    toast.error(`Could not create the automation: ${err instanceof Error ? err.message : err}`)
    return
  }
  await openByName(workspace, data.name)
}

async function handleUpdateAutomation(workspace: string, oldName: string, data: AutomationPayload) {
  try {
    await updateAutomation(workspace, oldName, data)
  } catch (err) {
    toast.error(`Could not save the automation: ${err instanceof Error ? err.message : err}`)
    return
  }
  await openByName(workspace, data.name)
}

function handleCancelForm() {
  void router.push(automationId.value ? toAutomation(automationId.value) : toAutomations())
}

// Select explicitly: the route watcher may not have flushed before triggering.
async function openAndSelect(auto: Automation) {
  await router.push(toAutomation(auto.id))
  selectedAutomationId.value = auto.id
}

async function runAutomation(auto: Automation) {
  await openAndSelect(auto)
  await handleTrigger()
}

async function stopRun(auto: Automation) {
  try {
    await stopAutomation(auto.workspace)
  } finally {
    void fetchAutomations()
  }
}

async function removeAutomation(auto: Automation) {
  try {
    await deleteAutomation(auto.workspace, auto.name)
  } catch {
    return
  }
  if (automationId.value === auto.id) await router.push(toAutomations())
}

async function confirmRemoveSelected() {
  const auto = selectedAutomation.value
  if (!auto) return
  const ok = await confirm({
    title: `Delete ${auto.name}?`,
    message: "Its schedule stops. Past runs stay in Activity until you clear them.",
    type: "error",
    confirmText: "Delete automation",
  })
  if (ok) await removeAutomation(auto)
}

async function handleCancelQueued(auto: Automation) {
  try {
    await cancelQueued(auto.workspace, auto.name)
  } catch {
    // Banner already shown by the composable.
  }
}

async function handleReplay(auto: Automation, recording: RecordingMeta) {
  await openAndSelect(auto)
  await handleReplayRecording(auto, recording)
}

const headerStatus = computed(() => (selectedAutomation.value ? automationStatus(selectedAutomation.value) : null))
const busy = computed(() => anyRunningInSelectedWorkspace.value && !selectedAutomation.value?.is_running)

onMounted(async () => {
  void fetchMetrics()
  void fetchWorkspaces()
  void refreshGlobalHistory()
  DispatcherService.getRecordingStatus()
    .then((status) => {
      recordingsEnabled.value = status.enabled
    })
    .catch((err) => {
      console.error("Failed to check recording status", err)
    })
  await fetchAutomations()
  automationsLoaded.value = true
})
usePolling(() => {
  void refreshGlobalHistory()
  void fetchAutomations(true)
  void fetchMetrics()
}, REFRESH_INTERVAL_MS)
</script>

<template>
  <div class="flex flex-col gap-4">
    <!-- The fleet -->
    <template v-if="page === 'list'">
      <PageHeader eyebrow="Fleet" title="Automations" subtitle="Scheduled and on-demand tasks across every workspace.">
        <template #actions>
          <BaseButton variant="secondary" icon="nav-activity" :aria-expanded="monitorOpen" @click="monitorOpen = true">Monitor</BaseButton>
          <RouterLink v-if="recordingsEnabled" :to="toAutomationRecordings()" class="header-link">Recordings</RouterLink>
          <RouterLink :to="toAutomationNew()" class="header-link header-link--primary offset-brand">+ New automation</RouterLink>
        </template>
      </PageHeader>
      <Panel title="All automations" flush>
        <AutomationList
          :automations="automations"
          :loading="!automationsLoaded"
          @run="runAutomation"
          @stop="stopRun"
          @delete="removeAutomation"
          @cancel-queued="handleCancelQueued"
        />
      </Panel>
    </template>

    <!-- New / edit -->
    <template v-else-if="page === 'new' || page === 'edit'">
      <PageHeader
        :eyebrow="page === 'edit' && selectedAutomation ? `Automations · ${selectedAutomation.workspace}` : 'Automations'"
        :title="page === 'edit' && selectedAutomation ? `Edit ${selectedAutomation.name}` : 'New automation'"
      />
      <AutomationForm
        :workspaces="workspaces"
        :workspaceFiles="workspaceFiles"
        :editAutomation="page === 'edit' ? selectedAutomation : null"
        @create-automation="handleCreateAutomation"
        @update-automation="handleUpdateAutomation"
        @cancel="handleCancelForm"
        @fetch-files="fetchWorkspaceTree"
      />
    </template>

    <!-- One automation -->
    <template v-else-if="page === 'detail' && selectedAutomation">
      <PageHeader :eyebrow="`Automations · ${selectedAutomation.workspace}`" :title="selectedAutomation.name" :subtitle="triggerLabel(selectedAutomation)">
        <template #actions>
          <StatusTag v-if="headerStatus" v-bind="headerStatus" />
          <BaseButton v-if="selectedAutomation.is_running" variant="danger" icon="stop" @click="handleStop">Stop</BaseButton>
          <BaseButton v-else icon="play" :loading="triggering" :disabled="busy" @click="handleTrigger">Run now</BaseButton>
          <RouterLink v-if="!selectedAutomation.is_running && !busy" :to="toAutomationEdit(selectedAutomation.id)" class="header-link">Edit</RouterLink>
          <BaseButton variant="ghost" icon="trash" icon-only :label="`Delete ${selectedAutomation.name}`" :disabled="busy || !!selectedAutomation.is_running" @click="confirmRemoveSelected" />
        </template>
      </PageHeader>
      <p v-if="busy" role="note" class="m-0 text-[length:var(--text-small)] text-muted">
        Another automation is running in {{ selectedAutomation.workspace }}; this one can start once it finishes.
      </p>
      <AutomationDetails
        :key="selectedAutomation.id"
        :automation="selectedAutomation"
        :last-trigger-result="lastTriggerResult"
        :is-executing="triggering || (selectedAutomation.is_running ?? false)"
        @open-run="showRun"
        @delete-run="handleDeleteRun"
        @delete-automation-runs="handleClearAutomationRuns"
      />
    </template>

    <!-- Recordings -->
    <template v-else-if="page === 'recordings'">
      <PageHeader eyebrow="Automations" title="Recordings" subtitle="Replay a recorded run instead of calling the model." />
      <Panel title="Recordings by automation">
        <RecordingsPanel
          :automations="automations"
          :workspaces="[...new Set(automations.map((a) => a.workspace))]"
          @replay-recording="handleReplay"
          @stop-automation="handleStopRecording"
          @show-automation="(id: string) => router.push(toAutomation(id))"
        />
      </Panel>
    </template>

    <LoadingState v-else-if="page === 'loading'" label="Loading automations" />

    <ErrorState v-else :title="`No automation ${automationId}`" cause="It may have been renamed or deleted." next="Pick another automation from the list.">
      <template #action><RouterLink :to="toAutomations()" class="header-link">All automations</RouterLink></template>
    </ErrorState>

    <ContextDrawer v-model:open="runOpen" title="Run details" wide>
      <HistoricalRunDetails
        v-if="openRun"
        :run="openRun"
        embedded
        @delete-run="handleDeleteRun"
        @delete-automation-runs="handleClearAutomationRuns"
      />
    </ContextDrawer>

    <ContextDrawer v-model:open="monitorOpen" title="Monitor">
      <MonitorPanel
        :history="workspaceHistory"
        :loading="loading"
        :metrics="metrics"
        @select-run="showRun"
        @delete-run="handleDeleteRun"
      />
    </ContextDrawer>
  </div>
</template>

<style scoped lang="postcss">
.header-link {
  @apply inline-flex h-[30px] items-center rounded-[var(--radius-sm)] border border-control px-3 font-mono text-[length:var(--text-small)] font-medium text-primary hover:bg-surface-hover focus-visible:outline-none focus-visible:ring-2;
}

.header-link--primary {
  @apply border-text-primary bg-text-primary text-inverse hover:bg-text-secondary;
}
</style>
