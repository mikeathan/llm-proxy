import { describe, expect, it } from 'vitest'
import type { StackModule } from '../../../types/stacks'
import { layerNumber, moduleStatus } from '../../../utils/stacks/modules'

const mod = (over: Partial<StackModule> = {}): StackModule => ({ id: 'm', title: 'M', purpose: '', sends: 'lan', step_up: false, enabled: false, needs: [], ...over })

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
