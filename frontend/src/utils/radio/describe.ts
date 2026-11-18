import {
  RADIO_ACCESS_POINT, RADIO_CLASS_WORDS, RADIO_MAKER_RANDOM, radioChannelBand, radioDotLabel, radioHeardAgo, radioShiftWord, radioSortsIn, radioWifiWord,
} from "../../constants/radio"
import { RADIO_KIND_WORDS, radioBroadcasts } from "../../constants/radioIdentity"
import { brandModel, nameOf, shortBrand } from "./identity"
import type { RadioDeviceTag, RadioLiveDevice } from "../../types/radio"

// Radio · what a device row says about a device, from what its frames revealed. The title is the owner's name, its name on the owner's network,
// its make and model, its network, its maker and kind (utils/radio/identity); the short id stays as a small handle. Nothing here is an address, and a
// network name only appears while the device is heard.

/** The name a device goes by; never its short id. */
export function deviceTitle(d: RadioLiveDevice): string {
  return nameOf(d)
}

/** The short name drawn beside a dot. */
export function deviceLabel(d: RadioLiveDevice): string {
  return d.mine ? radioDotLabel(d) : nameOf(d)
}

/** The line under the title: what it is, its Wi-Fi generation, and the channel it was heard on. */
export function deviceFacts(d: RadioLiveDevice): string[] {
  const out: string[] = []
  const kind = d.ident?.kind
  if (kind && kind !== "unknown") out.push(RADIO_KIND_WORDS[kind])
  else if (d.detail?.wifi_generation) out.push(RADIO_ACCESS_POINT)
  else if (!d.mine && !d.ident) out.push(RADIO_CLASS_WORDS[d.class])
  if (d.detail?.wifi_generation) out.push(radioWifiWord(d.detail.wifi_generation))
  if (d.network && nameOf(d) !== d.network) out.push(radioBroadcasts(d.network))
  if (d.channel) out.push(radioChannelBand(d.channel))
  return out
}

/** Who made it, shown beside a title that is not the maker already (a network name, a name on your network); empty otherwise. */
export function deviceMakerNote(d: RadioLiveDevice): string {
  const brand = d.ident?.brand ?? d.detail?.maker ?? ""
  const title = nameOf(d)
  if (!brand || title.toLowerCase().includes(shortBrand(brand).toLowerCase()) || brandModel(d.ident) === title) return ""
  return shortBrand(brand)
}

/** Why a neighbour's row has no maker, when the reason is worth saying: a private address hides it on purpose. Empty otherwise (an unlisted
 * maker says nothing the title does not already say). */
export function deviceMakerGap(d: RadioLiveDevice): string {
  if (d.mine || d.ident?.brand || d.detail?.maker) return ""
  return d.detail?.random_address ? RADIO_MAKER_RANDOM : ""
}

/** How long ago it was last heard, in seconds, or null when this run has not heard it. */
export function secondsSinceHeard(d: RadioLiveDevice, nowSec: number): number | null {
  return d.last_at ? Math.max(0, nowSec - d.last_at) : null
}

export function heardAgoWord(d: RadioLiveDevice, nowSec: number): string {
  const s = secondsSinceHeard(d, nowSec)
  return s === null ? "" : radioHeardAgo(s)
}

/** Short tags for a row: far from its usual, or how many more cycles before it can be sorted. */
export function deviceTags(d: RadioLiveDevice): RadioDeviceTag[] {
  const out: RadioDeviceTag[] = []
  if (d.unusual && !d.usual && d.usual_dbm) out.push({ text: radioShiftWord(d.shift_db ?? 0, d.usual_dbm), warn: true, test: "radio-row-unusual" })
  if (d.detail && d.detail.why === "needs_more_listening") out.push({ text: radioSortsIn(Math.max(1, d.detail.need_heard - d.detail.cycles_heard)), warn: false, test: "radio-row-tag" })
  return out
}

/** The mini trail of where a device was over the last cycles, as polyline points in a width by height box, nearest at the bottom. Empty under two points. */
export function trailPoints(tailM: number[] | undefined, width: number, height: number): string {
  const v = tailM ?? []
  if (v.length < 2) return ""
  const lo = Math.min(...v)
  const hi = Math.max(...v)
  const pad = 2
  return v.map((m, i) => `${((i * width) / (v.length - 1)).toFixed(1)},${(hi === lo ? height / 2 : pad + ((m - lo) / (hi - lo)) * (height - 2 * pad)).toFixed(1)}`).join(" ")
}
