import { PORT_NAMES, ROUTER_PORTS_SHOWN } from '../../constants/netaudit'
import type { ActivityFilter, ActivityGroupOfRows, ActivityItem, ActivityRow } from '../../types/netaudit'

const MS_PER_DAY = 86_400_000

const plural = (n: number, one: string, many: string): string => `${n} ${n === 1 ? one : many}`

/** A port by what it is when that is well known ("SSH (22)"), else by its number. */
export function portLabel(port: number): string {
  const name = PORT_NAMES[port]
  return name ? `${name} (${port})` : `port ${port}`
}

/** The busiest ports and how often each was tried, on one line: "SSH (22) 700 · MySQL (3306) 300". */
export function portsLine(ports: ActivityRow['ports']): string {
  return (ports ?? []).slice(0, ROUTER_PORTS_SHOWN).map((p) => `${portLabel(p.port)} ${p.count.toLocaleString('en-US')}`).join(' · ')
}

function listWords(items: string[]): string {
  return items.length < 2 ? (items[0] ?? '') : `${items.slice(0, -1).join(', ')} and ${items[items.length - 1]}`
}

function blocksText(count: number, ports: ActivityRow['ports']): string {
  const attempts = count === 1 ? 'connection attempt' : 'connection attempts'
  const base = `The router refused ${count.toLocaleString('en-US')} ${attempts} from the internet`
  const busiest = (ports ?? []).slice(0, ROUTER_PORTS_SHOWN).map((p) => portLabel(p.port))
  return busiest.length ? `${base}, mostly to ${listWords(busiest)}` : base
}

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
    case 'router_blocks':
      return blocksText(row.count ?? 0, row.ports)
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

const MS_PER_MINUTE = 60_000
/** A presence alert that comes this soon after a new device is the same event told twice. */
const ECHO_WINDOW_MS = 60 * MS_PER_MINUTE
/** How many new devices on one day are shown as one line, and how many of their names that line gives. */
const FOLD_FROM = 3
const FOLD_NAMES = 3

/** One short line for a day: what happened, counted, in a fixed order. Empty for a day with nothing in it. */
export function daySummary(rows: ActivityRow[]): string {
  const count = (kind: ActivityRow['kind']) => rows.filter((r) => r.kind === kind).length
  const sum = (field: 'opened' | 'closed') => rows.filter((r) => r.kind === 'ports_changed').reduce((n, r) => n + (r[field] ?? 0), 0)
  const alerts = rows.filter((r) => r.kind === 'alert').reduce((n, r) => n + (r.count ?? 1), 0)
  const refused = rows.filter((r) => r.kind === 'router_blocks').reduce((n, r) => n + (r.count ?? 0), 0)
  const parts = [
    count('new_device') ? plural(count('new_device'), 'new device', 'new devices') : '',
    count('address_changed') ? plural(count('address_changed'), 'moved address', 'moved addresses') : '',
    sum('opened') || sum('closed') ? portsText(sum('opened'), sum('closed')) : '',
    count('audit') ? 'audit result' : '',
    alerts ? plural(alerts, 'alert', 'alerts') : '',
    count('related') ? 'possibly related' : '',
    refused ? `${refused.toLocaleString('en-US')} refused at the router` : '',
  ]
  return parts.filter(Boolean).join(' · ')
}

/**
 * Three or more new devices of one day become one line that names the first three; everything else stays as it is, in order.
 * The fold sits where the newest of the folded rows was.
 */
export function foldRows(rows: ActivityRow[]): ActivityItem[] {
  const devices = rows.filter((r) => r.kind === 'new_device')
  if (devices.length < FOLD_FROM) return rows.map((row) => ({ kind: 'row', row }))
  const names = devices.slice(0, FOLD_NAMES).map(who).join(', ')
  const more = devices.length > FOLD_NAMES ? ` and ${devices.length - FOLD_NAMES} more` : ''
  const fold: ActivityItem = { kind: 'fold', rows: devices, text: `${devices.length} new devices appeared: ${names}${more}` }
  const items: ActivityItem[] = []
  let placed = false
  for (const row of rows) {
    if (row.kind !== 'new_device') items.push({ kind: 'row', row })
    else if (!placed) {
      items.push(fold)
      placed = true
    }
  }
  return items
}

/** Removes a presence alert that only repeats a new-device row from the hour before it; the alert is the same event. */
export function dropEchoedAlerts(rows: ActivityRow[]): ActivityRow[] {
  const devices = rows.filter((r) => r.kind === 'new_device').map((r) => new Date(r.at).getTime())
  return rows.filter((r) => {
    if (r.kind !== 'alert' || r.alert_kind !== 'presence') return true
    const at = new Date(r.at).getTime()
    return !devices.some((d) => at - d >= 0 && at - d < ECHO_WINDOW_MS)
  })
}
