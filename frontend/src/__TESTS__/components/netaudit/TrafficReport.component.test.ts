import { beforeEach, describe, expect, it, vi } from 'vitest'
import { RouterLinkStub, flushPromises, mount } from '@vue/test-utils'
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
const stubs = { RouterLink: RouterLinkStub, DeviceProbe: { props: ['ip', 'mac', 'udpAt'], template: '<div data-test="probe-stub">{{ ip }}|{{ mac }}|{{ udpAt }}</div>' } }
const show = (t: HostTraffic) => mount(TrafficReport, { props: { traffic: t }, global: { stubs } })
const noTraces: RouteTracesState = { supported: true, traces: [] }
const openRow = async (w: ReturnType<typeof show>, label: string) => {
  await w.findAll('[data-test="talk-open"]').find((b) => b.text().includes(label))!.trigger('click')
}
// An internet place is a company group on the device's page; here it opens from the Advanced conversation table.
const openFlow = async (w: ReturnType<typeof show>, label: string) => {
  await w.findAll('[data-test="flow"]').find((r) => r.text().includes(label))!.trigger('click')
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

  describe('the footer', () => {
    it('always says what the view can and cannot see, the tables and the route trace included', () => {
      const note = show(traffic()).get('[data-test="traffic-privacy"]').text()
      expect(note).toContain('only the traffic to and from this computer')
      expect(note).toContain('route trace')
    })
  })

  describe('who it talked to', () => {
    it('lists this network, then broadcast chatter closed by default, and leaves the internet to the company groups of the page', () => {
      const w = show(traffic())
      const names = (id: string) => w.get(`[data-test="talk-${id}"]`).findAll('[data-test="talk-name"]').map((n) => n.text())
      expect(w.find('[data-test="talk-internet"]').exists()).toBe(false)
      expect(names('network')).toEqual(['192.168.50.7'])
      const shared = w.get('[data-test="talk-shared"]')
      expect(shared.element.tagName).toBe('DETAILS')
      expect((shared.element as HTMLDetailsElement).open).toBe(false)
      expect(names('shared')).toEqual(['224.0.0.251'])
    })

    it('says what each place was, and which way the bytes went', () => {
      const row = show(traffic()).get('[data-test="talk-network"]').findAll('[data-test="talk-row"]')[0]!
      expect(row.get('[data-test="talk-detail"]').text()).toBe('DNS')
      expect(row.get('[data-test="talk-direction"]').attributes('aria-label')).toBe('sent 100 B, received 200 B')
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
      const w = show(traffic({ destinations: [{ name: evil, address: '192.168.50.99', bytes: MB, internet: false }], flows: [] }))
      expect(w.find('img').exists()).toBe(false)
      expect(w.get('[data-test="talk-name"]').text()).toBe(evil)
    })
  })

  describe('a place opened', () => {
    it('says who runs an internet address, where it is used and where it is registered, apart, and offers the route', async () => {
      useNetauditViewMode().set('advanced')
      const owner = { asn: 13335, name: 'CLOUDFLARENET - Cloudflare, Inc.', country: 'US', place_country: 'CA', city: 'Toronto', region: 'Ontario' }
      const w = show(traffic({
        destinations: [{ name: 'one.one.one.one', address: '104.16.1.1', bytes: MB, internet: true, owner }],
        flows: [{ address: '104.16.1.1', name: 'one.one.one.one', port: 443, proto: 'tcp', kind: 'https', internet: true, bytes_out: 0, bytes_in: MB, owner }],
      }))
      await openFlow(w, 'one.one.one.one')
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
      useNetauditViewMode().set('advanced')
      const w = show(traffic())
      await openFlow(w, 'github.com')
      await w.get('[data-test="route-start"]').trigger('click')
      await flushPromises()
      expect(service.startRouteTrace).toHaveBeenCalledWith('140.82.1.1', false)
      expect(w.get('[data-test="route-start"]').text()).toContain('Tracing')
      service.routeTraces.mockResolvedValue(noTraces)
      const { useRouteTrace } = await import('../../../composables/netaudit/useRouteTrace')
      await useRouteTrace().load()
    })

    it('says a trace cannot run here instead of offering the button', async () => {
      service.routeTraces.mockResolvedValue({ supported: false, traces: [] })
      const { useRouteTrace } = await import('../../../composables/netaudit/useRouteTrace')
      await useRouteTrace().load()
      useNetauditViewMode().set('advanced')
      const w = show(traffic())
      await openFlow(w, 'github.com')
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
      useNetauditViewMode().set('advanced')
      const w = show(traffic())
      await openFlow(w, 'github.com')
      await w.get('[data-test="traffic-detail-close"]').trigger('click')
      expect(w.find('[data-test="traffic-detail-panel"]').exists()).toBe(false)
      await openFlow(w, 'github.com')
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

    it('answers "what kind" with bars in Simple, and keeps the conversation table for Advanced', async () => {
      useNetauditViewMode().set('simple')
      const w = show(traffic())
      expect(w.findAll('[data-test="kinds-bar"]')).toHaveLength(3)
      expect(w.get('[data-test="kinds-bars"]').text()).toContain('Web, encrypted (HTTPS)')
      expect(w.find('[data-test="flows"]').exists()).toBe(false)
    })

    it('lists the conversations with one Service column, filters them by kind, and opens a place from a row', async () => {
      useNetauditViewMode().set('advanced')
      const w = show(traffic())
      const headers = w.get('[data-test="flows"]').findAll('th').map((h) => h.text())
      expect(headers).toEqual(['Destination', 'Service', 'Sent', 'Received'])
      expect(w.findAll('[data-test="flow-service"]')[0]!.text()).toMatch(/^HTTPS\s*443$/)
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
    flows: [
      { address: '192.168.50.7', port: 53, proto: 'udp', kind: 'dns', internet: false, bytes_out: 100, bytes_in: 200 },
      { address: '140.82.1.1', name: 'github.com', port: 443, proto: 'tcp', kind: 'https', internet: true, bytes_out: 0, bytes_in: MB },
    ],
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
    useNetauditViewMode().set('advanced')
    await flushPromises()
    await w.findAll('[data-test="flow"]').find((r) => r.text().includes('github.com'))!.trigger('click')
    expect(w.find('[data-test="place-problems"]').exists()).toBe(false)
    useNetauditViewMode().set('simple')
    const none = await open({ of: () => null })
    await none.get('[data-test="talk-network"] [data-test="talk-open"]').trigger('click')
    expect(none.find('[data-test="place-problems"]').exists()).toBe(false)
  })
})
