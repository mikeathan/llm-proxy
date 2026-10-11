import { describe, it, expect, vi, beforeEach } from 'vitest'

const readMock = vi.fn()
const writeMock = vi.fn()
const treeMock = vi.fn()
vi.mock('../../../services/automation/dispatcherService', () => ({
  DispatcherService: {
    readWorkspaceFile: (...args: unknown[]) => readMock(...args),
    writeWorkspaceFile: (...args: unknown[]) => writeMock(...args),
    listWorkspaceTree: (...args: unknown[]) => treeMock(...args),
  },
}))
const confirmMock = vi.fn()
vi.mock('../../../composables/ui/useConfirm', () => ({ useConfirm: () => ({ confirm: confirmMock }) }))

import { useWorkspaceFileWriter } from '../../../composables/editor/useWorkspaceFileWriter'

describe('useWorkspaceFileWriter', () => {
  beforeEach(() => {
    readMock.mockReset().mockRejectedValue(new Error('not found'))
    treeMock.mockReset().mockResolvedValue({ entries: [], truncated: false })
    writeMock.mockReset().mockResolvedValue(undefined)
    confirmMock.mockReset().mockResolvedValue(true)
  })

  const write = (content = 'NEW') => useWorkspaceFileWriter().writeFile('ws', 'task.md', content)

  it('creates a file that does not exist, without asking', async () => {
    expect(await write()).toBe('created')
    expect(confirmMock).not.toHaveBeenCalled()
    expect(writeMock).toHaveBeenCalledWith('ws', 'task.md', 'NEW')
  })

  // The backend answers a read of a missing file with 200 and empty content.
  it('treats an empty file as nothing to lose', async () => {
    readMock.mockResolvedValue('')
    expect(await write()).toBe('created')
    expect(confirmMock).not.toHaveBeenCalled()
  })

  it('leaves a file that already holds exactly this text', async () => {
    readMock.mockResolvedValue('NEW')
    expect(await write()).toBe('unchanged')
    expect(writeMock).not.toHaveBeenCalled()
  })

  it('asks before replacing different content, and replaces on yes', async () => {
    readMock.mockResolvedValue('OLD')
    expect(await write()).toBe('replaced')
    expect(confirmMock).toHaveBeenCalledWith(expect.objectContaining({ title: 'Replace existing file?', message: expect.stringContaining('task.md') }))
    expect(writeMock).toHaveBeenCalledWith('ws', 'task.md', 'NEW')
  })

  it('writes nothing when the replacement is declined', async () => {
    readMock.mockResolvedValue('OLD')
    confirmMock.mockResolvedValue(false)
    expect(await write()).toBe('cancelled')
    expect(writeMock).not.toHaveBeenCalled()
  })

  // A failed read is not proof the file is absent: the listing decides, and when
  // the file is listed but unreadable it is treated as something to lose.
  it('asks when the file is listed but cannot be read', async () => {
    treeMock.mockResolvedValue({ entries: [{ type: 'file', path: 'task.md' }], truncated: false })
    await write()
    expect(confirmMock).toHaveBeenCalled()
  })

  it('writes nothing and reports the error when the listing fails too', async () => {
    treeMock.mockRejectedValue(new Error('offline'))
    await expect(write()).rejects.toThrow('offline')
    expect(writeMock).not.toHaveBeenCalled()
  })
})
