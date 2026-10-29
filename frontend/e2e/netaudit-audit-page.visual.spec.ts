import { expect, test, type Page } from '@playwright/test'
import { expectNoSidewaysScroll, lookAt, mockAdminApi, setNetauditView } from './support/adminApi'

// An audit automation's page: split into Results, Live run, History and Setup; while a scan runs the report's buttons are
// off and a banner says so. Nothing may scroll sideways and the tabs and the banner button are touch-sized.
const finding = (over: Record<string, unknown>) => ({
  key: 'k', device: '192.0.2.10', port: 22, proto: 'tcp', service: 'ssh', check: 'open_port', severity: 'info', new: false, changed: false, accepted: false, ...over,
})
const report = {
  scanned: true, scanned_at: '2026-10-30T02:00:00Z', engine: 'nmap', devices_up: 2, unscanned: [], resolved: [],
  findings: [finding({ key: 'a', port: 22 }), finding({ key: 'b', device: '192.0.2.11', port: 445, service: 'microsoft-ds', new: true })],
}
const run = { id: 'r1', timestamp: '2026-10-30T01:30:00Z', error: '', output: '', duration_ms: 1000, model: '' }
const audit = (running: boolean) => ({
  id: 'ws/network-audit', workspace: 'ws', name: 'network-audit', task_file: 'network-audit.md', strategy: 'persistent', trigger: 'cron',
  trigger_value: '0 2 * * *', profile: 'security-auditor', network_grant: 'lan', scan: { depth: 'thorough', checks: true }, history: [run], is_running: running,
  last_output: '## Observed (generated from the scan data, not written by the model)\n\nScanned 254.\n\n---\n\n## Assessment (written by the model)\n\nSamba is old.',
})

async function open(page: Page, running: boolean, view: 'simple' | 'advanced' = 'simple') {
  await setNetauditView(page, view)
  await mockAdminApi(page, { answers: { '/netaudit/findings': report, '/netaudit/expectations': [], '/netaudit/deep': [], '/automations': [audit(running)], '/workspaces': [{ id: 'ws' }] } })
  await page.goto('automations/ws%2Fnetwork-audit')
  await expect(page.getByRole('heading', { name: 'Latest network scan' })).toBeVisible()
}

test('the audit page opens on the results and keeps the rest on its own tabs', async ({ page }, testInfo) => {
  await open(page, false)
  await expect(page.getByRole('radio', { name: 'Results' })).toBeChecked()
  await expect(page.getByRole('heading', { name: 'Configuration' })).toHaveCount(0)
  if (testInfo.project.name === 'mobile') {
    const box = await page.getByRole('radio', { name: 'History' }).boundingBox()
    expect(box?.height).toBeGreaterThanOrEqual(44)
  }
  await page.getByRole('radio', { name: 'History' }).click()
  await expect(page.getByText('Samba is old.')).toBeVisible()
  await expect(page.getByText('Scanned 254.')).toHaveCount(0)
  await page.getByRole('radio', { name: 'Setup' }).click()
  await expect(page.getByRole('heading', { name: 'Configuration' })).toBeVisible()
  await expectNoSidewaysScroll(page)
  await lookAt(page, testInfo, 'audit-setup.png')
})

test('while a scan runs the report says so, its buttons are off, and the banner leads to the live run', async ({ page }, testInfo) => {
  await open(page, true, 'advanced')
  const banner = page.getByRole('status').filter({ hasText: 'A scan is running.' })
  await expect(banner).toBeVisible()
  await expect(page.getByRole('button', { name: 'Rescan this host' }).first()).toBeDisabled()
  const watch = page.getByRole('button', { name: 'Watch it live' })
  if (testInfo.project.name === 'mobile') expect((await watch.boundingBox())?.height).toBeGreaterThanOrEqual(44)
  // The tab bar says the page is live: a dot on Live run, and the chosen tab is tinted instead of plain.
  await expect(page.getByRole('radio', { name: /Live run/ }).locator('[data-test="live-dot"]')).toBeVisible()
  await expect(page.getByRole('radio', { name: 'Results' })).toHaveClass(/state-running/)
  await expectNoSidewaysScroll(page)
  await lookAt(page, testInfo, 'audit-running.png')
  await watch.click()
  await expect(page.getByRole('radio', { name: /Live run/ })).toBeChecked()
})

test('the report opens on its problems and each part is one pill away', async ({ page }, testInfo) => {
  const problem = finding({ key: 'v1', device: '192.0.2.10', port: 22, check: 'known_vulns', check_title: 'Software with known flaws', severity: 'low', evidence: 'The software on this port may be affected by 29 known flaws: CVE-2024-6387 and 28 more.', fix: 'Update the software to the vendor\'s current release.' })
  const withProblem = { ...report, findings: [...report.findings, problem] }
  await setNetauditView(page, 'advanced')
  await mockAdminApi(page, { answers: { '/netaudit/findings': withProblem, '/netaudit/expectations': [], '/netaudit/deep': [], '/automations': [audit(false)], '/workspaces': [{ id: 'ws' }] } })
  await page.goto('automations/ws%2Fnetwork-audit')
  await expect(page.getByRole('radio', { name: /^Problems \(1\)/ })).toBeChecked()
  await expect(page.getByText('Software with known flaws')).toBeVisible()
  await expect(page.locator('section[aria-label="192.0.2.10"]')).toHaveCount(0)
  await expectNoSidewaysScroll(page)
  await lookAt(page, testInfo, 'report-problems.png', true)
  await page.getByRole('radio', { name: /^Devices/ }).click()
  await expect(page.locator('section[aria-label="192.0.2.10"]')).toBeVisible()
  await expectNoSidewaysScroll(page)
  await lookAt(page, testInfo, 'report-devices.png', true)
})
