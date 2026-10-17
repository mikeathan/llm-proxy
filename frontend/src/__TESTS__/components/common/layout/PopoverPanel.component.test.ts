import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import PopoverPanel from '../../../../components/common/layout/PopoverPanel.vue'

function mountPanel() {
  return mount(PopoverPanel, { props: { id: 'p1', title: 'Run activity' }, slots: { default: '<p data-test="body">content</p>' } })
}

describe('PopoverPanel', () => {
  it('is a labelled dialog carrying its id, with the slot as its body', () => {
    const w = mountPanel()
    const dialog = w.get('[role="dialog"]')
    expect(dialog.attributes('id')).toBe('p1')
    expect(dialog.attributes('aria-label')).toBe('Run activity')
    expect(w.get('[data-test="body"]').text()).toBe('content')
  })

  it('asks to close from its labelled close button and from the mobile scrim', async () => {
    const w = mountPanel()
    await w.get('button[aria-label="Close run activity"]').trigger('click')
    await w.get('[aria-hidden="true"].fixed').trigger('click')
    expect(w.emitted('close')).toHaveLength(2)
  })
})

describe('PopoverPanel placement', () => {
  const classes = (placement?: 'below' | 'above') =>
    mount(PopoverPanel, { props: { id: 'p', title: 'T', ...(placement ? { placement } : {}) }, global: { stubs: { Icon: true } } }).get('[role="dialog"]').classes()

  it('opens below its control by default, for a header control', () => {
    expect(classes()).toContain('sm:top-full')
    expect(classes()).not.toContain('sm:bottom-full')
  })

  it('opens above its control for one at the bottom of the screen', () => {
    expect(classes('above')).toContain('sm:bottom-full')
    expect(classes('above')).not.toContain('sm:top-full')
  })
})
