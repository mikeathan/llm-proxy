<script setup lang="ts">
import { safeWebLink } from "../../utils/newsroom/safeLink"
import StatusTag from "../common/display/StatusTag.vue"
import NewsroomPlate from "./NewsroomPlate.vue"
import { WATCHES_BLOCK_LABEL, WATCHES_SINCE_LAST, WATCH_SOURCE_PROBLEM, watchNewWords } from "../../constants/newsroom"
import type { BriefWatchLine } from "../../types/newsroom"

// What the owner's watches found since the edition before this one: one line per watch, with the newest finding and a mark on any source that could not
// be read. The text is the watch journal's own (headline and summary written by the watch), shown as text.
defineProps<{ lines: BriefWatchLine[] }>()
</script>

<template>
  <section class="flex flex-col rounded-[var(--radius-md)] bg-surface-raised" :aria-label="WATCHES_BLOCK_LABEL" data-test="watches-block">
    <header class="flex items-center gap-3 p-3">
      <NewsroomPlate :label="WATCHES_BLOCK_LABEL" kind="watch" />
      <div class="flex min-w-0 flex-1 flex-col">
        <h2 class="m-0 text-[length:var(--text-body)] font-medium text-primary">{{ WATCHES_BLOCK_LABEL }}</h2>
        <span class="text-[length:var(--text-small)] text-faint">{{ WATCHES_SINCE_LAST }}</span>
      </div>
    </header>
    <ul class="m-0 flex list-none flex-col divide-y divide-hairline border-t border-hairline px-3 p-0">
      <li v-for="l in lines" :key="l.watch" class="flex min-w-0 flex-col gap-1 py-3">
        <div class="flex flex-wrap items-center gap-2">
          <span class="font-medium text-primary">{{ l.watch }}</span>
          <StatusTag v-if="l.new > 0" state="info" :label="watchNewWords(l.new)" />
          <StatusTag v-if="l.problems?.length" state="error" :label="WATCH_SOURCE_PROBLEM" />
        </div>
        <a v-if="l.headline && safeWebLink(l.link)" :href="safeWebLink(l.link) ?? undefined" target="_blank" rel="noopener noreferrer" class="break-words text-primary hover:underline focus-visible:outline-none focus-visible:ring-2">{{ l.headline }}</a>
        <p v-else-if="l.headline" class="m-0 break-words text-primary">{{ l.headline }}</p>
        <p v-if="l.summary" class="m-0 break-words text-[length:var(--text-small)] text-secondary">{{ l.summary }}</p>
        <p v-for="p in l.problems ?? []" :key="p" class="m-0 break-words text-[length:var(--text-small)] text-state-error">{{ p }}</p>
      </li>
    </ul>
  </section>
</template>
