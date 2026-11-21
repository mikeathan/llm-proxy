<script setup lang="ts">
import { computed, ref, watch as onChange } from "vue"
import BaseToggle from "../common/buttons/BaseToggle.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import SegmentedControl from "../common/forms/SegmentedControl.vue"
import NewsroomWatchChips from "./NewsroomWatchChips.vue"
import Icon from "../icons/Icon.vue"
import NewsroomEntryItem from "./NewsroomEntryItem.vue"
import { dayEntries, unreadIds } from "../../utils/newsroom/dayEntries"
import {
  CLEAR_ALL_LABEL, CLEAR_LABEL, FILTER_PERIOD_LABEL, PERIOD_OPTIONS, CLEAR_READ_LABEL, DONE_LABEL, FEED_FILTER_EMPTY, FILTER_UNREAD_LABEL, MARK_ALL_READ_LABEL, MIN_WATCHES_FOR_CHIPS, MARK_READ_LABEL,
  SELECT_ALL_LABEL, SELECT_LABEL, SHOW_OLDER_LABEL, selectedWords, unreadWords, updatesWords,
} from "../../constants/newsroom"
import type { NewsroomDay, NewsroomEntry, NewsroomFeedFilter, NewsroomWatch } from "../../types/newsroom"

// The feed: a toolbar (filters, mark all read, select, clear), then one section per day, newest first. The newest day is open and older days are folded
// to a count, so a week of findings is a short page. A day is a flat list of compact rows; the watch is named on a row only when more than one watch
// has findings. It shows and reports; the page owns the data, asks before anything is deleted and does the deleting.
const DAYS_AT_FIRST = 7
const props = defineProps<{ days: NewsroomDay[]; watches: NewsroomWatch[]; filter: NewsroomFeedFilter }>()
const emit = defineEmits<{
  (e: "update:filter", filter: NewsroomFeedFilter): void
  (e: "mark-read", ids: number[]): void
  (e: "clear", what: { ids: number[] } | { scope: "read" | "all" }): void
}>()

const ALL = ""
// A watch that was deleted while it was chosen would leave an empty feed with no chip lit, so the choice goes back to all watches.
onChange(() => props.watches, (list) => {
  if (props.filter.watch !== null && !list.some((t) => t.id === props.filter.watch)) emit("update:filter", { ...props.filter, watch: null })
})

const shown = ref(DAYS_AT_FIRST)
const visibleDays = computed(() => props.days.slice(0, shown.value))
const hiddenDays = computed(() => props.days.length - visibleDays.value.length)
const entriesOf = (day: NewsroomDay): NewsroomEntry[] => dayEntries(day)
const allEntries = computed(() => props.days.flatMap(entriesOf))
const manyWatches = computed(() => new Set(allEntries.value.map((e) => e.watch_id)).size > 1)
const nameOf = (e: NewsroomEntry): string | undefined => (manyWatches.value ? props.watches.find((t) => t.id === e.watch_id)?.name : undefined)
const allUnread = computed(() => unreadIds(allEntries.value))
const anyRead = computed(() => allEntries.value.some((e) => e.read))

// A day the owner opened or folded by hand overrides the default (only the newest day is open).
const toggled = ref<Record<string, boolean>>({})
const isOpen = (day: NewsroomDay): boolean => toggled.value[day.day] ?? day.day === props.days[0]?.day
const toggleDay = (day: NewsroomDay) => (toggled.value = { ...toggled.value, [day.day]: !isOpen(day) })

// Select mode: checkboxes on the rows and a bar with what can be done to them.
const selecting = ref(false)
const selected = ref<Set<number>>(new Set())
const selectedIds = computed(() => [...selected.value])
const everyoneSelected = computed(() => allEntries.value.length > 0 && allEntries.value.every((e) => selected.value.has(e.id)))
function flip(id: number) {
  const next = new Set(selected.value)
  if (!next.delete(id)) next.add(id)
  selected.value = next
}
const selectAll = (on: boolean) => (selected.value = on ? new Set(allEntries.value.map((e) => e.id)) : new Set())
function finishSelecting() {
  selecting.value = false
  selected.value = new Set()
}
function clearSelected() {
  emit("clear", { ids: selectedIds.value })
  finishSelecting()
}
</script>

<template>
  <div class="flex flex-col gap-4">
    <NewsroomWatchChips v-if="watches.length >= MIN_WATCHES_FOR_CHIPS" :watches="watches" :model-value="filter.watch" @update:model-value="emit('update:filter', { ...filter, watch: $event })" />
    <div class="flex flex-wrap items-center gap-x-3 gap-y-2">
      <SegmentedControl
        :model-value="filter.days === null ? ALL : String(filter.days)"
        :options="PERIOD_OPTIONS"
        :label="FILTER_PERIOD_LABEL"
        variant="compact"
        @update:model-value="emit('update:filter', { ...filter, days: $event === ALL ? null : Number($event) })"
      />
      <BaseToggle :model-value="filter.unreadOnly" :label="FILTER_UNREAD_LABEL" @update:model-value="emit('update:filter', { ...filter, unreadOnly: $event })" />
      <div v-if="!selecting" class="ml-auto flex flex-wrap items-center gap-1.5">
        <BaseButton v-if="allUnread.length" variant="ghost" size="sm" @click="emit('mark-read', allUnread)">{{ MARK_ALL_READ_LABEL }}</BaseButton>
        <BaseButton variant="ghost" size="sm" icon="check" @click="selecting = true">{{ SELECT_LABEL }}</BaseButton>
        <BaseButton v-if="anyRead" variant="ghost" size="sm" @click="emit('clear', { scope: 'read' })">{{ CLEAR_READ_LABEL }}</BaseButton>
        <BaseButton variant="ghost" size="sm" @click="emit('clear', { scope: 'all' })">{{ CLEAR_ALL_LABEL }}</BaseButton>
      </div>
    </div>

    <div v-if="selecting" class="flex flex-wrap items-center gap-x-3 gap-y-2 border border-hairline bg-surface-raised p-2" role="toolbar" aria-label="Selected entries">
      <label class="flex min-h-6 cursor-pointer items-center gap-2 text-[length:var(--text-small)] text-primary">
        <input
          type="checkbox"
          :aria-label="SELECT_ALL_LABEL"
          :checked="everyoneSelected"
          :indeterminate="selected.size > 0 && !everyoneSelected"
          class="h-4 w-4 flex-none cursor-pointer sm:h-3.5 sm:w-3.5"
          @change="selectAll(($event.target as HTMLInputElement).checked)"
        />
        {{ SELECT_ALL_LABEL }}
      </label>
      <span role="status" class="font-mono text-[length:var(--text-small)] tabular-nums text-muted">{{ selectedWords(selected.size) }}</span>
      <div class="ml-auto flex items-center gap-1.5">
        <BaseButton variant="ghost" size="sm" :disabled="!selected.size" @click="emit('mark-read', selectedIds)">{{ MARK_READ_LABEL }}</BaseButton>
        <BaseButton variant="danger" size="sm" icon="trash" :disabled="!selected.size" @click="clearSelected">{{ CLEAR_LABEL }}</BaseButton>
        <BaseButton variant="ghost" size="sm" @click="finishSelecting">{{ DONE_LABEL }}</BaseButton>
      </div>
    </div>

    <p v-if="!visibleDays.length" class="m-0 text-muted" data-test="feed-filter-empty">{{ FEED_FILTER_EMPTY }}</p>

    <section v-for="day in visibleDays" :key="day.day" class="flex flex-col" :aria-label="day.label">
      <header class="flex items-center gap-2 border-b border-hairline pb-2">
        <button
          type="button"
          class="flex min-w-0 flex-1 items-center gap-2 border-0 bg-transparent p-0 text-left focus-visible:outline-none focus-visible:ring-2"
          :aria-expanded="isOpen(day)"
          @click="toggleDay(day)"
        >
          <Icon name="chevron-down" size="xs" :class-name="['flex-none text-faint transition-transform duration-fast', isOpen(day) ? '' : '-rotate-90'].join(' ')" />
          <h2 class="m-0 text-[length:var(--text-body)] font-medium text-primary">{{ day.label }}</h2>
          <span class="text-[length:var(--text-small)] text-faint">{{ updatesWords(day.updates) }}<template v-if="day.unread"> · {{ unreadWords(day.unread) }}</template></span>
        </button>
        <BaseButton v-if="day.unread" variant="ghost" size="sm" @click="emit('mark-read', unreadIds(entriesOf(day)))">{{ MARK_READ_LABEL }}</BaseButton>
      </header>
      <ul v-if="isOpen(day)" class="m-0 flex list-none flex-col divide-y divide-hairline p-0">
        <li v-for="entry in entriesOf(day)" :key="entry.id">
          <NewsroomEntryItem
            :entry="entry"
            :watch-name="nameOf(entry)"
            :selectable="selecting"
            :selected="selected.has(entry.id)"
            @toggle-select="flip(entry.id)"
            @open="!entry.read && emit('mark-read', [entry.id])"
          />
        </li>
      </ul>
    </section>
    <BaseButton v-if="hiddenDays > 0" variant="secondary" size="sm" class-name="self-start" @click="shown += DAYS_AT_FIRST">{{ SHOW_OLDER_LABEL }} ({{ hiddenDays }})</BaseButton>
  </div>
</template>
