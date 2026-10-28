import { expect, test, type Page } from '@playwright/test'
import { expectNoSidewaysScroll, lookAt, mockAdminApi, setNetauditView } from './support/adminApi'

// The audit's report with problems among plain open ports, in Simple and Advanced at both widths: nothing may scroll
// sideways, and the screenshots are for looking at how clearly a problem stands out.
const finding = (over: Record<string, unknown>) => ({
  key: 'k', device: '192.0.2.10', port: 22, proto: 'tcp', service: 'ssh', check: 'open_port', severity: 'info', new: false, changed: false, accepted: false, ...over,
})
const report = {
  scanned: true, scanned_at: '2026-10-26T13:38:00Z', engine: 'nmap', devices_up: 8, unscanned: ['192.0.2.107'], resolved: [],
  findings: [
    ...[1, 10, 43, 60, 63, 241].flatMap((n, i) => [22, 80, 443, 8080].slice(0, (i % 4) + 1).map((port) => finding({ key: `p${n}-${port}`, device: `192.0.2.${n}`, port, service: port === 22 ? 'ssh' : 'http' }))),
    finding({ key: 'cert', device: '192.0.2.43', port: 8009, service: 'ajp13 (guess)', check: 'tls_cert_expiring', check_title: 'Certificate about to expire', severity: 'low', evidence: 'The certificate expires on 2026-11-20', new: true }),
    finding({ key: 'smb', device: '192.0.2.10', port: 445, service: 'microsoft-ds', check: 'smb_v1', check_title: 'SMBv1 enabled', severity: 'high', evidence: 'SMBv1, an old and unsafe file-sharing version, is enabled', new: true }),
    finding({
      key: 'vulns', device: '192.0.2.43', port: 22, service: 'ssh', version: 'OpenSSH 9.6p1 Ubuntu 3ubuntu13.14', check: 'known_vulns', check_title: 'Software with known flaws', severity: 'low', new: true,
      evidence: 'The software on this port may be affected by 12 known flaws: CVE-2024-6387, CVE-2026-60002, CVE-2026-35385, CVE-2023-51767, CVE-2025-26465 and 7 more. This is a distribution build, so the flaws may already be fixed.',
      cves: Array.from({ length: 12 }, (_, i) => `CVE-2024-${String(6387 + i).padStart(4, '0')}`), exploited: ['CVE-2024-6390'],
      cve_scores: Object.fromEntries(Array.from({ length: 12 }, (_, i) => [`CVE-2024-${String(6387 + i).padStart(4, '0')}`, 5 + (i % 5)])),
    }),
    finding({
      key: 'smb-fix', device: '192.0.2.10', port: 139, check: 'smb_v1', check_title: 'SMBv1 enabled', severity: 'high', evidence: 'SMBv1, an old and unsafe file-sharing version, is enabled', new: true,
      fix: 'Turn off SMBv1.', fix_state: 'still_present', fix_marked_at: '2026-10-26T09:00:00Z',
      guide: {
        steps: ['Find which devices still need SMBv1. Very old printers and Windows XP machines are the usual ones.', 'Require SMB2 or newer in the [global] section, then restart the service.'],
        commands: [{ platform: 'Linux', command: 'sudo cp /etc/samba/smb.conf /etc/samba/smb.conf.bak\n# in the [global] section add:\n#   server min protocol = SMB2\ntestparm\nsudo systemctl restart smbd' }],
        risk: 'Devices that only speak SMBv1 lose access to the shares until they are updated or replaced.',
        undo: 'Put the saved smb.conf.bak back and restart smbd.',
      },
    }),
    finding({ key: 'redis', device: '192.0.2.60', port: 6379, service: 'redis', check: 'redis_no_auth', check_title: 'Redis with no password', severity: 'critical', evidence: 'Redis answers without a password' }),
  ],
}
const automation = {
  id: 'ws/network-audit', workspace: 'ws', name: 'network-audit', task_file: 'network-audit.md', strategy: 'persistent', trigger: 'cron',
  trigger_value: '0 2 * * *', profile: 'security-auditor', network_grant: 'lan', scan: { depth: 'thorough', checks: true }, history: [{ id: 'r1', timestamp: '2026-10-26T13:30:00Z', error: '', output: '', duration_ms: 1000, model: '' }],
}

async function open(page: Page, view: 'simple' | 'advanced') {
  await setNetauditView(page, view)
  await mockAdminApi(page, { answers: { '/netaudit/findings': report, '/netaudit/expectations': [], '/automations': [automation], '/workspaces': [{ id: 'ws' }] } })
  await page.goto('automations/ws%2Fnetwork-audit')
  await expect(page.getByRole('heading', { name: 'Latest network scan' })).toBeVisible()
}

for (const view of ['simple', 'advanced'] as const) {
  test(`audit report with problems (${view}) fits the screen`, async ({ page }, testInfo) => {
    await open(page, view)
    if (view === 'advanced') await page.getByRole('button', { name: 'All 12 known flaws' }).click()
    await page.getByRole('button', { name: 'How to fix it' }).first().click()
    await expect(page.getByText('Still showing after you marked it fixed').first()).toBeVisible()
    if (testInfo.project.name === 'mobile') {
      const again = await page.getByRole('button', { name: 'Scan this device again' }).first().boundingBox()
      expect(again?.height).toBeGreaterThanOrEqual(44)
    }
    await expectNoSidewaysScroll(page)
    await page.getByRole('heading', { name: 'Latest network scan' }).scrollIntoViewIfNeeded()
    await lookAt(page, testInfo, `report-${view}.png`, true)
  })
}
