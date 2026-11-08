import { DEVICES_NONE_YET, DEVICE_ONLINE_WINDOW_MS, DEVICE_PRIVATE, DEVICE_UNNAMED, UDP_COVERAGE } from '../../constants/netaudit'
import { formatRelativeTime, isRecorded } from '../format/time'
import type { DeviceFilter, DeviceListGroup, DeviceStatus, NetworkDevice, PresenceState } from '../../types/netaudit'
import type { ChoiceOption } from '../../types/ui'

/** What a device is called on screen: the person's name, else the maker of its card, else what its address suggests. */
export function deviceLabel(device: NetworkDevice): string {
  if (device.name) return device.name
  if (device.vendor && !device.randomized) return `${device.vendor} device`
  return device.randomized ? DEVICE_PRIVATE : DEVICE_UNNAMED
}

/**
 * The address a device is known by: its IPv4 address, else its newest IPv6 one. An IPv6-only device with several addresses
 * says how many more there are in Simple and lists them all in Advanced.
 */
export function deviceAddress(device: NetworkDevice, advanced: boolean): string {
  if (device.ip) return device.ip
  const addresses = device.ipv6 ?? []
  if (!addresses.length) return 'no address'
  if (advanced) return addresses.join(', ')
  return `${addresses[0]}${addresses.length > 1 ? ` and ${addresses.length - 1} more` : ''}`
}

/** When a device was last on the network, as the line under its name. */
export const deviceSeen = (device: NetworkDevice, nowMs: number = Date.now()): string => `seen ${formatRelativeTime(device.last_seen, nowMs)}`

/**
 * The devices naming can do something for: no name, a real hardware address and an IPv4 address. A private-address phone
 * rotates and an IPv6-only device has no finding key, so the missing-device check cannot follow either.
 */
export function nameableDevices(devices: NetworkDevice[]): NetworkDevice[] {
  return devices.filter((d) => !d.name && !d.randomized && Boolean(d.ip))
}

/** What a scan found open on a device, as one line, or '' for a device no scan has finished looking at. */
export function portsLine(device: NetworkDevice, nowMs: number = Date.now()): string {
  if (!isRecorded(device.ports_scanned_at)) return ''
  const when = `scanned ${formatRelativeTime(device.ports_scanned_at!, nowMs)}`
  const ports = device.open_ports ?? []
  return ports.length ? `Open ports: ${ports.join(', ')} · ${when}` : `No open ports · ${when}`
}

/** The one line a folded list shows: how many devices, and how many need a decision. */
export function deviceSummary(devices: NetworkDevice[]): string {
  if (!devices.length) return DEVICES_NONE_YET
  const count = `${devices.length} ${devices.length === 1 ? 'device' : 'devices'}`
  const fresh = devices.filter((d) => d.new).length
  return fresh ? `${count} · ${fresh} new` : count
}

/** What a scan found open on a device, as a count (Simple), or '' for a device no scan has finished looking at. */
export function portsSummary(device: NetworkDevice, nowMs: number = Date.now()): string {
  if (!isRecorded(device.ports_scanned_at)) return ''
  const count = device.open_ports?.length ?? 0
  return `${count ? `${count} open ${count === 1 ? 'port' : 'ports'}` : 'No open ports'} · scanned ${formatRelativeTime(device.ports_scanned_at!, nowMs)}`
}

/**
 * How many devices have a kept UDP scan, and how old the oldest is, or '' when none has one. A device that was never scanned has no UDP
 * findings and is not clean, so once any device has a result the line says how many do.
 */
export function udpCoverageLine(devices: NetworkDevice[], nowMs: number = Date.now()): string {
  const addressed = devices.filter((d) => d.ip)
  const scanned = addressed.filter((d) => isRecorded(d.udp_scanned_at))
  if (!scanned.length) return ''
  const oldest = scanned.map((d) => Date.parse(d.udp_scanned_at!)).reduce((a, b) => Math.min(a, b))
  return UDP_COVERAGE(scanned.length, addressed.length, formatRelativeTime(new Date(oldest).toISOString(), nowMs))
}

/**
 * Where a device belongs in the list. One that nobody approved and that is still in the news (the server's `new`) needs a
 * look; the rest are told apart by whether they were on the network lately. "Lately" is a fixed window, because the
 * server stores when a device was last seen but not whether it is there now.
 */
export function deviceStatus(device: NetworkDevice, nowMs: number = Date.now()): DeviceStatus {
  if (device.new) return 'attention'
  return nowMs - Date.parse(device.last_seen) <= DEVICE_ONLINE_WINDOW_MS ? 'online' : 'offline'
}

const FILTER_TEST: Record<DeviceFilter, (device: NetworkDevice, nowMs: number) => boolean> = {
  all: () => true,
  attention: (d, now) => deviceStatus(d, now) === 'attention',
  online: (d, now) => deviceStatus(d, now) === 'online',
  offline: (d, now) => deviceStatus(d, now) === 'offline',
  unnamed: (d) => !d.name,
}

/** How many devices each filter chip would show, so a chip can say what is behind it. */
export function filterCounts(devices: NetworkDevice[], nowMs: number = Date.now()): Record<DeviceFilter, number> {
  const count = (filter: DeviceFilter) => devices.filter((d) => FILTER_TEST[filter](d, nowMs)).length
  return { all: count('all'), attention: count('attention'), online: count('online'), offline: count('offline'), unnamed: count('unnamed') }
}

/** True when the text is in the device's name, suggested name, hardware address or any address (case ignored). */
export function matchesQuery(device: NetworkDevice, query: string): boolean {
  const q = query.trim().toLowerCase()
  if (!q) return true
  return [device.name, device.suggested_name, device.vendor, device.mac, device.ip, ...(device.ipv6 ?? [])].some((field) => field?.toLowerCase().includes(q))
}

const ipKey = (device: NetworkDevice) => (device.ip ?? '').split('.').map((n) => n.padStart(3, '0')).join('.')
const byName = (a: NetworkDevice, b: NetworkDevice) => {
  if (Boolean(a.name) !== Boolean(b.name)) return a.name ? -1 : 1
  return (a.name ?? '').localeCompare(b.name ?? '') || ipKey(a).localeCompare(ipKey(b)) || a.mac.localeCompare(b.mac)
}

const GROUP_ORDER: DeviceStatus[] = ['attention', 'online', 'offline']

/**
 * The devices that pass the filter and the search, split into groups in the order Needs a look, Online, Offline. A group
 * with nothing in it is left out. Named devices come first inside a group, by name; the rest by address.
 */
export function groupDevices(devices: NetworkDevice[], filter: DeviceFilter, query: string, nowMs: number = Date.now()): DeviceListGroup[] {
  const shown = devices.filter((d) => FILTER_TEST[filter](d, nowMs) && matchesQuery(d, query))
  return GROUP_ORDER
    .map((status) => ({ status, devices: shown.filter((d) => deviceStatus(d, nowMs) === status).sort(byName) }))
    .filter((group) => group.devices.length > 0)
}

/** What the presence check says about itself: that it is off, what is wrong, or when it last looked. Trouble is never softened. */
export function presenceLine(state: PresenceState, nowMs: number = Date.now()): string {
  if (!state.enabled) return 'Off.'
  if (state.problem) return `Cannot look right now: ${state.problem}`
  if (!isRecorded(state.last_check)) return `On. The first check runs within ${state.interval_minutes} minutes.`
  return `On. Last checked ${formatRelativeTime(state.last_check, nowMs)}.`
}

/** The choices for how often the check runs, from what the server offers. */
export function intervalOptions(state: PresenceState): ChoiceOption[] {
  return state.allowed_intervals.map((minutes) => ({ value: String(minutes), label: `Every ${minutes} minutes` }))
}
