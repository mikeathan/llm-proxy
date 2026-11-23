<script setup lang="ts">
import { computed } from "vue"
import StatusTag from "../common/display/StatusTag.vue"
import { formatRelativeTime } from "../../utils/format/time"
import { safeWebLink } from "../../utils/newsroom/safeLink"
import { HEADLINE_ONLY_LABEL, UPDATE_LABEL, briefReportedWords, postedWords } from "../../constants/newsroom"
import type { BriefStory } from "../../types/newsroom"

// One story of an edition: the headline (a link only if it is a real http or https address), what was written about it, who reported it and when. All text
// came from the open internet or a model that read it, so it is shown as text.
const props = defineProps<{ story: BriefStory; lead?: boolean; quiet?: boolean; section?: string }>()
const link = computed(() => safeWebLink(props.story.link))
</script>

<template>
  <article class="flex min-w-0 flex-col gap-1 py-3">
    <div class="flex flex-wrap items-center gap-x-2 gap-y-1 text-[length:var(--text-small)] text-faint">
      <span v-if="section" class="font-medium text-secondary" data-test="story-section">{{ section }}</span>
      <time v-if="story.posted" :datetime="story.posted">{{ postedWords(formatRelativeTime(story.posted)) }}</time>
      <StatusTag v-if="story.update" state="warning" :label="UPDATE_LABEL" />
      <StatusTag v-if="story.headline_only && !quiet" state="neutral" :label="HEADLINE_ONLY_LABEL" />
    </div>
    <a
      v-if="link"
      :href="link"
      target="_blank"
      rel="noopener noreferrer"
      :class="['min-w-0 break-words font-medium text-primary hover:underline focus-visible:outline-none focus-visible:ring-2', lead ? 'text-[length:var(--text-h3,1.25rem)]' : '']"
    >{{ story.headline }}</a>
    <p v-else :class="['m-0 min-w-0 break-words font-medium text-primary', lead ? 'text-[length:var(--text-h3,1.25rem)]' : '']">{{ story.headline }}</p>
    <p v-if="story.summary" class="m-0 break-words text-[length:var(--text-small)] text-secondary">{{ story.summary }}</p>
    <p v-if="story.outlets.length" class="m-0 break-words text-[length:var(--text-small)] text-faint">{{ briefReportedWords(story.outlets) }}</p>
  </article>
</template>
