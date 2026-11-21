<script setup lang="ts">
import { GUIDE_EXAMPLE_LABEL, GUIDE_STEPS_LABEL, SOURCE_GUIDES } from "../../constants/newsroom"
import type { SourceKind } from "../../types/newsroom"

// The steps for one kind of source. Shown on the Help tab and under the source in the watch form, so the words are written once.
defineProps<{ kind: SourceKind }>()
</script>

<template>
  <div class="flex flex-col gap-2 text-[length:var(--text-small)]" :data-test="'guide-' + kind">
    <p class="m-0 font-medium text-primary">{{ GUIDE_STEPS_LABEL }}</p>
    <ol class="m-0 flex list-decimal flex-col gap-1.5 pl-5 text-secondary">
      <li v-for="step in SOURCE_GUIDES[kind].steps" :key="step" class="break-words">{{ step }}</li>
    </ol>
    <p class="m-0 text-secondary">{{ GUIDE_EXAMPLE_LABEL }}: <code class="break-all font-mono text-primary">{{ SOURCE_GUIDES[kind].example }}</code></p>
    <p v-if="SOURCE_GUIDES[kind].note" class="m-0 break-words text-muted">{{ SOURCE_GUIDES[kind].note }}</p>
  </div>
</template>
