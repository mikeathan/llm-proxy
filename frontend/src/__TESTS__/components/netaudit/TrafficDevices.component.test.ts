import { beforeEach, describe, expect, it, vi } from 'vitest'
import { RouterLinkStub, flushPromises, mount } from '@vue/test-utils'
import type { HostTraffic } from '../../../types/netaudit'

const service = vi.hoisted(() => ({
  helper: vi.fn(), lastHostTraffic: vi.fn(), captureHost: vi.fn(), routeTraces: vi.fn(), destinationData: vi.fn(), destinationPlaces: vi.fn(), devices: vi.fn(), routerConn: vi.fn(), deviceLookups: vi.fn(),
}))
vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: service }))

import TrafficDevices from '../../../components/netaudit/TrafficDevices.vue'

const kept = {
  at: '2026-11-09T20:00:00Z', interface: 'wlp2s0', stopped_by: 'window', elapsed_ms: 60_000, frames: 5, unreadable: 0, kernel_seen: 5, kernel_dropped: 0, sample: 1,
  bytes_in: 2048, bytes_out: 1024, bytes_through: 0, internet_bytes: 3072, local_network_bytes: 0, lookups: 0, kinds: [{ kind: 'https', bytes: 3072 }], per_second: [3072],
  destinations: [{ address: '1.1.1.1', bytes: 3072, internet: true }], flows: [], other_flows: 0, other_bytes: 0,
} as HostTraffic
const mountIt = async () => {
  const w = mount(TrafficDevices, { global: { stubs: { RouterLink: RouterLinkStub } } })
  await flushPromises()
  return w
}

describe('TrafficDevices (the dashboard card)', () => {
  beforeEach(() => {
    for (const fn of Object.values(service)) fn.mockReset()
    service.lastHostTraffic.mockResolvedValue(null)
    service.devices.mockResolvedValue({ devices: [] })
    service.routerConn.mockResolvedValue({ address: '', user: '', key_saved: false })
    service.deviceLookups.mockResolvedValue({ enabled: true, devices: [] })
  })

  it('shows loading until the last look has been read, then invites a look when there are no devices', async () => {
    service.lastHostTraffic.mockReturnValue(new Promise(() => undefined))
    const pending = mount(TrafficDevices, { global: { stubs: { RouterLink: RouterLinkStub } } })
    expect(pending.find('[data-test="traffic-loading"]').exists()).toBe(true)
    service.lastHostTraffic.mockResolvedValue(null)
    const w = await mountIt()
    expect(w.get('[data-test="traffic-never"]').text()).toContain('No devices yet')
  })

  it('says when the last look could not be read', async () => {
    service.lastHostTraffic.mockRejectedValue(new Error('disk'))
    const w = await mountIt()
    expect(w.get('[data-test="traffic-load-error"]').text()).not.toBe('')
  })

  describe('by device', () => {
    const GOOGLE = { asn: 15169, name: 'GOOGLE - Google LLC', country: 'US', place_country: 'US', city: 'Mountain View', point: { lat: 37.4, lon: -122.1 } }
    const READ = {
      address: '192.168.50.1', user: 'admin', key_saved: true, fingerprint: 'SHA256:abc',
      last: {
        at: '2026-11-12T10:00:00Z', lines: 245, kept: 40,
        devices: [
          { address: '192.168.50.251', total: 39, places: [
            { address: '142.251.155.120', port: 443, proto: 'udp', count: 31, owner: GOOGLE },
            { address: '142.251.30.95', port: 443, proto: 'udp', count: 8, owner: GOOGLE },
          ] },
          { address: '192.168.50.10', total: 1, places: [{ address: '8.8.8.8', port: 53, proto: 'udp', count: 1 }] },
        ],
      },
    }

    it('shows the router\'s devices, and the map and company groups of the busiest one, with no look at this computer', async () => {
      service.routerConn.mockResolvedValue(READ)
      service.devices.mockResolvedValue({ devices: [{ mac: 'aa:aa', ip: '192.168.50.251', name: 'M4 MacBook Air', first_seen: 'x', last_seen: 'x', approved: true, randomized: false, new: false }] })
      const w = await mountIt()
      expect(w.find('[data-test="traffic-never"]').exists()).toBe(false)
      expect(w.findAll('[data-test="rail-device"]').map((d) => d.get('span span').text())).toEqual(['M4 MacBook Air', 'Device 192.168.50.10'])
      expect(w.get('[data-test="device-name"]').text()).toContain('M4 MacBook Air')
      expect(w.findAll('[data-test="map-dot"]')).toHaveLength(1)
      expect(w.findAll('[data-test="place-group"]')).toHaveLength(1)
      expect(w.get('[data-test="group-title"]').text()).toBe('Google LLC')
      expect(w.get('[data-test="group-figure"]').text()).toBe('39')
    })

    it('puts this computer first once it has looked, with its headline, and its internet places as company groups', async () => {
      service.lastHostTraffic.mockResolvedValue({ ...kept, destinations: [{ address: '104.16.1.1', bytes: 3072, internet: true, owner: { asn: 13335, name: 'CLOUDFLARENET - Cloudflare, Inc.', country: 'US', city: 'Toronto', place_country: 'CA', point: { lat: 43.7, lon: -79.4 } } }] })
      service.routerConn.mockResolvedValue(READ)
      const w = await mountIt()
      expect(w.findAll('[data-test="rail-device"]')[0]!.text()).toContain('This computer')
      expect(w.get('[data-test="traffic-headline"]').text()).toContain('sent 1 KB')
      expect(w.get('[data-test="group-title"]').text()).toBe('Cloudflare, Inc.')
      expect(w.findAll('[data-test="map-dot"]')).toHaveLength(1)
      await w.findAll('[data-test="rail-device"]')[1]!.trigger('click')
      expect(w.find('[data-test="traffic-headline"]').exists()).toBe(false)
      expect(w.get('[data-test="device-source"]').text()).toBe('The router\u2019s table')
    })
  })
})
