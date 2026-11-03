import {
  NEARBY_FAILED, NEARBY_KIND_LABELS, NEARBY_NEVER, NEARBY_NOTHING, NEARBY_RUNNING, NEARBY_SERVICE_NAMES, NEARBY_SILENCE_NOTE, NEARBY_STATUS_LABELS,
} from '../../constants/netaudit'
import { formatRelativeTime } from '../format/time'
import type { NearbyDevice, NearbyFilter, NearbyKind, NearbyState, NearbyStatus } from '../../types/netaudit'

// What a device says about itself, turned into words for the Nearby card. Everything here is a claim made by the device, so
// every function returns plain text for the screen to show as text; nothing builds markup.

const MS_PER_SECOND = 1000
/** A device counts as heard in the latest look when its last answer is this close to the look's end. */
const SAME_LOOK_SECONDS = 90

/** Text a device sent, without control characters and the invisible ones that reverse the direction of text. The server cleans it too; this keeps a stored or forged value from flipping a row. */
export const plain = (s: string | undefined): string => (s ?? '').replace(/[\p{Cc}\p{Cf}]/gu, '').trim()

const has = (d: NearbyDevice, ...types: string[]) => types.some((t) => d.services?.includes(t))

/** What the device advertises as, from its service types and UPnP device type. */
export function nearbyKind(d: NearbyDevice): NearbyKind {
  if (d.gateway) return 'gateway'
  if (has(d, '_ipp._tcp', '_ipps._tcp', '_printer._tcp', '_pdl-datastream._tcp')) return 'printer'
  if (has(d, '_rtsp._tcp')) return 'camera'
  if (has(d, '_airplay._tcp', '_googlecast._tcp', '_raop._tcp', '_spotify-connect._tcp') || d.upnp_type?.startsWith('MediaRenderer')) return 'tv'
  if (has(d, '_ssh._tcp', '_smb._tcp', '_afpovertcp._tcp', '_workstation._tcp')) return 'computer'
  return 'other'
}

export const nearbyKindLabel = (d: NearbyDevice) => NEARBY_KIND_LABELS[nearbyKind(d)]

/** The best name: the one the person gave, then what the device calls itself, then its maker and model, then its address. */
export function nearbyName(d: NearbyDevice): string {
  return plain(d.name) || plain(d.names?.[0]) || [plain(d.maker), plain(d.model)].filter(Boolean).join(' ') || d.ip
}

/** A service type in words: a known one by name, any other as its bare name ("_foo._tcp" becomes "foo"). */
export function serviceLabel(raw: string): string {
  return NEARBY_SERVICE_NAMES[raw] ?? raw.replace(/^_/, '').replace(/\._(tcp|udp)$/, '')
}

export function nearbyStatus(d: NearbyDevice, lastLook: string | undefined): NearbyStatus {
  if (d.problem) return 'attention'
  if (d.new) return 'new'
  if (!lastLook) return 'quiet'
  const gap = Math.abs(new Date(lastLook).getTime() - new Date(d.last_heard).getTime()) / MS_PER_SECOND
  return gap <= SAME_LOOK_SECONDS ? 'heard' : 'quiet'
}

export const nearbyStatusLabel = (s: NearbyStatus) => NEARBY_STATUS_LABELS[s]

export function nearbyCounts(state: NearbyState): Record<NearbyFilter, number> {
  const counts: Record<NearbyFilter, number> = { all: state.devices.length, attention: 0, new: 0, quiet: 0 }
  for (const d of state.devices) {
    const s = nearbyStatus(d, state.last_look)
    if (s === 'attention' || s === 'new' || s === 'quiet') counts[s]++
  }
  return counts
}

export function filterNearby(state: NearbyState, filter: NearbyFilter): NearbyDevice[] {
  return filter === 'all' ? state.devices : state.devices.filter((d) => nearbyStatus(d, state.last_look) === filter)
}

/** Devices in the order they are listed: a problem first, then new, then heard, then not heard; the server sorted within each. */
export function groupNearby(devices: NearbyDevice[], lastLook: string | undefined): { status: NearbyStatus; devices: NearbyDevice[] }[] {
  const order: NearbyStatus[] = ['attention', 'new', 'heard', 'quiet']
  return order
    .map((status) => ({ status, devices: devices.filter((d) => nearbyStatus(d, lastLook) === status) }))
    .filter((g) => g.devices.length > 0)
}

const plural = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`

/** The line above the list: how many were heard and how many need a look, and when. Silence is never described as "none there". */
export function nearbySummary(state: NearbyState): string {
  if (state.running) return NEARBY_RUNNING
  if (!state.last_look) return NEARBY_NEVER
  const failed = state.last_outcome ? NEARBY_FAILED[state.last_outcome] : undefined
  if (failed) return failed
  if (state.last_outcome === 'nothing_heard' && state.devices.length === 0) return NEARBY_NOTHING
  const needs = nearbyCounts(state).attention
  return `${plural(state.devices.length, 'device', 'devices')} heard, ${needs} ${needs === 1 ? 'needs' : 'need'} a look. Last checked ${formatRelativeTime(state.last_look)}.`
}

/** The note under the summary: the reminder that a quiet device is not a missing one, only once there is something to be quiet. */
export const nearbyNote = (state: NearbyState) => (state.devices.length > 0 && !state.running ? NEARBY_SILENCE_NOTE : '')

/** What to bring back for review: counts and the raw service facts, no names. */
export function nearbyReport(state: NearbyState): object {
  return {
    last_look: state.last_look ?? '',
    last_outcome: state.last_outcome ?? '',
    devices: state.devices.map((d) => ({
      ip: d.ip,
      kind: nearbyKind(d),
      services: d.services ?? [],
      ports: d.ports ?? [],
      upnp_type: d.upnp_type ?? '',
      sources: d.sources ?? [],
      known: d.known,
      last_heard: d.last_heard,
    })),
  }
}
