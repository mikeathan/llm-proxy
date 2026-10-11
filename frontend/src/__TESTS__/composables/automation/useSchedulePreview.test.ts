import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { effectScope, nextTick, ref } from 'vue'
import { flushPromises } from '@vue/test-utils'

const { previewMock } = vi.hoisted(() => ({ previewMock: vi.fn() }))

vi.mock('../../../services/automation/dispatcherService', () => ({
  DispatcherService: { previewSchedule: previewMock },
}))

import { useSchedulePreview, SCHEDULE_PREVIEW_DEBOUNCE_MS } from '../../../composables/automation/useSchedulePreview'
import type { SchedulePreview } from '../../../types/schedule'
import type { TriggerType } from '../../../types/automation'

const ok = (runs: string[] = ['2026-10-12T07:00:00+02:00']): SchedulePreview => ({ valid: true, timezone: 'CEST (UTC+02:00)', next_runs: runs })

function setup(type: TriggerType = 'cron', value = '0 7 * * *') {
  const trigger = ref({ type, value })
  const scope = effectScope()
  const preview = scope.run(() => useSchedulePreview(() => trigger.value))!
  return { trigger, preview, scope }
}

const settle = async () => {
  await vi.advanceTimersByTimeAsync(SCHEDULE_PREVIEW_DEBOUNCE_MS)
  await flushPromises()
}

describe('useSchedulePreview', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.clearAllMocks()
    previewMock.mockResolvedValue(ok())
  })
  afterEach(() => vi.useRealTimers())

  it('asks the server once typing pauses, and reports what it says', async () => {
    const { preview, trigger } = setup()
    trigger.value = { type: 'cron', value: '0 8 * * *' }
    trigger.value = { type: 'cron', value: '0 9 * * *' }
    await nextTick()
    expect(previewMock).not.toHaveBeenCalled()
    await settle()
    expect(previewMock).toHaveBeenCalledTimes(1)
    expect(previewMock).toHaveBeenCalledWith({ type: 'cron', value: '0 9 * * *' })
    expect(preview.preview.value).toEqual(ok())
    expect(preview.failed.value).toBe(false)
  })

  it('does not ask for a manual or an empty schedule', async () => {
    const { preview, trigger } = setup('manual', '')
    await settle()
    trigger.value = { type: 'cron', value: '' }
    await settle()
    expect(previewMock).not.toHaveBeenCalled()
    expect(preview.preview.value).toBeNull()
  })

  it('clears a stale answer as soon as the schedule changes', async () => {
    const { preview, trigger } = setup()
    await settle()
    expect(preview.preview.value).not.toBeNull()
    trigger.value = { type: 'manual', value: '' }
    await nextTick()
    expect(preview.preview.value).toBeNull()
  })

  it('ignores an answer that arrives after the schedule changed again', async () => {
    let resolveFirst: (p: SchedulePreview) => void = () => {}
    previewMock.mockImplementationOnce(() => new Promise<SchedulePreview>((r) => (resolveFirst = r)))
    previewMock.mockResolvedValueOnce(ok(['2026-10-13T07:00:00+02:00']))
    const { preview, trigger } = setup()
    await settle()
    trigger.value = { type: 'cron', value: '0 8 * * *' }
    await settle()
    resolveFirst(ok(['stale']))
    await flushPromises()
    expect(preview.preview.value?.next_runs).toEqual(['2026-10-13T07:00:00+02:00'])
  })

  it('says the check failed when the server cannot be reached', async () => {
    previewMock.mockRejectedValue(new Error('network'))
    const { preview } = setup()
    await settle()
    expect(preview.failed.value).toBe(true)
    expect(preview.preview.value).toBeNull()
  })

  it('does not ask after its scope is gone', async () => {
    const { trigger, scope } = setup()
    trigger.value = { type: 'cron', value: '0 8 * * *' }
    await nextTick()
    scope.stop()
    await settle()
    expect(previewMock).not.toHaveBeenCalled()
  })
})
