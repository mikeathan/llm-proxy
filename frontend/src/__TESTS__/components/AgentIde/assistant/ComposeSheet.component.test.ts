import { afterEach, describe, expect, it } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { nextTick } from 'vue'
import ComposeSheet from '../../../../components/AgentIde/assistant/ComposeSheet.vue'

const mounted: VueWrapper[] = []
function mountSheet(props: { open?: boolean; modelValue?: string } = {}) {
  const w = mount(ComposeSheet, {
    props: { open: true, modelValue: 'first line\nsecond line', ...props },
    global: { stubs: { Icon: true } },
    attachTo: document.body,
  })
  mounted.push(w)
  return w
}

afterEach(() => {
  mounted.splice(0).forEach((w) => w.unmount())
  document.body.innerHTML = ''
})

describe('ComposeSheet', () => {
  it('renders nothing while closed', () => {
    expect(mountSheet({ open: false }).find('[role="dialog"]').exists()).toBe(false)
  })

  it('is a labelled modal dialog holding the draft, with its size', () => {
    const w = mountSheet()
    const dialog = w.get('[role="dialog"]')
    expect(dialog.attributes('aria-modal')).toBe('true')
    expect(dialog.attributes('aria-label')).toBe('Write a message')
    expect((w.get('textarea').element as HTMLTextAreaElement).value).toBe('first line\nsecond line')
    expect(w.get('[data-test="sheet-size"]').text()).toBe('2 lines · 22 characters')
  })

  it('puts the cursor in the message when it opens', async () => {
    const w = mountSheet({ open: false })
    await w.setProps({ open: true })
    await nextTick()
    expect(document.activeElement).toBe(w.get('textarea').element)
  })

  it('reports what is typed', async () => {
    const w = mountSheet()
    await w.get('textarea').setValue('changed')
    const updates = w.emitted('update:modelValue')!
    expect(updates[updates.length - 1]).toEqual(['changed'])
  })

  it('sends from the header, and cannot send an empty message', async () => {
    const w = mountSheet()
    const send = () => w.findAll('button').find((b) => /send/i.test(b.text()))!
    await send().trigger('click')
    expect(w.emitted('send')).toHaveLength(1)
    await w.setProps({ modelValue: '   ' })
    expect(send().attributes('disabled')).toBeDefined()
  })

  it('closes with Done and with Escape', async () => {
    const w = mountSheet()
    await w.findAll('button').find((b) => /done/i.test(b.text()))!.trigger('click')
    await w.get('[role="dialog"]').trigger('keydown', { key: 'Escape' })
    expect(w.emitted('close')).toHaveLength(2)
  })

  it('returns focus to what opened it', async () => {
    const opener = document.createElement('button')
    document.body.appendChild(opener)
    opener.focus()
    const w = mountSheet({ open: false })
    await w.setProps({ open: true })
    await nextTick()
    await w.setProps({ open: false })
    await nextTick()
    expect(document.activeElement).toBe(opener)
  })
})
