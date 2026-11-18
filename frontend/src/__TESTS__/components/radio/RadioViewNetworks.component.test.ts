import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const service = vi.hoisted(() => ({ aroundNetworks: vi.fn() }))
vi.mock('../../../services/radio/radioService', () => ({ RadioService: service }))

import RadioViewNetworks from '../../../components/radio/RadioViewNetworks.vue'

const mounted: Array<{ unmount: () => void }> = []
afterEach(() => { mounted.splice(0).forEach((w) => w.unmount()) })
beforeEach(() => {
  service.aroundNetworks.mockResolvedValue({
    total: 3,
    groups: [
      { id: 'aa000001', yours: true, names: ['home', 'home-guest'], strongest_dbm: -45, heard: true,
        radios: [{ id: 'r1', mine: false, class: 'infrastructure', heard: true, dbm: -45, ident: { kind: 'router', kind_confidence: 90, kind_source: 'sends_beacons', brand: 'ASUSTek COMPUTER INC.', model: 'RT-AX58U', confidence: 90, your_ap: true } },
          { id: 'r2', mine: false, class: 'infrastructure', heard: true, dbm: -47 }],
        clients: [{ id: 'p1', mine: true, label: 'my phone', class: 'personal', heard: true, dbm: -50 }] },
      { id: 'bb000002', yours: false, names: ['SKY1234'], strongest_dbm: -75, heard: true,
        radios: [{ id: 's1', mine: false, class: 'neighbour', heard: true, dbm: -75, ident: { kind: 'router', kind_confidence: 90, kind_source: 'sends_beacons', brand: 'SKY UK', confidence: 70 } }], clients: [] },
    ],
    unconnected: [{ id: 'x1', mine: false, class: 'transient', heard: true, dbm: -80 }],
  })
})

describe('RadioViewNetworks', () => {
  it('shows your router first with its radios and clients, then the others, each named by make and model', async () => {
    const w = mount(RadioViewNetworks, { global: { stubs: { RouterLink: { props: ['to'], template: '<a><slot /></a>' } } } })
    mounted.push(w)
    await flushPromises()
    const groups = w.findAll('[data-test="radio-network-group"]')
    expect(groups).toHaveLength(2)
    expect(groups[0]!.attributes('data-yours')).toBe('1')
    expect(groups[0]!.get('[data-test="radio-network-title"]').text()).toBe('ASUSTek RT-AX58U')
    expect(groups[0]!.get('[data-test="radio-network-names"]').text()).toContain('home · home-guest')
    expect(groups[0]!.findAll('[data-test="radio-network-clients"] [data-test="radio-member"]')).toHaveLength(1)
    expect(groups[1]!.get('[data-test="radio-network-title"]').text()).toBe('SKY UK router')
    expect(w.findAll('[data-test="radio-networks-unconnected"] [data-test="radio-member"]')).toHaveLength(1)
  })
})
