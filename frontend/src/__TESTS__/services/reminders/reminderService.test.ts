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

describe('ReminderService.edit', () => {
  it('sends the whole form to the reminder and returns the saved item', async () => {
    const saved = { id: 7, message: 'call the bank', fire_at: '2026-10-12T08:00:00Z', timezone: 'Europe/London', connector: 'telegram', channel: 'telegram', status: 'scheduled' }
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: async () => saved })
    vi.stubGlobal('fetch', fetchMock)
    const edit = { message: 'call the bank', fire_at: '2026-10-12T08:00:00.000Z', connector: 'telegram' }
    expect(await ReminderService.edit(7, edit)).toEqual(saved)
    const [url, init] = fetchMock.mock.calls[0]!
    expect(url).toBe('/admin/api/reminders/7')
    expect(init.method).toBe('PUT')
    expect(init.headers).toEqual({ 'Content-Type': 'application/json' })
    expect(JSON.parse(init.body)).toEqual(edit)
  })

  it("reports the server's reason when it refuses the edit", async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, status: 400, text: async () => '{"error":"invalid reminder: reminder time is in the past"}' }))
    await expect(ReminderService.edit(7, { message: 'x', connector: '' })).rejects.toThrow('reminder time is in the past')
  })
})

describe('ReminderService.remove', () => {
  it('deletes the reminder', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, status: 204 })
    vi.stubGlobal('fetch', fetchMock)
    await ReminderService.remove(4)
    expect(fetchMock).toHaveBeenCalledWith('/admin/api/reminders/4', { method: 'DELETE' })
  })

  it('reports a reminder that is already gone', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, status: 404, text: async () => '{"error":"reminder not found"}' }))
    await expect(ReminderService.remove(4)).rejects.toThrow('reminder not found')
  })
})

describe('ReminderService.create', () => {
  it('posts the form and returns the created item', async () => {
    const created = { id: 11, message: 'x', fire_at: '2026-10-12T08:00:00Z', timezone: 'Europe/London', connector: '', channel: 'in-app', status: 'scheduled' }
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: async () => created })
    vi.stubGlobal('fetch', fetchMock)
    const body = { message: 'x', fire_at: '2026-10-12T08:00:00.000Z', connector: '', timezone: 'Europe/London' }
    expect(await ReminderService.create(body)).toEqual(created)
    const [url, init] = fetchMock.mock.calls[0]!
    expect(url).toBe('/admin/api/reminders')
    expect(init.method).toBe('POST')
    expect(JSON.parse(init.body)).toEqual(body)
  })

  it("reports the server's reason", async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, status: 400, text: async () => '{"error":"invalid reminder: too many open reminders"}' }))
    await expect(ReminderService.create({ message: 'x', connector: '' })).rejects.toThrow('too many open reminders')
  })
})

describe.each(['pause', 'resume', 'skip'] as const)('ReminderService.%s', (action) => {
  it('posts to the reminders action and returns the item', async () => {
    const item = { id: 3, status: 'paused' }
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: async () => item })
    vi.stubGlobal('fetch', fetchMock)
    expect(await ReminderService[action](3)).toEqual(item)
    expect(fetchMock).toHaveBeenCalledWith(`/admin/api/reminders/3/${action}`, { method: 'POST' })
  })

  it('reports a refusal', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, status: 400, text: async () => '{"error":"invalid reminder: only a repeating reminder can be paused"}' }))
    await expect(ReminderService[action](3)).rejects.toThrow('only a repeating reminder')
  })
})
