<script setup lang="ts">
import { computed, ref } from "vue"
import { useMetrics } from "../../composables/system/useMetrics"
import { useDismissable } from "../../composables/ui/useDismissable"
import { BUSY_PERCENT, MEMORY_THRESHOLDS, loadLevel, summariseHost } from "../../domain/hostStats"
import { formatPercent, formatTokenRate } from "../../utils/format/units"
import { toOverview } from "../../router/routes"
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

const VRAM_BUSY_PERCENT = 75
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

const meterState = (percent: number, busyAt: number) => (percent >= busyAt ? "running" : "info")
</script>

<template>
  <div v-if="host" ref="root" class="relative flex-none">
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
      <section class="flex flex-col gap-3 border-b border-hairline p-3">
        <MicroLabel>Host resources</MicroLabel>
        <Meter label="CPU load" :value="host.cpuPercent" :text="formatPercent(host.cpuPercent)" :state="meterState(host.cpuPercent, BUSY_PERCENT)" />
        <Meter label="Memory used" :value="host.memory.percent" :text="host.memory.text" :state="meterState(host.memory.percent, MEMORY_THRESHOLDS.busy)" />
      </section>

      <section class="flex flex-col gap-3 border-b border-hairline p-3">
        <MicroLabel>{{ host.gpu ? host.gpu.name : "GPU" }}</MicroLabel>
        <template v-if="host.gpu">
          <Meter label="VRAM used" :value="host.gpu.vramPercent" :text="host.gpu.vramText" :state="meterState(host.gpu.vramPercent, VRAM_BUSY_PERCENT)" />
          <dl class="m-0 flex flex-col gap-1.5 text-[length:var(--text-small)]">
            <div class="flex justify-between gap-2">
              <dt class="text-muted">Core utilization</dt>
              <dd class="m-0 font-mono tabular-nums text-secondary">{{ formatPercent(host.gpu.corePercent) }}</dd>
            </div>
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

      <RouterLink :to="toOverview()" class="px-3 py-2.5 font-mono text-[length:var(--text-small)] text-accent-info-text hover:underline focus-visible:outline-none focus-visible:ring-2" @click="open = false">
        Open Overview
      </RouterLink>
    </PopoverPanel>
  </div>
</template>
