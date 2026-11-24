import type { RouteLocationNamedRaw } from 'vue-router'
import { toNetwork, toRadio } from '../../router/routes'
import type { BriefHomeLevel, BriefHomeLine } from '../../types/newsroom'

const RADIO_PREFIX = 'radio_'

/** A line's level; one written before levels existed (none) is routine. */
export const homeLevel = (l: BriefHomeLine): BriefHomeLevel => l.level ?? 'info'

/** The page where a fact is looked at and acted on: a Radio fact on Radio, everything else on Network. */
export const homeLineRoute = (l: BriefHomeLine): RouteLocationNamedRaw => (l.id.startsWith(RADIO_PREFIX) ? toRadio() : toNetwork())

/** The lines split by what they ask of the owner; the core already sends them most urgent first, and that order is kept. */
export function splitHome(lines: BriefHomeLine[]): { critical: BriefHomeLine[]; check: BriefHomeLine[]; routine: BriefHomeLine[] } {
  return {
    critical: lines.filter((l) => homeLevel(l) === 'critical'),
    check: lines.filter((l) => homeLevel(l) === 'check'),
    routine: lines.filter((l) => homeLevel(l) === 'info'),
  }
}
