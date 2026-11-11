import {
  DEVICE_UNNAMED, PLACES_DEVICE_NAME, PLACES_DOT_KEY_DIGITS, PLACES_NO_COUNTRY, PLACES_THIS_COMPUTER, PLACES_UNKNOWN_OWNER, PLACES_WHERE_SHOWN,
} from '../../constants/netaudit'
import { deviceLabel } from './devices'
import { countryName, ownerName, talkGroups, usedIn } from './traffic'
import type {
  DeviceLookups, HostTraffic, LookupFlag, MapDot, NetworkDevice, PlaceGroup, PlaceGroupBy, PlaceMeasure, PlaceRow, RouterConnections, RouterDevice, TrafficDevice,
} from '../../types/netaudit'

// The shared model of the Traffic page. A look at this computer and a read of the router's table both end up as PlaceRow lists, and everything
// after that (the groups, the dots, the device list) is written once over those rows.

/** The places of one device of the router's table, as rows. */
export function placesFromRouter(device: RouterDevice): PlaceRow[] {
  return device.places.map((p) => ({ address: p.address, port: p.port, proto: p.proto, state: p.state, count: p.count, owner: p.owner, fresh: p.new || undefined, source: 'router' as const }))
}

/** The internet places of a look at this computer, as rows: bytes from the look, and how many conversations were held. This network and broadcast chatter are not places. */
export function placesFromHost(t: HostTraffic, deviceNames: Readonly<Record<string, string>>): PlaceRow[] {
  const internet = talkGroups(t, deviceNames).find((g) => g.id === 'internet')?.rows ?? []
  return internet.map((r) => ({
    address: r.address,
    count: Math.max(1, t.flows.filter((f) => f.address === r.address && !f.self).length),
    bytes: r.bytes,
    name: r.named ? r.label : undefined,
    owner: r.owner,
    source: 'host' as const,
  }))
}

/** Bytes when the rows have them (a look at this computer), connections otherwise (the router keeps no bytes). */
export function measureOf(rows: readonly PlaceRow[]): PlaceMeasure {
  return rows.some((r) => r.bytes !== undefined) ? 'bytes' : 'connections'
}

const weightOf = (r: PlaceRow, measure: PlaceMeasure): number => (measure === 'bytes' ? (r.bytes ?? 0) : r.count)
const sumBy = (rows: readonly PlaceRow[], measure: PlaceMeasure): number => rows.reduce((n, r) => n + weightOf(r, measure), 0)
const unique = (items: readonly string[]): string[] => [...new Set(items)]

/** The key and title of the group a row belongs to. A row with no owner (or, by country, no country) is gathered into one group of its own. */
function groupOf(r: PlaceRow, by: PlaceGroupBy): { key: string; title: string } {
  if (by === 'company') {
    const title = r.owner ? ownerName(r.owner) : PLACES_UNKNOWN_OWNER
    return { key: r.owner ? `AS${r.owner.asn}` : '', title }
  }
  const code = r.owner?.place_country || r.owner?.country
  return code ? { key: code, title: countryName(code) } : { key: '', title: PLACES_NO_COUNTRY }
}

/** The rows grouped by company or by country, heaviest group first (by bytes when the rows have them). Rows keep that order inside a group. */
export function groupPlaces(rows: readonly PlaceRow[], by: PlaceGroupBy): PlaceGroup[] {
  const measure = measureOf(rows)
  const groups = new Map<string, PlaceGroup>()
  for (const r of [...rows].sort((a, b) => weightOf(b, measure) - weightOf(a, measure) || a.address.localeCompare(b.address))) {
    const { key, title } = groupOf(r, by)
    const g = groups.get(key) ?? { key, title, rows: [], count: 0, bytes: 0, where: [], unlocated: true, fresh: 0 }
    g.rows.push(r)
    g.count += r.count
    g.bytes += r.bytes ?? 0
    if (r.owner?.point) g.unlocated = false
    if (r.fresh) g.fresh++
    groups.set(key, g)
  }
  for (const g of groups.values()) {
    g.where = by === 'company'
      ? unique(g.rows.flatMap((r) => (r.owner?.city ? [r.owner.city] : [])))
      : unique(g.rows.map((r) => (r.owner ? ownerName(r.owner) : PLACES_UNKNOWN_OWNER)))
  }
  return [...groups.values()].sort((a, b) => sumBy(b.rows, measure) - sumBy(a.rows, measure) || a.title.localeCompare(b.title))
}

/** "Mountain View, Montreal and 2 more"'s parts: the first names and how many are left out. */
export function whereShown(where: readonly string[]): { shown: string[]; more: number } {
  return { shown: where.slice(0, PLACES_WHERE_SHOWN), more: Math.max(0, where.length - PLACES_WHERE_SHOWN) }
}

/** How many places have no point on the map (no table of cities, or the address is not in it). */
/** How many places are new today (the router watch). */
export const freshCount = (rows: readonly PlaceRow[]): number => rows.filter((r) => r.fresh).length

export function unlocatedCount(rows: readonly PlaceRow[]): number {
  return rows.filter((r) => !r.owner?.point).length
}

const dotKey = (lat: number, lon: number): string => `${lat.toFixed(PLACES_DOT_KEY_DIGITS)},${lon.toFixed(PLACES_DOT_KEY_DIGITS)}`

/** The dots of the map: the places that have a point, joined by city so many addresses of one hub are one dot, heaviest first. `picked` marks the places the page has open. */
export function placeDots(rows: readonly PlaceRow[], picked: ReadonlySet<string>): MapDot[] {
  const measure = measureOf(rows)
  const dots = new Map<string, MapDot & { names: Set<string>; best: number }>()
  for (const r of rows) {
    const p = r.owner?.point
    if (!p || !r.owner) continue
    const key = dotKey(p.lat, p.lon)
    const w = weightOf(r, measure)
    const dot = dots.get(key) ?? { key, address: r.address, label: '', lat: p.lat, lon: p.lon, weight: 0, picked: false, names: new Set<string>(), best: -1 }
    dot.weight += w
    if (w > dot.best) Object.assign(dot, { address: r.address, best: w })
    dot.picked ||= picked.has(r.address)
    dot.names.add(ownerName(r.owner))
    dot.label = [usedIn(r.owner), [...dot.names].join(', ')].filter(Boolean).join(' · ')
    dots.set(key, dot)
  }
  return [...dots.values()].map(({ names: _names, best: _best, ...dot }) => dot).sort((a, b) => b.weight - a.weight)
}

type DeviceSources = {
  host: { interface: string; at: string; rows: PlaceRow[] } | null
  router: RouterConnections | null
  inventory: readonly NetworkDevice[]
  lookups: readonly DeviceLookups[]
}

/** The flags the watch raised for the device with this hardware address; none when there is no address or no lookups for it. */
function flagsOf(mac: string | undefined, lookups: readonly DeviceLookups[]): LookupFlag[] {
  if (!mac) return []
  return lookups.find((l) => l.mac.toLowerCase() === mac.toLowerCase())?.flags ?? []
}

/**
 * The devices of the page: this computer first (when it has looked), then every device of the router's table, busiest first. A device is named
 * from the inventory by its address; one the inventory does not know is called by its address, so nothing is invented.
 */
export function trafficDevices({ host, router, inventory, lookups }: DeviceSources): TrafficDevice[] {
  const out: TrafficDevice[] = []
  if (host) out.push({ id: 'host', kind: 'host', name: PLACES_THIS_COMPUTER, address: host.interface, at: host.at, rows: host.rows, measure: 'bytes', flags: [] })
  const byIp = new Map(inventory.filter((d) => d.ip).map((d) => [d.ip, d]))
  const devices = [...(router?.devices ?? [])].sort((a, b) => b.total - a.total || a.address.localeCompare(b.address))
  for (const d of devices) {
    const known = byIp.get(d.address)
    out.push({
      id: `router:${d.address}`, kind: 'router', name: known ? deviceLabel(known) : PLACES_DEVICE_NAME(d.address), address: d.address,
      rows: placesFromRouter(d), measure: 'connections', at: router?.at, mac: known?.mac, flags: flagsOf(known?.mac, lookups),
    })
  }
  // A device that asked the router for names but holds no connection in the table (or the table was not read) is still a device of the page.
  const listed = new Set(out.flatMap((d) => (d.mac ? [d.mac] : [])))
  const byMac = new Map(inventory.map((d) => [d.mac.toLowerCase(), d]))
  for (const l of lookups) {
    const known = byMac.get(l.mac.toLowerCase())
    if (known && listed.has(known.mac)) continue
    out.push({
      id: `router:${known?.ip || l.mac}`, kind: 'router', name: known ? deviceLabel(known) : DEVICE_UNNAMED, address: known?.ip || l.mac,
      rows: [], measure: 'connections', at: router?.at, mac: known?.mac ?? l.mac, flags: l.flags,
    })
  }
  return out
}
