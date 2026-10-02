import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ToolCallSegment from '../../../../components/AgentIde/assistant/ToolCallSegment.vue'
import { TOOL_RESULT_PREVIEW_CHARS } from '../../../../utils/assistant/toolSteps'

type Seg = { kind: 'tool_call'; name: string; args: string; status: string; result?: string; error?: string }
const seg = (over: Partial<Seg> = {}): Seg => ({ kind: 'tool_call', name: 'read_file', args: '{"path":"notes/a.md"}', status: 'success', ...over })

function mountStep(segment: Seg) {
  return mount(ToolCallSegment, {
    props: { segment, turnIdx: 0, segIdx: 0, expanded: false, compact: true },
    global: { stubs: { Icon: true } },
  })
}
const open = async (w: ReturnType<typeof mountStep>) => w.get('button[aria-expanded]').trigger('click')

// One step of the assistant's timeline: a plain verb, its target, and an
// outcome spelled out (not by colour alone); details on demand.
describe('ToolCallSegment', () => {
  it('reads as a step: verb, target and outcome', () => {
    const w = mountStep(seg())
    expect(w.element.tagName).toBe('LI')
    expect(w.get('.step-verb').text()).toBe('Read a file')
    expect(w.get('.step-target').text()).toBe('notes/a.md')
    expect(w.get('.step-status').text()).toBe('Done')
    expect(w.get('button').attributes('aria-expanded')).toBe('false')
    expect(mountStep(seg({ status: 'running' })).get('.step-status').text()).toBe('Running')
    expect(mountStep(seg({ status: 'error', error: 'no such file' })).get('.step-status').text()).toBe('Failed')
  })

  it('shows the arguments and the decoded output when opened', async () => {
    const w = mountStep(seg({ name: 'execute_terminal_command', args: '{"command":"node --version"}', result: '"v26.0.0\\n"' }))
    await open(w)
    expect(w.get('dl').text()).toContain('command')
    expect(w.get('dl').text()).toContain('node --version')
    expect(w.get('.step-output').text()).toBe('v26.0.0')
    expect(w.get('.step-output').classes()).toContain('step-output--terminal')
  })

  it('shows search results as links that open safely, and an unsafe one as text', async () => {
    const result = JSON.stringify([
      { title: 'Haiku 5.5', url: 'https://cellcog.ai/blog/x', snippet: 'Confirmed, undated.' },
      { title: 'Trap', url: 'javascript:alert(1)', snippet: '' },
    ])
    const w = mountStep(seg({ name: 'internet_search', args: '{"query":"haiku"}', result }))
    await open(w)
    const links = w.findAll('.step-hits a')
    expect(links).toHaveLength(1)
    expect(links[0]!.attributes()).toMatchObject({ href: 'https://cellcog.ai/blog/x', target: '_blank', rel: 'noopener noreferrer' })
    expect(w.get('.step-hits').text()).toContain('cellcog.ai')
    expect(w.get('.step-hits').text()).toContain('Trap')
  })

  it('shows the start of a very large output, and all of it on request', async () => {
    const w = mountStep(seg({ result: JSON.stringify('x'.repeat(TOOL_RESULT_PREVIEW_CHARS + 5)) }))
    await open(w)
    expect(w.get('.step-output').text()).toHaveLength(TOOL_RESULT_PREVIEW_CHARS)
    await w.findAll('button').find((b) => b.text() === 'Show all')!.trigger('click')
    expect(w.get('.step-output').text()).toHaveLength(TOOL_RESULT_PREVIEW_CHARS + 5)
  })

  it('shows the error of a failed step', async () => {
    const w = mountStep(seg({ status: 'error', error: 'no such file' }))
    await open(w)
    expect(w.get('.step-output--error').text()).toBe('no such file')
  })
})
