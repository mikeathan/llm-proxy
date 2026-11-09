import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { HelperStatus, OsGuess, UdpPort } from '../../../types/netaudit'

const net = vi.hoisted(() => ({ helper: vi.fn(), scanUdp: vi.fn(), detectOs: vi.fn(), clearProbe: vi.fn() }))
vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: net }))

import DeviceProbe from '../../../components/netaudit/DeviceProbe.vue'

const status = (over: Partial<HelperStatus> = {}): HelperStatus => ({
  supported: true, installed: true, ready: true, declared_ranges: ['192.168.50.0/24'], install_command: '', help_path: '',
  actions: [{ id: 'scan_udp', on: true, installed: true }, { id: 'os_detect', on: true, installed: true }],
  ...over,
})

type Props = { ip?: string; mac?: string; udpPorts?: UdpPort[]; udpAt?: string; osGuesses?: OsGuess[]; osAt?: string; noOpenPorts?: boolean }

async function mountProbe(props: Props = { ip: '192.168.50.1' }) {
  const w = mount(DeviceProbe, { props })
  await flushPromises()
  return w
}

describe('DeviceProbe', () => {
  beforeEach(() => {
    net.helper.mockReset().mockResolvedValue(status())
    net.scanUdp.mockReset()
    net.detectOs.mockReset()
    net.clearProbe.mockReset().mockResolvedValue({})
  })

  it('offers one button per action that is on and in the helper list', async () => {
    const w = await mountProbe()
    expect(w.get('[data-test="probe-scan_udp"]').text()).toContain('UDP scan')
    expect(w.get('[data-test="probe-os_detect"]').text()).toContain('OS detection')
    expect(w.text()).toContain('passphrase')
  })

  it('shows nothing when the helper is not ready, an action is off, or the device has no address', async () => {
    net.helper.mockResolvedValue(status({ ready: false }))
    expect((await mountProbe()).find('[data-test="probe"]').exists()).toBe(false)
    net.helper.mockResolvedValue(status({ actions: [{ id: 'scan_udp', on: false, installed: true }, { id: 'os_detect', on: true, installed: false }] }))
    expect((await mountProbe()).find('[data-test="probe"]').exists()).toBe(false)
    net.helper.mockResolvedValue(status())
    expect((await mountProbe({})).find('[data-test="probe"]').exists()).toBe(false)
  })

  it('shows only the action that is on', async () => {
    net.helper.mockResolvedValue(status({ actions: [{ id: 'scan_udp', on: true, installed: true }, { id: 'os_detect', on: false, installed: true }] }))
    const w = await mountProbe()
    expect(w.find('[data-test="probe-scan_udp"]').exists()).toBe(true)
    expect(w.find('[data-test="probe-os_detect"]').exists()).toBe(false)
  })

  it('runs a UDP scan of this device on all the allowed ports and lists what answered', async () => {
    net.scanUdp.mockResolvedValue({ parsed: { udp_ports: [{ port: 53, state: 'open', service: 'domain' }, { port: 123, state: 'open|filtered', service: 'ntp' }], os: [] } })
    const w = await mountProbe()
    await w.get('[data-test="probe-scan_udp"]').trigger('click')
    await flushPromises()
    expect(net.scanUdp).toHaveBeenCalledWith('192.168.50.1', [53, 67, 69, 123, 137, 161, 500, 1900, 5353])
    expect(w.emitted('probed')).toHaveLength(1)
    const out = w.get('[data-test="probe-udp"]').text()
    expect(out).toContain('53/udp')
    expect(out).toContain('domain')
    expect(out).toContain('123/udp')
    expect(out).toContain('no reply')
  })

  it('says plainly when no UDP service answered', async () => {
    net.scanUdp.mockResolvedValue({ parsed: { udp_ports: [], os: [] } })
    const w = await mountProbe()
    await w.get('[data-test="probe-scan_udp"]').trigger('click')
    await flushPromises()
    expect(w.get('[data-test="probe-udp"]').text()).toContain('No UDP service answered')
  })

  it('lists the OS guesses with their accuracy, and says when there is none', async () => {
    net.detectOs.mockResolvedValue({ parsed: { udp_ports: [], os: [{ name: 'Linux 4.15 - 5.19', accuracy: 96 }] } })
    const w = await mountProbe()
    await w.get('[data-test="probe-os_detect"]').trigger('click')
    await flushPromises()
    expect(net.detectOs).toHaveBeenCalledWith('192.168.50.1')
    expect(w.get('[data-test="probe-os"]').text()).toContain('Linux 4.15 - 5.19 (96%)')
    net.detectOs.mockResolvedValue({ parsed: { udp_ports: [], os: [] } })
    await w.get('[data-test="probe-os_detect"]').trigger('click')
    await flushPromises()
    expect(w.get('[data-test="probe-os"]').text()).toContain('No guess')
  })

  it('shows the reason when the server could not read the output, and when the call fails', async () => {
    net.scanUdp.mockResolvedValue({ parse_error: 'the output was cut at its size limit, so it was not read' })
    const w = await mountProbe()
    await w.get('[data-test="probe-scan_udp"]').trigger('click')
    await flushPromises()
    expect(w.get('[data-test="probe-error"]').text()).toContain('cut at its size limit')
    net.scanUdp.mockRejectedValue(new Error('target is outside the declared scan ranges'))
    await w.get('[data-test="probe-scan_udp"]').trigger('click')
    await flushPromises()
    expect(w.get('[data-test="probe-error"]').text()).toContain('outside the declared scan ranges')
    expect(w.find('[data-test="probe-udp"]').exists()).toBe(false)
    expect(w.emitted('probed')).toBeUndefined()
  })

  it('disables the buttons while a scan runs', async () => {
    let finish: (v: unknown) => void = () => {}
    net.scanUdp.mockReturnValue(new Promise((r) => { finish = r }))
    const w = await mountProbe()
    await w.get('[data-test="probe-scan_udp"]').trigger('click')
    expect(w.get('[data-test="probe-os_detect"]').attributes('disabled')).toBeDefined()
    expect(w.text()).toContain('Scanning')
    finish({ parsed: { udp_ports: [], os: [] } })
    await flushPromises()
    expect(w.get('[data-test="probe-os_detect"]').attributes('disabled')).toBeUndefined()
  })

  it('shows the result kept on the device with its time, even when the helper is off', async () => {
    net.helper.mockResolvedValue(status({ ready: false }))
    const w = await mountProbe({
      ip: '192.168.50.1', udpAt: new Date().toISOString(), udpPorts: [{ port: 161, state: 'open', service: 'snmp' }],
      osAt: new Date().toISOString(), osGuesses: [{ name: 'Linux 5', accuracy: 90 }],
    })
    expect(w.find('[data-test="probe-scan_udp"]').exists()).toBe(false)
    expect(w.get('[data-test="probe-udp"]').text()).toContain('161/udp')
    expect(w.get('[data-test="probe-os"]').text()).toContain('Linux 5 (90%)')
  })

  it('tells a scan that found nothing from one never run', async () => {
    const never = await mountProbe({ ip: '192.168.50.1' })
    expect(never.find('[data-test="probe-udp"]').exists()).toBe(false)
    const empty = await mountProbe({ ip: '192.168.50.1', udpAt: new Date().toISOString(), udpPorts: [] })
    expect(empty.get('[data-test="probe-udp"]').text()).toContain('No UDP service answered')
  })

  it('clears a kept result on request, and asks for the list again', async () => {
    const w = await mountProbe({ ip: '192.168.50.1', mac: 'aa:bb:cc:00:00:01', udpAt: new Date().toISOString(), udpPorts: [{ port: 161, state: 'open' }] })
    await w.get('[data-test="probe-clear-udp"]').trigger('click')
    await flushPromises()
    expect(net.clearProbe).toHaveBeenCalledWith('aa:bb:cc:00:00:01', 'udp')
    expect(w.emitted('probed')).toHaveLength(1)
  })

  it('shows the failure and keeps the result when clearing fails', async () => {
    net.clearProbe.mockRejectedValue(new Error('nope'))
    const w = await mountProbe({ ip: '192.168.50.1', mac: 'aa:bb:cc:00:00:01', udpAt: new Date().toISOString(), udpPorts: [{ port: 161, state: 'open' }] })
    await w.get('[data-test="probe-clear-udp"]').trigger('click')
    await flushPromises()
    expect(w.get('[data-test="probe-error"]').text()).toContain('nope')
    expect(w.emitted('probed')).toBeUndefined()
  })

  it('says the guesses are unreliable when the device showed no open port, and only then', async () => {
    const guess = [{ name: 'Roberts STREAM 202 Internet radio', accuracy: 93 }]
    const quiet = await mountProbe({ ip: '192.168.50.9', osGuesses: guess, osAt: '2026-11-08T12:00:00Z', noOpenPorts: true })
    expect(quiet.get('[data-test="probe-os-unreliable"]').text()).toContain('no open port')
    // Demoted, not hidden: the name stays, muted and with no percentage, after the warning.
    const demoted = quiet.get('[data-test="probe-os-guess"]')
    expect(demoted.text()).toBe('Roberts STREAM 202 Internet radio')
    expect(demoted.classes()).toContain('text-muted')
    const parts = quiet.get('[data-test="probe-os"]').html()
    expect(parts.indexOf('probe-os-unreliable')).toBeLessThan(parts.indexOf('probe-os-guess'))
    const open = await mountProbe({ ip: '192.168.50.9', osGuesses: guess, osAt: '2026-11-08T12:00:00Z' })
    expect(open.find('[data-test="probe-os-unreliable"]').exists()).toBe(false)
    expect(open.get('[data-test="probe-os-guess"]').text()).toBe('Roberts STREAM 202 Internet radio (93%)')
    expect(open.get('[data-test="probe-os-guess"]').classes()).not.toContain('text-muted')
    const none = await mountProbe({ ip: '192.168.50.9', osGuesses: [], osAt: '2026-11-08T12:00:00Z', noOpenPorts: true })
    expect(none.find('[data-test="probe-os-unreliable"]').exists()).toBe(false)
  })
})
