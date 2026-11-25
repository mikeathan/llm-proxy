<script setup lang="ts">
import { computed } from "vue"
import { RouterLink } from "vue-router"
import LoadingState from "../../common/feedback/LoadingState.vue"
import ErrorState from "../../common/feedback/ErrorState.vue"
import StatusTag from "../../common/display/StatusTag.vue"
import NewsroomBriefAttention from "../../newsroom/NewsroomBriefAttention.vue"
import NewsroomBriefChips from "../../newsroom/NewsroomBriefChips.vue"
import NewsroomBriefHeadline from "../../newsroom/NewsroomBriefHeadline.vue"
import NewsroomBriefIndex from "../../newsroom/NewsroomBriefIndex.vue"
import {
  BRIEF_ATTENTION_LABEL, BRIEF_CARD_NONE, BRIEF_CARD_RELOAD, BRIEF_CARD_SET_UP, BRIEF_CARD_WAITING, DAY_NONE, NEW_BRIEF_LABEL, TODAY_CARD_READ, briefMadeWords,
} from "../../../constants/newsroom"
import { useCardSize } from "../../../composables/dashboard/useCardSize"
import { useBriefCard } from "../../../composables/newsroom/useBriefCard"
import { toBrief, toNewsroom } from "../../../router/routes"
import { formatRelativeTime } from "../../../utils/format/time"

// Dashboard card · Today (Home): the short version of the newest brief for a Home that mixes everything: what needs a look first, your next reminders, the lead
// headline and one more, and the sections as links. Each opens the brief at that place (ADR-0064, ADR-0086); this card never marks it read.
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
      <RouterLink :to="toNewsroom('following')" class="text-accent-brand hover:underline">{{ BRIEF_CARD_SET_UP }}</RouterLink>
    </p>
    <p v-else-if="!latestOf" class="m-0 text-muted" data-test="today-waiting">{{ BRIEF_CARD_WAITING(edition.ready_by) }}</p>
    <p v-else-if="!issue" class="m-0 text-muted">…</p>
    <template v-else>
      <header class="flex flex-wrap items-start justify-between gap-x-4 gap-y-1">
        <div class="flex min-w-0 flex-col">
          <span class="flex flex-wrap items-center gap-2 font-semibold text-primary" data-test="today-name">{{ issue.name }}<StatusTag v-if="isNew" state="info" :label="NEW_BRIEF_LABEL" /><StatusTag v-if="needsAttention" state="error" :label="BRIEF_ATTENTION_LABEL" /></span>
          <span class="text-[length:var(--text-small)] text-faint">{{ briefMadeWords(formatRelativeTime(issue.made_at), issue.model_calls) }}</span>
        </div>
        <RouterLink :to="toBrief({ edition: edition.id })" class="inline-flex min-h-11 items-center text-[length:var(--text-small)] text-accent-brand hover:underline sm:min-h-8" data-test="today-open">{{ TODAY_CARD_READ }} →</RouterLink>
      </header>
      <NewsroomBriefAttention :lines="critical" />
      <template v-if="size !== 'xs'">
        <NewsroomBriefChips />
        <ul v-if="items.length" class="m-0 flex list-none flex-col p-0" data-test="today-day">
          <li v-for="(item, i) in items" :key="i" class="flex min-w-0 items-baseline gap-3 py-1 text-[length:var(--text-small)]">
            <span class="min-w-12 flex-none font-mono font-medium tabular-nums text-accent-brand">{{ item.clock }}</span><span class="min-w-0 break-words text-secondary">{{ item.title }}</span>
          </li>
        </ul>
        <p v-else-if="issue.day && !issue.day.unavailable" class="m-0 text-[length:var(--text-small)] text-muted" data-test="today-day-none">{{ DAY_NONE }}</p>
      </template>
      <div v-if="lead" class="flex min-w-0 flex-col divide-y divide-hairline border-t border-hairline" data-test="today-lead">
        <NewsroomBriefHeadline :story="lead" :edition="edition.id" :section="sectionOf.get(lead.pool_id)?.name" />
        <NewsroomBriefHeadline v-if="second && size !== 'sm' && size !== 'xs'" :story="second" :edition="edition.id" :section="sectionOf.get(second.pool_id)?.name" />
      </div>
      <NewsroomBriefIndex v-if="size !== 'xs'" class="border-t border-hairline pt-1" :sections="sections" :edition="edition.id" data-test="today-sections" />
    </template>
  </div>
</template>
