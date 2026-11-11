import { describe, expect, it } from 'vitest'
import { deviceLabel, isHostDevice, findingTitle, isCheck, severityRank, severityState, portCount, checkCount, lookSentence, unscannedSentence, describePort, portLabel, describeService, devicesIn, isGuessedService, isPlainText, needsLookSentence, portMeaning, seenSentence, placeSentence, shortTitle } from '../../../utils/netaudit/friendly'

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
    // No protocol is not "tcp": a port an outside database lists has none, and none is shown.
    expect(describePort({ port: 53 })).toBe('53')
    expect(portLabel({ port: 53, proto: 'udp' })).toBe('53/udp')
    expect(portLabel({})).toBe('')
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

describe('the computer the program runs on', () => {
  it('is named in words, not by the server\'s label, and is never a network address', () => {
    expect(isHostDevice('this-computer')).toBe(true)
    expect(isHostDevice('192.168.50.60')).toBe(false)
    expect(deviceLabel('this-computer')).toBe('This computer')
    expect(deviceLabel('192.168.50.60')).toBe('192.168.50.60')
  })

  it('reads as a sentence in a finding title', () => {
    const f = { device: 'this-computer', check: 'new_listener', evidence: 'Something is listening' }
    expect(findingTitle(f)).toBe('This computer: Something is listening')
  })
})

describe('placeSentence', () => {
  it('names a port and the device it is on', () => {
    expect(placeSentence({ device: '192.0.2.10', port: 22, service: 'ssh' })).toBe('Remote login (port 22) on 192.0.2.10')
  })
  it('names only the computer for a finding that has no port (the server sends port 0)', () => {
    expect(placeSentence({ device: 'this-computer', port: 0, service: '' })).toBe('This computer')
    expect(placeSentence({ device: 'this-computer', service: '' })).toBe('This computer')
  })
})

describe('shortTitle', () => {
  it('names a problem by its check and place, never by its evidence', () => {
    const f = { device: '192.0.2.10', port: 445, service: 'microsoft-ds', check: 'known_vulns', check_title: 'Software with known flaws', evidence: 'CVE-2022-45141, CVE-2023-3961 and 21 more' }
    expect(shortTitle(f)).toBe('Software with known flaws · File sharing (port 445) on 192.0.2.10')
    expect(shortTitle({ device: 'this-computer', check: 'host_updates_pending', check_title: 'Updates are waiting on this computer' })).toBe('Updates are waiting on this computer · This computer')
    expect(shortTitle({ device: '192.0.2.10', port: 22, service: 'ssh', check: 'open_port' })).toBe('Remote login (port 22) on 192.0.2.10')
  })
})

describe('footprint labels', () => {
  it('has no device actions for a domain or an email address, and says what it is about', async () => {
    const { isScannable, deviceLabel } = await import('../../../utils/netaudit/friendly')
    expect(isScannable('10.0.0.5')).toBe(true)
    expect(isScannable('this-computer')).toBe(false)
    expect(isScannable('domain:example.com')).toBe(false)
    expect(isScannable('email:a1b2c3d4')).toBe(false)
    expect(deviceLabel('domain:example.com')).toBe('Domain example.com')
    expect(deviceLabel('email:a1b2c3d4')).toBe('A watched email address')
  })
})
