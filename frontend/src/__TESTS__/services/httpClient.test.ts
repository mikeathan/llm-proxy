import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { apiFetch, del, get, post, put, setReauthHandler, setUnauthorizedHandler, signalUnauthorized } from '../../services/httpClient'

const reply = (status: number, body: unknown = {}) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })

describe('httpClient', () => {
  const fetchMock = vi.fn()
  beforeEach(() => {
    fetchMock.mockReset().mockImplementation(async () => reply(200)) // a fresh body each call
    vi.stubGlobal('fetch', fetchMock)
    setUnauthorizedHandler(null)
    setReauthHandler(null)
  })
  afterEach(() => vi.unstubAllGlobals())

  it('sends every request with the page\'s own credentials, whichever helper made it', async () => {
    await apiFetch('/admin/api/a')
    await get('/admin/api/b')
    await post('/admin/api/c', { x: 1 })
    await put('/admin/api/d', { x: 1 })
    await del('/admin/api/e')
    expect(fetchMock).toHaveBeenCalledTimes(5)
    for (const [, init] of fetchMock.mock.calls) expect(init?.credentials).toBe('same-origin')
  })

  it('keeps what the caller asked for (method, body, abort signal)', async () => {
    const signal = new AbortController().signal
    await apiFetch('/admin/api/x', { method: 'POST', body: '{}', signal, headers: { 'Content-Type': 'application/json' } })
    expect(fetchMock).toHaveBeenCalledWith('/admin/api/x', expect.objectContaining({ method: 'POST', body: '{}', signal, credentials: 'same-origin' }))
  })

  it('tells the handler once when the server says the user is not signed in, and still returns the answer', async () => {
    const handler = vi.fn()
    setUnauthorizedHandler(handler)
    fetchMock.mockResolvedValue(reply(401, { error: 'sign in' }))
    const res = await apiFetch('/admin/api/state')
    expect(res.status).toBe(401)
    expect(handler).toHaveBeenCalledTimes(1)
  })

  it('does not call the handler for other statuses, or for the sign-in calls themselves', async () => {
    const handler = vi.fn()
    setUnauthorizedHandler(handler)
    for (const status of [200, 403, 404, 500]) {
      fetchMock.mockResolvedValue(reply(status))
      await apiFetch('/admin/api/state')
    }
    fetchMock.mockResolvedValue(reply(401))
    await apiFetch('/admin/api/auth/login', { method: 'POST' })
    await apiFetch('/admin/api/auth/status')
    expect(handler).not.toHaveBeenCalled()
  })

  it('works with no handler set', async () => {
    fetchMock.mockResolvedValue(reply(401))
    await expect(get('/admin/api/state')).rejects.toThrow()
  })

  describe('a destructive action that needs the passphrase again', () => {
    const needsReauth = () => reply(403, { error: 'reauth_required' })

    it('asks for the passphrase and sends the request once more', async () => {
      const ask = vi.fn().mockResolvedValue(true)
      setReauthHandler(ask)
      fetchMock.mockImplementationOnce(async () => needsReauth()).mockImplementation(async () => reply(200, { ok: true }))
      const res = await apiFetch('/admin/api/system/wipeout', { method: 'POST', body: '{}' })
      expect(ask).toHaveBeenCalledTimes(1)
      expect(fetchMock).toHaveBeenCalledTimes(2)
      expect(res.status).toBe(200)
    })

    it('returns the refusal as it is when the user does not give the passphrase', async () => {
      setReauthHandler(vi.fn().mockResolvedValue(false))
      fetchMock.mockImplementation(async () => needsReauth())
      const res = await apiFetch('/admin/api/system/wipeout', { method: 'POST' })
      expect(res.status).toBe(403)
      expect(fetchMock).toHaveBeenCalledTimes(1)
    })

    it('tries only once, so a server that keeps refusing cannot loop', async () => {
      setReauthHandler(vi.fn().mockResolvedValue(true))
      fetchMock.mockImplementation(async () => needsReauth())
      const res = await apiFetch('/admin/api/system/wipeout', { method: 'POST' })
      expect(res.status).toBe(403)
      expect(fetchMock).toHaveBeenCalledTimes(2)
    })

    it('does not ask for any other 403, or when nothing is registered', async () => {
      const ask = vi.fn().mockResolvedValue(true)
      setReauthHandler(ask)
      fetchMock.mockImplementation(async () => reply(403, { error: 'cross-site request refused' }))
      await apiFetch('/admin/api/x', { method: 'POST' })
      expect(ask).not.toHaveBeenCalled()
      setReauthHandler(null)
      fetchMock.mockImplementation(async () => needsReauth())
      expect((await apiFetch('/admin/api/x', { method: 'POST' })).status).toBe(403)
    })

    it('sends a file body once more too: a file can be read again', async () => {
      const ask = vi.fn().mockResolvedValue(true)
      setReauthHandler(ask)
      fetchMock.mockImplementationOnce(async () => needsReauth()).mockImplementation(async () => reply(200, { ok: true }))
      const res = await apiFetch('/admin/api/netaudit/vulndata/import', { method: 'POST', body: new Blob(['x']) })
      expect(ask).toHaveBeenCalledTimes(1)
      expect(res.status).toBe(200)
    })

    it('leaves a streamed body alone: it cannot be sent twice', async () => {
      const ask = vi.fn().mockResolvedValue(true)
      setReauthHandler(ask)
      fetchMock.mockImplementation(async () => needsReauth())
      await apiFetch('/admin/api/x', { method: 'POST', body: new ReadableStream() })
      expect(ask).not.toHaveBeenCalled()
    })
  })

  it('signalUnauthorized reaches the registered handler (for a stream that cannot see a status)', () => {
    const handler = vi.fn()
    setUnauthorizedHandler(handler)
    signalUnauthorized()
    expect(handler).toHaveBeenCalledTimes(1)
  })
})
