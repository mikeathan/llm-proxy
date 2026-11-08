import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { CaptureSummary, HelperStatus } from '../../../types/netaudit'

const net = vi.hoisted(() => ({ helper: vi.fn(), captureWindow: vi.fn(), lastCapture: vi.fn() }))
vi.mock('../../../services/netaudit/netauditService', () => ({ NetauditService: net }))

import CaptureWindowCard from '../../../components/netaudit/CaptureWindowCard.vue'

const status = (over: Partial<HelperStatus> = {}): HelperStatus => ({
  supported: true, installed: true, ready: true, declared_ranges: [], install_command: '', help_path: '',
  actions: [{ id: 'capture_window', on: true, installed: true }], ...over,
})

const summary = (over: Partial<CaptureSummary> = {}): CaptureSummary => ({
  at: new Date().toISOString(), interface: 'wlp2s0', wireless: true, stopped_by: 'window', elapsed_ms: 60000, frames: 854, max_per_second: 139,
  kernel_seen: 854, kernel_dropped: 0,
  counts: { arp_requests: 10, arp_replies: 0, arp_gratuitous: 10, dhcp_from_server: 0, dhcp_from_client: 0, mdns: 700, nd_router_solicit: 0, nd_router_advert: 1, nd_neighbor_solicit: 2, nd_neighbor_advert: 3, nd_redirect: 0, unreadable: 4 },
  claims: [], dhcp_server_macs: [], conflicting_addresses: 0, ...over,
})

async function mountCard() {
  const w = mount(CaptureWindowCard)
  await flushPromises()
  return w
}

describe('CaptureWindowCard', () => {
  beforeEach(() => {
    net.helper.mockReset().mockResolvedValue(status())
    net.captureWindow.mockReset()
    net.lastCapture.mockReset().mockResolvedValue(null)
  })

  it('offers the button and says plainly that it is a snapshot', async () => {
    const w = await mountCard()
    expect(w.get('[data-test="capture-listen"]').text()).toContain('Listen for 60 seconds')
    expect(w.get('[data-test="capture-note"]').text()).toContain('snapshot, not monitoring')
    expect(w.find('[data-test="capture-never"]').exists()).toBe(true)
  })

  it('shows nothing when the helper is not ready or the action is off, and there is no kept result', async () => {
    net.helper.mockResolvedValue(status({ ready: false }))
    expect((await mountCard()).find('[data-test="capture"]').exists()).toBe(false)
    net.helper.mockResolvedValue(status({ actions: [{ id: 'capture_window', on: false, installed: true }] }))
    expect((await mountCard()).find('[data-test="capture"]').exists()).toBe(false)
    net.helper.mockResolvedValue(status({ actions: [{ id: 'capture_window', on: true, installed: false }] }))
    expect((await mountCard()).find('[data-test="capture"]').exists()).toBe(false)
  })

  it('still shows a kept result when the action is now off, without a button', async () => {
    net.helper.mockResolvedValue(status({ actions: [{ id: 'capture_window', on: false, installed: true }] }))
    net.lastCapture.mockResolvedValue(summary())
    const w = await mountCard()
    expect(w.find('[data-test="capture-listen"]').exists()).toBe(false)
    expect(w.get('[data-test="capture-frames"]').text()).toContain('854 frames')
  })

  it('listens once and shows the counts, the link and the stop reason', async () => {
    net.captureWindow.mockResolvedValue({ parsed: summary({ stopped_by: 'frame_limit' }) })
    net.lastCapture.mockResolvedValueOnce(null).mockResolvedValueOnce(summary({ stopped_by: 'frame_limit' }))
    const w = await mountCard()
    await w.get('[data-test="capture-listen"]').trigger('click')
    await flushPromises()
    expect(net.captureWindow).toHaveBeenCalledTimes(1)
    const text = w.get('[data-test="capture-result"]').text()
    expect(text).toContain('wlp2s0 (Wi-Fi)')
    expect(text).toContain('at most 139 in one second')
    expect(text).toContain('Stopped at the frame limit')
    expect(text).toContain('IPv6 neighbour messages 6')
  })

  it('says why when the window could not start', async () => {
    net.captureWindow.mockResolvedValue({ parsed: summary({ stopped_by: 'interface_down', frames: 0, max_per_second: 0 }) })
    net.lastCapture.mockResolvedValue(summary({ stopped_by: 'interface_down', frames: 0, max_per_second: 0 }))
    const w = await mountCard()
    await w.get('[data-test="capture-listen"]').trigger('click')
    await flushPromises()
    expect(w.get('[data-test="capture-result"]').text()).toContain('interface is down')
  })

  it('shows an error when the server gives no readable result or the call fails', async () => {
    net.captureWindow.mockResolvedValueOnce({ parse_error: 'the output was cut' })
    const w = await mountCard()
    await w.get('[data-test="capture-listen"]').trigger('click')
    await flushPromises()
    expect(w.get('[data-test="capture-error"]').text()).toContain('the output was cut')
    net.captureWindow.mockRejectedValueOnce(new Error('this action is off'))
    await w.get('[data-test="capture-listen"]').trigger('click')
    await flushPromises()
    expect(w.get('[data-test="capture-error"]').text()).toContain('this action is off')
  })

  it('calls out a second DHCP source and an address announced twice, without calling them an attack', async () => {
    net.lastCapture.mockResolvedValue(summary({ dhcp_server_macs: ['02:00:00:00:00:01', '02:00:00:00:00:02'], conflicting_addresses: 1 }))
    const w = await mountCard()
    expect(w.get('[data-test="capture-dhcp"]').text()).toContain('2 different sources')
    expect(w.get('[data-test="capture-dhcp"]').text()).toContain('not proof')
    expect(w.get('[data-test="capture-conflict"]').text()).toContain('on purpose')
  })
})
