import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { HttpError } from '../../../services/httpClient'

const service = vi.hoisted(() => ({
  plan: vi.fn(), beginSession: vi.fn(), start: vi.fn(), stop: vi.fn(), listening: vi.fn(), openSessions: vi.fn(), endSession: vi.fn(),
}))
vi.mock('../../../services/radio/radioService', () => ({ RadioService: service }))

import { useRadioListening } from '../../../composables/radio/useRadioListening'

const plan = { adapter: 'wlx00', plan: { channels: [1, 6, 11, 36, 60, 149], window_seconds: 20, cycle_seconds: 300 }, max_hours: 12, windows_per_hour: 144,
  requests: [{ action: 'radio_listen', params: { iface: 'wlx00', channel: 60, seconds: 20 } }] }
const running = { listening: true, cycles: 0, frames: 0, windows: [], plan: plan.plan, until: '2026-11-14T10:00:00Z' }

beforeEach(() => {
  vi.clearAllMocks()
  service.plan.mockResolvedValue(plan)
  service.beginSession.mockResolvedValue({ id: 's1', stack: 'radio' })
  service.start.mockResolvedValue(running)
  service.endSession.mockResolvedValue(undefined)
  service.openSessions.mockResolvedValue([{ id: 's1', stack: 'radio' }, { id: 'other', stack: 'agents' }])
  service.stop.mockResolvedValue({ ...running, listening: false, ended: 'stopped' })
})
afterEach(() => vi.useRealTimers())

describe('useRadioListening', () => {
  it('opens a session for the exact list the stack gave, then starts the run', async () => {
    const l = useRadioListening()
    await l.start('wlx00', 2)
    expect(service.beginSession).toHaveBeenCalledWith({ stack: 'radio', label: 'Listening on wlx00', hours: 2, requests: plan.requests, max_uses: 300 }) // 144 windows an hour (72 sweep, 72 focus) for 2 hours, plus a margin
    expect(service.start).toHaveBeenCalledWith('wlx00', 2)
    expect(l.state.value?.listening).toBe(true)
  })

  it('takes the length from the saved plan when the caller names none', async () => {
    const l = useRadioListening()
    service.plan.mockResolvedValue({ ...plan, hours: 168 })
    await l.start('wlx00')
    expect(service.beginSession.mock.calls[0]![0].hours).toBe(168)
    expect(service.start).toHaveBeenCalledWith('wlx00', 168)
  })

  it('never asks for more uses than the core allows', async () => {
    const l = useRadioListening()
    service.plan.mockResolvedValue({ ...plan, windows_per_hour: 240 })
    await l.start('wlx00', 168)
    expect(service.beginSession.mock.calls[0]![0].max_uses).toBe(7 * 24 * 240)
  })

  it('asks for the uses a seven-day watch needs, sweeps and focus windows both, from the stack windows an hour', async () => {
    const l = useRadioListening()
    service.plan.mockResolvedValue({ ...plan, windows_per_hour: 72 })
    await l.start('wlx00', 168)
    expect(service.beginSession.mock.calls[0]![0].max_uses).toBe(12096 + 12) // 72 windows an hour for 168 hours, plus a margin
    service.plan.mockResolvedValue(plan)
    await l.start('wlx00', 168)
    expect(service.beginSession.mock.calls[1]![0].max_uses).toBe(24192 + 12) // 144 an hour: focus windows never spend a sweep's session
  })

  it('resumes a stopped watch for the hours it had left, through the same session start', async () => {
    const l = useRadioListening()
    l.state.value = { ...running, listening: false, resume: { adapter: 'wlx00', until: '2026-11-20T12:00:00Z', hours_left: 42, why: 'stack_stopped' } }
    await l.resume()
    expect(service.beginSession.mock.calls[0]![0].hours).toBe(42)
    expect(service.start).toHaveBeenCalledWith('wlx00', 42)
  })

  it('does nothing on resume when there is no offer', async () => {
    const l = useRadioListening()
    await l.resume()
    expect(service.beginSession).not.toHaveBeenCalled()
  })

  it('ends the session it opened when the run would not start', async () => {
    service.start.mockRejectedValue(new HttpError('the Radio stack is switched off: switch it on first', undefined, 409))
    const l = useRadioListening()
    await l.start('wlx00', 1)
    expect(service.endSession).toHaveBeenCalledWith('s1')
    expect(l.error.value).toBe('the Radio stack is switched off: switch it on first')
  })

  it('opens no session when the passphrase is not given', async () => {
    service.beginSession.mockRejectedValue(new HttpError('cancelled', undefined, 403))
    const l = useRadioListening()
    await l.start('wlx00', 1)
    expect(service.start).not.toHaveBeenCalled()
    expect(service.endSession).not.toHaveBeenCalled()
  })

  it('stopping ends the run and only the radio sessions', async () => {
    const l = useRadioListening()
    await l.stop()
    expect(service.stop).toHaveBeenCalled()
    expect(service.endSession).toHaveBeenCalledTimes(1)
    expect(service.endSession).toHaveBeenCalledWith('s1')
    expect(l.state.value?.ended).toBe('stopped')
  })

  it('polls only while a run is going', async () => {
    vi.useFakeTimers()
    service.listening.mockResolvedValue(running)
    const l = useRadioListening()
    await l.refresh()
    await vi.advanceTimersByTimeAsync(30_000) // every 15 s: a cycle is 5 minutes, so nothing changes faster
    expect(service.listening).toHaveBeenCalledTimes(3)
    service.listening.mockResolvedValue({ ...running, listening: false, ended: 'time' })
    await vi.advanceTimersByTimeAsync(15_000)
    const calls = service.listening.mock.calls.length
    await vi.advanceTimersByTimeAsync(60_000)
    expect(service.listening.mock.calls.length).toBe(calls)
  })
})
