import { describe, it, expect } from 'vitest'
import { fixesSummary, notesSummary, parseAuditReport } from '../../../utils/netaudit/auditReport'

// A synthetic repeat-scan report in the shape the code writes (never data from a real network).
const REPORT = [
  '## Observed (generated from the scan data, not written by the model)',
  '',
  'Scanned 254 address(es) on 2026-11-12 15:13 GMT with nmap: 15 up, 239 down.',
  '',
  'Compared with the scan of 2026-11-12 14:48 UTC: 1 new, 1 resolved, 0 changed, 56 unchanged.',
  '',
  '### New',
  '',
  '| Device | Port | Service | Version |',
  '|---|---|---|---|',
  '| 192.0.2.43 | 10001/tcp |  |  |',
  '',
  '### Still open',
  '',
  '| Severity | Finding | Where |',
  '|---|---|---|',
  '| high | A program file no longer matches its package | this-computer |',
  '| low | The firewall is switched off | this-computer |',
  '',
  'Suggested fixes:',
  '',
  '- **Program file**: Reinstall the package.',
  '- **Firewall**: Switch it on.',
  '',
  '### Resolved',
  '',
  '| Device | Port | Service | Version |',
  '|---|---|---|---|',
  '| 192.0.2.64 | 49152/tcp |  |  |',
  '| 192.0.2.34 | 49152/tcp |  |  |',
  '',
  '### Exposed services worth reviewing',
  '',
  'Remote-administration services the scan identified.',
  '',
  '| Device | Port | Service | Version | Kind |',
  '|---|---|---|---|---|',
  '| 192.0.2.10 | 22/tcp | ssh | OpenSSH 9.9 | Remote administration |',
  '',
  '### Last 24h before this scan',
  '',
  '5 scan(s) recorded: devices up 15 to 16, open ports 36 to 38. The records end at the scan of 2026-11-12 14:48 UTC.',
  '',
  '- Went away: 192.0.2.64 port 49152 (open port).',
  '',
  '> Note: 14 watched locations could not be read.',
  '',
  '### Ways in that matter',
  '',
  '1. Any device → SSH accepts passwords → this computer. Set up key login.',
  '',
  '---',
  '',
  '## Assessment (written by the model)',
  '',
  '1. **high**: A program file no longer matches its package.',
].join('\n')

describe('parseAuditReport', () => {
  it('cuts a report at its headings, with the assessment apart and the lead kept', () => {
    const view = parseAuditReport(REPORT)!
    expect(view.sections.map((s) => s.title)).toEqual(['New', 'Still open', 'Resolved', 'Exposed services worth reviewing', 'Last 24h before this scan', 'Ways in that matter'])
    expect(view.lead).toContain('Scanned 254 address(es)')
    expect(view.lead).toContain('Compared with the scan of')
    expect(view.assessment).toBe('1. **high**: A program file no longer matches its package.')
    expect(view.preface).toBe('')
  })

  it('keeps what the operator acts on open and folds the reference parts with a one-line summary', () => {
    const byTitle = Object.fromEntries(parseAuditReport(REPORT)!.sections.map((s) => [s.title, s]))
    expect(['New', 'Still open', 'Ways in that matter'].every((t) => byTitle[t]!.open)).toBe(true)
    expect(byTitle['Resolved']).toMatchObject({ open: false, summary: '2 ports' })
    expect(byTitle['Exposed services worth reviewing']).toMatchObject({ open: false, summary: '1 service' })
    expect(byTitle['Last 24h before this scan']!.open).toBe(false)
    expect(byTitle['Last 24h before this scan']!.summary).toBe('5 scan(s) recorded: devices up 15 to 16, open ports 36 to 38.')
  })

  it('separates the suggested fixes from the table they follow', () => {
    const still = parseAuditReport(REPORT)!.sections.find((s) => s.title === 'Still open')!
    expect(still.body).toContain('| high |')
    expect(still.body).not.toContain('Suggested fixes')
    expect(fixesSummary(still.fixes)).toBe('2 fixes')
    expect(still.fixes).toContain('Reinstall the package')
  })

  it('lifts the code\'s notes out of the sections and counts them', () => {
    const view = parseAuditReport(REPORT)!
    expect(view.notes).toEqual(['Note: 14 watched locations could not be read.'])
    expect(notesSummary(view.notes)).toBe('1 note')
    expect(view.sections.find((s) => s.title === 'Last 24h before this scan')!.body).not.toContain('Note:')
  })

  it('shows a part it does not know rather than folding it away', () => {
    const view = parseAuditReport(REPORT.replace('### Ways in that matter', '### Something new'))!
    expect(view.sections.find((s) => s.title === 'Something new')!.open).toBe(true)
  })

  it('keeps a saved run\'s header lines as the preface', () => {
    const view = parseAuditReport(`[2026-11-12 15:09:32] ▶ Executing automation\n\n### Final Report\n\n${REPORT}`)!
    expect(view.preface).toContain('Executing automation')
    expect(view.sections[0]!.title).toBe('New')
  })

  it('handles a report with no assessment and one that is not an audit report', () => {
    expect(parseAuditReport(REPORT.split('---')[0]!)!.assessment).toBe('')
    expect(parseAuditReport('# A plain report\n\nNothing audited here.')).toBeNull()
  })
})
