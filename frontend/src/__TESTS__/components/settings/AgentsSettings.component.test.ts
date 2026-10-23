import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { ProfileDetail, ProfileField, PromptProfile } from '../../../types/profile'

const base = { summary: 'Finds and cites.', answer_first: true, may_ask: true, memory_save: 'model' as const, reminders: false, default_for: [] }
const research: ProfileDetail = { ...base, id: 'research', name: 'Research', source: 'builtin', instructions: 'Cite sources.', delegates: ['web-researcher'], delegating_to: [], replaces_builtin: false }
const helper: ProfileDetail = { ...base, id: 'web-researcher', name: 'Web researcher', summary: 'Searches.', source: 'builtin', instructions: 'Search.', delegating_to: ['research'], replaces_builtin: false }
const scout: ProfileDetail = { ...base, id: 'scout', name: 'Scout', summary: 'Quick looks.', source: 'user', instructions: 'Look fast.', delegating_to: [], replaces_builtin: false }
const listed: PromptProfile[] = [research, helper, scout]
const schema: ProfileField[] = [
  { key: 'name', label: 'Name', kind: 'text', help: 'What the picker shows.', required: true },
  { key: 'instructions', label: 'Instructions', kind: 'longtext', help: 'How it behaves.', required: true },
  { key: 'answer_first', label: 'Answer first', kind: 'toggle', help: 'Plain replies are answers.' },
  { key: 'max_steps', label: 'Step limit', kind: 'number', help: 'Most steps.', max: 200 },
  { key: 'tools', label: 'Tools', kind: 'choices', help: 'Only these.', options: [{ value: 'search', label: 'Web search' }, { value: 'memory', label: 'Memory' }] },
  { key: 'memory_save', label: 'Saving to memory', kind: 'choice', help: 'Who saves.', options: [{ value: 'model', label: 'The model decides' }, { value: 'off', label: 'Nothing' }] },
  { key: 'delegates', label: 'Helpers', kind: 'profiles', help: 'Who it may hand tasks to.' },
  { key: 'summary', label: 'Summary', kind: 'text', help: 'One line.' },
]

const service = vi.hoisted(() => ({ list: vi.fn(), get: vi.fn(), schema: vi.fn(), save: vi.fn(), remove: vi.fn() }))
vi.mock('../../../services/profile/profileService', () => ({ ProfileService: service }))

import { HttpError } from '../../../services/httpClient'
import AgentsSettings from '../../../components/settings/AgentsSettings.vue'

async function mountSettings() {
  const w = mount(AgentsSettings, { attachTo: document.body })
  await flushPromises()
  return w
}

describe('AgentsSettings', () => {
  beforeEach(() => {
    for (const fn of Object.values(service)) fn.mockReset()
    service.list.mockResolvedValue(listed)
    service.schema.mockResolvedValue(schema)
    service.get.mockImplementation(async (id: string) => listed.find((p) => p.id === id))
  })

  it('lists the user\'s own agents first, then the built-in ones, each with what it is for', async () => {
    const w = await mountSettings()
    const text = w.text()
    expect(text.indexOf('Yours')).toBeLessThan(text.indexOf('Built-in'))
    expect(text.indexOf('Scout')).toBeLessThan(text.indexOf('Research'))
    expect(text).toContain('Quick looks.')
    expect(text).toContain('1 helper')
    expect(text).toContain('Pick an agent to read it')
  })

  it('opens a built-in read-only with a way to copy it, and no Save or Delete', async () => {
    const w = await mountSettings()
    await w.findAll('button').find((b) => b.text().includes('Research'))!.trigger('click')
    await flushPromises()
    expect(w.text()).toContain('Built-in agents are read-only')
    expect((w.get('input[type="text"]').element as HTMLInputElement).disabled).toBe(true)
    expect(w.text()).toContain('Duplicate')
    expect(w.text()).not.toContain('Save agent')
    expect(w.text()).not.toContain('Delete')
  })

  it('draws every kind of setting from the server\'s description and offers the other agents as helpers', async () => {
    const w = await mountSettings()
    await w.findAll('button').find((b) => b.text().includes('Scout'))!.trigger('click')
    await flushPromises()
    expect(w.find('textarea').exists()).toBe(true)
    expect(w.find('input[type="number"]').attributes('max')).toBe('200')
    expect(w.findAll('select option').map((o) => o.text())).toEqual(['The model decides', 'Nothing'])
    const group = (label: string) => w.get(`[role="group"][aria-label="${label}"]`)
    expect(group('Tools').findAll('label').map((l) => l.text())).toEqual(['Web search', 'Memory'])
    expect(group('Helpers').findAll('label').map((l) => l.text())).toEqual(['Research', 'Web researcher'])
  })

  it('saves an edit, and shows the server\'s refusal under the setting it names', async () => {
    const w = await mountSettings()
    await w.findAll('button').find((b) => b.text().includes('Scout'))!.trigger('click')
    await flushPromises()
    expect(w.text()).toContain('Save agent')
    expect(w.findAll('button').find((b) => b.text() === 'Save agent')!.attributes('disabled')).toBeDefined()
    await w.get('input[type="text"]').setValue('Scout 2')
    service.save.mockRejectedValueOnce(new HttpError('name is required', 'name'))
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(w.get('[role="alert"]').text()).toContain('name is required')
    service.save.mockResolvedValueOnce({ ...scout, name: 'Scout 2' })
    await w.get('form').trigger('submit')
    await flushPromises()
    expect(service.save).toHaveBeenLastCalledWith('scout', expect.objectContaining({ name: 'Scout 2' }))
    expect(w.find('[role="alert"]').exists()).toBe(false)
  })

  it('starts a new agent with an id made from its name, and tells the page about unsaved edits', async () => {
    const w = await mountSettings()
    await w.findAll('button').find((b) => b.text().includes('New agent'))!.trigger('click')
    await flushPromises()
    const inputs = w.findAll('input[type="text"]')
    await inputs[0]!.setValue('My Scout')
    expect((inputs[inputs.length - 1]!.element as HTMLInputElement).value).toBe('my-scout')
    const reports = w.emitted('dirty-change') ?? []
    expect(reports[reports.length - 1]).toEqual([true])
  })

  it('shows one pane at a time on a phone: the list hides while an agent is open, with a way back', async () => {
    const w = await mountSettings()
    const panes = () => w.findAll('.grid > div')
    expect(panes()[0]!.classes()).not.toContain('hidden')
    expect(panes()[1]!.classes()).toContain('hidden')
    await w.findAll('button').find((b) => b.text().includes('Scout'))!.trigger('click')
    await flushPromises()
    expect(panes()[0]!.classes()).toContain('hidden')
    expect(panes()[1]!.classes()).not.toContain('hidden')
    expect(w.findAll('button').some((b) => b.text() === 'Agents')).toBe(true)
  })
})
