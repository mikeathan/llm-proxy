<script setup lang="ts">
import MicroLabel from "./MicroLabel.vue"

// A labelled figure: large tabular value + unit, an optional caption, and an
// optional visual (Sparkline or Meter) in the default slot.
// `plain` is for a value that is words, not a figure ("5 days ago"): body type, regular weight.
defineProps<{ label: string; value: string; unit?: string; caption?: string; plain?: boolean }>()
defineSlots<{ default?(): unknown }>()
</script>

<template>
  <div class="flex min-w-0 flex-col gap-1.5 bg-surface p-3.5">
    <MicroLabel>{{ label }}</MicroLabel>
    <p class="m-0 flex items-baseline gap-1">
      <span data-test="stat-value" :class="plain ? 'text-[length:var(--text-heading)] font-normal leading-[1.3] text-primary' : 'font-mono text-[length:var(--text-numeral)] font-medium leading-[1.1] tabular-nums text-primary'">{{ value }}</span>
      <span v-if="unit" class="text-[length:var(--text-small)] text-muted">{{ unit }}</span>
    </p>
    <slot />
    <p v-if="caption" class="m-0 text-[length:var(--text-small)] text-muted">{{ caption }}</p>
  </div>
</template>
