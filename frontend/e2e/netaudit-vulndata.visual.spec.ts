import { expect, test, type Page } from '@playwright/test'
import { expectNoSidewaysScroll, lookAt, mockAdminApi, setNetauditView } from './support/adminApi'

// Settings → Network scan → Vulnerability data, in each state it can be in, at both widths and in Simple and Advanced:
// nothing may scroll sideways and the button is a comfortable touch target. The screenshots are for looking at.
const NEVER = '0001-01-01T00:00:00Z'
const none = { imported_at: NEVER, cves: 0, entries: 0 }
const recent = new Date(Date.now() - 3 * 86_400_000).toISOString()
const old = new Date(Date.now() - 70 * 86_400_000).toISOString()
const held = {
  nvd: { imported_at: recent, cves: 160000, entries: 900000, years: [2022, 2023, 2024, 2025, 2026], no_config: 44242, rejected: 1093 },
  kev: { imported_at: recent, cves: 1758, entries: 0 },
}
const STATES = {
  'not-downloaded': { state: 'idle', files_done: 0, files_total: 6, nvd: none, kev: none },
  running: { state: 'running', file: 'NVD 2023', files_done: 1, files_total: 6, bytes_done: 10485760, bytes_total: 41943040, nvd: none, kev: held.kev },
  stopped: { state: 'stopped', files_done: 3, files_total: 6, reason: 'Could not download NVD 2025: the connection was reset.', ...held },
  'up-to-date': { state: 'done', files_done: 6, files_total: 6, ...held },
  old: { state: 'done', files_done: 6, files_total: 6, nvd: { ...held.nvd, imported_at: old }, kev: { ...held.kev, imported_at: old } },
} as const
const button = { 'not-downloaded': 'Download', running: 'Cancel', stopped: 'Resume', 'up-to-date': null, old: 'Update' } as const

const scope = { ranges: [{ cidr: '192.168.1.0/24', devices: 254 }], allow_unconfined: false, device_timeout_minutes: 0, suggestions: [] }

async function open(page: Page, view: 'simple' | 'advanced', vuln: object, readiness?: object) {
  await setNetauditView(page, view)
  await mockAdminApi(page, { answers: { '/netaudit/scope': scope, '/netaudit/vulndata': vuln, ...(readiness ? { '/netaudit/readiness': readiness } : {}) } })
  await page.goto('settings/network-scan')
  await expect(page.getByRole('heading', { name: 'Vulnerability data' })).toBeVisible()
}

for (const view of ['simple', 'advanced'] as const) {
  for (const name of Object.keys(STATES) as (keyof typeof STATES)[]) {
    test(`vulnerability data card, ${name} (${view}), fits the screen`, async ({ page }, testInfo) => {
      await open(page, view, STATES[name])
      const label = button[name]
      const action = page.getByRole('button', { name: label ?? 'none', exact: true })
      if (label) await expect(action).toBeVisible()
      else await expect(page.locator('[data-test="vuln-data"]').getByRole('button', { name: /^(Download|Resume|Cancel|Update)$/ })).toHaveCount(0)
      await expectNoSidewaysScroll(page)
      if (label && testInfo.project.name === 'mobile') expect((await action.boundingBox())?.height).toBeGreaterThanOrEqual(44)
      if (name === 'running') await expect(page.getByRole('meter', { name: 'Download progress' })).toBeVisible()
      if (view === 'advanced' && name !== 'not-downloaded') await expect(page.getByText('CISA exploited list')).toBeVisible()
      const startAgain = page.getByRole('button', { name: 'Start again', exact: true })
      if (view === 'advanced' && name !== 'not-downloaded' && name !== 'running') {
        await expect(startAgain).toBeVisible()
        if (testInfo.project.name === 'mobile') expect((await startAgain.boundingBox())?.height).toBeGreaterThanOrEqual(44)
      } else {
        await expect(startAgain).toHaveCount(0)
      }
      const remove = page.getByRole('button', { name: 'Delete data', exact: true })
      if (name === 'not-downloaded' || name === 'running') {
        await expect(remove).toHaveCount(0)
      } else {
        await expect(remove).toBeVisible()
        if (testInfo.project.name === 'mobile') expect((await remove.boundingBox())?.height).toBeGreaterThanOrEqual(44)
      }
      await lookAt(page.getByRole('region', { name: 'Vulnerability data' }), testInfo, `vuln-data-${name}-${view}`)
    })
  }
}

test('the audit notice shows beside the readiness list while the data is missing', async ({ page }) => {
  const notice = { code: 'vuln_data_missing', message: 'x', blocking: false, fix: { kind: 'open_settings', section: 'network-scan' } }
  await open(page, 'simple', STATES['not-downloaded'], { ready: true, requirements: [], notices: [notice] })
  await expect(page.getByText('Known-vulnerability data is not downloaded')).toBeVisible()
})

test('old data shows a notice with an Update now button that fits a phone', async ({ page }, testInfo) => {
  const old = { code: 'vuln_data_old', message: 'x', blocking: false, fix: { kind: 'open_settings', section: 'network-scan' } }
  await open(page, 'simple', STATES['up-to-date'], { ready: true, requirements: [], notices: [old] })
  await expect(page.getByText('Known-vulnerability data is out of date')).toBeVisible()
  const update = page.getByRole('button', { name: 'Update now' })
  await expect(update).toBeVisible()
  await expectNoSidewaysScroll(page)
  if (testInfo.project.name === 'mobile') expect((await update.boundingBox())?.height).toBeGreaterThanOrEqual(44)
})
