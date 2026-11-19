<script setup lang="ts">
import { onMounted, ref } from "vue"
import DataTable from "../../common/display/DataTable.vue"
import StatusTag from "../../common/display/StatusTag.vue"
import { CARD_LIST_SCROLL } from "../../../constants/dashboard"
import { useCardSize } from "../../../composables/dashboard/useCardSize"
import { useDispatcher } from "../../../composables/automation/useDispatcher"
import { historicalRunTarget } from "../../../utils/runs/runTarget"
import { runOutcomeTag } from "../../../utils/automation/runOutcome"
import { formatAbsoluteTime, formatDuration, formatRelativeTime } from "../../../utils/format/time"
import type { AutomationRun } from "../../../types/dispatcher"
import type { DataTableColumn } from "../../../types/ui"

// Dashboard card · Recent runs: the last automation runs, newest first. Small is the last run; medium a list that scrolls; large the table; both scroll inside the card, so every kept run can be reached at any size.
const RECENT_RUNS = 20
const size = useCardSize()
const { fetchGlobalActivity } = useDispatcher()
const runs = ref<AutomationRun[]>([])
const loading = ref(true)
const error = ref<string | null>(null)

async function load() {
  loading.value = true
  error.value = null
  try {
    const all = await fetchGlobalActivity()
    runs.value = [...all].sort((a, b) => Date.parse(b.timestamp) - Date.parse(a.timestamp)).slice(0, RECENT_RUNS)
  } catch (e) {
    error.value = e instanceof Error ? e.message : "The run history could not be read."
  } finally {
    loading.value = false
  }
}
onMounted(load)

const runLabel = (run: AutomationRun) => (run.workspace_id ? `${run.workspace_id}/${run.automation_name}` : run.automation_name)
const COLUMNS: DataTableColumn<AutomationRun>[] = [
  { key: "time", label: "When" },
  { key: "run", label: "Automation", value: runLabel },
  { key: "status", label: "Status" },
  { key: "duration", label: "Duration", numeric: true, value: (run) => formatDuration(run.duration_ms) },
]
const runKey = (run: AutomationRun) => run.id
</script>

<template>
  <div data-test="recent-runs-card">
    <p v-if="error" role="alert" class="m-0 text-state-error">{{ error }}</p>
    <template v-else>
      <div v-if="size === 'xs'">
        <template v-if="runs[0]">
          <StatusTag v-bind="runOutcomeTag(runs[0])" />
          <p class="m-0 mt-1 truncate text-primary">{{ runLabel(runs[0]) }}</p>
          <p class="m-0 text-[length:var(--text-small)] text-muted">{{ formatRelativeTime(runs[0].timestamp) }}</p>
        </template>
        <p v-else-if="!loading" class="m-0 text-muted">No runs yet</p>
      </div>
      <ul v-else-if="size === 'sm'" :class="['m-0 list-none p-0', CARD_LIST_SCROLL[size]]">
        <li v-for="run in runs" :key="run.id" class="flex flex-wrap items-center gap-2 border-b border-hairline/60 py-1.5 last:border-b-0">
          <StatusTag v-bind="runOutcomeTag(run)" />
          <RouterLink :to="historicalRunTarget(run)" class="min-w-0 flex-1 truncate font-mono text-[length:var(--text-small)] text-primary hover:underline focus-visible:outline-none focus-visible:ring-2">{{ runLabel(run) }}</RouterLink>
          <span class="text-[length:var(--text-small)] text-muted">{{ formatRelativeTime(run.timestamp) }}</span>
        </li>
        <li v-if="!runs.length && !loading" class="text-muted">No runs yet</li>
      </ul>
      <div v-else :class="CARD_LIST_SCROLL[size]" data-test="recent-runs-table">
        <DataTable :columns="COLUMNS" :rows="runs" :row-key="runKey" caption="Recent runs" :loading="loading" :error="error" empty-title="No runs yet" empty-body="Automation runs appear here as they finish.">
          <template #cell-time="{ row }"><time :datetime="row.timestamp" :title="formatAbsoluteTime(row.timestamp)" class="font-mono text-[length:var(--text-small)] text-muted">{{ formatRelativeTime(row.timestamp) }}</time></template>
          <template #cell-run="{ row }"><RouterLink :to="historicalRunTarget(row)" :aria-label="`Open run ${runLabel(row)}`" class="rounded-[var(--radius-sm)] font-mono text-[length:var(--text-small)] text-primary underline-offset-2 hover:underline focus-visible:outline-none focus-visible:ring-2">{{ runLabel(row) }}</RouterLink></template>
          <template #cell-status="{ row }"><StatusTag v-bind="runOutcomeTag(row)" /></template>
        </DataTable>
      </div>
    </template>
  </div>
</template>
