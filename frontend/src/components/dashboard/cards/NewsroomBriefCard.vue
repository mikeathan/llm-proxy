<script setup lang="ts">
import { computed } from "vue"
import { RouterLink } from "vue-router"
import SegmentedControl from "../../common/forms/SegmentedControl.vue"
import LoadingState from "../../common/feedback/LoadingState.vue"
import ErrorState from "../../common/feedback/ErrorState.vue"
import StatusTag from "../../common/display/StatusTag.vue"
import NewsroomBriefAttention from "../../newsroom/NewsroomBriefAttention.vue"
import NewsroomBriefChips from "../../newsroom/NewsroomBriefChips.vue"
import NewsroomBriefHeadline from "../../newsroom/NewsroomBriefHeadline.vue"
import NewsroomBriefIndex from "../../newsroom/NewsroomBriefIndex.vue"
import {
  BRIEF_ATTENTION_LABEL, BRIEF_CARD_EMPTY_ISSUE, BRIEF_CARD_NONE, BRIEF_CARD_OPEN, BRIEF_CARD_PICK_LABEL, BRIEF_CARD_RELOAD, BRIEF_CARD_SET_UP, BRIEF_CARD_WAITING, LEAD_STORY_LABEL, NEW_BRIEF_LABEL,
  briefCardCountWords, briefCardLineWords, briefMadeWords,
} from "../../../constants/newsroom"
import { useCardSize } from "../../../composables/dashboard/useCardSize"
import { useBriefCard } from "../../../composables/newsroom/useBriefCard"
import { toBrief, toNewsroom } from "../../../router/routes"
import { formatRelativeTime } from "../../../utils/format/time"
import type { BriefSection, BriefStory } from "../../../types/newsroom"

// Dashboard card · Brief: the newest written brief as a front page. Large: what needs a look, the lead, two headlines from each of up to three sections and the
// sections as links. Smaller sizes: the lead, then one line. Every headline and section opens the brief at that place (ADR-0064, ADR-0086): the card shows and
// links, and reading is on the Newsroom. A brief counts as read when the Newsroom shows it, never from here.
const COLUMNS = 3
const PER_COLUMN = 2
const size = useCardSize()
const { unread, editions, issue, error, picked, edition, latestOf, isNew, critical, needsAttention, lead, sectionOf, storyCount, sections } = useBriefCard()
const options = computed(() => (editions.value ?? []).map((e) => ({ value: String(e.id), label: unread.isUnread(e.id) ? `${e.name} · ${NEW_BRIEF_LABEL}` : e.name })))
// The columns of the front page: the sections with stories other than the lead, two headlines each, no story twice on the card.
const columns = computed(() => {
  const used = new Set(lead.value ? [lead.value.pool_id] : [])
  const out: { sec: BriefSection; stories: BriefStory[] }[] = []
  for (const sec of sections.value) {
    if (out.length === COLUMNS) break
    const stories = sec.stories.filter((s) => !used.has(s.pool_id)).slice(0, PER_COLUMN)
    stories.forEach((s) => used.add(s.pool_id))
    if (stories.length) out.push({ sec, stories })
  }
  return out
})
const leadSection = computed(() => (lead.value ? sectionOf.value.get(lead.value.pool_id)?.name : undefined))
</script>

<template>
  <div class="flex min-w-0 flex-col gap-3" data-test="brief-card" :data-unread="isNew ? 'true' : undefined">
    <ErrorState v-if="error && !editions" title="The brief could not be read" :cause="error" :next="BRIEF_CARD_RELOAD" />
    <LoadingState v-else-if="!editions" label="Reading the brief" :rows="2" />
    <p v-else-if="!edition" class="m-0 flex flex-wrap items-center gap-3 text-muted" data-test="brief-card-none">
      {{ BRIEF_CARD_NONE }}
      <RouterLink :to="toNewsroom('following')" class="text-accent-brand hover:underline">{{ BRIEF_CARD_SET_UP }}</RouterLink>
    </p>
    <template v-else>
      <SegmentedControl v-if="size !== 'xs' && editions.length > 1" :model-value="String(edition.id)" :options="options" :label="BRIEF_CARD_PICK_LABEL" variant="compact" @update:model-value="picked = Number($event)" />
      <p v-if="!latestOf" class="m-0 text-muted" data-test="brief-card-waiting">{{ BRIEF_CARD_WAITING(edition.ready_by) }}</p>
      <p v-else-if="!issue" class="m-0 text-muted">…</p>

      <!-- Smallest: one line. -->
      <p v-else-if="size === 'xs'" class="m-0 flex flex-wrap items-center gap-2 text-primary" data-test="brief-card-line">
        <StatusTag v-if="needsAttention" state="error" :label="BRIEF_ATTENTION_LABEL" /><StatusTag v-if="isNew" state="info" :label="NEW_BRIEF_LABEL" />
        <RouterLink :to="toBrief({ edition: edition.id })" class="text-primary hover:underline">{{ briefCardLineWords(issue.name, storyCount) }}</RouterLink>
      </p>

      <template v-else>
        <header class="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1 border-t-2 border-text-primary pt-2">
          <h3 class="m-0 flex flex-wrap items-center gap-2 break-words text-[length:var(--text-h3,1.25rem)] font-bold leading-tight text-primary" data-test="brief-card-name">
            {{ issue.name }}<StatusTag v-if="isNew" state="info" :label="NEW_BRIEF_LABEL" /><StatusTag v-if="needsAttention" state="error" :label="BRIEF_ATTENTION_LABEL" />
          </h3>
          <span class="font-mono text-[length:var(--text-small)] uppercase tracking-wider text-faint">{{ briefMadeWords(formatRelativeTime(issue.made_at), issue.model_calls) }} · {{ briefCardCountWords(storyCount, sections.length) }}</span>
        </header>
        <NewsroomBriefAttention :lines="critical" />
        <NewsroomBriefChips />
        <p v-if="!lead" class="m-0 text-muted">{{ BRIEF_CARD_EMPTY_ISSUE }}</p>
        <div v-else :class="['grid min-w-0 items-start gap-x-6 gap-y-2', size === 'sm' ? '' : 'lg:grid-cols-[minmax(0,2fr)_minmax(0,3fr)]']" data-test="brief-card-front">
          <div class="min-w-0" data-test="brief-card-lead">
            <span class="font-mono text-[length:var(--text-small)] uppercase tracking-wider text-accent-brand">{{ LEAD_STORY_LABEL }}<template v-if="leadSection"> · {{ leadSection }}</template></span>
            <NewsroomBriefHeadline :story="lead" :edition="edition.id" lead />
            <p v-if="lead.summary" class="m-0 break-words text-[length:var(--text-small)] text-secondary">{{ lead.summary }}</p>
          </div>
          <ul v-if="size !== 'sm' && columns.length" class="m-0 grid min-w-0 list-none gap-x-6 p-0 md:grid-cols-3" data-test="brief-card-columns">
            <li v-for="c in columns" :key="c.sec.topic_id" class="flex min-w-0 flex-col divide-y divide-hairline">
              <NewsroomBriefHeadline v-for="(s, i) in c.stories" :key="s.pool_id" :story="s" :edition="edition.id" :section="i === 0 ? c.sec.name : undefined" />
            </li>
          </ul>
        </div>
        <div class="flex flex-wrap items-center justify-between gap-x-4">
          <NewsroomBriefIndex v-if="size !== 'sm'" class="flex-1" :sections="sections" :edition="edition.id" />
          <RouterLink :to="toBrief({ edition: edition.id })" class="inline-flex min-h-11 items-center text-[length:var(--text-small)] text-accent-brand hover:underline sm:min-h-8" data-test="brief-card-open">{{ BRIEF_CARD_OPEN }} →</RouterLink>
        </div>
      </template>
    </template>
  </div>
</template>
