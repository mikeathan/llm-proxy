import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const service = vi.hoisted(() => ({ agentWatch: vi.fn(), lockdown: vi.fn(), engageLockdown: vi.fn(), releaseLockdown: vi.fn() }))
vi.mock('../../../services/security/securityService', () => ({ SecurityService: service }))
const confirm = vi.fn()
vi.mock('../../../composables/ui/useConfirm', () => ({ useConfirm: () => ({ confirm }) }))

import AgentLockdown from '../../../components/agents/AgentLockdown.vue'

async function show(on = false) {
  service.agentWatch.mockResolvedValue({ events: [], automations: [] })
  service.lockdown.mockResolvedValue(on ? { on: true, since: '2026-11-05T09:00:00Z', signed_out: 2 } : { on: false })
  const w = mount(AgentLockdown)
  await flushPromises()
  return w
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('AgentLockdown', () => {
  it('keeps only the switch: the record of what agents did is a dashboard card', async () => {
    const w = await show()
    expect(w.find('[data-test="agent-event"]').exists()).toBe(false)
    expect(w.find('[data-test="agent-activity"]').exists()).toBe(false)
    expect(w.find('[data-test="lockdown-engage"]').exists()).toBe(true)
  })

  it('locks down only after confirmation, with the token choice, and offers to lift it', async () => {
    const w = await show()
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
    const w = await show(true)
    confirm.mockResolvedValueOnce(true)
    service.releaseLockdown.mockResolvedValue({ on: false })
    await w.get('[data-test="lockdown-release"]').trigger('click')
    await flushPromises()
    expect(service.releaseLockdown).toHaveBeenCalled()
    expect(w.find('[data-test="lockdown-engage"]').exists()).toBe(true)
  })
})
