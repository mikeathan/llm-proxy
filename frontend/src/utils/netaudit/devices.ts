import { DEVICE_PRIVATE, DEVICE_UNNAMED } from '../../constants/netaudit'
import { formatRelativeTime } from '../format/time'
import type { NetworkDevice, PresenceState } from '../../types/netaudit'
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
