import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope, nextTick, ref } from 'vue'

const service = vi.hoisted(() => ({ checkTargets: vi.fn() }))
vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: service }))

import { useTargetCheck } from '../../../composables/netaudit/useTargetCheck'

const OUTSIDE = { entry: '10.9.9.9', reason: 'It is outside the networks allowed for scanning.' }

describe('useTargetCheck', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    service.checkTargets.mockReset().mockResolvedValue([])
  })
  afterEach(() => vi.useRealTimers())

  async function typed(text: string) {
    const targets = ref('')
    const scope = effectScope()
    const check = scope.run(() => useTargetCheck(() => targets.value))!
    targets.value = text
    await nextTick()
    return { targets, check, scope }
  }

  it('asks once the typing pauses, with the entries split like the form does', async () => {
    const { check } = await typed('192.168.1.5, 10.9.9.9\n10.0.0.1')
    expect(service.checkTargets).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(500)
    expect(service.checkTargets).toHaveBeenCalledTimes(1)
    expect(service.checkTargets).toHaveBeenCalledWith(['192.168.1.5', '10.9.9.9', '10.0.0.1'])
    expect(check.problems.value).toEqual([])
  })

  it('lists what the audit would refuse', async () => {
    service.checkTargets.mockResolvedValue([OUTSIDE])
    const { check } = await typed('10.9.9.9')
    await vi.advanceTimersByTimeAsync(500)
    expect(check.problems.value).toEqual([OUTSIDE])
  })

  it('asks nothing, and shows nothing, when no network is named', async () => {
    service.checkTargets.mockResolvedValue([OUTSIDE])
    const { targets, check } = await typed('10.9.9.9')
    await vi.advanceTimersByTimeAsync(500)
    targets.value = '  '
    await nextTick()
    await vi.advanceTimersByTimeAsync(500)
    expect(service.checkTargets).toHaveBeenCalledTimes(1)
    expect(check.problems.value).toEqual([])
  })

  it('keeps the newest answer when an older one arrives late', async () => {
    let releaseFirst: (p: unknown[]) => void = () => {}
    service.checkTargets.mockImplementationOnce(() => new Promise((resolve) => { releaseFirst = resolve }))
    const { targets, check } = await typed('10.9.9.9')
    await vi.advanceTimersByTimeAsync(500)
    service.checkTargets.mockResolvedValueOnce([])
    targets.value = '192.168.1.5'
    await nextTick()
    await vi.advanceTimersByTimeAsync(500)
    releaseFirst([OUTSIDE])
    await vi.advanceTimersByTimeAsync(0)
    expect(check.problems.value).toEqual([])
  })

  it('says nothing when the check itself fails: the run explains a refusal in the end', async () => {
    service.checkTargets.mockRejectedValue(new Error('offline'))
    const { check } = await typed('10.9.9.9')
    await vi.advanceTimersByTimeAsync(500)
    expect(check.problems.value).toEqual([])
  })

  it('does not ask after its owner is gone', async () => {
    const { scope } = await typed('10.9.9.9')
    scope.stop()
    await vi.advanceTimersByTimeAsync(500)
    expect(service.checkTargets).not.toHaveBeenCalled()
  })
})
