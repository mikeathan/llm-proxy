import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import FindingDetails from '../../../components/netaudit/FindingDetails.vue'
import QuietDevices from '../../../components/netaudit/QuietDevices.vue'
import type { FindingDetail } from '../../../types/netaudit'

const rows = (n: number): FindingDetail[] => Array.from({ length: n }, (_, i) => ({ label: 'Edited', value: `/etc/systemd/system/u${i}.service` }))

describe('FindingDetails', () => {
  it('lists each label with its value and shows a short list whole', () => {
    const w = mount(FindingDetails, { props: { details: [{ label: 'Added', value: '/etc/a' }, { label: 'Removed', value: '/etc/b' }] } })
    expect(w.findAll('[data-test="finding-detail"]')).toHaveLength(2)
    expect(w.text()).toContain('Added')
    expect(w.text()).toContain('/etc/b')
    expect(w.find('[data-test="details-toggle"]').exists()).toBe(false)
  })

  it('folds a long list to its first rows and opens on request', async () => {
    const w = mount(FindingDetails, { props: { details: rows(9) } })
    expect(w.findAll('[data-test="finding-detail-value"]')).toHaveLength(5)
    const toggle = w.get('[data-test="details-toggle"]')
    expect(toggle.text()).toBe('Show 4 more')
    await toggle.trigger('click')
    expect(w.findAll('[data-test="finding-detail-value"]')).toHaveLength(9)
    expect(w.get('[data-test="details-toggle"]').attributes('aria-expanded')).toBe('true')
  })

  it('does not fold a list that would hide a single row', () => {
    const w = mount(FindingDetails, { props: { details: rows(6) } })
    expect(w.findAll('[data-test="finding-detail-value"]')).toHaveLength(6)
    expect(w.find('[data-test="details-toggle"]').exists()).toBe(false)
  })

  it('wraps long paths instead of widening the page', () => {
    const w = mount(FindingDetails, { props: { details: rows(1) } })
    expect(w.get('[data-test="finding-detail-value"]').classes()).toContain('break-all')
  })

  it('names a label once when several rows share it', () => {
    const w = mount(FindingDetails, { props: { details: rows(3) } })
    expect(w.findAll('dt')).toHaveLength(1)
    expect(w.findAll('[data-test="finding-detail-value"]')).toHaveLength(3)
  })
})

describe('QuietDevices', () => {
  it('says how many answered with nothing open and names each one', async () => {
    const w = mount(QuietDevices, { props: { devices: ['10.0.0.4', '10.0.0.7'], names: { '10.0.0.7': 'Printer' } } })
    expect(w.text()).toContain('2 devices')
    await w.get('button').trigger('click')
    expect(w.findAll('[data-test="quiet-device"]')).toHaveLength(2)
    expect(w.text()).toContain('Printer')
  })

  it('draws nothing when every device that answered is listed elsewhere', () => {
    expect(mount(QuietDevices, { props: { devices: [] } }).find('[data-test="quiet-devices"]').exists()).toBe(false)
  })
})

describe('the Advanced report with details and quiet devices', () => {
  const base = { new: false, changed: false, accepted: false }
  const port = { ...base, key: 'a', device: '10.0.0.1', port: 22, proto: 'tcp', service: 'ssh', check: 'open_port', severity: 'info' }
  const problem = { ...base, key: 'b', device: 'this-computer', check: 'host_changed_cron', check_title: 'Scheduled jobs changed', severity: 'medium', evidence: 'A place changed', details: [{ label: 'Added', value: '/etc/cron.d/y' }] }
  const report = { scanned: true, engine: 'nmap', findings: [port, problem], resolved: [], unscanned: [], devices_up: 3, up: ['10.0.0.1', '10.0.0.2', '10.0.0.3'] }

  it('shows what changed inside the problem and counts every device that answered', async () => {
    const [{ default: FindingsAdvanced }, { summarize }] = await Promise.all([import('../../../components/netaudit/FindingsAdvanced.vue'), import('../../../utils/netaudit/report')])
    const w = mount(FindingsAdvanced, { props: { report, summary: summarize(report) } })
    expect(w.get('[data-test="finding-details"]').text()).toContain('/etc/cron.d/y')
    expect(w.text()).toContain('Devices (3)')
    const devices = w.findAll('button').find((b) => b.text().startsWith('Devices'))!
    await devices.trigger('click')
    expect(w.get('[data-test="quiet-devices"]').text()).toContain('2 devices')
  })
})
