<script setup lang="ts">
import { RouterLink } from "vue-router"
import { BRIEF_ATTENTION_LABEL, BRIEF_ATTENTION_LOOK } from "../../constants/newsroom"
import { homeLineRoute } from "../../utils/newsroom/homeLines"
import type { BriefHomeLine } from "../../types/newsroom"

// The critical home facts of a brief, said first wherever the brief is shown (the reader's side column, the dashboard cards): each in the error colour with a
// link to the place it is looked at. Nothing at all when there is none.
// In a narrow column (the reader's side) the label, the fact and the link stack; on a wide card they sit on one line.
defineProps<{ lines: BriefHomeLine[]; stacked?: boolean }>()
</script>

<template>
  <ul v-if="lines.length" class="m-0 flex list-none flex-col gap-2 p-0" :aria-label="BRIEF_ATTENTION_LABEL" data-test="brief-attention">
    <li v-for="l in lines" :key="l.id" :class="['flex min-w-0 flex-col gap-1 rounded-[var(--radius-md)] border border-state-error bg-surface-raised px-3 py-2 text-[length:var(--text-small)]', stacked ? '' : 'sm:flex-row sm:items-start sm:gap-3']">
      <span class="flex-none font-mono font-medium uppercase tracking-wider text-state-error">{{ BRIEF_ATTENTION_LABEL }}</span>
      <span class="min-w-0 break-words text-primary sm:flex-1">{{ l.words }}</span>
      <RouterLink :to="homeLineRoute(l)" :class="['inline-flex min-h-11 flex-none items-center text-accent-info-text hover:underline', stacked ? 'self-start sm:min-h-8' : 'sm:min-h-0']" data-test="brief-attention-look">{{ BRIEF_ATTENTION_LOOK }} →</RouterLink>
    </li>
  </ul>
</template>
