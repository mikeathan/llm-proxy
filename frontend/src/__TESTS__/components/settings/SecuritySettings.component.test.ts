import { describe, it, expect, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { AdminApiService } from '../../../services/admin/adminService'
import type { HostSettings, SandboxingConfig } from '../../../types/admin'

vi.mock('../../../services/admin/adminService', () => ({
  AdminApiService: {
    fetchHostSettings: vi.fn(),
    updateHostSettings: vi.fn(),
    clearRuntimeData: vi.fn(),
    factoryReset: vi.fn(),
    wipeout: vi.fn(),
    restartSystem: vi.fn(),
  },
}))
const confirm = vi.fn()
vi.mock('../../../composables/ui/useConfirm', () => ({ useConfirm: () => ({ confirm }) }))
const toast = { success: vi.fn(), error: vi.fn(), info: vi.fn() }
vi.mock('../../../composables/useToast', () => ({ useToast: () => toast }))

import SecuritySettings from '../../../components/settings/SecuritySettings.vue'
import BaseToggle from '../../../components/common/buttons/BaseToggle.vue'

const SANDBOX: SandboxingConfig = {
  enabled: true,
  functional: true,
  max_memory_mb: 2048,
  max_storage_gb: 10,
  filesystem: true,
  network: true,
  egress_proxy: 0,
}
const settings = (patch: Partial<SandboxingConfig> = {}): HostSettings => ({ sandboxing: { ...SANDBOX, ...patch } })

async function mountSecurity(host = settings(), active = true) {
  vi.mocked(AdminApiService.fetchHostSettings).mockResolvedValue(structuredClone(host))
  const onDirty = vi.fn()
  const w = mount(SecuritySettings, {
    props: { active },
    attrs: { onDirtyChange: onDirty },
    global: { stubs: { TerminalMonitor: true, Icon: true } },
  })
  await flushPromises()
  // Switch order on the page: master, workspace file confinement, agent network.
  const [master, filesystem, network] = w.findAllComponents(BaseToggle)
  const save = () => w.findAll('button').find((b) => /save/i.test(b.text()))!
  return { w, onDirty, master: master!, filesystem: filesystem!, network: network!, save }
}

describe('SecuritySettings', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(AdminApiService.updateHostSettings).mockImplementation(async (h) => h)
  })

  it('loads the host policy and offers nothing to save until it changes', async () => {
    const { save } = await mountSecurity()
    expect(AdminApiService.fetchHostSettings).toHaveBeenCalledTimes(1)
    expect(save().attributes('disabled')).toBeDefined()
  })

  it('turns the master off only after confirmation, then saves the whole policy', async () => {
    const { master, save } = await mountSecurity()
    confirm.mockResolvedValueOnce(false)
    master.vm.$emit('update:modelValue', false)
    await flushPromises()
    expect(save().attributes('disabled')).toBeDefined()

    confirm.mockResolvedValueOnce(true)
    master.vm.$emit('update:modelValue', false)
    await flushPromises()
    await save().trigger('click')
    await flushPromises()
    expect(AdminApiService.updateHostSettings).toHaveBeenCalledWith({ sandboxing: expect.objectContaining({ enabled: false }) })
    expect(toast.success).toHaveBeenCalled()
  })

  it('confirms restricting the agent network as a destructive change', async () => {
    const { network } = await mountSecurity()
    confirm.mockResolvedValueOnce(true)
    network.vm.$emit('update:modelValue', false)
    await flushPromises()
    expect(confirm).toHaveBeenCalledWith(expect.objectContaining({ type: 'error' }))
  })

  it('lets a legacy install decide the network switch from its banner', async () => {
    const { w, save } = await mountSecurity(settings({ network: undefined }))
    await w.findAll('button').find((b) => /^restrict network$/i.test(b.text()))!.trigger('click')
    await save().trigger('click')
    await flushPromises()
    expect(AdminApiService.updateHostSettings).toHaveBeenCalledWith({ sandboxing: expect.objectContaining({ network: false }) })
  })

  it('shows a failed save next to the Save button, keeping the edit', async () => {
    vi.mocked(AdminApiService.updateHostSettings).mockRejectedValue(new Error('read-only fs'))
    const { w, filesystem, save } = await mountSecurity()
    filesystem.vm.$emit('update:modelValue', false)
    await flushPromises()
    await save().trigger('click')
    await flushPromises()
    expect(w.get('[role="alert"]').text()).toContain('read-only fs')
    expect(save().attributes('disabled')).toBeUndefined()
  })

  it('reports unsaved edits to the page and discards them', async () => {
    const { w, onDirty, filesystem, save } = await mountSecurity()
    expect(onDirty).toHaveBeenLastCalledWith(false)
    filesystem.vm.$emit('update:modelValue', false)
    await flushPromises()
    expect(onDirty).toHaveBeenLastCalledWith(true)
    await w.findAll('button').find((b) => /discard/i.test(b.text()))!.trigger('click')
    await flushPromises()
    expect(onDirty).toHaveBeenLastCalledWith(false)
    expect(save().attributes('disabled')).toBeDefined()
  })

  it('monitors terminal sessions only while the section is showing', async () => {
    expect((await mountSecurity(settings(), true)).w.findComponent({ name: 'TerminalMonitor' }).exists()).toBe(true)
    expect((await mountSecurity(settings(), false)).w.findComponent({ name: 'TerminalMonitor' }).exists()).toBe(false)
  })

  it.each([
    [/clear runtime data/i, 'clearRuntimeData'],
    [/factory reset/i, 'factoryReset'],
    [/wipeout/i, 'wipeout'],
  ] as const)('runs %s only after confirmation', async (label, call) => {
    vi.mocked(AdminApiService[call]).mockResolvedValue({ key_externally_managed: false } as never)
    const { w } = await mountSecurity()
    const button = () => w.findAll('button').find((b) => label.test(b.text()))!
    confirm.mockResolvedValueOnce(false)
    await button().trigger('click')
    await flushPromises()
    expect(AdminApiService[call]).not.toHaveBeenCalled()
    confirm.mockResolvedValueOnce(true)
    await button().trigger('click')
    await flushPromises()
    expect(AdminApiService[call]).toHaveBeenCalledTimes(1)
  })
})
