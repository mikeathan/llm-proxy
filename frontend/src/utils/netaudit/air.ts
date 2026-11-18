import { DEVICE_AIR_BAND, DEVICE_AIR_HEARD, DEVICE_AIR_KIND, DEVICE_AIR_MOVING, DEVICE_AIR_NOT_HEARD, DEVICE_AIR_PLACE, DEVICE_AIR_YOUR_WIFI } from '../../constants/netaudit'
import { RADIO_KIND_WORDS } from '../../constants/radioIdentity'
import { formatRelativeTime } from '../format/time'
import type { AirState, DeviceAir } from '../../types/netaudit'

// Network · a device row: what Radio heard of the device over the air (ADR-0061), as one phrase that follows "First seen". Pure; nothing is
// said while Radio is off, has not listened in the last day or could not be read, so a quiet air is never shown as "not there".

/** The phrase, or '' when there is nothing to say. Simple says when and where; Advanced adds the band, movement, your Wi-Fi and the kind. */
export function airPhrase(air: DeviceAir | undefined, state: AirState | undefined, advanced: boolean, nowMs: number = Date.now()): string {
  if (!air || state !== 'listening') return ''
  if (!air.heard) return DEVICE_AIR_NOT_HEARD
  const parts = [DEVICE_AIR_HEARD(formatRelativeTime(air.heard, nowMs))]
  if (air.ring) parts.push(DEVICE_AIR_PLACE[air.ring])
  if (advanced) parts.push(...advancedParts(air))
  return parts.join(', ')
}

function advancedParts(air: DeviceAir): string[] {
  const out: string[] = []
  if (air.band) out.push(DEVICE_AIR_BAND[air.band])
  if (air.moving) out.push(DEVICE_AIR_MOVING)
  if (air.on_your_wifi) out.push(DEVICE_AIR_YOUR_WIFI)
  if (air.kind && air.kind !== 'unknown') out.push(DEVICE_AIR_KIND(RADIO_KIND_WORDS[air.kind]))
  return out
}
