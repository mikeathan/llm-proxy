<script setup lang="ts">
import { computed } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import NewsroomEntryItem from "./NewsroomEntryItem.vue"
import { formatRelativeTime } from "../../utils/format/time"
import { dayHeadline, EARLIER_LABEL, MARK_READ_LABEL } from "../../constants/newsroom"
import type { NewsroomTopicDay } from "../../types/newsroom"

// One topic's card for one day: the newest finding on top as the current one, and every earlier finding of the day still visible beneath it.
const props = defineProps<{ day: NewsroomTopicDay }>()
const emit = defineEmits<{ (e: "mark-read", ids: number[]): void }>()
const unreadIds = computed(() => [props.day.current, ...(props.day.earlier ?? [])].filter((e) => !e.read).map((e) => e.id))
</script>

<template>
  <section class="flex flex-col gap-3 border border-hairline bg-surface p-3" :aria-label="day.topic">
    <header class="flex flex-wrap items-baseline justify-between gap-2">
      <div class="flex min-w-0 flex-col">
        <h3 class="m-0 break-words text-[length:var(--text-body)] font-medium text-primary">{{ day.topic }}</h3>
        <p class="m-0 text-[length:var(--text-small)] text-faint">{{ dayHeadline(day.updates, formatRelativeTime(day.last_at)) }}</p>
      </div>
      <BaseButton v-if="unreadIds.length" variant="ghost" size="sm" @click="emit('mark-read', unreadIds)">{{ MARK_READ_LABEL }}</BaseButton>
    </header>
    <NewsroomEntryItem :entry="day.current" />
    <div v-if="day.earlier?.length" class="flex flex-col gap-3 border-t border-hairline pt-3">
      <p class="m-0 text-[length:var(--text-micro)] font-medium uppercase tracking-[var(--tracking-micro)] text-faint">{{ EARLIER_LABEL }}</p>
      <NewsroomEntryItem v-for="entry in day.earlier" :key="entry.id" :entry="entry" />
    </div>
  </section>
</template>
