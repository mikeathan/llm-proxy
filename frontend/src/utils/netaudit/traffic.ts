import { formatBytes } from '../format/formatters'
import {
  TRAFFIC_DROPPED, TRAFFIC_ENCRYPTED_KINDS, TRAFFIC_OWNER_LINE, TRAFFIC_KIND_LABELS, TRAFFIC_SAMPLED, TRAFFIC_STOPPED, TRAFFIC_TOP_DESTINATIONS,
} from '../../constants/netaudit'
import type { ChartBar, HostTraffic, TrafficDestination, TrafficFlow, TrafficKind, TrafficOwner, TrafficPlace, WhereBar } from '../../types/netaudit'

const MS_PER_SECOND = 1000
const SECONDS_PER_MINUTE = 60
const PERCENT = 100
/** The least height a bar has, so a quiet second is still a mark on the chart. */
const HAIRLINE = 1
/** The space between two bars, in the chart's own units. */
const BAR_GAP = 2

const plural = (n: number, one: string, many: string): string => (n === 1 ? one : many)
const percentOf = (part: number, whole: number): number => (whole > 0 ? Math.round((part / whole) * PERCENT) : 0)

/** The one-sentence summary of a look, and the details after it. `detail` is empty when there is nothing more to say. */
export function trafficSentence(t: HostTraffic): { headline: string; detail: string } {
  const seconds = Math.max(1, Math.round(t.elapsed_ms / MS_PER_SECOND))
  const when = `In ${seconds} ${plural(seconds, 'second', 'seconds')}`
  const total = t.bytes_in + t.bytes_out + t.bytes_through
  const headline = t.bytes_in + t.bytes_out === 0
    ? `${when} nothing was sent or received by this computer.`
    : `${when} this computer sent ${formatBytes(t.bytes_out)} and received ${formatBytes(t.bytes_in)}.`
  const parts: string[] = []
  const top = t.destinations.slice(0, TRAFFIC_TOP_DESTINATIONS)
  if (top.length === TRAFFIC_TOP_DESTINATIONS) {
    parts.push(`${TRAFFIC_TOP_DESTINATIONS} destinations took ${percentOf(top.reduce((n, d) => n + d.bytes, 0), total)}% of it.`)
  }
  const encrypted = t.kinds.filter((k) => TRAFFIC_ENCRYPTED_KINDS.includes(k.kind)).reduce((n, k) => n + k.bytes, 0)
  const kindTotal = t.kinds.reduce((n, k) => n + k.bytes, 0)
  if (encrypted > 0) parts.push(`${percentOf(encrypted, kindTotal)}% was encrypted web traffic (HTTPS or QUIC).`)
  if (t.lookups > 0) parts.push(`${t.lookups} ${plural(t.lookups, 'name was', 'names were')} looked up.`)
  return { headline, detail: parts.join(' ') }
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

/** A destination as the screen names it: the name a DNS answer gave, else the address. */
export function destinationLabel(d: TrafficDestination): { text: string; named: boolean } {
  return d.name ? { text: d.name, named: true } : { text: d.address, named: false }
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
  const internet = destination?.internet ?? flows[0]!.internet
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
    scannable: !internet && !isSharedAddress(address),
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

/** The holder of an address and its country on one line, as text: "GOOGLE - Google LLC · United States". Empty when no owner is known. */
export function ownerLine(owner: TrafficOwner | undefined): string {
  return owner ? TRAFFIC_OWNER_LINE(owner.name || `AS${owner.asn}`, owner.country ? countryName(owner.country) : '') : ''
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
    key: `AS${o.asn}`, label: o.name || `AS${o.asn}`, note: [`AS${o.asn}`, o.country ? countryName(o.country) : ''].filter(Boolean).join(' \u00b7 '),
    bytes: o.bytes, percent: percentOf(o.bytes, t.internet_bytes),
  }))
}

/** The look's internet bytes by country, biggest first. */
export function countryBars(t: HostTraffic): WhereBar[] {
  return (t.countries ?? []).map((c) => ({ key: c.country, label: countryName(c.country), note: c.country, bytes: c.bytes, percent: percentOf(c.bytes, t.internet_bytes) }))
}
