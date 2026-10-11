import { describe, it, expect, vi, afterEach } from 'vitest'
import { ReminderService } from '../../../services/reminders/reminderService'

afterEach(() => vi.unstubAllGlobals())

describe('ReminderService.list', () => {
  it('reads open and recent reminders', async () => {
    const body = { open: [{ id: 1, message: 'call bank', fire_at: '2026-10-12T08:00:00Z', timezone: 'Europe/London', channel: 'telegram', status: 'scheduled' }], recent: [] }
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: async () => body })
    vi.stubGlobal('fetch', fetchMock)
    expect(await ReminderService.list()).toEqual(body)
    expect(fetchMock).toHaveBeenCalledWith('/admin/api/reminders')
  })

  it('reports the reason on failure', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, status: 500, text: async () => '{"error":"failed to list reminders"}' }))
    await expect(ReminderService.list()).rejects.toThrow('failed to list reminders')
  })
})
