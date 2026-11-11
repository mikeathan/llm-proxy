import { formatBytes } from '../format/formatters'
import {
  ROUTE_FAILED, ROUTE_HOP_SILENT, ROUTE_HOP_TIME, ROUTE_HOP_WHERE, ROUTE_NOT_REACHED, ROUTE_PASSES_THROUGH, ROUTE_REACHED, ROUTE_SUMMARY, TRAFFIC_DROPPED, TRAFFIC_ENCRYPTED, TRAFFIC_ENCRYPTED_KINDS,
  TRAFFIC_HEADLINE, TRAFFIC_HEADLINE_QUIET, TRAFFIC_HEADLINE_UNKNOWN, TRAFFIC_KIND_LABELS, TRAFFIC_KIND_SHORT, TRAFFIC_LOOKUPS, TRAFFIC_OWNER_LINE, TRAFFIC_REACH,
  TRAFFIC_REGISTERED_IN, TRAFFIC_SAMPLED, TRAFFIC_SELF_LINE, TRAFFIC_SERVED, TRAFFIC_STOPPED,
} from '../../constants/netaudit'
import type {
  ChartBar, GeoPoint, HostTraffic, RouteHop, RouteLine, RouteTrace, TalkGroup, TalkGroupId, TalkRow, TrafficFlow, TrafficKind, TrafficOwner, TrafficPlace, WhereBar,
} from '../../types/netaudit'

const MS_PER_SECOND = 1000
/** IPtoASN names a holder "CLOUDFLARENET - Cloudflare, Inc.": its network's handle, then its own name. */
const OWNER_NAME_SEPARATOR = ' - '
const SECONDS_PER_MINUTE = 60
const PERCENT = 100
/** The least height a bar has, so a quiet second is still a mark on the chart. */
const HAIRLINE = 1
/** The space between two bars, in the chart's own units. */
const BAR_GAP = 2

const percentOf = (part: number, whole: number): number => (whole > 0 ? Math.round((part / whole) * PERCENT) : 0)

/** True when the look could not tell which packets were this computer's: the server says so, or (a look kept before it could) nothing counted as sent or received. */
export function directionUnknown(t: HostTraffic): boolean {
  return Boolean(t.direction_unknown) || (t.bytes_in + t.bytes_out === 0 && t.bytes_through > 0)
}

/** The one-sentence summary of a look, and the details after it. `detail` is empty when there is nothing more to say. */
export function trafficSentence(t: HostTraffic): { headline: string; detail: string } {
  const seconds = Math.max(1, Math.round(t.elapsed_ms / MS_PER_SECOND))
  const total = t.bytes_in + t.bytes_out + t.bytes_through
  const headline = directionUnknown(t)
    ? TRAFFIC_HEADLINE_UNKNOWN(seconds, formatBytes(total))
    : total === 0 ? TRAFFIC_HEADLINE_QUIET(seconds) : TRAFFIC_HEADLINE(seconds, formatBytes(t.bytes_out), formatBytes(t.bytes_in))
  if (total === 0) return { headline, detail: '' }
  const parts = [TRAFFIC_REACH(formatBytes(t.internet_bytes), formatBytes(t.local_network_bytes))]
  const encrypted = t.kinds.filter((k) => TRAFFIC_ENCRYPTED_KINDS.includes(k.kind)).reduce((n, k) => n + k.bytes, 0)
  const kindTotal = t.kinds.reduce((n, k) => n + k.bytes, 0)
  if (encrypted > 0) parts.push(TRAFFIC_ENCRYPTED(percentOf(encrypted, kindTotal)))
  if (t.lookups > 0) parts.push(TRAFFIC_LOOKUPS(t.lookups))
  return { headline, detail: parts.join(' ') }
}

/** The line that sets this program's own page and API apart; empty when the look had none. */
export function selfLine(t: HostTraffic): string {
  const self = t.self_bytes ?? 0
  if (self <= 0) return ''
  return TRAFFIC_SELF_LINE(formatBytes(self), percentOf(self, t.bytes_in + t.bytes_out + t.bytes_through))
}

/** Each kind of traffic with its words and its share of the whole, biggest first; none when there was no traffic. */
export function kindShares(kinds: readonly TrafficKind[]): { kind: TrafficKind['kind']; label: string; bytes: number; percent: number }[] {
  const total = kinds.reduce((n, k) => n + k.bytes, 0)
  if (total === 0) return []
  return kinds.map((k) => ({ kind: k.kind, label: TRAFFIC_KIND_LABELS[k.kind] ?? k.kind, bytes: k.bytes, percent: percentOf(k.bytes, total) }))
}

/** The bars of the per-second chart, scaled so the busiest second fills `height`, and which second that was (null when nothing moved). */
export function chartBars(perSecond: readonly number[], width: number, height: number): { bars: ChartBar[]; busiest: { second: number; bytes: number } | null } {
  if (perSecond.length === 0) return { bars: [], busiest: null }
  const max = Math.max(...perSecond)
  const slot = width / perSecond.length
  const bars = perSecond.map((bytes, second): ChartBar => {
    const h = bytes === 0 || max === 0 ? HAIRLINE : Math.max(HAIRLINE, Math.round((bytes / max) * height))
    return { x: second * slot, y: height - h, width: Math.max(HAIRLINE, slot - BAR_GAP), height: h, second, bytes }
  })
  if (max === 0) return { bars, busiest: null }
  const second = perSecond.indexOf(max)
  return { bars, busiest: { second, bytes: max } }
}

/** Says the figures are estimates when ordinary frames were sampled; empty when every frame was counted. */
export function sampleNote(t: HostTraffic): string {
  return t.sample > 1 ? TRAFFIC_SAMPLED(t.sample) : ''
}

/** Says why a window ended early or lost frames; empty for a window that ran its time and lost nothing. */
export function stoppedNote(t: HostTraffic): string {
  return [TRAFFIC_STOPPED[t.stopped_by] ?? '', t.kernel_dropped > 0 ? TRAFFIC_DROPPED(t.kernel_dropped) : ''].filter(Boolean).join(' ')
}

/** The share of the total each of two parts is, as whole percents that add to 100 (the second takes the rounding). */
export function splitPercent(first: number, second: number): [number, number] {
  const a = percentOf(first, first + second)
  return first + second === 0 ? [0, 0] : [a, PERCENT - a]
}


const MULTICAST_FIRST_OCTET = [224, 239] as const
const IPV4_BROADCAST = '255.255.255.255'
const IPV6_MULTICAST_PREFIX = 'ff'

/** True for an address many devices share (IPv4 multicast or broadcast, IPv6 multicast), which no one scan can mean. */
export function isSharedAddress(address: string): boolean {
  if (address === IPV4_BROADCAST || address.toLowerCase().startsWith(IPV6_MULTICAST_PREFIX)) return true
  const first = Number(address.split('.')[0])
  return first >= MULTICAST_FIRST_OCTET[0] && first <= MULTICAST_FIRST_OCTET[1]
}

/** Everything the look kept about one address: its name, size, share of the whole and its conversations; null when the look never saw it. */
export function trafficPlace(t: HostTraffic, address: string): TrafficPlace | null {
  const destination = t.destinations.find((d) => d.address === address)
  const flows = t.flows.filter((f) => f.address === address)
  if (!destination && flows.length === 0) return null
  const shared = Boolean(destination?.shared ?? flows[0]?.shared) || isSharedAddress(address)
  const internet = !shared && (destination?.internet ?? flows[0]!.internet)
  const bytes = destination?.bytes ?? flows.reduce((n, f) => n + f.bytes_in + f.bytes_out, 0)
  const total = t.bytes_in + t.bytes_out + t.bytes_through
  const named = destination?.name ? destination : flows.find((f) => f.name)
  return {
    address,
    name: named?.name,
    name_from: named?.name_from,
    owner: destination?.owner ?? flows.find((f) => f.owner)?.owner,
    internet,
    bytes,
    percent: percentOf(bytes, total),
    flows,
    scannable: !internet && !shared,
  }
}

/** The conversations of one kind, or all of them when no kind is picked. */
export function flowsOfKind(flows: readonly TrafficFlow[], kind: TrafficKind['kind'] | null): TrafficFlow[] {
  return kind ? flows.filter((f) => f.kind === kind) : [...flows]
}

/** The second a click at `x` of a chart `width` wide falls in, kept inside the seconds there are. */
export function secondAt(x: number, width: number, seconds: number): number {
  if (seconds <= 0 || width <= 0) return 0
  return Math.min(seconds - 1, Math.max(0, Math.floor((x / width) * seconds)))
}

/** A second as a clock, 0:07. */
export function secondClock(second: number): string {
  return `${Math.floor(second / SECONDS_PER_MINUTE)}:${String(second % SECONDS_PER_MINUTE).padStart(2, '0')}`
}

/** Where an address is used, smallest first, from the table of cities: "Toronto, Ontario, Canada". Empty without that table. */
export function usedIn(owner: TrafficOwner): string {
  if (!owner.city && !owner.region) return ''
  const region = owner.region && owner.region !== owner.city ? owner.region : ''
  return [owner.city, region, owner.place_country ? countryName(owner.place_country) : ''].filter(Boolean).join(', ')
}

/** Where an address is, as the screen says it: where it is used when the table of cities knows, else where it is registered. Never the two mixed. */
export function locationOf(owner: TrafficOwner): string {
  const used = usedIn(owner)
  if (used) return used
  return owner.country ? TRAFFIC_REGISTERED_IN(countryName(owner.country)) : ''
}

/** The holder of an address and where it is on one line, as text: "Cloudflare · Toronto, Ontario, Canada". Empty when no owner is known. */
export function ownerLine(owner: TrafficOwner | undefined): string {
  return owner ? TRAFFIC_OWNER_LINE(ownerName(owner), locationOf(owner)) : ''
}

/** The holder's own name, shortened to the part after "AS-NAME - " when the table gives both: "Cloudflare, Inc.". */
export function ownerName(owner: TrafficOwner): string {
  const name = owner.name || `AS${owner.asn}`
  const dash = name.indexOf(OWNER_NAME_SEPARATOR)
  return dash > 0 && dash + OWNER_NAME_SEPARATOR.length < name.length ? name.slice(dash + OWNER_NAME_SEPARATOR.length) : name
}

const regionNames = ((): Intl.DisplayNames | null => {
  try {
    return new Intl.DisplayNames(['en'], { type: 'region' })
  } catch {
    return null
  }
})()

/** A two-letter country code as its English name; the code itself when the browser does not know it. */
export function countryName(code: string): string {
  try {
    return regionNames?.of(code) ?? code
  } catch {
    return code
  }
}

/** The look's internet bytes by network holder, biggest first. */
export function ownerBars(t: HostTraffic): WhereBar[] {
  return (t.owners ?? []).map((o) => ({
    key: `AS${o.asn}`, label: ownerName(o), note: [`AS${o.asn}`, o.country ? countryName(o.country) : ''].filter(Boolean).join(' \u00b7 '),
    bytes: o.bytes, percent: percentOf(o.bytes, t.internet_bytes),
  }))
}

/** The look's internet bytes by country, biggest first. */
export function countryBars(t: HostTraffic): WhereBar[] {
  return (t.countries ?? []).map((c) => ({ key: c.country, label: countryName(c.country), note: c.country, bytes: c.bytes, percent: percentOf(c.bytes, t.internet_bytes) }))
}

/** The conversations to show first, and how many are left behind; all of them when `all` is set. */
export function visibleFlows(flows: readonly TrafficFlow[], all: boolean, limit: number): { shown: TrafficFlow[]; hidden: number } {
  return all || flows.length <= limit ? { shown: [...flows], hidden: 0 } : { shown: flows.slice(0, limit), hidden: flows.length - limit }
}

/** The group a place belongs to in "Who it talked to". */
export function talkGroupOf(address: string, internet: boolean, shared = false): TalkGroupId {
  // Shared first: a look kept before the server marked broadcasts called 255.255.255.255 an internet address.
  return shared || isSharedAddress(address) ? 'shared' : internet ? 'internet' : 'network'
}

/** What one conversation was, as a short phrase: "HTTPS", "DNS", "UDP 9999", or "SSH to this computer :22" for one served here. */
export function serviceOf(f: TrafficFlow): string {
  const named = f.kind !== 'tcp' && f.kind !== 'udp' ? TRAFFIC_KIND_SHORT[f.kind] : ''
  if (f.served) return TRAFFIC_SERVED(f.port, named)
  if (named) return named
  return f.port ? `${f.proto.toUpperCase()} ${f.port}` : f.proto.toUpperCase()
}

/** The bytes of this program's own conversations with each address, so a place's total can leave them out. */
function selfBytesByAddress(flows: readonly TrafficFlow[]): Map<string, number> {
  const out = new Map<string, number>()
  for (const f of flows) if (f.self) out.set(f.address, (out.get(f.address) ?? 0) + f.bytes_in + f.bytes_out)
  return out
}

/** A row for an address seen in a destination or a conversation, before its conversations are added. */
function newRow(place: { address: string; internet: boolean; shared?: boolean; name?: string; owner?: TrafficOwner }, deviceNames: Readonly<Record<string, string>>): TalkRow {
  const { address, internet, shared, name, owner } = place
  const label = name || deviceNames[address] || address
  return { address, label, named: label !== address, group: talkGroupOf(address, internet, shared), sent: 0, received: 0, bytes: 0, services: [], owner }
}

/**
 * "Who it talked to": one row per place, in three groups (internet, this network, broadcast chatter), biggest first. A row's traffic leaves out
 * this program's own page and API, which the summary sets apart; a place with nothing else is left out. The conversations add what each place
 * was (its services, by size) and which way the bytes went.
 */
export function talkGroups(t: HostTraffic, deviceNames: Readonly<Record<string, string>>): TalkGroup[] {
  const self = selfBytesByAddress(t.flows)
  const rows = new Map<string, TalkRow>()
  for (const d of t.destinations) {
    const row = rows.get(d.address) ?? newRow(d, deviceNames)
    row.bytes = Math.max(row.bytes, d.bytes - (self.get(d.address) ?? 0))
    rows.set(d.address, row)
  }
  const services = new Map<string, Map<string, number>>()
  const flowBytes = new Map<string, number>()
  for (const f of t.flows) {
    if (f.self) continue
    const row = rows.get(f.address) ?? newRow(f, deviceNames)
    row.sent += f.bytes_out
    row.received += f.bytes_in
    row.owner ??= f.owner
    if (!row.named && f.name) Object.assign(row, { label: f.name, named: true })
    rows.set(f.address, row)
    flowBytes.set(f.address, (flowBytes.get(f.address) ?? 0) + f.bytes_in + f.bytes_out)
    const byService = services.get(f.address) ?? new Map<string, number>()
    byService.set(serviceOf(f), (byService.get(serviceOf(f)) ?? 0) + f.bytes_in + f.bytes_out)
    services.set(f.address, byService)
  }
  for (const row of rows.values()) {
    row.bytes = Math.max(row.bytes, flowBytes.get(row.address) ?? 0)
    row.services = [...(services.get(row.address) ?? new Map<string, number>())].sort((a, b) => b[1] - a[1]).map(([name]) => name)
  }
  const kept = [...rows.values()].filter((r) => r.bytes > 0).sort((a, b) => b.bytes - a.bytes || a.address.localeCompare(b.address))
  return TALK_GROUP_ORDER.map((id) => {
    const groupRows = kept.filter((r) => r.group === id)
    return { id, rows: groupRows, bytes: groupRows.reduce((n, r) => n + r.bytes, 0) }
  })
}

const TALK_GROUP_ORDER: readonly TalkGroupId[] = ['internet', 'network', 'shared']

/** The points of a route in order, for the map's line: each answering router with a point, a city repeated in a row drawn once. */
export function routePoints(trace: RouteTrace | null): GeoPoint[] {
  const out: GeoPoint[] = []
  for (const hop of trace?.hops ?? []) {
    const p = hop.owner?.point
    if (!p) continue
    const last = out[out.length - 1]
    if (!last || last.lat !== p.lat || last.lon !== p.lon) out.push(p)
  }
  return out
}

/** The words of one step of a route: who answered, where, and how quickly. */
export function hopWords(hop: RouteHop): { who: string; where: string; time: string } {
  if (!hop.address) return { who: ROUTE_HOP_SILENT, where: '', time: '' }
  const where = hop.owner ? ownerLine(hop.owner) : hop.where ? ROUTE_HOP_WHERE[hop.where] : ''
  const time = hop.best_ms !== undefined ? ROUTE_HOP_TIME(msWords(hop.best_ms)) : ''
  return { who: hop.address, where, time }
}

/** A time in milliseconds as short digits: one decimal under ten, whole numbers above. */
export function msWords(ms: number): string {
  return ms < 10 ? ms.toFixed(1) : String(Math.round(ms))
}

/** The steps of a route with each run of two or more silent steps folded into one line; a single silent step stays as it is. */
export function foldRoute(hops: readonly RouteHop[]): RouteLine[] {
  const out: RouteLine[] = []
  let i = 0
  while (i < hops.length) {
    const hop = hops[i]!
    let end = i
    while (!hops[end]!.address && end + 1 < hops.length && !hops[end + 1]!.address) end++
    if (!hop.address && end > i) {
      out.push({ kind: 'silent', from: hops[i]!.ttl, to: hops[end]!.ttl, count: end - i + 1 })
      i = end + 1
    } else {
      out.push({ kind: 'hop', hop })
      i++
    }
  }
  return out
}

/** The one line over a route: how many steps, how many answered, how long the last answer took, and the countries it passes through. */
export function routeSummary(trace: RouteTrace): string {
  const answered = trace.hops.filter((h) => h.address)
  const last = answered[answered.length - 1]
  const line = ROUTE_SUMMARY(trace.hops.length, answered.length, last?.best_ms !== undefined ? msWords(last.best_ms) : null)
  const countries: string[] = []
  for (const h of answered) {
    const code = h.owner?.place_country || h.owner?.country
    const name = code ? countryName(code) : ''
    if (name && !countries.includes(name)) countries.push(name)
  }
  return countries.length > 1 ? `${line} ${ROUTE_PASSES_THROUGH(countries.join(', '))}` : line
}

/** What the repeated rounds measured at one step, as words: answered of sent, then the quickest, average and slowest time. */
export function statsWords(hop: RouteHop): { answered: string; best: string; average: string; worst: string } | null {
  const st = hop.stats
  if (!st || st.sent === 0) return null
  const ms = (v: number | undefined) => (v === undefined || st.answered === 0 ? '\u2013' : ROUTE_HOP_TIME(msWords(v)))
  return { answered: `${st.answered}/${st.sent}`, best: ms(st.best_ms), average: ms(st.avg_ms), worst: ms(st.worst_ms) }
}

/** What a finished trace says at its end: reached in N steps, not reached, or why it failed. Empty while it runs. */
export function routeOutcome(trace: RouteTrace): string {
  if (trace.state === 'running') return ''
  if (trace.state === 'failed') return ROUTE_FAILED[trace.error ?? 'probe_failed'] ?? ROUTE_FAILED.probe_failed
  return trace.reached ? ROUTE_REACHED(trace.hops.length) : ROUTE_NOT_REACHED
}
