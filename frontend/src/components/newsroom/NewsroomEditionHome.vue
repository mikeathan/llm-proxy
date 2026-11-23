<script setup lang="ts">
import { RouterLink } from "vue-router"
import NewsroomPlate from "./NewsroomPlate.vue"
import { ROUTE_NAMES } from "../../types/routes"
import { HOME_BLOCK_LABEL, HOME_COUNTS_ONLY } from "../../constants/newsroom"
import type { BriefHomeLine } from "../../types/newsroom"

// What the core knows about the home network and Radio: a count and the core's own sentence for each fact. It carries no device name, address or network
// name by design, so the block says so and links to the pages that do. The text is shown as text.
defineProps<{ lines: BriefHomeLine[] }>()
</script>

<template>
  <section class="flex flex-col rounded-[var(--radius-md)] bg-surface-raised" :aria-label="HOME_BLOCK_LABEL" data-test="home-block">
    <header class="flex items-center gap-3 p-3">
      <NewsroomPlate :label="HOME_BLOCK_LABEL" kind="home" />
      <div class="flex min-w-0 flex-1 flex-col">
        <h2 class="m-0 text-[length:var(--text-body)] font-medium text-primary">{{ HOME_BLOCK_LABEL }}</h2>
        <span class="text-[length:var(--text-small)] text-faint">{{ HOME_COUNTS_ONLY }}</span>
      </div>
    </header>
    <ul class="m-0 flex list-none flex-col divide-y divide-hairline border-t border-hairline px-3 p-0">
      <li v-for="l in lines" :key="l.id" class="flex min-w-0 items-baseline gap-3 py-2">
        <span v-if="l.count" class="min-w-8 flex-none text-right font-medium tabular-nums text-primary">{{ l.count }}</span>
        <span v-else class="min-w-8 flex-none" aria-hidden="true"></span>
        <span class="min-w-0 break-words text-[length:var(--text-small)] text-secondary">{{ l.words }}</span>
      </li>
    </ul>
    <div class="flex flex-wrap gap-4 border-t border-hairline p-3 text-[length:var(--text-small)]">
      <RouterLink :to="{ name: ROUTE_NAMES.network }" class="text-accent-info-text hover:underline">Open Network</RouterLink>
      <RouterLink :to="{ name: ROUTE_NAMES.radio }" class="text-accent-info-text hover:underline">Open Radio</RouterLink>
    </div>
  </section>
</template>
