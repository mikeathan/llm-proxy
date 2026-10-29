import { describe, expect, it } from 'vitest'
import { DEEP_COOLDOWN_MS, cooldownLeftMs, deepOutcome, describeDeepError, cooldownText } from '../../../utils/netaudit/deepCheck'
import type { DeepCheck } from '../../../types/netaudit'

const check = (over: Partial<DeepCheck> = {}): DeepCheck => ({
  device: '192.168.1.5', at: '2026-10-30T12:00:00Z', ports: [{ port: 22, tls_answered: false, ssh_answered: true }], findings: [], ...over,
})

describe('deepOutcome', () => {
  it('says what was found, in plain words', () => {
    const found = check({ findings: [{ device: '192.168.1.5', check: 'ssh_password_login', severity: 'medium', evidence: 'x' }] })
    expect(deepOutcome(found)).toEqual({ state: 'problems', text: '1 problem found.' })
    const two = check({ findings: [{ device: 'd', check: 'a', severity: 'low' }, { device: 'd', check: 'b', severity: 'low' }] })
    expect(deepOutcome(two).text).toBe('2 problems found.')
  })

  it('says nothing was wrong only when something was actually judged', () => {
    expect(deepOutcome(check())).toEqual({ state: 'clean', text: 'Checked the SSH service. Nothing wrong was found.' })
    const both = check({ ports: [{ port: 22, tls_answered: false, ssh_answered: true }, { port: 443, tls_answered: true, ssh_answered: false }] })
    expect(deepOutcome(both).text).toBe('Checked the SSH and TLS services. Nothing wrong was found.')
    expect(deepOutcome(check({ ports: [{ port: 443, tls_answered: true, ssh_answered: false }] })).text).toBe('Checked the TLS service. Nothing wrong was found.')
  })

  it('does not call a device with no TLS or SSH service fine: there was nothing to check', () => {
    const none = check({ ports: [{ port: 3389, tls_answered: false, ssh_answered: false }, { port: 445, tls_answered: false, ssh_answered: false }] })
    expect(deepOutcome(none)).toEqual({ state: 'nothing', text: 'No TLS or SSH service answered, so there was nothing to check. This is not the same as nothing wrong.' })
    expect(deepOutcome(check({ ports: [] })).state).toBe('nothing')
  })

  it('still reports problems when only one service answered', () => {
    const f = check({ ports: [{ port: 22, tls_answered: false, ssh_answered: true }, { port: 443, tls_answered: false, ssh_answered: false }], findings: [{ device: 'd', check: 'ssh_password_login', severity: 'medium' }] })
    expect(deepOutcome(f).state).toBe('problems')
  })
})

describe('cooldown', () => {
  it('counts down from the stored time of the last check', () => {
    const at = '2026-10-30T12:00:00Z'
    const now = Date.parse(at) + 3 * 60_000
    expect(cooldownLeftMs(at, now)).toBe(DEEP_COOLDOWN_MS - 3 * 60_000)
    expect(cooldownLeftMs(at, Date.parse(at) + DEEP_COOLDOWN_MS)).toBe(0)
    expect(cooldownLeftMs(at, Date.parse(at) + DEEP_COOLDOWN_MS + 1000)).toBe(0)
    expect(cooldownLeftMs(undefined, now)).toBe(0)
    expect(cooldownLeftMs('not a date', now)).toBe(0)
  })

  it('says how long to wait, rounding up', () => {
    expect(cooldownText(7 * 60_000)).toBe('Checked a moment ago. Try again in 7 minutes.')
    expect(cooldownText(61_000)).toBe('Checked a moment ago. Try again in 2 minutes.')
    expect(cooldownText(30_000)).toBe('Checked a moment ago. Try again in 1 minute.')
  })
})

describe('describeDeepError', () => {
  it('turns the server\'s refusals into what to do', () => {
    expect(describeDeepError(409, 'there is no open port to check on this device')).toContain('Run the audit again')
    expect(describeDeepError(409, 'another scan is already running')).toContain('Another scan')
    expect(describeDeepError(503, 'nmap is required')).toContain('nmap')
    expect(describeDeepError(504, 'x')).toContain('ran out of time')
    expect(describeDeepError(403, 'x')).toContain('Only you can start')
    expect(describeDeepError(500, 'disk full')).toBe('disk full')
  })
})
