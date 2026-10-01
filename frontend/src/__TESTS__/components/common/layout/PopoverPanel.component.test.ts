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
