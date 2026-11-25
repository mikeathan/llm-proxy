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

const edition = { id: 2, name: 'Morning', ready_by: '07:00', days: [], collect_every: '2h0m0s', collect_for: '12h0m0s', sections: [], enabled: true }
const issue = (id: number, name: string, madeAt: string) => ({ id, edition_id: 2, name, made_at: madeAt, model_calls: 1, sections: [] })
const mounted: { unmount(): void }[] = []
afterEach(() => { mounted.splice(0).forEach((w) => w.unmount()) })

beforeEach(() => {
  vi.clearAllMocks()
  resetBriefUnread()
  service.editions.mockResolvedValue([edition])
  service.topics.mockResolvedValue([])
  service.settings.mockResolvedValue({ model_calls_per_day: 0, default: 200, max: 2000 })
  service.sources.mockResolvedValue([])
  service.markRead.mockResolvedValue(undefined)
  service.jobs.mockResolvedValue([])
  // The list carries the issues whole in these tests; the page asks for the one it shows by id.
  service.issue.mockImplementation(async (id: number) => ((await service.issues()) as { id: number }[]).find((i) => i.id === id))
  service.latest.mockResolvedValue([{ edition_id: 2, name: 'Morning', issue_id: 7, made_at: '2026-11-23T06:00:00Z', read: true }])
})

describe('NewsroomBriefs status', () => {
  it('says plainly that today\'s brief needs nothing, and that an older one is not today\'s', async () => {
    service.issues.mockResolvedValue([issue(7, 'Today', new Date().toISOString())])
    let w = mount(NewsroomBriefs, { global: { stubs: { RouterLink: RouterLinkStub } } })
    mounted.push(w)
    await flushPromises()
    // Today's brief says nothing beyond the dateline: no "written automatically, nothing to press" line.
    expect(w.find('[data-test="brief-status"]').exists()).toBe(false)
    expect(w.text()).toContain('Write a new one now')
    w.unmount()
    service.issues.mockResolvedValue([issue(7, 'Old', new Date(Date.now() - 30 * 3_600_000).toISOString())])
    w = mount(NewsroomBriefs, { global: { stubs: { RouterLink: RouterLinkStub } } })
    mounted.push(w)
    await flushPromises()
    expect(w.get('[data-test="brief-status"]').attributes('data-tone')).toBe('old')
    expect(w.get('[data-test="brief-status"]').text()).toContain("not today's")
  })
})

describe('NewsroomBriefs earlier briefs', () => {
  it('offers no picker while only one brief exists', async () => {
    service.issues.mockResolvedValue([issue(7, 'Today', '2026-11-23T06:00:00Z')])
    const w = mount(NewsroomBriefs, { global: { stubs: { RouterLink: RouterLinkStub } } })
    mounted.push(w)
    await flushPromises()
    expect(w.find('[data-test="earlier-briefs"]').exists()).toBe(false)
  })

  it('opens the newest, lets the owner read an earlier one, and marks nothing for it', async () => {
    service.issues.mockResolvedValue([issue(7, 'Today', '2026-11-23T06:00:00Z'), issue(6, 'Yesterday', '2026-11-22T06:00:00Z')])
    const w = mount(NewsroomBriefs, { global: { stubs: { RouterLink: RouterLinkStub } } })
    mounted.push(w)
    await flushPromises()
    expect(w.get('[data-test="masthead"]').text()).toBe('Today')
    const options = w.get('[data-test="earlier-briefs"] select').findAll('option')
    expect(options).toHaveLength(2)
    expect(options[0]!.text()).toContain('(latest)')
    await w.get('[data-test="earlier-briefs"] select').setValue('6')
    // An earlier brief is read from the server by id the first time it is picked.
    await flushPromises()
    expect(service.issue).toHaveBeenCalledWith(6)
    expect(w.get('[data-test="masthead"]').text()).toBe('Yesterday')
    expect(service.markRead).not.toHaveBeenCalledWith(6)
  })
})
