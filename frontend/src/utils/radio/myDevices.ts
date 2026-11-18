import { radioHeardWhen, radioHoursLine, radioStretchSince, radioStretchSpan } from '../../constants/radio'
import { formatTime } from '../format/time'
import { whereOf } from './where'
import type { MyDeviceRow, RadioHeardStretch, RadioHourState, RadioLiveDevice, RadioOwnerDevice, RadioPresenceDevice } from '../../types/radio'

// Radio · My devices: one row per device of yours, from the three reads the tab makes (your devices, where they were hour by hour, and the live
// picture while listening). Pure functions over what the server sent; no state.

/** The live picture's ids are the first eight hex digits of the full id the device list uses. */
const SHORT_ID = 8
const DAY_HOURS = 24
/** How many stretches the sentence names, newest first kept; the rest are counted. */
const STRETCHES_SAID = 3
const STATE_ORDER: Record<RadioHourState, number> = { here: 0, away: 1, not_listening: 2 }

/** The rows, here now first, then away, then not known; by name within each. */
export function myDeviceRows(devices: RadioOwnerDevice[], presence: RadioPresenceDevice[], live: RadioLiveDevice[]): MyDeviceRow[] {
  const byID = new Map(presence.map((p) => [p.id, p]))
  const liveBy = new Map(live.map((d) => [d.id, d]))
  const rows = devices.map((d): MyDeviceRow => {
    const p = byID.get(d.id)
    const short = d.id.slice(0, SHORT_ID)
    const l = liveBy.get(short)
    const last = (p?.hours ?? []).slice(-DAY_HOURS)
    return {
      id: d.id, short, label: d.label, cls: d.class, classBy: d.class_by,
      state: p?.state ?? 'not_listening', lastHeard: p?.last_heard, signal: p?.signal,
      heard24: last.filter((h) => h.state === 'here').length, listened24: last.filter((h) => h.state !== 'not_listening').length, stretches: heardStretches(last),
      kind: l?.ident?.kind ?? 'unknown', where: l ? whereOf(l) : '', moving: !!l?.moving,
    }
  })
  return rows.sort((a, b) => STATE_ORDER[a.state] - STATE_ORDER[b.state] || a.label.localeCompare(b.label))
}

/** The hours heard in a row, joined: each stretch from the start of its first hour to the end of its last, and no end when it runs into the
 * newest hour (the current one). */
export function heardStretches(hours: RadioPresenceDevice['hours']): RadioHeardStretch[] {
  const out: RadioHeardStretch[] = []
  let from = ''
  hours.forEach((h, i) => {
    if (h.state === 'here' && !from) from = h.hour
    const ends = from && (h.state !== 'here' || i === hours.length - 1)
    if (!ends) return
    if (h.state === 'here') out.push({ from })
    else out.push({ from, to: h.hour })
    from = ''
  })
  return out
}

const stretchWords = (s: RadioHeardStretch): string => (s.to ? radioStretchSpan(formatTime(s.from), formatTime(s.to)) : radioStretchSince(formatTime(s.from)))

/** One sentence for the last day, instead of a chart: how many hours it was heard in, then when, as clock times. */
export const hoursLine = (r: MyDeviceRow): string => {
  const count = radioHoursLine(r.heard24, r.listened24, DAY_HOURS)
  if (!r.stretches.length) return count
  const said = r.stretches.slice(-STRETCHES_SAID)
  return radioHeardWhen(count, said.map(stretchWords), r.stretches.length - said.length)
}

/** Each name more than one device goes by, most shared first: the page asks you to rename them. */
export function duplicateNames(rows: MyDeviceRow[]): { label: string; count: number }[] {
  const n = new Map<string, number>()
  for (const r of rows) n.set(r.label, (n.get(r.label) ?? 0) + 1)
  return [...n].filter(([, c]) => c > 1).map(([label, count]) => ({ label, count })).sort((a, b) => b.count - a.count || a.label.localeCompare(b.label))
}
