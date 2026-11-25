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
import type { DashboardCardSize } from '../../../types/dashboard'
import type { BriefIssue, BriefStory } from '../../../types/newsroom'

const story = (n: number, over: Partial<BriefStory> = {}): BriefStory => ({ pool_id: n, headline: `Story ${n}`, outlets: ['BBC'], ...over })
const edition = { id: 2, name: 'Morning', ready_by: '07:00', days: [], collect_every: '2h0m0s', collect_for: '12h0m0s', sections: [], enabled: true }
const issue = (over: Partial<BriefIssue> = {}): BriefIssue => ({
  id: 5, edition_id: 2, name: 'Morning', made_at: '2026-11-23T06:00:00Z', model_calls: 2,
  top: [story(1, { headline: 'Lead story' }), story(2)],
  sections: [{ kind: 'news', topic_id: 7, name: 'Technology', stories: [story(1), story(4)] }],
  ...over,
})
const mounted: { unmount(): void }[] = []
afterEach(() => { mounted.splice(0).forEach((w) => w.unmount()) })
const mountCard = async (size: DashboardCardSize = 'lg') => {
  const w = mount(NewsroomTodayCard, { global: { stubs: { RouterLink: RouterLinkStub }, provide: { [DASHBOARD_CARD_SIZE as symbol]: ref(size) } } })
  mounted.push(w)
  await flushPromises()
  return w
}

beforeEach(() => {
  vi.clearAllMocks()
  resetBriefUnread()
  service.editions.mockResolvedValue([edition])
  service.latest.mockResolvedValue([{ edition_id: 2, name: 'Morning', issue_id: 5, made_at: '2026-11-23T06:00:00Z', read: false }])
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
    expect(service.markRead).not.toHaveBeenCalled()
  })

  it('puts what needs a look first, and the next reminders after it', async () => {
    service.issue.mockResolvedValue(issue({
      home: [{ id: 'radio_open_or_weak_ap', count: 1, words: 'Confirmed access points of yours that are open or use an outdated protection.', level: 'critical' }],
      day: { items: [{ clock: '14:00', title: 'Dentist' }, { clock: '17:30', title: 'Plumber' }, { clock: '18:00', title: 'C' }, { clock: '19:00', title: 'D' }] },
    }))
    const w = await mountCard()
    expect(w.get('[data-test="today-attention"]').text()).toContain('outdated protection')
    expect(w.get('[data-test="today-name"]').text()).toContain('Needs attention')
    expect(w.findAll('[data-test="today-day"] li')).toHaveLength(3)
    expect(w.get('[data-test="today-day"]').text()).toContain('Dentist')
  })

  it('shrinks: the small size drops the second headline and the section links', async () => {
    const w = await mountCard('sm')
    expect(w.text()).toContain('Lead story')
    expect(w.text()).not.toContain('Story 2')
    const xs = await mountCard('xs')
    expect(xs.find('[data-test="today-sections"]').exists()).toBe(false)
  })

  it('says how to set one up, and when it is due when none is written yet', async () => {
    service.editions.mockResolvedValue([])
    expect((await mountCard()).get('[data-test="today-none"]').text()).toContain('Set up a brief')
    resetBriefUnread()
    service.editions.mockResolvedValue([edition])
    service.latest.mockResolvedValue([])
    expect((await mountCard()).get('[data-test="today-waiting"]').text()).toContain('ready at 07:00')
  })
})
