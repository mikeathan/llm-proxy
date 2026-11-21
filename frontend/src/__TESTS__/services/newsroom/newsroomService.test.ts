import { describe, it, expect, vi, afterEach } from 'vitest'
import { NewsroomService } from '../../../services/newsroom/newsroomService'

afterEach(() => vi.unstubAllGlobals())

const ok = (body: unknown) => vi.fn().mockResolvedValue({ ok: true, json: async () => body })

describe('NewsroomService.feed', () => {
  it("asks for the reader's own days and sends only the filters that are set", async () => {
    const fetchMock = ok([])
    vi.stubGlobal('fetch', fetchMock)
    await NewsroomService.feed({ watch: null, unreadOnly: false }, 'Europe/London')
    expect(fetchMock.mock.calls[0]![0]).toBe('/admin/api/newsroom/feed?zone=Europe%2FLondon')
    await NewsroomService.feed({ watch: 3, unreadOnly: true }, 'UTC')
    expect(fetchMock.mock.calls[1]![0]).toBe('/admin/api/newsroom/feed?zone=UTC&watch=3&unread=true')
  })

  it('reads a null answer as an empty list, not as null', async () => {
    vi.stubGlobal('fetch', ok(null))
    expect(await NewsroomService.feed({ watch: null, unreadOnly: false }, 'UTC')).toEqual([])
    expect(await NewsroomService.watches()).toEqual([])
  })

  it('reports the reason the server gives', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, status: 400, text: async () => '{"error":"watch must be a watch id"}' }))
    await expect(NewsroomService.feed({ watch: null, unreadOnly: false }, 'UTC')).rejects.toThrow('watch must be a watch id')
  })
})

describe('NewsroomService writes', () => {
  const edit = { name: 'Go', interval: '1h', channel: '', enabled: true, look_for: '', keywords: [], skip: [], sources: [{ kind: 'feed' as const, target: 'https://go.dev/blog/feed.atom' }] }

  it('creates with POST, saves with PUT, deletes and runs by id', async () => {
    const fetchMock = ok({ id: 7 })
    vi.stubGlobal('fetch', fetchMock)
    await NewsroomService.create(edit)
    await NewsroomService.save(7, edit)
    await NewsroomService.remove(7)
    await NewsroomService.run(7)
    const calls = fetchMock.mock.calls.map(([url, init]) => [init.method ?? 'GET', url])
    expect(calls).toEqual([
      ['POST', '/admin/api/newsroom/watches'],
      ['PUT', '/admin/api/newsroom/watches/7'],
      ['DELETE', '/admin/api/newsroom/watches/7'],
      ['POST', '/admin/api/newsroom/watches/7/run'],
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
