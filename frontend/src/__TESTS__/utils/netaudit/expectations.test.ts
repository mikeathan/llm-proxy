import { describe, expect, it } from 'vitest'
import { formatPorts, openPortsOf, parsePorts } from '../../../utils/netaudit/expectations'
import type { ScanFinding } from '../../../types/netaudit'

const f = (over: Partial<ScanFinding>): ScanFinding => ({
  key: 'k', device: '10.0.0.5', port: 22, proto: 'tcp', service: 'ssh', check: 'open_port', severity: 'info', new: false, changed: false, accepted: false, ...over,
})

describe('openPortsOf', () => {
  it('lists the open TCP ports of one device, sorted and distinct, accepted ones included', () => {
    const findings = [f({ port: 631 }), f({ port: 22, accepted: true }), f({ port: 22 }), f({ port: 80, device: '10.0.0.6' }), f({ port: 445, check: 'smb_v1' }), f({ port: 53, proto: 'udp' })]
    expect(openPortsOf(findings, '10.0.0.5')).toEqual([22, 631])
    expect(openPortsOf(findings, '10.0.0.9')).toEqual([])
  })
})

describe('formatPorts', () => {
  it('joins the ports and names the empty list', () => {
    expect(formatPorts([22, 631])).toBe('22, 631')
    expect(formatPorts([])).toBe('nothing')
  })
})

describe('parsePorts', () => {
  it('reads commas and spaces, drops duplicates and sorts', () => {
    expect(parsePorts('631, 22  22,80')).toEqual([22, 80, 631])
    expect(parsePorts('  ')).toEqual([])
  })
  it('refuses anything that is not a usable port', () => {
    for (const bad of ['0', '70000', '22a', '-1', '1.5', 'http']) expect(parsePorts(bad)).toBeNull()
    expect(parsePorts(Array.from({ length: 65 }, (_, i) => i + 1).join(','))).toBeNull()
  })
})
