import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import DeviceFindings from '../../../components/netaudit/DeviceFindings.vue'
import type { ScanFinding } from '../../../types/netaudit'

const listener: ScanFinding = {
  key: 'this-computer|tcp|8080|new_listener', device: 'this-computer', port: 8080, proto: 'tcp', check: 'new_listener',
  severity: 'medium', evidence: 'Something on this computer is waiting for connections on this port', new: true, changed: false, accepted: false,
} as ScanFinding

const props = (device: string, items: ScanFinding[]) => ({ group: { device, items }, open: true, baseline: false, problems: items.length })

describe('DeviceFindings for the computer the program runs on', () => {
  it('says "This computer" and offers nothing that needs a network address', () => {
    const w = mount(DeviceFindings, { props: props('this-computer', [listener]) })
    expect(w.text()).toContain('This computer')
    expect(w.text()).not.toContain('this-computer')
    expect(w.text()).not.toContain('open ports')
    expect(w.find('[data-test="rescan-host"]').exists()).toBe(false)
    expect(w.find('[data-test="more-actions"]').exists()).toBe(false)
  })

  it('still offers them for a real device', () => {
    const w = mount(DeviceFindings, { props: props('192.168.50.20', [{ ...listener, device: '192.168.50.20', key: 'x' }]) })
    expect(w.find('[data-test="more-actions"]').exists()).toBe(true)
    expect(w.find('[data-test="rescan-host"]').exists()).toBe(false) // behind the "⋯" until it is pressed
  })

  it('opens every action of a real device behind one "⋯"', async () => {
    const w = mount(DeviceFindings, { props: props('192.168.50.20', [{ ...listener, device: '192.168.50.20', key: 'x' }]) })
    await w.get('[data-test="more-actions"]').trigger('click')
    for (const test of ['rescan-host', 'set-expected', 'deep-check']) expect(w.find(`[data-test="${test}"]`).exists()).toBe(true)
    await w.get('[data-test="more-actions"]').trigger('click')
    expect(w.find('[data-test="rescan-host"]').exists()).toBe(false)
  })
})

// The outside database lists a port number and no protocol: showing "1194/tcp" would state something nobody measured.
describe('DeviceFindings for a port that has no protocol', () => {
  const exposed: ScanFinding = {
    key: '192.168.50.1||1194|internet_exposed_port', device: '192.168.50.1', port: 1194, check: 'internet_exposed_port',
    severity: 'medium', evidence: 'An outside scan database lists port 1194 as open', new: true, changed: false, accepted: false,
  } as ScanFinding

  it('shows the bare port number, not a guessed protocol', () => {
    const w = mount(DeviceFindings, { props: props('192.168.50.1', [exposed]) })
    expect(w.text()).toContain('1194')
    expect(w.text()).not.toContain('1194/tcp')
  })

  it('still shows the protocol of a port a scan measured', () => {
    const w = mount(DeviceFindings, { props: props('192.168.50.1', [{ ...exposed, proto: 'udp', key: 'y' }]) })
    expect(w.text()).toContain('1194/udp')
  })
})
