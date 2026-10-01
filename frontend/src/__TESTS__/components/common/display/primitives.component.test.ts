import { describe, it, expect, vi, afterEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import MicroLabel from '../../../../components/common/display/MicroLabel.vue'
import StatusTag from '../../../../components/common/display/StatusTag.vue'
import IdChip from '../../../../components/common/display/IdChip.vue'

describe('MicroLabel', () => {
  it('renders its text as a label', () => {
    expect(mount(MicroLabel, { slots: { default: 'Tokens' } }).text()).toBe('Tokens')
  })
})

describe('StatusTag', () => {
  it('always shows its text, never colour alone', () => {
    const w = mount(StatusTag, { props: { state: 'error', label: 'Failed' } })
    expect(w.text()).toBe('Failed')
    expect(w.classes()).toContain('text-state-error')
  })

  it('draws queued with a hollow dot', () => {
    const w = mount(StatusTag, { props: { state: 'queued', label: 'Queued #2' } })
    expect(w.get('[data-test="dot"]').classes()).toContain('bg-transparent')
  })
})

describe('IdChip', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('truncates, keeps the full id in its name, and copies it with feedback', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    vi.stubGlobal('navigator', { clipboard: { writeText } })
    const w = mount(IdChip, { props: { id: 'run_0123456789abcdef' } })
    const button = w.get('button')
    expect(button.text()).toBe('run_0123…')
    expect(button.attributes('title')).toBe('run_0123456789abcdef')
    expect(button.attributes('aria-label')).toBe('Copy ID run_0123456789abcdef')
    await button.trigger('click')
    await flushPromises()
    expect(writeText).toHaveBeenCalledWith('run_0123456789abcdef')
    expect(w.get('[role="status"]').text()).toBe('Copied')
  })

  it('says when copying failed', async () => {
    vi.stubGlobal('navigator', { clipboard: { writeText: vi.fn().mockRejectedValue(new Error('denied')) } })
    const w = mount(IdChip, { props: { id: 'short' } })
    expect(w.get('button').text()).toBe('short')
    await w.get('button').trigger('click')
    await flushPromises()
    expect(w.get('[role="status"]').text()).toBe('Copy failed')
  })
})
