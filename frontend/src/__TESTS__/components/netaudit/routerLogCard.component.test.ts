import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { RouterLogState } from '../../../types/netaudit'

const service = vi.hoisted(() => ({ routerLog: vi.fn() }))
vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: service }))

import RouterLogCard from '../../../components/netaudit/RouterLogCard.vue'

const state = (over: Partial<RouterLogState> = {}): RouterLogState => ({ enabled: false, listening: false, port: 5514, ...over })

async function mountCard() {
  const w = mount(RouterLogCard)
  await flushPromises()
  return w
}

describe('RouterLogCard', () => {
  beforeEach(() => {
    service.routerLog.mockReset().mockResolvedValue(state())
  })

  it('says it is switched off, has no switch of its own and says nothing about the router', async () => {
    const w = await mountCard()
    expect(w.find('[role="switch"]').exists()).toBe(false)
    expect(w.get('[data-test="module-switch-state"]').text()).toContain('Switched off.')
    expect(w.find('[data-test="router-log-heard"]').exists()).toBe(false)
    expect(w.find('[data-test="router-log-silent"]').exists()).toBe(false)
  })

  it('says it has heard nothing yet while it is on', async () => {
    service.routerLog.mockResolvedValue(state({ enabled: true, listening: true }))
    const w = await mountCard()
    expect(w.get('[data-test="module-switch-state"]').text()).toContain('Switched on.')
    expect(w.get('[data-test="router-log-silent"]').text()).toContain('Nothing has been heard from your router yet')
  })

  it('says when the router was last heard', async () => {
    service.routerLog.mockResolvedValue(state({ enabled: true, listening: true, last_heard: new Date().toISOString() }))
    const w = await mountCard()
    expect(w.get('[data-test="router-log-heard"]').text()).toContain('Listening on port 5514')
  })

  it('states why it cannot listen', async () => {
    service.routerLog.mockResolvedValue(state({ enabled: true, problem: 'port 5514 is in use or not allowed' }))
    const w = await mountCard()
    expect(w.get('[data-test="router-log-problem"]').text()).toContain('in use or not allowed')
  })

  it('shows the redirect command and how to undo it, without running anything', async () => {
    const w = await mountCard()
    expect(w.get('[data-test="router-log-command"]').text()).toContain('redirect to :5514')
    expect(w.text()).toContain('sudo nft delete table ip syslogredirect')
  })
})
