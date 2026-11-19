import { beforeEach, describe, expect, it, vi } from 'vitest'
import { RouterLinkStub, flushPromises, mount } from '@vue/test-utils'

const service = vi.hoisted(() => ({ devices: vi.fn(), presence: vi.fn(), ipv6Discovery: vi.fn() }))
vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: service }))

import NetworkDevicesCard from '../../../components/dashboard/cards/NetworkDevicesCard.vue'

const now = Date.now()
const ago = (ms: number) => new Date(now - ms).toISOString()
const device = (mac: string, over: Record<string, unknown> = {}) => ({ mac, ip: `192.168.1.${mac.length}`, first_seen: ago(9e8), last_seen: ago(60_000), approved: true, randomized: false, new: false, ...over })
const mountCard = async () => {
  const w = mount(NetworkDevicesCard, { global: { stubs: { RouterLink: RouterLinkStub } } })
  await flushPromises()
  return w
}

beforeEach(() => {
  vi.clearAllMocks()
  service.presence.mockResolvedValue({ enabled: true })
  service.ipv6Discovery.mockResolvedValue({ enabled: false })
})

describe('NetworkDevicesCard', () => {
  it('lists the devices that need a look first, then online, then offline, in the words of the Network page', async () => {
    service.devices.mockResolvedValue({
      devices: [device('aa', { name: 'Old TV', last_seen: ago(5 * 86_400_000) }), device('bbb', { name: 'Phone' }), device('cccc', { new: true, name: 'Stranger' })],
      gateway: { changed: false }, baselined: true, updated_at: ago(0),
    })
    const w = await mountCard()
    const rows = w.findAll('li')
    expect(rows.map((r) => r.text())).toEqual([expect.stringContaining('Stranger'), expect.stringContaining('Phone'), expect.stringContaining('Old TV')])
    expect(rows[0]!.text()).toContain('Needs a look')
    expect(rows[2]!.text()).toContain('Offline')
    expect(w.get('[data-test="devices-card-summary"]').text()).not.toBe('')
  })

  it('says what Radio heard, and links it to the Radio page of the device only for a page id the core made', async () => {
    service.devices.mockResolvedValue({
      devices: [device('aa', { name: 'Phone', air: { heard: ago(120_000), ring: 'house', page: '4ded1379' } }), device('bbb', { name: 'Tablet', air: { heard: ago(120_000), ring: 'house', page: '<script>' } })],
      gateway: { changed: false }, baselined: true, air: { state: 'listening' }, updated_at: ago(0),
    })
    const w = await mountCard()
    const links = w.findAllComponents(RouterLinkStub).filter((l) => (l.props('to') as { name?: string }).name === 'radio-device')
    expect(links).toHaveLength(1)
    expect(links[0]!.props('to')).toEqual({ name: 'radio-device', params: { id: '4ded1379' } })
    expect(w.findAll('[data-test="devices-card-air"]')).toHaveLength(2)
  })

  it('says it could not read the devices, with what to do', async () => {
    service.devices.mockRejectedValue(new Error('down'))
    const w = await mountCard()
    expect(w.text()).toContain('The devices could not be read')
  })
})
