import { describe, it, expect } from 'vitest'
import { busyLabel, deliveryLabel, notifyFromForm, parseDedupDays } from '../../../utils/automation/delivery'

const form = (over = {}) => ({ notifyConnector: 'my-telegram', notifyDedup: false, notifyDedupDays: '', notifySendEmpty: false, ...over })

describe('parseDedupDays', () => {
  it.each([
    ['', undefined],
    ['  ', undefined],
    ['30', 30],
    [' 7 ', 7],
    ['0', null],
    ['-3', null],
    ['2.5', null],
    ['abc', null],
    ['3650', 3650],
    ['3651', null],
  ])('%j → %j', (text, want) => {
    expect(parseDedupDays(text)).toBe(want)
  })
})

describe('notifyFromForm', () => {
  it('is null when no connector is chosen, so an update clears delivery', () => {
    expect(notifyFromForm(form({ notifyConnector: '' }))).toBeNull()
  })

  it('sends explicit flags so an edit can turn options off', () => {
    expect(notifyFromForm(form())).toEqual({ connector: 'my-telegram', dedup: false, dedup_days: 0, send_empty: false })
  })

  it('carries a valid retention only while dedup is on', () => {
    expect(notifyFromForm(form({ notifyDedup: true, notifyDedupDays: '30' }))?.dedup_days).toBe(30)
    expect(notifyFromForm(form({ notifyDedup: false, notifyDedupDays: '30' }))?.dedup_days).toBe(0)
    expect(notifyFromForm(form({ notifyDedup: true, notifyDedupDays: '' }))?.dedup_days).toBe(0)
  })
})

describe('labels', () => {
  it('describes delivery', () => {
    expect(deliveryLabel(undefined)).toBe('Not sent')
    expect(deliveryLabel(null)).toBe('Not sent')
    expect(deliveryLabel({ connector: 'tg' })).toBe('tg')
    expect(deliveryLabel({ connector: 'tg', dedup: true, send_empty: true })).toBe('tg · skips repeats · reports quiet runs')
  })

  it('describes what happens when the model is busy', () => {
    expect(busyLabel(false)).toBe('Waits its turn')
    expect(busyLabel(undefined)).toBe('Waits its turn')
    expect(busyLabel(true)).toBe('Skips the run')
  })
})
