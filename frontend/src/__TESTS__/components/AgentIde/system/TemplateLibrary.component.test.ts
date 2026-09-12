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
  { id: 'pb-scan', name: 'Port Scan', category: 'Recon' },
  { id: 'pb-report', name: 'Write Report', category: 'Docs' },
]

function mountLibrary(props: Partial<{ show: boolean }> = {}) {
  return mount(TemplateLibrary, {
    props: { show: true, ...props },
    global: {
      stubs: { Icon: true, BaseButton: { props: ['icon'], template: '<button><slot /></button>' } },
    },
  })
}

beforeEach(() => {
  vi.mocked(TemplateService.listTemplates).mockResolvedValue(templates)
})

describe('TemplateLibrary panel behavior', () => {
  it('emits close when Escape is pressed while shown', async () => {
    const wrapper = mountLibrary()
    await document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('close')).toBeTruthy()
  })

  it('does not emit close on Escape when hidden', async () => {
    const wrapper = mountLibrary({ show: false })
    await document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('close')).toBeFalsy()
  })

  it('renders action buttons without hover (touch-friendly)', async () => {
    const wrapper = mountLibrary()
    await flushPromises()
    const buttons = wrapper.findAll('.template-actions button')
    expect(buttons.length).toBe(templates.length * 2)
    // regression guard: actions must never be permanently hidden via opacity-0
    const actions = wrapper.find('.template-actions')
    expect(actions.classes()).not.toContain('opacity-0')
  })

  it('uses a responsive multi-column grid so the list is not a tall single column', async () => {
    const wrapper = mountLibrary()
    await flushPromises()
    const grid = wrapper.find('.templates-grid')
    expect(grid.classes()).toContain('grid-cols-1')
    expect(grid.classes()).toContain('sm:grid-cols-2')
  })

  it('renders full-width card titles without truncation', async () => {
    const wrapper = mountLibrary()
    await flushPromises()
    const title = wrapper.find('.template-name')
    expect(title.classes()).not.toContain('truncate')
  })

  it('closes the overlay when clicking the backdrop', async () => {
    const wrapper = mountLibrary()
    await wrapper.find('.drawer-overlay').trigger('click')
    expect(wrapper.emitted('close')).toBeTruthy()
  })
})