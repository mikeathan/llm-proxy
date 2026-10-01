import { describe, it, expect, vi, beforeEach } from 'vitest'

const readMock = vi.fn()
const writeMock = vi.fn()
vi.mock('../../../services/automation/dispatcherService', () => ({
  DispatcherService: {
    readWorkspaceFile: (...args: unknown[]) => readMock(...args),
    writeWorkspaceFile: (...args: unknown[]) => writeMock(...args),
  },
}))

import { useFileEditor } from '../../../composables/editor/useFileEditor'

describe('useFileEditor dirty tracking', () => {
  beforeEach(() => {
    readMock.mockReset().mockResolvedValue('saved text')
    writeMock.mockReset().mockResolvedValue(undefined)
  })

  it('is clean after loading and dirty after an edit', async () => {
    const editor = useFileEditor({ error: vi.fn() })
    await editor.handleOpenFile('ws', 'a.md')
    expect(editor.isDirty.value).toBe(false)
    editor.fileContent.value = 'changed'
    expect(editor.isDirty.value).toBe(true)
  })

  it('is clean again after a successful save, and stays dirty when saving fails', async () => {
    const toast = { error: vi.fn() }
    const editor = useFileEditor(toast)
    await editor.handleOpenFile('ws', 'a.md')
    editor.fileContent.value = 'changed'
    writeMock.mockRejectedValueOnce(new Error('disk full'))
    await editor.handleSaveFile()
    expect(editor.isDirty.value).toBe(true)
    await editor.handleSaveFile()
    expect(editor.isDirty.value).toBe(false)
  })

  it('does not count a failed load or a closed file as unsaved', async () => {
    readMock.mockRejectedValueOnce(new Error('missing'))
    const editor = useFileEditor({ error: vi.fn() })
    await editor.handleOpenFile('ws', 'gone.md')
    expect(editor.isDirty.value).toBe(false)
    editor.fileContent.value = 'x'
    editor.closeFile()
    expect(editor.isDirty.value).toBe(false)
  })
})
