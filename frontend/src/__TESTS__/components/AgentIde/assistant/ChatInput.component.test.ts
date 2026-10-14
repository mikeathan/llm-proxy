import { describe, it, expect, afterEach, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { nextTick } from 'vue'
const commandService = vi.hoisted(() => ({ list: vi.fn(), preview: vi.fn() }))
vi.mock('../../../../services/commands/commandService', () => ({ CommandService: commandService }))

import ChatInput from '../../../../components/AgentIde/assistant/ChatInput.vue'

const mounted: VueWrapper[] = []
function mountInput(inputMessage = '', compact = false) {
  const w = mount(ChatInput, {
    props: { loading: false, paused: false, inputMessage, compact },
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

  it('starts three lines tall, so a message has room to be written', () => {
    expect(mountInput().get('textarea').attributes('rows')).toBe('3')
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

  describe('on a small screen', () => {
    it('opens the full-screen editor instead of growing in place', async () => {
      const w = mountInput(LONG_DRAFT, true)
      await w.get('button[aria-label="Open the full-screen editor"]').trigger('click')
      expect(w.find('[role="dialog"]').exists()).toBe(true)
      expect((w.get('[role="dialog"] textarea').element as HTMLTextAreaElement).value).toBe(LONG_DRAFT)
      expect(w.get('textarea').classes()).not.toContain('is-expanded')
    })

    it('edits the same draft, and closes the editor once the message is sent', async () => {
      const w = mountInput(LONG_DRAFT, true)
      await w.get('button[aria-label="Open the full-screen editor"]').trigger('click')
      await w.get('[role="dialog"] textarea').setValue('edited')
      const fills = w.emitted('update:inputMessage')!
      expect(fills[fills.length - 1]).toEqual(['edited'])
      await w.setProps({ inputMessage: '' })
      expect(w.find('[role="dialog"]').exists()).toBe(false)
    })

    it('sends from the editor', async () => {
      const w = mountInput(LONG_DRAFT, true)
      await w.get('button[aria-label="Open the full-screen editor"]').trigger('click')
      await w.findAll('[role="dialog"] button').find((b) => /send/i.test(b.text()))!.trigger('click')
      expect(w.emitted('send')).toHaveLength(1)
    })

    it('keeps send and stop at a touch-sized target', () => {
      const w = mountInput('hi', true)
      expect(w.get('button[aria-label="Send"]').classes().join(' ')).toContain('h-11')
    })
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

describe('ChatInput slash commands', () => {
  const lastFill = (w: VueWrapper) => { const all = w.emitted('update:inputMessage')!; return all[all.length - 1] }

  const remind = { name: 'remind', usage: '/remind <when> <message>', summary: 'Set a reminder', examples: ['tomorrow 9am call the bank'] }

  async function typed(text: string) {
    commandService.list.mockResolvedValue([remind])
    commandService.preview.mockResolvedValue({ matched: true, command: 'remind', text: 'Tue 13 Oct 2026 09:00 Europe/London' })
    const w = mountInput()
    await w.setProps({ inputMessage: text })
    await flushPromises()
    return w
  }

  it('opens a menu of commands on "/" and fills the one picked', async () => {
    const w = await typed('/')
    await w.get('[data-test="slash-command-remind"]').trigger('click')
    expect(lastFill(w)).toEqual(['/remind '])
  })

  it('completes the first match with Tab instead of moving focus', async () => {
    const w = await typed('/re')
    await w.get('textarea').trigger('keydown', { key: 'Tab' })
    expect(lastFill(w)).toEqual(['/remind '])
  })

  it('shows the usage and clickable examples once the command is named', async () => {
    const w = await typed('/remind ')
    expect(w.get('[data-test="slash-usage"]').text()).toBe('/remind <when> <message>')
    await w.get('[data-test="slash-example"]').trigger('click')
    expect(lastFill(w)).toEqual(['/remind tomorrow 9am call the bank'])
  })

  it('previews what the typed command would do', async () => {
    vi.useFakeTimers()
    try {
      const w = await typed('/remind tomorrow 9am call')
      await vi.advanceTimersByTimeAsync(300)
      expect(w.get('[data-test="slash-preview"]').text()).toContain('Tue 13 Oct 2026 09:00 Europe/London')
    } finally {
      vi.useRealTimers()
    }
  })

  it('shows nothing for ordinary text', async () => {
    const w = await typed('what is on my calendar')
    expect(w.find('[data-test="slash-helper"]').exists()).toBe(false)
  })
})
