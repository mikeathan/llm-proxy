import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import type { RadioNetworks as Networks } from '../../../types/radio'

const service = vi.hoisted(() => ({ networks: vi.fn(), setGuest: vi.fn() }))
vi.mock('../../../services/radio/radioService', () => ({ RadioService: service }))

import RadioNetworks from '../../../components/radio/RadioNetworks.vue'

const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/', component: { template: '<div/>' } }, { path: '/network/:tab?', name: 'network', component: { template: '<div/>' } }] })
const mountIt = async () => {
  const w = mount(RadioNetworks, { global: { plugins: [router] } })
  await flushPromises()
  return w
}

beforeEach(() => vi.clearAllMocks())

describe('RadioNetworks', () => {
  const nets = (over: Partial<Networks> = {}): Networks => ({
    names: ['home'],
    access_points: [{ id: 'aa', name: 'home', channel: 60, security: 'wpa2', confirmed: false, last_seen: '2026-11-14' }],
    ...over,
  })

  it('counts an unconfirmed access point and sends the owner to the one list to confirm it', async () => {
    service.networks.mockResolvedValue(nets())
    const w = await mountIt()
    expect(w.get('[data-test="radio-ap"]').attributes('data-confirmed')).toBe('false')
    expect(w.text()).toContain('1 access point broadcasts one of your network names and is not confirmed')
    expect(w.get('[data-test="radio-network-line"]').text()).toBe('No access point confirmed yet')
    expect(w.get('[data-test="radio-networks-open"]').attributes('href')).toBe('/network/settings?group=watching')
    expect(w.find('[data-test="radio-ap-confirm"]').exists()).toBe(false)
    expect(w.find('[data-test="radio-network-add-open"]').exists()).toBe(false)
  })

  it('says when the owner confirmed an access point here before the lists were one', async () => {
    service.networks.mockResolvedValue(nets({ access_points: [{ id: 'aa', name: 'home', channel: 60, security: 'wpa2', confirmed: false, confirmed_before: true, last_seen: '2026-11-14' }] }))
    const w = await mountIt()
    expect(w.get('[data-test="radio-ap-before"]').text()).toContain('confirm it once on the Network page')
  })

  it('reads a mesh as one network with two access points, and lists them in Advanced only', async () => {
    const { useNetauditViewMode } = await import('../../../composables/netaudit/useNetauditViewMode')
    useNetauditViewMode().set('simple')
    service.networks.mockResolvedValue(nets({ access_points: [
      { id: 'aa', name: 'home', channel: 60, security: 'wpa2', confirmed: true, last_seen: '2026-11-14' },
      { id: 'bb', name: 'home', channel: 60, security: 'wpa2', confirmed: true, last_seen: '2026-11-14' },
    ] }))
    const w = await mountIt()
    expect(w.findAll('[data-test="radio-network"]')).toHaveLength(1)
    expect(w.get('[data-test="radio-network-line"]').text()).toBe('5 GHz · channel 60 · 2 access points confirmed')
    expect(w.find('[data-test="radio-ap"]').exists()).toBe(false)
    useNetauditViewMode().set('advanced')
    await flushPromises()
    expect(w.findAll('[data-test="radio-ap"]')).toHaveLength(2)
    useNetauditViewMode().set('simple')
  })

  it('says it plainly when nothing is named', async () => {
    service.networks.mockResolvedValue(nets({ names: [], access_points: [] }))
    const w = await mountIt()
    expect(w.get('[data-test="radio-networks-empty"]').text()).toContain('name on the Network page')
  })
})
