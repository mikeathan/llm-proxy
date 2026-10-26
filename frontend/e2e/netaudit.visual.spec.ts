import { expect, test, type Page } from '@playwright/test'
import { DEFAULT_CONFIG } from '../src/composables/models/useConfig'

// Settings → Network scan at both widths, Simple and Advanced: nothing may scroll sideways and the main button is a
// comfortable touch target. The screenshots are for looking at; they are not compared.
const scope = {
  ranges: [{ cidr: '192.168.1.0/24', devices: 254 }],
  allow_unconfined: false,
  suggestions: [{ interface: 'en0', cidr: '10.0.0.0/24', devices: 254 }],
}
const audit = {
  id: 'ws/network-audit', workspace: 'ws', name: 'network-audit', task_file: 'network-audit.md', strategy: 'persistent',
  trigger: 'cron', trigger_value: '0 2 * * *', profile: 'security-auditor', history: [{ id: 'r1', timestamp: '2026-10-25T02:00:00Z' }],
}
const readiness = {
  ready: false,
  requirements: [
    { code: 'nmap_missing', message: 'nmap is not available.', blocking: false, fix: { kind: 'copy', text: 'sudo apt install nmap' } },
    { code: 'no_sandbox', message: 'This host cannot confine programs.', blocking: false, fix: { kind: 'confirm', action: 'allow_unconfined' } },
  ],
}

async function open(page: Page, view: 'simple' | 'advanced') {
  await page.addInitScript((v) => localStorage.setItem('admin-ui:netaudit-view', JSON.stringify({ version: 1, data: v })), view)
  await page.route('**/admin/api/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    const json = path.endsWith('/state')
      ? { config: structuredClone(DEFAULT_CONFIG), models: [], available_models: [] }
      : path.endsWith('/netaudit/scope')
        ? scope
        : path.endsWith('/netaudit/readiness')
          ? readiness
          : path.endsWith('/automations')
            ? [audit]
            : path.endsWith('/manifests') || path.endsWith('/mcp') || path.endsWith('/workspaces') || path.endsWith('/profiles') || path.endsWith('/profile-schema')
            ? []
            : {}
    await route.fulfill({ json })
  })
  await page.goto('settings/network-scan')
  await expect(page.getByRole('heading', { name: 'What may be scanned' })).toBeVisible()
}

for (const view of ['simple', 'advanced'] as const) {
  test(`network scan settings (${view}) fit the screen`, async ({ page }, testInfo) => {
    await open(page, view)
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)
    expect(overflow).toBeLessThanOrEqual(0)
    if (testInfo.project.name === 'mobile') {
      const open = await page.getByRole('button', { name: 'Open', exact: true }).boundingBox()
      expect(open?.height).toBeGreaterThanOrEqual(44)
      if (view === 'advanced') {
        const another = await page.getByRole('button', { name: 'Add another audit' }).boundingBox()
        expect(another?.height).toBeGreaterThanOrEqual(44)
      }
    }
    await page.screenshot({ path: testInfo.outputPath(`network-scan-${view}.png`), fullPage: true })
  })
}
