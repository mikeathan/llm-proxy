import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { HelperStatus } from '../../../types/netaudit'
import type { HostSettings } from '../../../types/admin'

const net = vi.hoisted(() => ({ helper: vi.fn() }))
const admin = vi.hoisted(() => ({ fetchHostSettings: vi.fn(), updateHostSettings: vi.fn() }))
const clip = vi.hoisted(() => ({ copyText: vi.fn() }))
vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: net }))
vi.mock('../../../services/admin/adminService', () => ({ AdminApiService: admin }))
vi.mock('../../../utils/clipboard', () => clip)

import HelperCard from '../../../components/netaudit/HelperCard.vue'

const COMMAND = 'sudo ./setup.sh --install --build --unattended --helper'
const status = (over: Partial<HelperStatus> = {}): HelperStatus => ({
  supported: true, installed: true, ready: true, declared_ranges: ['192.168.50.0/24'],
  install_command: COMMAND, help_path: 'docs/service_setup.md',
  actions: [{ id: 'scan_udp', on: false, installed: true }, { id: 'os_detect', on: false, installed: true }],
  ...over,
})
const host = (actions?: string[]): HostSettings => ({
  sandboxing: { enabled: true, functional: true, max_memory_mb: 256, max_storage_gb: 2, egress_proxy: 0, ...(actions ? { helper_actions: actions } : {}) },
  effective: { filesystem: { mechanism: 'landlock' }, network: { mechanism: 'none' }, unix_sockets: { mechanism: 'seccomp' }, provider: 'landlock' },
} as HostSettings)

async function mountCard() {
  const w = mount(HelperCard)
  await flushPromises()
  return w
}

describe('HelperCard', () => {
  beforeEach(() => {
    net.helper.mockReset().mockResolvedValue(status())
    admin.fetchHostSettings.mockReset().mockResolvedValue(host())
    admin.updateHostSettings.mockReset().mockResolvedValue(host())
    clip.copyText.mockReset().mockResolvedValue(true)
  })

  it('says it is not installed and gives the one command, with help and no switches', async () => {
    net.helper.mockResolvedValue(status({ installed: false, ready: false, actions: [{ id: 'scan_udp', on: false, installed: null }] }))
    const w = await mountCard()
    expect(w.get('[data-test="helper-not-installed"]').text()).toContain('not installed')
    expect(w.get('[data-test="helper-command"]').text()).toContain(COMMAND)
    expect(w.get('[data-test="helper-help"]').text()).toContain('docs/service_setup.md')
    expect(w.find('[role="switch"]').exists()).toBe(false)
  })

  it('copies the command', async () => {
    net.helper.mockResolvedValue(status({ installed: false, ready: false }))
    const w = await mountCard()
    await w.get('[data-test="helper-copy"]').trigger('click')
    await flushPromises()
    expect(clip.copyText).toHaveBeenCalledWith(COMMAND)
    expect(w.get('[data-test="helper-command"]').text()).toContain('Copied.')
    clip.copyText.mockResolvedValue(false)
    await w.get('[data-test="helper-copy"]').trigger('click')
    await flushPromises()
    expect(w.get('[data-test="helper-command"]').text()).toContain('Could not copy')
  })

  it('says a host without systemd cannot have it, and offers no command', async () => {
    net.helper.mockResolvedValue(status({ supported: false, installed: false, ready: false }))
    const w = await mountCard()
    expect(w.get('[data-test="helper-unsupported"]').text()).toContain('Linux')
    expect(w.find('[data-test="helper-command"]').exists()).toBe(false)
  })

  it('shows a switch per action and the ranges when the helper is ready', async () => {
    const w = await mountCard()
    expect(w.findAll('[role="switch"]')).toHaveLength(2)
    expect(w.get('[data-test="helper-action-scan_udp"]').text()).toContain('UDP scan')
    expect(w.get('[data-test="helper-action-os_detect"]').text()).toContain('OS detection')
    expect(w.get('[data-test="helper-held"]').text()).toBe('192.168.50.0/24')
    expect(w.get('[data-test="helper-ranges-note"]').text()).toContain('only these ranges')
    expect(w.find('[data-test="helper-command"]').exists()).toBe(false)
    expect(w.text()).toContain('needs your passphrase')
  })

  it('turns an action on by saving only the list of actions, without the read-only projection, then reads the state again', async () => {
    const w = await mountCard()
    net.helper.mockResolvedValue(status({ actions: [{ id: 'scan_udp', on: true, installed: true }, { id: 'os_detect', on: false, installed: true }] }))
    await w.get('[data-test="helper-action-scan_udp"] input').setValue(true)
    await flushPromises()
    const saved = admin.updateHostSettings.mock.calls[0]![0] as HostSettings
    expect(saved.sandboxing.helper_actions).toEqual(['scan_udp'])
    expect(saved.sandboxing.max_memory_mb).toBe(256)
    expect(saved.effective).toBeUndefined()
    expect(net.helper).toHaveBeenCalledTimes(2)
  })

  it('turns an action off and keeps the others', async () => {
    admin.fetchHostSettings.mockResolvedValue(host(['scan_udp', 'os_detect']))
    net.helper.mockResolvedValue(status({ actions: [{ id: 'scan_udp', on: true, installed: true }, { id: 'os_detect', on: true, installed: true }] }))
    const w = await mountCard()
    await w.get('[data-test="helper-action-scan_udp"] input').setValue(false)
    await flushPromises()
    expect((admin.updateHostSettings.mock.calls[0]![0] as HostSettings).sandboxing.helper_actions).toEqual(['os_detect'])
  })

  it('shows a failed save and reads the real state again', async () => {
    admin.updateHostSettings.mockRejectedValue(new Error('the passphrase was not confirmed'))
    const w = await mountCard()
    await w.get('[data-test="helper-action-scan_udp"] input').setValue(true)
    await flushPromises()
    expect(w.get('[data-test="helper-error"]').text()).toContain('passphrase')
    expect(net.helper).toHaveBeenCalledTimes(2)
  })

  it('will not switch an action the install does not list, or anything while the sandbox is not enforcing', async () => {
    net.helper.mockResolvedValue(status({ actions: [{ id: 'scan_udp', on: false, installed: true }, { id: 'os_detect', on: false, installed: false }] }))
    let w = await mountCard()
    expect(w.get('[data-test="helper-action-os_detect"] input').attributes('disabled')).toBeDefined()
    expect(w.get('[data-test="helper-action-os_detect"]').text()).toContain('run the install command again')
    expect(w.get('[data-test="helper-action-scan_udp"] input').attributes('disabled')).toBeUndefined()

    net.helper.mockResolvedValue(status({ ready: false, not_ready: 'The sandbox is not enforcing file confinement and Unix-socket denial, so the service will not call the helper.' }))
    w = await mountCard()
    expect(w.get('[data-test="helper-not-ready"]').text()).toContain('sandbox')
    for (const input of w.findAll('[role="switch"]')) expect(input.attributes('disabled')).toBeDefined()
  })

  it('shows the ranges declared in the Network settings, which are the only ones it scans, and says when there are none', async () => {
    net.helper.mockResolvedValue(status({ declared_ranges: ['192.168.50.0/24', '10.0.0.0/8'] }))
    let w = await mountCard()
    expect(w.get('[data-test="helper-held"]').text()).toBe('192.168.50.0/24, 10.0.0.0/8')
    expect(w.get('[data-test="helper-ranges-note"]').text()).toContain('a change there applies at once')
    expect(w.find('[data-test="helper-command"]').exists()).toBe(false)

    net.helper.mockResolvedValue(status({ declared_ranges: [] }))
    w = await mountCard()
    expect(w.get('[data-test="helper-held"]').text()).toBe('—')
    expect(w.get('[data-test="helper-ranges-note"]').text()).toContain('nothing can be scanned')
  })

  it('offers a retry when the status cannot be read', async () => {
    net.helper.mockRejectedValueOnce(new Error('offline'))
    const w = await mountCard()
    expect(w.text()).toContain('could not be read')
    const retry = w.findAll('button').find((b) => /retry/i.test(b.text()))!
    await retry.trigger('click')
    await flushPromises()
    expect(w.find('[data-test="helper-actions"]').exists()).toBe(true)
  })
})
