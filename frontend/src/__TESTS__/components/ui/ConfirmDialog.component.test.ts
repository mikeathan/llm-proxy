import { describe, it, expect, afterEach } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { nextTick } from 'vue'
import ConfirmDialog from '../../../components/ui/ConfirmDialog.vue'

const mounted: VueWrapper[] = []
function mountDialog(props: Record<string, unknown> = {}) {
  const w = mount(ConfirmDialog, {
    props: { modelValue: false, title: 'Delete file?', message: 'plan.md will be removed.', confirmText: 'Delete', ...props },
    attachTo: document.body,
  })
  mounted.push(w)
  return w
}

describe('ConfirmDialog', () => {
  afterEach(() => {
    mounted.splice(0).forEach((w) => w.unmount())
    document.body.innerHTML = ''
  })

  it('is a labelled, described alert dialog that focuses the safe choice', async () => {
    const opener = document.createElement('button')
    document.body.appendChild(opener)
    opener.focus()
    const w = mountDialog()
    await w.setProps({ modelValue: true })
    await nextTick()
    const dialog = w.get('[role="alertdialog"]')
    expect(dialog.attributes('aria-modal')).toBe('true')
    expect(w.get(`#${dialog.attributes('aria-labelledby')}`).text()).toBe('Delete file?')
    expect(w.get(`#${dialog.attributes('aria-describedby')}`).text()).toBe('plan.md will be removed.')
    expect(document.activeElement?.textContent?.trim()).toBe('Cancel')

    await w.setProps({ modelValue: false })
    await nextTick()
    expect(document.activeElement).toBe(opener)
  })

  it('confirms or cancels, closing either way', async () => {
    const w = mountDialog({ modelValue: true })
    await w.findAll('button').find((b) => b.text() === 'Delete')!.trigger('click')
    expect(w.emitted('confirm')).toHaveLength(1)
    expect(w.emitted('update:modelValue')).toEqual([[false]])
  })

  it('cancels on Escape', async () => {
    const w = mountDialog({ modelValue: true })
    await w.get('[role="alertdialog"]').trigger('keydown', { key: 'Escape' })
    expect(w.emitted('cancel')).toHaveLength(1)
    expect(w.emitted('update:modelValue')).toEqual([[false]])
  })

  it('marks the confirm action as destructive for warning and error dialogs', () => {
    const w = mountDialog({ modelValue: true, type: 'warning' })
    expect(w.findAll('button').find((b) => b.text() === 'Delete')!.attributes('data-variant')).toBe('danger')
    const info = mountDialog({ modelValue: true, type: 'info', confirmText: 'OK' })
    expect(info.findAll('button').find((b) => b.text() === 'OK')!.attributes('data-variant')).toBe('primary')
  })
})
