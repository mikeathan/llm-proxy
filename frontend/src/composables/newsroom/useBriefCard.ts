import { computed, ref, watch } from 'vue'
import { sharedPoll } from '../ui/useSharedPoll'
import { useBriefUnread } from './useBriefUnread'
import { BriefService } from '../../services/newsroom/briefService'
import { BRIEF_CARD_LOAD_FAILED } from '../../constants/newsroom'
import { homeLevel } from '../../utils/newsroom/homeLines'
import type { BriefEdition, BriefHomeLine, BriefIssue, BriefSection, BriefStory } from '../../types/newsroom'

const REFRESH_MS = 60000

// Shared by every card that shows the newest brief (the Brief card, the Today card): one list of editions on one timer, and each written edition fetched once
// by id, because an edition never changes its stories after it is written.
const editions = ref<BriefEdition[] | null>(null)
const issues = ref<Record<number, BriefIssue>>({})
const error = ref<string | null>(null)
const asking = new Set<number>()
const reason = (e: unknown): string => (e instanceof Error && e.message ? e.message : BRIEF_CARD_LOAD_FAILED)

async function loadEditions() {
  try {
    editions.value = await BriefService.editions()
    error.value = null
  } catch (e) {
    error.value = reason(e)
  }
}
const poll = sharedPoll(loadEditions, REFRESH_MS)

async function ensureIssue(id: number) {
  if (issues.value[id] || asking.has(id)) return
  asking.add(id)
  try {
    const got = await BriefService.issue(id)
    issues.value = { ...issues.value, [id]: got }
  } catch (e) {
    error.value = reason(e)
  } finally {
    asking.delete(id)
  }
}

/**
 * What a dashboard card that shows the newest written brief needs: the editions, the pick, the newest issue of that edition and what can be said about it.
 * It only reads: a brief counts as read when the Newsroom shows it, never from a card, so the dot stays until it is really opened (ADR-0064).
 */
export function useBriefCard() {
  poll.use()
  const unread = useBriefUnread()
  const picked = ref<number | null>(null)

  const edition = computed(() => editions.value?.find((e) => e.id === picked.value) ?? editions.value?.find((e) => unread.isUnread(e.id)) ?? editions.value?.[0] ?? null)
  const latestOf = computed(() => unread.latest.value.find((l) => l.edition_id === edition.value?.id) ?? null)
  const issue = computed<BriefIssue | null>(() => (latestOf.value ? (issues.value[latestOf.value.issue_id] ?? null) : null))
  watch(() => latestOf.value?.issue_id ?? null, (id) => { if (id !== null) void ensureIssue(id) }, { immediate: true })

  const isNew = computed(() => latestOf.value !== null && !latestOf.value.read)
  // The facts that are serious on their own (the core's own ranking, ADR-0084) and the ones to check, so a card can say them without opening the Home block.
  const critical = computed<BriefHomeLine[]>(() => (issue.value?.home ?? []).filter((l) => homeLevel(l) === 'critical'))
  const toCheck = computed<BriefHomeLine[]>(() => (issue.value?.home ?? []).filter((l) => homeLevel(l) !== 'info'))
  const needsAttention = computed(() => critical.value.length > 0)
  const lead = computed<BriefStory | undefined>(() => issue.value?.top?.[0] ?? issue.value?.sections.find((s) => s.stories.length)?.stories[0])
  const otherTop = computed(() => (issue.value?.top ?? []).slice(1))
  const sectionOf = computed(() => new Map((issue.value?.sections ?? []).flatMap((sec) => sec.stories.map((s) => [s.pool_id, sec] as const))))
  const storyCount = computed(() => (issue.value?.sections ?? []).reduce((n, s) => n + s.stories.length, 0))
  const sections = computed<BriefSection[]>(() => (issue.value?.sections ?? []).filter((s) => s.stories.length))

  return { unread, editions, issue, error, picked, edition, latestOf, isNew, critical, toCheck, needsAttention, lead, otherTop, sectionOf, storyCount, sections }
}

/** Test seam: forgets the editions and the issues read, and stops the timer. */
export function resetBriefCard(): void {
  poll.reset()
  editions.value = null
  issues.value = {}
  error.value = null
  asking.clear()
}
