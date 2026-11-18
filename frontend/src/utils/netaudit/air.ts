import { DEVICE_AIR_BAND, DEVICE_AIR_HEARD, DEVICE_AIR_HEARD_LAST, DEVICE_AIR_KIND, DEVICE_AIR_MOVING, DEVICE_AIR_NOT_HEARD, DEVICE_AIR_PLACE, DEVICE_AIR_YOUR_WIFI } from '../../constants/netaudit'
import { RADIO_KIND_WORDS } from '../../constants/radioIdentity'
import { formatRelativeTime } from '../format/time'
import type { AirState, AirWords, DeviceAir } from '../../types/netaudit'

// Network · a device row: what Radio heard of the device over the air (ADR-0061), as one phrase that follows "First seen". Pure; nothing is
// said while Radio is off, has not listened in the last day or could not be read, so a quiet air is never shown as "not there".

/** The phrase, or null when there is nothing to say. Simple says when and where; Advanced adds the band, movement, your Wi-Fi and the kind.
 * A device heard in Radio's latest cycle says so instead of a time: Radio listens in windows, so "3 minutes ago" there is as recent as it gets
 * and would read as if the device had left. */
export function airWords(air: DeviceAir | undefined, state: AirState | undefined, advanced: boolean, lastCycle?: string, nowMs: number = Date.now()): AirWords | null {
  if (!air || state !== 'listening') return null
  if (!air.heard) return { heard: DEVICE_AIR_NOT_HEARD, rest: '' }
  const parts: string[] = []
  if (air.ring) parts.push(DEVICE_AIR_PLACE[air.ring])
  if (advanced) parts.push(...advancedParts(air))
  return { heard: inLastCycle(air.heard, lastCycle) ? DEVICE_AIR_HEARD_LAST : DEVICE_AIR_HEARD(formatRelativeTime(air.heard, nowMs)), rest: parts.join(', ') }
}

/** Radio stamps a device with the end of the cycle that heard it, so the latest cycle's devices carry exactly its time. */
function inLastCycle(heard: string, lastCycle: string | undefined): boolean {
  if (!lastCycle) return false
  const h = Date.parse(heard)
  const c = Date.parse(lastCycle)
  return Number.isFinite(h) && Number.isFinite(c) && h >= c
}

function advancedParts(air: DeviceAir): string[] {
  const out: string[] = []
  if (air.band) out.push(DEVICE_AIR_BAND[air.band])
  if (air.moving) out.push(DEVICE_AIR_MOVING)
  if (air.on_your_wifi) out.push(DEVICE_AIR_YOUR_WIFI)
  if (air.kind && air.kind !== 'unknown') out.push(DEVICE_AIR_KIND(RADIO_KIND_WORDS[air.kind]))
  return out
}
