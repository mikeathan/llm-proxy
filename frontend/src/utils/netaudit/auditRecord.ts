import type { AuditRecord, AuditRecordCounts, AuditRecordModule, AuditRecordVulns, AuditRecordWindow } from "../../types/auditRecord"

// The audit's typed record sits beside the report in the run's folder. These read its path and its text; they never throw, so a record
// that is missing, from a newer server or damaged simply means the Markdown report is all there is.

export const RECORD_VERSION = 1
const REPORT_FILE = "report.md"
const RECORD_FILE = "audit.json"
const SEVERITY_ORDER = ["critical", "high", "medium", "low", "info"] as const

/** The record's path for a report path ("…/<run>/report.md" gives "…/<run>/audit.json"); "" for a report saved before run folders. */
export function recordPathOf(reportPath: string): string {
  const at = reportPath.lastIndexOf("/")
  return at >= 0 && reportPath.slice(at + 1) === REPORT_FILE ? `${reportPath.slice(0, at + 1)}${RECORD_FILE}` : ""
}

const isObject = (value: unknown): value is Record<string, unknown> => typeof value === "object" && value !== null && !Array.isArray(value)
const list = <T>(value: unknown): T[] => (Array.isArray(value) ? (value as T[]) : [])
const num = (value: unknown): number => (typeof value === "number" && Number.isFinite(value) ? value : 0)

function countsOf(raw: unknown): AuditRecordCounts {
  const c = isObject(raw) ? raw : {}
  const severity = isObject(c.by_severity) ? Object.fromEntries(Object.entries(c.by_severity).map(([k, v]) => [k, num(v)])) : {}
  return { new: num(c.new), changed: num(c.changed), resolved: num(c.resolved), accepted: num(c.accepted), open_ports: num(c.open_ports), open: num(c.open), by_severity: severity }
}

function vulnsOf(raw: unknown): AuditRecordVulns {
  const v = isObject(raw) ? raw : {}
  return { enabled: v.enabled === true, skipped: v.skipped === true, total: num(v.total), checked: num(v.checked), coarse: num(v.coarse), uncomparable: num(v.uncomparable) }
}

function windowOf(raw: unknown): AuditRecordWindow | undefined {
  if (!isObject(raw) || typeof raw.window !== "string") return undefined
  const scans = isObject(raw.scans) ? raw.scans : {}
  const range = (r: unknown) => (isObject(r) ? { min: num(r.min), max: num(r.max) } : { min: 0, max: 0 })
  return {
    window: raw.window,
    scans: { count: num(scans.count), devices_up: range(scans.devices_up), open_ports: range(scans.open_ports) },
    new_in_window: list(raw.new_in_window), new_more: num(raw.new_more),
    gone_in_window: list(raw.gone_in_window), gone_more: num(raw.gone_more),
    returned_in_window: list(raw.returned_in_window), returned_more: num(raw.returned_more),
    activity: isObject(raw.activity) ? raw.activity : {},
    modules: list<AuditRecordModule>(raw.modules),
    unread: list<string>(raw.unread),
  }
}

/** Reads a record's text; null when it is not valid JSON, not a record, or a version this screen does not know. */
export function parseAuditRecord(text: string): AuditRecord | null {
  let raw: unknown
  try {
    raw = JSON.parse(text)
  } catch {
    return null
  }
  if (!isObject(raw) || raw.version !== RECORD_VERSION || typeof raw.scanned_at !== "string") return null
  return {
    version: RECORD_VERSION,
    audit: typeof raw.audit === "string" ? raw.audit : "",
    scanned_at: raw.scanned_at,
    previous_scan_at: typeof raw.previous_scan_at === "string" ? raw.previous_scan_at : undefined,
    engine: typeof raw.engine === "string" ? raw.engine : "",
    scanned: num(raw.scanned),
    up: num(raw.up),
    counts: countsOf(raw.counts),
    vulns: vulnsOf(raw.vulns),
    exposed: list(raw.exposed),
    window: windowOf(raw.window),
    notes: list<string>(raw.notes),
    assessment: typeof raw.assessment === "string" ? raw.assessment : "",
  }
}

/** The open problems by severity, worst first, only the severities that have any. */
export function severityCounts(counts: AuditRecordCounts): { severity: string; count: number }[] {
  return SEVERITY_ORDER.filter((s) => (counts.by_severity[s] ?? 0) > 0).map((s) => ({ severity: s, count: counts.by_severity[s]! }))
}

const plural = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`
const span = (r: { min: number; max: number }) => (r.min === r.max ? `${r.min}` : `${r.min}–${r.max}`)

/** The closed line of the window part: how many scans, and how far the device and port counts moved. */
export function windowSummary(w: AuditRecordWindow): string {
  if (w.scans.count === 0) return "no scan recorded"
  return `${plural(w.scans.count, "scan", "scans")} · devices up ${span(w.scans.devices_up)} · open ports ${span(w.scans.open_ports)}`
}

/** One module's tally as a sentence: "885 comparisons, 0 differed, 2 could not be compared". */
export function moduleLine(m: AuditRecordModule): string {
  const parts = [`${m.done} ${m.noun}`, `${m.flagged} ${m.flagged_word}`]
  if (m.unclear > 0) parts.push(`${m.unclear} could not be compared`)
  return parts.join(", ")
}

/** How far known-vulnerability matching reached, in one sentence; "" when it was not asked for. */
export function vulnLine(v: AuditRecordVulns): string {
  if (!v.enabled) return ""
  if (v.skipped) return "Known-vulnerability matching was on, but there was no vulnerability data to compare with."
  return `Compared ${v.checked} of ${plural(v.total, "service", "services")} with the vulnerability data (${v.coarse} had too vague a version).`
}
