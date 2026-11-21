import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const service = vi.hoisted(() => ({ watches: vi.fn(), feed: vi.fn() }))
vi.mock('../../../services/newsroom/newsroomService', () => ({ NewsroomService: service }))

import NewsroomLatestCard from '../../../components/dashboard/cards/NewsroomLatestCard.vue'

const entry = (id: number, title: string, link: string) => ({ id, watch_id: 1, at: new Date().toISOString(), source: 's', title, link })
const day = (current: ReturnType<typeof entry>) => ({ day: 'd', label: 'Today', updates: 1, unread: 1, watches: [{ watch_id: 1, watch: 'Go', updates: 1, unread: 1, last_at: '', current }] })
const watch = (over = {}) => ({ id: 1, name: 'Go', interval: '1h0m0s', enabled: true, sources: [], ...over })
const mountCard = async () => { const w = mount(NewsroomLatestCard); await flushPromises(); return w }

beforeEach(() => { vi.clearAllMocks() })

describe('NewsroomLatestCard watch chips', () => {
  const two = () => service.watches.mockResolvedValue([watch(), watch({ id: 2, name: 'Security' })])

  it('shows the short headline in place of the long title', async () => {
    service.watches.mockResolvedValue([watch()])
    service.feed.mockResolvedValue([day({ ...entry(1, 'A very long original post title that goes on', 'https://go.dev/x'), headline: 'Short headline' } as ReturnType<typeof entry>)])
    const w = await mountCard()
    expect(w.get('[data-test="newsroom-headline"]').text()).toContain('Short headline')
    expect(w.text()).not.toContain('very long original')
  })

  it('offers a chip per watch and asks the feed for the chosen one', async () => {
    two()
    service.feed.mockResolvedValue([])
    const w = await mountCard()
    expect(service.feed.mock.calls[0]![0]).toMatchObject({ watch: null })
    await w.findAll('[data-test="watch-chips"] button')[2]!.trigger('click')
    await flushPromises()
    expect(service.feed.mock.calls[service.feed.mock.calls.length - 1]![0]).toMatchObject({ watch: 2 })
  })

  it('shows no chips for a single watch', async () => {
    service.watches.mockResolvedValue([watch()])
    service.feed.mockResolvedValue([])
    expect((await mountCard()).find('[data-test="watch-chips"]').exists()).toBe(false)
  })
})

describe('NewsroomLatestCard', () => {
  it('shows the newest headline as a link and has nothing to press', async () => {
    service.watches.mockResolvedValue([watch()])
    service.feed.mockResolvedValue([day(entry(1, 'Go 1.26 released', 'https://go.dev/x'))])
    const w = await mountCard()
    expect(w.get('[data-test="newsroom-headline"] a').attributes('href')).toBe('https://go.dev/x')
    expect(w.find('button').exists()).toBe(false)
    expect(w.find('[data-test="newsroom-failing"]').exists()).toBe(false)
  })

  it('shows a headline whose link is not http or https as plain text', async () => {
    service.watches.mockResolvedValue([watch()])
    service.feed.mockResolvedValue([day(entry(1, 'Sneaky', 'javascript:alert(1)'))])
    const w = await mountCard()
    expect(w.find('[data-test="newsroom-headline"] a').exists()).toBe(false)
    expect(w.text()).toContain('Sneaky')
  })

  it('flags a watch that cannot read a source, and says when nothing is followed', async () => {
    service.watches.mockResolvedValue([watch({ last_status: 'failing' })])
    service.feed.mockResolvedValue([])
    const w = await mountCard()
    expect(w.get('[data-test="newsroom-failing"]').text()).toContain('Go cannot read a source')
    service.watches.mockResolvedValue([])
    expect((await mountCard()).find('[data-test="newsroom-empty"]').text()).toContain('You follow nothing yet')
  })
})
