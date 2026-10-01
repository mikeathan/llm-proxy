import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import TemplateLibrary from '../../../../components/AgentIde/system/TemplateLibrary.vue'
import { TemplateService } from '../../../../services/template/templateService'

vi.mock('../../../../services/template/templateService', () => ({
  TemplateService: {
    listTemplates: vi.fn(),
    getTemplate: vi.fn(),
  },
}))

const templates = [
  { id: 'pb-scan', name: 'Port Scan with a deliberately long playbook title that must stay whole', category: 'Recon' },
  { id: 'pb-report', name: 'Write Report', category: 'Docs' },
]

async function mountLibrary(props: Record<string, unknown> = {}) {
  const w = mount(TemplateLibrary, { props: { appendTarget: null, ...props }, global: { stubs: { Icon: true, BrandMark: true } } })
  await flushPromises()
  return w
}
const button = (w: Awaited<ReturnType<typeof mountLibrary>>, text: string) => w.findAll('button').filter((b) => b.text().includes(text))

beforeEach(() => {
  vi.mocked(TemplateService.listTemplates).mockReset().mockResolvedValue(templates)
  vi.mocked(TemplateService.getTemplate).mockReset().mockImplementation(async (id: string) => ({ id, name: id, category: 'x', description: '', content: `# ${id}` }))
})

describe('TemplateLibrary (Playbooks section)', () => {
  it('lists every playbook with visible actions and whole titles', async () => {
    const w = await mountLibrary()
    expect(w.findAll('[data-test="playbook"]')).toHaveLength(2)
    expect(button(w, 'New file')).toHaveLength(2)
    expect(w.text()).toContain('deliberately long playbook title that must stay whole')
    expect(w.find('.truncate').exists()).toBe(false)
  })

  it('filters by search text and by category', async () => {
    const w = await mountLibrary()
    await w.get('input[type="search"]').setValue('report')
    expect(w.findAll('[data-test="playbook"]')).toHaveLength(1)
    await w.get('input[type="search"]').setValue('')
    await w.get('select').setValue('Recon')
    expect(w.findAll('[data-test="playbook"]').map((p) => p.text())).toEqual([expect.stringContaining('Port Scan')])
  })

  it('creates a new file from a playbook', async () => {
    const w = await mountLibrary()
    await button(w, 'New file')[1]!.trigger('click')
    await flushPromises()
    expect(w.emitted('inject')).toEqual([[{ id: 'pb-report', name: 'pb-report', category: 'x', description: '', content: '# pb-report' }, 'create']])
  })

  it('appends only into an open file, naming it', async () => {
    const closed = await mountLibrary()
    expect(button(closed, 'Append')[0]!.attributes('disabled')).toBeDefined()
    expect(closed.text()).toContain('Open a file to append')

    const open = await mountLibrary({ appendTarget: 'plan.md' })
    await button(open, 'Append to plan.md')[0]!.trigger('click')
    await flushPromises()
    expect(open.emitted('inject')![0]![1]).toBe('append')
  })

  it('reports a failed load and an empty library', async () => {
    vi.mocked(TemplateService.listTemplates).mockImplementation(async () => {
      throw new Error('template dir missing')
    })
    expect((await mountLibrary()).get('[role="alert"]').text()).toContain('template dir missing')
    vi.mocked(TemplateService.listTemplates).mockReset().mockResolvedValue([])
    expect((await mountLibrary()).text()).toContain('No playbooks')
  })
})
