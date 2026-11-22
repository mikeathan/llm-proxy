<script setup lang="ts">
import { ref } from "vue"
import Icon from "../icons/Icon.vue"
import NewsroomBriefStory from "./NewsroomBriefStory.vue"
import { formatRelativeTime } from "../../utils/format/time"
import { BRIEF_PROBLEMS_LABEL, SECTION_EMPTY, SECTION_NEVER_READ, TOP_STORIES_LABEL, briefAsOfWords, briefMadeWords, briefStoryCountWords } from "../../constants/newsroom"
import type { BriefIssue } from "../../types/newsroom"

// One edition as it was made: the top stories, then a foldable section for each topic. The first two sections are open. A section says what the reader
// should know about it (collected long ago, not written by the model) and which sources could not be read, so a thin section explains itself.
const OPEN_AT_FIRST = 2
defineProps<{ issue: BriefIssue }>()

// A section the reader opened or folded by hand overrides the default.
const toggled = ref<Record<number, boolean>>({})
const isOpen = (topicId: number, index: number): boolean => toggled.value[topicId] ?? index < OPEN_AT_FIRST
const toggle = (topicId: number, index: number) => (toggled.value = { ...toggled.value, [topicId]: !isOpen(topicId, index) })
</script>

<template>
  <div class="flex flex-col gap-6">
    <p class="m-0 -mb-2 text-[length:var(--text-small)] text-faint">{{ briefMadeWords(formatRelativeTime(issue.made_at), issue.model_calls) }}</p>

    <section v-if="issue.top?.length" class="flex flex-col" :aria-label="TOP_STORIES_LABEL">
      <h2 class="m-0 border-b border-hairline pb-2 text-[length:var(--text-body)] font-medium text-primary">{{ TOP_STORIES_LABEL }}</h2>
      <ul class="m-0 flex list-none flex-col divide-y divide-hairline p-0">
        <li v-for="s in issue.top" :key="s.pool_id"><NewsroomBriefStory :story="s" /></li>
      </ul>
    </section>

    <section v-for="(sec, i) in issue.sections" :key="sec.topic_id" class="flex flex-col" :aria-label="sec.name">
      <header class="flex items-center gap-2 border-b border-hairline pb-2">
        <button
          type="button"
          class="flex min-w-0 flex-1 items-center gap-2 border-0 bg-transparent p-0 text-left focus-visible:outline-none focus-visible:ring-2"
          :aria-expanded="isOpen(sec.topic_id, i)"
          @click="toggle(sec.topic_id, i)"
        >
          <Icon name="chevron-down" size="xs" :class-name="['flex-none text-faint transition-transform duration-fast', isOpen(sec.topic_id, i) ? '' : '-rotate-90'].join(' ')" />
          <h2 class="m-0 text-[length:var(--text-body)] font-medium text-primary">{{ sec.name }}</h2>
          <span class="text-[length:var(--text-small)] text-faint">{{ briefStoryCountWords(sec.stories.length) }}</span>
        </button>
        <span v-if="sec.as_of" class="text-[length:var(--text-small)] text-faint">{{ briefAsOfWords(formatRelativeTime(sec.as_of)) }}</span>
      </header>
      <template v-if="isOpen(sec.topic_id, i)">
        <p v-if="sec.note" class="m-0 pt-2 text-[length:var(--text-small)] text-state-running" data-test="section-note">{{ sec.note }}</p>
        <div v-if="sec.problems?.length" class="pt-2 text-[length:var(--text-small)] text-state-error" data-test="section-problems">
          <p class="m-0 font-medium">{{ BRIEF_PROBLEMS_LABEL }}</p>
          <ul class="m-0 list-none p-0"><li v-for="p in sec.problems" :key="p" class="break-words">{{ p }}</li></ul>
        </div>
        <ul v-if="sec.stories.length" class="m-0 flex list-none flex-col divide-y divide-hairline p-0">
          <li v-for="s in sec.stories" :key="s.pool_id"><NewsroomBriefStory :story="s" /></li>
        </ul>
        <p v-else class="m-0 py-3 text-muted">{{ sec.as_of ? SECTION_EMPTY : SECTION_NEVER_READ }}</p>
      </template>
    </section>
  </div>
</template>
