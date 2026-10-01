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
  sendMessage: vi.fn(),
  deleteSession: vi.fn(),
  deleteSessionsByIds: vi.fn(),
  cancelSession: vi.fn(),
  connectSSE: vi.fn(),
  activeWorkspaceId: ref<string | null>(null),
  cancel: vi.fn(),
  liveEvents: ref([]),
}
vi.mock('../../../../composables/assistant/useAssistant', () => ({ useAssistant: () => a }))
const confirm = vi.fn()
vi.mock('../../../../composables/ui/useConfirm', () => ({ useConfirm: () => ({ confirm }) }))

import AssistantChat from '../../../../components/AgentIde/assistant/AssistantChat.vue'

function mountChat() {
  const w = mount(AssistantChat, { props: { workspaceId: 'ws' }, global: { stubs: { Icon: true, ChatMessages: true, GuardrailBanner: true } } })
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
})
