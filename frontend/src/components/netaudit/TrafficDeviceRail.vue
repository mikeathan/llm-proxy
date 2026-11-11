<script setup lang="ts">
import { computed, ref } from "vue"
import SegmentedControl from "../common/forms/SegmentedControl.vue"
import { formatBytes } from "../../utils/format/formatters"
import {
  DEVICE_PICKER_CLOSE, DEVICE_PICKER_OPEN, DEVICE_RAIL_ALL, DEVICE_RAIL_FIND, DEVICE_RAIL_FILTER_LABEL, DEVICE_RAIL_FIND_LABEL, DEVICE_RAIL_FLAGGED, DEVICE_RAIL_FLAG_LABEL, DEVICE_RAIL_LABEL, DEVICE_RAIL_NONE,
  DEVICE_RAIL_SECTION_FLAGGED, DEVICE_RAIL_SECTION_OTHERS, ROUTER_CONN_COUNT, PLACES_THIS_COMPUTER,
} from "../../constants/netaudit"
import type { TrafficDevice } from "../../types/netaudit"
import type { ChoiceOption } from "../../types/ui"

// The devices of the Traffic page, as a list that stays usable with many of them: a search, "needs a look" first (the devices the phone-home
// watch raised a flag for), then the rest, busiest first, in a list that scrolls on its own. On a phone the list is one button that shows the
// device in use and opens the same list. It only chooses; the device's own page is TrafficDevicePane.
const props = defineProps<{ devices: readonly TrafficDevice[]; selectedId: string | null }>()
const emit = defineEmits<{ (e: "select", id: string): void }>()

const query = ref("")
const filter = ref<"all" | "flagged">("all")
const pickerOpen = ref(false)

const weightOf = (d: TrafficDevice): number => d.rows.reduce((n, r) => n + (d.measure === "bytes" ? (r.bytes ?? 0) : r.count), 0)
const sizeOf = (d: TrafficDevice): string => (d.measure === "bytes" ? formatBytes(weightOf(d)) : String(weightOf(d)))
const sizeLabel = (d: TrafficDevice): string => (d.measure === "bytes" ? formatBytes(weightOf(d)) : ROUTER_CONN_COUNT(weightOf(d)))
const flagged = (d: TrafficDevice): boolean => d.flags.length > 0

const filterOptions = computed<ChoiceOption[]>(() => [
  { value: "all", label: DEVICE_RAIL_ALL(props.devices.length) },
  { value: "flagged", label: DEVICE_RAIL_FLAGGED(props.devices.filter(flagged).length) },
])

const matching = computed(() => {
  const q = query.value.trim().toLowerCase()
  return props.devices.filter((d) => (!q || `${d.name} ${d.address}`.toLowerCase().includes(q)) && (filter.value === "all" || flagged(d)))
})
const host = computed(() => matching.value.filter((d) => d.kind === "host"))
const needsLook = computed(() => matching.value.filter((d) => d.kind !== "host" && flagged(d)))
const others = computed(() => matching.value.filter((d) => d.kind !== "host" && !flagged(d)))
const selected = computed(() => props.devices.find((d) => d.id === props.selectedId) ?? null)

function pick(id: string) {
  pickerOpen.value = false
  emit("select", id)
}
</script>

<template>
  <nav :aria-label="DEVICE_RAIL_LABEL" data-test="rail">
    <!-- The phone's picker: the device in use, and the same list on request. -->
    <button
      v-if="selected"
      type="button"
      class="flex min-h-11 w-full items-center justify-between gap-3 rounded-[var(--radius-md)] border border-control bg-canvas px-3 py-2 text-left lg:hidden"
      :aria-expanded="pickerOpen"
      data-test="picker-toggle"
      @click="pickerOpen = !pickerOpen"
    >
      <span class="flex min-w-0 flex-col">
        <span class="truncate text-primary">{{ selected.name }}</span>
        <span class="truncate font-mono text-[length:var(--text-small)] text-muted">{{ selected.address }} · {{ sizeLabel(selected) }}</span>
      </span>
      <span class="flex-none text-[length:var(--text-small)] text-accent-info-text">{{ pickerOpen ? DEVICE_PICKER_CLOSE : DEVICE_PICKER_OPEN }}</span>
    </button>

    <div :class="['flex flex-col gap-2 rounded-[var(--radius-md)] bg-surface-raised p-3', pickerOpen ? 'mt-2' : 'max-lg:hidden']" data-test="rail-list">
      <input
        v-model="query"
        type="search"
        autocomplete="off"
        :placeholder="DEVICE_RAIL_FIND(devices.length)"
        :aria-label="DEVICE_RAIL_FIND_LABEL"
        class="h-11 w-full min-w-0 rounded-[var(--radius-md)] border border-control bg-canvas px-2 text-[length:var(--text-small)] text-primary focus-visible:outline-none focus-visible:ring-2 sm:h-8"
        data-test="rail-search"
      />
      <SegmentedControl v-model="filter" :options="filterOptions" :label="DEVICE_RAIL_FILTER_LABEL" variant="compact" data-test="rail-filter" />

      <div class="flex max-h-[34rem] flex-col gap-0.5 overflow-y-auto">
        <template v-for="section in [
          { id: 'host', title: PLACES_THIS_COMPUTER, items: host },
          { id: 'flagged', title: DEVICE_RAIL_SECTION_FLAGGED(needsLook.length), items: needsLook },
          { id: 'others', title: DEVICE_RAIL_SECTION_OTHERS(others.length), items: others },
        ]" :key="section.id">
          <template v-if="section.items.length">
            <h3 class="m-0 px-1 pb-0.5 pt-2 font-mono text-[length:var(--text-micro)] font-normal uppercase tracking-[var(--tracking-micro)]" :class="section.id === 'flagged' ? 'text-state-error' : 'text-muted'" :data-test="`rail-section-${section.id}`">{{ section.title }}</h3>
            <button
              v-for="d in section.items"
              :key="d.id"
              type="button"
              class="flex min-h-11 w-full items-center justify-between gap-2 rounded-[var(--radius-md)] px-2 py-1.5 text-left hover:bg-surface-hover"
              :class="d.id === selectedId ? 'bg-surface-hover' : ''"
              :aria-pressed="d.id === selectedId"
              data-test="rail-device"
              @click="pick(d.id)"
            >
              <span class="flex min-w-0 flex-col">
                <span class="truncate text-primary">{{ d.name }}</span>
                <span class="truncate font-mono text-[length:var(--text-small)] text-muted">{{ d.address }}</span>
              </span>
              <span class="flex flex-none items-baseline gap-1 font-mono text-[length:var(--text-small)] text-muted">
                {{ sizeOf(d) }}
                <span v-if="flagged(d)" class="text-state-error" role="img" :aria-label="DEVICE_RAIL_FLAG_LABEL(d.name)" data-test="rail-flag">⚑</span>
              </span>
            </button>
          </template>
        </template>
        <p v-if="!matching.length" class="m-0 px-1 py-2 text-muted" data-test="rail-none">{{ DEVICE_RAIL_NONE }}</p>
      </div>
    </div>
  </nav>
</template>
