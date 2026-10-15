<script setup lang="ts">
import { useId } from "vue"
import SectionHeading from "./SectionHeading.vue"

// A raised region (tone, not a border, sets it apart from the page) with an optional
// numbered title and actions. `flush` drops the body padding for tables/lists; a
// flush panel keeps a faint rule under its title so the rows below read as a list.
defineProps<{ title?: string; flush?: boolean; preserveCase?: boolean }>()
defineSlots<{ default(): unknown; actions?(): unknown }>()
const titleId = useId()
</script>

<template>
  <section
    :aria-labelledby="title ? titleId : undefined"
    class="relative min-w-0 rounded-[var(--radius-md)] bg-surface-raised"
  >
    <header
      v-if="title"
      :class="['flex min-h-11 flex-wrap items-center justify-between gap-2 px-4 py-2', flush ? 'border-b border-hairline/60' : '']"
    >
      <SectionHeading :id="titleId" :preserve-case="preserveCase">{{ title }}</SectionHeading>
      <div v-if="$slots.actions" class="flex flex-wrap items-center gap-2"><slot name="actions" /></div>
    </header>
    <div data-test="panel-body" :class="flush ? '' : 'p-4'"><slot /></div>
  </section>
</template>
