<script setup lang="ts">
import { computed, ref } from "vue"
import BaseButton from "../common/buttons/BaseButton.vue"
import Icon from "../icons/Icon.vue"
import NewsroomBriefStory from "./NewsroomBriefStory.vue"
import NewsroomEditionDay from "./NewsroomEditionDay.vue"
import NewsroomEditionHome from "./NewsroomEditionHome.vue"
import NewsroomEditionWatches from "./NewsroomEditionWatches.vue"
import NewsroomPlate from "./NewsroomPlate.vue"
import { formatRelativeTime } from "../../utils/format/time"
import {
  BRIEF_PROBLEMS_ACTION, BRIEF_PROBLEMS_FIX, BRIEF_PROBLEMS_LABEL, BRIEF_SECTION_SHOWN, COLLAPSE_ALL_LABEL, EXPAND_ALL_LABEL, LEAD_STORY_LABEL, SECTION_EMPTY, SECTION_HEADLINES_ONLY, SECTION_IN_TOP, SECTION_NEVER_READ, SHOW_FEWER_LABEL, TOP_STORIES_LABEL,
  briefAsOfWords, briefMadeWords, briefOutletsWords, briefSectionCountWords, showMoreWords,
} from "../../constants/newsroom"
import type { BriefIssue, BriefSection, BriefStory } from "../../types/newsroom"

// One edition as it was made: a masthead, the lead story, the other top stories, then a foldable card for each topic. A card never grows past a few
// stories (the rest sit behind "Show more"), so a long edition stays one screen of headlines. The first two cards are open. A card says what the reader
// should know about it (collected long ago, not written by the model) and which sources could not be read, so a thin section explains itself.
const OPEN_AT_FIRST = 2
const props = defineProps<{ issue: BriefIssue }>()
// A source that could not be read is fixed on Set up; the page that holds both views switches.
defineEmits<{ (e: "open-setup"): void }>()

// A section the reader opened or folded by hand overrides the default; "all" sets every card at once.
const toggled = ref<Record<number, boolean>>({})
const expanded = ref<Record<number, boolean>>({})
const isOpen = (topicId: number, index: number): boolean => toggled.value[topicId] ?? index < OPEN_AT_FIRST
const toggle = (topicId: number, index: number) => (toggled.value = { ...toggled.value, [topicId]: !isOpen(topicId, index) })
const anyOpen = computed(() => props.issue.sections.some((s, i) => isOpen(s.topic_id, i)))
function setAll(open: boolean) {
  toggled.value = Object.fromEntries(props.issue.sections.map((s) => [s.topic_id, open]))
  if (!open) expanded.value = {}
}
const showAll = (topicId: number) => expanded.value[topicId] === true
const flipAll = (topicId: number) => (expanded.value = { ...expanded.value, [topicId]: !showAll(topicId) })
const shown = <T,>(stories: T[], topicId: number): T[] => (showAll(topicId) ? stories : stories.slice(0, BRIEF_SECTION_SHOWN))

// A card whose every story is headline-only says so once, instead of tagging each story.
// A story already in the top stories is not repeated in its card.
const topIds = computed(() => new Set((props.issue.top ?? []).map((s) => s.pool_id)))
const storiesOf = (sec: BriefSection): BriefStory[] => sec.stories.filter((s) => !topIds.value.has(s.pool_id))
const headlinesOnly = (sec: BriefSection): boolean => storiesOf(sec).length > 0 && storiesOf(sec).every((s) => s.headline_only)

// Which section a top story came from, so a story from outside the topic's heading does not look misplaced.
const sectionOf = computed(() => new Map(props.issue.sections.flatMap((sec) => sec.stories.map((s) => [s.pool_id, sec.name] as const))))
const inTopCount = (sec: BriefSection): number => sec.stories.length - storiesOf(sec).length

const lead = computed(() => props.issue.top?.[0])
const otherTop = computed(() => props.issue.top?.slice(1) ?? [])
const madeDate = computed(() => new Date(props.issue.made_at).toLocaleDateString(undefined, { weekday: "long", day: "numeric", month: "long", year: "numeric" }))
</script>

<template>
  <div class="flex flex-col gap-5 print:gap-3" data-test="edition">
    <header class="flex flex-wrap items-end justify-between gap-x-4 gap-y-2 border-t-2 border-accent-brand pt-3">
      <div class="flex min-w-0 flex-col gap-1">
        <h2 class="m-0 break-words text-[length:var(--text-h2,1.75rem)] font-semibold leading-tight text-primary" data-test="masthead">{{ issue.name }}</h2>
        <p class="m-0 text-[length:var(--text-small)] text-faint">{{ madeDate }} · {{ briefMadeWords(formatRelativeTime(issue.made_at), issue.model_calls) }}<template v-if="issue.outlets_read"> · {{ briefOutletsWords(issue.outlets_read) }}</template></p>
      </div>
      <BaseButton v-if="issue.sections.length > 1" class-name="print:hidden" size="sm" variant="secondary" data-test="collapse-all" @click="setAll(!anyOpen)">{{ anyOpen ? COLLAPSE_ALL_LABEL : EXPAND_ALL_LABEL }}</BaseButton>
    </header>

    <div class="grid items-start gap-5 lg:grid-cols-[minmax(0,1fr)_20rem] print:block" data-test="edition-body">
    <div class="flex min-w-0 flex-col gap-5" data-test="edition-main">
    <section v-if="lead" class="flex flex-col gap-2" :aria-label="TOP_STORIES_LABEL">
      <div class="flex items-start gap-4 rounded-[var(--radius-md)] bg-surface-raised p-4" data-test="lead-story">
        <div class="min-w-0 flex-1">
          <p class="m-0 text-[length:var(--text-small)] font-medium uppercase tracking-wide text-accent-brand">{{ LEAD_STORY_LABEL }}</p>
          <NewsroomBriefStory :story="lead" :section="sectionOf.get(lead.pool_id)" lead quiet />
        </div>
        <NewsroomPlate class="hidden sm:block" :label="issue.name" size="lg" />
      </div>
      <ul v-if="otherTop.length" class="m-0 flex list-none flex-col divide-y divide-hairline p-0">
        <li v-for="s in otherTop" :key="s.pool_id"><NewsroomBriefStory :story="s" :section="sectionOf.get(s.pool_id)" quiet /></li>
      </ul>
    </section>

    <section v-for="(sec, i) in issue.sections" :key="sec.topic_id" class="flex flex-col rounded-[var(--radius-md)] bg-surface-raised" :aria-label="sec.name">
      <header class="flex items-center gap-3 p-3">
        <NewsroomPlate :label="sec.name" :category="sec.category" />
        <button
          type="button"
          class="flex min-w-0 flex-1 items-center gap-2 border-0 bg-transparent p-0 text-left focus-visible:outline-none focus-visible:ring-2"
          :aria-expanded="isOpen(sec.topic_id, i)"
          @click="toggle(sec.topic_id, i)"
        >
          <span class="flex min-w-0 flex-1 flex-col">
            <h2 class="m-0 break-words text-[length:var(--text-body)] font-medium text-primary">{{ sec.name }}</h2>
            <span class="text-[length:var(--text-small)] text-faint">{{ briefSectionCountWords(sec.stories.length, inTopCount(sec)) }}<template v-if="sec.as_of"> · {{ briefAsOfWords(formatRelativeTime(sec.as_of)) }}</template></span>
          </span>
          <Icon name="chevron-down" size="xs" :class-name="['flex-none text-faint transition-transform duration-fast', isOpen(sec.topic_id, i) ? '' : '-rotate-90'].join(' ')" />
        </button>
      </header>
      <div v-if="isOpen(sec.topic_id, i)" class="flex flex-col border-t border-hairline px-3 pb-2">
        <p v-if="sec.note" class="m-0 pt-2 text-[length:var(--text-small)] text-state-running" data-test="section-note">{{ sec.note }}</p>
        <div v-if="sec.problems?.length" class="pt-2 text-[length:var(--text-small)] text-state-error" data-test="section-problems">
          <p class="m-0 font-medium">{{ BRIEF_PROBLEMS_LABEL }}</p>
          <ul class="m-0 list-none p-0"><li v-for="p in sec.problems" :key="p" class="break-words">{{ p }}</li></ul>
          <p class="m-0 pt-1 text-faint" data-test="problems-fix">{{ BRIEF_PROBLEMS_FIX }}</p>
          <BaseButton class-name="mt-1 print:hidden" size="sm" variant="secondary" data-test="problems-open-setup" @click="$emit('open-setup')">{{ BRIEF_PROBLEMS_ACTION }}</BaseButton>
        </div>
        <p v-if="headlinesOnly(sec)" class="m-0 pt-2 text-[length:var(--text-small)] text-faint" data-test="headlines-only">{{ SECTION_HEADLINES_ONLY }}</p>
        <ul v-if="storiesOf(sec).length" class="m-0 flex list-none flex-col divide-y divide-hairline p-0">
          <li v-for="s in shown(storiesOf(sec), sec.topic_id)" :key="s.pool_id"><NewsroomBriefStory :story="s" :quiet="headlinesOnly(sec)" /></li>
        </ul>
        <p v-else class="m-0 py-3 text-muted">{{ sec.stories.length ? SECTION_IN_TOP : sec.as_of ? SECTION_EMPTY : SECTION_NEVER_READ }}</p>
        <BaseButton
          v-if="storiesOf(sec).length > BRIEF_SECTION_SHOWN"
          class-name="self-start"
          size="sm"
          variant="ghost"
          data-test="show-more"
          @click="flipAll(sec.topic_id)"
        >{{ showAll(sec.topic_id) ? SHOW_FEWER_LABEL : showMoreWords(storiesOf(sec).length - BRIEF_SECTION_SHOWN) }}</BaseButton>
      </div>
    </section>
    </div>
    <aside v-if="issue.day || issue.home?.length || issue.watches?.length" class="flex min-w-0 flex-col gap-5 print:mt-3" data-test="edition-side" aria-label="Beside the news">
      <NewsroomEditionDay v-if="issue.day" :day="issue.day" />
      <NewsroomEditionHome v-if="issue.home?.length" :lines="issue.home" />
      <NewsroomEditionWatches v-if="issue.watches?.length" :lines="issue.watches" />
    </aside>
    </div>
  </div>
</template>
