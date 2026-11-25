import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { RouterLinkStub, flushPromises, mount } from '@vue/test-utils'
import { ref } from 'vue'

const service = vi.hoisted(() => ({ editions: vi.fn(), latest: vi.fn(), issue: vi.fn(), markRead: vi.fn() }))
vi.mock('../../../services/newsroom/briefService', () => ({ BriefService: service }))
// The chips for failed runs and unsent alerts read two other services; a quiet day has none.
vi.mock('../../../services/automation/dispatcherService', () => ({ DispatcherService: { getGlobalActivity: vi.fn().mockResolvedValue([]) } }))
vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: { alerts: vi.fn().mockResolvedValue({ cursor: 0, items: [] }) } }))

import NewsroomBriefCard from '../../../components/dashboard/cards/NewsroomBriefCard.vue'
import { DASHBOARD_CARD_SIZE } from '../../../constants/dashboard'
import { resetBriefUnread } from '../../../composables/newsroom/useBriefUnread'
import type { DashboardCardSize } from '../../../types/dashboard'
import type { BriefIssue, BriefStory } from '../../../types/newsroom'

vi.mock('../../../services/modelStatsService', () => ({ ModelStatsService: { get: vi.fn().mockResolvedValue({ days: [], today: [] }) } }))

const story = (n: number, over: Partial<BriefStory> = {}): BriefStory => ({ pool_id: n, headline: `Story ${n}`, outlets: ['BBC'], ...over })
const edition = (over = {}) => ({ id: 2, name: 'Morning', ready_by: '07:00', days: [], collect_every: '2h0m0s', collect_for: '12h0m0s', sections: [], enabled: true, ...over })
const latest = (over = {}) => ({ edition_id: 2, name: 'Morning', issue_id: 5, made_at: '2026-11-23T06:00:00Z', read: false, ...over })
const issue = (over: Partial<BriefIssue> = {}): BriefIssue => ({
  id: 5, edition_id: 2, name: 'Morning', made_at: '2026-11-23T06:00:00Z', model_calls: 2,
  top: [story(1, { headline: 'Lead story' }), story(2), story(3)],
  sections: [{ kind: 'news', topic_id: 7, name: 'Technology', stories: [story(1), story(2), story(4)] }, { kind: 'news', topic_id: 8, name: 'World', stories: [story(5)] }],
  ...over,
})
// The unread state is shared by every card on screen, so a card left mounted would keep answering the next test's data.
const mounted: { unmount(): void }[] = []
afterEach(() => { mounted.splice(0).forEach((w) => w.unmount()) })
const mountCard = async (size: DashboardCardSize = 'lg') => {
  const w = mount(NewsroomBriefCard, { global: { stubs: { RouterLink: RouterLinkStub }, provide: { [DASHBOARD_CARD_SIZE as symbol]: ref(size) } } })
  mounted.push(w)
  await flushPromises()
  return w
}

beforeEach(() => {
  vi.clearAllMocks()
  resetBriefUnread()
  service.editions.mockResolvedValue([edition()])
  service.latest.mockResolvedValue([latest()])
  service.issue.mockResolvedValue(issue())
})

describe('NewsroomBriefCard', () => {
  it('is big when a brief is waiting: the masthead with a New tag, the lead, the other top stories and a tab per section', async () => {
    const w = await mountCard()
    expect(w.get('[data-test="brief-card-name"]').text()).toContain('Morning')
    expect(w.get('[data-test="brief-card-name"]').text()).toContain('New')
    expect(w.get('[data-test="brief-card-lead"]').text()).toContain('Lead story')
    expect(w.get('[data-test="brief-card-top"]').text()).toContain('Story 2')
    expect(w.findAll('[data-test="brief-card-tab"]').map((c) => c.text())).toEqual(['Top stories', 'Technology3', 'World1'])
    expect(w.get('[data-test="brief-card"]').attributes('data-unread')).toBe('true')
  })

  it('only shows and links: reading it here does not mark it read', async () => {
    const w = await mountCard()
    expect(service.markRead).not.toHaveBeenCalled()
    expect(w.getComponent(RouterLinkStub).props('to')).toMatchObject({ params: { tab: 'briefs' } })
  })

  it('drops the New tag once the brief is read', async () => {
    service.latest.mockResolvedValue([latest({ read: true })])
    const w = await mountCard()
    expect(w.get('[data-test="brief-card-name"]').text()).not.toContain('New')
    expect(w.get('[data-test="brief-card"]').attributes('data-unread')).toBeUndefined()
  })

  it('switches the body in place with tabs: one is selected, nothing opens elsewhere, and there is no expand control', async () => {
    const w = await mountCard()
    const tabs = w.findAll('[data-test="brief-card-tab"]')
    expect(tabs.map((x) => x.attributes('aria-selected'))).toEqual(['true', 'false', 'false'])
    expect(w.get('[data-test="brief-card-panel"]').text()).toContain('Lead story')
    await tabs[1]!.trigger('click')
    expect(w.findAll('[data-test="brief-card-tab"]').map((x) => x.attributes('aria-selected'))).toEqual(['false', 'true', 'false'])
    expect(w.get('[data-test="brief-card-section"]').text()).toContain('Story 4')
    expect(w.get('[data-test="brief-card-panel"]').text()).not.toContain('Lead story')
    await tabs[2]!.trigger('click')
    expect(w.get('[data-test="brief-card-section"]').text()).toContain('Story 5')
    expect(w.find('[data-test="brief-card-toggle"]').exists()).toBe(false)
    expect(w.find('[data-test="brief-card-all"]').exists()).toBe(false)
  })

  it('moves between tabs with the arrow keys', async () => {
    const w = await mountCard()
    await w.get('[data-test="brief-card-tabs"]').trigger('keydown', { key: 'ArrowRight' })
    expect(w.findAll('[data-test="brief-card-tab"]')[1]!.attributes('aria-selected')).toBe('true')
    await w.get('[data-test="brief-card-tabs"]').trigger('keydown', { key: 'ArrowLeft' })
    expect(w.findAll('[data-test="brief-card-tab"]')[0]!.attributes('aria-selected')).toBe('true')
  })

  it('shows the home block and the watches beside the news, only when the edition carries them', async () => {
    const plain = await mountCard()
    expect(plain.find('[data-test="brief-card-beside"]').exists()).toBe(false)
    service.issue.mockResolvedValue(issue({
      home: [{ id: 'network_not_marked_mine', count: 2, words: 'Devices on your network that you have not marked as yours.', level: 'check' }],
      watches: [{ watch: 'Kernel', new: 1, headline: 'A release' }],
      day: { items: [{ clock: '14:00', title: 'Call the dentist' }] },
    }))
    resetBriefUnread()
    const w = await mountCard()
    expect(w.get('[data-test="brief-card-home"]').text()).toContain('1 to check')
    expect(w.get('[data-test="watches-block"]').text()).toContain('Kernel')
    expect(w.get('[data-test="day-block"]').text()).toContain('Call the dentist')
  })

  it('says "Needs attention" in the header only when a critical fact is in the edition, at every size, and says the fact on its own line above the tabs', async () => {
    const quiet = await mountCard()
    expect(quiet.get('[data-test="brief-card-name"]').text()).not.toContain('Needs attention')
    service.issue.mockResolvedValue(issue({ home: [
      { id: 'radio_open_or_weak_ap', count: 1, words: 'Confirmed access points of yours that are open or use an outdated protection.', level: 'critical' },
      { id: 'network_online', count: 16, words: 'Devices the network sees right now.', level: 'info' },
    ] }))
    resetBriefUnread()
    const w = await mountCard()
    expect(w.get('[data-test="brief-card-name"]').text()).toContain('Needs attention')
    expect(w.get('[data-test="brief-card-attention"]').text()).toContain('open or use an outdated protection')
    for (const size of ['sm', 'xs'] as const) {
      resetBriefUnread()
      expect((await mountCard(size)).text(), size).toContain('Needs attention')
    }
  })

  it('asks for an edition once for each new issue, not on every refresh', async () => {
    await mountCard()
    expect(service.issue).toHaveBeenCalledTimes(1)
    expect(service.issue).toHaveBeenCalledWith(5)
  })

  it('shrinks: the lead and a count when medium, one line when small', async () => {
    const sm = await mountCard('sm')
    expect(sm.get('[data-test="brief-card-small"]').text()).toContain('Lead story')
    expect(sm.get('[data-test="brief-card-small"]').text()).toContain('4 stories in 2 sections')
    expect(sm.find('[data-test="brief-card-top"]').exists()).toBe(false)
    const xs = await mountCard('xs')
    expect(xs.get('[data-test="brief-card-line"]').text()).toContain('Morning · 4 stories')
  })

  it('says how to set one up when there is no edition, and when it is due when none is written yet', async () => {
    service.editions.mockResolvedValue([])
    const none = await mountCard()
    expect(none.get('[data-test="brief-card-none"]').text()).toContain('Set up a brief')
    resetBriefUnread()
    service.editions.mockResolvedValue([edition()])
    service.latest.mockResolvedValue([])
    const waiting = await mountCard()
    expect(waiting.get('[data-test="brief-card-waiting"]').text()).toContain('ready at 07:00')
  })

  it('offers a chip for each brief and shows the one picked', async () => {
    service.editions.mockResolvedValue([edition(), edition({ id: 3, name: 'Evening' })])
    service.latest.mockResolvedValue([latest(), latest({ edition_id: 3, name: 'Evening', issue_id: 6, read: true })])
    service.issue.mockImplementation(async (id: number) => issue({ id, name: id === 6 ? 'Evening' : 'Morning' }))
    const w = await mountCard()
    expect(w.get('[data-test="brief-card-name"]').text()).toContain('Morning')
    await w.findAll('button').find((b) => b.text() === 'Evening')!.trigger('click')
    await flushPromises()
    expect(w.get('[data-test="brief-card-name"]').text()).toContain('Evening')
  })
})
