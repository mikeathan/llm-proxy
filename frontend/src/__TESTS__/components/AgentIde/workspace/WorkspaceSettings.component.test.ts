import { describe, expect, it, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { DispatcherService } from '../../../../services/automation/dispatcherService'
import { DEFAULT_CONFIG } from '../../../../composables/models/useConfig'
import { fieldByLabel } from '../../../helpers/fieldByLabel'
import type { AgentGuardrailsConfig } from '../../../../types/admin'

vi.mock('../../../../services/automation/dispatcherService', () => ({
  DispatcherService: { getWorkspaceConfig: vi.fn(), updateWorkspaceConfig: vi.fn() },
}))
const toast = { success: vi.fn(), error: vi.fn(), info: vi.fn() }
vi.mock('../../../../composables/useToast', () => ({ useToast: () => toast }))
const confirm = vi.fn()
vi.mock('../../../../composables/ui/useConfirm', () => ({ useConfirm: () => ({ confirm }) }))

import WorkspaceSettings from '../../../../components/AgentIde/workspace/WorkspaceSettings.vue'

const GLOBAL: AgentGuardrailsConfig = {
  ...structuredClone(DEFAULT_CONFIG.guardrails),
  terminal: { ...DEFAULT_CONFIG.guardrails.terminal, enabled: true, allowed_commands: ['ls'] },
}

async function mountSettings(config: Record<string, unknown>) {
  vi.mocked(DispatcherService.getWorkspaceConfig).mockResolvedValue(structuredClone(config))
  const onDirty = vi.fn()
  const w = mount(WorkspaceSettings, {
    props: { workspaceId: 'ws', globalGuardrails: GLOBAL },
    attrs: { onDirtyChange: onDirty },
    global: { stubs: { Icon: true } },
  })
  await flushPromises()
  const button = (name: RegExp) => w.findAll('button').find((b) => name.test(b.text()))
  return { w, onDirty, button }
}
const saved = () => vi.mocked(DispatcherService.updateWorkspaceConfig).mock.lastCall![1] as Record<string, unknown> & { guardrails?: AgentGuardrailsConfig }

// Characterised against the old WorkspaceSettings first (plan D22), then moved
// to the layered policy page.
describe('WorkspaceSettings', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(DispatcherService.updateWorkspaceConfig).mockResolvedValue()
    confirm.mockResolvedValue(true)
  })

  it('loads the workspace configuration', async () => {
    await mountSettings({ cron_schedule: '' })
    expect(DispatcherService.getWorkspaceConfig).toHaveBeenCalledWith('ws')
  })

  it('says a workspace without its own policy uses the global one, and can start one', async () => {
    const { w, button } = await mountSettings({ cron_schedule: '0 * * * *' })
    expect(w.text()).toMatch(/uses the global policy/i)
    expect(w.find('textarea').exists()).toBe(false)
    await button(/customise for this workspace/i)!.trigger('click')
    await fieldByLabel(w, /allowed commands/i).setValue('git')
    await button(/^save workspace policy$/i)!.trigger('click')
    await flushPromises()
    expect(saved().cron_schedule).toBe('0 * * * *')
    expect(saved().guardrails!.terminal.allowed_commands).toEqual(['git'])
    expect(toast.success).toHaveBeenCalled()
  })

  it('reads an old full-copy policy as only what the workspace adds', async () => {
    const stored = structuredClone(GLOBAL)
    stored.terminal.allowed_commands = ['ls', 'git']
    const { w } = await mountSettings({ guardrails: stored })
    expect((fieldByLabel(w, /allowed commands/i).element as HTMLTextAreaElement).value).toBe('git')
    expect(w.get('[data-test="policy-summary"]').text()).toMatch(/1 exception/)
  })

  it('offers Save and Discard only for changes, and reports them to the page', async () => {
    const { w, button, onDirty } = await mountSettings({ guardrails: structuredClone(GLOBAL) })
    expect(button(/^save workspace policy$/i)!.attributes('disabled')).toBeDefined()
    await fieldByLabel(w, /allowed commands/i).setValue('git')
    expect(onDirty).toHaveBeenLastCalledWith(true)
    await button(/discard changes/i)!.trigger('click')
    expect((fieldByLabel(w, /allowed commands/i).element as HTMLTextAreaElement).value).toBe('')
    expect(onDirty).toHaveBeenLastCalledWith(false)
  })

  it('resets to the global policy only after confirmation, removing the layer', async () => {
    const { w, button } = await mountSettings({ cron_schedule: 'x', guardrails: structuredClone(GLOBAL) })
    confirm.mockResolvedValueOnce(false)
    await button(/reset to global policy/i)!.trigger('click')
    await flushPromises()
    expect(DispatcherService.updateWorkspaceConfig).not.toHaveBeenCalled()
    await button(/reset to global policy/i)!.trigger('click')
    await flushPromises()
    expect(saved()).toEqual({ cron_schedule: 'x' })
    expect(w.text()).toMatch(/uses the global policy/i)
  })

  it('warns when the workspace reaches paths outside its directory', async () => {
    const guardrails = structuredClone(GLOBAL)
    guardrails.terminal.allowed_external_paths = ['/mnt/data']
    const { w } = await mountSettings({ guardrails })
    expect(w.text()).toMatch(/external/i)
    expect(w.text()).toContain('/mnt/data')
  })

  it('offers a retry when the configuration cannot be loaded', async () => {
    vi.mocked(DispatcherService.getWorkspaceConfig).mockRejectedValueOnce(new Error('gone'))
    const w = mount(WorkspaceSettings, { props: { workspaceId: 'ws', globalGuardrails: GLOBAL }, global: { stubs: { Icon: true } } })
    await flushPromises()
    expect(w.text()).toContain('gone')
    vi.mocked(DispatcherService.getWorkspaceConfig).mockResolvedValueOnce({})
    await w.findAll('button').find((b) => /retry/i.test(b.text()))!.trigger('click')
    await flushPromises()
    expect(w.text()).toMatch(/uses the global policy/i)
  })
})
