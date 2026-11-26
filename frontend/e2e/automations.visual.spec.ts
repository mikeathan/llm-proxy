import { expect, test, type Page } from '@playwright/test'
import { expectNoSidewaysScroll, expectTapArea, lookAt, mockAdminApi } from './support/adminApi'
import { AUTOMATIONS } from './support/automationFixtures'

// Automations (plan assistant-calm-mobile, frames E-H): a phone list says how each automation last went in one row, and
// every automation's page is the same tabs.
const NOW = new Date('2026-11-26T16:00:00Z')

async function open(page: Page, path: string) {
  await page.clock.setFixedTime(NOW)
  await mockAdminApi(page, {
    answers: {
      '/dispatcher/workspaces': [{ id: 'workspace-1' }],
      '/dispatcher/automations': AUTOMATIONS,
      '/reports': { reports: [], truncated: false },
      '/active-runs': { holders: [], queued: [], lanes: [] },
    },
  })
  await page.emulateMedia({ colorScheme: 'dark' })
  await page.goto(path)
  await page.evaluate(() => document.fonts.ready)
}

test('a phone lists each automation in one row: schedule, how it last went, Run', async ({ page }, info) => {
  test.skip(info.project.name !== 'mobile', 'phone layout')
  await open(page, 'automations')
  const rows = page.locator('[data-test="automation-row"]:visible')
  await expect(rows).toHaveCount(2)
  await expect(rows.first()).toContainText('Every day at 02:00 · worked')
  const height = (await rows.first().boundingBox())!.height
  expect(height).toBeLessThan(80)
  await expectTapArea(rows.first().getByRole('button', { name: /^Run / }))
  await expectNoSidewaysScroll(page)
  await lookAt(page, info, 'automations-phone')
})

test('every automation opens on its result, with Live run, History and Setup beside it', async ({ page }, info) => {
  await open(page, 'automations/workspace-1%2Fradio-watch-report')
  for (const name of ['Results', 'Live run', 'History', 'Setup']) await expect(page.getByRole('radio', { name })).toBeVisible()
  await expect(page.getByRole('radio', { name: 'Results' })).toHaveAttribute('aria-checked', 'true')
  await expect(page.getByText('Final Report')).toHaveCount(0)
  await expect(page.getByText('Task file')).toHaveCount(0)
  await expectNoSidewaysScroll(page)
  await lookAt(page, info, 'automation-result')
  await page.getByRole('radio', { name: 'History' }).click()
  await lookAt(page, info, 'automation-history')
})
