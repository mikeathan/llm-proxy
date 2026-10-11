import { describe, it, expect, vi, beforeEach } from 'vitest'
import { flushPromises } from '@vue/test-utils'

const { listMock, getMock, writeMock, confirmMock } = vi.hoisted(() => ({ listMock: vi.fn(), getMock: vi.fn(), writeMock: vi.fn(), confirmMock: vi.fn() }))
vi.mock('../../../services/template/templateService', () => ({ TemplateService: { listTemplates: listMock, getTemplate: getMock } }))
vi.mock('../../../composables/editor/useWorkspaceFileWriter', () => ({ useWorkspaceFileWriter: () => ({ writeFile: writeMock }) }))
vi.mock('../../../composables/ui/useConfirm', () => ({ useConfirm: () => ({ confirm: confirmMock }) }))

import { useTaskAuthoring, taskFileName } from '../../../composables/automation/useTaskAuthoring'

const BRIEF = { id: 'brief', name: 'Brief', category: 'research', description: '', content: 'BRIEF TEXT' }

describe('useTaskAuthoring', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    listMock.mockResolvedValue([{ id: 'brief', name: 'Brief', category: 'research' }])
    getMock.mockResolvedValue(BRIEF)
    writeMock.mockResolvedValue('created')
    confirmMock.mockResolvedValue(true)
  })

  it('names the task file after the automation', () => {
    expect(taskFileName('morning-news')).toBe('morning-news.md')
  })

  it('starts on an existing file with nothing written', () => {
    const t = useTaskAuthoring()
    expect(t.mode.value).toBe('existing')
    expect(t.content.value).toBe('')
  })

  it('lists the playbooks once, however often it is asked', async () => {
    const t = useTaskAuthoring()
    await t.loadPlaybooks()
    await t.loadPlaybooks()
    expect(listMock).toHaveBeenCalledTimes(1)
    expect(t.playbooks.value.map((p) => p.id)).toEqual(['brief'])
  })

  it('says so when the playbooks cannot be listed, and tries again next time', async () => {
    listMock.mockRejectedValueOnce(new Error('offline'))
    const t = useTaskAuthoring()
    await t.loadPlaybooks()
    expect(t.playbooksFailed.value).toBe(true)
    await t.loadPlaybooks()
    expect(t.playbooksFailed.value).toBe(false)
    expect(t.playbooks.value).toHaveLength(1)
  })

  it('fills the text from a chosen playbook', async () => {
    const t = useTaskAuthoring()
    expect(await t.choosePlaybook('brief')).toBe(true)
    expect(t.content.value).toBe('BRIEF TEXT')
    expect(t.playbookId.value).toBe('brief')
    expect(confirmMock).not.toHaveBeenCalled()
  })

  it('asks before a playbook replaces text the user wrote, and keeps it on no', async () => {
    const t = useTaskAuthoring()
    t.content.value = 'my own task'
    confirmMock.mockResolvedValue(false)
    expect(await t.choosePlaybook('brief')).toBe(false)
    expect(confirmMock).toHaveBeenCalled()
    expect(t.content.value).toBe('my own task')
    expect(t.playbookId.value).toBe('')
  })

  it('swaps one playbook for another without asking while the text is still the first one', async () => {
    const t = useTaskAuthoring()
    await t.choosePlaybook('brief')
    getMock.mockResolvedValue({ ...BRIEF, id: 'other', content: 'OTHER' })
    await t.choosePlaybook('other')
    expect(confirmMock).not.toHaveBeenCalled()
    expect(t.content.value).toBe('OTHER')
  })

  it('asks again once the user has edited a playbook\'s text', async () => {
    const t = useTaskAuthoring()
    await t.choosePlaybook('brief')
    t.content.value = 'BRIEF TEXT, edited'
    await t.choosePlaybook('brief')
    expect(confirmMock).toHaveBeenCalledTimes(1)
  })

  it('keeps the text and reports a playbook that cannot be read', async () => {
    getMock.mockRejectedValue(new Error('gone'))
    const t = useTaskAuthoring()
    t.content.value = ''
    await expect(t.choosePlaybook('brief')).rejects.toThrow('gone')
    expect(t.content.value).toBe('')
  })

  it('saves the text as <name>.md and returns that file', async () => {
    const t = useTaskAuthoring()
    t.content.value = 'do the thing'
    expect(await t.saveTask('ws', 'morning')).toBe('morning.md')
    expect(writeMock).toHaveBeenCalledWith('ws', 'morning.md', 'do the thing')
  })

  it('returns nothing when replacing an existing file was declined', async () => {
    writeMock.mockResolvedValue('cancelled')
    const t = useTaskAuthoring()
    t.content.value = 'do the thing'
    expect(await t.saveTask('ws', 'morning')).toBeNull()
  })

  it('does not report a file when the write fails', async () => {
    writeMock.mockRejectedValue(new Error('disk'))
    const t = useTaskAuthoring()
    t.content.value = 'x'
    await expect(t.saveTask('ws', 'morning')).rejects.toThrow('disk')
  })

  it('goes back to the start on reset', async () => {
    const t = useTaskAuthoring()
    await t.choosePlaybook('brief')
    t.mode.value = 'new'
    t.reset()
    await flushPromises()
    expect([t.mode.value, t.content.value, t.playbookId.value]).toEqual(['existing', '', ''])
  })
})
