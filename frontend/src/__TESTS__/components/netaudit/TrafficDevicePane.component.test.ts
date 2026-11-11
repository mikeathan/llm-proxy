import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { RouterLinkStub, flushPromises, mount } from '@vue/test-utils'
import TrafficDevicePane from '../../../components/netaudit/TrafficDevicePane.vue'
import { useNetauditViewMode } from '../../../composables/netaudit/useNetauditViewMode'
import type { PlaceRow, RouteTracesState, TrafficDevice, TrafficOwner } from '../../../types/netaudit'

const service = vi.hoisted(() => ({ routeTraces: vi.fn(), startRouteTrace: vi.fn() }))
vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: service }))

const owner = (name: string, asn: number, city?: string, point?: { lat: number; lon: number }, over: Partial<TrafficOwner> = {}): TrafficOwner =>
  ({ asn, name, country: 'US', ...(city ? { city, place_country: 'US' } : {}), ...(point ? { point } : {}), ...over })
const GOOGLE = (city: string, lat: number, lon: number) => owner('GOOGLE - Google LLC', 15169, city, { lat, lon })
const row = (address: string, count: number, o?: TrafficOwner, over: Partial<PlaceRow> = {}): PlaceRow => ({ address, port: 443, proto: 'udp', count, owner: o, source: 'router', ...over })
const device = (rows: PlaceRow[], over: Partial<TrafficDevice> = {}): TrafficDevice => ({
  id: 'router:192.168.50.251', kind: 'router', name: 'M4 MacBook Air', address: '192.168.50.251', measure: 'connections', at: '2026-11-12T10:00:00Z', mac: 'aa:aa', flags: [], rows, ...over,
})
const rows = [
  row('142.251.155.120', 31, GOOGLE('Mountain View', 37.4, -122.1)),
  row('142.251.30.95', 8, GOOGLE('Mountain View', 37.4, -122.1)),
  row('142.250.151.100', 11, { ...GOOGLE('Montreal', 45.5, -73.6), place_country: 'CA' }),
  row('160.79.104.10', 4, owner('ANTHROPIC - Anthropic, PBC', 399358, 'San Francisco', { lat: 37.8, lon: -122.4 })),
  row('198.51.100.7', 2),
]
const setWide = (wide: boolean) => {
  window.matchMedia = ((q: string) => ({ matches: wide && q.includes('1024'), media: q, addEventListener: () => undefined, removeEventListener: () => undefined })) as unknown as typeof window.matchMedia
}
const show = (d: TrafficDevice = device(rows)) =>
  mount(TrafficDevicePane, { props: { device: d, lookups: null, lookupsEnabled: true }, global: { stubs: { RouterLink: RouterLinkStub } } })
const group = (w: ReturnType<typeof show>, title: string) => w.findAll('[data-test="group-open"]').find((b) => b.text().includes(title))!
const noTraces: RouteTracesState = { supported: true, traces: [] }

describe('TrafficDevicePane', () => {
  beforeEach(() => {
    service.routeTraces.mockReset()
    service.routeTraces.mockResolvedValue(noTraces)
    service.startRouteTrace.mockReset()
    useNetauditViewMode().set('simple')
    setWide(true)
  })
  afterEach(() => setWide(false))

  it('says who it is and where it was read, with the figures of its places', () => {
    const w = show()
    expect(w.get('[data-test="device-name"]').text()).toContain('M4 MacBook Air')
    expect(w.get('[data-test="device-source"]').text()).toBe('The router’s table')
    expect(['total', 'places', 'companies', 'countries'].map((id) => w.get(`[data-test="stat-${id}"]`).text())).toEqual(['56', '5', '3', '2'])
  })

  it('collapses the addresses of one company into one row, with the cities it is used in, biggest first', () => {
    const w = show()
    const titles = w.findAll('[data-test="group-title"]').map((t) => t.text())
    expect(titles).toEqual(['Google LLC', 'Anthropic, PBC', 'Owner not known'])
    expect(w.findAll('[data-test="group-where"]')[0]!.text()).toBe('Mountain View, Montreal · 3 places')
    expect(w.findAll('[data-test="group-figure"]')[0]!.text()).toBe('50')
  })

  it('draws the map from the places alone, one dot per city, and says its size is connections', () => {
    const w = show()
    expect(w.findAll('[data-test="map-dot"]')).toHaveLength(3)
    expect(w.get('[data-test="map-notes"]').text()).toContain('Dot size is connections held. The router keeps no byte counts.')
  })

  it('shows the places of a picked company beside the list, and a dot picks the company, a second press clears it', async () => {
    const w = show()
    expect(w.get('[data-test="detail-empty"]').text()).toContain('Pick a company or a dot')
    await group(w, 'Google LLC').trigger('click')
    expect(w.get('[data-test="detail-title"]').text()).toBe('Google LLC')
    expect(w.findAll('[data-test="place-address"]').map((a) => a.text())).toEqual(['142.251.155.120:443 udp', '142.250.151.100:443 udp', '142.251.30.95:443 udp'])
    await w.get('[data-test="detail-clear"]').trigger('click')
    expect(w.find('[data-test="detail-title"]').exists()).toBe(false)
    await w.findAll('[data-test="map-dot"]')[0]!.trigger('click')
    expect(w.get('[data-test="detail-title"]').text()).toBe('Google LLC')
    await w.findAll('[data-test="map-dot"]')[0]!.trigger('click')
    expect(w.find('[data-test="detail-title"]').exists()).toBe(false)
  })

  it('opens the route under the place, says it is traced from this computer, and starts it only on the press', async () => {
    service.startRouteTrace.mockResolvedValue({ supported: true, traces: [{ address: '160.79.104.10', state: 'running', started_at: '2026-11-12T10:00:00Z', reached: false, hops: [] }] })
    const w = show()
    await group(w, 'Anthropic').trigger('click')
    await w.get('[data-test="place-route"]').trigger('click')
    expect(w.get('[data-test="place-route-from"]').text()).toBe('Routes are traced from this computer, not from the device.')
    expect(service.startRouteTrace).not.toHaveBeenCalled()
    await w.get('[data-test="route-start"]').trigger('click')
    await flushPromises()
    expect(service.startRouteTrace).toHaveBeenCalledWith('160.79.104.10', false)
    await w.get('[data-test="place-route"]').trigger('click')
    expect(w.find('[data-test="place-route-panel"]').exists()).toBe(false)
  })

  it('draws the open route on the map', async () => {
    service.routeTraces.mockResolvedValue({
      supported: true,
      traces: [{ address: '160.79.104.10', state: 'done', started_at: '2026-11-12T10:00:00Z', reached: true, hops: [
        { ttl: 1, address: '62.252.1.1', sent: 3, answered: 3, best_ms: 5, where: 'internet', owner: { asn: 1, name: 'ISP', point: { lat: 51.5, lon: -0.1 } } },
        { ttl: 2, address: '160.79.104.10', sent: 3, answered: 3, best_ms: 140, where: 'internet', reached: true, owner: owner('ANTHROPIC - Anthropic, PBC', 399358, 'San Francisco', { lat: 37.8, lon: -122.4 }) },
      ] }],
    } satisfies RouteTracesState)
    const { useRouteTrace } = await import('../../../composables/netaudit/useRouteTrace')
    await useRouteTrace().load()
    const w = show()
    await group(w, 'Anthropic').trigger('click')
    expect(w.find('[data-test="map-route"]').exists()).toBe(false)
    await w.get('[data-test="place-route"]').trigger('click')
    expect(w.find('[data-test="map-route"]').exists()).toBe(true)
    expect(w.findAll('[data-test="map-route-point"]')).toHaveLength(2)
    expect(w.get('[data-test="map-route-note"]').text()).toContain('not at this computer')
    service.routeTraces.mockResolvedValue(noTraces)
    await useRouteTrace().load()
  })

  it('regroups by country, where an address is used, and lists the companies there', async () => {
    const w = show()
    await w.findAll('[role="radio"]').find((b) => b.text() === 'Country')!.trigger('click')
    expect(w.findAll('[data-test="group-title"]').map((t) => t.text())).toEqual(['United States', 'Canada', 'No location'])
    expect(w.findAll('[data-test="group-where"]')[0]!.text()).toBe('Google LLC, Anthropic, PBC · 3 places')
  })

  it('lists only the places with no location on request, and says how many there are', async () => {
    const w = show()
    expect(w.get('[data-test="map-unlocated"]').text()).toBe('1 place has no location')
    await w.get('[data-test="map-unlocated"]').trigger('click')
    expect(w.findAll('[data-test="group-title"]').map((t) => t.text())).toEqual(['Owner not known'])
    expect(w.find('[data-test="map-empty"]').exists()).toBe(true)
  })

  it('finds a company or address by what is typed', async () => {
    const w = show()
    await w.get('[data-test="group-search"]').setValue('160.79')
    expect(w.findAll('[data-test="group-title"]').map((t) => t.text())).toEqual(['Anthropic, PBC'])
    await w.get('[data-test="group-search"]').setValue('zzz')
    expect(w.get('[data-test="groups-none"]').text()).toBe('Nothing matches.')
  })

  it('measures this computer in bytes and says the dot size is bytes', () => {
    const w = show(device([row('104.16.1.1', 2, GOOGLE('Toronto', 43.7, -79.4), { bytes: 3 * 1024 * 1024, source: 'host' })], { kind: 'host', id: 'host', name: 'This computer', address: 'wlp2s0', measure: 'bytes' }))
    expect(w.get('[data-test="stat-total"]').text()).toBe('3 MB')
    expect(w.get('[data-test="map-notes"]').text()).toContain('Dot size is bytes carried.')
    expect(w.get('[data-test="group-figure"]').text()).toBe('3 MB')
  })

  it('on a phone opens the places right under the company, where a route can open too', async () => {
    setWide(false)
    const w = show()
    expect(w.find('[data-test="group-detail"]').exists()).toBe(false)
    await group(w, 'Anthropic').trigger('click')
    expect(w.get('[data-test="group-body"] [data-test="place-address"]').text()).toBe('160.79.104.10:443 udp')
    await w.get('[data-test="place-route"]').trigger('click')
    expect(w.find('[data-test="place-route-panel"]').exists()).toBe(true)
  })

  it('says what is missing when a device has no places: the router not read, or no connection at the read', () => {
    expect(show(device([], { at: undefined })).get('[data-test="device-empty"]').text()).toContain('has not been read yet')
    expect(show(device([])).get('[data-test="device-empty"]').text()).toContain('held no connection')
  })

  it('says how to get the map when no address has a city, with a link to the tables', () => {
    const w = show(device([row('198.51.100.7', 2, owner('X - X Ltd', 1))]))
    expect(w.get('[data-test="map-empty"]').text()).toContain('Download the table')
    expect(w.getComponent(RouterLinkStub).props('to')).toEqual({ name: 'network', params: { tab: 'settings' }, query: { group: 'data' } })
  })

  it('marks what is new today on the company, the place and the figures, and can list only that', async () => {
    const withNew = [row('47.89.80.209', 5, owner('ALIBABA - Alibaba (US)', 45102, 'Frankfurt', { lat: 50.1, lon: 8.7 }), { fresh: true }), ...rows]
    const w = show(device(withNew))
    expect(w.get('[data-test="stat-fresh"]').text()).toBe('1')
    expect(w.findAll('[data-test="group-new"]')).toHaveLength(1)
    await w.findAll('[role="radio"]').find((b) => b.text() === 'New today 1')!.trigger('click')
    expect(w.findAll('[data-test="group-title"]').map((t) => t.text())).toEqual(['Alibaba (US)'])
    await group(w, 'Alibaba').trigger('click')
    expect(w.get('[data-test="place-new"]').text()).toBe('New today')
  })

  it('shows no New today stat or filter when nothing is new, as with the watch off', () => {
    const w = show()
    expect(w.find('[data-test="stat-fresh"]').exists()).toBe(false)
    expect(w.find('[data-test="group-new"]').exists()).toBe(false)
    expect(w.findAll('[role="radio"]').some((b) => b.text().startsWith('New today'))).toBe(false)
  })

  it('starts clean for another device', async () => {
    const w = show()
    await group(w, 'Google LLC').trigger('click')
    expect(w.find('[data-test="detail-title"]').exists()).toBe(true)
    await w.setProps({ device: device(rows, { id: 'router:other' }) })
    expect(w.find('[data-test="detail-title"]').exists()).toBe(false)
  })
})
