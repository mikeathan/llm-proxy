<script setup lang="ts">
import { computed } from "vue"
import BaseToggle from "../common/buttons/BaseToggle.vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import SelectInput from "../common/forms/SelectInput.vue"
import NewsroomWatchDay from "./NewsroomWatchDay.vue"
import { FILTER_ALL_WATCHES, FILTER_WATCH_LABEL, FILTER_UNREAD_LABEL, MARK_READ_LABEL, updatesWords } from "../../constants/newsroom"
import type { ChoiceOption } from "../../types/ui"
import type { NewsroomDay, NewsroomFeedFilter, NewsroomWatch } from "../../types/newsroom"

// The feed: a filter row, then one block per day, a card per watch inside it. It shows and reports; the page owns the data.
const props = defineProps<{ days: NewsroomDay[]; watches: NewsroomWatch[]; filter: NewsroomFeedFilter }>()
const emit = defineEmits<{ (e: "update:filter", filter: NewsroomFeedFilter): void; (e: "mark-read", ids: number[]): void }>()

const ALL = ""
const watchOptions = computed<ChoiceOption[]>(() => [{ value: ALL, label: FILTER_ALL_WATCHES }, ...props.watches.map((t) => ({ value: String(t.id), label: t.name }))])
const dayUnreadIds = (day: NewsroomDay): number[] =>
  day.watches.flatMap((t) => [t.current, ...(t.earlier ?? [])]).filter((e) => !e.read).map((e) => e.id)
</script>

<template>
  <div class="flex flex-col gap-4">
    <div class="flex flex-wrap items-center gap-3">
      <SelectInput
        :model-value="filter.watch === null ? ALL : String(filter.watch)"
        :options="watchOptions"
        :label="FILTER_WATCH_LABEL"
        @update:model-value="emit('update:filter', { ...filter, watch: $event === ALL ? null : Number($event) })"
      />
      <BaseToggle :model-value="filter.unreadOnly" :label="FILTER_UNREAD_LABEL" @update:model-value="emit('update:filter', { ...filter, unreadOnly: $event })" />
    </div>
    <section v-for="day in days" :key="day.day" class="flex flex-col gap-3" :aria-label="day.label">
      <header class="flex flex-wrap items-baseline justify-between gap-2">
        <h2 class="m-0 text-[length:var(--text-body)] font-medium text-primary">
          {{ day.label }} <span class="font-normal text-faint">{{ updatesWords(day.updates) }}</span>
        </h2>
        <BaseButton v-if="dayUnreadIds(day).length" variant="ghost" size="sm" @click="emit('mark-read', dayUnreadIds(day))">{{ MARK_READ_LABEL }}</BaseButton>
      </header>
      <NewsroomWatchDay v-for="t in day.watches" :key="t.watch_id" :day="t" @mark-read="emit('mark-read', $event)" />
    </section>
  </div>
</template>
