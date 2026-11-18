import { RADIO_KIND_ORDER } from "../../constants/radioIdentity"
import type { RadioAroundFilter, RadioChange, RadioChannelCount, RadioFindLead, RadioKindCount, RadioKindFilter, RadioLiveDevice } from "../../types/radio"
import { isRouter, ownershipOf } from "./identity"

// Radio · Around you: the pure rules behind the filter chips, the three views and the "find my device" shortlist. No state, no timers.

const FIND_MAX = 3
/** How many dB the strongest must lead the next by to be clearly the one held next to the computer: more than the signal's own wobble. */
const FIND_CLEAR_DB = 10

/** Whether a device belongs to a chip: everyone, yours, likely yours (the evidence, not your word), new today, those unlike their usual,
 * or one kind of someone else's. `today` (YYYY-MM-DD) is what "new" is measured against. */
export function matchesFilter(d: RadioLiveDevice, filter: RadioAroundFilter, today = ""): boolean {
  switch (filter) {
    case "all": return true
    case "mine": return d.mine
    case "likely": return ownershipOf(d) === "likely"
    case "new": return !!today && d.detail?.first_seen === today
    case "unusual": return !!d.unusual
  }
  return !d.mine && d.class === filter
}

/** Whether a device is of a kind (the second, independent filter): a router is anything that says it is one or serves devices, as on the
 * device page, and every other kind leaves routers out, so a device is counted under one kind only. "unknown" is a device with no kind yet. */
export function matchesKind(d: RadioLiveDevice, kind: RadioKindFilter): boolean {
  if (kind === "any") return true
  if (kind === "router") return isRouter(d)
  return !isRouter(d) && (d.ident?.kind ?? "unknown") === kind
}

/** How many devices are of each kind, in the kinds' order, leaving out the kinds with none. */
export const kindCounts = (devices: RadioLiveDevice[]): RadioKindCount[] =>
  RADIO_KIND_ORDER.map((kind) => ({ kind, count: devices.filter((d) => matchesKind(d, kind)).length })).filter((k) => k.count > 0)

export const countFor = (devices: RadioLiveDevice[], filter: RadioAroundFilter, today = ""): number => devices.filter((d) => matchesFilter(d, filter, today)).length

/** Closest first, the owner's own before everyone else's. */
export function byCloseness(list: RadioLiveDevice[]): RadioLiveDevice[] {
  return [...list].sort((a, b) => Number(b.mine) - Number(a.mine) || a.near_m * a.far_m - b.near_m * b.far_m)
}

/** The devices the radar and the list show: the chip, the kind, the channel, and whether the ones not heard now are drawn. */
export function narrow(devices: RadioLiveDevice[], o: { filter: RadioAroundFilter; kind?: RadioKindFilter; channel: number | null; showUsual: boolean; hideBrief?: boolean; today?: string }): RadioLiveDevice[] {
  return devices.filter((d) => matchesFilter(d, o.filter, o.today) && matchesKind(d, o.kind ?? "any") && (o.channel === null || d.channel === o.channel) && (o.showUsual || !d.usual) && !(o.hideBrief && !d.mine && d.class === "transient"))
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

/** The strongest devices heard now that are not already the owner's: the shortlist when the owner holds a device next to the computer. This
 * computer's own Wi-Fi card is left out: it is always the strongest, and never the device in the owner's hand. */
export function strongestNow(devices: RadioLiveDevice[]): RadioLiveDevice[] {
  return devices.filter((d) => !d.mine && !d.usual && d.heard && !d.ident?.this_computer).sort((a, b) => b.dbm - a.dbm).slice(0, FIND_MAX)
}

/** How far the strongest of the shortlist leads the next, in dB (null with one alone), and whether that is clear; null with none. */
export function findLead(candidates: RadioLiveDevice[]): RadioFindLead | null {
  const [first, second] = candidates
  if (!first) return null
  if (!second) return { db: null, clear: true }
  const db = first.dbm - second.dbm
  return { db, clear: db >= FIND_CLEAR_DB }
}
