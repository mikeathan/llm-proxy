import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { HelperStatus, HostTraffic } from '../../../types/netaudit'

const service = vi.hoisted(() => ({ helper: vi.fn(), lastHostTraffic: vi.fn(), captureHost: vi.fn() }))
vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: service }))

import { useHostTraffic } from '../../../composables/netaudit/useHostTraffic'

const ready = (over: Partial<HelperStatus> = {}): HelperStatus => ({
  supported: true, installed: true, ready: true, declared_ranges: [], install_command: 'sudo x', help_path: 'docs',
  actions: [{ id: 'capture_host', on: true, installed: true }], ...over,
})
const summary = { at: '2026-11-09T20:00:00Z', interface: 'wlp2s0', frames: 3 } as HostTraffic

describe('useHostTraffic', () => {
  beforeEach(() => {
    for (const fn of Object.values(service)) fn.mockReset()
    service.lastHostTraffic.mockResolvedValue(null)
  })

  it('is offered when the helper is ready and the action is on and installed', async () => {
    service.helper.mockResolvedValue(ready())
    const t = useHostTraffic()
    await t.load()
    expect(t.loaded.value).toBe(true)
    expect(t.availability.value).toEqual({ ok: true, reason: '' })
  })

  it.each([
    ['this computer is not Linux', ready({ supported: false }), undefined],
    ['the helper is not installed', ready({ installed: false, ready: false }), 'settings'],
    ['the helper is not ready, and says why', ready({ ready: false, not_ready: 'the sandbox is not in force' }), 'settings'],
    ['the action is switched off', ready({ actions: [{ id: 'capture_host', on: false, installed: true }] }), 'settings'],
    ['the action is not in the helper\'s own list', ready({ actions: [{ id: 'capture_host', on: true, installed: false }] }), 'settings'],
    ['the helper does not know the action at all', ready({ actions: [] }), 'settings'],
  ])('says why it cannot be pressed when %s, and where the fix is', async (_name, status, fixTab) => {
    service.helper.mockResolvedValue(status)
    const t = useHostTraffic()
    await t.load()
    expect(t.availability.value.ok).toBe(false)
    expect(t.availability.value.reason).not.toBe('')
    expect(t.availability.value.fixTab).toBe(fixTab)
  })

  it('names the helper\'s own reason when it is not ready', async () => {
    service.helper.mockResolvedValue(ready({ ready: false, not_ready: 'the sandbox is not in force' }))
    const t = useHostTraffic()
    await t.load()
    expect(t.availability.value.reason).toContain('the sandbox is not in force')
  })

  it('says so when the helper\'s state cannot be read, and still shows the last look', async () => {
    service.helper.mockRejectedValue(new Error('down'))
    service.lastHostTraffic.mockResolvedValue(summary)
    const t = useHostTraffic()
    await t.load()
    expect(t.availability.value.ok).toBe(false)
    expect(t.availability.value.reason).toContain('could not be read')
    expect(t.summary.value).toEqual(summary)
  })

  it('keeps a failed read of the last look as an error and no summary', async () => {
    service.helper.mockResolvedValue(ready())
    service.lastHostTraffic.mockRejectedValue(new Error('disk'))
    const t = useHostTraffic()
    await t.load()
    expect(t.summary.value).toBeNull()
    expect(t.loadError.value).not.toBe('')
  })

  it('listens once, then shows the copy the server kept', async () => {
    service.helper.mockResolvedValue(ready())
    service.captureHost.mockResolvedValue({ parsed: { ...summary, frames: 1 } })
    service.lastHostTraffic.mockResolvedValue(summary)
    const t = useHostTraffic()
    await t.load()
    expect(await t.listen()).toBe(true)
    expect(service.captureHost).toHaveBeenCalledTimes(1)
    expect(t.summary.value).toEqual(summary)
    expect(t.running.value).toBe(false)
  })

  it('does not start a second look while one runs', async () => {
    service.helper.mockResolvedValue(ready())
    let finish: (v: unknown) => void = () => undefined
    service.captureHost.mockReturnValue(new Promise((resolve) => { finish = resolve }))
    const t = useHostTraffic()
    await t.load()
    const first = t.listen()
    expect(t.running.value).toBe(true)
    expect(await t.listen()).toBe(false)
    finish({ parsed: summary })
    await first
    expect(service.captureHost).toHaveBeenCalledTimes(1)
  })

  it('reports why a look gave no summary, and a failed press', async () => {
    service.helper.mockResolvedValue(ready())
    service.captureHost.mockResolvedValueOnce({ parse_error: 'the output could not be read' })
    const t = useHostTraffic()
    await t.load()
    expect(await t.listen()).toBe(false)
    expect(t.error.value).toBe('the output could not be read')
    service.captureHost.mockRejectedValueOnce(new Error('step-up required'))
    expect(await t.listen()).toBe(false)
    expect(t.error.value).not.toBe('')
  })

  it('keeps a look running across leaving and reopening the tab, and shows its result', async () => {
    service.helper.mockResolvedValue(ready())
    let finish: (v: unknown) => void = () => undefined
    service.captureHost.mockReturnValue(new Promise((resolve) => { finish = resolve }))
    service.lastHostTraffic.mockResolvedValue(null)
    const first = useHostTraffic()
    await first.load()
    const pending = first.listen()
    const reopened = useHostTraffic()
    await reopened.load()
    expect(reopened.running.value).toBe(true)
    expect(await reopened.listen()).toBe(false)
    service.lastHostTraffic.mockResolvedValue(summary)
    finish({ parsed: summary })
    await pending
    expect(reopened.running.value).toBe(false)
    expect(reopened.summary.value).toEqual(summary)
  })
})
