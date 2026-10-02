import { describe, it, expect, afterEach } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { nextTick } from 'vue'
import ChatInput from '../../../../components/AgentIde/assistant/ChatInput.vue'

const mounted: VueWrapper[] = []
function mountInput(inputMessage = '') {
  const w = mount(ChatInput, {
    props: { loading: false, paused: false, inputMessage },
    global: { stubs: { Icon: true } },
    attachTo: document.body,
  })
  mounted.push(w)
  return w
}

// happy-dom does no layout: give the textarea the content height a browser would.
function fakeContentHeight(w: VueWrapper, px: number) {
  Object.defineProperty(w.get('textarea').element, 'scrollHeight', { configurable: true, get: () => px })
}

const LONG_DRAFT = Array.from({ length: 40 }, (_, i) => `line ${i}`).join('\n')

afterEach(() => {
  mounted.splice(0).forEach((w) => w.unmount())
  document.body.innerHTML = ''
})

describe('ChatInput', () => {
  it('grows with the text it holds, and shrinks back once cleared', async () => {
    const w = mountInput()
    fakeContentHeight(w, 240)
    await w.setProps({ inputMessage: 'a\nb\nc\nd\ne\nf' })
    await nextTick()
    expect((w.get('textarea').element as HTMLTextAreaElement).style.height).toBe('240px')

    fakeContentHeight(w, 40)
    await w.setProps({ inputMessage: '' })
    await nextTick()
    expect((w.get('textarea').element as HTMLTextAreaElement).style.height).toBe('40px')
  })

  it('keeps its height while hidden, when the browser reports no content height', async () => {
    const w = mountInput()
    fakeContentHeight(w, 120)
    await w.setProps({ inputMessage: 'a\nb\nc' })
    await nextTick()
    fakeContentHeight(w, 0)
    await w.setProps({ inputMessage: 'a\nb\nc\nd' })
    await nextTick()
    expect((w.get('textarea').element as HTMLTextAreaElement).style.height).toBe('120px')
  })

  it('keeps a short message compact, without a size note or an expand control', () => {
    const w = mountInput('hello')
    expect(w.find('[data-test="draft-size"]').exists()).toBe(false)
    expect(w.find('button[aria-label="Expand the message box"]').exists()).toBe(false)
  })

  it('says how big a long paste is and lets it be expanded to read', async () => {
    const w = mountInput(LONG_DRAFT)
    expect(w.get('[data-test="draft-size"]').text()).toBe('40 lines · 309 characters')
    const toggle = w.get('button[aria-label="Expand the message box"]')
    expect(toggle.attributes('aria-pressed')).toBe('false')
    await toggle.trigger('click')
    expect(w.get('textarea').classes()).toContain('is-expanded')
    expect(w.get('button[aria-label="Shrink the message box"]').attributes('aria-pressed')).toBe('true')
  })

  it('collapses again once the message is sent', async () => {
    const w = mountInput(LONG_DRAFT)
    await w.get('button[aria-label="Expand the message box"]').trigger('click')
    await w.setProps({ inputMessage: '' })
    expect(w.get('textarea').classes()).not.toContain('is-expanded')
  })

  it('sends on Enter and keeps Shift+Enter for a new line', async () => {
    const w = mountInput('hi')
    await w.get('textarea').trigger('keydown', { key: 'Enter', shiftKey: true })
    expect(w.emitted('send')).toBeUndefined()
    await w.get('textarea').trigger('keydown', { key: 'Enter' })
    expect(w.emitted('send')).toHaveLength(1)
  })
})
