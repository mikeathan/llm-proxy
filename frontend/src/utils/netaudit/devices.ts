import { DEVICES_NONE_YET, DEVICE_PRIVATE, DEVICE_UNNAMED, FAMILIES, FAMILY_LABELS } from '../../constants/netaudit'
import { formatRelativeTime } from '../format/time'
import type { DeviceFamily, NetworkDevice, PresenceState } from '../../types/netaudit'
import type { ChoiceOption } from '../../types/ui'

/** What a device is called on screen: the person's name, else what its address suggests. */
export function deviceLabel(device: NetworkDevice): string {
  if (device.name) return device.name
  return device.randomized ? DEVICE_PRIVATE : DEVICE_UNNAMED
}

/** The line under the name: where it is and when it was last there. */
export function deviceDetail(device: NetworkDevice, nowMs: number = Date.now()): string {
  const address = device.ip ?? device.ipv6?.[0] ?? 'no address'
  return `${address} · seen ${formatRelativeTime(device.last_seen, nowMs)}`
}

/**
 * The devices naming can do something for: no name, a real hardware address and an IPv4 address. A private-address phone
 * rotates and an IPv6-only device has no finding key, so the missing-device check cannot follow either.
 */
export function nameableDevices(devices: NetworkDevice[]): NetworkDevice[] {
  return devices.filter((d) => !d.name && !d.randomized && Boolean(d.ip))
}

/** What a scan found open on a device, as one line, or '' for a device no scan has finished looking at. */
export function portsLine(device: NetworkDevice, nowMs: number = Date.now()): string {
  if (!device.ports_scanned_at) return ''
  const when = `scanned ${formatRelativeTime(device.ports_scanned_at, nowMs)}`
  const ports = device.open_ports ?? []
  return ports.length ? `Open ports: ${ports.join(', ')} · ${when}` : `No open ports · ${when}`
}

/** The devices that belong on one tab: those with an address of that family. A dual-stack device is on both. */
export function devicesIn(devices: NetworkDevice[], family: DeviceFamily): NetworkDevice[] {
  return devices.filter((d) => (family === 'ipv4' ? Boolean(d.ip) : Boolean(d.ipv6?.length)))
}

/** The one line a folded list shows: how many devices, and how many need a decision. */
export function deviceSummary(devices: NetworkDevice[]): string {
  if (!devices.length) return DEVICES_NONE_YET
  const count = `${devices.length} ${devices.length === 1 ? 'device' : 'devices'}`
  const fresh = devices.filter((d) => d.new).length
  return fresh ? `${count} · ${fresh} new` : count
}

/** The tabs, each with how many devices it holds. */
export function familyOptions(devices: NetworkDevice[]): ChoiceOption[] {
  return FAMILIES.map((family) => ({ value: family, label: `${FAMILY_LABELS[family]} (${devicesIn(devices, family).length})` }))
}

/** The line under a device on the IPv6 tab: its newest address (all of them in Advanced) and when it was last seen. */
export function ipv6Detail(device: NetworkDevice, advanced: boolean, nowMs: number = Date.now()): string {
  const addresses = device.ipv6 ?? []
  const shown = advanced ? addresses.join(', ') : `${addresses[0] ?? 'no address'}${addresses.length > 1 ? ` and ${addresses.length - 1} more` : ''}`
  return `${shown} · seen ${formatRelativeTime(device.last_seen, nowMs)}`
}

/** What the presence check says about itself: that it is off, what is wrong, or when it last looked. Trouble is never softened. */
export function presenceLine(state: PresenceState, nowMs: number = Date.now()): string {
  if (!state.enabled) return 'Off.'
  if (state.problem) return `Cannot look right now: ${state.problem}`
  if (!state.last_check) return `On. The first check runs within ${state.interval_minutes} minutes.`
  return `On. Last checked ${formatRelativeTime(state.last_check, nowMs)}.`
}

/** The choices for how often the check runs, from what the server offers. */
export function intervalOptions(state: PresenceState): ChoiceOption[] {
  return state.allowed_intervals.map((minutes) => ({ value: String(minutes), label: `Every ${minutes} minutes` }))
}
