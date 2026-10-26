import { beforeEach, describe, expect, it, vi } from 'vitest'

const service = vi.hoisted(() => ({ findings: vi.fn(), accept: vi.fn(), revoke: vi.fn() }))
vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: service }))

import { useFindings } from '../../../composables/netaudit/useFindings'

const report = { scanned: true, findings: [], resolved: [] }

describe('useFindings', () => {
  beforeEach(() => {
    for (const fn of Object.values(service)) fn.mockReset()
    service.findings.mockResolvedValue(report)
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

  it('takes an acceptance back', async () => {
    service.revoke.mockResolvedValue(undefined)
    const f = useFindings(() => 'ws', () => 'nightly')
    await f.revoke('k1')
    expect(service.revoke).toHaveBeenCalledWith('ws', 'k1')
  })
})
