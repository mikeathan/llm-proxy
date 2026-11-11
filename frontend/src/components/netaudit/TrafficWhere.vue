<script setup lang="ts">
import { computed } from "vue"
import Panel from "../common/layout/Panel.vue"
import { countryBars, ownerBars } from "../../utils/netaudit/traffic"
import { formatBytes } from "../../utils/format/formatters"
import {
  TRAFFIC_WHERE_COUNTRIES, TRAFFIC_WHERE_NO_TABLE, TRAFFIC_WHERE_NOTHING, TRAFFIC_WHERE_OF_INTERNET, TRAFFIC_WHERE_OWNERS, TRAFFIC_WHERE_TITLE,
} from "../../constants/netaudit"
import type { HostTraffic } from "../../types/netaudit"

// Where the internet traffic of one look went: by the company that runs the network and by country, as ranked bars. The names are the holders' own
// words, so they are shown as text only. With no table downloaded the panel says how to get one; it never guesses.
const props = defineProps<{ traffic: HostTraffic }>()
const owners = computed(() => ownerBars(props.traffic))
const countries = computed(() => countryBars(props.traffic))
const lists = computed(() => [
  { id: "owners", title: TRAFFIC_WHERE_OWNERS, bars: owners.value },
  { id: "countries", title: TRAFFIC_WHERE_COUNTRIES, bars: countries.value },
].filter((l) => l.bars.length > 0))
const barWidth = (percent: number) => `${Math.max(2, Math.min(100, percent))}%`
</script>

<template>
  <Panel v-if="props.traffic.internet_bytes > 0" :title="TRAFFIC_WHERE_TITLE">
    <p v-if="!traffic.owners_used" class="m-0 text-secondary" data-test="where-no-table">{{ TRAFFIC_WHERE_NO_TABLE }}</p>
    <p v-else-if="!lists.length" class="m-0 text-secondary" data-test="where-nothing">{{ TRAFFIC_WHERE_NOTHING }}</p>
    <div v-else class="flex flex-col gap-4" data-test="where">
      <div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <section v-for="l in lists" :key="l.id" :aria-label="l.title" :data-test="`where-${l.id}`">
          <h3 class="m-0 mb-2 font-mono text-[length:var(--text-micro)] font-normal uppercase tracking-[var(--tracking-micro)] text-muted">{{ l.title }}</h3>
          <ul class="m-0 flex list-none flex-col gap-3 p-0">
            <li v-for="b in l.bars" :key="b.key" class="flex flex-col gap-1" data-test="where-row">
              <span class="flex w-full items-baseline justify-between gap-3">
                <span class="min-w-0 break-words text-primary" data-test="where-label"><span>{{ b.label }}</span> <span v-if="b.note" class="text-[length:var(--text-small)] text-muted">{{ b.note }}</span></span>
                <span class="flex-none text-[length:var(--text-small)] text-muted">{{ b.percent }}% · {{ formatBytes(b.bytes) }}</span>
              </span>
              <span class="block h-2 bg-surface-hover" aria-hidden="true"><span class="block h-2 bg-accent-info" :style="{ width: barWidth(b.percent) }"></span></span>
            </li>
          </ul>
        </section>
      </div>
      <p class="m-0 text-[length:var(--text-small)] text-muted">{{ TRAFFIC_WHERE_OF_INTERNET }}</p>
    </div>
  </Panel>
</template>
