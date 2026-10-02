import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { MemoryService } from '../../../services/memory/memoryService'

const json = (body: unknown, status = 200) => ({ ok: status < 400, status, json: async () => body, text: async () => JSON.stringify(body) }) as Response

describe('MemoryService', () => {
  const fetchMock = vi.fn()
  beforeEach(() => vi.stubGlobal('fetch', fetchMock.mockReset()))
  afterEach(() => vi.unstubAllGlobals())

  it('lists the always-injected facts with hot=true (the server uses the injection query)', async () => {
    fetchMock.mockResolvedValue(json([]))
    await MemoryService.list('ws', undefined, 'hot')
    expect(fetchMock).toHaveBeenCalledWith('/admin/api/memory/ws?hot=true')
  })

  it('lists the facts never sent or found with unused=true', async () => {
    fetchMock.mockResolvedValue(json([]))
    await MemoryService.list('ws', undefined, 'unused')
    expect(fetchMock).toHaveBeenCalledWith('/admin/api/memory/ws?unused=true')
  })

  it('still lists by type', async () => {
    fetchMock.mockResolvedValue(json([]))
    await MemoryService.list('ws', 'daily')
    expect(fetchMock).toHaveBeenCalledWith('/admin/api/memory/ws?type=daily')
  })

  it('creates a fact as JSON and returns the stored entry', async () => {
    fetchMock.mockResolvedValue(json({ id: 5 }, 201))
    const created = await MemoryService.create('ws', { content: 'x', scope: 'workspace', mode: 'always', keep: 'permanent' })
    const [url, init] = fetchMock.mock.calls[0]!
    expect(url).toBe('/admin/api/memory/ws')
    expect(init).toMatchObject({ method: 'POST', headers: { 'Content-Type': 'application/json' } })
    expect(JSON.parse(init.body)).toEqual({ content: 'x', scope: 'workspace', mode: 'always', keep: 'permanent' })
    expect(created.id).toBe(5)
  })

  it('surfaces the server’s reason when creating fails (e.g. a duplicate)', async () => {
    fetchMock.mockResolvedValue(json({ error: 'that fact is already saved' }, 409))
    await expect(MemoryService.create('ws', { content: 'x', scope: 'workspace', mode: 'on_demand', keep: 'permanent' }))
      .rejects.toThrow(/already saved/)
  })

  it('sends hot only when it was asked to change', async () => {
    fetchMock.mockResolvedValue(json({}))
    await MemoryService.update('ws', 1, 't', 'c')
    expect(JSON.parse(fetchMock.mock.calls[0]![1].body)).toEqual({ title: 't', content: 'c' })
    await MemoryService.update('ws', 1, 't', 'c', { hot: true })
    expect(JSON.parse(fetchMock.mock.calls[1]![1].body)).toEqual({ title: 't', content: 'c', hot: true })
    await MemoryService.update('ws', 1, 't', 'c', { priority: 2 })
    expect(JSON.parse(fetchMock.mock.calls[2]![1].body)).toEqual({ title: 't', content: 'c', priority: 2 })
  })

  it('asks for the injection preview of a model, URL-encoded', async () => {
    fetchMock.mockResolvedValue(json({}))
    await MemoryService.injectionPreview('ws', 'my model')
    expect(fetchMock).toHaveBeenCalledWith('/admin/api/memory/ws/injection-preview?model=my%20model')
    await MemoryService.injectionPreview('ws', '')
    expect(fetchMock).toHaveBeenLastCalledWith('/admin/api/memory/ws/injection-preview')
  })

  it('reads the operator notes (MEMORY.md) of a workspace', async () => {
    fetchMock.mockResolvedValue(json({ global: 'g', workspace: 'w', max_chars: 6000 }))
    const notes = await MemoryService.getNotes('ws')
    expect(fetchMock).toHaveBeenCalledWith('/admin/api/memory/ws/notes')
    expect(notes).toEqual({ global: 'g', workspace: 'w', max_chars: 6000 })
  })

  it('saves one notes file as JSON, naming the scope', async () => {
    fetchMock.mockResolvedValue(json({ status: 'saved' }))
    await MemoryService.saveNotes('ws', 'global', 'British English.')
    const [url, init] = fetchMock.mock.calls[0]!
    expect(url).toBe('/admin/api/memory/ws/notes')
    expect(init).toMatchObject({ method: 'PUT', headers: { 'Content-Type': 'application/json' } })
    expect(JSON.parse(init.body)).toEqual({ scope: 'global', content: 'British English.' })
  })

  it('surfaces the reason when the notes are too long', async () => {
    fetchMock.mockResolvedValue(json({ error: 'operator notes are too long: 7000 characters, limit 6000' }, 400))
    await expect(MemoryService.saveNotes('ws', 'workspace', 'x')).rejects.toThrow(/too long/)
  })

  it('downloads the markdown export as text', async () => {
    fetchMock.mockResolvedValue({ ok: true, status: 200, text: async () => '# Memory export — ws' } as Response)
    expect(await MemoryService.exportMarkdown('ws')).toBe('# Memory export — ws')
    expect(fetchMock).toHaveBeenCalledWith('/admin/api/memory/ws/export')
  })

  it('imports markdown as JSON and returns what happened', async () => {
    fetchMock.mockResolvedValue(json({ created: 2, skipped: 1, issues: [{ line: 7, message: 'unknown priority "urgent"' }] }))
    const result = await MemoryService.importMarkdown('ws', '### a\nb')
    const [url, init] = fetchMock.mock.calls[0]!
    expect(url).toBe('/admin/api/memory/ws/import')
    expect(init).toMatchObject({ method: 'POST', headers: { 'Content-Type': 'application/json' } })
    expect(JSON.parse(init.body)).toEqual({ markdown: '### a\nb' })
    expect(result).toEqual({ created: 2, skipped: 1, issues: [{ line: 7, message: 'unknown priority "urgent"' }] })
  })

  it('surfaces the reason when an import is refused', async () => {
    fetchMock.mockResolvedValue(json({ error: 'no facts found: each fact starts with a "### Title" heading' }, 400))
    await expect(MemoryService.importMarkdown('ws', 'prose')).rejects.toThrow(/no facts found/)
  })
})
