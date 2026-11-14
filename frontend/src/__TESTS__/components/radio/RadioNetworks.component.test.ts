import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { RadioAir, RadioNetworks as Networks } from '../../../types/radio'

const service = vi.hoisted(() => ({ air: vi.fn(), networks: vi.fn(), setNetworkNames: vi.fn(), confirmNetwork: vi.fn(), dropNetwork: vi.fn() }))
vi.mock('../../../services/radio/radioService', () => ({ RadioService: service }))

import RadioAround from '../../../components/radio/RadioAround.vue'
import RadioNetworks from '../../../components/radio/RadioNetworks.vue'

const air = (over: Partial<RadioAir> = {}): RadioAir => ({
  rules: { version: 1, provisional: true, window_days: 7, min_cycles_listened: 12 },
  listened_cycles: 30,
  counts: { infrastructure: 4, personal: 2, neighbour: 9, transient: 1, unknown: 3 },
  total: 19,
  changes: [{ day: '2026-11-14', from: '', to: 'infrastructure', version: 1, reason: 'always_here_steady_signal', n: 4 }],
  ...over,
})

beforeEach(() => vi.clearAllMocks())

describe('RadioAround', () => {
  it('counts other people’s devices by kind, says the rules are a draft, and shows why devices were sorted', async () => {
    service.air.mockResolvedValue(air())
    const w = mount(RadioAround)
    await flushPromises()
    expect(w.get('[data-test="radio-around-total"]').text()).toBe('19 devices around you')
    expect(w.findAll('[data-test="radio-class"]').map((c) => c.attributes('data-class'))).toEqual(['infrastructure', 'personal', 'neighbour', 'transient', 'unknown'])
    expect(w.get('[data-class="neighbour"]').text()).toContain('9')
    expect(w.get('[data-test="radio-around-provisional"]').text()).toContain('first draft')
    expect(w.get('[data-test="radio-change"]').text()).toContain('4 devices first sorted as always here, always here with a steady signal')
  })

  it('says how much listening is still needed before devices are sorted, and the empty state when nothing was heard', async () => {
    service.air.mockResolvedValue(air({ listened_cycles: 3 }))
    let w = mount(RadioAround)
    await flushPromises()
    expect(w.get('[data-test="radio-around-listened"]').text()).toBe('Listened through 3 of the 12 cycles needed before devices are sorted. Keep listening.')
    service.air.mockResolvedValue(air({ listened_cycles: 0, total: 0, changes: [] }))
    w = mount(RadioAround)
    await flushPromises()
    expect(w.get('[data-test="radio-around-empty"]').text()).toContain('Start listening')
  })
})

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
