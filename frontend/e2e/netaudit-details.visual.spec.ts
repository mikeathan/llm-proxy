import { expect, test } from '@playwright/test'
import { expectNoSidewaysScroll, lookAt, mockAdminApi, setNetauditView } from './support/adminApi'

// A finding that names what changed (files added, edited, removed) and a long list of changed program files, beside a report where
// 5 of 8 devices that answered have nothing open. The details must read as a list, fold when long, and never push a phone sideways.
const base = { new: false, changed: false, accepted: false }
const port = (device: string, p: number) => ({ ...base, key: `${device}:${p}`, device, port: p, proto: 'tcp', service: 'ssh', check: 'open_port', severity: 'info' })
const units = Array.from({ length: 9 }, (_, i) => ({ label: 'Changed file', value: `/usr/lib/x86_64-linux-gnu/some/long/library/path/libexample-${i}.so.1.2.3` }))
const report = {
  scanned: true, scanned_at: '2026-11-12T16:19:00Z', engine: 'nmap', devices_up: 8, resolved: [], unscanned: [],
  up: Array.from({ length: 8 }, (_, i) => `192.0.2.${i + 1}`),
  findings: [
    port('192.0.2.1', 22), port('192.0.2.2', 22), port('192.0.2.3', 22),
    { ...base, key: 'h1', device: 'this-computer', check: 'host_changed_systemd_units', check_title: 'Startup services changed on this computer', severity: 'medium', new: true,
      evidence: 'A place that decides what runs on this computer after a reboot changed since the audit first recorded it (edited, added to or removed)', fix: 'Look at the unit files.',
      details: [{ label: 'Added', value: '/etc/systemd/system/backup-sync.service' }, { label: 'Edited', value: '/etc/systemd/system/logrotate.timer' }, { label: 'Compared with', value: 'the list kept on 12 Nov 2026' }] },
    { ...base, key: 'h2', device: 'this-computer', check: 'host_file_modified', check_title: 'A program file on this computer no longer matches its package', severity: 'high',
      evidence: '12 program files that packages installed on this computer no longer match the checksum the package shipped. A change by hand, by an attack or by a failing disk looks the same; the audit cannot tell which',
      fix: 'Reinstall the package.', details: [...units, { label: 'Not listed', value: '3 more' }] },
  ],
}
const automation = {
  id: 'ws/network-audit', workspace: 'ws', name: 'network-audit', task_file: 'network-audit.md', strategy: 'persistent', trigger: 'cron', trigger_value: '0 2 * * *',
  profile: 'security-auditor', network_grant: 'lan', scan: { depth: 'thorough', checks: true }, history: [{ id: 'r1', timestamp: '2026-11-12T16:18:00Z', error: '', output: '', duration_ms: 71000, model: '' }],
}

test('a finding names what changed as a list, and quiet devices are counted', async ({ page }, testInfo) => {
  await setNetauditView(page, 'advanced')
  await mockAdminApi(page, { answers: { '/netaudit/findings': report, '/netaudit/expectations': [], '/netaudit/deep': [], '/automations': [automation], '/workspaces': [{ id: 'ws' }] } })
  await page.goto('automations/ws%2Fnetwork-audit')
  await expect(page.getByText('/etc/systemd/system/backup-sync.service')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Show 5 more' })).toBeVisible()
  await expectNoSidewaysScroll(page)
  await lookAt(page, testInfo, 'details-problems.png', true)
  await page.getByRole('button', { name: 'Show 5 more' }).click()
  await expect(page.getByText('3 more')).toBeVisible()
  await expectNoSidewaysScroll(page)
  await page.getByRole('radio', { name: /^Devices \(8\)/ }).click()
  await page.getByText('Answered, nothing open').click()
  await expect(page.getByText('192.0.2.8')).toBeVisible()
  await expectNoSidewaysScroll(page)
  await lookAt(page, testInfo, 'details-devices.png', true)
})

// The audit's typed record sits beside its report; above the problems the Results tab shows the assessment, the figures and one folded line per part.
const recordText = JSON.stringify({
  version: 1, audit: 'network-audit', scanned_at: '2026-11-12T16:19:00Z', engine: 'nmap', scanned: 20, up: 8,
  counts: { new: 1, changed: 0, resolved: 2, accepted: 5, open_ports: 3, open: 2, by_severity: { high: 1, medium: 1 } },
  vulns: { enabled: true, total: 3, checked: 3, coarse: 0, uncomparable: 0 },
  exposed: [{ device: '192.0.2.1', port: 22, proto: 'tcp', service: 'ssh', detail: 'OpenSSH 9.6p1', kind: 'remote administration' }],
  notes: ['The previous scan result could not be read, so no comparison was made.'],
  assessment: '1. **A program file no longer matches its package** (high): look at `logrotate.timer` first.\n2. Startup services changed (medium): check the new unit.',
  window: {
    window: 'Last 24 hours', scans: { count: 2, devices_up: { min: 7, max: 8 }, open_ports: { min: 3, max: 3 } },
    new_in_window: [{ device: '192.0.2.9', port: 8080, check: 'New open port', at: '2026-11-12T02:00:00Z' }], gone_in_window: [], returned_in_window: [],
    activity: { new_devices: 1, ports_opened: 1 }, modules: [{ title: 'Router DNS check', noun: 'comparisons', flagged_word: 'differed', done: 885, flagged: 0, unclear: 0 }], unread: [],
  },
})

test('the overview sits above the problems and reads cleanly at phone width', async ({ page }, testInfo) => {
  await setNetauditView(page, 'advanced')
  const withReport = { ...automation, last_report_path: 'reports/network-audit/20261112T161800Z-r1/report.md' }
  await mockAdminApi(page, { answers: { '/netaudit/findings': report, '/netaudit/expectations': [], '/netaudit/deep': [], '/automations': [withReport], '/workspaces': [{ id: 'ws' }], 'audit.json': { content: recordText } } })
  await page.goto('automations/ws%2Fnetwork-audit')
  await expect(page.locator('[data-test="audit-overview"]')).toBeVisible()
  await expect(page.locator('[data-test="overview-stats"]')).toContainText('Open problems')
  await expectNoSidewaysScroll(page)
  await lookAt(page, testInfo, 'overview-closed.png', true)
  for (const part of ['overview-window', 'overview-exposed', 'overview-modules', 'overview-notes']) await page.locator('[data-test="' + part + '"]').getByRole('button').first().click()
  await expect(page.locator('[data-test="exposed-service"]')).toBeVisible()
  await expectNoSidewaysScroll(page)
  await lookAt(page, testInfo, 'overview-open.png', true)
})
