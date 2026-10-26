import { describe, expect, it } from 'vitest'
import { findingTitle, isCheck, severityRank, severityState, portCount, checkCount, lookSentence, unscannedSentence, describePort, describeService, devicesIn, isGuessedService, isPlainText, needsLookSentence, portMeaning, seenSentence } from '../../../utils/netaudit/friendly'

describe('friendly wording', () => {
  it('says what a port is usually for, then falls back to the service, then the number', () => {
    expect(portMeaning({ port: 22, service: 'ssh' })).toBe('Remote login')
    expect(portMeaning({ port: 9999, service: 'foo (guess)' })).toBe('foo')
    expect(portMeaning({ port: 9999 })).toBe('Port 9999')
    expect(portMeaning({})).toBe('Something')
  })

  it('adds the port number to the meaning only when the meaning is not just the number', () => {
    expect(describeService({ port: 443 })).toBe('Secure web pages (port 443)')
    expect(describeService({ port: 9999 })).toBe('Port 9999')
  })

  it('writes the technical form for the Advanced view', () => {
    expect(describePort({ port: 22, proto: 'tcp', service: 'ssh' })).toBe('22/tcp ssh')
    expect(describePort({ port: 53 })).toBe('53/tcp')
    expect(describePort({})).toBe('')
  })

  it('flags guessed services and unencrypted ports', () => {
    expect(isGuessedService('http (guess)')).toBe(true)
    expect(isGuessedService('http')).toBe(false)
    expect(isPlainText({ port: 23 })).toBe(true)
    expect(isPlainText({ port: 22 })).toBe(false)
  })

  it('counts in plain sentences', () => {
    expect(needsLookSentence(0)).toBe('Nothing needs a look')
    expect(needsLookSentence(1)).toBe('1 thing needs a look')
    expect(needsLookSentence(4)).toBe('4 things need a look')
    expect(seenSentence(1, 1)).toBe('1 service on 1 device')
    expect(devicesIn([{ device: 'a' }, { device: 'a' }, { device: 'b' }])).toBe(2)
    expect(seenSentence(2, 3)).toBe('3 services on 2 devices')
  })

  it('says when only the version moved, and counts devices not scanned to completion', () => {
    expect(lookSentence({ device: 'a', port: 22, new: true, version: '9.7p1' })).toBe('Remote login (port 22) on a')
    expect(lookSentence({ device: 'a', port: 22, new: false, version: '9.7p1' })).toBe('Remote login (port 22) on a changed to 9.7p1')
    expect(unscannedSentence(1)).toBe('1 device was not scanned to completion')
    expect(unscannedSentence(3)).toBe('3 devices were not scanned to completion')
  })

  it('tells a check from an open port, and words a check as the device then what was found', () => {
    const port = { device: '10.0.0.1', port: 22, check: 'open_port', new: true, version: '' }
    const check = { device: '10.0.0.1', port: 445, check: 'smb_v1', evidence: 'SMBv1 is enabled', new: true, version: '' }
    expect(isCheck(port)).toBe(false)
    expect(isCheck(check)).toBe(true)
    expect(findingTitle(check)).toBe('10.0.0.1: SMBv1 is enabled')
    expect(findingTitle(port)).toBe('Remote login (port 22) on 10.0.0.1')
    expect(findingTitle({ device: 'a', port: 1, check: 'x' })).toBe('a: A check found a problem')
    expect(lookSentence(check)).toBe('10.0.0.1: SMBv1 is enabled')
    expect(portCount([port, check])).toBe(1)
    expect(checkCount([port, check])).toBe(1)
    expect(seenSentence(2, 3, 0)).toBe('3 services on 2 devices')
    expect(seenSentence(2, 3, 2)).toBe('3 services on 2 devices, 2 problems found by checks')
    expect(seenSentence(1, 1, 1)).toBe('1 service on 1 device, 1 problem found by checks')
  })

  it('ranks severities worst first and shows them in the tags', () => {
    expect(['low', 'critical', 'info', 'high', 'medium', 'weird'].sort((a, b) => severityRank(b) - severityRank(a))).toEqual(['critical', 'high', 'medium', 'low', 'info', 'weird'])
    expect(severityState('critical')).toBe('error')
    expect(severityState('high')).toBe('error')
    expect(severityState('medium')).toBe('warning')
    expect(severityState('low')).toBe('warning')
    expect(severityState('info')).toBe('neutral')
  })
})
