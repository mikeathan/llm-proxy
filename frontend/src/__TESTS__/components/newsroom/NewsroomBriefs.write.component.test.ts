import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { RouterLinkStub, flushPromises, mount } from '@vue/test-utils'

const service = vi.hoisted(() => ({
  editions: vi.fn(), topics: vi.fn(), issues: vi.fn(), issue: vi.fn(), jobs: vi.fn(), makeNow: vi.fn(), saveTopic: vi.fn(), settings: vi.fn(), sources: vi.fn(), latest: vi.fn(), markRead: vi.fn(),
}))
vi.mock('../../../services/newsroom/briefService', () => ({ BriefService: service }))
vi.mock('vue-router', async (orig) => ({ ...(await orig<typeof import('vue-router')>()), useRoute: () => ({ query: {} }) }))
vi.mock('../../../services/modelStatsService', () => ({ ModelStatsService: { get: vi.fn().mockResolvedValue({ days: [], today: [] }) } }))
vi.mock('../../../services/automation/dispatcherService', () => ({ DispatcherService: { getGlobalActivity: vi.fn().mockResolvedValue([]) } }))
vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: { alerts: vi.fn().mockResolvedValue({ items: [] }) } }))

import NewsroomBriefs from '../../../components/newsroom/NewsroomBriefs.vue'
import { resetBriefUnread } from '../../../composables/newsroom/useBriefUnread'
import { resetModelUse } from '../../../composables/newsroom/useModelUse'
import { resetBriefChips } from '../../../composables/newsroom/useBriefChips'

const edition = { id: 2, name: 'Morning', ready_by: '07:00', days: [], collect_every: '2h0m0s', collect_for: '12h0m0s', sections: [], enabled: true }
const issue = (id: number) => ({ id, edition_id: 2, name: `Issue ${id}`, made_at: new Date().toISOString(), model_calls: 1, sections: [] })
const job = (over = {}) => ({ edition_id: 2, state: 'writing', step: { stage: 'writing', done: 0, total: 3, section: 'World' }, started_at: new Date().toISOString(), ...over })

afterEach(() => {
  vi.useRealTimers()
  resetModelUse()
  resetBriefChips()
})
beforeEach(() => {
  vi.clearAllMocks()
  resetBriefUnread()
  service.editions.mockResolvedValue([edition])
  service.topics.mockResolvedValue([])
  service.settings.mockResolvedValue({ model_calls_per_day: 0, default: 200, max: 2000 })
  service.sources.mockResolvedValue([])
  service.latest.mockResolvedValue([])
  service.markRead.mockResolvedValue(undefined)
  service.issues.mockResolvedValue([{ id: 5, edition_id: 2, made_at: new Date().toISOString(), read: true }])
  service.issue.mockImplementation(async (id: number) => issue(id))
  service.jobs.mockResolvedValue([])
})

describe('NewsroomBriefs writing by hand', () => {
  it('says how far the write has got, and opens the new brief when it ends', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    const w = mount(NewsroomBriefs, { global: { stubs: { RouterLink: RouterLinkStub } } })
    await flushPromises()
    expect(w.get('[data-test="masthead"]').text()).toContain('Issue 5')
    service.makeNow.mockResolvedValue(job())
    service.jobs.mockResolvedValue([job()])
    // Writing is a rare action, so it sits in the brief's menu; its progress shows in the masthead row while it runs.
    await w.get('[data-test="brief-more"]').trigger('click')
    await w.get('[data-test="write-now"]').trigger('click')
    await flushPromises()
    await vi.advanceTimersByTimeAsync(2100)
    expect(w.get('[data-test="write-status"]').text()).toBe('Writing World (1 of 3)…')
    expect(w.get('[data-test="write-status"]').attributes('role')).toBe('status')
    await w.get('[data-test="brief-more"]').trigger('click')
    expect(w.get('[data-test="write-now"]').attributes('disabled')).toBeDefined()
    await w.get('[data-test="brief-more"]').trigger('click')
    service.jobs.mockResolvedValue([job({ state: 'done', issue_id: 6 })])
    service.issues.mockResolvedValue([{ id: 6, edition_id: 2, made_at: new Date().toISOString(), read: false }, { id: 5, edition_id: 2, made_at: new Date().toISOString(), read: true }])
    await vi.advanceTimersByTimeAsync(2100)
    await flushPromises()
    expect(w.get('[data-test="masthead"]').text()).toContain('Issue 6')
    w.unmount()
  })
})

describe('NewsroomBriefs story feedback', () => {
  it('turns "less like this" into skip words on the story\'s topic, keeping the rest of the topic', async () => {
    const topic = { id: 7, name: 'Economy', category: 'Business', queries: ['economy'], ignore: ['horoscope'] }
    service.topics.mockResolvedValue([topic])
    service.saveTopic.mockResolvedValue(topic)
    service.issue.mockResolvedValue({ ...issue(5), sections: [{ kind: 'news', topic_id: 7, name: 'Business', stories: [{ pool_id: 1, headline: 'Crypto prices slide again', outlets: ['A'] }] }] })
    const w = mount(NewsroomBriefs, { global: { stubs: { RouterLink: RouterLinkStub } } })
    await flushPromises()
    await w.get('[data-test="story-less"]').trigger('click')
    await w.findAll('[data-test="tune-word"]')[1]!.trigger('click')
    await w.get('[data-test="tune-apply"]').trigger('click')
    await flushPromises()
    expect(service.saveTopic).toHaveBeenCalledWith(7, { name: 'Economy', category: 'Business', queries: ['economy'], region: '', sources: [], ignore: ['horoscope', 'crypto'] })
    w.unmount()
  })
})
