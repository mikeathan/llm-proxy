import { describe, expect, it, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import CopyButton from '../../../../components/common/display/CopyButton.vue'

describe('CopyButton', () => {
  const writeText = vi.fn()
  beforeEach(() => {
    writeText.mockReset().mockResolvedValue(undefined)
    vi.stubGlobal('navigator', { clipboard: { writeText } })
  })

  it('is named by its title and copies text as-is, objects as JSON', async () => {
    const w = mount(CopyButton, { props: { text: 'https://x/hook', title: 'Copy registered URL' }, global: { stubs: { Icon: true } } })
    await w.get('button[aria-label="Copy registered URL"]').trigger('click')
    expect(writeText).toHaveBeenCalledWith('https://x/hook')

    const obj = mount(CopyButton, { props: { text: { a: 1 } }, global: { stubs: { Icon: true } } })
    await obj.get('button[aria-label="Copy to clipboard"]').trigger('click')
    expect(writeText).toHaveBeenLastCalledWith('{\n  "a": 1\n}')
  })

  it('announces the result politely', async () => {
    const w = mount(CopyButton, { props: { text: 'x' }, global: { stubs: { Icon: true } } })
    await w.get('button').trigger('click')
    await flushPromises()
    expect(w.get('[role="status"]').text()).toBe('Copied')
    writeText.mockRejectedValue(new Error('denied'))
    await w.get('button').trigger('click')
    await flushPromises()
    expect(w.get('[role="status"]').text()).toBe('Copy failed')
  })

  it('shows a copy icon that turns into a check once copied', async () => {
    const w = mount(CopyButton, { props: { text: 'x' }, global: { stubs: { Icon: true } } })
    expect(w.get('icon-stub').attributes('name')).toBe('copy')
    await w.get('button').trigger('click')
    await flushPromises()
    expect(w.get('icon-stub').attributes('name')).toBe('check')
  })
})
