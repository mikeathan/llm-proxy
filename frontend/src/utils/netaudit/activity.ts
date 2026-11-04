import type { ActivityFilter, ActivityGroupOfRows, ActivityRow } from '../../types/netaudit'

const MS_PER_DAY = 86_400_000

const plural = (n: number, one: string, many: string): string => `${n} ${n === 1 ? one : many}`

function who(row: ActivityRow): string {
  return row.name || row.ip || row.mac || 'A device'
}

function portsText(opened: number, closed: number): string {
  if (opened && closed) return `${plural(opened, 'port', 'ports')} opened and ${plural(closed, 'port', 'ports')} closed`
  return opened ? `${plural(opened, 'port', 'ports')} opened` : `${plural(closed, 'port', 'ports')} closed`
}

function auditText(opened: number, closed: number): string {
  const found = opened ? `The audit found ${plural(opened, 'new problem', 'new problems')}` : 'The audit found nothing new'
  return closed ? `${found}; ${closed} went away` : found
}

/**
 * The words for one row. They come from the kind and its typed fields, so a device's own text never becomes a sentence; an
 * alert is the exception and shows the alert's wording, which the program wrote.
 */
export function activityText(row: ActivityRow): string {
  switch (row.kind) {
    case 'new_device':
      return `${who(row)} appeared on the network`
    case 'address_changed':
      return `${row.ip ?? 'An address'} moved to a different device`
    case 'alert':
      return (row.count ?? 1) > 1 ? `${row.text ?? 'An alert'} (${row.count} times)` : (row.text ?? 'An alert')
    case 'ports_changed':
      return portsText(row.opened ?? 0, row.closed ?? 0)
    case 'audit':
      return auditText(row.opened ?? 0, row.closed ?? 0)
    case 'related':
      return `${who(row)} appeared within an hour before a decoy service was touched`
  }
}

export function filterRows(rows: ActivityRow[], filter: ActivityFilter): ActivityRow[] {
  return filter === 'all' ? rows : rows.filter((r) => r.group === filter)
}

const startOfDay = (ms: number): number => {
  const d = new Date(ms)
  return new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime()
}

/** Today, Yesterday, or the weekday and date, in the browser's time. */
export function dayHeading(iso: string, nowMs: number = Date.now(), locale?: string): string {
  const days = Math.round((startOfDay(nowMs) - startOfDay(new Date(iso).getTime())) / MS_PER_DAY)
  if (days <= 0) return 'Today'
  if (days === 1) return 'Yesterday'
  return new Intl.DateTimeFormat(locale, { weekday: 'short', day: 'numeric', month: 'short' }).format(new Date(iso))
}

/** The hour and minute of a row, in the browser's time; a row that stands for a whole day has none. */
export function clockTime(row: ActivityRow, locale?: string): string {
  return row.day ? '' : new Intl.DateTimeFormat(locale, { hour: '2-digit', minute: '2-digit' }).format(new Date(row.at))
}

/** The rows, newest first as given, under one heading per day. */
export function groupByDay(rows: ActivityRow[], nowMs: number = Date.now(), locale?: string): ActivityGroupOfRows[] {
  const groups: ActivityGroupOfRows[] = []
  for (const row of rows) {
    const heading = dayHeading(row.at, nowMs, locale)
    const last = groups[groups.length - 1]
    if (last?.heading === heading) last.rows.push(row)
    else groups.push({ heading, rows: [row] })
  }
  return groups
}
