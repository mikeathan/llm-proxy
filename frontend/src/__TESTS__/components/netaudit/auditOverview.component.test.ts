import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import AuditOverview from '../../../components/netaudit/AuditOverview.vue'
import type { AuditRecord } from '../../../types/auditRecord'

const base: AuditRecord = {
  version: 1, audit: 'network-audit', scanned_at: '2026-11-12T02:00:00Z', engine: 'nmap', scanned: 20, up: 15,
  counts: { new: 1, changed: 0, resolved: 2, accepted: 5, open_ports: 37, open: 15, by_severity: { high: 1, medium: 4, low: 10 } },
  vulns: { enabled: true, total: 37, checked: 30, coarse: 2, uncomparable: 0 }, exposed: [], notes: [], assessment: '',
}
const full: AuditRecord = {
  ...base, assessment: '1. Look at the **logrotate** file.', notes: ['One thing could not be read.'],
  exposed: [{ device: '10.0.0.5', port: 445, proto: 'tcp', service: 'microsoft-ds', detail: 'Samba 4.19', kind: 'file sharing' }],
  window: {
    window: 'Last 24 hours', scans: { count: 2, devices_up: { min: 14, max: 15 }, open_ports: { min: 36, max: 37 } },
    new_in_window: [{ device: '10.0.0.9', port: 8080, check: 'New open port', at: '2026-11-12T02:00:00Z' }], new_more: 3, gone_in_window: [], returned_in_window: [],
    activity: { new_devices: 1, ports_opened: 2 }, modules: [{ title: 'Router DNS check', noun: 'comparisons', flagged_word: 'differed', done: 885, flagged: 0, unclear: 0 }], unread: [],
  },
}

describe('AuditOverview', () => {
  it('shows the figures and the severity split even when the record has nothing else', () => {
    const w = mount(AuditOverview, { props: { record: base } })
    expect(w.get('[data-test="overview-stats"]').text()).toContain('15')
    expect(w.get('[data-test="severity-split"]').text()).toContain('1 high')
    expect(w.find('[data-test="overview-assessment"]').exists()).toBe(false)
    for (const part of ['window', 'exposed', 'modules', 'notes']) expect(w.find(`[data-test="overview-${part}"]`).exists()).toBe(false)
    expect(w.get('[data-test="overview-vulns"]').text()).toContain('Compared 30 of 37 services')
  })

  it('shows the assessment as markdown and one folded line for each part', () => {
    const w = mount(AuditOverview, { props: { record: full } })
    expect(w.get('[data-test="overview-assessment"] strong').text()).toBe('logrotate')
    expect(w.get('[data-test="overview-window"]').text()).toContain('2 scans · devices up 14–15 · open ports 36–37')
    expect(w.get('[data-test="overview-exposed"]').text()).toContain('1 service')
    expect(w.get('[data-test="overview-modules"]').text()).toContain('Router DNS check')
    expect(w.get('[data-test="overview-notes"]').text()).toContain('1 note')
    expect(w.get('[data-test="overview-exposed"] button').attributes('aria-expanded')).toBe('false')
  })

  it('opens a part to its detail', async () => {
    const w = mount(AuditOverview, { props: { record: full, names: { '10.0.0.5': 'NAS' } } })
    await w.get('[data-test="overview-exposed"] button').trigger('click')
    expect(w.get('[data-test="exposed-service"]').text()).toContain('NAS')
    expect(w.get('[data-test="exposed-service"]').text()).toContain('445/tcp')
    await w.get('[data-test="overview-window"] button').trigger('click')
    expect(w.get('[data-test="window-changes"]').text()).toContain('New open port')
    expect(w.get('[data-test="window-changes"]').text()).toContain('and 3 more')
    expect(w.get('[data-test="window-activity"]').text()).toContain('2 ports opened')
  })

  it('says when part of the window could not be read, so a gap is not a quiet network', async () => {
    const rec = { ...full, window: { ...full.window!, unread: ['alert log'] } }
    const w = mount(AuditOverview, { props: { record: rec } })
    await w.get('[data-test="overview-window"] button').trigger('click')
    expect(w.get('[data-test="window-unread"]').text()).toContain('alert log')
  })
})
