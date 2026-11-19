import { afterEach, describe, expect, it, vi } from 'vitest'
import { McpApiService } from '../../../services/mcp/mcpService'
import type { McpServerBody } from '../../../types/mcp'

const BODY: McpServerBody = { name: 'files', url: 'https://x/mcp', trust: 'ask', auth_kind: 'none', auth_header: '', ca_cert: '', enabled: true, disabled_tools: [] }

function answer(status: number, body: unknown) {
  return vi.fn().mockImplementation(() => Promise.resolve(new Response(status === 204 ? null : JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })))
}

afterEach(() => vi.unstubAllGlobals())

describe('McpApiService', () => {
  it('reads the list from the stack\'s route', async () => {
    const f = answer(200, { enabled: true, servers: [] })
    vi.stubGlobal('fetch', f)
    expect(await McpApiService.list()).toEqual({ enabled: true, servers: [] })
    expect(f.mock.calls[0]![0]).toBe('/admin/api/mcp/servers')
  })

  it('adds with POST and changes with PUT, the name in the body', async () => {
    const f = answer(200, { name: 'files' })
    vi.stubGlobal('fetch', f)
    await McpApiService.add(BODY)
    await McpApiService.update(BODY)
    expect(f.mock.calls.map((c) => [c[0], (c[1] as RequestInit).method])).toEqual([['/admin/api/mcp/servers', 'POST'], ['/admin/api/mcp/servers', 'PUT']])
    expect(JSON.parse((f.mock.calls[1]![1] as RequestInit).body as string)).toMatchObject({ name: 'files', trust: 'ask' })
  })

  it('removes with the name in the query, encoded', async () => {
    const f = answer(204, null)
    vi.stubGlobal('fetch', f)
    await McpApiService.remove('my server')
    expect(f.mock.calls[0]![0]).toBe('/admin/api/mcp/servers?name=my%20server')
    expect((f.mock.calls[0]![1] as RequestInit).method).toBe('DELETE')
  })

  it('tests a saved server by name and an entry as typed with its value', async () => {
    const f = answer(200, { status: 'ready' })
    vi.stubGlobal('fetch', f)
    await McpApiService.test({ name: 'files' })
    await McpApiService.test({ ...BODY, secret: 's3cret' })
    expect(f.mock.calls[0]![0]).toBe('/admin/api/mcp/test')
    expect(JSON.parse((f.mock.calls[0]![1] as RequestInit).body as string)).toEqual({ name: 'files' })
    expect(JSON.parse((f.mock.calls[1]![1] as RequestInit).body as string).secret).toBe('s3cret')
  })

  it('says why a write was refused, in the server\'s words', async () => {
    vi.stubGlobal('fetch', answer(400, { error: 'a server with that name is already listed' }))
    await expect(McpApiService.add(BODY)).rejects.toThrow('a server with that name is already listed')
  })
})
