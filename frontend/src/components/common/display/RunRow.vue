<script setup lang="ts">
import type { RouteLocationNamedRaw } from 'vue-router'
import type { StatusState } from '../../../types/ui'
import StatusTag from './StatusTag.vue'

// One line of running or waiting work: a state tag, a name and a trailing note.
// With a destination the whole line is a link (so it can be opened, copied or
// middle-clicked like any link); without one it is plain text.
defineProps<{
  state: StatusState
  tag: string
  title: string
  note?: string
  /** Where the row leads; null/absent renders it as plain text. */
  to?: RouteLocationNamedRaw | null
  /** Dim the name (queued work). */
  muted?: boolean
}>()

const ROW_CLASS = 'flex min-w-0 flex-1 flex-wrap items-center gap-2'
</script>

<template>
  <RouterLink
    v-if="to"
    :to="to"
    :aria-label="`Open ${title}`"
    :class="[ROW_CLASS, '-mx-1 rounded-[var(--radius-sm)] px-1 py-0.5 hover:bg-surface-hover focus-visible:outline-none focus-visible:ring-2']"
  >
    <StatusTag :state="state" :label="tag" />
    <span class="min-w-0 flex-1 truncate font-mono text-[length:var(--text-small)]" :class="muted ? 'text-secondary' : 'text-primary'">{{ title }}</span>
    <span v-if="note" class="font-mono text-[length:var(--text-small)] tabular-nums text-muted">{{ note }}</span>
    <span aria-hidden="true" class="font-mono text-[length:var(--text-small)] text-muted">→</span>
  </RouterLink>
  <div v-else :class="ROW_CLASS">
    <StatusTag :state="state" :label="tag" />
    <span class="min-w-0 flex-1 truncate font-mono text-[length:var(--text-small)]" :class="muted ? 'text-secondary' : 'text-primary'">{{ title }}</span>
    <span v-if="note" class="font-mono text-[length:var(--text-small)] tabular-nums text-muted">{{ note }}</span>
  </div>
</template>
