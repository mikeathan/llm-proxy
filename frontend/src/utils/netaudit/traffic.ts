import { formatBytes } from '../format/formatters'
import {
  TRAFFIC_DROPPED, TRAFFIC_ENCRYPTED_KINDS, TRAFFIC_KIND_LABELS, TRAFFIC_SAMPLED, TRAFFIC_STOPPED, TRAFFIC_TOP_DESTINATIONS,
} from '../../constants/netaudit'
import type { ChartBar, HostTraffic, TrafficDestination, TrafficKind } from '../../types/netaudit'

const MS_PER_SECOND = 1000
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
