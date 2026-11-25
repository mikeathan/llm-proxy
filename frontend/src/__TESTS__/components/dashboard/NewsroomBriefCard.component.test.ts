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
  const w = mount(NewsroomBriefCard, { global: { stubs: { RouterLink: RouterLinkStub }, provide: { [DASHBOARD_CARD_SIZE as symbol]: ref(size) } } })
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

describe('NewsroomBriefCard', () => {
  it('is a front page when a brief is waiting: the name with a New tag, the lead, headlines by section, each section label a link', async () => {
    const w = await mountCard()
    expect(w.get('[data-test="brief-card-name"]').text()).toContain('Morning')
    expect(w.get('[data-test="brief-card-name"]').text()).toContain('New')
    expect(w.get('[data-test="brief-card-lead"]').text()).toContain('Top story · Technology')
    expect(w.get('[data-test="brief-card-lead"]').text()).toContain('Lead story')
    expect(w.get('[data-test="brief-card-lead"]').text()).toContain('What it says.')
    const columns = w.get('[data-test="brief-card-columns"]').text()
    expect(columns).toContain('Story 2')
    expect(columns).toContain('Story 5')
    expect(columns).not.toContain('Lead story')
    expect(links(w)).toContainEqual(toBrief({ edition: 2, section: 7 }))
    expect(w.find('[data-test="brief-index"]').exists(), 'every section has a column, so no index repeats them').toBe(false)
    expect(w.find('[data-test="brief-card-tab"]').exists(), 'no tabs: reading is on the Newsroom').toBe(false)
    expect(w.get('[data-test="brief-card"]').attributes('data-unread')).toBe('true')
  })

  it('only shows and links, each headline and section to its place in the brief, and never marks it read', async () => {
    const w = await mountCard()
    expect(service.markRead).not.toHaveBeenCalled()
    expect(links(w)).toContainEqual(toBrief({ edition: 2, story: 1 }))
    expect(links(w)).toContainEqual(toBrief({ edition: 2, section: 8 }))
    expect(links(w)).toContainEqual(toBrief({ edition: 2 }))
    expect(w.findAll('a[target="_blank"]')).toHaveLength(0)
  })

  it('fits its columns to the sections: two sections fill two columns of three headlines, with no empty third column', async () => {
    const sec = (topic_id: number, name: string, from: number) => ({ kind: 'news' as const, topic_id, name, stories: [0, 1, 2, 3, 4].map((i) => story(from + i)) })
    service.issue.mockResolvedValue(issue({ top: [], sections: [sec(7, 'Technology', 10), sec(8, 'News', 20)] }))
    const w = await mountCard()
    const cols = w.findAll('[data-test="brief-card-columns"] > li')
    expect(cols).toHaveLength(2)
    expect(cols.map((c) => c.findAll('[data-test="brief-headline"]').length)).toEqual([3, 3])
    expect(w.get('[data-test="brief-card-columns"]').classes()).toContain('md:grid-cols-2')
    expect(w.get('[data-test="brief-card-columns"]').classes()).not.toContain('md:grid-cols-3')
  })

  it('lists in its footer only the sections that have no column of their own', async () => {
    const sec = (topic_id: number, name: string, n: number) => ({ kind: 'news' as const, topic_id, name, stories: [story(n), story(n + 1)] })
    service.issue.mockResolvedValue(issue({ top: [], sections: [sec(7, 'A', 10), sec(8, 'B', 20), sec(9, 'C', 30), sec(11, 'Sport', 40)] }))
    const w = await mountCard()
    expect(w.findAll('[data-test="brief-card-columns"] > li')).toHaveLength(3)
    const index = w.get('[data-test="brief-index"]').text()
    expect(index).toContain('Sport')
    expect(index).not.toContain('C')
    expect(links(w)).toContainEqual(toBrief({ edition: 2, section: 11 }))
  })

  it('names the outlets of the top story, three at most, in place of a bare count', async () => {
    service.issue.mockResolvedValue(issue({ top: [story(1, { headline: 'Lead story', outlets: ['BBC', 'Reuters', 'FT', 'AP'] })] }))
    const lead = (await mountCard()).get('[data-test="brief-card-lead"]').text()
    expect(lead).toContain('BBC · Reuters · FT +1')
    expect(lead).not.toContain('outlets')
  })

  it('does not repeat the edition name under its own tab: the name is kept for screen readers and the tab says New', async () => {
    service.editions.mockResolvedValue([edition(), edition({ id: 3, name: 'Evening' })])
    const w = await mountCard()
    expect(w.get('[data-test="brief-card-name"] .sr-only').text()).toBe('Morning')
    expect(w.get('[data-test="brief-card-name"]').text()).not.toContain('New')
    expect(w.findAll('[role="radio"]').map((b) => b.text())).toContain('Morning · New')
  })

  it('opens the edition picked on the card, not another one', async () => {
    service.editions.mockResolvedValue([edition(), edition({ id: 3, name: 'Evening' })])
    service.latest.mockResolvedValue([latest({ read: true }), latest({ edition_id: 3, name: 'Evening', issue_id: 6, read: true })])
    service.issue.mockImplementation(async (id: number) => issue({ id, edition_id: id === 6 ? 3 : 2, name: id === 6 ? 'Evening' : 'Morning' }))
    const w = await mountCard()
    await w.findAll('[role="radio"]').find((b) => b.text() === 'Evening')!.trigger('click')
    await flushPromises()
    expect(w.get('[data-test="brief-card-name"]').text()).toContain('Evening')
    expect(links(w)).toContainEqual(toBrief({ edition: 3 }))
  })

  it('asks for a written edition once, however many cards show it and however often they refresh', async () => {
    await mountCard()
    await mountCard('sm')
    expect(service.issue).toHaveBeenCalledTimes(1)
    expect(service.editions).toHaveBeenCalledTimes(1)
  })

  it('says what needs a look first, in the header at every size', async () => {
    service.issue.mockResolvedValue(issue({ home: [{ id: 'radio_open_or_weak_ap', count: 1, words: 'An open access point.', level: 'critical' }] }))
    const w = await mountCard()
    expect(w.get('[data-test="brief-card-name"]').text()).toContain('Needs attention')
    expect(w.get('[data-test="brief-attention"]').text()).toContain('An open access point.')
    expect((await mountCard('xs')).get('[data-test="brief-card-line"]').text()).toContain('Needs attention')
  })

  it('shrinks: the lead when medium, one line when small', async () => {
    const sm = await mountCard('sm')
    expect(sm.get('[data-test="brief-card-lead"]').text()).toContain('Lead story')
    expect(sm.find('[data-test="brief-card-columns"]').exists()).toBe(false)
    const xs = await mountCard('xs')
    expect(xs.get('[data-test="brief-card-line"]').text()).toContain('Morning · 4 stories')
  })

  it('says how to set one up when there is no edition, and when it is due when none is written yet', async () => {
    service.editions.mockResolvedValue([])
    const none = await mountCard()
    expect(none.get('[data-test="brief-card-none"]').text()).toContain('No brief yet')
    expect(links(none)).toContainEqual(toNewsroom('following'))
    resetBriefCard()
    service.editions.mockResolvedValue([edition()])
    service.latest.mockResolvedValue([])
    resetBriefUnread()
    expect((await mountCard()).get('[data-test="brief-card-waiting"]').text()).toContain('07:00')
  })
})
