<script setup lang="ts">
import type { RouteLocationNamedRaw } from 'vue-router'

// The name of a run in a compact list (the header run pill): a link to where the
// run lives when it has a destination, plain text otherwise. RunRow is the
// whole-line variant for roomier lists; both take their destination from
// utils/runs/runTarget.
defineProps<{
  title: string
  /** Where the run lives; null/absent renders plain text. */
  to?: RouteLocationNamedRaw | null
  /** Hover text, e.g. the model the run uses. */
  tip?: string
  /** Dim the name (queued work). */
  muted?: boolean
}>()
defineEmits<{ (e: 'navigate'): void }>()
</script>

<template>
  <RouterLink
    v-if="to"
    :to="to"
    :title="tip"
    :aria-label="`Open ${title}`"
    class="min-w-0 flex-1 truncate rounded-[var(--radius-sm)] underline-offset-2 hover:underline focus-visible:outline-none focus-visible:ring-2"
    :class="muted ? 'text-secondary' : 'text-primary'"
    @click="$emit('navigate')"
  >{{ title }}</RouterLink>
  <span v-else class="min-w-0 flex-1 truncate" :class="muted ? 'text-secondary' : 'text-primary'" :title="tip">{{ title }}</span>
</template>
