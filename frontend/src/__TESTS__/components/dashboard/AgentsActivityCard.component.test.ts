import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const service = vi.hoisted(() => ({ agentWatch: vi.fn(), lockdown: vi.fn(), engageLockdown: vi.fn(), releaseLockdown: vi.fn() }))
vi.mock('../../../services/security/securityService', () => ({ SecurityService: service }))

import AgentsActivityCard from '../../../components/dashboard/cards/AgentsActivityCard.vue'

const event = (i: number) => ({ at: new Date(Date.now() - i * 60_000).toISOString(), kind: 'tool_refused', workspace: 'ws', automation: 'nightly', tool: `tool${i}` })
const mountCard = async () => {
  const w = mount(AgentsActivityCard)
  await flushPromises()
  return w
}

beforeEach(() => {
  vi.clearAllMocks()
  service.lockdown.mockResolvedValue({ on: false })
})

describe('AgentsActivityCard', () => {
  it('says lock down is off and lists the latest events newest first, with no switch on it', async () => {
    service.agentWatch.mockResolvedValue({ events: [event(1), event(2)], automations: [] })
    const w = await mountCard()
    expect(w.get('[data-test="agents-lockdown"]').text()).toContain('Lock down is off')
    expect(w.findAll('[data-test="agents-event"]').map((e) => e.text())).toEqual([expect.stringContaining('tool1'), expect.stringContaining('tool2')])
    expect(w.find('button').exists()).toBe(false) // the switch is on the Agents page
  })

  it('shows lock down on as a tag, and says when nothing has happened', async () => {
    service.lockdown.mockResolvedValue({ on: true, since: new Date().toISOString() })
    service.agentWatch.mockResolvedValue({ events: [], automations: [] })
    const w = await mountCard()
    expect(w.get('[data-test="agents-lockdown"]').text()).toContain('Lock down')
    expect(w.find('[data-test="agents-empty"]').exists()).toBe(true)
  })

  it('lists where automations fetch from, closed until opened, once it is large', async () => {
    service.agentWatch.mockResolvedValue({ events: [event(1)], automations: [{ workspace: 'ws', automation: 'nightly', hosts: ['example.com'] }] })
    const w = await mountCard()
    expect(w.find('[data-test="agents-destinations"]').exists()).toBe(true)
  })

  it('says it could not read the record', async () => {
    service.agentWatch.mockRejectedValue(new Error('down'))
    const w = await mountCard()
    expect(w.text()).toContain('The agent activity could not be read')
  })
})
