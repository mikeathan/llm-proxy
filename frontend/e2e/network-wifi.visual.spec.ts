import { expect, test } from '@playwright/test'
import { expectNoSidewaysScroll, mockAdminApi } from './support/adminApi'

// Network → Settings → Wi-Fi look-alikes: the access points that advertise the operator's name, each with its address, signal and
// security, a checkbox only on the unconfirmed ones, and one confirm button. At both widths nothing scrolls sideways, and on a phone the
// controls are 44 px targets. The data is invented; the shape is the server's.
const WIFI = {
  enabled: true, ready: true, last_look: new Date(Date.now() - 120_000).toISOString(),
  networks: [{ name: 'home', confirmed: ['aa:bb:cc:00:00:01'], seen: [
    { bssid: 'aa:bb:cc:00:00:01', signal: 55, security: 'wpa2', confirmed: true },
    { bssid: 'de:ad:be:ef:00:09', signal: 40, security: 'open', confirmed: false },
  ] }],
}

test('the Wi-Fi look-alikes card lists what advertises the name and fits the screen', async ({ page }, testInfo) => {
  await mockAdminApi(page, { answers: { '/netaudit/wifi': WIFI } })
  await page.goto('network/settings?group=watching')
  const card = page.locator('[data-test="wifi"]')
  await expect(card).toBeVisible()
  await expect(card.locator('[data-test="wifi-seen"]')).toHaveCount(2)
  await expect(card).toContainText('de:ad:be:ef:00:09')
  await expect(card).toContainText('no security')
  await expectNoSidewaysScroll(page)
  if (testInfo.project.name === 'mobile') {
    for (const target of [card.locator('[data-test="wifi-look"]'), card.locator('[data-test="wifi-remove"]'), card.locator('[data-test="wifi-name"]')]) {
      expect((await target.boundingBox())?.height).toBeGreaterThanOrEqual(44)
    }
    // The box is small by the repo's convention (CheckboxGroup); the label row around it is the 44 px target.
    expect((await card.locator('label:has(input[type="checkbox"])').first().boundingBox())?.height).toBeGreaterThanOrEqual(44)
  }
})
