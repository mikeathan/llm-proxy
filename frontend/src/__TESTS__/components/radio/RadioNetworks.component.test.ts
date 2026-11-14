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
    expect(w.get('[data-test="radio-ap"]').text()).toContain('Candidate')
    await w.get('[data-test="radio-network-input"]').setValue('guest')
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(service.setNetworkNames).toHaveBeenCalledWith(['home', 'guest'])
    await w.get('[data-test="radio-ap-confirm"]').trigger('click')
    await flushPromises()
    expect(service.confirmNetwork).toHaveBeenCalledWith('aa')
  })

  it('removes a name and says it plainly when nothing is named', async () => {
    service.networks.mockResolvedValue(nets({ names: [], access_points: [] }))
    const w = mount(RadioNetworks)
    await flushPromises()
    expect(w.get('[data-test="radio-networks-empty"]').text()).toBe('No network named yet.')
  })
})
