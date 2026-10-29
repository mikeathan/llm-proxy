import { expect, test } from '@playwright/test'
import { expectNoSidewaysScroll, lookAt, mockAdminApi, setNetauditView } from './support/adminApi'

// The shape of a real audit (vertex, 2026-10-29, addresses replaced): eight devices, 31 open ports, three "software with
// known flaws" problems, two of them on one device accepted, and a stored deep check with a problem. The report must open
// on the problems, count them the same everywhere, and show accepted rows quietly.
const port = (device: string, p: number, service: string, version = '') => ({
  key: `${device}:${p}`, device, port: p, proto: 'tcp', service, version, check: 'open_port', severity: 'info', new: false, changed: false, accepted: false,
})
const flaws = (device: string, p: number, n: number, severity: string, accepted = false) => ({
  key: `kv:${device}:${p}`, device, port: p, proto: 'tcp', service: 'ssh', version: 'OpenSSH 9.6p1 Ubuntu 3ubuntu13.20', check: 'known_vulns', check_title: 'Software with known flaws', severity,
  evidence: `The software on this port may be affected by ${n} known flaws: CVE-2024-6387, CVE-2026-60002, CVE-2026-35385, CVE-2023-51767, CVE-2025-26465 and ${n - 5} more. This is a distribution build, so the flaws may already be fixed.`,
  fix: "Update the software to the vendor's current release and check the vendor's notice for the listed numbers.", cves: Array.from({ length: n }, (_, i) => `CVE-2024-${String(6387 + i).padStart(4, '0')}`), exploited: ['CVE-2024-6387'], new: false, changed: false, accepted,
  ...(accepted ? { accepted_until: '2027-01-26T16:26:00Z' } : {}),
})
const devices = ['1', '7', '10', '43', '60', '125', '188', '241'].map((n) => `192.0.2.${n}`)
const ports = devices.flatMap((d, i) => Array.from({ length: i % 3 === 0 ? 4 : 3 }, (_, k) => port(d, [22, 80, 443, 8080][k]!, ['ssh', 'http', 'https', 'http-proxy'][k]!, k === 0 ? 'OpenSSH 9.6p1' : '')))
const report = {
  scanned: true, scanned_at: '2026-10-29T16:37:16Z', engine: 'nmap', devices_up: 8, unscanned: [], resolved: [],
  findings: [...ports, flaws('192.0.2.60', 22, 29, 'low'), flaws('192.0.2.241', 22, 29, 'low'), flaws('192.0.2.10', 22, 28, 'low'), flaws('192.0.2.10', 139, 26, 'high', true), flaws('192.0.2.10', 445, 26, 'high', true)],
}
const deep = [{
  device: '192.0.2.60', at: '2026-10-29T18:18:02Z', ports: [{ port: 22, tls_answered: false, ssh_answered: true }],
  findings: [{ device: '192.0.2.60', port: 22, proto: 'tcp', service: 'ssh', check: 'ssh_password_login', check_title: 'SSH accepts password login', severity: 'medium', evidence: 'The SSH server accepts password login', fix: 'Set up key login, then turn password login off (PasswordAuthentication no).',
    guide: { steps: ['Put your public key on the account you use.', 'Set PasswordAuthentication to no.'], risk: 'An account with no working key can no longer sign in.', undo: 'Put the saved sshd_config back.' } }],
}]
const automation = {
  id: 'ws/network-audit', workspace: 'ws', name: 'network-audit', task_file: 'network-audit.md', strategy: 'persistent', trigger: 'cron', trigger_value: '0 2 * * *',
  profile: 'security-auditor', network_grant: 'lan', scan: { depth: 'thorough', checks: true }, history: [{ id: 'r1', timestamp: '2026-10-29T16:36:05Z', error: '', output: '', duration_ms: 71000, model: '' }],
}

test('a real-shaped report opens on its problems, counts agree and accepted rows are quiet', async ({ page }, testInfo) => {
  await setNetauditView(page, 'advanced')
  await mockAdminApi(page, { answers: { '/netaudit/findings': report, '/netaudit/expectations': [], '/netaudit/deep': deep, '/automations': [automation], '/workspaces': [{ id: 'ws' }] } })
  await page.goto('automations/ws%2Fnetwork-audit')
  await expect(page.getByRole('radio', { name: /^Problems \(4\)/ })).toBeChecked()
  await expect(page.getByText('4 problems found by checks (2 accepted)')).toBeVisible()
  await expect(page.getByText('SSH accepts password login').first()).toBeVisible()
  await expect(page.getByText('From the deep check of')).toBeVisible()
  await expectNoSidewaysScroll(page)
  await lookAt(page, testInfo, 'real-problems.png', true)
  await page.getByRole('radio', { name: /^Devices/ }).click()
  await expectNoSidewaysScroll(page)
  await lookAt(page, testInfo, 'real-devices.png', true)
})
