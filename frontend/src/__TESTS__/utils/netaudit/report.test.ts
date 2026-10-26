import { describe, expect, it } from 'vitest'
import { groupByDevice, summarize } from '../../../utils/netaudit/report'
import type { FindingsReport, ScanFinding } from '../../../types/netaudit'

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
