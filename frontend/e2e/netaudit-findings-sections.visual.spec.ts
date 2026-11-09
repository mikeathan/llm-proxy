import { expect, test } from '@playwright/test'
import { expectNoSidewaysScroll, expectTapArea, lookAt, mockAdminApi } from './support/adminApi'

// Network → Findings (Simple): what needs the reader sits on top in full, and the quiet findings below it fold to one line each, so nine
// identical low findings never bury the one that matters. "This is fine for all" accepts a whole kind at once, and "By device" lays the same
// findings out one card per device. At both widths nothing scrolls sideways, and on a phone the controls are 44 px targets.
const dns = (n: number) => ({
  key: `192.168.50.${n}||0|phone_home_note`, device: `192.168.50.${n}`, port: 0, proto: 'tcp', check: 'ping_note', check_title: 'A quiet note', severity: 'low',
  evidence: 'Seen quiet for a day.', fix: 'Accept it if you know the device.', new: false, changed: false, accepted: false,
})
const report = {
  scanned: true, scanned_at: '2026-11-09T15:57:00Z', engine: 'nmap', devices_up: 16, unscanned: [], resolved: [], first_scan: false,
  findings: [
    { key: 'h', device: 'this-computer', port: 0, proto: 'tcp', check: 'host_integrity', check_title: 'A program file on this computer no longer matches its package', severity: 'high', evidence: 'One file differs.', fix: 'Reinstall the package.', new: false, changed: false, accepted: false },
    ...[107, 10, 43, 34, 63].map(dns),
  ],
}
const audit = {
  id: 'ws/network-audit', workspace: 'ws', name: 'network-audit', task_file: 'network-audit.md', strategy: 'persistent', trigger: 'cron',
  trigger_value: '0 2 * * *', profile: 'security-auditor', history: [{ id: 'r1', timestamp: '2026-11-09T15:00:00Z', error: '', output: '', duration_ms: 1000, model: '' }],
}
const devices = { devices: [{ mac: 'aa:bb:cc:00:00:01', ip: '192.168.50.107', name: 'Living room Chromecast', first_seen: '2026-10-01T00:00:00Z', last_seen: new Date().toISOString(), approved: true, randomized: false, new: false }], gateway: { changed: false }, baselined: true, updated_at: '2026-11-09T00:00:00Z' }
const answers = { '/netaudit/findings': report, '/netaudit/expectations': [], '/netaudit/deep': [], '/netaudit/devices': devices, '/automations': [audit], '/workspaces': [{ id: 'ws' }] }

test('the serious finding is in full and the quiet ones fold into notes with one button for all', async ({ page }, testInfo) => {
  await mockAdminApi(page, { answers })
  await page.goto('network/findings')
  const needs = page.locator('[data-test="needs-you"]')
  await expect(needs.locator('[data-test="problem-title"]')).toHaveText(['A program file on this computer no longer matches its package'])
  const notes = page.locator('[data-test="notes"]')
  await expect(notes.locator('[data-test="notes-heading"]')).toContainText('5')
  await expect(notes.locator('[data-test="accept-all"]')).toHaveCount(0) // folded until opened
  await notes.locator('[data-test="card-toggle"]').click()
  await expect(notes.locator('[data-test="problem"]')).toHaveCount(5)
  await expect(notes.locator('[data-test="problem-place"]').first()).toContainText('Living room Chromecast · 192.168.50.107')
  await expect(notes.locator('[data-test="shared-evidence"]')).toHaveText('Seen quiet for a day.')
  await expectNoSidewaysScroll(page)
  if (testInfo.project.name === 'mobile') {
    // The area test asks the browser what is under a point, so the button must be in view, clear of the fixed bottom bar.
    await notes.locator('[data-test="accept-all"]').evaluate((el) => el.scrollIntoView({ block: 'center' }))
    await expectTapArea(notes.locator('[data-test="accept-all"]'))
  }
  await lookAt(page, testInfo, 'findings-sections.png', true)
})

test('"By device" shows one card per device and the choice is remembered', async ({ page }) => {
  await mockAdminApi(page, { answers })
  await page.goto('network/findings')
  await page.locator('[data-test="layout-switch"]').getByRole('radio', { name: 'By device' }).click()
  const cards = page.locator('[data-test="device-card"]')
  await expect(cards).toHaveCount(6)
  await expect(cards.first()).toContainText('This computer')
  await expectNoSidewaysScroll(page)
  await page.reload()
  await expect(page.locator('[data-test="problems-by-device"]')).toBeVisible()
})
