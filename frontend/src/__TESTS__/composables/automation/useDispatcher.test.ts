import { describe, it, expect, vi, beforeEach } from 'vitest'

const { triggerAutomationMock, cancelQueuedMock, listAutomationsMock, listWorkspaceTreeMock, show, clear } = vi.hoisted(() => ({
  listWorkspaceTreeMock: vi.fn(),
  triggerAutomationMock: vi.fn(),
  cancelQueuedMock: vi.fn(),
  listAutomationsMock: vi.fn(),
  show: vi.fn(),
  clear: vi.fn(),
}))

vi.mock('../../../services/automation/dispatcherService', () => ({
  DispatcherService: {
    triggerAutomation: triggerAutomationMock,
    cancelQueued: cancelQueuedMock,
    listAutomations: listAutomationsMock,
    listWorkspaceTree: listWorkspaceTreeMock,
  },
}))

vi.mock('../../../composables/ui/useAppBanner', () => ({
  useAppBanner: () => ({ show, clear }),
}))

import { useDispatcher } from '../../../composables/automation/useDispatcher'

describe('useDispatcher', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    listAutomationsMock.mockResolvedValue([])
  })

  it('cancelQueued calls the service, refetches automations and stays silent on success', async () => {
    cancelQueuedMock.mockResolvedValue(undefined)

    await useDispatcher().cancelQueued('ws', 'nightly')

    expect(cancelQueuedMock).toHaveBeenCalledWith('ws', 'nightly')
    expect(listAutomationsMock).toHaveBeenCalled()
    expect(show).not.toHaveBeenCalled()
  })

  it('cancelQueued surfaces an error banner and rethrows on failure', async () => {
    cancelQueuedMock.mockRejectedValue(new Error('boom'))

    await expect(useDispatcher().cancelQueued('ws', 'nightly')).rejects.toThrow('boom')

    expect(show).toHaveBeenCalledWith({ severity: 'error', message: 'boom' })
  })

  it('triggerAutomation announces the queue position when the run waits', async () => {
    triggerAutomationMock.mockResolvedValue({ status: 'queued', position: 2, workspace: 'ws', automation: 'nightly' })

    await useDispatcher().triggerAutomation('ws', 'nightly')

    expect(show).toHaveBeenCalledWith({
      severity: 'notice',
      message: 'Queued #2 — nightly starts when the lane frees',
    })
  })

  it('triggerAutomation stays silent when the run starts immediately', async () => {
    triggerAutomationMock.mockResolvedValue({ status: 'started', workspace: 'ws', automation: 'nightly' })

    await useDispatcher().triggerAutomation('ws', 'nightly')

    expect(show).not.toHaveBeenCalled()
  })

  it('fetchWorkspaceTree keeps the tree and derives the nested file list from it', async () => {
    listWorkspaceTreeMock.mockResolvedValue({
      entries: [{ path: 'docs', type: 'dir' }, { path: 'docs/plan.md', type: 'file' }, { path: 'task.md', type: 'file' }],
      truncated: false,
    })
    const { fetchWorkspaceTree, workspaceTrees, workspaceFiles } = useDispatcher()
    await fetchWorkspaceTree('ws')
    expect(workspaceTrees.value.ws?.entries).toHaveLength(3)
    expect(workspaceFiles.value.ws).toEqual(['docs/plan.md', 'task.md'])
  })
})
