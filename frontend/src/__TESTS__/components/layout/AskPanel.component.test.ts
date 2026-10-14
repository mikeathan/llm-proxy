import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { defineComponent, h, nextTick } from 'vue'
import { createMemoryHistory, createRouter, type Router } from 'vue-router'
import { memoryStorage } from '../../helpers/memoryStorage'
import { ROUTE_NAMES } from '../../../types/routes'

const { openId } = vi.hoisted(() => ({ openId: { value: null as string | null } }))
vi.mock('../../../composables/assistant/useAssistant', async () => {
  const { ref } = await import('vue')
  const id = ref<string | null>(null)
  Object.defineProperty(openId, 'value', { get: () => id.value, set: (v: string | null) => (id.value = v) })
  return { openConversationId: id }
})

const focusInput = vi.fn()
const Chat = defineComponent({
  props: { workspaceId: { type: String, required: true }, conversationId: { type: String, default: null }, variant: { type: String, default: 'page' } },
  emits: ['close'],
  setup(props, { emit, expose }) {
    expose({ focusInput })
    return () => h('div', { 'data-test': 'chat', 'data-ws': props.workspaceId, 'data-conversation': props.conversationId ?? '', 'data-variant': props.variant }, [
      h('button', { 'data-test': 'chat-close', onClick: () => emit('close') }),
    ])
  },
})

import AskPanel from '../../../components/layout/AskPanel.vue'
import { useAskPanel } from '../../../composables/ui/useAskPanel'
import { useLastWorkspace } from '../../../composables/ui/useLastWorkspace'

const empty = { render: () => null }
function makeRouter(): Router {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/overview', name: ROUTE_NAMES.overview, component: empty },
      { path: '/workspaces/:ws/assistant/:conversationId?', name: ROUTE_NAMES.workspaceAssistant, component: empty },
    ],
  })
}

let narrow = false
const mounted: VueWrapper[] = []
async function mountPanel(path = '/overview') {
  const router = makeRouter()
  await router.push(path)
  const w = mount(AskPanel, { global: { plugins: [router], stubs: { AssistantChat: Chat } }, attachTo: document.body })
  mounted.push(w)
  await flushPromises()
  return { w, router }
}

describe('AskPanel', () => {
  beforeEach(() => {
    vi.stubGlobal('localStorage', memoryStorage())
    vi.stubGlobal('matchMedia', (q: string) => ({ matches: !narrow && /min-width/.test(q), addEventListener: vi.fn(), removeEventListener: vi.fn() }))
    narrow = false
    openId.value = null
    focusInput.mockReset()
    useLastWorkspace().remember('demo')
  })
  afterEach(() => {
    mounted.splice(0).forEach((w) => w.unmount())
    document.body.innerHTML = ''
    useAskPanel().hide()
    useLastWorkspace().forget('demo')
    vi.unstubAllGlobals()
  })

  it('shows nothing until it is opened', async () => {
    const { w } = await mountPanel()
    expect(w.find('[data-test="chat"]').exists()).toBe(false)
  })

  it('opens the assistant of the workspace last used, as the narrow panel variant', async () => {
    const { w } = await mountPanel()
    useAskPanel().show()
    await flushPromises()
    const chat = w.get('[data-test="chat"]')
    expect(chat.attributes('data-ws')).toBe('demo')
    expect(chat.attributes('data-variant')).toBe('panel')
  })

  it('resumes the conversation that is open, so a run started elsewhere carries on here', async () => {
    openId.value = 's7'
    const { w } = await mountPanel()
    useAskPanel().show()
    await flushPromises()
    expect(w.get('[data-test="chat"]').attributes('data-conversation')).toBe('s7')
  })

  it('has nothing to open without a workspace', async () => {
    useLastWorkspace().forget('demo')
    const { w } = await mountPanel()
    useAskPanel().show()
    await flushPromises()
    expect(w.find('[data-test="chat"]').exists()).toBe(false)
  })

  it('is a complementary region on a wide screen and a modal dialog on a narrow one', async () => {
    const wide = await mountPanel()
    useAskPanel().show()
    await flushPromises()
    expect(wide.w.get('#ask-panel').attributes('role')).toBe('complementary')
    wide.w.unmount()
    useAskPanel().hide()

    narrow = true
    const phone = await mountPanel()
    useAskPanel().show()
    await flushPromises()
    expect(phone.w.get('#ask-panel').attributes('role')).toBe('dialog')
    expect(phone.w.get('#ask-panel').attributes('aria-modal')).toBe('true')
  })

  it('closes from the chat and with Escape, and puts the cursor in the message when it opens', async () => {
    const { w } = await mountPanel()
    useAskPanel().show()
    await flushPromises()
    expect(focusInput).toHaveBeenCalled()
    await w.get('[data-test="chat-close"]').trigger('click')
    expect(useAskPanel().open.value).toBe(false)
    useAskPanel().show()
    await flushPromises()
    await w.get('#ask-panel').trigger('keydown', { key: 'Escape' })
    expect(useAskPanel().open.value).toBe(false)
  })

  it('resumes whichever conversation is open each time it opens', async () => {
    openId.value = 's7'
    const { w } = await mountPanel()
    useAskPanel().show()
    await flushPromises()
    useAskPanel().hide()
    await nextTick()
    openId.value = 's8'
    useAskPanel().show()
    await flushPromises()
    expect(w.get('[data-test="chat"]').attributes('data-conversation')).toBe('s8')
  })

  it('gives way to the Assistant page, and stays closed after leaving it', async () => {
    const { w, router } = await mountPanel()
    useAskPanel().show()
    await flushPromises()
    await router.push('/workspaces/demo/assistant')
    await flushPromises()
    expect(w.find('[data-test="chat"]').exists()).toBe(false)
    expect(useAskPanel().open.value).toBe(false)
    await router.push('/overview')
    await flushPromises()
    expect(w.find('[data-test="chat"]').exists()).toBe(false)
  })
})
