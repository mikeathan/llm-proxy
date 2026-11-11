import { expect, test } from '@playwright/test'
import { expectNoSidewaysScroll, mockAdminApi } from './support/adminApi'

// Network → Settings → Known-bad names: the description, the link to abuse.ch, the field for the address (hidden, a secret), the
// download button and what is held. At both widths nothing scrolls sideways and the controls are touch targets on a phone.
const scope = { ranges: [{ cidr: '192.168.1.0/24', devices: 254 }], allow_unconfined: false, device_timeout_minutes: 0, suggestions: [] }
const loadedAt = new Date(Date.now() - 20 * 60_000).toISOString()

for (const [name, state] of Object.entries({
  fresh: { count: 0, has_address: false, ready: false, reason: 'no list address is saved yet' },
  saved: { count: 0, has_address: true, ready: false, reason: 'the list is not downloaded yet' },
  loaded: { count: 453, loaded_at: loadedAt, has_address: true, ready: true },
})) {
  test(`known-bad names card, ${name}, fits the screen`, async ({ page }, testInfo) => {
    await mockAdminApi(page, { answers: { '/netaudit/scope': scope, '/netaudit/known-bad': state } })
    await page.goto('network/settings?group=outside')
    const card = page.locator('[data-test="known-bad"]')
    await expect(card).toBeVisible()
    await expect(card.locator('[data-test="known-bad-link"]')).toHaveAttribute('href', 'https://urlhaus.abuse.ch/api/')
    await expect(card.locator('[data-test="known-bad-address"]')).toHaveAttribute('type', 'password')
    await expect(card.locator('[data-test="known-bad-download"]')).toBeVisible()
    if (state.count) await expect(card.locator('[data-test="known-bad-line"]')).toContainText('453 malware host names held')
    await card.locator('[data-test="known-bad-howto-toggle"]').click()
    await expect(card.locator('[data-test="known-bad-howto"]')).toBeVisible()
    await expectNoSidewaysScroll(page)
    if (testInfo.project.name === 'mobile') {
      for (const sel of ['known-bad-download', 'known-bad-address', 'known-bad-howto-toggle', 'known-bad-link']) {
        expect((await card.locator(`[data-test="${sel}"]`).boundingBox())?.height, sel).toBeGreaterThanOrEqual(44)
      }
    }
  })
}
