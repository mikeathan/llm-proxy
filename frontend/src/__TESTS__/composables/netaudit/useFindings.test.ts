import { beforeEach, describe, expect, it, vi } from 'vitest'

const service = vi.hoisted(() => ({ findings: vi.fn(), accept: vi.fn(), revoke: vi.fn(), expectations: vi.fn(), setExpectation: vi.fn(), removeExpectation: vi.fn() }))
vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: service }))
const dispatcher = vi.hoisted(() => ({ triggerAutomation: vi.fn() }))
vi.mock('../../../services/automation/dispatcherService', () => ({ DispatcherService: dispatcher }))

import { useFindings } from '../../../composables/netaudit/useFindings'

const report = { scanned: true, findings: [], resolved: [] }

describe('useFindings', () => {
  beforeEach(() => {
    for (const fn of [...Object.values(service), dispatcher.triggerAutomation]) fn.mockReset()
    service.findings.mockResolvedValue(report)
    service.expectations.mockResolvedValue([])
  })

  it('loads the report of the automation', async () => {
    const f = useFindings(() => 'ws', () => 'nightly')
    await f.load()
    expect(service.findings).toHaveBeenCalledWith('ws', 'nightly')
    expect(f.report.value).toEqual(report)
  })

  it('accepts a finding and reloads the report', async () => {
    service.accept.mockResolvedValue({})
    const f = useFindings(() => 'ws', () => 'nightly')
    expect(await f.accept('k1', 'my printer', 90)).toBe(true)
    expect(service.accept).toHaveBeenCalledWith({ workspace_id: 'ws', automation: 'nightly', key: 'k1', note: 'my printer', days: 90 })
    expect(service.findings).toHaveBeenCalledTimes(1)
  })

  it('keeps the report and shows the reason when accepting fails', async () => {
    service.accept.mockRejectedValue(new Error('no such finding'))
    const f = useFindings(() => 'ws', () => 'nightly')
    expect(await f.accept('k1', '', 30)).toBe(false)
    expect(f.actionError.value).toBe('no such finding')
    expect(service.findings).not.toHaveBeenCalled()
  })

  it('scans the audit again, or one device of it, and says the run started', async () => {
    dispatcher.triggerAutomation.mockResolvedValue({ status: 'started' })
    const f = useFindings(() => 'ws', () => 'nightly')
    expect(await f.rescan()).toBe(true)
    expect(dispatcher.triggerAutomation).toHaveBeenLastCalledWith('ws', 'nightly', undefined, undefined)
    expect(await f.rescan('10.0.0.5')).toBe(true)
    expect(dispatcher.triggerAutomation).toHaveBeenLastCalledWith('ws', 'nightly', undefined, '10.0.0.5')
    expect(f.notice.value).toMatch(/10\.0\.0\.5/)
  })

  it('says a scan is waiting when the run was queued, and shows the reason when it cannot start', async () => {
    dispatcher.triggerAutomation.mockResolvedValueOnce({ status: 'queued', position: 2 })
    const f = useFindings(() => 'ws', () => 'nightly')
    await f.rescan()
    expect(f.notice.value).toMatch(/waiting/i)

    dispatcher.triggerAutomation.mockRejectedValueOnce(new Error('a rescan takes one IP address'))
    expect(await f.rescan('x')).toBe(false)
    expect(f.actionError.value).toBe('a rescan takes one IP address')
    expect(f.notice.value).toBe('')
  })

  it('takes an acceptance back', async () => {
    service.revoke.mockResolvedValue(undefined)
    const f = useFindings(() => 'ws', () => 'nightly')
    await f.revoke('k1')
    expect(service.revoke).toHaveBeenCalledWith('ws', 'k1')
  })

  it('loads the expectations with the report and keeps the report when they cannot be read', async () => {
    const exp = { device: '10.0.0.5', ports: [631], set_at: 'x' }
    service.expectations.mockResolvedValueOnce([exp])
    const f = useFindings(() => 'ws', () => 'nightly')
    await f.load()
    expect(f.expectations.value).toEqual([exp])
    service.expectations.mockRejectedValueOnce(new Error('down'))
    await f.load()
    expect(f.expectations.value).toEqual([])
    expect(f.report.value).toEqual(report)
    expect(f.error.value).toBe('')
  })

  it('sets and removes an expectation, reloading after each, and says when it fails', async () => {
    service.setExpectation.mockResolvedValue({})
    service.removeExpectation.mockResolvedValue(undefined)
    const f = useFindings(() => 'ws', () => 'nightly')
    expect(await f.setExpected('10.0.0.5', [22])).toBe(true)
    expect(service.setExpectation).toHaveBeenCalledWith({ workspace_id: 'ws', device: '10.0.0.5', ports: [22], note: '' })
    expect(await f.removeExpected('10.0.0.5')).toBe(true)
    expect(service.removeExpectation).toHaveBeenCalledWith('ws', '10.0.0.5')
    expect(service.findings).toHaveBeenCalledTimes(2)
    service.setExpectation.mockRejectedValue(new Error('refused'))
    expect(await f.setExpected('10.0.0.5', [22])).toBe(false)
    expect(f.actionError.value).not.toBe('')
  })
})
