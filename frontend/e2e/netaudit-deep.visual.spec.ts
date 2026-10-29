import { expect, test, type Page } from '@playwright/test'
import { expectNoSidewaysScroll, lookAt, mockAdminApi, setNetauditView } from './support/adminApi'

// The deep check on a device (Advanced view): the question that says what will be sent, and the stored results (one with
// problems and a fix, one that found no TLS or SSH service to judge). Nothing may scroll sideways; buttons are touch-sized.
const finding = (over: Record<string, unknown>) => ({
  key: 'k', device: '192.0.2.10', port: 22, proto: 'tcp', service: 'ssh', check: 'open_port', severity: 'info', new: false, changed: false, accepted: false, ...over,
})
const report = {
  scanned: true, scanned_at: '2026-10-30T02:00:00Z', engine: 'nmap', devices_up: 3, unscanned: [], resolved: [],
  findings: [
    finding({ key: 'a', port: 22 }), finding({ key: 'b', port: 443, service: 'https' }),
    finding({ key: 'c', device: '192.0.2.11', port: 3389, service: 'ms-wbt-server' }), finding({ key: 'd', device: '192.0.2.11', port: 445, service: 'microsoft-ds' }),
    finding({ key: 'e', device: '192.0.2.12', port: 22 }),
  ],
}
const deepChecks = [
  {
    device: '192.0.2.10', at: '2026-10-30T12:00:00Z',
    ports: [{ port: 22, tls_answered: false, ssh_answered: true }, { port: 443, tls_answered: true, ssh_answered: false }],
    findings: [
      {
        device: '192.0.2.10', port: 22, proto: 'tcp', service: 'ssh', check: 'ssh_password_login', check_title: 'SSH accepts password login', severity: 'medium',
        evidence: 'The SSH server accepts password login (from the deep check of 2026-10-30)', fix: 'Set up key login, then turn password login off (PasswordAuthentication no).',
        guide: {
          steps: ['Put your public key on the account you use, and confirm a key login works in a new window.', 'Set PasswordAuthentication to no, test the configuration, then restart the SSH service.'],
          commands: [{ platform: 'Linux', command: 'sudo cp /etc/ssh/sshd_config /etc/ssh/sshd_config.bak\n# PasswordAuthentication no\nsudo sshd -t && sudo systemctl restart ssh' }],
          risk: 'An account with no working key can no longer sign in over SSH.', undo: 'Put the saved sshd_config.bak back and restart the SSH service.',
        },
      },
      { device: '192.0.2.10', port: 443, proto: 'tcp', service: 'https', check: 'tls_weak_protocol', check_title: 'Old TLS or SSL versions still accepted', severity: 'medium', evidence: 'It still accepts old protocol versions: TLS 1.0, TLS 1.1 (from the deep check of 2026-10-30)', fix: 'Allow TLS 1.2 and 1.3 only.' },
    ],
  },
  { device: '192.0.2.11', at: '2026-10-30T12:00:00Z', ports: [{ port: 3389, tls_answered: false, ssh_answered: false }, { port: 445, tls_answered: false, ssh_answered: false }], findings: [] },
]
const automation = {
  id: 'ws/network-audit', workspace: 'ws', name: 'network-audit', task_file: 'network-audit.md', strategy: 'persistent', trigger: 'cron',
  trigger_value: '0 2 * * *', profile: 'security-auditor', network_grant: 'lan', scan: { depth: 'thorough', checks: true }, history: [{ id: 'r1', timestamp: '2026-10-30T01:30:00Z', error: '', output: '', duration_ms: 1000, model: '' }],
}

async function open(page: Page) {
  await setNetauditView(page, 'advanced')
  await mockAdminApi(page, { answers: { '/netaudit/findings': report, '/netaudit/expectations': [], '/netaudit/deep': deepChecks, '/automations': [automation], '/workspaces': [{ id: 'ws' }] } })
  await page.goto('automations/ws%2Fnetwork-audit')
  await expect(page.getByRole('heading', { name: 'Latest network scan' })).toBeVisible()
}

test('the deep check question says exactly what will be sent, and fits the screen', async ({ page }, testInfo) => {
  await open(page)
  await page.locator('section[aria-label="192.0.2.12"]').getByRole('button', { name: 'Deep check' }).click()
  const dialog = page.getByRole('dialog')
  await expect(dialog.getByRole('heading', { name: 'Deep check of 192.0.2.12?' })).toBeVisible()
  await expect(dialog.getByText('about 60 connections')).toBeVisible()
  await expect(dialog.getByText('made-up user name')).toBeVisible()
  if (testInfo.project.name === 'mobile') {
    for (const name of ['Cancel', 'Run deep check']) {
      const box = await dialog.getByRole('button', { name }).boundingBox()
      expect(box?.height).toBeGreaterThanOrEqual(44)
    }
  }
  await expectNoSidewaysScroll(page)
  await lookAt(page, testInfo, 'deep-dialog.png')
})

test('stored deep results show their problems and fixes, and a device with nothing to judge is not called fine', async ({ page }, testInfo) => {
  await open(page)
  const first = page.locator('section[aria-label="192.0.2.10"]')
  await expect(first.getByText('2 problems found.')).toBeVisible()
  await expect(first.getByText('SSH accepts password login')).toBeVisible()
  const second = page.locator('section[aria-label="192.0.2.11"]')
  await expect(second.getByText('Nothing to check', { exact: true })).toBeVisible()
  await expect(second.getByText('not the same as nothing wrong')).toBeVisible()
  await expectNoSidewaysScroll(page)
  await page.locator('section[aria-label="192.0.2.10"]').scrollIntoViewIfNeeded()
  await lookAt(page, testInfo, 'deep-results.png', true)
})
