<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from "vue"
import { RouterLink } from "vue-router"
import BaseButton from "../../common/buttons/BaseButton.vue"
import SegmentedControl from "../../common/forms/SegmentedControl.vue"
import LoadingState from "../../common/feedback/LoadingState.vue"
import ErrorState from "../../common/feedback/ErrorState.vue"
import StatusTag from "../../common/display/StatusTag.vue"
import NewsroomBriefStory from "../../newsroom/NewsroomBriefStory.vue"
import NewsroomPlate from "../../newsroom/NewsroomPlate.vue"
import {
  BRIEF_CARD_COLLAPSE, BRIEF_CARD_EMPTY_ISSUE, BRIEF_CARD_EXPAND, BRIEF_CARD_LOAD_FAILED, BRIEF_CARD_NONE, BRIEF_CARD_OPEN, BRIEF_CARD_PICK_LABEL, BRIEF_CARD_RELOAD, BRIEF_CARD_SET_UP,
  BRIEF_CARD_WAITING, LEAD_STORY_LABEL, NEW_BRIEF_LABEL, briefCardCountWords, briefCardLineWords, briefMadeWords,
} from "../../../constants/newsroom"
import { useCardSize } from "../../../composables/dashboard/useCardSize"
import { useBriefUnread } from "../../../composables/newsroom/useBriefUnread"
import { usePolling } from "../../../composables/ui/usePolling"
import { toNewsroom } from "../../../router/routes"
import { BriefService } from "../../../services/newsroom/briefService"
import { formatRelativeTime } from "../../../utils/format/time"
import type { BriefEdition, BriefIssue, BriefStory } from "../../../types/newsroom"

// Dashboard card · Brief: the newest written brief, big when there is one. Large: the masthead (name, when it was written, a New tag while unread), the lead story with
// its art beside the other top stories, and a chip for each section; "Show every section" opens the whole edition inside the card (in two columns on a wide card, so it
// grows by less; "Show less" folds it back). Smaller sizes: the lead headline, then one line. It only shows and links (ADR-0064): a brief counts as read when the Newsroom's
// Briefs page shows it, never from here, so the dot on the tab stays until it is really opened.
const REFRESH_MS = 60000
const TOP_SHOWN = 3
const size = useCardSize()
const unread = useBriefUnread()
const editions = ref<BriefEdition[] | null>(null)
const issue = ref<BriefIssue | null>(null)
const error = ref<string | null>(null)
const picked = ref<number | null>(null)
const expanded = ref(false)

const edition = computed(() => editions.value?.find((e) => e.id === picked.value) ?? editions.value?.find((e) => unread.isUnread(e.id)) ?? editions.value?.[0] ?? null)
const latestOf = computed(() => unread.latest.value.find((l) => l.edition_id === edition.value?.id) ?? null)
const isNew = computed(() => latestOf.value !== null && !latestOf.value.read)

async function loadEditions() {
  try {
    editions.value = await BriefService.editions()
    error.value = null
  } catch (e) {
    error.value = e instanceof Error && e.message ? e.message : BRIEF_CARD_LOAD_FAILED
  }
}
// An edition never changes after it is written, so it is asked for once for each new issue id.
async function loadIssue(id: number | null) {
  if (id === null) {
    issue.value = null
    return
  }
  try {
    const got = await BriefService.issue(id)
    if (latestOf.value?.issue_id === id) issue.value = got
  } catch (e) {
    error.value = e instanceof Error && e.message ? e.message : BRIEF_CARD_LOAD_FAILED
  }
}
onMounted(() => void loadEditions())
usePolling(() => void loadEditions(), REFRESH_MS)
watch(() => latestOf.value?.issue_id ?? null, (id) => { expanded.value = false; void loadIssue(id) }, { immediate: true })

const lead = computed<BriefStory | undefined>(() => issue.value?.top?.[0] ?? issue.value?.sections.find((s) => s.stories.length)?.stories[0])
const otherTop = computed(() => (issue.value?.top ?? []).slice(1, 1 + TOP_SHOWN))
const sectionOf = computed(() => new Map((issue.value?.sections ?? []).flatMap((sec) => sec.stories.map((s) => [s.pool_id, sec.name] as const))))
const storyCount = computed(() => (issue.value?.sections ?? []).reduce((n, s) => n + s.stories.length, 0))
const sections = computed(() => (issue.value?.sections ?? []).filter((s) => s.stories.length))
// A chip opens the whole edition and brings its section into view.
async function openSection(topicId: number) {
  expanded.value = true
  await nextTick()
  document.getElementById(`brief-section-${topicId}`)?.scrollIntoView({ block: "nearest", behavior: "smooth" })
}
const options = computed(() => (editions.value ?? []).map((e) => ({ value: String(e.id), label: unread.isUnread(e.id) ? `${e.name} · ${NEW_BRIEF_LABEL}` : e.name })))
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
        <StatusTag v-if="isNew" state="running" :label="NEW_BRIEF_LABEL" />{{ briefCardLineWords(issue.name, storyCount) }}
      </p>

      <!-- Medium: the lead headline and the count. -->
      <div v-else-if="size === 'sm'" class="flex min-w-0 flex-col gap-1" data-test="brief-card-small">
        <p class="m-0 flex flex-wrap items-center gap-2 text-[length:var(--text-small)] text-faint">
          <StatusTag v-if="isNew" state="running" :label="NEW_BRIEF_LABEL" /><span class="font-medium text-secondary">{{ issue.name }}</span>
          <span>{{ briefCardCountWords(storyCount, sections.length) }}</span>
        </p>
        <NewsroomBriefStory v-if="lead" :story="lead" quiet />
        <p v-else class="m-0 text-muted">{{ BRIEF_CARD_EMPTY_ISSUE }}</p>
        <RouterLink :to="toNewsroom('briefs')" class="self-start text-[length:var(--text-small)] text-accent-brand hover:underline">{{ BRIEF_CARD_OPEN }}</RouterLink>
      </div>

      <!-- Large: masthead, lead with art, the other top stories, section chips, and the whole edition on request. -->
      <div v-else class="flex min-w-0 flex-col gap-4" data-test="brief-card-large">
        <header class="flex flex-wrap items-end justify-between gap-x-4 gap-y-1 border-t-2 border-accent-brand pt-2">
          <div class="flex min-w-0 flex-col">
            <h3 class="m-0 flex flex-wrap items-center gap-2 break-words text-[length:var(--text-h3,1.25rem)] font-semibold leading-tight text-primary" data-test="brief-card-name">
              {{ issue.name }}<StatusTag v-if="isNew" state="running" :label="NEW_BRIEF_LABEL" />
            </h3>
            <span class="text-[length:var(--text-small)] text-faint">{{ briefMadeWords(formatRelativeTime(issue.made_at), issue.model_calls) }} · {{ briefCardCountWords(storyCount, sections.length) }}</span>
          </div>
          <RouterLink :to="toNewsroom('briefs')" class="text-[length:var(--text-small)] text-accent-brand hover:underline" data-test="brief-card-open">{{ BRIEF_CARD_OPEN }}</RouterLink>
        </header>

        <p v-if="!lead" class="m-0 text-muted">{{ BRIEF_CARD_EMPTY_ISSUE }}</p>
        <div v-else class="grid min-w-0 items-start gap-4 md:grid-cols-2">
          <div class="flex min-w-0 items-start gap-4 rounded-[var(--radius-md)] bg-surface-raised p-4" data-test="brief-card-lead">
            <div class="min-w-0 flex-1">
              <p class="m-0 text-[length:var(--text-small)] font-medium uppercase tracking-wide text-accent-brand">{{ LEAD_STORY_LABEL }}</p>
              <NewsroomBriefStory :story="lead" :section="sectionOf.get(lead.pool_id)" lead quiet />
            </div>
            <NewsroomPlate class="hidden sm:block" :label="issue.name" size="lg" />
          </div>
          <ul v-if="otherTop.length" class="m-0 flex min-w-0 list-none flex-col divide-y divide-hairline p-0" data-test="brief-card-top">
            <li v-for="s in otherTop" :key="s.pool_id"><NewsroomBriefStory :story="s" :section="sectionOf.get(s.pool_id)" quiet /></li>
          </ul>
        </div>

        <ul v-if="sections.length" class="m-0 flex list-none flex-wrap gap-2 p-0" aria-label="Sections">
          <li v-for="sec in sections" :key="sec.topic_id">
            <button type="button" class="flex items-center gap-2 rounded-[var(--radius-md)] border border-hairline bg-transparent py-1 pl-1.5 pr-3 text-left text-[length:var(--text-small)] text-secondary hover:bg-surface-hover focus-visible:outline-none focus-visible:ring-2" data-test="brief-card-chip" @click="openSection(sec.topic_id)">
              <NewsroomPlate :label="sec.name" :category="sec.category" /><span class="font-medium text-primary">{{ sec.name }}</span><span class="text-faint">{{ sec.stories.length }}</span>
            </button>
          </li>
        </ul>

        <div v-if="expanded" class="grid min-w-0 items-start gap-x-6 gap-y-4 md:grid-cols-2" data-test="brief-card-all">
          <section v-for="sec in sections" :id="`brief-section-${sec.topic_id}`" :key="sec.topic_id" class="flex min-w-0 scroll-mt-20 flex-col" :aria-label="sec.name">
            <h4 class="m-0 flex items-center gap-2 text-[length:var(--text-body)] font-medium text-primary"><NewsroomPlate :label="sec.name" :category="sec.category" />{{ sec.name }}</h4>
            <ul class="m-0 flex list-none flex-col divide-y divide-hairline p-0">
              <li v-for="s in sec.stories" :key="s.pool_id"><NewsroomBriefStory :story="s" quiet /></li>
            </ul>
          </section>
        </div>
        <BaseButton v-if="sections.length" class-name="self-start" size="sm" variant="secondary" :aria-expanded="expanded" data-test="brief-card-toggle" @click="expanded = !expanded">
          {{ expanded ? BRIEF_CARD_COLLAPSE : BRIEF_CARD_EXPAND }}
        </BaseButton>
      </div>
    </template>
  </div>
</template>
