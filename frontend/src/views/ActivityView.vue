<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from "vue"
import { useRouter } from "vue-router"
import { useDispatcher } from "../composables/automation/useDispatcher"
import { useLogs } from "../composables/system/useLogs"
import { useLogLevel } from "../composables/system/useMetrics"
import { useDestinationRoute } from "../composables/ui/useDestinationRoute"
import { usePolling } from "../composables/ui/usePolling"
import { useConfirm } from "../composables/ui/useConfirm"
import { toActivity, toWorkspace } from "../router/routes"
import { automationTarget } from "../utils/runs/runTarget"
import { filterRuns, runWorkspaces } from "../utils/runs/filterRuns"
import { formatAbsoluteTime, formatDuration, formatRelativeTime } from "../utils/format/time"
import { POLL_INTERVAL_MS, LOG_LEVELS } from "../constants/api"
import type { AutomationRun } from "../types/dispatcher"
import type { ActivityFilters } from "../types/routes"
import type { ChoiceOption, DataTableColumn } from "../types/ui"
import PageHeader from "../components/common/layout/PageHeader.vue"
import Panel from "../components/common/layout/Panel.vue"
import DataTable from "../components/common/display/DataTable.vue"
import StatusTag from "../components/common/display/StatusTag.vue"
import IdChip from "../components/common/display/IdChip.vue"
import LogViewer from "../components/common/display/LogViewer.vue"
import SegmentedControl from "../components/common/forms/SegmentedControl.vue"
import SearchInput from "../components/common/forms/SearchInput.vue"
import SelectInput from "../components/common/forms/SelectInput.vue"
import BaseButton from "../components/common/buttons/BaseButton.vue"
import EmptyState from "../components/common/feedback/EmptyState.vue"
import LoadingState from "../components/common/feedback/LoadingState.vue"
import ErrorState from "../components/common/feedback/ErrorState.vue"
import ContextDrawer from "../components/layout/ContextDrawer.vue"
import HistoricalRunDetails from "../components/AgentIde/automation/HistoricalRunDetails.vue"

// Activity (plan D9, Phase 5): the automation run ledger across all workspaces
// — searchable, with its filters and the open run in the URL (D18), so a run
// row elsewhere can link straight to its details — and the app and process
// logs. Kept alive (D19): the chosen view survives a trip elsewhere, and the
// run and log polls pause while it is in the background.

type ActivityView = "runs" | "app" | "process"
const VIEWS: ChoiceOption[] = [
  { value: "runs", label: "Runs" },
  { value: "app", label: "App log" },
  { value: "process", label: "Process log" },
]
const STATUS_OPTIONS: ChoiceOption[] = [
  { value: "", label: "Any status" },
  { value: "completed", label: "Completed" },
  { value: "failed", label: "Failed" },
]
const LEVEL_OPTIONS: ChoiceOption[] = LOG_LEVELS.map((lvl) => ({ value: lvl, label: lvl }))
const SEARCH_DEBOUNCE_MS = 250
const ALL_WORKSPACES = ""

const router = useRouter()
const current = useDestinationRoute("activity")
const { fetchGlobalActivity, deleteRun, deleteAutomationRuns } = useDispatcher()
const { confirm } = useConfirm()

const view = ref<ActivityView>("runs")

// ── Filters (route query) ────────────────────────────────────────────────
const queryText = (key: keyof ActivityFilters) => {
  const value = current.value.query[key]
  return typeof value === "string" ? value : ""
}
const filters = computed<ActivityFilters>(() => ({
  status: queryText("status"),
  workspace: queryText("workspace"),
  q: queryText("q"),
}))
const hasFilters = computed(() => Object.values(filters.value).some(Boolean))

function setFilters(next: ActivityFilters) {
  void router.replace(toActivity({ ...filters.value, ...next }))
}

// Search is typed locally and written to the URL once the user pauses.
const search = ref(filters.value.q ?? "")
let searchTimer: ReturnType<typeof setTimeout> | null = null
watch(search, (q) => {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(() => setFilters({ q }), SEARCH_DEBOUNCE_MS)
})
watch(() => filters.value.q, (q) => {
  if ((q ?? "") !== search.value) search.value = q ?? ""
})
onUnmounted(() => {
  if (searchTimer) clearTimeout(searchTimer)
})

function clearFilters() {
  search.value = ""
  void router.replace(toActivity())
}

// ── Runs ─────────────────────────────────────────────────────────────────
const runs = ref<AutomationRun[]>([])
const runsLoading = ref(true)
const runsError = ref<string | null>(null)

async function loadRuns() {
  runsError.value = null
  try {
    runs.value = await fetchGlobalActivity()
  } catch (e) {
    runsError.value = e instanceof Error ? e.message : "The run history could not be read."
  } finally {
    runsLoading.value = false
  }
}
void loadRuns()
usePolling(() => void loadRuns(), POLL_INTERVAL_MS)

const visibleRuns = computed(() => filterRuns(runs.value, filters.value))
const workspaceOptions = computed<ChoiceOption[]>(() => [
  { value: ALL_WORKSPACES, label: "All workspaces" },
  ...runWorkspaces(runs.value).map((ws) => ({ value: ws, label: ws })),
])

const RUN_COLUMNS: DataTableColumn<AutomationRun>[] = [
  { key: "finished", label: "Finished" },
  { key: "automation", label: "Automation" },
  { key: "workspace", label: "Workspace", value: (run) => run.workspace_id },
  { key: "status", label: "Status" },
  { key: "duration", label: "Duration", numeric: true, value: (run) => formatDuration(run.duration_ms) },
  { key: "model", label: "Model", value: (run) => run.model || "default" },
  { key: "id", label: "ID" },
]
const runKey = (run: AutomationRun) => run.id

// Run details open in the drawer, addressed by ?run= so they can be linked to;
// deleting there refreshes the list. A linked run past the kept history says so.
const openRunId = computed(() => queryText("run"))
const selectedRun = computed(() => runs.value.find((run) => run.id === openRunId.value) ?? null)

// Kept alive, the list can be older than a run linked from elsewhere (a run
// that just finished): read the ledger once more before calling it missing.
const fetchingLinkedRun = ref(false)
watch(openRunId, async (id) => {
  if (!id || runsLoading.value || selectedRun.value) return
  fetchingLinkedRun.value = true
  try {
    await loadRuns()
  } finally {
    fetchingLinkedRun.value = false
  }
})
const linkedRunMissing = computed(() => !!openRunId.value && !runsLoading.value && !fetchingLinkedRun.value && !selectedRun.value)

function openRun(run: AutomationRun) {
  void router.replace(toActivity({ ...filters.value, run: run.id }))
}
function closeRun() {
  void router.replace(toActivity(filters.value))
}
const detailsOpen = computed({
  get: () => !!openRunId.value,
  set: (open: boolean) => {
    if (!open) closeRun()
  },
})

// The delete composables surface errors via a banner; a thrown error means
// "did not happen", so the drawer stays open.
async function onDeleteRun(run: AutomationRun) {
  try {
    await deleteRun(run)
  } catch {
    return
  }
  closeRun()
  void loadRuns()
}

async function onDeleteAutomationRuns(auto: { workspace: string; name: string }) {
  try {
    await deleteAutomationRuns(auto.workspace, auto.name)
  } catch {
    return
  }
  closeRun()
  void loadRuns()
}

// ── Logs ─────────────────────────────────────────────────────────────────
const {
  processLogLines,
  processLogRunning,
  processLogName,
  processLogReady,
  appLogLines,
  appLogsFetched,
  appLogsActive,
  clearProcessLogs,
  clearAppLogs,
} = useLogs()
const { logLevel, updateLogLevel } = useLogLevel()

// The app log is fetched only while it is on screen.
watch(view, (v) => {
  appLogsActive.value = v === "app"
}, { immediate: true })

async function clearLog() {
  const isApp = view.value === "app"
  const ok = await confirm({
    title: isApp ? "Clear the app log?" : "Clear the process log?",
    message: "The lines shown here are deleted. New lines keep arriving.",
    type: "warning",
    confirmText: "Clear log",
  })
  if (!ok) return
  if (isApp) await clearAppLogs()
  else await clearProcessLogs()
}
</script>

<template>
  <div class="flex flex-col gap-4">
    <PageHeader eyebrow="History" title="Activity" subtitle="Automation runs and logs across all workspaces">
      <template #actions>
        <SegmentedControl :model-value="view" :options="VIEWS" label="Activity view" @update:model-value="view = $event as ActivityView" />
      </template>
    </PageHeader>

    <Panel v-if="view === 'runs'" title="Runs" flush>
      <template #actions>
        <SelectInput :model-value="filters.status ?? ''" :options="STATUS_OPTIONS" label="Status" @update:model-value="setFilters({ status: $event })" />
        <SelectInput :model-value="filters.workspace ?? ''" :options="workspaceOptions" label="Workspace" @update:model-value="setFilters({ workspace: $event })" />
        <SearchInput v-model="search" label="Search runs" />
      </template>

      <ErrorState v-if="runsError && !runs.length" title="Could not load run history" :cause="runsError" next="The server may be busy loading a model. Retry in a moment.">
        <template #action><BaseButton variant="secondary" icon="refresh" @click="loadRuns">Retry</BaseButton></template>
      </ErrorState>
      <DataTable
        v-else
        :columns="RUN_COLUMNS"
        :rows="visibleRuns"
        :row-key="runKey"
        caption="Automation runs"
        :loading="runsLoading"
        activatable
        @activate="openRun"
      >
        <template #empty>
          <EmptyState
            v-if="hasFilters"
            title="No runs match these filters"
            body="Clear the filters to see every run (the last 100 are kept)."
          >
            <template #action><BaseButton variant="secondary" @click="clearFilters">Clear filters</BaseButton></template>
          </EmptyState>
          <EmptyState v-else title="No runs yet" body="Automation runs appear here as they finish." />
        </template>
        <template #cell-finished="{ row }">
          <time :datetime="row.timestamp" :title="formatAbsoluteTime(row.timestamp)" class="whitespace-nowrap font-mono text-[length:var(--text-small)] text-muted">{{ formatRelativeTime(row.timestamp) }}</time>
        </template>
        <template #cell-automation="{ row }">
          <!-- One block, so the stacked mobile card keeps name and error together. -->
          <span class="block min-w-0">
            <RouterLink
              v-if="automationTarget(row)"
              :to="automationTarget(row)!"
              :aria-label="`Open automation ${row.automation_name}`"
              class="block rounded-[var(--radius-sm)] text-primary underline-offset-2 hover:underline focus-visible:outline-none focus-visible:ring-2"
            >{{ row.automation_name }}</RouterLink>
            <span v-else class="block text-primary">{{ row.automation_name }}</span>
            <span v-if="row.error" class="block max-w-[48ch] truncate font-mono text-[length:var(--text-micro)] text-state-error" :title="row.error">{{ row.error }}</span>
          </span>
        </template>
        <template #cell-workspace="{ row }">
          <RouterLink
            v-if="row.workspace_id"
            :to="toWorkspace(row.workspace_id)"
            :aria-label="`Open workspace ${row.workspace_id}`"
            class="rounded-[var(--radius-sm)] font-mono text-[length:var(--text-small)] underline-offset-2 hover:text-primary hover:underline focus-visible:outline-none focus-visible:ring-2"
          >{{ row.workspace_id }}</RouterLink>
        </template>
        <template #cell-status="{ row }">
          <StatusTag :state="row.error ? 'error' : 'success'" :label="row.error ? 'Failed' : 'Completed'" />
        </template>
        <template #cell-id="{ row }">
          <span @click.stop><IdChip :id="row.id" /></span>
        </template>
      </DataTable>
    </Panel>

    <Panel v-else :title="view === 'app' ? 'App log' : 'Process log'">
      <template #actions>
        <template v-if="view === 'process'">
          <StatusTag v-if="processLogRunning" :state="processLogReady ? 'success' : 'running'" :label="processLogReady ? 'Ready' : 'Starting'" />
          <StatusTag v-else state="neutral" label="Stopped" />
          <span v-if="processLogRunning" class="font-mono text-[length:var(--text-small)] text-primary">{{ processLogName }}</span>
        </template>
        <SegmentedControl :model-value="logLevel" :options="LEVEL_OPTIONS" label="Log level" @update:model-value="updateLogLevel" />
        <BaseButton variant="ghost" size="sm" icon="trash" @click="clearLog">Clear log</BaseButton>
      </template>
      <LogViewer
        v-if="view === 'app'"
        :text="appLogLines"
        label="App log lines"
        :empty-text="appLogsFetched ? 'No application log lines yet.' : 'Loading the application log…'"
      />
      <LogViewer
        v-else
        :text="processLogLines"
        label="Process log lines"
        :empty-text="processLogRunning ? 'The process has not logged anything yet.' : 'No model process is running.'"
      />
    </Panel>

    <ContextDrawer v-model:open="detailsOpen" title="Run details" wide>
      <HistoricalRunDetails
        v-if="selectedRun"
        :run="selectedRun"
        embedded
        @delete-run="onDeleteRun"
        @delete-automation-runs="onDeleteAutomationRuns"
      />
      <ErrorState v-else-if="runsError" title="Could not load the run" :cause="runsError" next="The run history is retried automatically." />
      <EmptyState
        v-else-if="linkedRunMissing"
        title="This run is no longer in the run history"
        body="Only the most recent runs are kept; it may also have been deleted."
      />
      <LoadingState v-else label="Loading the run" :rows="3" />
    </ContextDrawer>
  </div>
</template>
