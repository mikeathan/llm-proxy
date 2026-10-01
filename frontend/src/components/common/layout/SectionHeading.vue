<script setup lang="ts">
// `01 ── Title`: the index comes from a CSS counter (reset per page on the
// shell's content element), so numbering always follows document order.
// `preserveCase` keeps the title's case (file paths, where case matters).
withDefaults(defineProps<{ level?: 2 | 3 | 4; id?: string; preserveCase?: boolean }>(), { level: 2, preserveCase: false })
defineSlots<{ default(): unknown }>()
</script>

<template>
  <component
    :is="`h${level}`"
    :id="id"
    :class="['m-0 min-w-0 truncate font-mono text-[length:var(--text-small)] font-medium tracking-[0.04em] text-primary', preserveCase ? '' : 'uppercase']"
  >
    <span aria-hidden="true" class="mr-2 text-accent-brand [counter-increment:section] before:content-[counter(section,decimal-leading-zero)]"></span>
    <span aria-hidden="true" class="mr-2 inline-block h-px w-4 bg-text-decorative align-middle"></span>
    <slot />
  </component>
</template>
