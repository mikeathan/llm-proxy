import type { RadioAroundFilter, RadioChange, RadioChannelCount, RadioLiveDevice } from "../../types/radio"
import { isRouter, ownershipOf } from "./identity"

// Radio · Around you: the pure rules behind the filter chips, the three views and the "find my device" shortlist. No state, no timers.

const FIND_MAX = 3

/** Whether a device belongs to a chip: everyone, yours, likely yours (the evidence, not your word), routers, new today, those unlike their usual,
 * or one kind of someone else's. `today` (YYYY-MM-DD) is what "new" is measured against. */
export function matchesFilter(d: RadioLiveDevice, filter: RadioAroundFilter, today = ""): boolean {
  switch (filter) {
    case "all": return true
    case "mine": return d.mine
    case "likely": return ownershipOf(d) === "likely"
    case "routers": return isRouter(d)
    case "new": return !!today && d.detail?.first_seen === today
    case "unusual": return !!d.unusual
  }
  return !d.mine && d.class === filter
}

export const countFor = (devices: RadioLiveDevice[], filter: RadioAroundFilter, today = ""): number => devices.filter((d) => matchesFilter(d, filter, today)).length

/** Closest first, the owner's own before everyone else's. */
export function byCloseness(list: RadioLiveDevice[]): RadioLiveDevice[] {
  return [...list].sort((a, b) => Number(b.mine) - Number(a.mine) || a.near_m * a.far_m - b.near_m * b.far_m)
}

/** The devices the radar and the list show: the chip, the channel, and whether the ones not heard now are drawn. */
export function narrow(devices: RadioLiveDevice[], o: { filter: RadioAroundFilter; channel: number | null; showUsual: boolean; hideBrief?: boolean; today?: string }): RadioLiveDevice[] {
  return devices.filter((d) => matchesFilter(d, o.filter, o.today) && (o.channel === null || d.channel === o.channel) && (o.showUsual || !d.usual) && !(o.hideBrief && !d.mine && d.class === "transient"))
}

/** Arrivals first heard today, devices unlike their usual, and devices usually here that are not heard now. A day is the finest the stored history knows. */
export function changesOf(devices: RadioLiveDevice[], today: string): RadioChange[] {
  const out: RadioChange[] = []
  for (const d of devices) {
    if (d.mine && !d.unusual) continue
    if (d.unusual && !d.usual) out.push({ kind: "unusual", device: d })
    else if (d.usual) out.push({ kind: "departure", device: d })
    else if (d.detail?.first_seen === today) out.push({ kind: "arrival", device: d })
  }
  const order = { unusual: 0, arrival: 1, departure: 2 } as const
  return out.sort((a, b) => order[a.kind] - order[b.kind] || a.device.near_m - b.device.near_m)
}

/** How many devices heard now are on each channel, in channel order. */
export function channelCounts(devices: RadioLiveDevice[]): RadioChannelCount[] {
  const n = new Map<number, number>()
  for (const d of devices) if (!d.usual && d.channel) n.set(d.channel, (n.get(d.channel) ?? 0) + 1)
  return [...n.entries()].map(([channel, count]) => ({ channel, devices: count })).sort((a, b) => a.channel - b.channel)
}

/** The strongest devices heard now that are not already the owner's: the shortlist when the owner holds a device next to the computer. */
export function strongestNow(devices: RadioLiveDevice[]): RadioLiveDevice[] {
  return devices.filter((d) => !d.mine && !d.usual && d.heard).sort((a, b) => b.dbm - a.dbm).slice(0, FIND_MAX)
}
