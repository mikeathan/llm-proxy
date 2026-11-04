import { expect, test, type Page } from '@playwright/test'
import { expectNoSidewaysScroll, lookAt, mockAdminApi, setNetauditView } from './support/adminApi'

// Network → Settings → Ubuntu security data, in each state it can be in, at both widths and in Simple and Advanced: nothing may
// scroll sideways, the buttons are touch targets on a phone, and a failed update says why in the server's own words.
const HELD = { supported: true, host_release: 'noble', running: false, release: 'noble', notices: 981, packages: 2218, generated_at: '2026-11-04T00:46:40Z', imported_at: new Date(Date.now() - 3_600_000).toISOString() }
const STATES = {
  empty: { supported: true, host_release: 'noble', running: false, notices: 0, packages: 0 },
  held: HELD,
  old: { ...HELD, imported_at: new Date(Date.now() - 30 * 86_400_000).toISOString() },
  unsupported: { supported: false, running: false, notices: 0, packages: 0 },
  trouble: { ...HELD, reason: 'The saved Ubuntu security data could not be read, so it was set aside. Update it again.' },
} as const

async function open(page: Page, view: 'simple' | 'advanced', state: (typeof STATES)[keyof typeof STATES]) {
  await setNetauditView(page, view)
  await mockAdminApi(page, { answers: { '/netaudit/ubuntu-data': state } })
  await page.goto('network/settings')
  await expect(page.getByRole('region', { name: 'Ubuntu security data' })).toBeVisible()
}

for (const view of ['simple', 'advanced'] as const) {
  for (const name of Object.keys(STATES) as (keyof typeof STATES)[]) {
    test(`ubuntu data, ${name} (${view}), fits the screen`, async ({ page }, testInfo) => {
      await open(page, view, STATES[name])
      const card = page.locator('[data-test="ubuntu-data"]')
      await expectNoSidewaysScroll(page)
      if (name === 'unsupported') {
        await expect(card.getByText('not running Ubuntu')).toBeVisible()
      } else {
        const button = card.locator('[data-test="ubuntu-update"]')
        await expect(button).toBeVisible()
        if (testInfo.project.name === 'mobile') expect((await button.boundingBox())?.height).toBeGreaterThanOrEqual(44)
      }
      if (name === 'held' && view === 'advanced') await expect(card.getByText('2,218')).toBeVisible()
      if (name === 'held' && view === 'simple') await expect(card.getByText('2,218')).toHaveCount(0)
      if (name === 'old') await expect(card.getByText('more than two weeks old')).toBeVisible()
      await lookAt(card, testInfo, `ubuntu-data-${name}-${view}`)
    })
  }

  test(`ubuntu data, a failed update (${view}) says why and keeps what was held`, async ({ page }, testInfo) => {
    await open(page, view, HELD)
    await page.route('**/admin/api/netaudit/ubuntu-data/update', (route) => route.fulfill({ status: 502, contentType: 'application/json', body: JSON.stringify({ error: 'The notices could not be downloaded. Check the connection and the Guardrails internet switch, then try again.' }) }))
    const card = page.locator('[data-test="ubuntu-data"]')
    await card.locator('[data-test="ubuntu-update"]').click()
    await expect(card.locator('[data-test="ubuntu-error"]')).toContainText('Guardrails internet switch')
    await expect(card.locator('[data-test="ubuntu-line"]')).toContainText('981 security notices')
    await expectNoSidewaysScroll(page)
    await lookAt(card, testInfo, `ubuntu-data-failed-${view}`)
  })
}
