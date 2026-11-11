import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import TrafficReport from '../../../components/netaudit/TrafficReport.vue'
import { useNetauditViewMode } from '../../../composables/netaudit/useNetauditViewMode'
import type { HostTraffic, RouteTracesState } from '../../../types/netaudit'

const service = vi.hoisted(() => ({ devices: vi.fn(), routeTraces: vi.fn(), startRouteTrace: vi.fn() }))
vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: service }))

const MB = 1024 * 1024
const google = { asn: 15169, name: 'GOOGLE - Google LLC', country: 'US', place_country: 'US', city: 'Mountain View', region: 'California', point: { lat: 37.4, lon: -122.1 } }
const traffic = (over: Partial<HostTraffic> = {}): HostTraffic => ({
  at: '2026-11-09T20:00:00Z', interface: 'wlp2s0', stopped_by: 'window', elapsed_ms: 60_000, frames: 100, unreadable: 0, kernel_seen: 100, kernel_dropped: 0, sample: 1,
  bytes_in: 38 * MB, bytes_out: 4 * MB, bytes_through: 0, internet_bytes: 41 * MB, local_network_bytes: MB, lookups: 212,
  kinds: [{ kind: 'https', bytes: 40 * MB }, { kind: 'dns', bytes: MB }, { kind: 'udp', bytes: MB }],
  per_second: [100, 5000, 200],
  destinations: [
    { name: 'generativelanguage.googleapis.com', name_from: 'tls', address: '142.250.1.1', bytes: 22 * MB, internet: true, owner: google },
    { name: 'github.com', address: '140.82.1.1', bytes: 10 * MB, internet: true },
    { address: '192.168.50.7', bytes: 2 * MB, internet: false },
    { address: '224.0.0.251', bytes: 1000, internet: false },
  ],
  flows: [
    { address: '142.250.1.1', name: 'generativelanguage.googleapis.com', name_from: 'tls', port: 443, proto: 'tcp', kind: 'https', internet: true, bytes_out: 2 * MB, bytes_in: 20 * MB, owner: google },
    { address: '140.82.1.1', name: 'github.com', port: 443, proto: 'tcp', kind: 'https', internet: true, bytes_out: MB, bytes_in: 9 * MB },
    { address: '192.168.50.7', port: 53, proto: 'udp', kind: 'dns', internet: false, bytes_out: 100, bytes_in: 200 },
    { address: '224.0.0.251', port: 5353, proto: 'udp', kind: 'discovery', internet: false, bytes_out: 1000, bytes_in: 0 },
  ],
  other_flows: 3, other_bytes: 2 * MB, ...over,
})
const stubs = { DeviceProbe: { props: ['ip', 'mac', 'udpAt'], template: '<div data-test="probe-stub">{{ ip }}|{{ mac }}|{{ udpAt }}</div>' } }
const show = (t: HostTraffic) => mount(TrafficReport, { props: { traffic: t }, global: { stubs } })
const noTraces: RouteTracesState = { supported: true, traces: [] }
const openRow = async (w: ReturnType<typeof show>, label: string) => {
  await w.findAll('[data-test="talk-open"]').find((b) => b.text().includes(label))!.trigger('click')
}

describe('TrafficReport', () => {
  beforeEach(() => {
    service.devices.mockReset()
    service.devices.mockResolvedValue({ devices: [] })
    service.routeTraces.mockReset()
    service.routeTraces.mockResolvedValue(noTraces)
    service.startRouteTrace.mockReset()
    useNetauditViewMode().set('simple')
  })

  describe('the summary', () => {
    it('opens with one sentence, where it went and how encrypted it was', () => {
      const w = show(traffic())
      expect(w.get('[data-test="traffic-headline"]').text()).toBe('In 60 seconds this computer sent 4 MB and received 38 MB.')
      expect(w.get('[data-test="traffic-detail"]').text()).toContain('41 MB to the internet, 1 MB on this network.')
      expect(w.get('[data-test="traffic-only-this"]').text()).toContain('your phone and your other computers are not')
    })

    it('says it could not tell which way traffic went for a look kept before the fix, never that nothing was sent', () => {
      const w = show(traffic({ bytes_in: 0, bytes_out: 0, bytes_through: 1_664_770 }))
      expect(w.get('[data-test="traffic-headline"]').text()).toContain('could not tell which way it went')
    })

    it('sets this program\'s own page apart', () => {
      expect(show(traffic({ self_bytes: 21 * MB })).get('[data-test="traffic-self"]').text()).toContain('this program\'s own page and API')
      expect(show(traffic()).find('[data-test="traffic-self"]').exists()).toBe(false)
    })

    it('keeps the totals and the chart for Advanced', async () => {
      const w = show(traffic())
      expect(w.find('[data-test="traffic-stats"]').exists()).toBe(false)
      expect(w.find('[data-test="traffic-chart"]').exists()).toBe(false)
      useNetauditViewMode().set('advanced')
      await flushPromises()
      expect(w.get('[data-test="stat-sent"]').text()).toBe('4 MB')
      expect(w.get('[data-test="traffic-chart"]').attributes('aria-label')).toContain('busiest second')
    })

    it('says when the numbers are estimates, partial, or lost frames, and nothing for a clean window', () => {
      expect(show(traffic()).find('[data-test="traffic-notes"]').exists()).toBe(false)
      const notes = show(traffic({ sample: 8, stopped_by: 'frame_limit', kernel_dropped: 4 })).get('[data-test="traffic-notes"]').text()
      expect(notes).toContain('Estimated: one ordinary frame in 8')
      expect(notes).toContain('4 frames were dropped')
    })

    it('always says what the view can and cannot see, the tables and the route trace included', () => {
      const note = show(traffic()).get('[data-test="traffic-privacy"]').text()
      expect(note).toContain('only the traffic to and from this computer')
      expect(note).toContain('route trace')
    })
  })

  describe('who it talked to', () => {
    it('lists the internet, then this network, with broadcast chatter closed by default', () => {
      const w = show(traffic())
      const names = (id: string) => w.get(`[data-test="talk-${id}"]`).findAll('[data-test="talk-name"]').map((n) => n.text())
      expect(names('internet')).toEqual(['generativelanguage.googleapis.com', 'github.com'])
      expect(names('network')).toEqual(['192.168.50.7'])
      const shared = w.get('[data-test="talk-shared"]')
      expect(shared.element.tagName).toBe('DETAILS')
      expect((shared.element as HTMLDetailsElement).open).toBe(false)
      expect(names('shared')).toEqual(['224.0.0.251'])
    })

    it('says what each place was and who runs it, and which way the bytes went', () => {
      const row = show(traffic()).get('[data-test="talk-internet"]').findAll('[data-test="talk-row"]')[0]!
      expect(row.get('[data-test="talk-detail"]').text()).toBe('HTTPS · Google LLC · Mountain View, California, United States')
      expect(row.get('[data-test="talk-direction"]').text()).toContain('2 MB')
      expect(row.get('[data-test="talk-direction"]').attributes('aria-label')).toBe('sent 2 MB, received 20 MB')
    })

    it('names a device on this network the way the person does', async () => {
      service.devices.mockResolvedValue({ devices: [{ mac: 'aa:bb:cc:dd:ee:07', ip: '192.168.50.7', name: 'Printer' }] })
      const w = show(traffic())
      await flushPromises()
      const row = w.get('[data-test="talk-network"] [data-test="talk-row"]')
      expect(row.get('[data-test="talk-name"]').text()).toBe('Printer')
      expect(row.get('[data-test="talk-detail"]').text()).toContain('192.168.50.7')
    })

    it('leaves this program\'s own page out of the list', () => {
      const w = show(traffic({
        self_bytes: 2 * MB,
        flows: [{ address: '192.168.50.7', port: 4001, proto: 'tcp', kind: 'tcp', internet: false, served: true, self: true, bytes_out: 2 * MB, bytes_in: 0 }],
      }))
      expect(w.find('[data-test="talk-network"]').exists()).toBe(false)
    })

    it('counts what was not kept separately as "other" so the list adds up', () => {
      expect(show(traffic()).get('[data-test="talk-other"]').text()).toContain('3 other places')
    })

    it('treats a name from the network as text, never as markup', () => {
      const evil = '<img src=x onerror=alert(1)>'
      const w = show(traffic({ destinations: [{ name: evil, address: '1.2.3.4', bytes: MB, internet: true }], flows: [] }))
      expect(w.find('img').exists()).toBe(false)
      expect(w.get('[data-test="talk-name"]').text()).toBe(evil)
    })
  })

  describe('the map', () => {
    it('draws one dot per city of the internet traffic, and a click opens the place', async () => {
      const w = show(traffic())
      expect(w.findAll('[data-test="map-dot"]')).toHaveLength(1)
      await w.get('[data-test="map-dot"]').trigger('click')
      expect(w.get('[data-test="place-address"]').text()).toBe('142.250.1.1')
    })

    it('says how to get the map when no address has a city, and is left out when nothing went to the internet', () => {
      const plain = traffic({ destinations: [{ address: '1.2.3.4', bytes: MB, internet: true }], flows: [] })
      expect(show(plain).get('[data-test="map-empty"]').text()).toContain('Download the table')
      const local = traffic({ internet_bytes: 0, destinations: [{ address: '192.168.50.7', bytes: MB, internet: false }], flows: [] })
      expect(show(local).find('[data-test="traffic-map"]').exists()).toBe(false)
      expect(show(local).find('[data-test="map-empty"]').exists()).toBe(false)
    })

    it('draws the route of the open place when it has been traced', async () => {
      service.routeTraces.mockResolvedValue({
        supported: true,
        traces: [{ address: '142.250.1.1', state: 'done', started_at: '2026-11-11T14:00:00Z', reached: true, hops: [
          { ttl: 1, address: '62.252.1.1', sent: 3, answered: 3, best_ms: 5, where: 'internet', owner: { asn: 1, name: 'ISP', point: { lat: 51.5, lon: -0.1 } } },
          { ttl: 2, address: '142.250.1.1', sent: 3, answered: 3, best_ms: 90, where: 'internet', reached: true, owner: google },
        ] }],
      } satisfies RouteTracesState)
      const { useRouteTrace } = await import('../../../composables/netaudit/useRouteTrace')
      await useRouteTrace().load()
      const w = show(traffic())
      await openRow(w, 'generativelanguage')
      expect(w.find('[data-test="map-route"]').exists()).toBe(true)
      expect(w.findAll('[data-test="map-route-point"]')).toHaveLength(2)
      expect(w.get('[data-test="map-route-note"]').text()).toContain('not at this computer')
      expect(w.findAll('[data-test="route-hop"]')).toHaveLength(2)
      expect(w.get('[data-test="route-outcome"]').text()).toBe('Reached in 2 steps.')
      service.routeTraces.mockResolvedValue(noTraces)
      await useRouteTrace().load()
    })
  })

  describe('a place opened', () => {
    it('says who runs an internet address, where it is used and where it is registered, apart, and offers the route', async () => {
      const w = show(traffic({ destinations: [{ name: 'one.one.one.one', address: '104.16.1.1', bytes: MB, internet: true, owner: { asn: 13335, name: 'CLOUDFLARENET - Cloudflare, Inc.', country: 'US', place_country: 'CA', city: 'Toronto', region: 'Ontario' } }], flows: [] }))
      await openRow(w, 'one.one.one.one')
      expect(w.get('[data-test="place-owner"]').text()).toContain('Cloudflare, Inc.')
      expect(w.get('[data-test="place-owner"]').text()).toContain('AS13335')
      const where = w.get('[data-test="place-location"]').text()
      expect(where).toContain('used in Toronto, Ontario, Canada')
      expect(where).toContain('(approximate)')
      expect(where).toContain('registered in United States')
      expect(w.get('[data-test="route-start"]').text()).toBe('Trace the route')
      expect(w.find('[data-test="probe-stub"]').exists()).toBe(false)
    })

    it('starts a trace of the open address', async () => {
      service.startRouteTrace.mockResolvedValue({ supported: true, traces: [{ address: '140.82.1.1', state: 'running', started_at: '2026-11-11T14:00:00Z', reached: false, hops: [] }] })
      const w = show(traffic())
      await openRow(w, 'github.com')
      await w.get('[data-test="route-start"]').trigger('click')
      await flushPromises()
      expect(service.startRouteTrace).toHaveBeenCalledWith('140.82.1.1')
      expect(w.get('[data-test="route-start"]').text()).toContain('Tracing')
      service.routeTraces.mockResolvedValue(noTraces)
      const { useRouteTrace } = await import('../../../composables/netaudit/useRouteTrace')
      await useRouteTrace().load()
    })

    it('says a trace cannot run here instead of offering the button', async () => {
      service.routeTraces.mockResolvedValue({ supported: false, traces: [] })
      const { useRouteTrace } = await import('../../../composables/netaudit/useRouteTrace')
      await useRouteTrace().load()
      const w = show(traffic())
      await openRow(w, 'github.com')
      expect(w.find('[data-test="route-start"]').exists()).toBe(false)
      expect(w.get('[data-test="route-unsupported"]').text()).toContain('needs Linux')
      service.routeTraces.mockResolvedValue(noTraces)
      await useRouteTrace().load()
    })

    it('names a conversation served here by this computer\'s port, and marks this page\'s own', async () => {
      const w = show(traffic({
        destinations: [{ address: '192.168.50.251', bytes: 3 * MB, internet: false }],
        flows: [
          { address: '192.168.50.251', port: 4001, proto: 'tcp', kind: 'tcp', internet: false, served: true, self: true, bytes_out: 2 * MB, bytes_in: 0 },
          { address: '192.168.50.251', port: 22, proto: 'tcp', kind: 'ssh', internet: false, served: true, bytes_out: MB, bytes_in: 0 },
        ],
      }))
      await openRow(w, '192.168.50.251')
      const services = w.findAll('[data-test="place-flow"]').map((r) => r.text())
      expect(services[0]).toContain('to this computer :4001')
      expect(services[0]).toContain('this page')
      expect(services[1]).toContain('SSH to this computer :22')
    })

    it('offers the device scans for a device on this network, with its MAC and kept scan', async () => {
      service.devices.mockResolvedValue({ devices: [{ mac: 'aa:bb:cc:dd:ee:07', ip: '192.168.50.7', udp_scanned_at: '2026-11-09T19:00:00Z' }] })
      const w = show(traffic())
      await flushPromises()
      await openRow(w, '192.168.50.7')
      await flushPromises()
      expect(w.get('[data-test="place-where"]').text()).toBe('On this network')
      expect(w.get('[data-test="probe-stub"]').text()).toBe('192.168.50.7|aa:bb:cc:dd:ee:07|2026-11-09T19:00:00Z')
    })

    it('never offers a scan or a trace for a multicast address, and says why', async () => {
      const w = show(traffic())
      await w.get('[data-test="talk-shared"] summary').trigger('click')
      await openRow(w, '224.0.0.251')
      expect(w.find('[data-test="probe-stub"]').exists()).toBe(false)
      expect(w.find('[data-test="route"]').exists()).toBe(false)
      expect(w.find('[data-test="place-shared"]').exists()).toBe(true)
    })

    it('closes the detail, and drops it when a new look arrives', async () => {
      const w = show(traffic())
      await openRow(w, 'github.com')
      await w.get('[data-test="traffic-detail-close"]').trigger('click')
      expect(w.find('[data-test="traffic-detail-panel"]').exists()).toBe(false)
      await openRow(w, 'github.com')
      await w.setProps({ traffic: traffic() })
      expect(w.find('[data-test="traffic-detail-panel"]').exists()).toBe(false)
    })
  })

  describe('Advanced', () => {
    beforeEach(() => { useNetauditViewMode().set('advanced') })
    const owned = () => traffic({
      owners_used: true,
      owners: [{ asn: 15169, name: 'GOOGLE - Google LLC', country: 'US', bytes: 22 * MB }, { asn: 36459, name: 'GITHUB', country: 'US', bytes: 10 * MB }],
      countries: [{ country: 'US', bytes: 32 * MB }, { country: 'DE', bytes: 4 * MB }],
    })
    const pick = async (w: ReturnType<typeof show>, label: string) => { await w.findAll('[role="radio"]').find((b) => b.text() === label)!.trigger('click') }

    it('ranks the internet traffic by operator and by country', async () => {
      const w = show(owned())
      const owners = w.get('[data-test="where-operators"]').findAll('[data-test="where-row"]')
      expect(owners.map((r) => r.get('[data-test="where-label"]').text())).toEqual(['Google LLC AS15169 · United States', 'GITHUB AS36459 · United States'])
      await pick(w, 'Countries')
      expect(w.get('[data-test="where-countries"]').findAll('[data-test="where-label"]')[1]!.text()).toContain('Germany')
    })

    it('says where to get owners when the look was made without the table', () => {
      expect(show(traffic()).get('[data-test="where-no-table"]').text()).toContain('Settings')
    })

    it('lists the conversations, filters them by kind, and opens a place from a row', async () => {
      const w = show(traffic())
      expect(w.findAll('[data-test="flow"]')).toHaveLength(4)
      await w.findAll('[data-test="kind-pick"]')[1]!.trigger('click')
      expect(w.findAll('[data-test="flow"]')).toHaveLength(1)
      await w.get('[data-test="kind-filter-clear"]').trigger('click')
      await w.findAll('[data-test="flow"]')[2]!.trigger('click')
      expect(w.get('[data-test="place-address"]').text()).toBe('192.168.50.7')
    })
  })
})

describe('TrafficReport problems link', () => {
  const lan = () => traffic({
    destinations: [{ address: '192.168.50.7', bytes: 2 * MB, internet: false }, { name: 'github.com', address: '140.82.1.1', bytes: MB, internet: true }],
    flows: [{ address: '192.168.50.7', port: 53, proto: 'udp', kind: 'dns', internet: false, bytes_out: 100, bytes_in: 200 }],
  })
  const open = async (lookup: { of: (a: string) => { count: number; severity: string } | null }) => {
    const { createMemoryHistory } = await import('vue-router')
    const { createAppRouter } = await import('../../../router')
    const { DEVICE_PROBLEMS_INJECTION } = await import('../../../composables/netaudit/useDeviceProblems')
    return mount(TrafficReport, { props: { traffic: lan() }, global: { stubs: { DeviceProbe: true }, plugins: [createAppRouter(createMemoryHistory())], provide: { [DEVICE_PROBLEMS_INJECTION as symbol]: lookup } } })
  }
  beforeEach(() => {
    service.devices.mockResolvedValue({ devices: [] })
    service.routeTraces.mockResolvedValue(noTraces)
  })

  it('links a device on this network that has open problems to them', async () => {
    const w = await open({ of: (a) => (a === '192.168.50.7' ? { count: 3, severity: 'high' } : null) })
    await w.get('[data-test="talk-network"] [data-test="talk-open"]').trigger('click')
    const link = w.get('[data-test="place-problems"] a')
    expect(link.attributes('href')).toBe('/network/findings?device=192.168.50.7')
    expect(link.text()).toBe('3 problems found on this device')
  })

  it('offers no link for an internet address or a device without problems', async () => {
    const w = await open({ of: () => ({ count: 1, severity: 'low' }) })
    await w.get('[data-test="talk-internet"] [data-test="talk-open"]').trigger('click')
    expect(w.find('[data-test="place-problems"]').exists()).toBe(false)
    const none = await open({ of: () => null })
    await none.get('[data-test="talk-network"] [data-test="talk-open"]').trigger('click')
    expect(none.find('[data-test="place-problems"]').exists()).toBe(false)
  })
})
