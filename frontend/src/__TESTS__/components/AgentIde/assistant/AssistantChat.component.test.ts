import { describe, expect, it, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { ref } from 'vue'
import { memoryStorage } from '../../../helpers/memoryStorage'
import type { SessionBrief } from '../../../../types/assistant'

const a = {
  loading: ref(false),
  messages: ref([]),
  sessions: ref<SessionBrief[]>([{ id: 's1', snippet: 'Plan the move', updated_at: new Date().toISOString() }]),
  currentSessionId: ref<string | null>(null),
  pendingDecision: ref<unknown>(null),
  submitDecision: vi.fn(),
  thinking: ref(false),
  liveReasoning: ref(''),
  paused: ref(false),
  phase: ref('idle'),
  modelBusy: ref<string | null>(null),
  dismissModelBusy: vi.fn(),
  sseConnected: ref(true),
  fetchSessions: vi.fn(),
  loadSession: vi.fn(),
  newSession: vi.fn(),
  openWorkspace: vi.fn(),
  sendMessage: vi.fn(),
  deleteSession: vi.fn(),
  deleteSessionsByIds: vi.fn(),
  cancelSession: vi.fn(),
  cancel: vi.fn(),
  liveEvents: ref([]),
}
vi.mock('../../../../composables/assistant/useAssistant', () => ({ useAssistant: () => a }))
const review = {
  open: ref(false), loading: ref(false), saving: ref(false), error: ref(''), items: ref([]), selectedCount: ref(0),
  start: vi.fn(), toggle: vi.fn(), save: vi.fn().mockResolvedValue(0), close: vi.fn(),
}
vi.mock('../../../../composables/memory/useMemoryReview', () => ({ useMemoryReview: () => review }))
const confirm = vi.fn()
vi.mock('../../../../composables/ui/useConfirm', () => ({ useConfirm: () => ({ confirm }) }))

const commandService = vi.hoisted(() => ({ list: vi.fn(), run: vi.fn(), preview: vi.fn() }))
vi.mock('../../../../services/commands/commandService', () => ({ CommandService: commandService }))

import AssistantChat from '../../../../components/AgentIde/assistant/AssistantChat.vue'

function mountChat() {
  const w = mount(AssistantChat, { props: { workspaceId: 'ws' }, global: { stubs: { Icon: true, ChatMessages: { template: '<div />', methods: { scrollToBottom() {} } }, GuardrailBanner: true } } })
  const button = (name: RegExp) => w.findAll('button').find((b) => name.test(b.attributes('aria-label') ?? '') || name.test(b.text()))
  const status = () => w.get('[data-test="chat-status"]').text()
  return { w, button, status }
}

describe('AssistantChat', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.stubGlobal('localStorage', memoryStorage())
    a.loading.value = false
    a.pendingDecision.value = null
    a.modelBusy.value = null
    a.sseConnected.value = true
    confirm.mockResolvedValue(true)
    commandService.list.mockResolvedValue([])
    commandService.run.mockResolvedValue({ handled: false, reply: '', failed: false })
  })

  it('states what the assistant is doing, from the real run state', async () => {
    const { status } = mountChat()
    expect(status()).toBe('Ready')
    a.loading.value = true
    await flushPromises()
    expect(status()).toBe('Working')
    a.sseConnected.value = false
    await flushPromises()
    expect(status()).toBe('Reconnecting to the run')
    a.modelBusy.value = 'The local model is busy'
    await flushPromises()
    expect(status()).toBe('Waiting for the model')
    a.pendingDecision.value = { decision_id: 'd1' }
    await flushPromises()
    expect(status()).toBe('Waiting for your approval')
  })

  it('names its header controls and toggles the conversation list', async () => {
    const { w, button } = mountChat()
    await button(/^show conversations$/i)!.trigger('click')
    expect(button(/^hide conversations$/i)).toBeDefined()
    expect(w.text()).toContain('Plan the move')
    expect(button(/^new chat$/i)).toBeDefined()
    expect(button(/^close the assistant$/i)).toBeDefined()
  })

  it('keeps the conversation list open between visits, on a roomy screen', async () => {
    const first = mountChat()
    await first.button(/^show conversations$/i)!.trigger('click')
    first.w.unmount()
    const again = mountChat()
    expect(again.button(/^hide conversations$/i)).toBeDefined()
  })

  it('shows the open conversation as the title, and a new one as such', async () => {
    const { w } = mountChat()
    expect(w.get('h2').text()).toBe('New conversation')
    a.currentSessionId.value = 's1'
    await flushPromises()
    expect(w.get('h2').text()).toBe('Plan the move')
    a.currentSessionId.value = null
  })

  it('puts New next to the list toggle while the list is closed, and in the list once it is open', async () => {
    const { w, button } = mountChat()
    expect(w.find('[data-test="header-new"]').exists()).toBe(true)
    await w.get('[data-test="header-new"]').trigger('click')
    expect(a.newSession).toHaveBeenCalled()
    await button(/^show conversations$/i)!.trigger('click')
    expect(w.find('[data-test="header-new"]').exists()).toBe(false)
    expect(w.get('[data-test="session-new"]').text()).toMatch(/new chat/i)
  })

  describe('as the ask panel', () => {
    function mountPanel() {
      const w = mount(AssistantChat, {
        props: { workspaceId: 'ws', variant: 'panel' },
        global: { stubs: { Icon: true, ChatMessages: { template: '<div />', methods: { scrollToBottom() {} } }, GuardrailBanner: true } },
      })
      const button = (name: RegExp) => w.findAll('button').find((b) => name.test(b.attributes('aria-label') ?? '') || name.test(b.text()))
      return { w, button }
    }

    it('never shows the conversation list as a column, and opens it over the chat', async () => {
      const { w, button } = mountPanel()
      expect(w.find('aside.chat-sidebar').exists()).toBe(false)
      await button(/^show conversations$/i)!.trigger('click')
      const drawer = w.get('.mobile-drawer')
      expect(drawer.classes()).toContain('mobile-drawer--panel')
      expect(w.text()).toContain('Plan the move')
    })

    it('does not touch the list preference of the Assistant page', async () => {
      const { button } = mountPanel()
      await button(/^show conversations$/i)!.trigger('click')
      expect(localStorage.getItem('admin-ui:assistant-history-open')).toBeNull()
    })

    it('keeps its header icon-only, and can be closed', async () => {
      const { w, button } = mountPanel()
      expect(w.get('[data-test="header-new"]').text()).toBe('')
      await button(/^close the assistant$/i)!.trigger('click')
      expect(w.emitted('close')).toHaveLength(1)
    })

    it('puts the cursor in the message on request', async () => {
      const w = mount(AssistantChat, {
        props: { workspaceId: 'ws', variant: 'panel' },
        global: { stubs: { Icon: true, ChatMessages: { template: '<div />', methods: { scrollToBottom() {} } }, GuardrailBanner: true } },
        attachTo: document.body,
      })
      ;(w.vm as unknown as { focusInput: () => void }).focusInput()
      expect(document.activeElement).toBe(w.get('textarea').element)
      w.unmount()
    })
  })

  it('says "Ready" only to assistive technology, and shows a working state in words', async () => {
    const { w } = mountChat()
    expect(w.get('[data-test="chat-status"] .sr-only').text()).toBe('Ready')
    a.loading.value = true
    await flushPromises()
    expect(w.find('[data-test="chat-status"] .sr-only').exists()).toBe(false)
  })

  // The review needs a saved conversation and a finished turn; it is the operator's per-chat control over memory.
  it('offers "Review for memories" only for a saved conversation that is not running', async () => {
    const { button } = mountChat()
    const reviewButton = () => button(/^review for memories$/i)!
    expect(reviewButton().attributes('disabled')).toBeDefined()
    a.currentSessionId.value = 's1'
    await flushPromises()
    expect(reviewButton().attributes('disabled')).toBeUndefined()
    await reviewButton().trigger('click')
    expect(review.start).toHaveBeenCalledTimes(1)
    a.loading.value = true
    await flushPromises()
    expect(reviewButton().attributes('disabled')).toBeDefined()
    a.currentSessionId.value = null
  })

  it('deletes a conversation only after confirming', async () => {
    const { button } = mountChat()
    await button(/^show conversations$/i)!.trigger('click')
    confirm.mockResolvedValueOnce(false)
    await button(/^delete plan the move$/i)!.trigger('click')
    await flushPromises()
    expect(a.deleteSession).not.toHaveBeenCalled()
    await button(/^delete plan the move$/i)!.trigger('click')
    await flushPromises()
    expect(confirm).toHaveBeenLastCalledWith(expect.objectContaining({ type: 'warning' }))
    expect(a.deleteSession).toHaveBeenCalledWith('ws', 's1')
  })

  // A slash command is answered by the backend and never reaches the assistant.
  describe('slash commands', () => {
    async function send(w: ReturnType<typeof mountChat>['w'], text: string) {
      await w.get('textarea').setValue(text)
      await w.get('textarea').trigger('keydown', { key: 'Enter' })
      await flushPromises()
    }

    it('runs a command in code and clears the box', async () => {
      commandService.run.mockResolvedValue({ handled: true, reply: 'Reminder 4 set', failed: false })
      const { w } = mountChat()
      await send(w, '/remind tomorrow 9am call')
      expect(commandService.run).toHaveBeenCalledWith('/remind tomorrow 9am call')
      expect(a.sendMessage).not.toHaveBeenCalled()
      expect((w.get('textarea').element as HTMLTextAreaElement).value).toBe('')
    })

    it('keeps the text of a refused command so it can be corrected', async () => {
      commandService.run.mockResolvedValue({ handled: true, reply: 'couldn\'t read "blah"', failed: true })
      const { w } = mountChat()
      await send(w, '/remind blah 9am call')
      expect(a.sendMessage).not.toHaveBeenCalled()
      expect((w.get('textarea').element as HTMLTextAreaElement).value).toBe('/remind blah 9am call')
    })

    it('sends an unknown /word to the assistant as usual', async () => {
      const { w } = mountChat()
      await send(w, '/etc/hosts is odd')
      expect(a.sendMessage).toHaveBeenCalledWith('ws', '/etc/hosts is odd')
    })

    it('sends plain text to the assistant without asking the command endpoint', async () => {
      const { w } = mountChat()
      await send(w, 'remind me tomorrow')
      expect(commandService.run).not.toHaveBeenCalled()
      expect(a.sendMessage).toHaveBeenCalledWith('ws', 'remind me tomorrow')
    })
  })
})
