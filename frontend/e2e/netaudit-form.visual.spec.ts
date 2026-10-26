import { expect, test, type Page } from '@playwright/test'
import { DEFAULT_CONFIG } from '../src/composables/models/useConfig'

// The audit's edit form with its scan settings, Simple and Advanced at both widths: nothing may scroll sideways, and the
// screenshots are for looking at.
const automation = {
  id: 'ws/network-audit', workspace: 'ws', name: 'network-audit', task_file: 'network-audit.md', strategy: 'persistent', trigger: 'cron',
  trigger_value: '0 2 * * *', profile: 'security-auditor', network_grant: 'lan', journal: false,
  scan: { depth: 'thorough', checks: true }, history: [],
}
const profiles = [{ id: 'security-auditor', name: 'Security auditor', summary: 'Scans the networks you allowed.', source: 'builtin', answer_first: false, may_ask: false, memory_save: 'off', reminders: false, default_for: [] }]

async function open(page: Page, view: 'simple' | 'advanced') {
  await page.addInitScript((v) => localStorage.setItem('admin-ui:netaudit-view', JSON.stringify({ version: 1, data: v })), view)
  await page.route('**/admin/api/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    const json = path.endsWith('/state')
      ? { config: structuredClone(DEFAULT_CONFIG), models: [], available_models: [] }
      : path.endsWith('/netaudit/readiness')
        ? { ready: true, requirements: [] }
        : path.endsWith('/automations')
          ? [automation]
          : path.endsWith('/workspaces')
            ? [{ id: 'ws' }]
            : path.endsWith('/profiles')
              ? profiles
              : path.endsWith('/manifests') || path.endsWith('/mcp') || path.endsWith('/profile-schema')
                ? []
                : {}
    await route.fulfill({ json })
  })
  await page.goto('automations/ws%2Fnetwork-audit/edit')
  await expect(page.getByRole('heading', { name: 'What the audit scans' })).toBeVisible()
}

for (const view of ['simple', 'advanced'] as const) {
  test(`audit form scan settings (${view}) fit the screen`, async ({ page }, testInfo) => {
    await open(page, view)
    expect(await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)).toBeLessThanOrEqual(0)
    await page.getByRole('heading', { name: 'What the audit scans' }).scrollIntoViewIfNeeded()
    await page.screenshot({ path: testInfo.outputPath(`form-${view}.png`), fullPage: true })
  })
}
