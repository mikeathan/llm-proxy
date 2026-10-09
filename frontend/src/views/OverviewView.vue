<script setup lang="ts">
import { computed, onMounted, ref } from "vue"
import { useModels } from "../composables/models/useModels"
import { useMetrics } from "../composables/system/useMetrics"
import { summariseHost } from "../domain/hostStats"
import { useGlobalRunActivity } from "../composables/assistant/useGlobalRunActivity"
import { useDispatcher } from "../composables/automation/useDispatcher"
import { toAutomations, toModels } from "../router/routes"
import { historicalRunTarget, runTarget, runTitle } from "../utils/runs/runTarget"
import { formatAbsoluteTime, formatDuration, formatRelativeTime } from "../utils/format/time"
import type { AutomationRun } from "../types/dispatcher"
import { LANE_LABELS, RUN_KIND_LABELS } from "../constants/runs"
import type { DataTableColumn } from "../types/ui"
import PageHeader from "../components/common/layout/PageHeader.vue"
import Panel from "../components/common/layout/Panel.vue"
import StatCard from "../components/common/display/StatCard.vue"
import Sparkline from "../components/common/display/Sparkline.vue"
import Meter from "../components/common/display/Meter.vue"
import SlotBar from "../components/common/display/SlotBar.vue"
import StatusTag from "../components/common/display/StatusTag.vue"
import { runOutcomeTag } from "../utils/automation/runOutcome"
import RunRow from "../components/common/display/RunRow.vue"
import DataTable from "../components/common/display/DataTable.vue"
import BaseButton from "../components/common/buttons/BaseButton.vue"
import LoadingState from "../components/common/feedback/LoadingState.vue"
import EmptyState from "../components/common/feedback/EmptyState.vue"
import ErrorState from "../components/common/feedback/ErrorState.vue"

// Overview (plan D9, Phase 5): health, what is running now, and recent runs.
// Throughput and CPU history is the client's own (the backend keeps none, V8),
// so it is labelled "since page load".

const MB_PER_GB = 1024
const VRAM_BUSY_PERCENT = 75
const RECENT_RUNS = 5
const HISTORY_CAPTION = "since page load"
const NO_VALUE = "—"

const { activeModel, stopModel } = useModels()
const { metrics, history } = useMetrics()
const { laneHolders, queuedRuns, lanes, error: laneError } = useGlobalRunActivity()
const { fetchGlobalActivity } = useDispatcher()

const oneDecimal = (n: number) => n.toFixed(1)
const tpsHistory = computed(() => history.value.map((s) => Number(oneDecimal(s.tokensPerSecond))))
const loadHistory = computed(() => history.value.map((s) => Math.round(s.loadPercent)))

// One derivation of the host figures, shared with the header strip (HostStats).
const host = computed(() => (metrics.value ? summariseHost(metrics.value) : null))
const memory = computed(() => {
  const m = metrics.value
  if (!m || !host.value) return null
  return {
    used: oneDecimal(m.mem_used_mb / MB_PER_GB),
    total: oneDecimal(m.mem_total_mb / MB_PER_GB),
    percent: host.value.memory.percent,
  }
})
const gpu = computed(() => {
  const g = host.value?.gpu
  if (!g) return null
  const hasTemperature = g.temperatureC !== null
  return {
    name: g.name,
    temperature: hasTemperature ? String(Math.round(g.temperatureC as number)) : NO_VALUE,
    temperatureCaption: hasTemperature ? undefined : "Temperature not reported",
    vramPercent: g.vramPercent,
    vramText: g.vramText,
  }
})

const nothingRunning = computed(() => !laneHolders.value.length && !queuedRuns.value.length)
// Re-evaluated whenever the lane poll replaces the holder list, so it stays
// within one poll interval without a clock of its own.
const runningFor = (since: string) => formatDuration(Math.max(0, Date.now() - Date.parse(since)))

// ── Recent runs ──────────────────────────────────────────────────────────
const runs = ref<AutomationRun[]>([])
const runsLoading = ref(true)
const runsError = ref<string | null>(null)

async function loadRuns() {
  runsLoading.value = true
  runsError.value = null
  try {
    const all = await fetchGlobalActivity()
    runs.value = [...all].sort((a, b) => Date.parse(b.timestamp) - Date.parse(a.timestamp)).slice(0, RECENT_RUNS)
  } catch (e) {
    runsError.value = e instanceof Error ? e.message : "The run history could not be read."
  } finally {
    runsLoading.value = false
  }
}
onMounted(loadRuns)

const runLabel = (run: AutomationRun) => (run.workspace_id ? `${run.workspace_id}/${run.automation_name}` : run.automation_name)
const RUN_COLUMNS: DataTableColumn<AutomationRun>[] = [
  { key: "time", label: "When" },
  { key: "run", label: "Automation", value: runLabel },
  { key: "status", label: "Status" },
  { key: "duration", label: "Duration", numeric: true, value: (run) => formatDuration(run.duration_ms) },
]
const runKey = (run: AutomationRun) => run.id
</script>

<template>
  <div class="flex flex-col gap-4">
    <PageHeader eyebrow="System health" title="Overview" subtitle="Health, running work and recent runs" />

    <Panel title="Health">
      <template #actions>
        <template v-if="activeModel">
          <StatusTag :state="activeModel.ready ? 'success' : 'running'" :label="activeModel.ready ? 'Ready' : 'Loading'" />
          <span class="max-w-[24ch] truncate font-mono text-[length:var(--text-small)] text-primary" :title="activeModel.name">{{ activeModel.name }}</span>
          <BaseButton variant="ghost" size="sm" icon="stop" @click="stopModel">
            {{ activeModel.provider === "local" ? "Stop model" : "Deselect model" }}
          </BaseButton>
        </template>
        <template v-else>
          <StatusTag state="neutral" label="No model loaded" />
          <RouterLink :to="toModels()" class="font-mono text-[length:var(--text-small)] text-accent-info-text hover:underline focus-visible:outline-none focus-visible:ring-2">Load a model</RouterLink>
        </template>
      </template>

      <LoadingState v-if="!metrics" label="Loading health" :rows="3" />
      <div v-else class="grid grid-cols-[repeat(auto-fit,minmax(min(100%,200px),1fr))] gap-px border border-hairline bg-border-hairline">
        <StatCard label="Throughput" :value="oneDecimal(metrics.llm_tokens_per_sec ?? 0)" unit="tok/s" :caption="HISTORY_CAPTION">
          <Sparkline :values="tpsHistory" label="Tokens per second" unit="tok/s" state="success" />
        </StatCard>
        <StatCard label="CPU load" :value="String(Math.round(metrics.load_percent ?? 0))" unit="%" :caption="HISTORY_CAPTION">
          <Sparkline :values="loadHistory" label="CPU load" unit="%" state="info" />
        </StatCard>
        <StatCard v-if="memory" label="Memory" :value="memory.used" :unit="`/ ${memory.total} GB`">
          <Meter label="Memory used" :value="memory.percent" />
        </StatCard>
        <StatCard v-if="gpu" :label="gpu.name" :value="gpu.temperature" :unit="gpu.temperatureCaption ? undefined : '°C'" :caption="gpu.temperatureCaption">
          <Meter label="VRAM used" :value="gpu.vramPercent" :text="gpu.vramText" :state="gpu.vramPercent > VRAM_BUSY_PERCENT ? 'running' : 'info'" />
        </StatCard>
        <StatCard v-else label="GPU" :value="NO_VALUE" :caption="metrics.gpu_error || 'No GPU detected'" />
      </div>
    </Panel>

    <div class="grid grid-cols-[repeat(auto-fit,minmax(min(100%,420px),1fr))] gap-4">
      <Panel title="Running now">
        <div v-if="lanes.length && !laneError" class="mb-4 grid grid-cols-[repeat(auto-fit,minmax(min(100%,160px),1fr))] gap-4">
          <SlotBar v-for="lane in lanes" :key="lane.lane" :label="`${LANE_LABELS[lane.lane] ?? lane.lane} lane`" :used="lane.running" :limit="lane.limit" />
        </div>
        <ErrorState
          v-if="laneError && nothingRunning"
          title="Could not read running work"
          :cause="laneError"
          next="It is retried automatically every few seconds."
        />
        <EmptyState
          v-else-if="nothingRunning"
          title="Nothing is running"
          body="Automations, chats and API callers appear here while they hold a model slot."
        />
        <ul v-else class="m-0 flex list-none flex-col gap-2 p-0">
          <li v-for="holder in laneHolders" :key="holder.key" class="flex min-w-0">
            <RunRow
              :state="holder.kind === 'inbound' ? 'info' : 'running'"
              :tag="RUN_KIND_LABELS[holder.kind]"
              :title="runTitle(holder)"
              :note="runningFor(holder.since)"
              :to="runTarget(holder)"
            />
          </li>
          <li v-for="entry in queuedRuns" :key="entry.key" class="flex min-w-0">
            <RunRow state="queued" :tag="`Queued #${entry.position}`" :title="runTitle(entry)" :to="runTarget(entry)" muted />
          </li>
        </ul>
      </Panel>

      <Panel title="Recent runs" flush>
        <template #actions>
          <RouterLink :to="toAutomations()" class="font-mono text-[length:var(--text-small)] text-muted hover:text-primary focus-visible:outline-none focus-visible:ring-2">All automations →</RouterLink>
        </template>
        <DataTable
          :columns="RUN_COLUMNS"
          :rows="runs"
          :row-key="runKey"
          caption="Recent runs"
          :loading="runsLoading"
          :error="runsError"
          empty-title="No runs yet"
          empty-body="Automation runs appear here as they finish."
        >
          <template #cell-time="{ row }">
            <time :datetime="row.timestamp" :title="formatAbsoluteTime(row.timestamp)" class="font-mono text-[length:var(--text-small)] text-muted">{{ formatRelativeTime(row.timestamp) }}</time>
          </template>
          <template #cell-run="{ row }">
            <RouterLink
              :to="historicalRunTarget(row)"
              :aria-label="`Open run ${runLabel(row)}`"
              class="rounded-[var(--radius-sm)] font-mono text-[length:var(--text-small)] text-primary underline-offset-2 hover:underline focus-visible:outline-none focus-visible:ring-2"
            >{{ runLabel(row) }}</RouterLink>
          </template>
          <template #cell-status="{ row }">
            <StatusTag v-bind="runOutcomeTag(row)" />
          </template>
        </DataTable>
      </Panel>
    </div>
  </div>
</template>
