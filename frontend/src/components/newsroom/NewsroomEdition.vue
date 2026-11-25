<script setup lang="ts">
import { computed, ref } from "vue"
import { RouterLink } from "vue-router"
import { toNewsroom, toSettings } from "../../router/routes"
import BaseButton from "../common/buttons/BaseButton.vue"
import StatusTag from "../common/display/StatusTag.vue"
import NewsroomBriefAttention from "./NewsroomBriefAttention.vue"
import NewsroomBriefChips from "./NewsroomBriefChips.vue"
import NewsroomBriefStory from "./NewsroomBriefStory.vue"
import NewsroomEditionDay from "./NewsroomEditionDay.vue"
import NewsroomEditionHome from "./NewsroomEditionHome.vue"
import NewsroomEditionModelUse from "./NewsroomEditionModelUse.vue"
import NewsroomEditionWatches from "./NewsroomEditionWatches.vue"
import NewsroomPlate from "./NewsroomPlate.vue"
import { useStoryKeys } from "../../composables/newsroom/useStoryKeys"
import { useToast } from "../../composables/useToast"
import { formatRelativeTime } from "../../utils/format/time"
import { briefText } from "../../utils/newsroom/briefText"
import { copyText } from "../../utils/clipboard"
import { splitHome } from "../../utils/newsroom/homeLines"
import {
  BRIEF_BESIDE_LABEL, BRIEF_COPIED, BRIEF_COPY_FAILED, BRIEF_COPY_LABEL, BRIEF_END_WORDS, BRIEF_INDEX_LABEL, BRIEF_PRINT_LABEL, BRIEF_PROBLEMS_ACTION, BRIEF_PROBLEMS_FIX, BRIEF_PROBLEMS_LABEL,
  BRIEF_SECTION_SHOWN, BRIEF_SEND_FIX, BRIEF_SEND_PROBLEM, LEAD_STORY_LABEL, NEW_BRIEF_LABEL, SECTION_EMPTY, SECTION_HEADLINES_ONLY, SECTION_IN_TOP, SECTION_NEVER_READ, SHOW_FEWER_LABEL, TOP_STORIES_LABEL,
  briefAsOfWords, briefCallsWords, briefOutletsWords, briefWrittenWords, briefSectionCountWords, briefSentWords, showMoreWords,
} from "../../constants/newsroom"
import type { BriefIssue, BriefSection, BriefStatus, BriefStory } from "../../types/newsroom"

// One edition as it was written, laid out as a paper to read (ADR-0086): a nameplate and one dateline, the lead and the other top stories, then every section
// open, one after another, and an end mark. The index of the sections is a row of links that stays under the header; on a wide screen what is beside the news sits on
// the right (needs a look, your day, alerts, home, model use), and on a phone the side column follows the top stories. Every
// section and story is an anchor, so a card links straight to it, and j and k move between the stories.
// `topics` names the topic behind each section (its first topic's id), so a story there can be tuned; without it the stories carry no actions.
// nameInSwitch: an edition switch beside the nameplate already names the brief, so on a phone the nameplate is left to screen readers.
const props = defineProps<{ issue: BriefIssue; status: BriefStatus; isNew?: boolean; topics?: Record<number, string>; nameInSwitch?: boolean }>()
// masthead-end: the page's controls beside the nameplate; when: a picker that stands in for the date in the dateline (the date still prints).
defineSlots<{ 'masthead-end'?(): unknown; when?(): unknown }>()
const emit = defineEmits<{ (e: "tune", topicId: number, more: boolean, words: string[]): void; (e: "follow", words: string[]): void }>()
// The topic each story came from: the section that holds it.
const topicOf = computed(() => new Map(props.issue.sections.flatMap((sec) => sec.stories.map((s) => [s.pool_id, sec.topic_id] as const))))
const topicName = (s: BriefStory): string | undefined => {
  const id = topicOf.value.get(s.pool_id)
  return id === undefined ? undefined : props.topics?.[id]
}
const tune = (s: BriefStory, more: boolean, words: string[]) => {
  const id = topicOf.value.get(s.pool_id)
  if (id !== undefined) emit("tune", id, more, words)
}
const root = ref<HTMLElement | null>(null)
useStoryKeys(root)
const toast = useToast()

const expanded = ref<Record<number, boolean>>({})
const showAll = (topicId: number) => expanded.value[topicId] === true
const flipAll = (topicId: number) => (expanded.value = { ...expanded.value, [topicId]: !showAll(topicId) })
const shown = <T,>(stories: T[], topicId: number): T[] => (showAll(topicId) ? stories : stories.slice(0, BRIEF_SECTION_SHOWN))

// A story already in the top stories is not repeated in its section, and a section whose every story is headline-only says so once.
const topIds = computed(() => new Set((props.issue.top ?? []).map((s) => s.pool_id)))
const storiesOf = (sec: BriefSection): BriefStory[] => sec.stories.filter((s) => !topIds.value.has(s.pool_id))
const headlinesOnly = (sec: BriefSection): boolean => storiesOf(sec).length > 0 && storiesOf(sec).every((s) => s.headline_only)
const inTopCount = (sec: BriefSection): number => sec.stories.length - storiesOf(sec).length
const sectionOf = computed(() => new Map(props.issue.sections.flatMap((sec) => sec.stories.map((s) => [s.pool_id, sec.name] as const))))

const lead = computed(() => props.issue.top?.[0])
const otherTop = computed(() => props.issue.top?.slice(1) ?? [])
const home = computed(() => splitHome(props.issue.home ?? []))
// The critical facts are said first, above everything beside the news; the Home block keeps the rest.
const homeRest = computed(() => [...home.value.check, ...home.value.routine])
// The other top stories share the row evenly: one story takes the whole width, two take half each, three or more sit three to a row (a rule between
// stories in a row, none at the start of a row). Static class strings, so the stylesheet keeps them.
const TOP_GRIDS = {
  1: { list: '', item: '' },
  2: { list: 'md:grid-cols-2', item: 'md:[&:nth-child(2n)]:border-l md:[&:nth-child(2n)]:pl-6' },
  3: { list: 'md:grid-cols-3', item: 'md:[&:not(:nth-child(3n+1))]:border-l md:[&:not(:nth-child(3n+1))]:pl-6' },
} as const
const topGrid = computed(() => TOP_GRIDS[Math.min(Math.max(otherTop.value.length, 1), 3) as 1 | 2 | 3])
const sectionAnchor = (sec: BriefSection) => `brief-section-${sec.topic_id}`

// "nothing" is an issue with nothing to say: no send was made, so neither a problem nor a sent line.
const sendProblem = computed(() => (props.issue.send_state && props.issue.send_state !== "nothing" ? props.issue.send_state : null))
const sentClock = computed(() => (props.issue.sent_at ? new Date(props.issue.sent_at).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }) : ""))
const madeDate = computed(() => new Date(props.issue.made_at).toLocaleDateString(undefined, { weekday: "long", day: "numeric", month: "long", year: "numeric" }))
// On a phone the dateline stays one line: the short date, and the model calls are left to wide screens.
const madeDateShort = computed(() => new Date(props.issue.made_at).toLocaleDateString(undefined, { weekday: "short", day: "numeric", month: "short" }))

// Through the shared helper: on plain HTTP (vertex.local) the browser has no clipboard API, and the helper falls back to the copy command.
async function copy() {
  if (await copyText(briefText(props.issue, madeDate.value))) toast.success(BRIEF_COPIED)
  else toast.error(BRIEF_COPY_FAILED)
}
const print = () => window.print()
</script>

<template>
  <div ref="root" class="flex flex-col gap-5 print:gap-3" data-test="edition">
    <header class="flex flex-col" data-test="nameplate">
      <div class="flex flex-wrap items-end gap-x-4 gap-y-3">
        <h2 :class="['m-0 flex min-w-0 flex-wrap items-center gap-3 break-words text-[length:var(--text-display,2.5rem)] font-bold leading-none tracking-tight text-primary max-sm:text-[length:var(--text-title)]', nameInSwitch ? 'max-sm:sr-only' : '']" data-test="masthead">
          {{ issue.name }}<StatusTag v-if="isNew" state="info" :label="NEW_BRIEF_LABEL" />
        </h2>
        <div :class="['ml-auto flex flex-wrap items-center gap-2 print:hidden', nameInSwitch ? 'max-sm:ml-0 max-sm:w-full' : '']"><slot name="masthead-end" /></div>
      </div>
      <div class="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1 border-y border-hairline py-1.5 font-mono text-[length:var(--text-small)] uppercase tracking-wider text-muted max-sm:mt-1" data-test="dateline">
        <template v-if="$slots.when"><slot name="when" /><span class="hidden print:inline">{{ madeDate }}</span></template>
        <template v-else><span class="max-sm:hidden">{{ madeDate }}</span><span class="sm:hidden">{{ madeDateShort }}</span></template>
        <span>{{ briefWrittenWords(formatRelativeTime(issue.made_at)) }}</span>
        <span class="max-sm:hidden" data-test="model-calls">{{ briefCallsWords(issue.model_calls) }}</span>
        <span v-if="issue.outlets_read">{{ briefOutletsWords(issue.outlets_read) }}</span>
        <span v-if="!sendProblem && sentClock" class="print:hidden" data-test="send-sent">{{ briefSentWords(sentClock) }}</span>
      </div>
      <p v-if="sendProblem" class="m-0 mt-2 flex flex-wrap items-center gap-x-2 text-[length:var(--text-small)] text-state-running print:hidden" role="status" data-test="send-problem">
        {{ BRIEF_SEND_PROBLEM[sendProblem] }}
        <RouterLink v-if="sendProblem === 'refused'" :to="toSettings('communication')" class="text-accent-info-text hover:underline" data-test="send-fix">{{ BRIEF_SEND_FIX }}</RouterLink>
      </p>
      <!-- Said only when the brief is not today's: the dateline already says when it was written, and the end mark when the next one comes. -->
      <p v-if="status.tone === 'old'" class="m-0 mt-2 text-[length:var(--text-small)] font-medium text-state-running print:hidden" :data-tone="status.tone" data-test="brief-status">
        {{ status.title }}. {{ status.detail }}
      </p>
    </header>

    <!-- The sections as one row of links that stays in view, at every width: a column of its own left most of the page empty. -->
    <nav v-if="issue.sections.length > 1" class="sticky top-0 z-10 -mx-1 flex items-center gap-0 overflow-x-auto border-b border-hairline bg-canvas print:hidden" :aria-label="BRIEF_INDEX_LABEL" data-test="index-row">
      <a v-if="lead" href="#brief-top" class="flex-none whitespace-nowrap px-3 py-3 text-[length:var(--text-small)] text-muted hover:text-primary">{{ TOP_STORIES_LABEL }}</a>
      <a v-for="sec in issue.sections" :key="sec.topic_id" :href="`#${sectionAnchor(sec)}`" class="flex-none whitespace-nowrap px-3 py-3 text-[length:var(--text-small)] text-muted hover:text-primary">{{ sec.name }} <span class="font-mono text-faint">{{ sec.stories.length }}</span></a>
    </nav>

    <div class="grid items-start gap-6 lg:grid-cols-[minmax(0,1fr)_20rem] xl:gap-8 print:block" data-test="edition-body">

      <!-- The news column dissolves below the wide layout, so the side column falls between the top stories and the sections on a phone. -->
      <div class="contents min-w-0 lg:flex lg:flex-col lg:gap-6 print:flex print:flex-col print:gap-5" data-test="edition-main">
        <section v-if="lead" id="brief-top" class="order-1 flex scroll-mt-24 flex-col lg:order-none" :aria-label="TOP_STORIES_LABEL">
          <div class="border-b border-hairline pb-3" data-test="lead-story">
            <p class="m-0 font-mono text-[length:var(--text-small)] uppercase tracking-wider text-accent-brand">{{ LEAD_STORY_LABEL }}<template v-if="sectionOf.get(lead.pool_id)"> · {{ sectionOf.get(lead.pool_id) }}</template></p>
            <NewsroomBriefStory :story="lead" lead quiet anchor :topic="topicName(lead)" @tune="(more, w) => tune(lead!, more, w)" @follow="emit('follow', $event)" />
          </div>
          <ul v-if="otherTop.length" :class="['m-0 grid list-none gap-x-6 p-0', topGrid.list]" data-test="top-stories">
            <li v-for="s in otherTop" :key="s.pool_id" :class="['min-w-0 border-hairline', topGrid.item]"><NewsroomBriefStory :story="s" :section="sectionOf.get(s.pool_id)" quiet anchor :topic="topicName(s)" @tune="(more, w) => tune(s, more, w)" @follow="emit('follow', $event)" /></li>
          </ul>
        </section>

        <div class="order-3 flex min-w-0 flex-col gap-6 lg:order-none print:gap-3">
          <section v-for="sec in issue.sections" :id="sectionAnchor(sec)" :key="sec.topic_id" class="flex scroll-mt-24 flex-col" :aria-label="sec.name" data-test="section">
            <header class="flex flex-wrap items-center gap-x-3 gap-y-1 border-t border-hairline pt-3">
              <NewsroomPlate :label="sec.name" :category="sec.category" />
              <h2 class="m-0 min-w-0 break-words font-mono text-[length:var(--text-body)] font-semibold uppercase tracking-wider text-primary">{{ sec.name }}</h2>
              <span class="ml-auto text-[length:var(--text-small)] text-faint">{{ briefSectionCountWords(sec.stories.length, inTopCount(sec)) }}<template v-if="sec.as_of"> · {{ briefAsOfWords(formatRelativeTime(sec.as_of)) }}</template></span>
            </header>
            <p v-if="sec.note" class="m-0 pt-2 text-[length:var(--text-small)] text-state-running" data-test="section-note">{{ sec.note }}</p>
            <div v-if="sec.problems?.length" class="pt-2 text-[length:var(--text-small)] text-state-error" data-test="section-problems">
              <p class="m-0 font-medium">{{ BRIEF_PROBLEMS_LABEL }}</p>
              <ul class="m-0 list-none p-0"><li v-for="p in sec.problems" :key="p" class="break-words">{{ p }}</li></ul>
              <p class="m-0 pt-1 text-faint" data-test="problems-fix">{{ BRIEF_PROBLEMS_FIX }}</p>
              <RouterLink :to="toNewsroom('following')" class="text-accent-info-text hover:underline print:hidden" data-test="problems-open-setup">{{ BRIEF_PROBLEMS_ACTION }} →</RouterLink>
            </div>
            <p v-if="headlinesOnly(sec)" class="m-0 pt-2 text-[length:var(--text-small)] text-faint" data-test="headlines-only">{{ SECTION_HEADLINES_ONLY }}</p>
            <ul v-if="storiesOf(sec).length" class="m-0 grid list-none gap-x-6 p-0 md:grid-cols-2 2xl:grid-cols-3">
              <li v-for="s in shown(storiesOf(sec), sec.topic_id)" :key="s.pool_id" class="min-w-0 border-b border-hairline"><NewsroomBriefStory :story="s" :quiet="headlinesOnly(sec)" anchor :topic="topicName(s)" @tune="(more, w) => tune(s, more, w)" @follow="emit('follow', $event)" /></li>
            </ul>
            <p v-else class="m-0 py-3 text-muted">{{ sec.stories.length ? SECTION_IN_TOP : sec.as_of ? SECTION_EMPTY : SECTION_NEVER_READ }}</p>
            <BaseButton v-if="storiesOf(sec).length > BRIEF_SECTION_SHOWN" class-name="mt-2 self-start print:hidden" size="sm" variant="ghost" data-test="show-more" @click="flipAll(sec.topic_id)">
              {{ showAll(sec.topic_id) ? SHOW_FEWER_LABEL : showMoreWords(storiesOf(sec).length - BRIEF_SECTION_SHOWN) }}
            </BaseButton>
          </section>
          <footer class="flex flex-wrap items-center gap-3 border-t border-hairline pt-3 text-[length:var(--text-small)] text-muted" data-test="end-mark">
            <span class="inline-block h-2.5 w-2.5 flex-none bg-accent-brand" aria-hidden="true" />
            <span>{{ BRIEF_END_WORDS }} {{ status.next }}</span>
            <span class="ml-auto flex gap-2 print:hidden">
              <BaseButton size="sm" variant="ghost" data-test="print" @click="print">{{ BRIEF_PRINT_LABEL }}</BaseButton>
              <BaseButton size="sm" variant="ghost" data-test="copy-text" @click="copy">{{ BRIEF_COPY_LABEL }}</BaseButton>
            </span>
          </footer>
        </div>
      </div>

      <aside class="order-2 flex min-w-0 flex-col gap-4 lg:order-none print:mt-3" data-test="edition-side" :aria-label="BRIEF_BESIDE_LABEL">
        <NewsroomBriefAttention :lines="home.critical" stacked />
        <NewsroomBriefChips />
        <NewsroomEditionDay v-if="issue.day" :day="issue.day" />
        <NewsroomEditionWatches v-if="issue.watches?.length" :lines="issue.watches" />
        <NewsroomEditionHome v-if="homeRest.length" :lines="homeRest" />
        <NewsroomEditionModelUse />
      </aside>
    </div>
  </div>
</template>
