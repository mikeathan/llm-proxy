import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import Disclosure from '../../../../components/common/layout/Disclosure.vue'

function mountDisclosure(props: Record<string, unknown> = {}) {
  return mount(Disclosure, {
    props: { title: 'Terminal tool', summary: '2 allowed commands', open: false, ...props },
    slots: { default: '<p data-test="body">fields</p>', aside: '<span data-test="aside">switch</span>' },
    global: { stubs: { Icon: true } },
  })
}

describe('Disclosure', () => {
  it('shows the title and summary, with the body hidden until opened', () => {
    const w = mountDisclosure()
    expect(w.text()).toContain('Terminal tool')
    expect(w.text()).toContain('2 allowed commands')
    const button = w.get('button')
    expect(button.attributes('aria-expanded')).toBe('false')
    expect(w.get('[role="region"]').attributes('style')).toContain('display: none')
  })

  it('asks to open on click, and the button names the body it controls', async () => {
    const w = mountDisclosure()
    await w.get('button').trigger('click')
    expect(w.emitted('update:open')![0]).toEqual([true])
    const region = w.get('[role="region"]')
    expect(w.get('button').attributes('aria-controls')).toBe(region.attributes('id'))
    expect(region.attributes('aria-labelledby')).toBe(w.get('button').attributes('id'))
  })

  it('shows the body when open and asks to close on click', async () => {
    const w = mountDisclosure({ open: true })
    expect(w.get('button').attributes('aria-expanded')).toBe('true')
    expect(w.get('[role="region"]').attributes('style') ?? '').not.toContain('display: none')
    await w.get('button').trigger('click')
    expect(w.emitted('update:open')![0]).toEqual([false])
  })

  it('keeps the aside outside the toggle button so a switch there is not nested in it', () => {
    const w = mountDisclosure()
    expect(w.get('button').find('[data-test="aside"]').exists()).toBe(false)
    expect(w.find('[data-test="aside"]').exists()).toBe(true)
  })

  it('has no toggle when there is nothing to open', () => {
    const w = mountDisclosure({ expandable: false })
    expect(w.find('button').exists()).toBe(false)
    expect(w.text()).toContain('Terminal tool')
    expect(w.find('[data-test="body"]').exists()).toBe(false)
  })
})
