<script setup lang="ts">
import { computed, ref } from "vue"
import { RouterLink } from "vue-router"
import SegmentedControl from "../../common/forms/SegmentedControl.vue"
import LoadingState from "../../common/feedback/LoadingState.vue"
import ErrorState from "../../common/feedback/ErrorState.vue"
import StatusTag from "../../common/display/StatusTag.vue"
import NewsroomBriefStory from "../../newsroom/NewsroomBriefStory.vue"
import NewsroomPlate from "../../newsroom/NewsroomPlate.vue"
import NewsroomEditionDay from "../../newsroom/NewsroomEditionDay.vue"
import NewsroomEditionModelUse from "../../newsroom/NewsroomEditionModelUse.vue"
import NewsroomBriefChips from "../../newsroom/NewsroomBriefChips.vue"
import NewsroomEditionWatches from "../../newsroom/NewsroomEditionWatches.vue"
import {
  BRIEF_ATTENTION_LABEL, BRIEF_CARD_ALL_IN_BRIEF, BRIEF_CARD_EMPTY_ISSUE, BRIEF_CARD_HOME_QUIET, BRIEF_CARD_NONE, BRIEF_CARD_OPEN, BRIEF_CARD_PICK_LABEL, BRIEF_CARD_RELOAD, BRIEF_CARD_SET_UP,
  BRIEF_CARD_SIDE_LABEL, BRIEF_CARD_TABS_LABEL, BRIEF_CARD_TOP_TAB, BRIEF_CARD_WAITING, LEAD_STORY_LABEL, NEW_BRIEF_LABEL, briefCardCountWords, briefCardHomeCheck, briefCardLineWords, briefMadeWords,
} from "../../../constants/newsroom"
import { useCardSize } from "../../../composables/dashboard/useCardSize"
import { useBriefCard } from "../../../composables/newsroom/useBriefCard"
import { toNewsroom } from "../../../router/routes"
import { formatRelativeTime } from "../../../utils/format/time"
import { homeLineRoute } from "../../../utils/newsroom/homeLines"

// Dashboard card · Brief: the newest written brief. Large: a masthead, a line for anything that needs a look, then tabs (Top stories and one per section) that switch the
// body in place, with a side column for your day, the home and the model use. A tab only changes what is shown inside the card: nothing opens off-screen, and the one
// link out opens the whole brief. Smaller sizes: the lead headline, then one line. It only shows and links (ADR-0064): a brief counts as read when the Newsroom's Briefs
// page shows it, never from here, so the dot on the tab stays until it is really opened.
const TOP_SHOWN = 3
const SECTION_SHOWN = 4
const size = useCardSize()
const tab = ref<"top" | number>("top")
const c = useBriefCard(() => (tab.value = "top"))

const { unread, editions, issue, error, picked, edition, latestOf, isNew, critical, toCheck, needsAttention, lead, otherTop, sectionOf, storyCount, sections } = c
const hasSide = computed(() => !!(issue.value?.day || issue.value?.home?.length || issue.value?.watches?.length))
const hasTop = computed(() => !!lead.value)
const active = computed(() => (tab.value === "top" || !sections.value.some((s) => s.topic_id === tab.value) ? "top" : tab.value))
const activeSection = computed(() => sections.value.find((s) => s.topic_id === active.value))
const sectionStories = computed(() => activeSection.value?.stories.slice(0, SECTION_SHOWN) ?? [])
const topList = computed(() => otherTop.value.slice(0, TOP_SHOWN))
const options = computed(() => (editions.value ?? []).map((e) => ({ value: String(e.id), label: unread.isUnread(e.id) ? `${e.name} · ${NEW_BRIEF_LABEL}` : e.name })))
const homeSaid = computed(() => (toCheck.value.length ? briefCardHomeCheck(toCheck.value.length) : BRIEF_CARD_HOME_QUIET))
const homeTo = computed(() => (toCheck.value[0] ? homeLineRoute(toCheck.value[0]) : null))

const tabId = (key: "top" | number) => `brief-tab-${key}`
const keys = computed<Array<"top" | number>>(() => [...(hasTop.value ? (["top"] as const) : []), ...sections.value.map((s) => s.topic_id)])
// Left and right move between tabs, as a tab list does; the focus follows the selection.
function moveTab(step: number) {
  const list = keys.value
  const next = list[(list.indexOf(active.value) + step + list.length) % list.length]
  if (next === undefined) return
  tab.value = next
  document.getElementById(tabId(next))?.focus()
}
</script>

<template>
  <div class="flex min-w-0 flex-col gap-3" data-test="brief-card" :data-unread="isNew ? 'true' : undefined">
    <ErrorState v-if="error && !editions" title="The brief could not be read" :cause="error" :next="BRIEF_CARD_RELOAD" />
    <LoadingState v-else-if="!editions" label="Reading the brief" :rows="2" />
    <p v-else-if="!edition" class="m-0 flex flex-wrap items-center gap-3 text-muted" data-test="brief-card-none">
      {{ BRIEF_CARD_NONE }}
      <RouterLink :to="toNewsroom('briefs')" class="text-accent-brand hover:underline">{{ BRIEF_CARD_SET_UP }}</RouterLink>
    </p>
    <template v-else>
      <SegmentedControl v-if="size !== 'xs' && editions.length > 1" :model-value="String(edition.id)" :options="options" :label="BRIEF_CARD_PICK_LABEL" variant="compact" @update:model-value="picked = Number($event)" />
      <p v-if="!latestOf" class="m-0 text-muted" data-test="brief-card-waiting">{{ BRIEF_CARD_WAITING(edition.ready_by) }}</p>
      <p v-else-if="!issue" class="m-0 text-muted">…</p>

      <!-- Smallest: one line. -->
      <p v-else-if="size === 'xs'" class="m-0 flex flex-wrap items-center gap-2 text-primary" data-test="brief-card-line">
        <StatusTag v-if="needsAttention" state="error" :label="BRIEF_ATTENTION_LABEL" /><StatusTag v-if="isNew" state="running" :label="NEW_BRIEF_LABEL" />{{ briefCardLineWords(issue.name, storyCount) }}
      </p>

      <!-- Medium: the lead headline and the count. -->
      <div v-else-if="size === 'sm'" class="flex min-w-0 flex-col gap-1" data-test="brief-card-small">
        <p class="m-0 flex flex-wrap items-center gap-2 text-[length:var(--text-small)] text-faint">
          <StatusTag v-if="needsAttention" state="error" :label="BRIEF_ATTENTION_LABEL" /><StatusTag v-if="isNew" state="running" :label="NEW_BRIEF_LABEL" /><span class="font-medium text-secondary">{{ issue.name }}</span>
          <span>{{ briefCardCountWords(storyCount, sections.length) }}</span>
        </p>
        <NewsroomBriefStory v-if="lead" :story="lead" quiet />
        <p v-else class="m-0 text-muted">{{ BRIEF_CARD_EMPTY_ISSUE }}</p>
        <RouterLink :to="toNewsroom('briefs')" class="self-start text-[length:var(--text-small)] text-accent-brand hover:underline">{{ BRIEF_CARD_OPEN }}</RouterLink>
      </div>

      <!-- Large: masthead, what needs a look, tabs in place and a side column. -->
      <div v-else class="flex min-w-0 flex-col gap-3" data-test="brief-card-large">
        <header class="flex flex-wrap items-start justify-between gap-x-4 gap-y-1 border-t-2 border-accent-brand pt-2">
          <div class="flex min-w-0 flex-col">
            <h3 class="m-0 flex flex-wrap items-center gap-2 break-words text-[length:var(--text-h3,1.25rem)] font-semibold leading-tight text-primary" data-test="brief-card-name">
              {{ issue.name }}<StatusTag v-if="isNew" state="running" :label="NEW_BRIEF_LABEL" /><StatusTag v-if="needsAttention" state="error" :label="BRIEF_ATTENTION_LABEL" />
            </h3>
            <span class="text-[length:var(--text-small)] text-faint">{{ briefMadeWords(formatRelativeTime(issue.made_at), issue.model_calls) }} · {{ briefCardCountWords(storyCount, sections.length) }}</span>
          </div>
          <RouterLink :to="toNewsroom('briefs')" class="inline-flex min-h-11 items-center text-[length:var(--text-small)] text-accent-brand hover:underline sm:min-h-8" data-test="brief-card-open">{{ BRIEF_CARD_OPEN }} →</RouterLink>
        </header>

        <ul v-if="critical.length" class="m-0 flex list-none flex-col gap-1 p-0" data-test="brief-card-attention">
          <li v-for="l in critical" :key="l.id" class="flex min-w-0 flex-col gap-1 rounded-[var(--radius-md)] border border-state-error bg-surface-raised px-3 py-2 text-[length:var(--text-small)] sm:flex-row sm:items-start sm:gap-3">
            <span class="flex-none font-medium uppercase tracking-wide text-state-error">{{ BRIEF_ATTENTION_LABEL }}</span>
            <span class="min-w-0 break-words text-primary sm:flex-1">{{ l.words }}</span>
            <RouterLink :to="homeLineRoute(l)" class="inline-flex min-h-11 flex-none items-center text-accent-info-text hover:underline sm:min-h-0" data-test="brief-card-attention-look">Look →</RouterLink>
          </li>
        </ul>

        <NewsroomBriefChips />

        <p v-if="!hasTop" class="m-0 text-muted">{{ BRIEF_CARD_EMPTY_ISSUE }}</p>
        <div v-else :class="['grid min-w-0 items-start gap-4', hasSide ? 'lg:grid-cols-[minmax(0,1fr)_17rem]' : '']" data-test="brief-card-body">
          <div class="flex min-w-0 flex-col gap-3">
            <div role="tablist" :aria-label="BRIEF_CARD_TABS_LABEL" class="flex gap-0 overflow-x-auto border-b border-hairline" data-test="brief-card-tabs" @keydown.right.prevent="moveTab(1)" @keydown.left.prevent="moveTab(-1)">
              <button
                v-for="k in keys"
                :id="tabId(k)"
                :key="k"
                type="button"
                role="tab"
                :aria-selected="active === k"
                :aria-controls="'brief-panel'"
                :tabindex="active === k ? 0 : -1"
                :class="['-mb-px inline-flex min-h-11 flex-none items-center gap-2 border-0 border-b-2 bg-transparent px-3 text-[length:var(--text-small)] focus-visible:outline-none focus-visible:ring-2 sm:min-h-9', active === k ? 'border-accent-brand font-medium text-primary' : 'border-transparent text-muted hover:text-primary']"
                data-test="brief-card-tab"
                @click="tab = k"
              >
                <template v-if="k === 'top'">{{ BRIEF_CARD_TOP_TAB }}</template>
                <template v-else>{{ sections.find((s) => s.topic_id === k)?.name }}<span class="text-faint">{{ sections.find((s) => s.topic_id === k)?.stories.length }}</span></template>
              </button>
            </div>

            <div id="brief-panel" role="tabpanel" :aria-labelledby="tabId(active)" class="flex min-w-0 flex-col gap-2 lg:min-h-72" data-test="brief-card-panel">
              <template v-if="active === 'top' && lead">
                <div class="flex min-w-0 items-start gap-4 rounded-[var(--radius-md)] bg-surface-raised p-4" data-test="brief-card-lead">
                  <div class="min-w-0 flex-1">
                    <p class="m-0 text-[length:var(--text-small)] font-medium uppercase tracking-wide text-accent-brand">{{ LEAD_STORY_LABEL }}</p>
                    <NewsroomBriefStory :story="lead" :section="sectionOf.get(lead.pool_id)" lead quiet />
                  </div>
                  <NewsroomPlate class="hidden sm:block" :label="issue.name" size="lg" />
                </div>
                <ul v-if="topList.length" class="m-0 grid min-w-0 list-none gap-x-6 p-0 md:grid-cols-2" data-test="brief-card-top">
                  <li v-for="s in topList" :key="s.pool_id" class="border-t border-hairline first:border-t-0 md:[&:nth-child(2)]:border-t-0"><NewsroomBriefStory :story="s" :section="sectionOf.get(s.pool_id)" quiet /></li>
                </ul>
              </template>
              <template v-else-if="activeSection">
                <ul class="m-0 grid min-w-0 list-none gap-x-6 p-0 md:grid-cols-2" data-test="brief-card-section">
                  <li v-for="s in sectionStories" :key="s.pool_id" class="border-t border-hairline first:border-t-0 md:[&:nth-child(2)]:border-t-0"><NewsroomBriefStory :story="s" quiet /></li>
                </ul>
                <RouterLink v-if="activeSection.stories.length > SECTION_SHOWN" :to="toNewsroom('briefs')" class="self-start text-[length:var(--text-small)] text-accent-brand hover:underline">{{ BRIEF_CARD_ALL_IN_BRIEF(activeSection.stories.length) }} →</RouterLink>
              </template>
            </div>
          </div>

          <aside v-if="hasSide" class="flex min-w-0 flex-col gap-3" :aria-label="BRIEF_CARD_SIDE_LABEL" data-test="brief-card-beside">
            <NewsroomEditionDay v-if="issue.day" :day="issue.day" />
            <p v-if="issue.home?.length" class="m-0 flex flex-wrap items-center justify-between gap-2 rounded-[var(--radius-md)] bg-surface-raised px-3 py-2 text-[length:var(--text-small)] text-secondary" data-test="brief-card-home">
              <span>{{ homeSaid }}</span>
              <RouterLink v-if="homeTo" :to="homeTo" class="text-accent-info-text hover:underline">Look →</RouterLink>
            </p>
            <NewsroomEditionModelUse v-if="issue.home?.length" />
            <NewsroomEditionWatches v-if="issue.watches?.length" :lines="issue.watches" />
          </aside>
        </div>
      </div>
    </template>
  </div>
</template>
