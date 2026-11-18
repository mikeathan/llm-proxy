import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { RadioNetworks as Networks } from '../../../types/radio'

const service = vi.hoisted(() => ({ networks: vi.fn(), setNetworkNames: vi.fn(), confirmNetwork: vi.fn(), dropNetwork: vi.fn() }))
vi.mock('../../../services/radio/radioService', () => ({ RadioService: service }))

import RadioNetworks from '../../../components/radio/RadioNetworks.vue'

beforeEach(() => vi.clearAllMocks())

describe('RadioNetworks', () => {
  const nets = (over: Partial<Networks> = {}): Networks => ({
    names: ['home'],
    access_points: [{ id: 'aa', name: 'home', channel: 60, security: 'wpa2', confirmed: false, last_seen: '2026-11-14' }],
    ...over,
  })

  it('adds a name, and keeps an access point a candidate until the owner confirms it', async () => {
    service.networks.mockResolvedValue(nets())
    service.setNetworkNames.mockResolvedValue({ names: ['home', 'guest'] })
    service.confirmNetwork.mockResolvedValue({ confirmed: true })
    const w = mount(RadioNetworks)
    await flushPromises()
    expect(w.get('[data-test="radio-ap"]').attributes('data-confirmed')).toBe('false')
    expect(w.text()).toContain('1 access point broadcasts one of your network names')
    expect(w.get('[data-test="radio-network-line"]').text()).toBe('No access point confirmed yet')
    await w.get('[data-test="radio-network-add-open"]').trigger('click')
    await w.get('[data-test="radio-network-input"]').setValue('guest')
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(service.setNetworkNames).toHaveBeenCalledWith(['home', 'guest'])
    await w.get('[data-test="radio-ap-confirm"]').trigger('click')
    await flushPromises()
    expect(service.confirmNetwork).toHaveBeenCalledWith('aa')
  })

  it('reads a mesh as one network with two access points, and lists them in Advanced only', async () => {
    const { useNetauditViewMode } = await import('../../../composables/netaudit/useNetauditViewMode')
    useNetauditViewMode().set('simple')
    service.networks.mockResolvedValue(nets({ access_points: [
      { id: 'aa', name: 'home', channel: 60, security: 'wpa2', confirmed: true, last_seen: '2026-11-14' },
      { id: 'bb', name: 'home', channel: 60, security: 'wpa2', confirmed: true, last_seen: '2026-11-14' },
    ] }))
    const w = mount(RadioNetworks)
    await flushPromises()
    expect(w.findAll('[data-test="radio-network"]')).toHaveLength(1)
    expect(w.get('[data-test="radio-network-line"]').text()).toBe('5 GHz · channel 60 · 2 access points confirmed')
    expect(w.find('[data-test="radio-ap"]').exists()).toBe(false)
    useNetauditViewMode().set('advanced')
    await flushPromises()
    expect(w.findAll('[data-test="radio-ap"]')).toHaveLength(2)
    useNetauditViewMode().set('simple')
  })

  it('removes a name and says it plainly when nothing is named', async () => {
    service.networks.mockResolvedValue(nets({ names: [], access_points: [] }))
    const w = mount(RadioNetworks)
    await flushPromises()
    expect(w.get('[data-test="radio-networks-empty"]').text()).toBe('No network named yet.')
  })
})
