import { describe, it, expect, vi, beforeEach } from 'vitest'
import { ref } from 'vue'

const getActiveRunsMock = vi.fn()

vi.mock('../../../services/assistant/assistantService', () => ({
  AssistantService: {
    getActiveRuns: (...args: unknown[]) => getActiveRunsMock(...args),
  },
}))

import { useRunningActivity } from '../../../composables/assistant/useRunningActivity'

function workspaceRef(id: string | null) {
  return ref<string | null>(id)
}

describe('useRunningActivity', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('exposes the workspace-scoped running state', async () => {
    getActiveRunsMock.mockResolvedValue({
      assistant_running: true,
      automation_running: false,
      assistant_conversation_id: 'conv_1',
      assistant_queued: true,
    })

    const { assistantRunning, assistantQueued, assistantConversationId } = useRunningActivity(workspaceRef('ws'))

    await vi.waitFor(() => {
      expect(assistantQueued.value).toBe(true)
    })
    expect(assistantRunning.value).toBe(true)
    expect(assistantConversationId.value).toBe('conv_1')
  })

  it('defaults the queued flag when the payload omits it', async () => {
    getActiveRunsMock.mockResolvedValue({
      assistant_running: false,
      automation_running: false,
      assistant_conversation_id: '',
    })

    const ws = workspaceRef(null)
    const { assistantQueued } = useRunningActivity(ws)
    ws.value = 'ws'

    await vi.waitFor(() => {
      expect(getActiveRunsMock).toHaveBeenCalled()
    })
    expect(assistantQueued.value).toBe(false)
  })
})
