<script setup lang="ts">
import MicroLabel from './MicroLabel.vue'

// A finished run's non-fatal problems (e.g. a report that could not be delivered). The run still completed, so this
// reads as a caution, not an error.
defineProps<{ warnings: string[] }>()
</script>

<template>
  <section v-if="warnings.length" class="flex flex-col gap-1" data-test="run-warnings">
    <MicroLabel>Warnings</MicroLabel>
    <ul class="m-0 flex list-none flex-col gap-1 p-0">
      <li
        v-for="(warning, i) in warnings"
        :key="i"
        class="whitespace-pre-wrap break-words border border-state-running/40 bg-state-running/10 p-2 font-mono text-[length:var(--text-small)] text-state-running"
      >
        {{ warning }}
      </li>
    </ul>
  </section>
</template>
