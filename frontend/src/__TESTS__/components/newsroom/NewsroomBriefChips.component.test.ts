import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { RouterLinkStub, flushPromises, mount } from '@vue/test-utils'

const dispatcher = vi.hoisted(() => ({ getGlobalActivity: vi.fn() }))
const netaudit = vi.hoisted(() => ({ alerts: vi.fn() }))
vi.mock('../../../services/automation/dispatcherService', () => ({ DispatcherService: dispatcher }))
vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: netaudit }))

import NewsroomBriefChips from '../../../components/newsroom/NewsroomBriefChips.vue'

const now = Date.now()
const run = (over = {}) => ({ id: 'r', automation_name: 'a', timestamp: new Date(now - 3600_000).toISOString(), ...over })
const alert = (over = {}) => ({ id: 1, kind: 'decoy', message: 'm', delivery: 'sent', count: 1, at: now - 3600_000, ...over })
const mounted: { unmount(): void }[] = []
afterEach(() => { mounted.splice(0).forEach((w) => w.unmount()) })
const mountIt = async () => {
  const w = mount(NewsroomBriefChips, { global: { stubs: { RouterLink: RouterLinkStub } } })
  mounted.push(w)
  await flushPromises()
  return w
}
beforeEach(() => {
  vi.clearAllMocks()
  dispatcher.getGlobalActivity.mockResolvedValue([])
  netaudit.alerts.mockResolvedValue({ cursor: 0, items: [] })
})

describe('NewsroomBriefChips', () => {
  it('shows nothing on a quiet day', async () => {
    expect((await mountIt()).find('[data-test="brief-chips"]').exists()).toBe(false)
  })

  it('counts the runs that failed in the last day and links to Automations', async () => {
    dispatcher.getGlobalActivity.mockResolvedValue([run({ error: 'boom' }), run({ id: 'r2', error: 'x' }), run({ id: 'r3' }), run({ id: 'old', error: 'old', timestamp: new Date(now - 3 * 86400_000).toISOString() })])
    const w = await mountIt()
    expect(w.get('[data-chip="failed_runs"]').text()).toContain('2 automation runs failed in the last day')
    expect(w.getComponent(RouterLinkStub).props('to')).toMatchObject({ name: 'automations' })
  })

  it('counts the alerts no connector took in the last day and links to the connector settings', async () => {
    netaudit.alerts.mockResolvedValue({ cursor: 3, items: [alert({ delivery: 'no_connector' }), alert({ id: 2, delivery: 'sent' }), alert({ id: 3, delivery: 'in_app' }), alert({ id: 4, delivery: 'failed', at: now - 3 * 86400_000 })] })
    const w = await mountIt()
    expect(w.get('[data-chip="blocked"]').text()).toContain('1 alert could not be sent')
    expect(w.getComponent(RouterLinkStub).props('to')).toMatchObject({ params: { section: 'communication' } })
  })

  it('shows no chip, and no error, for a source that cannot be read', async () => {
    dispatcher.getGlobalActivity.mockRejectedValue(new Error('down'))
    netaudit.alerts.mockRejectedValue(new Error('down'))
    const w = await mountIt()
    expect(w.find('[data-test="brief-chips"]').exists()).toBe(false)
    expect(w.text()).toBe('')
  })
})
