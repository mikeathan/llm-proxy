import {
  NETWORK_POSTURE_HEADLINES,
} from '../../constants/netaudit'
import type { NetworkChange, NetworkDay, NetworkOverview } from '../../types/netaudit'

const plural = (n: number, one: string, many: string): string => `${n} ${n === 1 ? one : many}`

/** Why the page says "needs a look", one plain sentence per reason, most serious first. */
export function attentionReasons(o: NetworkOverview): string[] {
  const out: string[] = []
  if (o.gateway_changed) out.push("The router's hardware address changed")
  if (o.findings.serious > 0) out.push(`${plural(o.findings.serious, 'serious finding', 'serious findings')} in the last audit`)
  if (o.devices.new > 0) out.push(`${plural(o.devices.new, 'new device', 'new devices')} nobody has approved`)
  if (o.shared_address) out.push('Two addresses answer as one device')
  return out
}

export const postureHeadline = (o: NetworkOverview): string => NETWORK_POSTURE_HEADLINES[o.posture]

/** One value per day for a trend, oldest first; a day without the figure repeats the one before so a gap is not a drop. */
export function seriesOf(days: NetworkDay[], pick: (d: NetworkDay) => number | null): number[] {
  const out: number[] = []
  let last = 0
  for (const d of days) {
    const v = pick(d)
    last = v ?? last
    out.push(last)
  }
  return out
}

/** The sentence for one recent change. A device is named by what the operator called it, else by its address. */
export function changeText(c: NetworkChange): string {
  const who = c.name || c.ip || c.mac || 'A device'
  return c.kind === 'new_device' ? `${who} appeared on the network` : `${c.ip ?? 'An address'} moved to a different device`
}
