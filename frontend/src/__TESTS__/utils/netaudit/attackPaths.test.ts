import { describe, expect, it } from 'vitest'
import type { AttackPath } from '../../../types/netaudit'
import { breaksLine, prizeText, stepText } from '../../../utils/netaudit/attackPaths'

const path = (over: Partial<AttackPath> = {}): AttackPath => ({
  id: 'network|this-computer',
  entry: { kind: 'network', text: 'Any device on your network' },
  steps: [{ key: 'k', check: 'host_ssh_password_login', title: 'SSH accepts password login', severity: 'low' }],
  prize: { kind: 'computer', where: 'this-computer' },
  fix: { key: 'k', text: 'Turn password login off.' },
  breaks: 1,
  ...over,
})

describe('prizeText', () => {
  it('names this computer and the router in plain words', () => {
    expect(prizeText(path().prize)).toBe('This computer')
    expect(prizeText({ kind: 'router', where: '192.168.1.1' })).toBe('Your router')
  })

  it('names an important device by the name the person gave it, else by its address', () => {
    const prize = { kind: 'important' as const, where: '192.168.1.10' }
    expect(prizeText({ ...prize, name: 'Nas' })).toBe('Nas')
    expect(prizeText(prize)).toBe('The device at 192.168.1.10')
  })
})

describe('stepText', () => {
  it('adds the port and marks an accepted step with a word, not a colour', () => {
    expect(stepText({ key: 'k', check: 'c', title: 'Telnet is open', port: 23, severity: 'high' })).toBe('Telnet is open (port 23)')
    expect(stepText({ key: 'k', check: 'c', title: 'Telnet is open', severity: 'high', accepted: true })).toBe('Telnet is open · accepted')
  })
})

describe('breaksLine', () => {
  it('says nothing for a fix that breaks only its own path, and counts the rest', () => {
    expect(breaksLine(1)).toBe('')
    expect(breaksLine(2)).toBe('This one fix breaks 2 of these paths.')
  })
})
