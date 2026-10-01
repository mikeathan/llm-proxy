import { describe, it, expect, afterEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import ContextDrawer from '../../../components/layout/ContextDrawer.vue'

function mountDrawer(open = false) {
  return mount(ContextDrawer, {
    props: { open, title: 'Monitor' },
    slots: { default: '<button data-test="inside">Inside</button>' },
    attachTo: document.body,
  })
}

describe('ContextDrawer', () => {
  afterEach(() => {
    document.body.innerHTML = ''
  })

  it('renders nothing while closed', () => {
    const wrapper = mountDrawer()
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
  })

  it('is a labelled dialog that takes focus when opened', async () => {
    const wrapper = mountDrawer()
    await wrapper.setProps({ open: true })
    await nextTick()
    const dialog = wrapper.get('[role="dialog"]')
    expect(dialog.attributes('aria-modal')).toBe('true')
    expect(wrapper.get(`#${dialog.attributes('aria-labelledby')}`).text()).toBe('Monitor')
    expect(document.activeElement?.getAttribute('aria-label')).toBe('Close Monitor')
  })

  it('closes on Escape, the scrim and the close button', async () => {
    const wrapper = mountDrawer(true)
    await wrapper.get('[role="dialog"]').trigger('keydown', { key: 'Escape' })
    await wrapper.get('[data-test="drawer-scrim"]').trigger('click')
    await wrapper.get('[aria-label="Close Monitor"]').trigger('click')
    expect(wrapper.emitted('update:open')).toEqual([[false], [false], [false]])
  })

  it('returns focus to the opener on close', async () => {
    const opener = document.createElement('button')
    document.body.appendChild(opener)
    opener.focus()
    const wrapper = mountDrawer()
    await wrapper.setProps({ open: true })
    await nextTick()
    await wrapper.setProps({ open: false })
    await nextTick()
    expect(document.activeElement).toBe(opener)
  })
})
