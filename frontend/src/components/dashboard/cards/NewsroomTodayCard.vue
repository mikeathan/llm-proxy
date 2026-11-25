<script setup lang="ts">
import { computed } from "vue"
import { RouterLink } from "vue-router"
import LoadingState from "../../common/feedback/LoadingState.vue"
import ErrorState from "../../common/feedback/ErrorState.vue"
import StatusTag from "../../common/display/StatusTag.vue"
import NewsroomBriefChips from "../../newsroom/NewsroomBriefChips.vue"
import NewsroomBriefStory from "../../newsroom/NewsroomBriefStory.vue"
import {
  BRIEF_ATTENTION_LABEL, BRIEF_CARD_NONE, BRIEF_CARD_RELOAD, BRIEF_CARD_SET_UP, BRIEF_CARD_WAITING, DAY_NONE, NEW_BRIEF_LABEL, TODAY_CARD_READ, briefMadeWords,
} from "../../../constants/newsroom"
import { useCardSize } from "../../../composables/dashboard/useCardSize"
import { useBriefCard } from "../../../composables/newsroom/useBriefCard"
import { toNewsroom } from "../../../router/routes"
import { formatRelativeTime } from "../../../utils/format/time"
import { homeLineRoute } from "../../../utils/newsroom/homeLines"

// Dashboard card · Today (Home): the short version of the newest brief for a Home that mixes everything: what needs a look first, your next reminders, the lead
// headline and one more, and a link into each section. Reading the brief in full stays on the Newsroom (ADR-0064): this card shows and links, and never marks it read.
const REMINDERS_SHOWN = 3
const size = useCardSize()
const { editions, issue, error, edition, latestOf, isNew, critical, needsAttention, lead, otherTop, sectionOf, sections } = useBriefCard()
const items = computed(() => issue.value?.day?.items.slice(0, REMINDERS_SHOWN) ?? [])
const second = computed(() => otherTop.value[0])
</script>

<template>
  <div class="flex min-w-0 flex-col gap-3" data-test="today-card" :data-unread="isNew ? 'true' : undefined">
    <ErrorState v-if="error && !editions" title="The brief could not be read" :cause="error" :next="BRIEF_CARD_RELOAD" />
    <LoadingState v-else-if="!editions" label="Reading the brief" :rows="2" />
    <p v-else-if="!edition" class="m-0 flex flex-wrap items-center gap-3 text-muted" data-test="today-none">
      {{ BRIEF_CARD_NONE }}
      <RouterLink :to="toNewsroom('briefs')" class="text-accent-brand hover:underline">{{ BRIEF_CARD_SET_UP }}</RouterLink>
    </p>
    <p v-else-if="!latestOf" class="m-0 text-muted" data-test="today-waiting">{{ BRIEF_CARD_WAITING(edition.ready_by) }}</p>
    <p v-else-if="!issue" class="m-0 text-muted">…</p>
    <template v-else>
      <header class="flex flex-wrap items-start justify-between gap-x-4 gap-y-1">
        <div class="flex min-w-0 flex-col">
          <span class="flex flex-wrap items-center gap-2 font-medium text-primary" data-test="today-name">{{ issue.name }}<StatusTag v-if="isNew" state="running" :label="NEW_BRIEF_LABEL" /><StatusTag v-if="needsAttention" state="error" :label="BRIEF_ATTENTION_LABEL" /></span>
          <span class="text-[length:var(--text-small)] text-faint">{{ briefMadeWords(formatRelativeTime(issue.made_at), issue.model_calls) }}</span>
        </div>
        <RouterLink :to="toNewsroom('briefs')" class="inline-flex min-h-11 items-center text-[length:var(--text-small)] text-accent-brand hover:underline sm:min-h-8" data-test="today-open">{{ TODAY_CARD_READ }} →</RouterLink>
      </header>
      <ul v-if="critical.length" class="m-0 flex list-none flex-col gap-1 p-0" data-test="today-attention">
        <li v-for="l in critical" :key="l.id" class="flex min-w-0 flex-col gap-1 rounded-[var(--radius-md)] border border-state-error bg-surface-raised px-3 py-2 text-[length:var(--text-small)] sm:flex-row sm:items-start sm:gap-3">
          <span class="min-w-0 break-words text-primary sm:flex-1">{{ l.words }}</span>
          <RouterLink :to="homeLineRoute(l)" class="inline-flex min-h-11 flex-none items-center text-accent-info-text hover:underline sm:min-h-0">Look →</RouterLink>
        </li>
      </ul>
      <NewsroomBriefChips v-if="size !== 'xs'" />
      <template v-if="size !== 'xs'">
        <ul v-if="items.length" class="m-0 flex list-none flex-col p-0" data-test="today-day">
          <li v-for="(item, i) in items" :key="i" class="flex min-w-0 items-baseline gap-3 py-1 text-[length:var(--text-small)]">
            <span class="min-w-12 flex-none font-medium tabular-nums text-accent-brand">{{ item.clock }}</span><span class="min-w-0 break-words text-secondary">{{ item.title }}</span>
          </li>
        </ul>
        <p v-else-if="issue.day && !issue.day.unavailable" class="m-0 text-[length:var(--text-small)] text-muted" data-test="today-day-none">{{ DAY_NONE }}</p>
      </template>
      <div v-if="lead" class="flex min-w-0 flex-col divide-y divide-hairline border-t border-hairline">
        <NewsroomBriefStory :story="lead" :section="sectionOf.get(lead.pool_id)" quiet data-test="today-lead" />
        <NewsroomBriefStory v-if="second && size !== 'sm' && size !== 'xs'" :story="second" :section="sectionOf.get(second.pool_id)" quiet />
      </div>
      <ul v-if="sections.length && size !== 'xs'" class="m-0 flex list-none flex-wrap gap-2 p-0" aria-label="Sections" data-test="today-sections">
        <li v-for="sec in sections" :key="sec.topic_id">
          <RouterLink :to="toNewsroom('briefs')" class="inline-flex min-h-11 items-center gap-2 rounded-[var(--radius-md)] border border-hairline px-3 text-[length:var(--text-small)] text-primary hover:bg-surface-hover sm:min-h-8">{{ sec.name }}<span class="text-faint">{{ sec.stories.length }}</span></RouterLink>
        </li>
      </ul>
    </template>
  </div>
</template>
