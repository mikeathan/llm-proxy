<script setup lang="ts">
import { useId } from "vue"
import SectionHeading from "./SectionHeading.vue"

// A hairline-bordered region with an optional numbered title and actions
// (supersedes a plain card). `flush` drops the body padding for tables/lists.
defineProps<{ title?: string; flush?: boolean; preserveCase?: boolean }>()
defineSlots<{ default(): unknown; actions?(): unknown }>()
const titleId = useId()
</script>

<template>
  <section
    :aria-labelledby="title ? titleId : undefined"
    class="relative min-w-0 rounded-[var(--radius-sm)] border border-hairline bg-surface"
  >
    <header
      v-if="title"
      class="flex min-h-11 flex-wrap items-center justify-between gap-2 border-b border-hairline px-4 py-2"
    >
      <SectionHeading :id="titleId" :preserve-case="preserveCase">{{ title }}</SectionHeading>
      <div v-if="$slots.actions" class="flex flex-wrap items-center gap-2"><slot name="actions" /></div>
    </header>
    <div data-test="panel-body" :class="flush ? '' : 'p-4'"><slot /></div>
  </section>
</template>
