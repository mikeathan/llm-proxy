import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { RouterLinkStub, flushPromises, mount } from '@vue/test-utils'
import { ref } from 'vue'

const service = vi.hoisted(() => ({ editions: vi.fn(), latest: vi.fn(), issue: vi.fn(), markRead: vi.fn() }))
vi.mock('../../../services/newsroom/briefService', () => ({ BriefService: service }))
// The chips for failed runs and unsent alerts read two other services; a quiet day has none.
vi.mock('../../../services/automation/dispatcherService', () => ({ DispatcherService: { getGlobalActivity: vi.fn().mockResolvedValue([]) } }))
vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: { alerts: vi.fn().mockResolvedValue({ cursor: 0, items: [] }) } }))

import NewsroomTodayCard from '../../../components/dashboard/cards/NewsroomTodayCard.vue'
import { DASHBOARD_CARD_SIZE } from '../../../constants/dashboard'
import { resetBriefUnread } from '../../../composables/newsroom/useBriefUnread'
import { resetBriefCard } from '../../../composables/newsroom/useBriefCard'
import { resetBriefChips } from '../../../composables/newsroom/useBriefChips'
import { toBrief, toNewsroom } from '../../../router/routes'
import type { DashboardCardSize } from '../../../types/dashboard'
import type { BriefIssue, BriefStory } from '../../../types/newsroom'

const story = (n: number, over: Partial<BriefStory> = {}): BriefStory => ({ pool_id: n, headline: `Story ${n}`, outlets: ['BBC'], ...over })
const edition = (over = {}) => ({ id: 2, name: 'Morning', ready_by: '07:00', days: [], collect_every: '2h0m0s', collect_for: '12h0m0s', sections: [], enabled: true, stories_per_section: 5, top_stories: 4, repeat_after: '24h0m0s', max_story_age: '48h0m0s', ...over })
const latest = (over = {}) => ({ edition_id: 2, name: 'Morning', issue_id: 5, made_at: '2026-11-23T06:00:00Z', read: false, ...over })
const issue = (over: Partial<BriefIssue> = {}): BriefIssue => ({
  id: 5, edition_id: 2, name: 'Morning', made_at: '2026-11-23T06:00:00Z', model_calls: 2,
  top: [story(1, { headline: 'Lead story', summary: 'What it says.' }), story(2), story(3)],
  sections: [{ kind: 'news', topic_id: 7, name: 'Technology', stories: [story(1), story(2), story(4)] }, { kind: 'news', topic_id: 8, name: 'World', stories: [story(5)] }],
  ...over,
})
// The brief and the unread state are shared by every card on screen, so each test starts them afresh.
const mounted: { unmount(): void }[] = []
afterEach(() => {
  mounted.splice(0).forEach((w) => w.unmount())
  resetBriefCard()
  resetBriefChips()
})
const mountCard = async (size: DashboardCardSize = 'lg') => {
  const w = mount(NewsroomTodayCard, { global: { stubs: { RouterLink: RouterLinkStub }, provide: { [DASHBOARD_CARD_SIZE as symbol]: ref(size) } } })
  mounted.push(w)
  await flushPromises()
  return w
}
const links = (w: ReturnType<typeof mount>) => w.findAllComponents(RouterLinkStub).map((l) => l.props('to'))

beforeEach(() => {
  vi.clearAllMocks()
  resetBriefUnread()
  service.editions.mockResolvedValue([edition()])
  service.latest.mockResolvedValue([latest()])
  service.issue.mockResolvedValue(issue())
})

describe('NewsroomTodayCard', () => {
  it('shows the lead and one more headline, a link into each section, and never marks the brief read', async () => {
    const w = await mountCard()
    expect(w.get('[data-test="today-name"]').text()).toContain('Morning')
    expect(w.get('[data-test="today-name"]').text()).toContain('New')
    expect(w.text()).toContain('Lead story')
    expect(w.text()).toContain('Story 2')
    expect(w.get('[data-test="today-sections"]').text()).toContain('Technology')
    expect(w.get('[data-test="today-open"]').text()).toContain('Read the brief')
    expect(links(w)).toContainEqual(toBrief({ edition: 2 }))
    expect(links(w)).toContainEqual(toBrief({ edition: 2, section: 7 }))
    expect(links(w)).toContainEqual(toBrief({ edition: 2, story: 1 }))
    expect(service.markRead).not.toHaveBeenCalled()
  })

  it('puts what needs a look first, and the next reminders after it', async () => {
    service.issue.mockResolvedValue(issue({
      home: [{ id: 'radio_open_or_weak_ap', count: 1, words: 'An open access point.', level: 'critical' }],
      day: { items: [{ clock: '08:30', title: 'Dentist' }, { clock: '12:00', title: 'Call' }, { clock: '15:00', title: 'Bins' }, { clock: '19:00', title: 'Fourth' }] },
    }))
    const w = await mountCard()
    const attention = w.get('[data-test="brief-attention"]')
    const day = w.get('[data-test="today-day"]')
    expect(attention.text()).toContain('An open access point.')
    expect(attention.element.compareDocumentPosition(day.element) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(day.findAll('li')).toHaveLength(3)
    expect(day.text()).toContain('08:30')
  })

  it('says how to set one up, and when it is due when none is written yet', async () => {
    service.editions.mockResolvedValue([])
    const none = await mountCard()
    expect(none.get('[data-test="today-none"]').text()).toContain('No brief yet')
    expect(links(none)).toContainEqual(toNewsroom('following'))
    resetBriefCard()
    resetBriefUnread()
    service.editions.mockResolvedValue([edition()])
    service.latest.mockResolvedValue([])
    expect((await mountCard()).get('[data-test="today-waiting"]').text()).toContain('07:00')
  })
})
