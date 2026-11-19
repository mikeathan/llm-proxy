import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { ref } from 'vue'
import AgentTools from '../../../components/agents/AgentTools.vue'
import { McpApiService } from '../../../services/mcp/mcpService'
import type { McpServer, McpServersAnswer } from '../../../types/mcp'

vi.mock('../../../services/mcp/mcpService', () => ({
  McpApiService: { list: vi.fn(), add: vi.fn(), update: vi.fn(), remove: vi.fn(), test: vi.fn() },
}))
const toast = { success: vi.fn(), error: vi.fn(), info: vi.fn(), show: vi.fn() }
vi.mock('../../../composables/useToast', () => ({ useToast: () => toast }))
const confirm = vi.fn()
vi.mock('../../../composables/ui/useConfirm', () => ({ useConfirm: () => ({ confirm, state: ref(null) }) }))
const api = vi.mocked(McpApiService)

const server = (over: Partial<McpServer> = {}): McpServer => ({
  name: 'files', url: 'https://x/mcp', enabled: true, trust: 'ask', auth_kind: 'none', has_secret: false, status: 'ready', server_name: 'fixture', transport: 'streamable_http',
  tools: [{ name: 'read_text' }], disabled_tools: [], ...over,
})
const answerOf = (servers: McpServer[], enabled = true): McpServersAnswer => ({ enabled, servers })

const mounted: VueWrapper[] = []
async function mountTab() {
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/agents/:tab?', name: 'agents', component: { template: '<div />' } }] })
  await router.push('/agents/tools')
  const w = mount(AgentTools, { global: { plugins: [router], stubs: { Icon: true, BrandMark: true } } })
  mounted.push(w)
  await flushPromises()
  return w
}

beforeEach(() => {
  vi.clearAllMocks()
  api.list.mockResolvedValue(answerOf([server()]))
})
afterEach(() => { for (const w of mounted.splice(0)) w.unmount() })

describe('AgentTools', () => {
  it('lists the servers with the one-line explanation above', async () => {
    const w = await mountTab()
    expect(w.text()).toContain('Each tool asks you first until you set its server to Allow')
    expect(w.find('[data-test="mcp-server-files"]').exists()).toBe(true)
  })

  it('says there are none, with the one thing to do, and opens the form from it', async () => {
    api.list.mockResolvedValue(answerOf([]))
    const w = await mountTab()
    expect(w.text()).toContain('No MCP servers')
    await w.get('[data-test="mcp-add-empty"]').trigger('click')
    expect(w.find('[data-test="mcp-form"]').exists()).toBe(true)
    expect(w.text()).not.toContain('No MCP servers')
  })

  it('says why the list is missing, with the cause and a next step', async () => {
    api.list.mockRejectedValue(new Error('the stack is not running'))
    const w = await mountTab()
    expect(w.get('[role="alert"]').text()).toContain('MCP servers unavailable')
    expect(w.text()).toContain('the stack is not running')
    expect(w.text()).toContain('Reload the page')
  })

  it('says when the module is off and links to where it is switched on', async () => {
    api.list.mockResolvedValue(answerOf([server({ status: 'off' })], false))
    const w = await mountTab()
    const off = w.get('[data-test="mcp-off"]')
    expect(off.text()).toContain('MCP servers are switched off')
    expect(off.get('a').attributes('href')).toBe('/agents/modules')
  })

  it('adds a server, tells the person, and closes the form', async () => {
    api.list.mockResolvedValue(answerOf([]))
    api.add.mockResolvedValue(server())
    const w = await mountTab()
    await w.get('[data-test="mcp-add-empty"]').trigger('click')
    await w.get('[data-test="mcp-name"]').setValue('files')
    await w.get('[data-test="mcp-url"]').setValue('https://x/mcp')
    api.list.mockResolvedValue(answerOf([server()]))
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(api.add).toHaveBeenCalledWith(expect.objectContaining({ name: 'files', url: 'https://x/mcp', trust: 'ask', enabled: true }))
    expect(toast.success).toHaveBeenCalledWith('files added')
    expect(w.find('[data-test="mcp-form"]').exists()).toBe(false)
  })

  it('keeps the form open and says why when the server refuses it', async () => {
    api.list.mockResolvedValue(answerOf([]))
    api.add.mockRejectedValue(new Error('a server with that name is already listed'))
    const w = await mountTab()
    await w.get('[data-test="mcp-add-empty"]').trigger('click')
    await w.get('[data-test="mcp-name"]').setValue('files')
    await w.get('[data-test="mcp-url"]').setValue('https://x/mcp')
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(w.get('[data-test="mcp-form-error"]').text()).toContain('already listed')
    expect(toast.success).not.toHaveBeenCalled()
  })

  it('edits a server in place, filled in without the sign-in value', async () => {
    api.list.mockResolvedValue(answerOf([server({ auth_kind: 'bearer', has_secret: true })]))
    api.update.mockResolvedValue(server())
    const w = await mountTab()
    await w.get('[data-test="mcp-edit"]').trigger('click')
    expect((w.get('[data-test="mcp-url"]').element as HTMLInputElement).value).toBe('https://x/mcp')
    expect((w.get('[data-test="mcp-secret"]').element as HTMLInputElement).value).toBe('')
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(api.update).toHaveBeenCalledWith(expect.objectContaining({ name: 'files', auth_kind: 'bearer' }))
    expect(api.update.mock.calls[0]![0]).not.toHaveProperty('secret')
    expect(toast.success).toHaveBeenCalledWith('files saved')
  })

  it('asks before removing, and removes only when the person agrees', async () => {
    api.remove.mockResolvedValue(undefined)
    const w = await mountTab()
    confirm.mockResolvedValueOnce(false)
    await w.get('[data-test="mcp-remove"]').trigger('click')
    await flushPromises()
    expect(api.remove).not.toHaveBeenCalled()
    confirm.mockResolvedValueOnce(true)
    await w.get('[data-test="mcp-remove"]').trigger('click')
    await flushPromises()
    expect(api.remove).toHaveBeenCalledWith('files')
    expect(toast.success).toHaveBeenCalledWith('files removed')
    expect(confirm.mock.calls[0]![0]).toMatchObject({ title: 'Remove files?' })
  })

  it('changes the trust from the card at once and says when it was refused', async () => {
    api.update.mockRejectedValueOnce(new Error('the passphrase was not confirmed'))
    const w = await mountTab()
    await w.findAll('[role="radio"]').find((r) => r.text() === 'Allow')!.trigger('click')
    await flushPromises()
    expect(api.update).toHaveBeenCalledWith(expect.objectContaining({ trust: 'allow' }))
    expect(toast.error).toHaveBeenCalledWith('the passphrase was not confirmed')
  })

  it('tests a saved server and shows the answer on its card', async () => {
    api.test.mockResolvedValue(server({ status: 'stopped', problem: 'can\'t reach the server', problem_code: 'unreachable', tools: [] }))
    const w = await mountTab()
    await w.get('[data-test="mcp-test"]').trigger('click')
    await flushPromises()
    expect(api.test).toHaveBeenCalledWith({ name: 'files' })
    expect(w.get('[data-test="mcp-test-result"]').text()).toContain('can\'t reach the server')
  })
})
