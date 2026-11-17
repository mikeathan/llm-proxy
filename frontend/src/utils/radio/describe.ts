import {
  RADIO_ACCESS_POINT, RADIO_CLASS_WORDS, RADIO_MAKER_RANDOM, RADIO_MAKER_UNKNOWN, radioChannelBand, radioDotLabel, radioHeardAgo, radioShiftWord, radioSortsIn, radioWifiWord,
} from "../../constants/radio"
import type { RadioDeviceTag, RadioLiveDevice } from "../../types/radio"

// Radio · what a device row says about a device, from what its frames revealed. The title is the owner's name, else the maker; the short id stays as
// a handle. Nothing here is an address, and a network name only appears while the device is heard.

/** The owner's name for it, else its maker, else why there is no maker. */
export function deviceTitle(d: RadioLiveDevice): string {
  if (d.mine) return radioDotLabel(d)
  if (d.network) return d.network
  if (d.detail?.maker) return d.detail.maker
  return d.detail?.random_address ? RADIO_MAKER_RANDOM : RADIO_MAKER_UNKNOWN
}

/** The short name drawn beside a dot: the owner's name, the network it broadcasts, its maker, else the short id. */
export function deviceLabel(d: RadioLiveDevice): string {
  return d.mine ? radioDotLabel(d) : d.network || d.detail?.maker || d.id
}

/** The line under the title: the kind it was sorted into, its Wi-Fi generation, and the channel it was heard on. */
export function deviceFacts(d: RadioLiveDevice): string[] {
  const out: string[] = []
  if (!d.mine) out.push(RADIO_CLASS_WORDS[d.class])
  if (d.detail?.wifi_generation) out.push(RADIO_ACCESS_POINT, radioWifiWord(d.detail.wifi_generation))
  if (d.channel) out.push(radioChannelBand(d.channel))
  return out
}

/** The maker, shown beside a network name so both are visible; empty when it is the title already or unknown. */
export function deviceMakerNote(d: RadioLiveDevice): string {
  return d.network && d.detail?.maker ? d.detail.maker : ""
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
