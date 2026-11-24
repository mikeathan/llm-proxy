import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { RouterLinkStub, flushPromises, mount } from '@vue/test-utils'
import { ref } from 'vue'

const service = vi.hoisted(() => ({ editions: vi.fn(), latest: vi.fn(), issue: vi.fn(), markRead: vi.fn() }))
vi.mock('../../../services/newsroom/briefService', () => ({ BriefService: service }))

import NewsroomBriefCard from '../../../components/dashboard/cards/NewsroomBriefCard.vue'
import { DASHBOARD_CARD_SIZE } from '../../../constants/dashboard'
import { resetBriefUnread } from '../../../composables/newsroom/useBriefUnread'
import type { DashboardCardSize } from '../../../types/dashboard'
import type { BriefIssue, BriefStory } from '../../../types/newsroom'

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
  it('is big when a brief is waiting: the masthead with a New tag, the lead, the other top stories and a chip per section', async () => {
    const w = await mountCard()
    expect(w.get('[data-test="brief-card-name"]').text()).toContain('Morning')
    expect(w.get('[data-test="brief-card-name"]').text()).toContain('New')
    expect(w.get('[data-test="brief-card-lead"]').text()).toContain('Lead story')
    expect(w.get('[data-test="brief-card-top"]').text()).toContain('Story 2')
    expect(w.findAll('[data-test="brief-card-chip"]').map((c) => c.text())).toEqual(['Technology3', 'World1'])
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

  it('opens the whole edition inside the card and folds it again', async () => {
    const w = await mountCard()
    expect(w.find('[data-test="brief-card-all"]').exists()).toBe(false)
    await w.get('[data-test="brief-card-toggle"]').trigger('click')
    expect(w.get('[data-test="brief-card-all"]').text()).toContain('Story 4')
    await w.get('[data-test="brief-card-toggle"]').trigger('click')
    expect(w.find('[data-test="brief-card-all"]').exists()).toBe(false)
  })

  it('opens the whole edition when a section chip is pressed', async () => {
    const w = await mountCard()
    await w.findAll('[data-test="brief-card-chip"]')[1]!.trigger('click')
    expect(w.get('[data-test="brief-card-all"]').text()).toContain('Story 5')
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
