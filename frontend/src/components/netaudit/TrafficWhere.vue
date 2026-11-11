<script setup lang="ts">
import { computed } from "vue"
import { countryBars, ownerBars } from "../../utils/netaudit/traffic"
import { formatBytes } from "../../utils/format/formatters"
import { TRAFFIC_WHERE_NO_TABLE, TRAFFIC_WHERE_NOTHING, TRAFFIC_WHERE_OF_INTERNET, TRAFFIC_WHERE_COUNTRIES, TRAFFIC_WHERE_OWNERS } from "../../constants/netaudit"
import type { HostTraffic, TrafficWhereView } from "../../types/netaudit"

// The internet traffic of one look grouped by the company that runs the network, or by country, as ranked bars. The names are the holders' own
// words, so they are shown as text only. With no table downloaded it says how to get one; it never guesses.
const props = defineProps<{ traffic: HostTraffic; view: TrafficWhereView }>()
const bars = computed(() => (props.view === "operators" ? ownerBars(props.traffic) : countryBars(props.traffic)))
const title = computed(() => (props.view === "operators" ? TRAFFIC_WHERE_OWNERS : TRAFFIC_WHERE_COUNTRIES))
const barWidth = (percent: number) => `${Math.max(2, Math.min(100, percent))}%`
</script>

<template>
  <p v-if="!traffic.owners_used" class="m-0 text-secondary" data-test="where-no-table">{{ TRAFFIC_WHERE_NO_TABLE }}</p>
  <p v-else-if="!bars.length" class="m-0 text-secondary" data-test="where-nothing">{{ TRAFFIC_WHERE_NOTHING }}</p>
  <div v-else class="flex flex-col gap-3" data-test="where" :aria-label="title">
    <ul class="m-0 flex list-none flex-col gap-3 p-0" :data-test="`where-${view}`">
      <li v-for="b in bars" :key="b.key" class="flex flex-col gap-1" data-test="where-row">
        <span class="flex w-full items-baseline justify-between gap-3">
          <span class="min-w-0 break-words text-primary" data-test="where-label"><span>{{ b.label }}</span> <span v-if="b.note" class="text-[length:var(--text-small)] text-muted">{{ b.note }}</span></span>
          <span class="flex-none text-[length:var(--text-small)] text-muted">{{ b.percent }}% · {{ formatBytes(b.bytes) }}</span>
        </span>
        <span class="block h-2 bg-surface-hover" aria-hidden="true"><span class="block h-2 bg-accent-info" :style="{ width: barWidth(b.percent) }"></span></span>
      </li>
    </ul>
    <p class="m-0 text-[length:var(--text-small)] text-muted">{{ TRAFFIC_WHERE_OF_INTERNET }}</p>
  </div>
</template>
