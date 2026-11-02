import { expect, test, type Page } from '@playwright/test'
import { expectNoSidewaysScroll, lookAt, mockAdminApi, setNetauditView } from './support/adminApi'

// The Findings report in every kind of theme (dark and light presets): collapsible cards, rows with one aligned action column and
// a "⋯" that opens the rest. Colour comes only from tokens, so each theme must read and nothing may scroll sideways at 1440 or 390.
const SELECTION_KEY = 'admin-ui:theme-selection'
const APPLIED_KEY = 'admin-ui:theme-applied'
const THEMES = { 'retro-dark': 'dark', 'retro-midnight': 'dark', 'retro-forest': 'dark', 'retro-paper': 'light', 'retro-sand': 'light' } as const

const finding = (over: Record<string, unknown>) => ({
  key: 'k', device: '192.168.50.10', port: 22, proto: 'tcp', service: 'ssh', version: 'OpenSSH 9.9p1 Ubuntu 3ubuntu3.2', check: 'open_port', severity: 'info', new: false, changed: false, accepted: false, ...over,
})
const report = {
  scanned: true, scanned_at: '2026-10-28T18:26:00Z', engine: 'nmap', devices_up: 2, unscanned: [], resolved: [],
  findings: [
    finding({ key: 'a', port: 22 }),
    finding({ key: 'b', port: 3389, service: 'tcpwrapped', version: '', new: true }),
    finding({ key: 'c', device: '192.168.50.60', port: 22, version: 'OpenSSH 9.6p1 Ubuntu 3ubuntu13.20' }),
    finding({ key: 'v1', port: 22, check: 'known_vulns', check_title: 'Software with known flaws', severity: 'low', evidence: '26 known flaws', fix: "Update the software to the vendor's current release." }),
  ],
}
const audit = {
  id: 'ws/network-audit', workspace: 'ws', name: 'network-audit', task_file: 'network-audit.md', strategy: 'persistent', trigger: 'cron',
  trigger_value: '0 2 * * *', profile: 'security-auditor', history: [{ id: 'r1', timestamp: '2026-10-28T18:00:00Z', error: '', output: '', duration_ms: 1000, model: '' }],
}

async function open(page: Page, id: keyof typeof THEMES) {
  await page.addInitScript(([s, a, themeId, scheme]) => {
    const envelope = (data: unknown) => JSON.stringify({ version: 1, data })
    localStorage.setItem(s, envelope({ kind: 'preset', id: themeId }))
    localStorage.setItem(a, envelope({ themeId, presetId: themeId, scheme, overrides: {} }))
  }, [SELECTION_KEY, APPLIED_KEY, id, THEMES[id]] as const)
  await setNetauditView(page, 'advanced')
  await mockAdminApi(page, { answers: { '/netaudit/findings': report, '/netaudit/expectations': [], '/netaudit/deep': [], '/automations': [audit], '/workspaces': [{ id: 'ws' }] } })
  await page.goto('network/findings')
  await expect(page.locator('html')).toHaveAttribute('data-theme', id)
  await expect(page.getByRole('radio', { name: /^Problems \(1\)/ })).toBeChecked()
}

for (const id of Object.keys(THEMES) as (keyof typeof THEMES)[]) {
  test(`findings read in ${id}: collapsible cards, aligned actions, a "⋯" that opens the rest`, async ({ page }, testInfo) => {
    await open(page, id)
    await expectNoSidewaysScroll(page)
    const card = page.locator('[data-check="known_vulns"]')
    await expect(card.getByText('Software with known flaws')).toBeVisible()
    const accept = card.getByRole('button', { name: 'Accept…' })
    if (testInfo.project.name === 'mobile') expect((await accept.boundingBox())?.height).toBeGreaterThanOrEqual(44)
    // The "⋯" opens the rest of the row's actions under it, and pressing it again closes them.
    const more = card.getByRole('button', { name: 'More actions' })
    await more.click()
    await expect(card.getByRole('button', { name: 'Rescan this host' })).toBeVisible()
    await more.click()
    await expect(card.getByRole('button', { name: 'Rescan this host' })).toHaveCount(0)
    // The card folds from its header and opens again.
    await card.locator('[data-test="card-toggle"]').click()
    await expect(card.getByRole('button', { name: 'Accept…' })).toHaveCount(0)
    await card.locator('[data-test="card-toggle"]').click()
    await lookAt(page, testInfo, `findings-problems-${id}.png`, true)

    await page.getByRole('radio', { name: /^Devices/ }).click()
    const device = page.locator('article[aria-label="192.168.50.10"]')
    await expect(device.getByText('3389/tcp')).toBeVisible()
    await device.getByRole('button', { name: 'More actions' }).click()
    for (const name of ['Rescan this host', 'Set as expected', 'Deep check']) {
      const button = device.getByRole('button', { name })
      await expect(button).toBeVisible()
      if (testInfo.project.name === 'mobile') expect((await button.boundingBox())?.height).toBeGreaterThanOrEqual(44)
    }
    await expectNoSidewaysScroll(page)
    await lookAt(page, testInfo, `findings-devices-${id}.png`, true)
  })
}
