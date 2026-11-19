import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
import { bodyFromForm, bodyFromServer, NEW_ENTRY, useMcpServers } from '../../../composables/agents/useMcpServers'
import { McpApiService } from '../../../services/mcp/mcpService'
import { blankMcpForm } from '../../../constants/mcp'
import type { McpServer, McpServersAnswer } from '../../../types/mcp'

vi.mock('../../../services/mcp/mcpService', () => ({
  McpApiService: { list: vi.fn(), add: vi.fn(), update: vi.fn(), remove: vi.fn(), test: vi.fn() },
}))
const api = vi.mocked(McpApiService)

const server = (over: Partial<McpServer> = {}): McpServer => ({
  name: 'files', url: 'https://x/mcp', enabled: true, trust: 'ask', auth_kind: 'bearer', has_secret: true, status: 'ready', tools: [], disabled_tools: ['rm'], ...over,
})
const answerOf = (servers: McpServer[], enabled = true): McpServersAnswer => ({ enabled, servers })

let state!: ReturnType<typeof useMcpServers>
const mounted: VueWrapper[] = []
// The data is shared by every consumer, so each test unmounts its own: the last one to go drops it.
function mountIt() {
  const w = mount(defineComponent({ setup() { state = useMcpServers(); return () => h('div') } }))
  mounted.push(w)
  return w
}

beforeEach(() => {
  vi.useFakeTimers()
  vi.clearAllMocks()
  api.list.mockResolvedValue(answerOf([server()]))
})
afterEach(() => {
  for (const w of mounted.splice(0)) w.unmount()
  vi.useRealTimers()
})

describe('useMcpServers', () => {
  it('reads the list on mount and drops it when the last consumer goes', async () => {
    const w = mountIt()
    await flushPromises()
    expect(state.answer.value?.servers.map((s) => s.name)).toEqual(['files'])
    expect(state.loading.value).toBe(false)
    w.unmount()
    expect(state.answer.value).toBeNull()
  })

  it('says why the list could not be read', async () => {
    api.list.mockRejectedValue(new Error('boom'))
    mountIt()
    await flushPromises()
    expect(state.error.value).toBe('boom')
  })

  it('reads faster while a server is connecting and slower once it settles', async () => {
    api.list.mockResolvedValue(answerOf([server({ status: 'connecting' })]))
    const w = mountIt()
    await flushPromises()
    expect(api.list).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(2100)
    expect(api.list).toHaveBeenCalledTimes(2)
    api.list.mockResolvedValue(answerOf([server()]))
    await vi.advanceTimersByTimeAsync(2100)
    expect(api.list).toHaveBeenCalledTimes(3)
    await vi.advanceTimersByTimeAsync(5000)
    expect(api.list).toHaveBeenCalledTimes(3)
    await vi.advanceTimersByTimeAsync(5100)
    expect(api.list).toHaveBeenCalledTimes(4)
    w.unmount()
    await vi.advanceTimersByTimeAsync(30000)
    expect(api.list).toHaveBeenCalledTimes(4)
  })

  it('a card change sends the whole entry with the tools already switched off, and no sign-in value', async () => {
    api.update.mockResolvedValue(server())
    mountIt()
    await flushPromises()
    expect(await state.setTrust(server(), 'allow')).toBe(true)
    expect(api.update).toHaveBeenCalledWith({
      name: 'files', url: 'https://x/mcp', enabled: true, trust: 'allow', auth_kind: 'bearer', auth_header: '', ca_cert: '', disabled_tools: ['rm'],
    })
    await state.setToolOff(server(), 'ls', true)
    expect(api.update).toHaveBeenLastCalledWith(expect.objectContaining({ disabled_tools: ['rm', 'ls'] }))
    await state.setToolOff(server(), 'rm', false)
    expect(api.update).toHaveBeenLastCalledWith(expect.objectContaining({ disabled_tools: [] }))
    await state.setEnabled(server(), false)
    expect(api.update).toHaveBeenLastCalledWith(expect.objectContaining({ enabled: false }))
  })

  it('keeps the tools switched off while the server is down and lists none', () => {
    const down = server({ status: 'stopped', tools: [], disabled_tools: ['rm', 'ls'] })
    expect(bodyFromServer(down).disabled_tools).toEqual(['rm', 'ls'])
  })

  it('sends the typed value only when there is one', () => {
    const form = { ...blankMcpForm(), name: 'files', url: 'https://x/mcp', auth_kind: 'bearer' as const }
    expect('secret' in bodyFromForm(form)).toBe(false)
    expect(bodyFromForm({ ...form, secret: 'tok' }).secret).toBe('tok')
    expect(bodyFromForm(form, server({ enabled: false })).enabled).toBe(false)
    expect(bodyFromForm(form).enabled).toBe(true)
  })

  it('reports a refused change in words and leaves the list as it was', async () => {
    api.add.mockRejectedValue(new Error('a server with that name is already listed'))
    mountIt()
    await flushPromises()
    expect(await state.add({ ...blankMcpForm(), name: 'files', url: 'https://x/mcp' })).toBe(false)
    expect(state.actionError.value).toBe('a server with that name is already listed')
    expect(state.saving.value).toBe(false)
  })

  it('tests a saved server by name and the form\'s entry under its own key, and clears them', async () => {
    api.test.mockResolvedValue(server({ status: 'stopped', problem_code: 'sign_in' }))
    mountIt()
    await flushPromises()
    await state.testSaved('files')
    expect(api.test).toHaveBeenLastCalledWith({ name: 'files' })
    expect(state.tests.value['files']?.problem_code).toBe('sign_in')
    await state.testForm({ ...blankMcpForm(), name: 'new', url: 'https://y/mcp', secret: 'v' })
    expect(api.test).toHaveBeenLastCalledWith(expect.objectContaining({ url: 'https://y/mcp', secret: 'v', enabled: true }))
    expect(state.tests.value[NEW_ENTRY]).toBeDefined()
    state.clearTest(NEW_ENTRY)
    expect(state.tests.value[NEW_ENTRY]).toBeUndefined()
    expect(state.tests.value['files']).toBeDefined()
  })

  it('a failed test is said in the page, not thrown', async () => {
    api.test.mockRejectedValue(new Error('the request is too large'))
    mountIt()
    await flushPromises()
    await state.testSaved('files')
    expect(state.actionError.value).toBe('the request is too large')
    expect(state.testing.value).toBe('')
  })
})
