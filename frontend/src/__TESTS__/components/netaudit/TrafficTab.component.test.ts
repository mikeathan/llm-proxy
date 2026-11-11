import { beforeEach, describe, expect, it, vi } from 'vitest'
import { RouterLinkStub, flushPromises, mount } from '@vue/test-utils'
import type { HelperStatus, HostTraffic } from '../../../types/netaudit'

const service = vi.hoisted(() => ({
  helper: vi.fn(), lastHostTraffic: vi.fn(), captureHost: vi.fn(), routeTraces: vi.fn(), destinationData: vi.fn(), destinationPlaces: vi.fn(), devices: vi.fn(), routerConn: vi.fn(), deviceLookups: vi.fn(),
}))
vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: service }))

import TrafficTab from '../../../components/netaudit/TrafficTab.vue'

const ready = (over: Partial<HelperStatus> = {}): HelperStatus => ({
  supported: true, installed: true, ready: true, declared_ranges: [], install_command: 'sudo x', help_path: 'docs',
  actions: [{ id: 'capture_host', on: true, installed: true }], ...over,
})
const kept = {
  at: '2026-11-09T20:00:00Z', interface: 'wlp2s0', stopped_by: 'window', elapsed_ms: 60_000, frames: 5, unreadable: 0, kernel_seen: 5, kernel_dropped: 0, sample: 1,
  bytes_in: 2048, bytes_out: 1024, bytes_through: 0, internet_bytes: 3072, local_network_bytes: 0, lookups: 0, kinds: [{ kind: 'https', bytes: 3072 }], per_second: [3072],
  destinations: [{ address: '1.1.1.1', bytes: 3072, internet: true }], flows: [], other_flows: 0, other_bytes: 0,
} as HostTraffic
const mountTab = async () => {
  const w = mount(TrafficTab, { global: { stubs: { RouterLink: RouterLinkStub } } })
  await flushPromises()
  return w
}

describe('TrafficTab', () => {
  beforeEach(() => {
    for (const fn of Object.values(service)) fn.mockReset()
    service.helper.mockResolvedValue(ready())
    service.lastHostTraffic.mockResolvedValue(null)
    service.routeTraces.mockResolvedValue({ supported: true, traces: [] })
    service.destinationData.mockResolvedValue({ ranges: 0, source: 'iptoasn.com' })
    service.destinationPlaces.mockResolvedValue({ ranges: 0, source: 'db-ip.com' })
    service.devices.mockResolvedValue({ devices: [] })
    service.routerConn.mockResolvedValue({ address: '', user: '', key_saved: false })
    service.deviceLookups.mockResolvedValue({ enabled: true, devices: [] })
  })

  it('says in one line whether the naming tables are on and links to Settings, instead of drawing their cards', async () => {
    const w = await mountTab()
    expect(w.get('[data-test="traffic-tables-line"]').text()).toContain('are off')
    expect(w.getComponent(RouterLinkStub).props('to')).toEqual({ name: 'network', params: { tab: 'settings' }, query: { group: 'data' } })
    expect(w.get('[data-test="traffic-tables-link"]').text()).toBe('Turn on')
    expect(w.find('[data-test="destination-data"]').exists()).toBe(false)
    service.destinationData.mockResolvedValue({ ranges: 500_000, source: 'iptoasn.com' })
    service.destinationPlaces.mockResolvedValue({ ranges: 7_000_000, source: 'db-ip.com' })
    const on = await mountTab()
    expect(on.get('[data-test="traffic-tables-line"]').text()).toBe('Operator and city names are on.')
    expect(on.get('[data-test="traffic-tables-link"]').text()).toBe('Manage')
  })

  it('shows loading until the helper and the last look have been read', () => {
    service.helper.mockReturnValue(new Promise(() => undefined))
    const w = mount(TrafficTab, { global: { stubs: { RouterLink: RouterLinkStub } } })
    expect(w.find('[data-test="traffic-loading"]').exists()).toBe(true)
    expect(w.find('[data-test="traffic-listen"]').exists()).toBe(false)
  })

  it('invites the first look with one button, and says it asks for the passphrase', async () => {
    const w = await mountTab()
    expect(w.get('[data-test="traffic-never"]').text()).toContain('No devices yet')
    expect(w.get('[data-test="traffic-listen"]').text()).toBe('Listen now')
    expect(w.text()).toContain('Asks for your passphrase each time.')
  })

  it('says why it cannot listen and where the fix is, instead of showing a dead button', async () => {
    service.helper.mockResolvedValue(ready({ actions: [{ id: 'capture_host', on: false, installed: true }] }))
    const w = await mountTab()
    expect(w.find('[data-test="traffic-listen"]').exists()).toBe(false)
    expect(w.get('[data-test="traffic-unavailable"]').text()).toContain('switched off')
    await w.get('[data-test="traffic-fix"]').trigger('click')
    expect(w.emitted('open-tab')).toEqual([['settings']])
  })

  it('has no fix button when the fix is not on this page, as on a computer that is not Linux', async () => {
    service.helper.mockResolvedValue(ready({ supported: false }))
    const w = await mountTab()
    expect(w.get('[data-test="traffic-unavailable"]').text()).toContain('needs Linux')
    expect(w.find('[data-test="traffic-fix"]').exists()).toBe(false)
  })

  it('still shows the last look when listening is not available now', async () => {
    service.helper.mockResolvedValue(ready({ installed: false, ready: false }))
    service.lastHostTraffic.mockResolvedValue(kept)
    const w = await mountTab()
    expect(w.find('[data-test="traffic-unavailable"]').exists()).toBe(true)
    expect(w.find('[data-test="traffic-headline"]').exists()).toBe(true)
  })

  it('listens, says it is listening, then shows the result with when and where', async () => {
    let finish: (v: unknown) => void = () => undefined
    service.captureHost.mockReturnValue(new Promise((resolve) => { finish = resolve }))
    const w = await mountTab()
    await w.get('[data-test="traffic-listen"]').trigger('click')
    expect(w.get('[data-test="traffic-listen"]').text()).toBe('Listening. This takes up to a minute.')
    expect(w.get('[data-test="traffic-listen"]').attributes('disabled')).toBeDefined()
    service.lastHostTraffic.mockResolvedValue(kept)
    finish({ parsed: kept })
    await flushPromises()
    expect(w.get('[data-test="traffic-headline"]').text()).toContain('sent 1 KB and received 2 KB')
    expect(w.get('[data-test="traffic-when"]').text()).toContain('wlp2s0')
    expect(w.find('[data-test="traffic-never"]').exists()).toBe(false)
  })

  it('says plainly when a press failed, keeps the last look, and lets the person try again', async () => {
    service.lastHostTraffic.mockResolvedValue(kept)
    service.captureHost.mockRejectedValue(new Error('the passphrase was not confirmed'))
    const w = await mountTab()
    await w.get('[data-test="traffic-listen"]').trigger('click')
    await flushPromises()
    expect(w.get('[data-test="traffic-error"]').text()).toContain('passphrase')
    expect(w.find('[data-test="traffic-headline"]').exists()).toBe(true)
    expect(w.get('[data-test="traffic-listen"]').attributes('disabled')).toBeUndefined()
  })

  it('says when the last look could not be read, without hiding the button', async () => {
    service.lastHostTraffic.mockRejectedValue(new Error('disk'))
    const w = await mountTab()
    expect(w.get('[data-test="traffic-load-error"]').text()).not.toBe('')
    expect(w.find('[data-test="traffic-listen"]').exists()).toBe(true)
  })
  it('after a reload mid-look, shows the look as running and then its result, with no press', async () => {
    vi.useFakeTimers()
    try {
      service.helper.mockResolvedValue(ready({ actions: [{ id: 'capture_host', on: true, installed: true, running: true }] }))
      const w = await mountTab()
      expect(w.get('[data-test="traffic-listen"]').attributes('disabled')).toBeDefined()
      expect(w.get('[data-test="traffic-listen"]').text()).toBe('Listening. This takes up to a minute.')
      service.helper.mockResolvedValue(ready())
      service.lastHostTraffic.mockResolvedValue(kept)
      await vi.advanceTimersByTimeAsync(3000)
      await flushPromises()
      expect(w.get('[data-test="traffic-listen"]').attributes('disabled')).toBeUndefined()
      expect(w.find('[data-test="traffic-report"]').exists()).toBe(true)
      w.unmount()
    } finally {
      vi.useRealTimers()
    }
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
      const w = await mountTab()
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
      const w = await mountTab()
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
