import { describe, expect, it } from 'vitest'
import { deepProblemsOf, groupByDevice, groupProblems, groupProblemsByDevice, sharedEvidence, splitProblems, summarize } from '../../../utils/netaudit/report'
import type { DeepCheck, FindingsReport, ScanFinding } from '../../../types/netaudit'

const f = (key: string, over: Partial<ScanFinding> = {}): ScanFinding =>
  ({ key, device: 'a', port: 1, check: 'open_port', severity: 'info', new: false, changed: false, accepted: false, ...over })
const report = (findings: ScanFinding[]): FindingsReport => ({ scanned: true, findings, resolved: [{ device: 'z' }], unscanned: [], devices_up: 1 })

describe('summarize', () => {
  it('needs a look: new or changed, and not accepted', () => {
    const s = summarize(report([f('new', { new: true }), f('chg', { changed: true }), f('old'), f('ok', { new: true, accepted: true })]))
    expect(s.needLook.map((x) => x.key)).toEqual(['new', 'chg'])
    expect(s.added.map((x) => x.key)).toEqual(['new'])
    expect(s.changed.map((x) => x.key)).toEqual(['chg'])
    expect(s.accepted.map((x) => x.key)).toEqual(['ok'])
    expect(s.resolved).toHaveLength(1)
  })
})

describe('groupByDevice', () => {
  it('groups in the order devices first appear', () => {
    const groups = groupByDevice([{ device: 'b', n: 1 }, { device: 'a', n: 2 }, { device: 'b', n: 3 }])
    expect(groups.map((g) => [g.device, g.items.length])).toEqual([['b', 2], ['a', 1]])
  })

  it('keeps an unaccepted problem in view after the scan it was found in, until it is accepted', () => {
    const s = summarize(report([f('smb', { check: 'smb_v1', severity: 'high' }), f('port'), f('done', { check: 'smb_v1', severity: 'high', accepted: true })]))
    expect(s.problems.map((x) => x.key)).toEqual(['smb'])
    expect(s.needLook.map((x) => x.key)).toEqual(['smb'])
    expect(s.accepted.map((x) => x.key)).toEqual(['done'])
  })

  it('puts the worst things first, and counts a changed or new check as needing a look', () => {
    const s = summarize(report([
      f('port', { new: true, severity: 'info' }),
      f('smb', { new: true, check: 'smb_v1', severity: 'high' }),
      f('redis', { new: true, check: 'redis_no_auth', severity: 'critical' }),
      f('cert', { new: true, check: 'tls_cert_expiring', severity: 'low' }),
    ]))
    expect(s.needLook.map((x) => x.key)).toEqual(['redis', 'smb', 'cert', 'port'])
  })
})

describe('summarize on the first scan', () => {
  it('counts no open port as new, but keeps every unaccepted problem', () => {
    const s = summarize({ ...report([f('port', { new: true }), f('smb', { new: true, check: 'smb_v1', severity: 'high' })]), first_scan: true })
    expect(s.baseline).toBe(true)
    expect(s.added.map((x) => x.key)).toEqual(['smb'])
    expect(s.needLook.map((x) => x.key)).toEqual(['smb'])
  })

  it('is not a baseline when the server does not say so', () => {
    expect(summarize(report([f('port', { new: true })])).baseline).toBe(false)
    expect(summarize(report([f('port', { new: true })])).added).toHaveLength(1)
  })
})

describe('groupProblems', () => {
  const p = (key: string, over: Partial<ScanFinding>) => f(key, { check: 'smb_v1', severity: 'high', check_title: 'SMBv1 enabled', fix: 'Turn it off', ...over })

  it('makes one group per check, with the fix and title once, worst group and worst place first', () => {
    const groups = groupProblems([
      p('a', { device: 'x', severity: 'medium' }),
      p('b', { device: 'y', severity: 'high' }),
      p('c', { check: 'redis_no_auth', severity: 'critical', check_title: 'Redis with no password', fix: 'Set one' }),
    ])
    expect(groups.map((g) => [g.check, g.severity, g.items.map((i) => i.key)])).toEqual([
      ['redis_no_auth', 'critical', ['c']],
      ['smb_v1', 'high', ['b', 'a']],
    ])
    expect(groups[1]).toMatchObject({ title: 'SMBv1 enabled', fix: 'Turn it off' })
  })

  it('falls back to the evidence when a server sent no title', () => {
    const [g] = groupProblems([p('a', { check_title: undefined, evidence: 'SMBv1 is on' })])
    expect(g!.title).toBe('SMBv1 is on')
  })
})

describe('deepProblemsOf', () => {
  const NOW = Date.parse('2026-10-30T00:00:00Z')
  const deepFinding = { device: 'a', port: 22, proto: 'tcp', service: 'ssh', check: 'ssh_password_login', severity: 'medium', evidence: 'x' }
  const check = (over: Partial<DeepCheck> = {}): DeepCheck => ({ device: 'a', at: '2026-10-29T18:00:00Z', ports: [], findings: [deepFinding], ...over })
  const open22 = f('p', { port: 22 })

  it('lists a stored deep problem the report does not carry yet, dated and read-only', () => {
    const [p] = deepProblemsOf(report([open22]), [check()], NOW)
    expect(p).toMatchObject({ check: 'ssh_password_login', deep_at: '2026-10-29T18:00:00Z', accepted: false, key: 'deep:a:22:ssh_password_login' })
  })

  it('does not list one the report already carries', () => {
    expect(deepProblemsOf(report([open22, f('d', { port: 22, check: 'ssh_password_login', severity: 'medium' })]), [check()], NOW)).toEqual([])
  })

  it('does not list one whose port the scan no longer sees open, as the audit would not carry it either', () => {
    expect(deepProblemsOf(report([f('q', { port: 80 })]), [check()], NOW)).toEqual([])
  })

  it('stops listing it after 90 days', () => {
    expect(deepProblemsOf(report([open22]), [check({ at: '2026-07-01T00:00:00Z' })], NOW)).toEqual([])
  })

  it('does not list one on a device the last scan ran out of time on, since the audit would not carry it', () => {
    expect(deepProblemsOf({ ...report([open22]), unscanned: ['a'] }, [check()], NOW)).toEqual([])
  })

  it('matches the protocol too: an open udp port does not stand in for the tcp one', () => {
    expect(deepProblemsOf(report([f('u', { port: 22, proto: 'udp' })]), [check()], NOW)).toEqual([])
  })
})


describe('splitProblems', () => {
  const p = (key: string, check: string, severity: string, over: Partial<ScanFinding> = {}) => f(key, { check, severity, ...over })

  it('puts high and medium kinds, and any kind with a new or changed place, in needs-you; the rest are notes', () => {
    const groups = groupProblems([
      p('a', 'serious', 'high'), p('b', 'middling', 'medium'), p('c', 'quiet', 'low'),
      p('d', 'fresh', 'low', { new: true }), p('e', 'moved', 'low', { changed: true }),
    ])
    const { needsYou, notes } = splitProblems(groups)
    expect(needsYou.map((g) => g.check)).toEqual(['serious', 'middling', 'fresh', 'moved'])
    expect(notes.map((g) => g.check)).toEqual(['quiet'])
  })

  it('keeps one group whole on one side, so a card is never split in two', () => {
    const groups = groupProblems([p('a', 'dns', 'low'), p('b', 'dns', 'low', { new: true })])
    const { needsYou, notes } = splitProblems(groups)
    expect(needsYou).toHaveLength(1)
    expect(needsYou[0]!.items).toHaveLength(2)
    expect(notes).toEqual([])
  })
})

describe('groupProblemsByDevice', () => {
  it('lists the worst device first and each device\'s worst problem first', () => {
    const rows = groupProblemsByDevice([
      f('1', { device: 'quiet', check: 'x', severity: 'low' }),
      f('2', { device: 'loud', check: 'y', severity: 'low' }),
      f('3', { device: 'loud', check: 'z', severity: 'high' }),
    ])
    expect(rows.map((r) => r.device)).toEqual(['loud', 'quiet'])
    expect(rows[0]!.severity).toBe('high')
    expect(rows[0]!.items.map((i) => i.key)).toEqual(['3', '2'])
  })
})

describe('sharedEvidence', () => {
  it('returns the line every place says, and nothing when places differ or there is one place', () => {
    const same = [f('1', { evidence: 'same' }), f('2', { evidence: 'same' })]
    expect(sharedEvidence(same)).toBe('same')
    expect(sharedEvidence([f('1', { evidence: 'same' }), f('2', { evidence: 'other' })])).toBe('')
    expect(sharedEvidence([f('1', { evidence: 'same' })])).toBe('')
    expect(sharedEvidence([f('1'), f('2')])).toBe('')
  })
})
