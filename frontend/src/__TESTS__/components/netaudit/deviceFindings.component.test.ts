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
    expect(w.find('[data-test="rescan-host"]').exists()).toBe(true)
    expect(w.find('[data-test="more-actions"]').exists()).toBe(true)
  })
})
