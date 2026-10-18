import { describe, expect, it, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const catalog = [
  { id: 'builder', name: 'Builder', summary: 'Builds.', source: 'builtin', answer_first: false, may_ask: true, memory_save: 'model', reminders: false, default_for: [] },
  { id: 'general', name: 'General', summary: 'Everyday assistant.', source: 'builtin', answer_first: true, may_ask: true, memory_save: 'model', reminders: true, default_for: ['assistant'] },
]
const profileService = vi.hoisted(() => ({ list: vi.fn() }))
vi.mock('../../../../services/profile/profileService', () => ({ ProfileService: profileService }))
const dispatcher = vi.hoisted(() => ({ getWorkspaceConfig: vi.fn(), updateWorkspaceConfig: vi.fn() }))
vi.mock('../../../../services/automation/dispatcherService', () => ({ DispatcherService: dispatcher }))

import WorkspaceProfileField from '../../../../components/AgentIde/workspace/WorkspaceProfileField.vue'

async function mountField(config: Record<string, unknown> = {}) {
  dispatcher.getWorkspaceConfig.mockResolvedValue(config)
  const w = mount(WorkspaceProfileField, { props: { workspaceId: 'ws' } })
  await flushPromises()
  return w
}

describe('WorkspaceProfileField', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    profileService.list.mockResolvedValue(catalog)
    dispatcher.updateWorkspaceConfig.mockResolvedValue(undefined)
    // useProfiles keeps its catalog for the page's lifetime: start each test from a fresh read
    vi.resetModules()
  })

  it('offers the default (named) and every profile, and shows what the default is for', async () => {
    const w = await mountField()
    const select = w.get('select[aria-label="Profile for new chats"]')
    expect(select.findAll('option').map((o) => o.text())).toEqual(['Default (General)', 'Builder', 'General'])
    expect((select.element as HTMLSelectElement).value).toBe('')
    expect(w.text()).toContain('Everyday assistant.')
  })

  it("shows the workspace's saved choice, and saves a change on its own", async () => {
    const w = await mountField({ assistant_profile: 'builder', assistant_memory: 'on' })
    const select = w.get('select')
    expect((select.element as HTMLSelectElement).value).toBe('builder')
    expect(w.text()).toContain('Builds.')

    await select.setValue('')
    await flushPromises()
    expect(dispatcher.updateWorkspaceConfig).toHaveBeenCalledWith('ws', { assistant_profile: '', assistant_memory: 'on' })
  })

  it('reads a choice whose profile no longer exists as the default', async () => {
    const w = await mountField({ assistant_profile: 'deleted-file' })
    expect((w.get('select').element as HTMLSelectElement).value).toBe('')
  })

  it('says why a save failed', async () => {
    dispatcher.updateWorkspaceConfig.mockRejectedValue(new Error('unknown assistant_profile "x"'))
    const w = await mountField()
    await w.get('select').setValue('builder')
    await flushPromises()
    expect(w.get('[role="alert"]').text()).toContain('unknown assistant_profile')
  })
})
