import { expect, test } from '@playwright/test'
import { expectNoSidewaysScroll, mockAdminApi } from './support/adminApi'

// Stacks: Network and Agents sit under a Stacks heading, in the sidebar on a wide screen and in the More sheet on a phone, and
// every Network tab is reachable from the page. Nothing scrolls sideways; touch targets are 44 px on a phone.
test('Network and Agents are under Stacks and every Network tab fits the screen', async ({ page }, testInfo) => {
  await mockAdminApi(page)
  await page.goto('network')
  await expect(page.getByRole('heading', { name: 'Network', level: 1 })).toBeVisible()
  const mobile = testInfo.project.name === 'mobile'
  if (mobile) {
    await page.getByRole('button', { name: 'More' }).click()
    const sheet = page.getByRole('dialog', { name: 'More' })
    await expect(sheet.getByText('Stacks', { exact: true })).toBeVisible()
    for (const name of ['Network', 'Agents']) {
      const link = sheet.getByRole('link', { name })
      await expect(link).toBeVisible()
      expect((await link.boundingBox())?.height).toBeGreaterThanOrEqual(44)
    }
    await page.keyboard.press('Escape')
  } else {
    const group = page.getByRole('button', { name: 'Stacks' })
    await expect(group).toHaveAttribute('aria-expanded', 'true')
    await expect(page.locator('#group-stacks').getByRole('link', { name: 'Network' })).toHaveAttribute('aria-current', 'page')
    await expect(page.locator('#group-stacks').getByRole('link', { name: 'Agents' })).toBeVisible()
    await group.click()
    // While Network is open the group stays open: the current page is never hidden.
    await expect(group).toHaveAttribute('aria-expanded', 'true')
  }
  for (const tab of ['Overview', 'Devices', 'Findings', 'Audits', 'Modules', 'Settings']) {
    const link = page.getByRole('link', { name: tab, exact: true }).last()
    await link.click()
    await expect(page.getByRole('heading', { name: 'Network', level: 1 })).toBeVisible()
    await expectNoSidewaysScroll(page)
  }
})
