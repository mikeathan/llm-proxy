import { describe, it, expect } from 'vitest'
import { defaultConnectorTypes, deliveryLabel, enabledConnectorTypes, withConnectorSkipped } from '../../../utils/reminders/channels'

describe('reminder channels', () => {
  it('lists each enabled connector type once', () => {
    expect(enabledConnectorTypes({
      a: { type: 'telegram', enabled: true, settings: {} },
      b: { type: 'telegram', enabled: true, settings: {} },
      c: { type: 'slack', enabled: false, settings: {} },
      d: { type: 'discord', enabled: true, settings: {} },
    })).toEqual(['telegram', 'discord'])
    expect(enabledConnectorTypes(undefined)).toEqual([])
  })

  it('the default is every enabled connector that is not skipped', () => {
    expect(defaultConnectorTypes(['telegram', 'slack'], undefined)).toEqual(['telegram', 'slack'])
    expect(defaultConnectorTypes(['telegram', 'slack'], ['slack', 'gone'])).toEqual(['telegram'])
    expect(defaultConnectorTypes([], ['slack'])).toEqual([])
  })

  it('names where a reminder goes, the web UI always included', () => {
    expect(deliveryLabel([])).toBe('Web UI')
    expect(deliveryLabel(['telegram', 'slack'])).toBe('Web UI + telegram, slack')
  })

  it('ticks a connector off and on without losing other entries or duplicating', () => {
    expect(withConnectorSkipped(undefined, 'slack', false)).toEqual(['slack'])
    expect(withConnectorSkipped(['slack'], 'slack', false)).toEqual(['slack'])
    expect(withConnectorSkipped(['slack', 'discord'], 'slack', true)).toEqual(['discord'])
  })
})
