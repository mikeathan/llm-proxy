<script setup lang="ts">
import { computed } from "vue"
import StatCard from "../../common/display/StatCard.vue"
import Sparkline from "../../common/display/Sparkline.vue"
import Meter from "../../common/display/Meter.vue"
import StatusTag from "../../common/display/StatusTag.vue"
import LoadingState from "../../common/feedback/LoadingState.vue"
import { useCardSize } from "../../../composables/dashboard/useCardSize"
import { useModels } from "../../../composables/models/useModels"
import { useMetrics } from "../../../composables/system/useMetrics"
import { MEMORY_THRESHOLDS, VRAM_THRESHOLDS, loadMeterState, summariseHost } from "../../../domain/hostStats"
import { formatTokenCount, formatTokenRate } from "../../../utils/format/units"
import { formatElapsedSeconds, formatRelativeTime, isRecorded } from "../../../utils/format/time"
import { useModelUse } from "../../../composables/newsroom/useModelUse"
import { speedOf } from "../../../utils/modelUse"
import { MODEL_USE_FAILED, MODEL_USE_KIND, MODEL_USE_NONE_TODAY, MODEL_USE_TODAY_HEADING } from "../../../constants/modelStats"
import { toModels } from "../../../router/routes"

// Dashboard card · Host health: whether a model is ready, how fast it answers and how loaded this computer is. Small shows the speed and
// the model; medium adds the trend and the load; large shows every figure. Throughput and CPU history is the client's own (the backend
// keeps none), so it reads "since page load". The large size also lists today's calls by model (the model stats the brief reads,
// ADR-0085). Stopping the model is on the Models page, not here.
const MB_PER_GB = 1024
const HISTORY_CAPTION = "since page load"
const NO_VALUE = "—"
const MS_PER_SECOND = 1000
// The busiest models of today the card has room for.
const TODAY_ROWS = 6

const size = useCardSize()
const { activeModel } = useModels()
const { report: use } = useModelUse()
const running = computed(() => {
  const a = activeModel.value
  if (!a) return ""
  const parts: string[] = []
  if (isRecorded(a.started_at)) parts.push(`up ${formatElapsedSeconds((Date.now() - Date.parse(a.started_at)) / MS_PER_SECOND)}`)
  if (isRecorded(a.last_used_at)) parts.push(`last call ${formatRelativeTime(a.last_used_at)}`)
  return parts.join(" · ")
})
const today = computed(() => (use.value?.today ?? []).slice(0, TODAY_ROWS))
const { metrics, history } = useMetrics()
const host = computed(() => (metrics.value ? summariseHost(metrics.value) : null))
const tps = computed(() => formatTokenRate(metrics.value?.llm_tokens_per_sec ?? 0))
const cpu = computed(() => String(Math.round(metrics.value?.load_percent ?? 0)))
const tpsHistory = computed(() => history.value.map((s) => Number(formatTokenRate(s.tokensPerSecond))))
const loadHistory = computed(() => history.value.map((s) => Math.round(s.loadPercent)))
const memory = computed(() => {
  const m = metrics.value
  if (!m || !host.value) return null
  return { used: (m.mem_used_mb / MB_PER_GB).toFixed(1), total: (m.mem_total_mb / MB_PER_GB).toFixed(1), percent: host.value.memory.percent }
})
const gpu = computed(() => {
  const g = host.value?.gpu
  if (!g) return null
  const hasTemperature = g.temperatureC !== null
  return {
    name: g.name,
    temperature: hasTemperature ? String(Math.round(g.temperatureC as number)) : NO_VALUE,
    temperatureCaption: hasTemperature ? undefined : "Temperature not reported",
    corePercent: g.corePercent,
    vramPercent: g.vramPercent,
    vramText: g.vramText,
  }
})
</script>

<template>
  <div data-test="host-health-card">
    <div class="mb-3 flex flex-wrap items-center gap-2" data-test="host-model">
      <template v-if="activeModel">
        <StatusTag :state="activeModel.ready ? 'success' : 'running'" :label="activeModel.ready ? 'Ready' : 'Loading'" />
        <span class="max-w-[24ch] truncate font-mono text-[length:var(--text-small)] text-primary" :title="activeModel.name">{{ activeModel.name }}</span>
        <span v-if="running && size !== 'xs'" class="font-mono text-[length:var(--text-small)] text-muted" data-test="host-model-running">{{ running }}</span>
      </template>
      <template v-else>
        <StatusTag state="neutral" label="No model loaded" />
        <RouterLink :to="toModels()" class="font-mono text-[length:var(--text-small)] text-accent-info-text hover:underline focus-visible:outline-none focus-visible:ring-2">Load a model</RouterLink>
      </template>
    </div>
    <LoadingState v-if="!metrics" label="Loading health" :rows="2" />
    <template v-else>
      <StatCard v-if="size === 'xs'" label="Throughput" :value="tps" unit="tok/s" />
      <div v-else-if="size === 'sm'">
        <StatCard label="Throughput" :value="tps" unit="tok/s" :caption="HISTORY_CAPTION"><Sparkline :values="tpsHistory" label="Tokens per second" unit="tok/s" state="success" /></StatCard>
        <p class="m-0 mt-2 text-[length:var(--text-small)] text-muted">CPU {{ cpu }}%<template v-if="memory"> · Memory {{ memory.used }} of {{ memory.total }} GB</template></p>
      </div>
      <div v-else>
        <div class="grid grid-cols-[repeat(auto-fit,minmax(min(100%,200px),1fr))] gap-px border border-hairline bg-border-hairline">
          <StatCard label="Throughput" :value="tps" unit="tok/s" :caption="HISTORY_CAPTION"><Sparkline :values="tpsHistory" label="Tokens per second" unit="tok/s" state="success" /></StatCard>
          <StatCard label="CPU load" :value="cpu" unit="%" :caption="HISTORY_CAPTION"><Sparkline :values="loadHistory" label="CPU load" unit="%" state="info" /></StatCard>
          <StatCard v-if="memory" label="Memory" :value="memory.used" :unit="`/ ${memory.total} GB`"><Meter label="Memory used" :value="memory.percent" :state="loadMeterState(memory.percent, MEMORY_THRESHOLDS)" /></StatCard>
          <StatCard v-if="gpu" :label="gpu.name" :value="gpu.temperature" :unit="gpu.temperatureCaption ? undefined : '°C'" :caption="gpu.temperatureCaption">
            <Meter label="GPU load" :value="gpu.corePercent" :text="`${Math.round(gpu.corePercent)}%`" :state="loadMeterState(gpu.corePercent)" />
            <Meter label="VRAM used" :value="gpu.vramPercent" :text="gpu.vramText" :state="loadMeterState(gpu.vramPercent, VRAM_THRESHOLDS)" />
          </StatCard>
          <StatCard v-else label="GPU" :value="NO_VALUE" :caption="metrics.gpu_error || 'No GPU detected'" />
        </div>
        <section v-if="use" class="mt-3 flex flex-col gap-1.5" data-test="host-today" :aria-label="MODEL_USE_TODAY_HEADING">
          <h3 class="m-0 font-mono text-[length:var(--text-micro)] uppercase tracking-[var(--tracking-micro)] text-muted">{{ MODEL_USE_TODAY_HEADING }}</h3>
          <p v-if="!today.length" class="m-0 text-[length:var(--text-small)] text-muted">{{ MODEL_USE_NONE_TODAY }}</p>
          <ul v-else class="m-0 flex list-none flex-col p-0">
            <li v-for="m in today" :key="m.model" class="flex flex-wrap items-baseline gap-x-3 border-t border-hairline py-1.5 font-mono text-[length:var(--text-small)] first:border-t-0">
              <span class="min-w-0 flex-1 truncate text-primary" :title="m.model">{{ m.model }}</span>
              <span class="text-muted">{{ MODEL_USE_KIND(m.local) }}</span>
              <span class="tabular-nums text-secondary">{{ m.calls }} {{ m.calls === 1 ? "call" : "calls" }}<template v-if="m.failed"> · {{ MODEL_USE_FAILED(m.failed) }}</template></span>
              <span class="tabular-nums text-muted">{{ m.local ? speedOf(m) : formatTokenCount(m.prompt_tokens + m.completion_tokens) + " tokens" }}</span>
            </li>
          </ul>
        </section>
      </div>
    </template>
  </div>
</template>
