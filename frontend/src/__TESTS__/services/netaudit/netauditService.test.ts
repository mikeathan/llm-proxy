import { afterEach, describe, expect, it, vi } from 'vitest'
import { NetauditService } from '../../../services/netaudit/netauditService'

function respond(body: unknown, status = 200) {
  const fetchMock = vi.fn().mockResolvedValue({ ok: status < 400, status, statusText: 'x', json: async () => body })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

afterEach(() => vi.unstubAllGlobals())

describe('NetauditService', () => {
  it('reads findings for one automation', async () => {
    const fetchMock = respond({ scanned: false, findings: [], resolved: [] })
    await NetauditService.findings('ws 1', 'audit')
    expect(fetchMock.mock.calls[0]![0]).toBe('/admin/api/netaudit/findings?workspace_id=ws+1&automation=audit')
  })

  it('sends only the scope fields it was given', async () => {
    const fetchMock = respond({ ranges: [], allow_unconfined: false, suggestions: [] })
    await NetauditService.updateScope({ allow_unconfined: true })
    const init = fetchMock.mock.calls[0]![1] as RequestInit
    expect(init.method).toBe('PUT')
    expect(init.body).toBe('{"allow_unconfined":true}')
  })

  it('accepts and revokes by key', async () => {
    const accept = respond({ key: 'k' })
    await NetauditService.accept({ workspace_id: 'w', automation: 'a', key: 'k', note: '', days: 90 })
    expect((accept.mock.calls[0]![1] as RequestInit).method).toBe('POST')
    const revoke = respond({}, 204)
    await NetauditService.revoke('w', 'a|b')
    expect(revoke.mock.calls[0]![0]).toContain('key=a%7Cb')
    expect((revoke.mock.calls[0]![1] as RequestInit).method).toBe('DELETE')
  })

  it('surfaces the server refusal as the error message', async () => {
    respond({ error: 'public address refused' }, 400)
    await expect(NetauditService.updateScope({ entries: ['8.8.8.8'] })).rejects.toThrow('public address refused')
  })
})
