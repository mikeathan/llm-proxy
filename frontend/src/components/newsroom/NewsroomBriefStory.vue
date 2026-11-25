<script setup lang="ts">
import { computed, ref } from "vue"
import StatusTag from "../common/display/StatusTag.vue"
import NewsroomStoryTune from "./NewsroomStoryTune.vue"
import { storyWords } from "../../utils/newsroom/storyTuning"
import { formatRelativeTime } from "../../utils/format/time"
import { safeWebLink } from "../../utils/newsroom/safeLink"
import { COVERAGE_TICKS, HEADLINE_ONLY_LABEL, STORY_FOLLOW_LABEL, STORY_LESS_LABEL, STORY_MORE_LABEL, STORY_TUNE_LABEL, UPDATE_LABEL, briefReportedWords, coverageWords, postedWords } from "../../constants/newsroom"
import type { BriefStory } from "../../types/newsroom"

// One story of an edition: the headline (a link only if it is a real http or https address), what was written about it, who reported it and when. A row of
// ticks, one per outlet, says how widely it was reported before the names are read. All text came from the open internet or a model that read it, so it is
// shown as text. In the reader each story is an anchor (`brief-story-<id>`) a card can link to, and a stop for the j and k keys (data-story).
// In the reader a story can also be tuned (`topic` is the name of the topic it came from): more like it, less like it, or followed on its own.
const props = defineProps<{ story: BriefStory; lead?: boolean; quiet?: boolean; section?: string; anchor?: boolean; topic?: string }>()
const emit = defineEmits<{ (e: "tune", more: boolean, words: string[]): void; (e: "follow", words: string[]): void }>()
const link = computed(() => safeWebLink(props.story.link))
const words = computed(() => storyWords(props.story.headline))
const tuning = ref<"more" | "less" | null>(null)
function apply(picked: string[]) {
  emit("tune", tuning.value === "more", picked)
  tuning.value = null
}
const ticks = computed(() => Math.min(props.story.outlets.length, COVERAGE_TICKS))
</script>

<template>
  <article :id="anchor ? `brief-story-${story.pool_id}` : undefined" class="flex min-w-0 scroll-mt-24 flex-col gap-1 py-3" :data-story="anchor ? story.pool_id : undefined">
    <div v-if="section || story.update || (story.headline_only && !quiet)" class="flex flex-wrap items-center gap-x-2 gap-y-1">
      <span v-if="section" class="font-mono text-[length:var(--text-small)] uppercase tracking-wider text-accent-brand" data-test="story-section">{{ section }}</span>
      <StatusTag v-if="story.update" state="warning" :label="UPDATE_LABEL" />
      <StatusTag v-if="story.headline_only && !quiet" state="neutral" :label="HEADLINE_ONLY_LABEL" />
    </div>
    <a
      v-if="link"
      :href="link"
      target="_blank"
      rel="noopener noreferrer"
      :class="['min-w-0 break-words font-semibold text-primary hover:underline focus-visible:outline-none focus-visible:ring-2', lead ? 'text-[length:var(--text-h2,1.75rem)] leading-tight' : '']"
    >{{ story.headline }}</a>
    <p v-else :class="['m-0 min-w-0 break-words font-semibold text-primary', lead ? 'text-[length:var(--text-h2,1.75rem)] leading-tight' : '']">{{ story.headline }}</p>
    <p v-if="story.summary" :class="['m-0 break-words text-secondary', lead ? '' : 'text-[length:var(--text-small)]']">{{ story.summary }}</p>
    <!-- Flowing text, not a flex row, so a long list of outlets wraps like a sentence and the time stays on the line with the last outlet. -->
    <p class="m-0 min-w-0 break-words text-[length:var(--text-small)] text-faint">
      <span v-if="ticks" class="mr-2 inline-flex gap-0.5 align-[-0.05em]" role="img" :aria-label="coverageWords(story.outlets.length)" data-test="coverage">
        <span v-for="i in COVERAGE_TICKS" :key="i" :class="['inline-block h-2.5 w-1.5', i <= ticks ? 'bg-text-secondary' : 'bg-bar-track']" />
      </span>
      <span v-if="story.outlets.length">{{ briefReportedWords(story.outlets) }}</span>
      <time v-if="story.posted" :datetime="story.posted" class="whitespace-nowrap"><template v-if="story.outlets.length">&nbsp;· </template>{{ postedWords(formatRelativeTime(story.posted)) }}</time>
    </p>
    <div v-if="topic && words.length" class="flex flex-wrap gap-x-3 text-[length:var(--text-small)] print:hidden" role="group" :aria-label="STORY_TUNE_LABEL" data-test="story-actions">
      <button type="button" class="min-h-11 whitespace-nowrap border-0 bg-transparent p-0 text-faint hover:text-primary hover:underline sm:min-h-6" data-test="story-more" @click="tuning = tuning === 'more' ? null : 'more'">{{ STORY_MORE_LABEL }}</button>
      <button type="button" class="min-h-11 whitespace-nowrap border-0 bg-transparent p-0 text-faint hover:text-primary hover:underline sm:min-h-6" data-test="story-less" @click="tuning = tuning === 'less' ? null : 'less'">{{ STORY_LESS_LABEL }}</button>
      <button type="button" class="min-h-11 whitespace-nowrap border-0 bg-transparent p-0 text-faint hover:text-primary hover:underline sm:min-h-6" data-test="story-follow" @click="emit('follow', words.slice(0, 3))">{{ STORY_FOLLOW_LABEL }}</button>
    </div>
    <NewsroomStoryTune v-if="tuning && topic" :words="words" :more="tuning === 'more'" :topic="topic" @apply="apply" @cancel="tuning = null" />
  </article>
</template>
