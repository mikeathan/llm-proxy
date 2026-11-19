import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import McpServerCard from '../../../components/agents/McpServerCard.vue'
import type { McpServer } from '../../../types/mcp'

const server = (over: Partial<McpServer> = {}): McpServer => ({
  name: 'files', url: 'https://mcp.example.com/mcp', enabled: true, trust: 'ask', auth_kind: 'none', has_secret: false, status: 'ready',
  server_name: 'fixture', server_version: '1.2.3', transport: 'streamable_http', protocol: '2025-06-18',
  tools: [{ name: 'read_text', description: 'Read a file.' }, { name: 'rm', description: 'Delete a file.', disabled: true }], disabled_tools: ['rm'], ...over,
})

const mountCard = (s: McpServer, extra: Record<string, unknown> = {}) =>
  mount(McpServerCard, { props: { server: s, busy: false, testing: false, ...extra }, global: { stubs: { Icon: true } } })

describe('McpServerCard', () => {
  it('says a connected server\'s transport, how many tools are offered and what it calls itself', () => {
    const w = mountCard(server())
    expect(w.get('[data-test="mcp-status"]').text()).toContain('Connected')
    expect(w.get('[data-test="mcp-state"]').text()).toBe('over streamable HTTP · 1 tool of 2 · fixture 1.2.3')
  })

  it('names the older transport plainly', () => {
    expect(mountCard(server({ transport: 'sse' })).get('[data-test="mcp-state"]').text()).toContain('the older SSE transport')
  })

  it('says why a server is not working and what to do about it', () => {
    const w = mountCard(server({ status: 'stopped', tools: [], problem: 'the server wants a sign-in it was not given (401)', problem_code: 'sign_in' }))
    const text = w.get('[data-test="mcp-state"]').text()
    expect(text).toContain('the server wants a sign-in it was not given (401)')
    expect(text).toContain('Edit the server and give it the token or header it expects.')
    expect(w.get('[data-test="mcp-status"]').text()).toContain('Not working')
  })

  it('falls back to the server\'s own words for a problem it has no advice for', () => {
    const w = mountCard(server({ status: 'stopped', tools: [], problem: 'something new', problem_code: 'brand_new' }))
    expect(w.get('[data-test="mcp-state"]').text()).toBe('something new')
  })

  it('says a switched-off server sends nothing, and cannot be tested', () => {
    const w = mountCard(server({ status: 'off', enabled: false, tools: [] }))
    expect(w.get('[data-test="mcp-state"]').text()).toContain('nothing is sent')
    expect(w.get('[data-test="mcp-test"]').attributes('disabled')).toBeDefined()
  })

  it('reports each thing the person does and no more', async () => {
    const w = mountCard(server())
    await w.get('[data-test="mcp-test"]').trigger('click')
    await w.get('[data-test="mcp-edit"]').trigger('click')
    await w.get('[data-test="mcp-remove"]').trigger('click')
    await w.get('input[role="switch"]').setValue(false)
    expect(Object.keys(w.emitted()).filter((k) => ['test', 'edit', 'remove', 'enable'].includes(k)).sort()).toEqual(['edit', 'enable', 'remove', 'test'])
    expect(w.emitted('enable')![0]).toEqual([false])
  })

  it('changes the trust from the control, and says what each choice means', async () => {
    const w = mountCard(server())
    expect(w.text()).toContain('waits for your approval')
    await w.findAll('[role="radio"]').find((r) => r.text() === 'Allow')!.trigger('click')
    expect(w.emitted('trust')![0]).toEqual(['allow'])
    expect(mountCard(server({ trust: 'allow' })).text()).toContain('Choose this only for a server you trust')
  })

  it('lists the tools on demand with a switch each, and reports the tool and its new state', async () => {
    const w = mountCard(server())
    expect(w.find('[data-test="mcp-tools"]').exists()).toBe(false)
    const toggle = w.get('[data-test="mcp-tools-toggle"]')
    expect(toggle.attributes('aria-expanded')).toBe('false')
    await toggle.trigger('click')
    expect(toggle.attributes('aria-expanded')).toBe('true')
    const switches = w.get('[data-test="mcp-tools"]').findAll('input[role="switch"]')
    expect(switches.map((s) => (s.element as HTMLInputElement).checked)).toEqual([true, false])
    await switches[0]!.setValue(false)
    await switches[1]!.setValue(true)
    expect(w.emitted('tool')).toEqual([['read_text', true], ['rm', false]])
  })

  it('shows the last test where it was asked', () => {
    const ok = mountCard(server(), { test: server({ status: 'ready', tools: [{ name: 'read_text' }] }) })
    expect(ok.get('[data-test="mcp-test-result"]').text()).toContain('Connected. 1 tool.')
    const bad = mountCard(server(), { test: server({ status: 'stopped', problem: 'the server did not answer in time', problem_code: 'timeout', tools: [] }) })
    expect(bad.get('[data-test="mcp-test-result"]').text()).toContain('the server did not answer in time')
    expect(bad.get('[data-test="mcp-test-result"]').text()).toContain('Try again')
  })

  it('locks every control while a change is being saved', () => {
    const w = mountCard(server(), { busy: true })
    expect(w.get('[data-test="mcp-edit"]').attributes('disabled')).toBeDefined()
    expect(w.get('[data-test="mcp-remove"]').attributes('disabled')).toBeDefined()
    expect(w.get('input[role="switch"]').attributes('disabled')).toBeDefined()
  })

  it('gives every icon-only button a name', () => {
    const w = mountCard(server())
    expect(w.get('[data-test="mcp-edit"]').attributes('aria-label')).toBe('Edit files')
    expect(w.get('[data-test="mcp-remove"]').attributes('aria-label')).toBe('Remove files')
  })
})
