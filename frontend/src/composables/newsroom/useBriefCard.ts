import { computed, onMounted, ref, watch } from 'vue'
import { usePolling } from '../ui/usePolling'
import { useBriefUnread } from './useBriefUnread'
import { BriefService } from '../../services/newsroom/briefService'
import { BRIEF_CARD_LOAD_FAILED } from '../../constants/newsroom'
import { homeLevel } from '../../utils/newsroom/homeLines'
import type { BriefEdition, BriefHomeLine, BriefIssue, BriefSection, BriefStory } from '../../types/newsroom'

const REFRESH_MS = 60000

/**
 * What every dashboard card that shows the newest written brief needs: the editions, the pick, the newest issue of that edition (asked for once for each new issue
 * id, because an edition never changes after it is written) and what can be said about it. It only reads: a brief counts as read when the Newsroom's Briefs page
 * shows it, never from a card, so the dot on the tab stays until it is really opened (ADR-0064).
 */
export function useBriefCard(onNewIssue?: () => void) {
  const unread = useBriefUnread()
  const editions = ref<BriefEdition[] | null>(null)
  const issue = ref<BriefIssue | null>(null)
  const error = ref<string | null>(null)
  const picked = ref<number | null>(null)

  const edition = computed(() => editions.value?.find((e) => e.id === picked.value) ?? editions.value?.find((e) => unread.isUnread(e.id)) ?? editions.value?.[0] ?? null)
  const latestOf = computed(() => unread.latest.value.find((l) => l.edition_id === edition.value?.id) ?? null)
  const isNew = computed(() => latestOf.value !== null && !latestOf.value.read)
  // The facts that are serious on their own (the core's own ranking, ADR-0084) and the ones to check, so a card can say them without opening the Home block.
  const critical = computed<BriefHomeLine[]>(() => (issue.value?.home ?? []).filter((l) => homeLevel(l) === 'critical'))
  const toCheck = computed<BriefHomeLine[]>(() => (issue.value?.home ?? []).filter((l) => homeLevel(l) !== 'info'))
  const needsAttention = computed(() => critical.value.length > 0)

  async function loadEditions() {
    try {
      editions.value = await BriefService.editions()
      error.value = null
    } catch (e) {
      error.value = e instanceof Error && e.message ? e.message : BRIEF_CARD_LOAD_FAILED
    }
  }
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
  // A card that keeps state about the issue (a selected tab) resets it through the callback when a new issue arrives.
  watch(() => latestOf.value?.issue_id ?? null, (id) => { onNewIssue?.(); void loadIssue(id) }, { immediate: true })

  const lead = computed<BriefStory | undefined>(() => issue.value?.top?.[0] ?? issue.value?.sections.find((s) => s.stories.length)?.stories[0])
  const otherTop = computed(() => (issue.value?.top ?? []).slice(1))
  const sectionOf = computed(() => new Map((issue.value?.sections ?? []).flatMap((sec) => sec.stories.map((s) => [s.pool_id, sec.name] as const))))
  const storyCount = computed(() => (issue.value?.sections ?? []).reduce((n, s) => n + s.stories.length, 0))
  const sections = computed<BriefSection[]>(() => (issue.value?.sections ?? []).filter((s) => s.stories.length))

  return { unread, editions, issue, error, picked, edition, latestOf, isNew, critical, toCheck, needsAttention, lead, otherTop, sectionOf, storyCount, sections }
}
