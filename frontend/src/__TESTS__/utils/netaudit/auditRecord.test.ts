import { describe, expect, it } from 'vitest'
import { moduleLine, parseAuditRecord, recordPathOf, severityCounts, vulnLine, windowSummary } from '../../../utils/netaudit/auditRecord'

const record = (over: Record<string, unknown> = {}) => JSON.stringify({
  version: 1, audit: 'network-audit', scanned_at: '2026-11-12T02:00:00Z', engine: 'nmap', scanned: 20, up: 15,
  counts: { new: 1, changed: 0, resolved: 2, accepted: 5, open_ports: 37, open: 15, by_severity: { high: 1, medium: 4, low: 10 } },
  vulns: { enabled: true, total: 37, checked: 30, coarse: 2, uncomparable: 0 }, ...over,
})

describe('recordPathOf', () => {
  it('is the audit.json beside a report in a run folder', () => {
    expect(recordPathOf('reports/network-audit/20261112T020000Z-r1/report.md')).toBe('reports/network-audit/20261112T020000Z-r1/audit.json')
  })
  it('is empty for a report saved before run folders, and for anything else', () => {
    expect(recordPathOf('reports/network-audit/20261112T020000Z-r1.md')).toBe('')
    expect(recordPathOf('report.md')).toBe('')
    expect(recordPathOf('')).toBe('')
  })
})

describe('parseAuditRecord', () => {
  it('reads a version 1 record and fills what is missing with empty values', () => {
    const rec = parseAuditRecord(record())!
    expect(rec.up).toBe(15)
    expect(rec.counts.by_severity.medium).toBe(4)
    expect(rec.exposed).toEqual([])
    expect(rec.notes).toEqual([])
    expect(rec.window).toBeUndefined()
    expect(rec.assessment).toBe('')
  })
  it('is null for text that is not a record', () => {
    for (const text of ['', 'not json', '[]', '{}', record({ version: 2 }), record({ scanned_at: 7 })]) expect(parseAuditRecord(text)).toBeNull()
  })
  it('ignores a wrong type in a part rather than failing the whole record', () => {
    const rec = parseAuditRecord(record({ exposed: 'x', notes: 5, counts: { open: 'many' } }))!
    expect(rec.exposed).toEqual([])
    expect(rec.notes).toEqual([])
    expect(rec.counts.open).toBe(0)
  })
})

describe('what the overview says', () => {
  it('lists severities worst first and only those present', () => {
    expect(severityCounts({ new: 0, changed: 0, resolved: 0, accepted: 0, open_ports: 0, open: 3, by_severity: { low: 1, high: 2 } })).toEqual([{ severity: 'high', count: 2 }, { severity: 'low', count: 1 }])
  })
  it('sums up the window on one line', () => {
    const w = { window: 'Last 24 hours', scans: { count: 3, devices_up: { min: 14, max: 15 }, open_ports: { min: 37, max: 37 } }, new_in_window: [], gone_in_window: [], returned_in_window: [], activity: {}, modules: [], unread: [] }
    expect(windowSummary(w)).toBe('3 scans · devices up 14–15 · open ports 37')
    expect(windowSummary({ ...w, scans: { ...w.scans, count: 0 } })).toBe('no scan recorded')
  })
  it('words a module tally and the vulnerability coverage', () => {
    expect(moduleLine({ title: 'Router DNS check', noun: 'comparisons', flagged_word: 'differed', done: 885, flagged: 0, unclear: 2 })).toBe('885 comparisons, 0 differed, 2 could not be compared')
    expect(vulnLine({ enabled: false, total: 0, checked: 0, coarse: 0, uncomparable: 0 })).toBe('')
    expect(vulnLine({ enabled: true, total: 37, checked: 30, coarse: 2, uncomparable: 0 })).toContain('Compared 30 of 37 services')
    expect(vulnLine({ enabled: true, skipped: true, total: 0, checked: 0, coarse: 0, uncomparable: 0 })).toContain('no vulnerability data')
  })
})
