import { describe, expect, it } from 'vitest'
import { formFromScan, scanFromForm, scanSummary } from '../../../utils/netaudit/scan'

describe('scanSummary', () => {
  it('says what an audit will do in one plain line', () => {
    expect(scanSummary({ depth: 'thorough', checks: true })).toBe('Thorough scan · safe checks on · every network you allowed')
    expect(scanSummary({ depth: 'quick', checks: false, ports: '22,80', targets: ['192.168.1.0/24', '192.168.1.5'] })).toBe('Quick scan · checks off · ports 22,80 · 192.168.1.0/24, 192.168.1.5')
    expect(scanSummary(undefined)).toBe('Thorough scan · safe checks on · every network you allowed')
    expect(scanSummary({})).toBe('Thorough scan · safe checks on · every network you allowed')
  })
})

describe('scan form conversion', () => {
  it('turns what was typed into settings, and back', () => {
    const settings = scanFromForm({ scanTargets: ' 192.168.1.0/24,\n192.168.1.5 , ', scanDepth: 'quick', scanPorts: ' 22,80 ', scanChecks: false })
    expect(settings).toEqual({ targets: ['192.168.1.0/24', '192.168.1.5'], depth: 'quick', ports: '22,80', checks: false })
    expect(formFromScan(settings)).toEqual({ scanTargets: '192.168.1.0/24, 192.168.1.5', scanDepth: 'quick', scanPorts: '22,80', scanChecks: false })
  })

  it('leaves out what was left empty, and reads missing settings as the defaults', () => {
    expect(scanFromForm({ scanTargets: '', scanDepth: 'thorough', scanPorts: '', scanChecks: true })).toEqual({ depth: 'thorough', checks: true })
    expect(formFromScan(undefined)).toEqual({ scanTargets: '', scanDepth: 'thorough', scanPorts: '', scanChecks: true })
    expect(formFromScan({ depth: 'quick' })).toMatchObject({ scanDepth: 'quick', scanChecks: true })
  })
})
