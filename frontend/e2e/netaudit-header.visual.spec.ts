import { expect, test } from '@playwright/test'
import { expectNoSidewaysScroll, lookAt, mockAdminApi, setNetauditView } from './support/adminApi'

// The Network page's header: the title, Simple/Advanced and Help share one row on a phone, so the page does not lose
// a screen's worth of height to its own controls. Help is a "?" with the name "Help", and still a touch target.
for (const view of ['simple', 'advanced'] as const) {
  test(`the header is one row (${view})`, async ({ page }, testInfo) => {
    await setNetauditView(page, view)
    await mockAdminApi(page, {})
    await page.goto('network')
    const title = page.getByRole('heading', { level: 1, name: 'Network' })
    const detail = page.getByRole('radiogroup', { name: 'How much detail' })
    const help = page.getByRole('button', { name: 'Help' })
    await expect(title).toBeVisible()
    await expect(detail).toBeVisible()
    await expect(help).toBeVisible()
    await expectNoSidewaysScroll(page)
    if (testInfo.project.name === 'mobile') {
      const [t, d, h] = [await title.boundingBox(), await detail.boundingBox(), await help.boundingBox()]
      // Centres within a few pixels of each other: one row, not a stack.
      expect(Math.abs(t!.y + t!.height / 2 - (d!.y + d!.height / 2))).toBeLessThan(12)
      expect(Math.abs(d!.y + d!.height / 2 - (h!.y + h!.height / 2))).toBeLessThan(12)
      expect(h!.height).toBeGreaterThanOrEqual(44)
      expect(h!.width).toBeGreaterThanOrEqual(44)
    }
    await lookAt(title.locator('xpath=ancestor::header[1]'), testInfo, `network-header-${view}`)
  })
}
