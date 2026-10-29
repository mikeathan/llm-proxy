import { beforeEach, describe, expect, it, vi } from 'vitest'
import { HttpError } from '../../../services/httpClient'

const service = vi.hoisted(() => ({ deepChecks: vi.fn(), runDeepCheck: vi.fn() }))
vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: service }))

import { useDeepCheck } from '../../../composables/netaudit/useDeepCheck'
import type { DeepCheck } from '../../../types/netaudit'

const check = (device: string, at: string, findings: DeepCheck['findings'] = []): DeepCheck => ({
  device, at, ports: [{ port: 22, tls_answered: false, ssh_answered: true }], findings,
})

describe('useDeepCheck', () => {
  beforeEach(() => {
    for (const fn of Object.values(service)) fn.mockReset()
    service.deepChecks.mockResolvedValue([])
  })

  it('loads the stored checks and finds the last one of a device', async () => {
    service.deepChecks.mockResolvedValue([check('10.0.0.5', '2026-10-30T12:00:00Z'), check('10.0.0.6', '2026-10-29T12:00:00Z')])
    const d = useDeepCheck(() => 'ws', () => 'nightly')
    await d.load()
    expect(service.deepChecks).toHaveBeenCalledWith('ws')
    expect(d.lastOf('10.0.0.5')?.at).toBe('2026-10-30T12:00:00Z')
    expect(d.lastOf('10.0.0.9')).toBeUndefined()
  })

  it('a failure to read them leaves none and never hides the report', async () => {
    service.deepChecks.mockRejectedValue(new Error('boom'))
    const d = useDeepCheck(() => 'ws', () => 'nightly')
    await d.load()
    expect(d.checks.value).toEqual([])
  })

  it('runs one device, keeps the result, and shows the device as running meanwhile', async () => {
    let finish: (c: DeepCheck) => void = () => undefined
    service.runDeepCheck.mockReturnValue(new Promise<DeepCheck>((resolve) => { finish = resolve }))
    const d = useDeepCheck(() => 'ws', () => 'nightly')
    const pending = d.run('10.0.0.5')
    expect(d.running.value).toBe('10.0.0.5')
    expect(service.runDeepCheck).toHaveBeenCalledWith('ws', 'nightly', '10.0.0.5')
    finish(check('10.0.0.5', '2026-10-30T12:00:00Z'))
    expect(await pending).toBe(true)
    expect(d.running.value).toBe('')
    expect(d.lastOf('10.0.0.5')?.at).toBe('2026-10-30T12:00:00Z')
    expect(d.error.value).toBe('')
  })

  it('runs only one at a time', async () => {
    service.runDeepCheck.mockReturnValue(new Promise(() => undefined))
    const d = useDeepCheck(() => 'ws', () => 'nightly')
    void d.run('10.0.0.5')
    expect(await d.run('10.0.0.6')).toBe(false)
    expect(service.runDeepCheck).toHaveBeenCalledTimes(1)
  })

  it('says what a refusal means and keeps the earlier result', async () => {
    service.deepChecks.mockResolvedValue([check('10.0.0.5', '2026-10-29T12:00:00Z')])
    const d = useDeepCheck(() => 'ws', () => 'nightly')
    await d.load()
    service.runDeepCheck.mockRejectedValue(new HttpError('there is no open port to check on this device', '', 409))
    expect(await d.run('10.0.0.5')).toBe(false)
    expect(d.error.value).toContain('Run the audit again')
    expect(d.running.value).toBe('')
    expect(d.lastOf('10.0.0.5')?.at).toBe('2026-10-29T12:00:00Z')
  })

  it('a cool-down answer is shown with the time left, from the server, and the device stays clickable later', async () => {
    service.runDeepCheck.mockRejectedValue(new HttpError('this device was checked a moment ago; try again in 5 minute(s)', '', 429))
    const d = useDeepCheck(() => 'ws', () => 'nightly')
    await d.run('10.0.0.5')
    expect(d.error.value).toContain('checked a moment ago')
  })

  it('a refusal to confirm the passphrase is an error, not a result', async () => {
    service.runDeepCheck.mockRejectedValue(new HttpError('reauth_required', '', 403))
    const d = useDeepCheck(() => 'ws', () => 'nightly')
    expect(await d.run('10.0.0.5')).toBe(false)
    expect(d.checks.value).toEqual([])
  })
})
