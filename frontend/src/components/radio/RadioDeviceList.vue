<script setup lang="ts">
import { computed, ref } from "vue"
import {
  RADIO_CHIP_WORDS, RADIO_CLASS_ORDER, RADIO_CLASS_WORDS, RADIO_FILTER_ALL, RADIO_FILTER_LABEL, RADIO_FILTER_MINE, RADIO_LIST_LESS, RADIO_LIST_NONE, RADIO_RADAR_MINE,
  RADIO_RADAR_NOT_HEARD, RADIO_RADAR_USUAL_HIDE, RADIO_RADAR_USUAL_ROW, RADIO_RADAR_USUAL_SHOW, RADIO_RING_WORDS, RADIO_TREND_MARK, RADIO_TREND_WORDS,
  radioDotLabel, radioListMore, radioRangeLine, radioShiftWord, radioUsualMore,
} from "../../constants/radio"
import type { RadioClass, RadioLiveDevice } from "../../types/radio"

// Radio · the one list of devices: filter chips with counts (all, yours, then each kind), closest first, the ones not heard now folded away. A row opens
// the device. This replaces the separate kinds list, so a device appears once.
const props = defineProps<{ devices: RadioLiveDevice[]; selected: string | null }>()
const emit = defineEmits<{ (e: "pick", id: string): void }>()

const PAGE = 12
type Filter = "all" | "mine" | RadioClass
const filter = ref<Filter>("all")
const showUsual = ref(false)
const showAll = ref(false)

const inFilter = (d: RadioLiveDevice): boolean => filter.value === "all" || (filter.value === "mine" ? d.mine : !d.mine && d.class === filter.value)
const count = (f: Filter): number => props.devices.filter((d) => (f === "all" ? true : f === "mine" ? d.mine : !d.mine && d.class === f)).length
const chips = computed(() => [
  { id: "all" as Filter, label: RADIO_FILTER_ALL, n: count("all") },
  { id: "mine" as Filter, label: RADIO_FILTER_MINE, n: count("mine") },
  ...RADIO_CLASS_ORDER.map((c) => ({ id: c as Filter, label: RADIO_CHIP_WORDS[c], n: count(c) })).filter((c) => c.n > 0),
])
const byCloseness = (list: RadioLiveDevice[]) => [...list].sort((a, b) => Number(b.mine) - Number(a.mine) || a.near_m * a.far_m - b.near_m * b.far_m)
const matching = computed(() => props.devices.filter(inFilter))
const now = computed(() => byCloseness(matching.value.filter((d) => !d.usual)))
const usual = computed(() => byCloseness(matching.value.filter((d) => d.usual)))
const rows = computed(() => {
  const all = [...now.value, ...(showUsual.value ? usual.value : [])]
  return showAll.value ? all : all.slice(0, PAGE)
})
const hidden = computed(() => now.value.length + (showUsual.value ? usual.value.length : 0) - rows.value.length)
</script>

<template>
  <div class="flex min-w-0 flex-col gap-2" data-test="radio-device-list">
    <div role="group" :aria-label="RADIO_FILTER_LABEL" class="flex flex-wrap gap-1.5" data-test="radio-filter">
      <button
        v-for="c in chips" :key="c.id" type="button" :aria-pressed="filter === c.id" :data-filter="c.id" data-test="radio-chip"
        class="inline-flex min-h-11 items-center gap-1.5 rounded-[var(--radius-md)] border px-3 text-[length:var(--text-small)] focus-visible:outline-none focus-visible:ring-2 sm:min-h-8"
        :class="filter === c.id ? 'border-accent-brand bg-surface-active text-primary' : 'border-hairline text-secondary hover:bg-surface-hover'"
        @click="filter = c.id; showAll = false"
      >{{ c.label }}<span class="font-mono text-muted">{{ c.n }}</span></button>
    </div>
    <p v-if="!now.length && !(showUsual && usual.length)" class="m-0 text-secondary" data-test="radio-list-none">{{ RADIO_LIST_NONE }}</p>
    <ul class="m-0 flex list-none flex-col divide-y divide-hairline p-0" data-test="radio-radar-list">
      <li v-for="d in rows" :key="d.id" data-test="radio-radar-row" :data-usual="d.usual ? 1 : 0">
        <button type="button" class="flex min-h-11 w-full items-baseline justify-between gap-3 py-1.5 text-left hover:text-primary focus-visible:outline-none focus-visible:ring-2" :aria-pressed="selected === d.id" @click="emit('pick', d.id)">
          <span class="flex min-w-0 flex-col">
            <span class="truncate" :class="d.mine ? 'text-primary' : 'text-secondary'">{{ radioDotLabel(d) }}<span v-if="d.mine" class="ml-2 text-[length:var(--text-small)] text-accent-brand">{{ RADIO_RADAR_MINE }}</span></span>
            <span class="text-[length:var(--text-small)] text-muted">{{ RADIO_CLASS_WORDS[d.class] }} · {{ RADIO_RING_WORDS[d.ring].toLowerCase() }}<template v-if="d.usual"> · {{ RADIO_RADAR_USUAL_ROW }}</template><template v-else-if="!d.heard"> · {{ RADIO_RADAR_NOT_HEARD }}</template></span>
            <span v-if="d.unusual && d.usual_dbm" class="text-[length:var(--text-small)] text-state-running" data-test="radio-row-unusual">{{ radioShiftWord(d.shift_db ?? 0, d.usual_dbm) }}</span>
          </span>
          <span class="flex flex-none flex-col items-end text-[length:var(--text-small)]"><span class="font-mono text-primary">{{ radioRangeLine(d.near_m, d.far_m) }}</span><span v-if="!d.usual" class="text-muted">{{ RADIO_TREND_MARK[d.trend] }} {{ RADIO_TREND_WORDS[d.trend] }}</span></span>
        </button>
      </li>
    </ul>
    <p v-if="usual.length" class="m-0 flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1 text-[length:var(--text-small)] text-muted" data-test="radio-radar-usual">
      <span>{{ radioUsualMore(usual.length) }}</span>
      <button type="button" class="min-h-11 text-accent-brand hover:underline focus-visible:outline-none focus-visible:ring-2" :aria-pressed="showUsual" data-test="radio-usual-toggle" @click="showUsual = !showUsual">{{ showUsual ? RADIO_RADAR_USUAL_HIDE : RADIO_RADAR_USUAL_SHOW }}</button>
    </p>
    <button v-if="hidden > 0 || showAll" type="button" class="min-h-11 self-start text-accent-brand hover:underline focus-visible:outline-none focus-visible:ring-2" data-test="radio-list-more" @click="showAll = !showAll">{{ showAll ? RADIO_LIST_LESS : radioListMore(hidden) }}</button>
  </div>
</template>
