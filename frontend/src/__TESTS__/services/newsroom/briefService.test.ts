import { describe, it, expect, vi, afterEach } from 'vitest'
import { BriefService } from '../../../services/newsroom/briefService'

afterEach(() => vi.unstubAllGlobals())

describe('BriefService deleting briefs', () => {
  it('deletes one brief, or the older ones of its edition, and says how many went', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ deleted: 2 }) })
    vi.stubGlobal('fetch', fetchMock)
    expect(await BriefService.deleteIssue(7, false)).toBe(2)
    expect(await BriefService.deleteIssue(7, true)).toBe(2)
    expect(fetchMock.mock.calls.map(([url, init]) => [init.method, url])).toEqual([
      ['DELETE', '/admin/api/newsroom/briefs/issues/7'],
      ['DELETE', '/admin/api/newsroom/briefs/issues/7/older'],
    ])
  })

  it('reports the reason the server gives', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, status: 404, text: async () => '{"error":"not found"}' }))
    await expect(BriefService.deleteIssue(9, false)).rejects.toThrow('not found')
  })
})
