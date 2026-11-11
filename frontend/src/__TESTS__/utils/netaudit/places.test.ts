import { describe, expect, it } from 'vitest'
import { freshCount, groupPlaces, measureOf, placeDots, placesFromHost, placesFromRouter, trafficDevices, unlocatedCount } from '../../../utils/netaudit/places'
import type { DeviceLookups, HostTraffic, NetworkDevice, PlaceRow, RouterConnections, TrafficOwner } from '../../../types/netaudit'

const owner = (name: string, asn: number, city?: string, point?: { lat: number; lon: number }, country = 'US'): TrafficOwner => ({
  asn, name, country, ...(city ? { city, place_country: country } : {}), ...(point ? { point } : {}),
})
const GOOGLE = (city: string, point: { lat: number; lon: number }) => owner('GOOGLE - Google LLC', 15169, city, point)
const row = (address: string, count: number, o?: TrafficOwner, over: Partial<PlaceRow> = {}): PlaceRow => ({ address, port: 443, proto: 'udp', count, owner: o, source: 'router', ...over })
const MV = { lat: 37.4, lon: -122.1 }
const MTL = { lat: 45.5, lon: -73.6 }

describe('grouping the places of a device', () => {
  const rows = [
    row('142.251.155.120', 31, GOOGLE('Mountain View', MV)),
    row('142.251.30.95', 8, GOOGLE('Mountain View', MV)),
    row('142.250.151.100', 11, GOOGLE('Montreal', MTL)),
    row('160.79.104.10', 4, owner('ANTHROPIC - Anthropic, PBC', 399358, 'San Francisco', { lat: 37.8, lon: -122.4 })),
    row('198.51.100.7', 2),
  ]

  it('collapses the addresses of one company into one group, biggest first, with the cities it is used in', () => {
    const groups = groupPlaces(rows, 'company')
    expect(groups.map((g) => [g.title, g.rows.length, g.count])).toEqual([['Google LLC', 3, 50], ['Anthropic, PBC', 1, 4], ['Owner not known', 1, 2]])
    expect(groups[0]!.where).toEqual(['Mountain View', 'Montreal'])
    expect(groups[0]!.unlocated).toBe(false)
  })

  it('marks a group none of whose places has a point, and counts such places', () => {
    const groups = groupPlaces(rows, 'company')
    expect(groups.find((g) => g.title === 'Owner not known')!.unlocated).toBe(true)
    expect(unlocatedCount(rows)).toBe(1)
  })

  it('groups by country using where an address is used before where it is registered, and lists the companies there', () => {
    const ca = row('142.250.151.100', 11, { ...GOOGLE('Montreal', MTL), place_country: 'CA' })
    const groups = groupPlaces([...rows.slice(0, 2), ca], 'country')
    expect(groups.map((g) => [g.title, g.count])).toEqual([['United States', 39], ['Canada', 11]])
    expect(groups[0]!.where).toEqual(['Google LLC'])
  })

  it('counts the places that are new today in each group, and over a device', () => {
    const fresh = [row('1.1.1.1', 3, owner('A - Alpha', 1), { fresh: true }), row('1.1.1.2', 2, owner('A - Alpha', 1)), row('2.2.2.2', 1, owner('B - Beta', 2))]
    expect(groupPlaces(fresh, 'company').map((g) => [g.title, g.fresh])).toEqual([['Alpha', 1], ['Beta', 0]])
    expect(freshCount(fresh)).toBe(1)
  })

  it('orders by bytes when the rows have them, and by connections otherwise', () => {
    const byBytes = groupPlaces([row('1.1.1.1', 9, owner('A - Alpha', 1), { bytes: 10 }), row('2.2.2.2', 1, owner('B - Beta', 2), { bytes: 900 })], 'company')
    expect(byBytes.map((g) => g.title)).toEqual(['Beta', 'Alpha'])
    expect(byBytes[0]!.bytes).toBe(900)
    expect(measureOf([row('1.1.1.1', 1, undefined, { bytes: 4 })])).toBe('bytes')
    expect(measureOf(rows)).toBe('connections')
  })
})

describe('the dots of the map', () => {
  it('draws a router place that has a point, with no look at this computer at all', () => {
    const router = placesFromRouter({ address: '192.168.50.251', total: 3, places: [{ address: '142.251.155.120', port: 443, proto: 'udp', count: 3, owner: GOOGLE('Mountain View', MV) }] })
    const dots = placeDots(router, new Set())
    expect(dots).toHaveLength(1)
    expect(dots[0]).toMatchObject({ address: '142.251.155.120', lat: 37.4, lon: -122.1, weight: 3, picked: false })
    expect(dots[0]!.label).toBe('Mountain View, United States · Google LLC')
  })

  it('joins places of one city into one dot that opens the heaviest of them, and marks the picked places', () => {
    const rows = [row('142.251.30.95', 8, GOOGLE('Mountain View', MV)), row('142.251.155.120', 31, GOOGLE('Mountain View', MV)), row('1.1.1.1', 2)]
    const dots = placeDots(rows, new Set(['142.251.30.95']))
    expect(dots).toHaveLength(1)
    expect(dots[0]).toMatchObject({ address: '142.251.155.120', weight: 39, picked: true })
  })
})

describe('the adapters', () => {
  it('keeps the port, protocol and state of a router place and counts its connections', () => {
    const [r] = placesFromRouter({ address: '192.168.50.9', total: 2, places: [{ address: '8.8.8.8', port: 53, proto: 'udp', state: 'ESTABLISHED', count: 2 }] })
    expect(r).toEqual({ address: '8.8.8.8', port: 53, proto: 'udp', state: 'ESTABLISHED', count: 2, owner: undefined, source: 'router' })
  })

  it('carries the router watch\'s mark that a place is new today', () => {
    const [r] = placesFromRouter({ address: '192.168.50.9', total: 1, places: [{ address: '8.8.8.8', port: 53, proto: 'udp', count: 1, new: true }] })
    expect(r!.fresh).toBe(true)
  })

  it('turns the internet places of a look into rows with bytes, leaving this network and broadcast chatter out', () => {
    const t = {
      bytes_in: 100, bytes_out: 100, bytes_through: 0, internet_bytes: 150, local_network_bytes: 50, lookups: 0, kinds: [], per_second: [],
      destinations: [
        { address: '104.16.1.1', bytes: 150, internet: true, owner: owner('CLOUDFLARENET - Cloudflare, Inc.', 13335, 'Toronto', { lat: 43.7, lon: -79.4 }) },
        { address: '192.168.50.9', bytes: 50, internet: false },
      ],
      flows: [
        { address: '104.16.1.1', port: 443, proto: 'tcp', kind: 'https', internet: true, bytes_out: 50, bytes_in: 100, name: 'example.com' },
        { address: '104.16.1.1', port: 443, proto: 'udp', kind: 'quic', internet: true, bytes_out: 0, bytes_in: 0 },
      ],
      other_flows: 0, other_bytes: 0,
    } as unknown as HostTraffic
    const rows = placesFromHost(t, {})
    expect(rows).toHaveLength(1)
    expect(rows[0]).toMatchObject({ address: '104.16.1.1', bytes: 150, count: 2, name: 'example.com', source: 'host' })
  })
})

describe('the devices of the page', () => {
  const router: RouterConnections = {
    at: '2026-11-11T16:00:00Z', lines: 10, kept: 8,
    devices: [
      { address: '192.168.50.251', total: 171, places: [{ address: '142.251.155.120', port: 443, proto: 'udp', count: 171 }] },
      { address: '192.168.50.10', total: 5, places: [{ address: '8.8.8.8', port: 53, proto: 'udp', count: 5 }] },
      { address: '192.168.50.77', total: 40, places: [{ address: '1.1.1.1', port: 443, proto: 'tcp', count: 40 }] },
    ],
  }
  const inventory = (ip: string, name: string, mac: string) => ({ ip, name, mac, vendor: '', randomized: false }) as NetworkDevice
  const known = [inventory('192.168.50.251', 'M4 MacBook Air', 'aa:aa'), inventory('192.168.50.10', 'Pixel 8', 'bb:bb')]
  const lookups = [{ mac: 'bb:bb', flags: ['known_bad'] }] as unknown as DeviceLookups[]
  const hostRows = [row('104.16.1.1', 2, undefined, { bytes: 150, source: 'host' })]

  it('lists this computer first when it has looked, then the router devices by what they held, naming them from the inventory', () => {
    const devices = trafficDevices({ host: { interface: 'wlp2s0', at: '2026-11-11T16:01:00Z', rows: hostRows }, router, inventory: known, lookups })
    expect(devices.map((d) => [d.id, d.name])).toEqual([
      ['host', 'This computer'], ['router:192.168.50.251', 'M4 MacBook Air'], ['router:192.168.50.77', 'Device 192.168.50.77'], ['router:192.168.50.10', 'Pixel 8'],
    ])
    expect(devices[0]).toMatchObject({ kind: 'host', address: 'wlp2s0', measure: 'bytes' })
    expect(devices[1]).toMatchObject({ kind: 'router', address: '192.168.50.251', measure: 'connections', mac: 'aa:aa' })
  })

  it('carries the flags the watch raised for a device, found through its hardware address, and none for an unknown one', () => {
    const devices = trafficDevices({ host: null, router, inventory: known, lookups })
    expect(devices.find((d) => d.name === 'Pixel 8')!.flags).toEqual(['known_bad'])
    expect(devices.find((d) => d.name === 'M4 MacBook Air')!.flags).toEqual([])
    expect(devices.find((d) => d.address === '192.168.50.77')!.flags).toEqual([])
  })

  it('keeps a device that only asked for names, with no places, so what it looked up is still reachable', () => {
    const asked = [{ mac: 'cc:cc', flags: ['beaconing'] }, { mac: 'dd:dd', flags: [] }] as unknown as DeviceLookups[]
    const devices = trafficDevices({ host: null, router: null, inventory: [inventory('192.168.50.30', 'Thermostat', 'cc:cc')], lookups: asked })
    expect(devices.map((d) => [d.name, d.address, d.rows.length, d.flags])).toEqual([['Thermostat', '192.168.50.30', 0, ['beaconing']], ['Unknown device', 'dd:dd', 0, []]])
    expect(devices[0]!.mac).toBe('cc:cc')
  })

  it('has no devices before anything was read', () => {
    expect(trafficDevices({ host: null, router: null, inventory: [], lookups: [] })).toEqual([])
  })
})
