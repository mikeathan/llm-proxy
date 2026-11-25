<script setup lang="ts">
import { RouterLink } from "vue-router"
import { useBriefChips } from "../../composables/newsroom/useBriefChips"
import { BRIEF_CHIPS_LABEL } from "../../constants/newsroom"

// A short row of what needs a look outside the brief: failed automation runs and alerts that could not be sent. Nothing at all when there is nothing, so a quiet
// day is a quiet card. Each chip links to the page where it is fixed.
const { chips } = useBriefChips()
</script>

<template>
  <ul v-if="chips.length" class="m-0 flex list-none flex-wrap gap-2 p-0" :aria-label="BRIEF_CHIPS_LABEL" data-test="brief-chips">
    <li v-for="c in chips" :key="c.key">
      <RouterLink :to="c.to" class="inline-flex min-h-11 items-center gap-2 rounded-[var(--radius-md)] border border-state-warning px-3 text-[length:var(--text-small)] text-primary hover:bg-surface-hover sm:min-h-8" :data-chip="c.key">{{ c.label }} →</RouterLink>
    </li>
  </ul>
</template>
