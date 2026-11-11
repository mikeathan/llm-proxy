import { LOOKUPS_FLAG_LABELS } from '../../constants/netaudit'
import type { DeviceLookupName, DeviceLookups, LookupFlag, LookupGroup } from '../../types/netaudit'

/** The flags in order of how much they matter, most serious first. */
const FLAG_ORDER: readonly LookupFlag[] = ['known_bad', 'encrypted_dns', 'beaconing', 'new_place', 'around_dns']
const rank = (f: LookupFlag | undefined): number => (f ? FLAG_ORDER.length - FLAG_ORDER.indexOf(f) : 0)

/** A device's names grouped by domain (googleapis.com for a.b.googleapis.com), the newest group first. Names keep their order inside a group. */
export function groupLookups(names: readonly DeviceLookupName[]): LookupGroup[] {
  const groups = new Map<string, LookupGroup>()
  for (const n of names) {
    const domain = n.domain || n.name
    const g = groups.get(domain) ?? { domain, names: [], last: n.last }
    g.names.push(n)
    if (n.last > g.last) g.last = n.last
    if (rank(n.flag) > rank(g.flag)) g.flag = n.flag
    groups.set(domain, g)
  }
  return [...groups.values()].sort((a, b) => (a.last < b.last ? 1 : a.last > b.last ? -1 : 0))
}

/** A device's flags as words, the most serious first. */
export function flagWords(flags: readonly LookupFlag[]): { flag: LookupFlag; label: string }[] {
  return [...flags].sort((a, b) => rank(b) - rank(a)).map((flag) => ({ flag, label: LOOKUPS_FLAG_LABELS[flag] ?? flag }))
}

/** Devices that raised a flag come first (the server already orders them); this keeps that order and says whether any device has one. */
export const anyFlagged = (devices: readonly DeviceLookups[]): boolean => devices.some((d) => d.flags.length > 0)
