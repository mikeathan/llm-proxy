import { describe, it, expect } from 'vitest'
import {
  activeHoursInvalid,
  heartbeatEveryOptions,
  heartbeatStatusText,
  joinActiveHours,
  modelWakeNotice,
  splitActiveHours,
} from '../../../utils/automation/heartbeat'
import type { HeartbeatResult } from '../../../types/heartbeat'

describe('heartbeatEveryOptions', () => {
  it('offers the presets, with the current value kept when it is not one of them', () => {
    expect(heartbeatEveryOptions('30m').map((o) => o.value)).toEqual(['5m', '15m', '30m', '1h', '2h', '6h'])
    expect(heartbeatEveryOptions('45m').map((o) => o.value)).toEqual(['5m', '15m', '30m', '45m', '1h', '2h', '6h'])
    expect(heartbeatEveryOptions('45m').find((o) => o.value === '45m')!.label).toBe('45m')
    expect(heartbeatEveryOptions('').map((o) => o.value)).toEqual(['5m', '15m', '30m', '1h', '2h', '6h'])
  })

  it('labels the presets in words', () => {
    expect(heartbeatEveryOptions('30m').map((o) => o.label)).toEqual(['5 minutes', '15 minutes', '30 minutes', '1 hour', '2 hours', '6 hours'])
  })
})

describe('heartbeatStatusText', () => {
  it.each<[HeartbeatResult, RegExp]>([
    ['quiet', /nothing to report/i],
    ['alert', /alert/i],
    ['skipped_no_checks', /no checks/i],
    ['skipped_busy', /busy/i],
    ['skipped_outside_hours', /active hours/i],
    ['error', /failed/i],
  ])('says in words how a %s check ended', (result, words) => {
    expect(heartbeatStatusText(result)).toMatch(words)
  })
})

describe('modelWakeNotice', () => {
  it('stays silent for manual runs, which the user starts on purpose', () => {
    expect(modelWakeNotice('manual', 'local')).toBe('')
  })

  it('stays silent for a cloud connection', () => {
    expect(modelWakeNotice('interval', 'openai/main')).toBe('')
  })

  it('warns that a scheduled local model run starts the local model', () => {
    expect(modelWakeNotice('cron', 'local')).toMatch(/start the local model/i)
  })

  it('warns that the workspace default may be local and points at a cloud connection', () => {
    expect(modelWakeNotice('interval', '')).toMatch(/default/i)
    expect(modelWakeNotice('interval', '')).toMatch(/cloud/i)
  })
})

describe('active hours', () => {
  it('splits the stored window into its two times, and an unset window into two empties', () => {
    expect(splitActiveHours('08:00-22:00')).toEqual({ from: '08:00', to: '22:00' })
    expect(splitActiveHours('22:00-06:00')).toEqual({ from: '22:00', to: '06:00' })
    expect(splitActiveHours(undefined)).toEqual({ from: '', to: '' })
    expect(splitActiveHours('')).toEqual({ from: '', to: '' })
  })

  it('joins two times into the stored window, and nothing for an all-day heartbeat', () => {
    expect(joinActiveHours('08:00', '22:00')).toBe('08:00-22:00')
    expect(joinActiveHours('', '')).toBeUndefined()
  })

  it('flags a window that cannot be saved: half filled, or start equal to end', () => {
    expect(activeHoursInvalid('08:00', '')).toBe(true)
    expect(activeHoursInvalid('', '22:00')).toBe(true)
    expect(activeHoursInvalid('09:00', '09:00')).toBe(true)
    expect(activeHoursInvalid('08:00', '22:00')).toBe(false)
    expect(activeHoursInvalid('22:00', '06:00')).toBe(false)
    expect(activeHoursInvalid('', '')).toBe(false)
  })
})
