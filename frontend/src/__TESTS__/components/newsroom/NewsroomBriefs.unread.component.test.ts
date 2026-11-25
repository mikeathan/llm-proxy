import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { RouterLinkStub, flushPromises, mount } from '@vue/test-utils'

const service = vi.hoisted(() => ({
  editions: vi.fn(), topics: vi.fn(), issues: vi.fn(), issue: vi.fn(), jobs: vi.fn(), settings: vi.fn(), sources: vi.fn(), latest: vi.fn(), markRead: vi.fn(),
}))
vi.mock('../../../services/newsroom/briefService', () => ({ BriefService: service }))
vi.mock('vue-router', async (orig) => ({ ...(await orig<typeof import('vue-router')>()), useRoute: () => ({ query: {} }) }))
vi.mock('../../../services/modelStatsService', () => ({ ModelStatsService: { get: vi.fn().mockResolvedValue({ days: [], today: [] }) } }))
vi.mock('../../../services/automation/dispatcherService', () => ({ DispatcherService: { getGlobalActivity: vi.fn().mockResolvedValue([]) } }))
vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: { alerts: vi.fn().mockResolvedValue({ items: [] }) } }))

import NewsroomBriefs from '../../../components/newsroom/NewsroomBriefs.vue'
import { resetBriefUnread } from '../../../composables/newsroom/useBriefUnread'

const edition = (id: number, name: string) => ({ id, name, ready_by: '07:00', days: [], collect_every: '2h0m0s', collect_for: '12h0m0s', sections: [], enabled: true })
const issue = (id: number, editionId: number, name: string) => ({ id, edition_id: editionId, name, made_at: '2026-11-23T06:00:00Z', model_calls: 1, sections: [] })
const latest = (editionId: number, issueId: number, name: string, read: boolean) => ({ edition_id: editionId, name, issue_id: issueId, made_at: '2026-11-23T06:00:00Z', read })
const mounted: { unmount(): void }[] = []
afterEach(() => { mounted.splice(0).forEach((w) => w.unmount()) })

beforeEach(() => {
  vi.clearAllMocks()
  resetBriefUnread()
  service.topics.mockResolvedValue([])
  service.settings.mockResolvedValue({ model_calls_per_day: 0, default: 200, max: 2000 })
  service.sources.mockResolvedValue([])
  service.markRead.mockResolvedValue(undefined)
  service.jobs.mockResolvedValue([])
  // The list carries the issues whole in these tests; the page asks for the one it shows by id.
  service.issue.mockImplementation(async (id: number) => ((await service.issues()) as { id: number }[]).find((i) => i.id === id))
})

describe('NewsroomBriefs unread', () => {
  it('opens on the brief that is unread and marks only that one read', async () => {
    service.editions.mockResolvedValue([edition(2, 'Morning'), edition(3, 'Evening')])
    service.issues.mockResolvedValue([issue(5, 2, 'Morning'), issue(6, 3, 'Evening')])
    service.latest.mockResolvedValue([latest(2, 5, 'Morning', true), latest(3, 6, 'Evening', false)])
    const w = mount(NewsroomBriefs, { global: { stubs: { RouterLink: RouterLinkStub } } })
    mounted.push(w)
    await flushPromises()
    expect(w.get('[data-test="masthead"]').text()).toBe('Evening')
    expect(service.markRead).toHaveBeenCalledTimes(1)
    expect(service.markRead).toHaveBeenCalledWith(6)
  })

  it('marks nothing while the unread list is not known yet', async () => {
    service.editions.mockResolvedValue([edition(2, 'Morning')])
    service.issues.mockResolvedValue([issue(5, 2, 'Morning')])
    service.latest.mockReturnValue(new Promise(() => {}))
    const w = mount(NewsroomBriefs, { global: { stubs: { RouterLink: RouterLinkStub } } })
    mounted.push(w)
    await flushPromises()
    expect(service.markRead).not.toHaveBeenCalled()
  })
})
