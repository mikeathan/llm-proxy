<script setup lang="ts">
import { RouterLink } from "vue-router"
import { toBrief } from "../../router/routes"
import type { BriefStory } from "../../types/newsroom"

// A headline on a dashboard card: it opens the brief at that story (cards show and link, ADR-0064), never the article itself, so reading happens in one place.
defineProps<{ story: BriefStory; edition: number; section?: string; lead?: boolean }>()
</script>

<template>
  <div class="flex min-w-0 flex-col gap-0.5 py-2">
    <span v-if="section" class="font-mono text-[length:var(--text-small)] uppercase tracking-wider text-accent-brand">{{ section }}</span>
    <RouterLink
      :to="toBrief({ edition, story: story.pool_id })"
      :class="['min-w-0 break-words font-semibold text-primary hover:underline focus-visible:outline-none focus-visible:ring-2', lead ? 'text-[length:var(--text-h3,1.25rem)] leading-snug' : '']"
      data-test="brief-headline"
    >{{ story.headline }}</RouterLink>
    <span v-if="lead && story.outlets.length" class="text-[length:var(--text-small)] text-faint">{{ story.outlets.length }} {{ story.outlets.length === 1 ? "outlet" : "outlets" }}</span>
  </div>
</template>
