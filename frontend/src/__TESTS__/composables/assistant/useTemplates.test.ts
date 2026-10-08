import { describe, it, expect, vi, beforeEach } from 'vitest'
import { ref } from 'vue'

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
vi.mock('../../../composables/ui/useConfirm', () => ({
  useConfirm: () => ({ confirm: confirmMock }),
}))

const toast = { success: vi.fn(), error: vi.fn() }
vi.mock('../../../composables/useToast', () => ({ useToast: () => toast }))

import { useTemplates } from '../../../composables/assistant/useTemplates'

const template = { id: 'llm-ai-release-brief', name: 'Brief', category: 'research', description: '', content: 'NEW TEXT' }

function setup() {
  const fetchTree = vi.fn().mockResolvedValue(undefined)
  const openFile = vi.fn().mockResolvedValue(undefined)
  const { handleInjectTemplate } = useTemplates(
    ref('ws'), ref(null), ref(''), fetchTree, openFile,
  )
  return { handleInjectTemplate, fetchTree, openFile }
}

describe('useTemplates: creating a playbook file', () => {
  beforeEach(() => {
    readMock.mockReset().mockRejectedValue(new Error('not found'))
    treeMock.mockReset().mockResolvedValue({ entries: [], truncated: false })
    writeMock.mockReset().mockResolvedValue(undefined)
    confirmMock.mockReset().mockResolvedValue(true)
    toast.success.mockReset()
    toast.error.mockReset()
  })

  // The backend answers a read of a missing file with 200 and empty content, not an error: an empty file has
  // nothing to lose, so it is never "replaced" and never asked about (the file the operator just deleted).
  it('writes without asking when the backend reports a missing file as empty content', async () => {
    readMock.mockResolvedValue('')
    const { handleInjectTemplate, openFile } = setup()
    await handleInjectTemplate(template, 'create')
    expect(confirmMock).not.toHaveBeenCalled()
    expect(writeMock).toHaveBeenCalledWith('ws', 'llm-ai-release-brief.md', 'NEW TEXT')
    expect(openFile).toHaveBeenCalled()
  })

  it('writes without asking when the file does not exist', async () => {
    const { handleInjectTemplate, openFile } = setup()
    await handleInjectTemplate(template, 'create')
    expect(confirmMock).not.toHaveBeenCalled()
    expect(writeMock).toHaveBeenCalledWith('ws', 'llm-ai-release-brief.md', 'NEW TEXT')
    expect(openFile).toHaveBeenCalledWith('ws', 'llm-ai-release-brief.md')
  })

  it('asks before replacing a file that differs, and replaces it on confirm', async () => {
    readMock.mockResolvedValue('OLD TEXT')
    const { handleInjectTemplate } = setup()
    await handleInjectTemplate(template, 'create')
    expect(confirmMock).toHaveBeenCalledTimes(1)
    expect(confirmMock.mock.calls[0]![0].message).toContain('llm-ai-release-brief.md')
    expect(writeMock).toHaveBeenCalledWith('ws', 'llm-ai-release-brief.md', 'NEW TEXT')
  })

  it('leaves the existing file untouched when the user cancels', async () => {
    readMock.mockResolvedValue('OLD TEXT')
    confirmMock.mockResolvedValue(false)
    const { handleInjectTemplate, openFile } = setup()
    await handleInjectTemplate(template, 'create')
    expect(writeMock).not.toHaveBeenCalled()
    expect(openFile).not.toHaveBeenCalled()
  })

  it('does not ask or rewrite when the file already matches the playbook', async () => {
    readMock.mockResolvedValue('NEW TEXT')
    const { handleInjectTemplate, openFile } = setup()
    await handleInjectTemplate(template, 'create')
    expect(confirmMock).not.toHaveBeenCalled()
    expect(writeMock).not.toHaveBeenCalled()
    expect(openFile).toHaveBeenCalledWith('ws', 'llm-ai-release-brief.md')
  })

  // A failed read is not proof the file is missing: the tree says whether it exists, and when neither
  // answers nothing is written.
  it('asks before replacing a file it could list but not read', async () => {
    treeMock.mockResolvedValue({ entries: [{ path: 'llm-ai-release-brief.md', type: 'file' }], truncated: false })
    const { handleInjectTemplate } = setup()
    await handleInjectTemplate(template, 'create')
    expect(confirmMock).toHaveBeenCalledTimes(1)
    expect(writeMock).toHaveBeenCalled()
  })

  it('writes nothing when it cannot tell whether the file exists', async () => {
    treeMock.mockRejectedValue(new Error('backend down'))
    const { handleInjectTemplate } = setup()
    await handleInjectTemplate(template, 'create')
    expect(writeMock).not.toHaveBeenCalled()
    expect(toast.error).toHaveBeenCalledWith(expect.stringContaining('backend down'))
  })
})
