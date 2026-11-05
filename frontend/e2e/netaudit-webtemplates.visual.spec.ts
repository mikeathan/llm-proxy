import { expect, test } from '@playwright/test'
import { expectNoSidewaysScroll, mockAdminApi } from './support/adminApi'

// Network → Settings → Web check templates: how many reviewed checks are held, why the web checks cannot run yet, and one button.
// At both widths nothing scrolls sideways and the button is a touch target on a phone.
const scope = { ranges: [{ cidr: '192.168.1.0/24', devices: 254 }], allow_unconfined: false, device_timeout_minutes: 0, suggestions: [] }

for (const [name, state] of Object.entries({
  missing: { present: 0, total: 24, tag: 'v10.5.1', ready: false, reason: 'nuclei is not installed; install it with: brew install nuclei' },
  partial: { present: 9, total: 24, tag: 'v10.5.1', ready: false, reason: 'the reviewed web-check templates are not downloaded (9 of 24 present)' },
  ready: { present: 24, total: 24, tag: 'v10.5.1', ready: true },
})) {
  test(`web check templates, ${name}, fit the screen`, async ({ page }, testInfo) => {
    await mockAdminApi(page, { answers: { '/netaudit/scope': scope, '/netaudit/web-templates': state } })
    await page.goto('network/settings')
    const card = page.locator('[data-test="web-templates"]')
    await expect(card).toBeVisible()
    await expect(card.locator('[data-test="web-templates-line"]')).toContainText(`${state.present === state.total ? 'All 24' : `${state.present} of 24`}`)
    if (state.reason) await expect(card.locator('[data-test="web-templates-reason"]')).toContainText(state.reason)
    await expectNoSidewaysScroll(page)
    if (testInfo.project.name === 'mobile') expect((await card.getByRole('button').first().boundingBox())?.height).toBeGreaterThanOrEqual(44)
  })
}
