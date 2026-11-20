import { describe, it, expect, vi, afterEach } from 'vitest'
import { NewsroomService } from '../../../services/newsroom/newsroomService'

afterEach(() => vi.unstubAllGlobals())

const ok = (body: unknown) => vi.fn().mockResolvedValue({ ok: true, json: async () => body })

describe('NewsroomService.feed', () => {
  it("asks for the reader's own days and sends only the filters that are set", async () => {
    const fetchMock = ok([])
    vi.stubGlobal('fetch', fetchMock)
    await NewsroomService.feed({ topic: null, unreadOnly: false }, 'Europe/London')
    expect(fetchMock.mock.calls[0]![0]).toBe('/admin/api/newsroom/feed?zone=Europe%2FLondon')
    await NewsroomService.feed({ topic: 3, unreadOnly: true }, 'UTC')
    expect(fetchMock.mock.calls[1]![0]).toBe('/admin/api/newsroom/feed?zone=UTC&topic=3&unread=true')
  })

  it('reports the reason the server gives', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, status: 400, text: async () => '{"error":"topic must be a topic id"}' }))
    await expect(NewsroomService.feed({ topic: null, unreadOnly: false }, 'UTC')).rejects.toThrow('topic must be a topic id')
  })
})

describe('NewsroomService writes', () => {
  const edit = { name: 'Go', interval: '1h', channel: '', enabled: true, sources: [{ kind: 'feed' as const, target: 'https://go.dev/blog/feed.atom' }] }

  it('creates with POST, saves with PUT, deletes and runs by id', async () => {
    const fetchMock = ok({ id: 7 })
    vi.stubGlobal('fetch', fetchMock)
    await NewsroomService.create(edit)
    await NewsroomService.save(7, edit)
    await NewsroomService.remove(7)
    await NewsroomService.run(7)
    const calls = fetchMock.mock.calls.map(([url, init]) => [init.method ?? 'GET', url])
    expect(calls).toEqual([
      ['POST', '/admin/api/newsroom/topics'],
      ['PUT', '/admin/api/newsroom/topics/7'],
      ['DELETE', '/admin/api/newsroom/topics/7'],
      ['POST', '/admin/api/newsroom/topics/7/run'],
    ])
    expect(JSON.parse(fetchMock.mock.calls[0]![1].body)).toEqual(edit)
  })

  it('marks entries read or unread by id', async () => {
    const fetchMock = ok(null)
    vi.stubGlobal('fetch', fetchMock)
    await NewsroomService.markRead([4, 5], true)
    const [url, init] = fetchMock.mock.calls[0]!
    expect(url).toBe('/admin/api/newsroom/feed/read')
    expect(JSON.parse(init.body)).toEqual({ ids: [4, 5], read: true })
  })
})
