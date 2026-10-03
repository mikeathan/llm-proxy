import { expect, test } from '@playwright/test'
import { DEFAULT_CONFIG } from '../src/composables/models/useConfig'

test('saved Telegram secrets are visibly masked in the edit form', async ({ page }, testInfo) => {
  const config = structuredClone(DEFAULT_CONFIG)
  config.communication.connectors = {
    tg: { type: 'telegram', enabled: true, secret_ref: 'tg', settings: { chat_id: '42', webhook_token: 'fixture-webhook-secret' } },
  }
  await page.route('**/admin/api/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    const json = path.endsWith('/state')
      ? { config, models: [], available_models: [] }
      : path.endsWith('/secrets/tools')
        ? { secret: '••••1234' }
        : path.endsWith('/manifests') || path.endsWith('/mcp') || path.endsWith('/workspaces')
          ? []
          : {}
    await route.fulfill({ json })
  })
  await page.goto('settings/communication')
  await page.getByRole('button', { name: 'Edit tg', exact: true }).click()
  const token = page.getByLabel('Bot token', { exact: true })
  const webhookSecret = page.getByLabel('Webhook secret token', { exact: true })
  await expect(token).toHaveAttribute('placeholder', '••••1234')
  await expect(token).toHaveValue('')
  await expect(webhookSecret).toHaveAttribute('placeholder', '********')
  await expect(webhookSecret).toHaveValue('')
  expect(await page.content()).not.toContain('fixture-webhook-secret')
  await webhookSecret.scrollIntoViewIfNeeded()
  await page.screenshot({ path: testInfo.outputPath('telegram-saved-secrets.png'), fullPage: true })
})

test('Telegram setup help follows the theme and preserves the form draft', async ({ page }, testInfo) => {
  await page.route('**/admin/api/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    const json = path.endsWith('/state')
      ? { config: DEFAULT_CONFIG, models: [], available_models: [] }
      : path.endsWith('/manifests') || path.endsWith('/mcp') || path.endsWith('/workspaces')
        ? []
        : {}
    await route.fulfill({ json })
  })
  await page.emulateMedia({ colorScheme: 'dark' })
  await page.goto('settings/communication')
  await page.getByRole('button', { name: 'Add connector', exact: true }).click()
  await page.getByLabel('Connector name', { exact: true }).fill('my-draft')
  await page.getByRole('button', { name: 'Telegram setup help', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: 'Telegram setup help' })
  await expect(dialog).toBeVisible()
  await expect(dialog.getByText('Two values, two different jobs')).toBeVisible()
  const darkBackground = await dialog.evaluate((element) => getComputedStyle(element).backgroundColor)
  await page.evaluate(() => document.fonts.ready)
  await page.screenshot({ path: testInfo.outputPath('telegram-help-dark.png') })
  await page.emulateMedia({ colorScheme: 'light' })
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'retro-paper')
  await expect.poll(() => dialog.evaluate((element) => getComputedStyle(element).backgroundColor)).not.toBe(darkBackground)
  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true)
  await page.screenshot({ path: testInfo.outputPath('telegram-help-light.png') })
  await dialog.getByText('How messages travel', { exact: true }).scrollIntoViewIfNeeded()
  await page.screenshot({ path: testInfo.outputPath('telegram-help-flow.png') })
  const close = dialog.getByRole('button', { name: 'Close Telegram setup help' })
  await close.focus()
  await page.keyboard.press('Shift+Tab')
  await expect(dialog.getByRole('link', { name: "Source: Telegram's sending messages guide" })).toBeFocused()
  await page.keyboard.press('Tab')
  await expect(close).toBeFocused()
  await page.keyboard.press('Escape')
  await expect(dialog).toBeHidden()
  await expect(page.getByRole('button', { name: 'Telegram setup help', exact: true })).toBeFocused()
  await expect(page.getByLabel('Connector name', { exact: true })).toHaveValue('my-draft')
})
