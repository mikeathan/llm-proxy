import { RADIO_RING_WORDS, RADIO_WHERE_FAR_HOST, RADIO_WHERE_NEAR_HOST, radioWhereFar, radioWhereHouse, radioWhereJoined, radioWhereNearBox } from '../../constants/radio'
import type { RadioLiveDevice } from '../../types/radio'

// Radio · where in the house: one phrase from the two listeners there are, this computer's radar and your mesh router's boxes. A listener that
// hears a device strongly places it near that listener; otherwise only the box it is joined to is said, never a spot between them (one radar
// and a few boxes cannot draw that, docs/audits/2026-11-18-radio-continuous-monitoring.md §4a). No layout of the house is assumed: the boxes
// go by your names for them on the Network page.

/** A box's signal at or above this is "near it": the same line the sorting calls a strong signal (StrongDBm, stacks/radio/classes.go). */
const NEAR_BOX_DBM = -60

export function whereOf(d: Pick<RadioLiveDevice, 'ring' | 'heard' | 'router'>): string {
  const r = d.router
  if (d.heard && d.ring === 'room') return RADIO_WHERE_NEAR_HOST
  if (r && r.dbm >= NEAR_BOX_DBM) return radioWhereNearBox(r)
  if (r && !d.heard) return radioWhereJoined(r)
  if (r) return d.ring === 'house' ? radioWhereHouse(r) : radioWhereFar(r)
  if (!d.heard || d.ring === 'far') return ''
  // The radar's outer ring is "next door or the street" for a stranger, but your own devices two floors down sit on it too: say only "far".
  return d.ring === 'nearby' ? RADIO_WHERE_FAR_HOST : RADIO_RING_WORDS[d.ring]
}
