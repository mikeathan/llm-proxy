import {
  DNS_CHECK_LAST_CHECK_WAS, DNS_CHECK_LAST_CHECKED, DNS_CHECK_LAST_COMPARISON, DNS_CHECK_LATE_BUT, DNS_CHECK_LATE_MAY_HAVE_STOPPED, DNS_CHECK_MAX_DIFFERENCES,
  DNS_CHECK_NO_ADDRESS, DNS_CHECK_STOPPED_AFTER_RESTART,
  DNS_CHECK_INTERNET_OFF, DNS_CHECK_INTERNET_OFF_LINE, DNS_CHECK_LAN_OFF, DNS_CHECK_LAN_OFF_LINE, DNS_CHECK_NONE_COMPARABLE, DNS_CHECK_NOT_YET, DNS_CHECK_RECORD_ONLY,
} from '../../constants/netaudit'
import { formatRelativeTime } from '../format/time'
import type { DnsCheckState, DnsCheckSummary, DnsDifferenceRow, DnsFreshness } from '../../types/netaudit'

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

const NEVER_YEAR = 1
const MS_PER_MINUTE = 60_000
/** A check is called late once it is more than this many intervals overdue. */
const LATE_AFTER_INTERVALS = 2
const isDate = (iso: string) => new Date(iso).getUTCFullYear() > NEVER_YEAR

/**
 * When the check last ran, so a person can tell it is alive. The last-run time is kept in memory only: after the program
 * restarts it is empty until the next cycle, and then the last logged comparison says when it last did anything.
 */
export function dnsCheckFreshness(state: DnsCheckState, nowMs: number): DnsFreshness {
  const { last_run: lastRun } = state.status
  if (isDate(lastRun)) {
    const ago = formatRelativeTime(lastRun, nowMs)
    const late = nowMs - Date.parse(lastRun) > LATE_AFTER_INTERVALS * state.interval_minutes * MS_PER_MINUTE
    return late
      ? { late, text: `${DNS_CHECK_LAST_CHECK_WAS} ${ago}, ${DNS_CHECK_LATE_BUT} ${state.interval_minutes} ${DNS_CHECK_LATE_MAY_HAVE_STOPPED}` }
      : { late, text: `${DNS_CHECK_LAST_CHECKED} ${ago}.` }
  }
  const { last } = state.summary
  return isDate(last)
    ? { late: false, text: `${DNS_CHECK_STOPPED_AFTER_RESTART} ${DNS_CHECK_LAST_COMPARISON} ${formatRelativeTime(last, nowMs)}.` }
    : { late: false, text: '' }
}

/** The newest differences for the screen, with how many more there are than are shown. */
export function dnsDifferenceRows(summary: DnsCheckSummary, nowMs: number): { rows: DnsDifferenceRow[]; hidden: number } {
  const addresses = (list?: string[]) => (list && list.length > 0 ? list.join(', ') : DNS_CHECK_NO_ADDRESS)
  const rows = summary.differences.slice(0, DNS_CHECK_MAX_DIFFERENCES).map((d) => ({
    key: `${d.at}|${d.name}`,
    name: d.name,
    when: formatRelativeTime(d.at, nowMs),
    router: addresses(d.router),
    outside: addresses(d.outside),
    partly: d.overlap === true,
  }))
  return { rows, hidden: Math.max(0, summary.differs - rows.length) }
}
