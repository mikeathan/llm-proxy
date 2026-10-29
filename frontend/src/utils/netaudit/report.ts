import type { DeepCheck, DeviceGroup, FindingsReport, ProblemGroup, ReportSummary, ScanFinding } from '../../types/netaudit'
import { isCheck, severityRank } from './friendly'

// What a report means for the person reading it. A finding needs a look when the latest scan showed it for the first
// time or with a different version, and nobody has accepted it. On the first scan nothing is "new": the open ports are
// the baseline to compare the next scan with, so only the problems a check found need a look.
export function summarize(report: FindingsReport): ReportSummary {
  const baseline = !!report.first_scan
  const open = report.findings.filter((f) => !f.accepted)
  const isNew = (f: ScanFinding) => f.new && !(baseline && !isCheck(f))
  const added = open.filter(isNew)
  const changed = open.filter((f) => f.changed && !isNew(f))
  const worstFirst = (a: { severity: string }, b: { severity: string }) => severityRank(b.severity) - severityRank(a.severity)
  const problems = open.filter(isCheck).sort(worstFirst)
  return {
    baseline,
    // A new or changed finding needs a look once; a problem a check found keeps needing one until it is accepted.
    needLook: open.filter((f) => isNew(f) || f.changed || isCheck(f)).sort(worstFirst),
    problems,
    added,
    changed,
    accepted: report.findings.filter((f) => f.accepted),
    resolved: report.resolved,
    problemGroups: groupProblems(problems),
  }
}

/** How long an audit keeps carrying a deep finding (the server's rule); a stored result older than this is no longer shown as a problem. */
const DEEP_CARRY_DAYS = 90

/**
 * The problems of the stored deep checks that the latest audit report does not carry yet. An audit carries a deep finding
 * into its report on its next run, only while the device is up and the port is still open and for 90 days (and then it is
 * an ordinary finding, with a key to accept it by), so this lists the same ones: a finding the report already has is not
 * listed twice, and one whose port the scan no longer sees open is not listed. Until carried it is read-only and dated.
 */
export function deepProblemsOf(report: FindingsReport, checks: readonly DeepCheck[], now: number = Date.now()): ScanFinding[] {
  const id = (f: { device: string; port?: number; check: string }) => `${f.device}|${f.port}|${f.check}`
  const carried = new Set(report.findings.map(id))
  const open = new Set(report.findings.filter((f) => !isCheck(f)).map((f) => `${f.device}|${f.port}`))
  const fresh = (at: string) => now - Date.parse(at) <= DEEP_CARRY_DAYS * 86_400_000
  return checks
    .filter((c) => fresh(c.at))
    .flatMap((c) =>
      c.findings
        .filter((f) => !carried.has(id(f)) && open.has(`${f.device}|${f.port}`))
        .map((f): ScanFinding => ({ ...f, key: `deep:${f.device}:${f.port}:${f.check}`, new: false, changed: false, accepted: false, deep_at: c.at })),
    )
    .sort((a, b) => severityRank(b.severity) - severityRank(a.severity))
}

const UNTITLED = 'A check found a problem'

/** One group per check, worst first; within a group the worst place first, then in the order the scan listed them. */
export function groupProblems(problems: readonly ScanFinding[]): ProblemGroup[] {
  const groups = new Map<string, ProblemGroup>()
  for (const f of problems) {
    const group = groups.get(f.check) ?? { check: f.check, title: f.check_title || f.evidence || UNTITLED, severity: f.severity, fix: f.fix ?? '', items: [] }
    group.items.push(f)
    if (severityRank(f.severity) > severityRank(group.severity)) group.severity = f.severity
    groups.set(f.check, group)
  }
  const worstFirst = (a: { severity: string }, b: { severity: string }) => severityRank(b.severity) - severityRank(a.severity)
  return [...groups.values()].sort(worstFirst).map((g) => ({ ...g, items: [...g.items].sort(worstFirst) }))
}

/** Items grouped by device, in the order the devices first appear. */
export function groupByDevice<T extends { device: string }>(items: readonly T[]): DeviceGroup<T>[] {
  const groups = new Map<string, DeviceGroup<T>>()
  for (const item of items) {
    const group = groups.get(item.device) ?? { device: item.device, items: [] }
    group.items.push(item)
    groups.set(item.device, group)
  }
  return [...groups.values()]
}
