import { describe, it, expect, vi, afterEach } from 'vitest'
import { CommandService } from '../../../services/commands/commandService'

afterEach(() => vi.unstubAllGlobals())

describe('CommandService', () => {
  it('lists the commands', async () => {
    const commands = [{ name: 'remind', usage: '/remind <when> <message>', summary: 's', examples: ['e'] }]
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, json: async () => ({ commands }) }))
    expect(await CommandService.list()).toEqual(commands)
  })

  it('runs a command with the browser zone as the fallback', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ handled: true, reply: 'Reminder 4 set' }) })
    vi.stubGlobal('fetch', fetchMock)
    const result = await CommandService.run('/remind tomorrow 9am call')
    expect(result).toEqual({ handled: true, reply: 'Reminder 4 set', failed: false })
    const [url, init] = fetchMock.mock.calls[0]!
    expect(url).toBe('/admin/api/commands/run')
    const body = JSON.parse(init.body)
    expect(body.message).toBe('/remind tomorrow 9am call')
    expect(body.timezone).toBe(Intl.DateTimeFormat().resolvedOptions().timeZone)
  })

  it('reports a refused command', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, json: async () => ({ handled: true, reply: 'nope', failed: true }) }))
    expect(await CommandService.run('/remind')).toMatchObject({ handled: true, failed: true, reply: 'nope' })
  })

  it('treats plain text as not handled', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, json: async () => ({ handled: false }) }))
    expect(await CommandService.run('hello')).toEqual({ handled: false, reply: '', failed: false })
  })

  it('previews without running', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ matched: true, command: 'remind', text: 'Tue 13 Oct' }) })
    vi.stubGlobal('fetch', fetchMock)
    expect(await CommandService.preview('/remind tomorrow x')).toMatchObject({ matched: true, text: 'Tue 13 Oct' })
    expect(fetchMock.mock.calls[0]![0]).toBe('/admin/api/commands/preview')
  })

  it('throws when the server fails', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, status: 500, text: async () => 'boom' }))
    await expect(CommandService.run('/remind x')).rejects.toThrow('500')
  })
})
