<script setup lang="ts">
import { RouterLink } from "vue-router"
import { leadOutletsWords } from "../../constants/newsroom"
import { toBrief } from "../../router/routes"
import type { BriefStory } from "../../types/newsroom"

// A headline on a dashboard card: it opens the brief at that story (cards show and link, ADR-0064), never the article itself, so reading happens in one place.
// `sectionId` makes the section label a link to that section of the brief; the lead names its outlets under the headline.
defineProps<{ story: BriefStory; edition: number; section?: string; sectionId?: number; lead?: boolean }>()
const LABEL = "font-mono text-[length:var(--text-small)] uppercase tracking-wider text-accent-brand"
</script>

<template>
  <div class="flex min-w-0 flex-col gap-0.5 py-2">
    <RouterLink v-if="section && sectionId !== undefined" :to="toBrief({ edition, section: sectionId })" :class="[LABEL, 'self-start hover:underline focus-visible:outline-none focus-visible:ring-2']" data-test="brief-headline-section">{{ section }}</RouterLink>
    <span v-else-if="section" :class="LABEL">{{ section }}</span>
    <RouterLink
      :to="toBrief({ edition, story: story.pool_id })"
      :class="['min-w-0 break-words font-semibold text-primary hover:underline focus-visible:outline-none focus-visible:ring-2', lead ? 'text-[length:var(--text-h3,1.25rem)] leading-snug' : '']"
      data-test="brief-headline"
    >{{ story.headline }}</RouterLink>
    <span v-if="lead && story.outlets.length" class="text-[length:var(--text-small)] text-faint" data-test="brief-headline-outlets">{{ leadOutletsWords(story.outlets) }}</span>
  </div>
</template>
