import { describe, expect, it } from 'vitest'
import type { StackModule } from '../../../types/stacks'
import { layerNumber, moduleStatus } from '../../../utils/stacks/modules'

const mod = (over: Partial<StackModule> = {}): StackModule => ({ id: 'm', title: 'M', purpose: '', about: { watches: 'w.', never: 'n.', shows_up: 's.' }, finds: [], sends: 'lan', step_up: false, enabled: false, needs: [], ...over })

describe('moduleStatus', () => {
  it('says Off with what it would send', () => {
    expect(moduleStatus(mod())).toEqual({ text: 'Off · LAN', tone: 'muted' })
  })

  it('names the module a switched-off one needs', () => {
    expect(moduleStatus(mod({ sends: 'message', blocked: 'needs Presence check' }))).toEqual({ text: 'Off · sends a message · needs Presence check', tone: 'muted' })
  })

  it('says Working in green and Stopped in amber, only for a module that is on', () => {
    expect(moduleStatus(mod({ enabled: true, health: 'working', sends: 'listens' }))).toEqual({ text: 'Working · listens', tone: 'success' })
    expect(moduleStatus(mod({ enabled: true, health: 'stopped' }))).toEqual({ text: 'Stopped · LAN', tone: 'warning' })
  })

  it('says On when the module is on but has no monitor to tell', () => {
    expect(moduleStatus(mod({ enabled: true, sends: 'nothing' }))).toEqual({ text: 'On · sends nothing', tone: 'success' })
  })

  it('shows what blocks a module that is on, and does not call it working', () => {
    expect(moduleStatus(mod({ enabled: true, health: 'working', blocked: 'local network access is off in Guardrails' }))).toEqual({
      text: 'Blocked · LAN · local network access is off in Guardrails',
      tone: 'warning',
    })
  })
})

describe('layerNumber', () => {
  it('counts layers from the bottom, which is drawn last', () => {
    expect([0, 1, 2].map((i) => layerNumber(i, 3))).toEqual(['03', '02', '01'])
  })
})

describe('moduleStatus: when it last ran and what is wrong', () => {
  const NOW = Date.parse('2026-11-12T12:00:00Z')

  it('says when a module that is on last ran', () => {
    expect(moduleStatus(mod({ enabled: true, sends: 'message', last_run: '2026-11-12T09:00:00Z' }), NOW)).toEqual({ text: 'On · sends a message · last ran 3 hours ago', tone: 'success' })
  })

  it('says nothing about a run that has not happened, or a time the server sends for never', () => {
    expect(moduleStatus(mod({ enabled: true, sends: 'nothing' }), NOW).text).toBe('On · sends nothing')
    expect(moduleStatus(mod({ enabled: true, sends: 'nothing', last_run: '0001-01-01T00:00:00Z' }), NOW).text).toBe('On · sends nothing')
  })

  it('shows the stack\'s own words for a problem, in amber, and still says when it last ran', () => {
    const got = moduleStatus(mod({ enabled: true, sends: 'message', last_run: '2026-11-12T09:00:00Z', problem: 'the queue was full and some events were not recorded' }), NOW)
    expect(got).toEqual({ text: 'On · sends a message · last ran 3 hours ago · the queue was full and some events were not recorded', tone: 'warning' })
  })

  it('shows a problem under Working too, and never for a module that is off', () => {
    expect(moduleStatus(mod({ enabled: true, health: 'working', sends: 'nothing', problem: 'p.' }), NOW)).toEqual({ text: 'Working · sends nothing · p.', tone: 'warning' })
    expect(moduleStatus(mod({ enabled: false, problem: 'p.', last_run: '2026-11-12T09:00:00Z' }), NOW).text).toBe('Off · LAN')
  })
})
