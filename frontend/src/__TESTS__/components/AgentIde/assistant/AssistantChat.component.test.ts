import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { ref, shallowRef } from 'vue'
import { memoryStorage } from '../../../helpers/memoryStorage'
import type { RunEnd, SessionBrief } from '../../../../types/assistant'
import { AssistantService } from '../../../../services/assistant/assistantService'

const a = {
  loading: ref(false),
  runToken: ref(0),
  runEnd: shallowRef<RunEnd | null>(null),
  messages: ref([]),
  sessions: ref<SessionBrief[]>([{ id: 's1', snippet: 'Plan the move', updated_at: new Date().toISOString() }]),
  currentSessionId: ref<string | null>(null),
  conversationProfile: ref(''),
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

const catalog = [
  { id: 'builder', name: 'Builder', summary: 'Builds.', source: 'builtin', answer_first: false, may_ask: true, memory_save: 'model', reminders: false, default_for: [] },
  { id: 'general', name: 'General', summary: 'Everyday assistant.', source: 'builtin', answer_first: true, may_ask: true, memory_save: 'model', reminders: true, default_for: ['assistant'] },
  { id: 'research', name: 'Research', summary: 'Finds and cites.', source: 'builtin', answer_first: true, may_ask: true, memory_save: 'model', reminders: false, default_for: [] },
]
const profileService = vi.hoisted(() => ({ list: vi.fn() }))
vi.mock('../../../../services/profile/profileService', () => ({ ProfileService: profileService }))
const dispatcher = vi.hoisted(() => ({ getWorkspaceConfig: vi.fn() }))
vi.mock('../../../../services/automation/dispatcherService', () => ({ DispatcherService: dispatcher }))

const commandService = vi.hoisted(() => ({ list: vi.fn(), run: vi.fn(), preview: vi.fn() }))
vi.mock('../../../../services/commands/commandService', () => ({ CommandService: commandService }))

import AssistantChat from '../../../../components/AgentIde/assistant/AssistantChat.vue'

// Every chat mounted by a test, so it can be taken down: the queued message is shared by the whole app, and a chat left
// mounted would go on handling the next test's run endings.
const mounted: Array<Pick<VueWrapper, 'find' | 'unmount'>> = []
function track<W extends Pick<VueWrapper, 'find' | 'unmount'>>(wrapper: W): W {
  mounted.push(wrapper)
  return wrapper
}

function mountChat() {
  const w = track(mount(AssistantChat, { props: { workspaceId: 'ws' }, global: { stubs: { Icon: true, teleport: true, ChatMessages: { template: '<div />', methods: { scrollToBottom() {} } }, GuardrailBanner: true } } }))
  const button = (name: RegExp) => w.findAll('button').find((b) => name.test(b.attributes('aria-label') ?? '') || name.test(b.text()))
  const status = () => w.get('[data-test="chat-status"]').text()
  return { w, button, status }
}

describe('AssistantChat', () => {
  afterEach(async () => {
    for (const w of mounted.splice(0)) {
      const remove = w.find('[data-test="queued-message"] button')
      if (remove.exists()) await remove.trigger('click')
      w.unmount()
    }
  })

  beforeEach(() => {
    vi.clearAllMocks()
    profileService.list.mockResolvedValue(catalog)
    dispatcher.getWorkspaceConfig.mockResolvedValue({})
    a.conversationProfile.value = ''
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

  it('titles the open conversation, and says nothing for a new one (the empty chat already does)', async () => {
    const { w } = mountChat()
    expect(w.find('h2').exists()).toBe(false)
    a.currentSessionId.value = 's1'
    await flushPromises()
    expect(w.get('h2').text()).toBe('Plan the move')
    a.currentSessionId.value = null
  })

  it('puts New next to the list toggle while the list is closed, and in the list once it is open', async () => {
    a.currentSessionId.value = 's1'
    const { w, button } = mountChat()
    expect(w.find('[data-test="header-new"]').exists()).toBe(true)
    await w.get('[data-test="header-new"]').trigger('click')
    expect(a.newSession).toHaveBeenCalled()
    await button(/^show conversations$/i)!.trigger('click')
    expect(w.find('[data-test="header-new"]').exists()).toBe(false)
    expect(w.get('[data-test="session-new"]').text()).toMatch(/new chat/i)
    a.currentSessionId.value = null
  })

  it('offers no New while the conversation is already new', async () => {
    const { w } = mountChat()
    expect(w.find('[data-test="header-new"]').exists()).toBe(false)
    a.currentSessionId.value = 's1'
    await flushPromises()
    expect(w.find('[data-test="header-new"]').exists()).toBe(true)
    a.currentSessionId.value = null
  })

  describe('as the ask panel', () => {
    function mountPanel() {
      const w = track(mount(AssistantChat, {
        props: { workspaceId: 'ws', variant: 'panel' },
        global: { stubs: { Icon: true, teleport: true, ChatMessages: { template: '<div />', methods: { scrollToBottom() {} } }, GuardrailBanner: true } },
      }))
      const button = (name: RegExp) => w.findAll('button').find((b) => name.test(b.attributes('aria-label') ?? '') || name.test(b.text()))
      return { w, button }
    }

    it('never shows the conversation list as a column, and opens it over the chat', async () => {
      const { w, button } = mountPanel()
      expect(w.find('aside.chat-sidebar').exists()).toBe(false)
      await button(/^show conversations$/i)!.trigger('click')
      expect(w.find('.mobile-drawer').exists()).toBe(true)
      expect(w.text()).toContain('Plan the move')
    })

    it('does not touch the list preference of the Assistant page', async () => {
      const { button } = mountPanel()
      await button(/^show conversations$/i)!.trigger('click')
      expect(localStorage.getItem('admin-ui:assistant-history-open')).toBeNull()
    })

    it('keeps its header icon-only, and can be closed', async () => {
      a.currentSessionId.value = 's1'
      const { w, button } = mountPanel()
      expect(w.get('[data-test="header-new"]').text()).toBe('')
      await button(/^close the assistant$/i)!.trigger('click')
      expect(w.emitted('close')).toHaveLength(1)
      a.currentSessionId.value = null
    })

    it('puts the cursor in the message on request', async () => {
      const w = track(mount(AssistantChat, {
        props: { workspaceId: 'ws', variant: 'panel' },
        global: { stubs: { Icon: true, teleport: true, ChatMessages: { template: '<div />', methods: { scrollToBottom() {} } }, GuardrailBanner: true } },
        attachTo: document.body,
      }))
      ;(w.vm as unknown as { focusInput: () => void }).focusInput()
      expect(document.activeElement).toBe(w.get('textarea').element)
      w.unmount()
    })
  })

  it('shows nothing while ready (assistive technology still hears it), and the state in words once the chat is busy', async () => {
    const { w } = mountChat()
    expect(w.get('[data-test="chat-status"]').classes()).toContain('sr-only')
    expect(w.get('[data-test="chat-status"]').text()).toBe('Ready')
    a.loading.value = true
    await flushPromises()
    expect(w.get('[data-test="chat-status"]').classes()).not.toContain('sr-only')
    expect(w.get('[data-test="chat-status"]').text()).toBe('Working')
  })

  // The review needs a saved conversation and a finished turn; it is the operator's per-chat control over memory.
  it('offers the Memory review (in the message box) only for a saved conversation that is not running', async () => {
    const { button } = mountChat()
    const reviewButton = () => button(/^memory$/i)!
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
      expect(a.sendMessage).toHaveBeenCalledWith('ws', '/etc/hosts is odd', { model: '', profile: '' })
    })

    it('sends plain text to the assistant without asking the command endpoint', async () => {
      const { w } = mountChat()
      await send(w, 'remind me tomorrow')
      expect(commandService.run).not.toHaveBeenCalled()
      expect(a.sendMessage).toHaveBeenCalledWith('ws', 'remind me tomorrow', { model: '', profile: '' })
    })
  })

  // The profile a message runs under: the workspace's or the default until one is picked, then the picked one.
  describe('run settings', () => {
    const settingsButton = (w: ReturnType<typeof mountChat>['w']) => w.get('[data-test="run-settings"]')

    it('shows the profile a new chat starts with, and marks it as the default', async () => {
      const { w } = mountChat()
      await flushPromises()
      expect(settingsButton(w).text()).toBe('General')
      await settingsButton(w).trigger('click')
      const labels = w.findAll('select[aria-label="Profile"] option').map((o) => o.text())
      expect(labels).toEqual(['Builder', 'General (default)', 'Research'])
    })

    it("starts from the workspace's own profile when it has one", async () => {
      dispatcher.getWorkspaceConfig.mockResolvedValue({ assistant_profile: 'builder' })
      const { w } = mountChat()
      await flushPromises()
      expect(settingsButton(w).text()).toBe('Builder')
    })

    it('sends nothing about the profile until one is picked, then sends the pick', async () => {
      const { w } = mountChat()
      await flushPromises()
      await settingsButton(w).trigger('click')
      await w.get('select[aria-label="Profile"]').setValue('research')
      expect(a.conversationProfile.value).toBe('research')
      expect(settingsButton(w).text()).toBe('Research')

      await w.get('textarea').setValue('find me the changelog')
      await w.get('textarea').trigger('keydown', { key: 'Enter' })
      await flushPromises()
      expect(a.sendMessage).toHaveBeenCalledWith('ws', 'find me the changelog', { model: '', profile: 'research' })
    })

    it("shows an open conversation's own profile", async () => {
      a.conversationProfile.value = 'builder'
      const { w } = mountChat()
      await flushPromises()
      expect(settingsButton(w).text()).toBe('Builder')
    })

    it('stays usable when the conversation names a profile that no longer exists', async () => {
      a.conversationProfile.value = 'deleted-file'
      const { w } = mountChat()
      await flushPromises()
      expect(settingsButton(w).text()).toBe('General')
    })

    it('sends no profile for a conversation whose profile no longer exists, so the message is not refused', async () => {
      a.conversationProfile.value = 'deleted-file'
      const { w } = mountChat()
      await flushPromises()
      await w.get('textarea').setValue('carry on')
      await w.get('textarea').trigger('keydown', { key: 'Enter' })
      await flushPromises()
      expect(a.sendMessage).toHaveBeenCalledWith('ws', 'carry on', { model: '', profile: '' })
    })

    it('cannot be changed while a run is going', async () => {
      a.loading.value = true
      const { w } = mountChat()
      await flushPromises()
      expect(settingsButton(w).attributes('disabled')).toBeDefined()
    })
  })

  describe('a message written while a run is going', () => {
    async function queued(text = 'what about Leeds') {
      a.loading.value = true
      a.runToken.value = 1
      const m = mountChat()
      await flushPromises()
      await m.w.get('textarea').setValue(text)
      await m.w.get('textarea').trigger('keydown', { key: 'Enter' })
      await flushPromises()
      return m
    }
    const endRun = async (outcome: RunEnd['outcome']) => {
      a.runEnd.value = { token: 1, outcome }
      // The ending is handled on a promise chain, then the box re-renders: two turns.
      await flushPromises()
      await flushPromises()
    }

    beforeEach(() => {
      a.runEnd.value = null
      a.runToken.value = 0
      vi.spyOn(AssistantService, 'getActiveRuns').mockResolvedValue({ assistant_running: false, automation_running: false, assistant_conversation_id: '' })
    })

    it('is queued under the box instead of being sent, and does not cancel the run', async () => {
      const { w } = await queued()
      expect(a.sendMessage).not.toHaveBeenCalled()
      expect(a.cancel).not.toHaveBeenCalled()
      expect(w.get('[data-test="queued-message"]').text()).toContain('what about Leeds')
      expect((w.get('textarea').element as HTMLTextAreaElement).value).toBe('')
    })

    it('is sent once when the run completed', async () => {
      await queued()
      a.loading.value = false
      await endRun('completed')
      expect(a.sendMessage).toHaveBeenCalledExactlyOnceWith('ws', 'what about Leeds', { model: '', profile: '' })
    })

    it('goes back to the box, unsent, when the run failed', async () => {
      const { w } = await queued()
      a.loading.value = false
      await endRun('failed')
      expect(a.sendMessage).not.toHaveBeenCalled()
      expect((w.get('textarea').element as HTMLTextAreaElement).value).toBe('what about Leeds')
      expect(w.find('[data-test="queued-message"]').exists()).toBe(false)
    })

    it('is not sent if the server still has a run live, even though loading went false', async () => {
      await queued()
      vi.mocked(AssistantService.getActiveRuns).mockResolvedValue({ assistant_running: true, automation_running: false, assistant_conversation_id: 'c1' })
      a.loading.value = false
      await endRun('completed')
      expect(a.sendMessage).not.toHaveBeenCalled()
    })

    it('can be removed, which puts the text back in the box', async () => {
      const { w } = await queued()
      await w.get('[data-test="queued-message"] button').trigger('click')
      expect((w.get('textarea').element as HTMLTextAreaElement).value).toBe('what about Leeds')
      expect(w.find('[data-test="queued-message"]').exists()).toBe(false)
    })

    it('asks before a second message replaces the queued one', async () => {
      const { w } = await queued('first')
      confirm.mockResolvedValueOnce(false)
      await w.get('textarea').setValue('second')
      await w.get('textarea').trigger('keydown', { key: 'Enter' })
      await flushPromises()
      expect(w.get('[data-test="queued-message"]').text()).toContain('first')
      confirm.mockResolvedValueOnce(true)
      await w.get('textarea').trigger('keydown', { key: 'Enter' })
      await flushPromises()
      expect(w.get('[data-test="queued-message"]').text()).toContain('second')
    })
  })
})
