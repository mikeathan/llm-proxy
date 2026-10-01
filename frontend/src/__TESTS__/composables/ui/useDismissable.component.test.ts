import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent, h, nextTick, ref } from 'vue'
import { useDismissable } from '../../../composables/ui/useDismissable'

// Outside click and Escape dismiss an open popover; nothing else does.
function setup() {
  const open = ref(true)
  const Comp = defineComponent({
    setup() {
      const root = ref<HTMLElement | null>(null)
      useDismissable(root, open)
      return () => h('div', [h('div', { ref: root, 'data-test': 'inside' }, h('button', 'in')), h('button', { 'data-test': 'outside' }, 'out')])
    },
  })
  const w = mount(Comp, { attachTo: document.body })
  return { w, open }
}

const pointerDown = (el: Element) => el.dispatchEvent(new Event('pointerdown', { bubbles: true }))

describe('useDismissable', () => {
  it('closes on a pointer press outside the root, not inside it', async () => {
    const { w, open } = setup()
    pointerDown(w.get('[data-test="inside"] button').element)
    expect(open.value).toBe(true)
    pointerDown(w.get('[data-test="outside"]').element)
    expect(open.value).toBe(false)
    w.unmount()
  })

  it('closes on Escape', async () => {
    const { w, open } = setup()
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter' }))
    expect(open.value).toBe(true)
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(open.value).toBe(false)
    w.unmount()
  })

  it('does nothing while closed and stops listening after unmount', async () => {
    const { w, open } = setup()
    open.value = false
    pointerDown(w.get('[data-test="outside"]').element)
    expect(open.value).toBe(false)
    w.unmount()
    open.value = true
    await nextTick()
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    pointerDown(document.body)
    expect(open.value).toBe(true)
  })
})
