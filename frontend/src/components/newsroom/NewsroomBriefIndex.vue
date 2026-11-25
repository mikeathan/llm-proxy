<script setup lang="ts">
import { RouterLink } from "vue-router"
import { toBrief } from "../../router/routes"
import type { BriefSection } from "../../types/newsroom"

// The sections of a brief as one line of links, each opening the brief at that section.
defineProps<{ sections: BriefSection[]; edition: number }>()
</script>

<template>
  <ul v-if="sections.length" class="m-0 flex list-none flex-wrap gap-x-4 border-t border-hairline p-0 pt-1" aria-label="Sections" data-test="brief-index">
    <li v-for="sec in sections" :key="sec.topic_id">
      <RouterLink :to="toBrief({ edition, section: sec.topic_id })" class="inline-flex min-h-11 items-center gap-1.5 text-[length:var(--text-small)] text-secondary hover:text-primary hover:underline sm:min-h-8">
        {{ sec.name }}<span class="font-mono text-faint">{{ sec.stories.length }}</span>
      </RouterLink>
    </li>
  </ul>
</template>
