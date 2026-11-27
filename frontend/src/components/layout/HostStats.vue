<script setup lang="ts">
import { computed, ref, watch } from "vue"
import { useMetrics } from "../../composables/system/useMetrics"
import { useModels } from "../../composables/models/useModels"
import { useFootprint } from "../../composables/system/useFootprint"
import { useDismissable } from "../../composables/ui/useDismissable"
import { MEMORY_THRESHOLDS, VRAM_THRESHOLDS, loadLevel, loadMeterState, summariseFootprint, summariseHost } from "../../domain/hostStats"
import { formatPercent, formatTokenRate } from "../../utils/format/units"
import { formatElapsedSeconds, formatRelativeTime, isRecorded } from "../../utils/format/time"
import { toDashboard } from "../../router/routes"
import type { LoadLevel } from "../../types/metrics"
import PopoverPanel from "../common/layout/PopoverPanel.vue"
import Meter from "../common/display/Meter.vue"
import MicroLabel from "../common/display/MicroLabel.vue"
import Sparkline from "../common/display/Sparkline.vue"
import StatCard from "../common/display/StatCard.vue"

// Host stats in the top strip (every destination): CPU, memory, GPU core and
// token throughput at a glance, with the detail one click away. The polling
// belongs to useMetrics (one shared poll, paused on a hidden tab); this only
// reads it, so the strip costs a few text nodes per poll. Below `md` only the
// throughput stays in the strip; the panel becomes a bottom sheet.

const NO_VALUE = "Not reported"
const LEVEL_CLASS: Record<LoadLevel, string> = {
  ok: "text-secondary",
  busy: "text-state-running",
  high: "text-state-error",
}

const { metrics, history } = useMetrics()
const host = computed(() => (metrics.value ? summariseHost(metrics.value) : null))
const tpsHistory = computed(() => history.value.map((s) => Number(formatTokenRate(s.tokensPerSecond))))

const open = ref(false)
const root = ref<HTMLElement | null>(null)
useDismissable(root, open)

// What this service and its model server cost the machine: read only while the panel is open (its own endpoint, not the strip's poll).
const { footprint, failed: footprintFailed } = useFootprint(open)

// The local model that is loaded, read from the model catalogue (/state) each time the panel opens: the strip is on every page, so it
// does not poll the catalogue itself.
const NO_MODEL = "No local model running"
const MS_PER_SECOND = 1000
const { activeModel, refresh: refreshModels } = useModels()
watch(open, (isOpen) => {
  if (isOpen) void refreshModels()
})
const service = computed(() => (footprint.value && metrics.value ? summariseFootprint(footprint.value, metrics.value.mem_total_mb, activeModel.value?.name) : null))
const running = computed(() => {
  const m = activeModel.value
  if (!m) return null
  return {
    name: m.name,
    where: `:${m.port} · ${m.ready ? "ready" : "loading"}`,
    up: isRecorded(m.started_at) ? `up ${formatElapsedSeconds((Date.now() - Date.parse(m.started_at)) / MS_PER_SECOND)}` : "",
    lastCall: isRecorded(m.last_used_at) ? formatRelativeTime(m.last_used_at) : "",
  }
})
</script>

<template>
  <!-- A phone shows only the token rate, and only while a model is generating: an idle "0.0 t/s" is noise there. -->
  <div v-if="host" ref="root" :class="['relative flex-none', { 'max-md:hidden': host.tokensPerSecond <= 0 }]">
    <button
      type="button"
      :aria-expanded="open"
      aria-controls="host-stats-panel"
      aria-label="System stats — show details"
      class="flex h-[30px] items-center gap-3 whitespace-nowrap rounded-[var(--radius-md)] border border-hairline bg-surface-raised px-2.5 font-mono text-[length:var(--text-small)] transition-colors hover:border-strong hover:bg-surface-hover focus-visible:outline-none focus-visible:ring-2"
      @click="open = !open"
    >
      <span data-figure="cpu" :data-level="loadLevel(host.cpuPercent)" class="flex items-baseline gap-1.5 max-md:hidden">
        <MicroLabel>CPU</MicroLabel>
        <span :class="['tabular-nums', LEVEL_CLASS[loadLevel(host.cpuPercent)]]">{{ formatPercent(host.cpuPercent) }}</span>
      </span>
      <span data-figure="mem" :data-level="loadLevel(host.memory.percent, MEMORY_THRESHOLDS)" class="flex items-baseline gap-1.5 max-md:hidden">
        <MicroLabel>MEM</MicroLabel>
        <span :class="['tabular-nums', LEVEL_CLASS[loadLevel(host.memory.percent, MEMORY_THRESHOLDS)]]">{{ host.memory.percent }}%</span>
      </span>
      <span v-if="host.gpu" data-figure="gpu" :data-level="loadLevel(host.gpu.corePercent)" class="flex items-baseline gap-1.5 max-md:hidden">
        <MicroLabel>GPU</MicroLabel>
        <span :class="['tabular-nums', LEVEL_CLASS[loadLevel(host.gpu.corePercent)]]">{{ formatPercent(host.gpu.corePercent) }}</span>
      </span>
      <span data-figure="tps" class="flex items-baseline gap-1 md:border-l md:border-hairline md:pl-3">
        <span class="tabular-nums text-primary">{{ formatTokenRate(host.tokensPerSecond) }}</span>
        <MicroLabel>t/s</MicroLabel>
      </span>
    </button>

    <PopoverPanel v-if="open" id="host-stats-panel" title="System stats" width="sm:w-[340px]" @close="open = false">
      <section class="flex flex-col gap-2 border-b border-hairline p-3" data-test="running-model">
        <MicroLabel>Running model</MicroLabel>
        <template v-if="running">
          <p class="m-0 flex items-center gap-2 text-primary">
            <span class="size-2 flex-none rounded-full" :class="activeModel?.ready ? 'bg-state-success' : 'bg-state-running'" aria-hidden="true" />
            <span class="min-w-0 truncate font-medium" :title="running.name">{{ running.name }}</span>
          </p>
          <dl class="m-0 flex flex-col gap-1 font-mono text-[length:var(--text-small)] text-secondary">
            <div class="flex justify-between gap-2"><dt class="sr-only">Port and state</dt><dd class="m-0">{{ running.where }}</dd><dd v-if="running.up" class="m-0 tabular-nums">{{ running.up }}</dd></div>
            <div v-if="running.lastCall" class="flex justify-between gap-2"><dt class="text-muted">Last call</dt><dd class="m-0">{{ running.lastCall }}</dd></div>
          </dl>
        </template>
        <p v-else class="m-0 text-[length:var(--text-small)] text-muted">{{ NO_MODEL }}</p>
      </section>

      <section class="flex flex-col gap-3 border-b border-hairline p-3" data-test="service-footprint">
        <MicroLabel>This service</MicroLabel>
        <template v-if="service">
          <div v-for="row in service.rows" :key="row.key" class="flex flex-col gap-1.5" :data-footprint="row.key">
            <p class="m-0 truncate font-medium text-primary" :title="row.title">{{ row.title }}</p>
            <dl class="m-0 flex flex-col gap-1 text-[length:var(--text-small)]">
              <div class="flex justify-between gap-2"><dt class="text-muted">CPU</dt><dd class="m-0 font-mono tabular-nums text-secondary">{{ row.cpu }}</dd></div>
              <div class="flex justify-between gap-2"><dt class="text-muted">Memory</dt><dd class="m-0 font-mono tabular-nums text-secondary">{{ row.memory }}</dd></div>
              <div class="flex justify-between gap-2"><dt class="text-muted">GPU memory</dt><dd class="m-0 font-mono tabular-nums text-secondary">{{ row.gpu }}</dd></div>
              <div class="flex justify-between gap-2">
                <dt class="text-muted">Threads{{ row.openFiles === null ? "" : " · files" }}</dt>
                <dd class="m-0 font-mono tabular-nums text-secondary">{{ row.threads }}{{ row.openFiles === null ? "" : ` · ${row.openFiles}` }}</dd>
              </div>
              <div class="flex justify-between gap-2"><dt class="text-muted">Up</dt><dd class="m-0 font-mono tabular-nums text-secondary">{{ row.uptime }}</dd></div>
            </dl>
          </div>
          <p class="m-0 font-mono text-[length:var(--text-small)] text-muted" data-test="go-runtime">
            Go heap {{ service.go.heap }} of {{ service.go.total }} · {{ service.go.goroutines }} goroutines · {{ service.go.gcCycles }} GC cycles
          </p>
        </template>
        <p v-else class="m-0 text-[length:var(--text-small)] text-muted">{{ footprintFailed ? "Could not read the service figures" : "Reading…" }}</p>
      </section>

      <section class="flex flex-col gap-3 border-b border-hairline p-3">
        <MicroLabel>Host resources</MicroLabel>
        <Meter label="CPU load" :value="host.cpuPercent" :text="formatPercent(host.cpuPercent)" :state="loadMeterState(host.cpuPercent)" />
        <Meter label="Memory used" :value="host.memory.percent" :text="host.memory.text" :state="loadMeterState(host.memory.percent, MEMORY_THRESHOLDS)" />
      </section>

      <section class="flex flex-col gap-3 border-b border-hairline p-3">
        <MicroLabel>{{ host.gpu ? host.gpu.name : "GPU" }}</MicroLabel>
        <template v-if="host.gpu">
          <Meter label="GPU load" :value="host.gpu.corePercent" :text="formatPercent(host.gpu.corePercent)" :state="loadMeterState(host.gpu.corePercent)" />
          <Meter label="VRAM used" :value="host.gpu.vramPercent" :text="host.gpu.vramText" :state="loadMeterState(host.gpu.vramPercent, VRAM_THRESHOLDS)" />
          <dl class="m-0 flex flex-col gap-1.5 text-[length:var(--text-small)]">
            <div class="flex justify-between gap-2">
              <dt class="text-muted">Temperature</dt>
              <dd class="m-0 font-mono tabular-nums text-secondary">{{ host.gpu.temperatureC === null ? NO_VALUE : `${Math.round(host.gpu.temperatureC)}°C` }}</dd>
            </div>
          </dl>
        </template>
        <p v-else class="m-0 text-[length:var(--text-small)] text-muted">No GPU detected<template v-if="host.gpuError"> — {{ host.gpuError }}</template></p>
      </section>

      <div class="border-b border-hairline">
        <StatCard label="Throughput" :value="formatTokenRate(host.tokensPerSecond)" unit="tok/s" caption="since page load">
          <Sparkline :values="tpsHistory" label="Tokens per second" unit="tok/s" state="success" />
        </StatCard>
      </div>

      <RouterLink :to="toDashboard()" class="px-3 py-2.5 font-mono text-[length:var(--text-small)] text-accent-info-text hover:underline focus-visible:outline-none focus-visible:ring-2" @click="open = false">
        Open Overview
      </RouterLink>
    </PopoverPanel>
  </div>
</template>
