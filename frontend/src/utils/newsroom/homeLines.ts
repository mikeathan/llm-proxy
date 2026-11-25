import type { RouteLocationNamedRaw } from 'vue-router'
import { toNetwork, toNetworkCard, toRadio, toRadioFinding } from '../../router/routes'
import type { BriefHomeLevel, BriefHomeLine } from '../../types/newsroom'

const RADIO_PREFIX = 'radio_'

/** A line's level; one written before levels existed (none) is routine. */
export const homeLevel = (l: BriefHomeLine): BriefHomeLevel => l.level ?? 'info'

// A fact is a kind of finding, not one instance (the line carries an id and a count, never a name or an address), so a link goes to that kind's own place.
const DEVICE_FACTS = new Set(['network_online', 'network_not_marked_mine', 'network_address_changes', 'network_presence_unknown'])
const RADIO_NO_FINDING = new Set(['radio_off', 'radio_quiet', 'radio_unavailable'])

/** Where a fact is looked at and acted on: a Radio finding on the Radio board's finding card, scrolled to and marked; a device fact on the Devices board; anything else on its page. */
export function homeLineRoute(l: BriefHomeLine): RouteLocationNamedRaw {
  if (l.id.startsWith(RADIO_PREFIX)) return RADIO_NO_FINDING.has(l.id) ? toRadio() : toRadioFinding(l.id)
  return DEVICE_FACTS.has(l.id) ? toNetworkCard('network-devices') : toNetwork()
}

/** The lines split by what they ask of the owner; the core already sends them most urgent first, and that order is kept. */
export function splitHome(lines: BriefHomeLine[]): { critical: BriefHomeLine[]; check: BriefHomeLine[]; routine: BriefHomeLine[] } {
  return {
    critical: lines.filter((l) => homeLevel(l) === 'critical'),
    check: lines.filter((l) => homeLevel(l) === 'check'),
    routine: lines.filter((l) => homeLevel(l) === 'info'),
  }
}
