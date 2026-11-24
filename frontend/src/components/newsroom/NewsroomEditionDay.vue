<script setup lang="ts">
import { RouterLink } from "vue-router"
import NewsroomPlate from "./NewsroomPlate.vue"
import { ROUTE_NAMES } from "../../types/routes"
import { DAY_BLOCK_LABEL, DAY_NONE, DAY_REPEATS, DAY_UNAVAILABLE, dayHeading } from "../../constants/newsroom"
import type { BriefDay } from "../../types/newsroom"

// The owner's own reminders for the day, as they wrote them: a time and their words, shown as text. They are kept with the edition and read only here (never
// in a prompt, a fetch or a message), so this block is the one place they appear. Reminders off or unreadable is said, because it is not a quiet day.
defineProps<{ day: BriefDay }>()
</script>

<template>
  <section class="flex flex-col rounded-[var(--radius-md)] bg-surface-raised" :aria-label="DAY_BLOCK_LABEL" data-test="day-block">
    <header class="flex items-center gap-3 p-3">
      <NewsroomPlate :label="DAY_BLOCK_LABEL" kind="paper" />
      <div class="flex min-w-0 flex-1 flex-col">
        <h2 class="m-0 text-[length:var(--text-body)] font-medium text-primary">{{ DAY_BLOCK_LABEL }}</h2>
        <span class="text-[length:var(--text-small)] text-faint">{{ dayHeading(!!day.tomorrow) }}</span>
      </div>
    </header>
    <p v-if="day.unavailable" class="m-0 border-t border-hairline p-3 text-[length:var(--text-small)] text-state-running" data-test="day-unavailable">{{ DAY_UNAVAILABLE }}</p>
    <p v-else-if="!day.items.length" class="m-0 border-t border-hairline p-3 text-[length:var(--text-small)] text-muted" data-test="day-none">{{ DAY_NONE }}</p>
    <ul v-else class="m-0 flex list-none flex-col divide-y divide-hairline border-t border-hairline px-3 p-0">
      <li v-for="(item, i) in day.items" :key="i" class="flex min-w-0 items-baseline gap-3 py-2">
        <span class="min-w-12 flex-none text-right font-medium tabular-nums text-primary">{{ item.clock }}</span>
        <span class="min-w-0 break-words text-[length:var(--text-small)] text-secondary">{{ item.title }}<span v-if="item.repeats" class="text-faint"> · {{ DAY_REPEATS }}</span></span>
      </li>
    </ul>
    <div class="border-t border-hairline p-3 text-[length:var(--text-small)]">
      <RouterLink :to="{ name: ROUTE_NAMES.reminders }" class="text-accent-info-text hover:underline">Open Reminders</RouterLink>
    </div>
  </section>
</template>
