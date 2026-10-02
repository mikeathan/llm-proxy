import { describe, it, expect, vi, beforeEach } from 'vitest'

const { triggerAutomationMock, cancelQueuedMock, deleteAutomationMock, listAutomationsMock, listWorkspaceTreeMock, deleteWorkspacePathMock, show, clear } = vi.hoisted(() => ({
  listWorkspaceTreeMock: vi.fn(),
  deleteWorkspacePathMock: vi.fn(),
  triggerAutomationMock: vi.fn(),
  deleteAutomationMock: vi.fn(),
  cancelQueuedMock: vi.fn(),
  listAutomationsMock: vi.fn(),
  show: vi.fn(),
  clear: vi.fn(),
}))

vi.mock('../../../services/automation/dispatcherService', () => ({
  DispatcherService: {
    triggerAutomation: triggerAutomationMock,
    cancelQueued: cancelQueuedMock,
    deleteAutomation: deleteAutomationMock,
    listAutomations: listAutomationsMock,
    listWorkspaceTree: listWorkspaceTreeMock,
    deleteWorkspacePath: deleteWorkspacePathMock,
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

  it('deleteWorkspacePaths deletes each outermost path, refreshes the tree once and returns what went', async () => {
    deleteWorkspacePathMock.mockResolvedValue(undefined)
    listWorkspaceTreeMock.mockResolvedValue({ entries: [], truncated: false })

    const deleted = await useDispatcher().deleteWorkspacePaths('ws', ['docs', 'docs/a.md', 'plan.md'])

    expect(deleteWorkspacePathMock.mock.calls).toEqual([['ws', 'docs'], ['ws', 'plan.md']])
    expect(listWorkspaceTreeMock).toHaveBeenCalledTimes(1)
    expect(deleted).toEqual(['docs', 'plan.md'])
    expect(show).not.toHaveBeenCalled()
  })

  it('deleteWorkspacePaths keeps going past a failure and names it in one banner', async () => {
    deleteWorkspacePathMock.mockImplementation(async (_ws: string, path: string) => {
      if (path === 'locked.md') throw new Error('permission denied')
    })
    listWorkspaceTreeMock.mockResolvedValue({ entries: [], truncated: false })

    const deleted = await useDispatcher().deleteWorkspacePaths('ws', ['locked.md', 'plan.md'])

    expect(deleted).toEqual(['plan.md'])
    expect(show).toHaveBeenCalledTimes(1)
    expect(show).toHaveBeenCalledWith({ severity: 'error', message: 'Could not delete 1 of 2: locked.md (permission denied)' })
  })

  it('deleteAutomations keeps going past a failure, names it in one banner and refreshes once', async () => {
    deleteAutomationMock.mockImplementation(async (_ws: string, name: string) => {
      if (name === 'busy') throw new Error('workspace is running')
    })
    listAutomationsMock.mockResolvedValue([])

    const deleted = await useDispatcher().deleteAutomations([
      { workspace: 'ws', name: 'busy' },
      { workspace: 'ws', name: 'old' },
    ])

    expect(deleted).toEqual(['old'])
    expect(listAutomationsMock).toHaveBeenCalledTimes(1)
    expect(show).toHaveBeenCalledTimes(1)
    expect(show).toHaveBeenCalledWith({ severity: 'error', message: 'Could not delete 1 of 2: busy (workspace is running)' })
  })
})
