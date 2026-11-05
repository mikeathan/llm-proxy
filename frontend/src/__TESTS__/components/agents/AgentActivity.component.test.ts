import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { AgentWatchEvent, AgentWatchView } from '../../../types/security'

const service = vi.hoisted(() => ({ agentWatch: vi.fn(), lockdown: vi.fn(), engageLockdown: vi.fn(), releaseLockdown: vi.fn() }))
vi.mock('../../../services/security/securityService', () => ({ SecurityService: service }))
const confirm = vi.fn()
vi.mock('../../../composables/ui/useConfirm', () => ({ useConfirm: () => ({ confirm }) }))

import AgentActivity from '../../../components/agents/AgentActivity.vue'

const event = (i: number): AgentWatchEvent => ({ at: `2026-11-05T09:${String(i).padStart(2, '0')}:00Z`, kind: 'tool_refused', workspace: 'ws', automation: 'nightly', tool: 'read_file' })

async function show(record: AgentWatchView, on = false) {
  service.agentWatch.mockResolvedValue(record)
  service.lockdown.mockResolvedValue(on ? { on: true, since: '2026-11-05T09:00:00Z', signed_out: 2 } : { on: false })
  const w = mount(AgentActivity)
  await flushPromises()
  return w
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('AgentActivity', () => {
  it('says plainly when nothing is recorded yet', async () => {
    const w = await show({ events: [], automations: [] })
    expect(w.text()).toContain('Nothing recorded yet')
    expect(w.findAll('[data-test="agent-event"]')).toHaveLength(0)
  })

  it('shows the latest ten as one line each, and the rest one tap away', async () => {
    const w = await show({ events: Array.from({ length: 14 }, (_, i) => event(i)), automations: [] })
    const rows = w.findAll('[data-test="agent-event"]')
    expect(rows).toHaveLength(10)
    expect(rows[0]!.text()).toContain('The safety rules refused read_file')
    expect(rows[0]!.text()).toContain('nightly · ws')
    await w.get('[data-test="agent-show-all"]').trigger('click')
    expect(w.findAll('[data-test="agent-event"]')).toHaveLength(14)
  })

  it('locks down only after confirmation, with the token choice, and offers to lift it', async () => {
    const w = await show({ events: [], automations: [] })
    confirm.mockResolvedValueOnce(false)
    await w.get('[data-test="lockdown-engage"]').trigger('click')
    await flushPromises()
    expect(service.engageLockdown).not.toHaveBeenCalled()

    confirm.mockResolvedValueOnce(true)
    service.engageLockdown.mockResolvedValue({ on: true, since: '2026-11-05T09:00:00Z', signed_out: 2 })
    await w.get('[data-test="lockdown-engage"]').trigger('click')
    await flushPromises()
    expect(service.engageLockdown).toHaveBeenCalledWith(false)
    expect(w.get('[data-test="lockdown"]').text()).toContain('2 other sessions were signed out')
    expect(w.find('[data-test="lockdown-release"]').exists()).toBe(true)
  })

  it('shows what lock down ended and lifts it after confirmation', async () => {
    const w = await show({ events: [], automations: [] }, true)
    confirm.mockResolvedValueOnce(true)
    service.releaseLockdown.mockResolvedValue({ on: false })
    await w.get('[data-test="lockdown-release"]').trigger('click')
    await flushPromises()
    expect(service.releaseLockdown).toHaveBeenCalled()
    expect(w.find('[data-test="lockdown-engage"]').exists()).toBe(true)
  })
})
