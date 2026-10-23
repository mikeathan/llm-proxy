import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ProfileDetail, ProfileField, PromptProfile } from '../../../types/profile'

const service = vi.hoisted(() => ({ list: vi.fn(), get: vi.fn(), schema: vi.fn(), save: vi.fn(), remove: vi.fn() }))
vi.mock('../../../services/profile/profileService', () => ({ ProfileService: service }))
const confirmMock = vi.hoisted(() => vi.fn())
vi.mock('../../../composables/ui/useConfirm', () => ({ useConfirm: () => ({ confirm: confirmMock }) }))

const base = { summary: 's', answer_first: true, may_ask: true, memory_save: 'model' as const, reminders: false, default_for: [] }
const research: ProfileDetail = { ...base, id: 'research', name: 'Research', source: 'builtin', instructions: 'Cite.', delegates: ['web-researcher'], delegating_to: [], replaces_builtin: false }
const scout: ProfileDetail = { ...base, id: 'scout', name: 'Scout', source: 'user', instructions: 'Look.', delegating_to: ['boss'], replaces_builtin: false }
const listed: PromptProfile[] = [research, scout]
const schema: ProfileField[] = [{ key: 'name', label: 'Name', kind: 'text', help: 'h', required: true }]

async function editor() {
  vi.resetModules()
  const mod = await import('../../../composables/settings/useAgentsEditor')
  const e = mod.useAgentsEditor()
  await vi.waitFor(() => expect(e.profiles.value).toHaveLength(2))
  return e
}

describe('useAgentsEditor', () => {
  beforeEach(() => {
    for (const fn of Object.values(service)) fn.mockReset()
    confirmMock.mockReset().mockResolvedValue(true)
    service.list.mockResolvedValue(listed)
    service.schema.mockResolvedValue(schema)
    service.get.mockImplementation(async (id: string) => (id === 'scout' ? scout : research))
  })

  it('opens a built-in read-only and a user agent editable', async () => {
    const e = await editor()
    await e.select('research')
    expect(e.readOnly.value).toBe(true)
    expect(e.dirty.value).toBe(false)
    await e.select('scout')
    expect(e.readOnly.value).toBe(false)
    e.setField('summary', 'changed')
    expect(e.dirty.value).toBe(true)
    e.discard()
    expect(e.dirty.value).toBe(false)
  })

  it('duplicates a built-in under a free id and saves it as a new agent', async () => {
    const e = await editor()
    await e.select('research')
    await e.duplicate()
    expect(e.creating.value).toBe(true)
    expect(e.newId.value).toBe('research-copy')
    expect(e.draft.value.name).toBe('Research (copy)')
    service.save.mockResolvedValue({ ...scout, id: 'research-copy', name: 'Research (copy)' })
    await e.save()
    expect(service.save).toHaveBeenCalledWith('research-copy', expect.objectContaining({ name: 'Research (copy)', delegates: ['web-researcher'] }))
    expect(e.creating.value).toBe(false)
    expect(e.detail.value?.id).toBe('research-copy')
    expect(service.list).toHaveBeenCalledTimes(2)
  })

  it("makes a new agent's id from its name until the person types one", async () => {
    const e = await editor()
    await e.startNew()
    e.setField('name', 'My Scout')
    expect(e.newId.value).toBe('my-scout')
    e.setId('mine')
    e.setField('name', 'Other')
    expect(e.newId.value).toBe('mine')
  })

  it("shows the server's refusal beside the setting it names, and clears it once that setting changes", async () => {
    const e = await editor()
    const { HttpError } = await import('../../../services/httpClient') // the module graph editor() just reset
    await e.select('scout')
    e.setField('delegates', ['ghost'])
    service.save.mockRejectedValue(new HttpError('delegates: there is no profile "ghost"', 'delegates'))
    await e.save()
    expect(e.fieldErrors.value).toEqual({ delegates: 'delegates: there is no profile "ghost"' })
    expect(e.formError.value).toBe('')
    e.setField('delegates', [])
    expect(e.fieldErrors.value.delegates).toBe('')
    service.save.mockRejectedValue(new HttpError('could not write the profile'))
    await e.save()
    expect(e.formError.value).toBe('could not write the profile')
  })

  it('asks before leaving edits, and stays when declined', async () => {
    const e = await editor()
    await e.select('scout')
    e.setField('summary', 'changed')
    confirmMock.mockResolvedValueOnce(false)
    await e.select('research')
    expect(e.detail.value?.id).toBe('scout')
    expect(e.draft.value.summary).toBe('changed')
    await e.select('research')
    expect(e.detail.value?.id).toBe('research')
  })

  it('names who still lists an agent when deleting it, and reopens the built-in a file had replaced', async () => {
    const e = await editor()
    await e.select('scout')
    service.remove.mockResolvedValue({ restored_builtin: false, delegating_to: ['boss'] })
    await e.remove()
    expect(confirmMock).toHaveBeenCalledWith(expect.objectContaining({ title: 'Delete Scout?', message: expect.stringContaining('boss') }))
    expect(e.detail.value).toBeNull()
    const replaced = { ...research, source: 'user' as const, replaces_builtin: true }
    service.get.mockResolvedValueOnce(replaced)
    await e.select('research')
    service.get.mockResolvedValue(research)
    service.remove.mockResolvedValue({ restored_builtin: true, delegating_to: [] })
    await e.remove()
    expect(e.detail.value?.source).toBe('builtin')
  })
})
