import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'

const { previewMock } = vi.hoisted(() => ({ previewMock: vi.fn() }))
vi.mock('../../../../services/automation/dispatcherService', () => ({
  DispatcherService: { previewSchedule: previewMock },
}))

import SchedulePicker from '../../../../components/AgentIde/automation/SchedulePicker.vue'
import { SCHEDULE_PREVIEW_DEBOUNCE_MS } from '../../../../composables/automation/useSchedulePreview'
import type { TriggerType } from '../../../../types/automation'

const mountPicker = (type: TriggerType, value: string) => mount(SchedulePicker, { props: { type, value } })
const lastEmit = (w: ReturnType<typeof mountPicker>, name = 'update:trigger') => {
  const events = w.emitted(name)!
  return events[events.length - 1]![0]
}

describe('SchedulePicker', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.clearAllMocks()
    previewMock.mockResolvedValue({ valid: true, timezone: 'CEST (UTC+02:00)', next_runs: ['2026-10-12T07:00:00+02:00', '2026-10-13T07:00:00+02:00'] })
  })
  afterEach(() => vi.useRealTimers())

  it.each([
    ['manual', '', 'manual'],
    ['interval', '15m', 'every'],
    ['interval', '1h30m0s', 'every'],
    ['cron', '0 7 * * *', 'daily'],
    ['cron', '0 7 * * 1-5', 'weekdays'],
    ['cron', '30 9 * * 1,5', 'weekly'],
    ['cron', '0 8 1 * *', 'monthly'],
    ['cron', '*/5 * * * *', 'custom'],
  ] as [TriggerType, string, string][])('reads %s "%s" as %s', (type, value, kind) => {
    const w = mountPicker(type, value)
    expect((w.get('[data-test="kind"]').element as HTMLSelectElement).value).toBe(kind)
  })

  it('offers no cron to someone who does not want it: presets come first, cron is last', () => {
    const labels = mountPicker('manual', '').findAll('[data-test="kind"] option').map((o) => o.text())
    expect(labels).toEqual(['Manual only', 'Every few minutes or hours', 'Every day', 'Every weekday', 'Certain days of the week', 'Once a month', 'Custom (cron)'])
  })

  it('writes the cron for a preset the user picks, keeping the time', async () => {
    const w = mountPicker('cron', '45 18 * * *')
    await w.get('[data-test="kind"]').setValue('weekdays')
    expect(lastEmit(w)).toEqual({ type: 'cron', value: '45 18 * * 1-5' })
    await w.get('[data-test="kind"]').setValue('manual')
    expect(lastEmit(w)).toEqual({ type: 'manual', value: '' })
  })

  it('writes a new time of day', async () => {
    const w = mountPicker('cron', '0 7 * * *')
    await w.get('input[type="time"]').setValue('09:15')
    expect(lastEmit(w)).toEqual({ type: 'cron', value: '15 9 * * *' })
  })

  it('writes every N minutes or hours as an interval', async () => {
    const w = mountPicker('manual', '')
    await w.get('[data-test="kind"]').setValue('every')
    expect(lastEmit(w)).toEqual({ type: 'interval', value: '1h' })
    await w.get('input[type="number"]').setValue('15')
    await w.get('[data-test="unit"]').setValue('minutes')
    expect(lastEmit(w)).toEqual({ type: 'interval', value: '15m' })
  })

  it('writes the chosen weekdays, and has no schedule until one is chosen', async () => {
    const w = mountPicker('cron', '0 9 * * 1')
    const box = (name: string) => w.findAll('input[type="checkbox"]').find((c) => c.attributes('aria-label') === name)!
    await box('Friday').setValue(true)
    expect(lastEmit(w)).toEqual({ type: 'cron', value: '0 9 * * 1,5' })
    await box('Monday').setValue(false)
    await box('Friday').setValue(false)
    expect(lastEmit(w)).toEqual({ type: 'cron', value: '' })
  })

  it('writes the day of the month', async () => {
    const w = mountPicker('cron', '0 8 1 * *')
    await w.get('[data-test="month-day"]').setValue('15')
    expect(lastEmit(w)).toEqual({ type: 'cron', value: '0 8 15 * *' })
  })

  it('keeps a hand-written cron as typed', async () => {
    const w = mountPicker('cron', '*/5 * * * *')
    expect((w.get('[data-test="custom"]').element as HTMLInputElement).value).toBe('*/5 * * * *')
    await w.get('[data-test="custom"]').setValue('*/10 * * * *')
    expect(lastEmit(w)).toEqual({ type: 'cron', value: '*/10 * * * *' })
  })

  it('keeps an interval it has no preset for as an interval, labelled as one', async () => {
    const w = mountPicker('interval', '30s')
    expect(w.text()).toContain('Interval')
    expect(w.text()).not.toContain('Cron expression')
    expect((w.get('[data-test="kind"]').element as HTMLSelectElement).value).toBe('custom')
    await w.get('[data-test="custom"]').setValue('45s')
    expect(lastEmit(w)).toEqual({ type: 'interval', value: '45s' })
  })

  it('moves a preset into the cron field as the cron it wrote', async () => {
    const w = mountPicker('cron', '0 7 * * 1-5')
    await w.get('[data-test="kind"]').setValue('custom')
    expect(lastEmit(w)).toEqual({ type: 'cron', value: '0 7 * * 1-5' })
    expect((w.get('[data-test="custom"]').element as HTMLInputElement).value).toBe('0 7 * * 1-5')
  })

  it('follows the owner when the automation being edited changes', async () => {
    const w = mountPicker('cron', '0 7 * * *')
    await w.setProps({ type: 'interval', value: '2h' })
    expect((w.get('[data-test="kind"]').element as HTMLSelectElement).value).toBe('every')
    expect((w.get('input[type="number"]').element as HTMLInputElement).value).toBe('2')
  })

  it('reads the schedule in plain English and lists the next runs in the server zone', async () => {
    const w = mountPicker('cron', '0 7 * * 1-5')
    expect(w.get('[data-test="readout"]').text()).toContain('Every weekday at 07:00')
    await vi.advanceTimersByTimeAsync(SCHEDULE_PREVIEW_DEBOUNCE_MS)
    await flushPromises()
    const text = w.get('[data-test="readout"]').text()
    expect(text).toContain('Mon 12 Oct, 07:00')
    expect(text).toContain('Tue 13 Oct, 07:00')
    expect(text).toContain('server time')
    expect(text).toContain('CEST (UTC+02:00)')
    expect(previewMock).toHaveBeenCalledWith({ type: 'cron', value: '0 7 * * 1-5' })
  })

  it('shows the server\'s reason when a schedule is not valid', async () => {
    previewMock.mockResolvedValue({ valid: false, error: 'invalid cron expression "x"', timezone: 'UTC', next_runs: [] })
    const w = mountPicker('cron', 'x')
    await vi.advanceTimersByTimeAsync(SCHEDULE_PREVIEW_DEBOUNCE_MS)
    await flushPromises()
    expect(w.get('[role="alert"]').text()).toContain('invalid cron expression')
  })

  it('tells its owner when the server says the schedule is not valid, and when it is fine again', async () => {
    previewMock.mockResolvedValueOnce({ valid: false, error: 'bad', timezone: 'UTC', next_runs: [] })
    const w = mountPicker('cron', 'x')
    await vi.advanceTimersByTimeAsync(SCHEDULE_PREVIEW_DEBOUNCE_MS)
    await flushPromises()
    expect(lastEmit(w, 'update:error')).toBe('bad')
    await w.setProps({ type: 'manual', value: '' })
    expect(lastEmit(w, 'update:error')).toBe('')
  })

  it('says so when the server cannot check the schedule', async () => {
    previewMock.mockRejectedValue(new Error('offline'))
    const w = mountPicker('cron', '0 7 * * *')
    await vi.advanceTimersByTimeAsync(SCHEDULE_PREVIEW_DEBOUNCE_MS)
    await flushPromises()
    expect(w.get('[data-test="readout"]').text()).toContain('could not be checked')
  })

  it('does not ask the server about a manual schedule', async () => {
    const w = mountPicker('manual', '')
    await vi.advanceTimersByTimeAsync(SCHEDULE_PREVIEW_DEBOUNCE_MS)
    expect(previewMock).not.toHaveBeenCalled()
    expect(w.get('[data-test="readout"]').text()).toContain('Manual only')
  })

  describe('as the reminder repeat picker', () => {
    const REPEAT_KINDS = ['daily', 'weekdays', 'weekly', 'monthly', 'custom'] as const

    it('offers only the kinds it is given', () => {
      const w = mount(SchedulePicker, { props: { type: 'cron', value: '0 9 * * 1', kinds: [...REPEAT_KINDS] } })
      expect(w.findAll('[data-test="kind"] option').map((o) => o.attributes('value'))).toEqual([...REPEAT_KINDS])
    })

    it('does not ask the server for next runs in server time when preview is off', async () => {
      const w = mount(SchedulePicker, { props: { type: 'cron', value: '0 9 * * 1', kinds: [...REPEAT_KINDS], preview: false } })
      await vi.advanceTimersByTimeAsync(SCHEDULE_PREVIEW_DEBOUNCE_MS * 2)
      await flushPromises()
      expect(previewMock).not.toHaveBeenCalled()
      expect(w.find('[data-test="readout"]').text()).toContain('Every Monday at 09:00')
      expect(w.text()).not.toContain('server time')
    })
  })
})
