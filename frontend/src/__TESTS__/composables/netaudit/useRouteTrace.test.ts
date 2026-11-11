import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { RouteTrace } from '../../../types/netaudit'

const service = vi.hoisted(() => ({ routeTraces: vi.fn(), startRouteTrace: vi.fn() }))
vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: service }))

const trace = (over: Partial<RouteTrace> = {}): RouteTrace => ({ address: '1.1.1.1', state: 'running', started_at: '2026-11-11T14:00:00Z', reached: false, hops: [], ...over })

describe('useRouteTrace', () => {
  beforeEach(() => {
    vi.resetModules()
    service.routeTraces.mockReset()
    service.startRouteTrace.mockReset()
  })
  const fresh = async () => (await import('../../../composables/netaudit/useRouteTrace')).useRouteTrace()

  it('reads the kept traces and whether this system can trace', async () => {
    service.routeTraces.mockResolvedValue({ supported: false, traces: [trace({ state: 'done' })] })
    const r = await fresh()
    await r.load()
    expect(r.supported.value).toBe(false)
    expect(r.traceOf('1.1.1.1')?.state).toBe('done')
    expect(r.traceOf('8.8.8.8')).toBeNull()
  })

  it('starts one trace, refuses a second while it runs, and reads again only while one runs', async () => {
    service.startRouteTrace.mockResolvedValue({ supported: true, traces: [trace()] })
    const r = await fresh()
    expect(await r.start('1.1.1.1')).toBe(true)
    expect(r.running.value).toBe(true)
    expect(await r.start('8.8.8.8')).toBe(false)
    expect(service.startRouteTrace).toHaveBeenCalledTimes(1)
    service.routeTraces.mockResolvedValue({ supported: true, traces: [trace({ state: 'done', reached: true })] })
    await r.refresh()
    expect(r.running.value).toBe(false)
    await r.refresh()
    expect(service.routeTraces).toHaveBeenCalledTimes(1)
  })

  it('says why a trace was refused, in the server\'s words', async () => {
    service.startRouteTrace.mockRejectedValue(new Error('internet access is off in Settings → Guardrails, so nothing was sent'))
    const r = await fresh()
    expect(await r.start('1.1.1.1')).toBe(false)
    expect(r.error.value).toContain('internet access is off')
  })

  it('says the traces could not be read, and keeps what it had', async () => {
    service.routeTraces.mockResolvedValueOnce({ supported: true, traces: [trace({ state: 'done' })] }).mockRejectedValueOnce(new Error(''))
    const r = await fresh()
    await r.load()
    await r.load()
    expect(r.error.value).toBe('The route traces could not be read.')
    expect(r.traces.value).toHaveLength(1)
  })
})
