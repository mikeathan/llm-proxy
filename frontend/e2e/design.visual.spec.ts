import { expect, test, type Page } from '@playwright/test'

// Keys and version mirror src/composables/ui/useTheme.ts (contract-tested there).
const SELECTION_KEY = 'admin-ui:theme-selection'
const APPLIED_KEY = 'admin-ui:theme-applied'
const SCHEMES = { 'retro-dark': 'dark', 'retro-dark-soft': 'dark', 'retro-paper': 'light' } as const

async function storeTheme(page: Page, id: keyof typeof SCHEMES) {
  await page.addInitScript(
    ([selectionKey, appliedKey, themeId, scheme]) => {
      const envelope = (data: unknown) => JSON.stringify({ version: 1, data })
      localStorage.setItem(selectionKey, envelope({ kind: 'preset', id: themeId }))
      localStorage.setItem(appliedKey, envelope({ themeId, presetId: themeId, scheme, overrides: {} }))
    },
    [SELECTION_KEY, APPLIED_KEY, id, SCHEMES[id]] as const,
  )
}

for (const id of Object.keys(SCHEMES) as (keyof typeof SCHEMES)[]) {
  test(`/design in ${id}`, async ({ page }) => {
    await storeTheme(page, id)
    await page.goto('design')
    await expect(page.locator('html')).toHaveAttribute('data-theme', id)
    await expect(page.getByRole('heading', { name: 'Tokens', exact: true })).toBeVisible()
    await page.evaluate(() => document.fonts.ready)
    await expect(page.getByText('all pass')).toBeVisible()
    await expect(page).toHaveScreenshot(`design-${id}.png`, { fullPage: true })
  })
}

test('the stored theme is on <html> before the app script runs (no flash)', async ({ page }) => {
  await storeTheme(page, 'retro-paper')
  await page.addInitScript(() => {
    document.addEventListener('DOMContentLoaded', () => {
      ;(window as unknown as { themeAtParse: string | undefined }).themeAtParse = document.documentElement.dataset.theme
    })
  })
  await page.goto('design')
  expect(await page.evaluate(() => (window as unknown as { themeAtParse?: string }).themeAtParse)).toBe('retro-paper')
})

test('with no stored choice the theme follows the OS scheme', async ({ page }) => {
  await page.emulateMedia({ colorScheme: 'light' })
  await page.goto('design')
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'retro-paper')
  await page.emulateMedia({ colorScheme: 'dark' })
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'retro-dark-soft')
})
