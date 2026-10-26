import type { DeviceGroup, FindingsReport, ReportSummary } from '../../types/netaudit'

// What a report means for the person reading it. A finding needs a look when the latest scan showed it for the first
// time or with a different version, and nobody has accepted it.
export function summarize(report: FindingsReport): ReportSummary {
  const open = report.findings.filter((f) => !f.accepted)
  const added = open.filter((f) => f.new)
  const changed = open.filter((f) => f.changed && !f.new)
  return {
    needLook: open.filter((f) => f.new || f.changed),
    added,
    changed,
    accepted: report.findings.filter((f) => f.accepted),
    resolved: report.resolved,
  }
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
