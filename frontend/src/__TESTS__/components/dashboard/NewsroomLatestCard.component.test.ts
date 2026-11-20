import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const service = vi.hoisted(() => ({ topics: vi.fn(), feed: vi.fn() }))
vi.mock('../../../services/newsroom/newsroomService', () => ({ NewsroomService: service }))

import NewsroomLatestCard from '../../../components/dashboard/cards/NewsroomLatestCard.vue'

const entry = (id: number, title: string, link: string) => ({ id, topic_id: 1, at: new Date().toISOString(), source: 's', title, link })
const day = (current: ReturnType<typeof entry>) => ({ day: 'd', label: 'Today', updates: 1, unread: 1, topics: [{ topic_id: 1, topic: 'Go', updates: 1, unread: 1, last_at: '', current }] })
const topic = (over = {}) => ({ id: 1, name: 'Go', interval: '1h0m0s', enabled: true, sources: [], ...over })
const mountCard = async () => { const w = mount(NewsroomLatestCard); await flushPromises(); return w }

beforeEach(() => { vi.clearAllMocks() })

describe('NewsroomLatestCard', () => {
  it('shows the newest headline as a link and has nothing to press', async () => {
    service.topics.mockResolvedValue([topic()])
    service.feed.mockResolvedValue([day(entry(1, 'Go 1.26 released', 'https://go.dev/x'))])
    const w = await mountCard()
    expect(w.get('[data-test="newsroom-headline"] a').attributes('href')).toBe('https://go.dev/x')
    expect(w.find('button').exists()).toBe(false)
    expect(w.find('[data-test="newsroom-failing"]').exists()).toBe(false)
  })

  it('shows a headline whose link is not http or https as plain text', async () => {
    service.topics.mockResolvedValue([topic()])
    service.feed.mockResolvedValue([day(entry(1, 'Sneaky', 'javascript:alert(1)'))])
    const w = await mountCard()
    expect(w.find('[data-test="newsroom-headline"] a').exists()).toBe(false)
    expect(w.text()).toContain('Sneaky')
  })

  it('flags a topic that cannot read a source, and says when nothing is followed', async () => {
    service.topics.mockResolvedValue([topic({ last_status: 'failing' })])
    service.feed.mockResolvedValue([])
    const w = await mountCard()
    expect(w.get('[data-test="newsroom-failing"]').text()).toContain('Go cannot read a source')
    service.topics.mockResolvedValue([])
    expect((await mountCard()).find('[data-test="newsroom-empty"]').text()).toContain('You follow nothing yet')
  })
})
