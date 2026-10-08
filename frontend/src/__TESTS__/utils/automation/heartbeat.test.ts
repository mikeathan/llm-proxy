import { describe, it, expect } from 'vitest'
import { heartbeatEveryOptions, heartbeatStatusText, modelWakeNotice } from '../../../utils/automation/heartbeat'
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
