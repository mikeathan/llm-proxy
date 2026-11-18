import { radioHoursLine } from '../../constants/radio'
import { whereOf } from './where'
import type { MyDeviceRow, RadioHourState, RadioLiveDevice, RadioOwnerDevice, RadioPresenceDevice } from '../../types/radio'

// Radio · My devices: one row per device of yours, from the three reads the tab makes (your devices, where they were hour by hour, and the live
// picture while listening). Pure functions over what the server sent; no state.

/** The live picture's ids are the first eight hex digits of the full id the device list uses. */
const SHORT_ID = 8
const DAY_HOURS = 24
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
      heard24: last.filter((h) => h.state === 'here').length, listened24: last.filter((h) => h.state !== 'not_listening').length,
      kind: l?.ident?.kind ?? 'unknown', where: l ? whereOf(l) : '', moving: !!l?.moving, presence: p,
    }
  })
  return rows.sort((a, b) => STATE_ORDER[a.state] - STATE_ORDER[b.state] || a.label.localeCompare(b.label))
}

/** One sentence for the last day, instead of a chart. */
export const hoursLine = (r: MyDeviceRow): string => radioHoursLine(r.heard24, r.listened24, DAY_HOURS)

/** Each name more than one device goes by, most shared first: the page asks you to rename them. */
export function duplicateNames(rows: MyDeviceRow[]): { label: string; count: number }[] {
  const n = new Map<string, number>()
  for (const r of rows) n.set(r.label, (n.get(r.label) ?? 0) + 1)
  return [...n].filter(([, c]) => c > 1).map(([label, count]) => ({ label, count })).sort((a, b) => b.count - a.count || a.label.localeCompare(b.label))
}
