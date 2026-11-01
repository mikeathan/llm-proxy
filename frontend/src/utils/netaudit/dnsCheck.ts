import {
  DNS_CHECK_INTERNET_OFF, DNS_CHECK_INTERNET_OFF_LINE, DNS_CHECK_LAN_OFF, DNS_CHECK_LAN_OFF_LINE, DNS_CHECK_NONE_COMPARABLE, DNS_CHECK_NOT_YET, DNS_CHECK_RECORD_ONLY,
} from '../../constants/netaudit'
import type { DnsCheckState } from '../../types/netaudit'

const count = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`

/** What to bring back after the week: the results with the settings they were made under, so a paste explains itself. */
export function dnsCheckReport(state: DnsCheckState): object {
  return {
    names: state.names,
    server: state.server || 'default gateway',
    resolver_url: state.resolver_url,
    interval_minutes: state.interval_minutes,
    problem: state.status.problem ?? '',
    summary: state.summary,
  }
}

/**
 * The one line under the switch: what the check has recorded, in words. Comparisons that could not be made are counted apart
 * and never read as agreement. `since` is the first comparison's date already written for a person.
 */
export function dnsCheckLine(state: DnsCheckState, since: string): string {
  const { summary, status } = state
  if (status.problem === DNS_CHECK_LAN_OFF) return DNS_CHECK_LAN_OFF_LINE
  if (status.problem === DNS_CHECK_INTERNET_OFF) return DNS_CHECK_INTERNET_OFF_LINE
  if (summary.comparisons === 0) return summary.cannot_compare > 0 ? DNS_CHECK_NONE_COMPARABLE : DNS_CHECK_NOT_YET
  const parts = [`Compared ${count(summary.comparisons, 'time', 'times')}, starting ${since}: ${summary.matches} agreed, ${summary.differs} differed.`]
  if (summary.cannot_compare > 0) parts.push(`${count(summary.cannot_compare, 'try', 'tries')} could not be made.`)
  parts.push(DNS_CHECK_RECORD_ONLY)
  return parts.join(' ')
}
