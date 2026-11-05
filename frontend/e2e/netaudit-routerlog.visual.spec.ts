import { expect, test, type Page } from '@playwright/test'
import { expectNoSidewaysScroll, lookAt, mockAdminApi, setNetauditView } from './support/adminApi'

// Network → Devices → Router log: off, listening but silent, heard, and unable to listen. At both widths nothing may scroll
// sideways, the card says whether it is on and has no switch, and the setup command (long lines) scrolls inside its own box rather than the page.
const scope = { ranges: [{ cidr: '192.168.1.0/24', devices: 254 }], allow_unconfined: false, device_timeout_minutes: 0, suggestions: [] }
const heard = new Date(Date.now() - 4 * 60_000).toISOString()
const STATES = {
  off: { enabled: false, listening: false, port: 5514 },
  silent: { enabled: true, listening: true, port: 5514 },
  heard: { enabled: true, listening: true, port: 5514, last_heard: heard },
  problem: { enabled: true, listening: false, port: 5514, problem: 'port 5514 is in use or not allowed' },
} as const

async function open(page: Page, view: 'simple' | 'advanced', routerLog: object) {
  await setNetauditView(page, view)
  await mockAdminApi(page, { answers: { '/netaudit/scope': scope, '/netaudit/router-log': routerLog } })
  await page.goto('network/settings')
  await expect(page.getByRole('heading', { name: 'Router log' })).toBeVisible()
}

for (const view of ['simple', 'advanced'] as const) {
  for (const name of Object.keys(STATES) as (keyof typeof STATES)[]) {
    test(`router log card, ${name} (${view}), fits the screen`, async ({ page }, testInfo) => {
      await open(page, view, STATES[name])
      const card = page.locator('[data-test="router-log"]')
      await expect(card.locator('[data-test="module-switch-state"]')).toBeVisible()
      await expect(card.getByRole('switch')).toHaveCount(0)
      if (name === 'heard') await expect(card.getByText(/router was last heard/)).toBeVisible()
      if (name === 'silent') await expect(card.getByText(/Nothing has been heard/)).toBeVisible()
      if (name === 'problem') await expect(card.getByText(/cannot listen/)).toBeVisible()
      await card.getByText('Point the router at this computer').click()
      await expect(card.getByText('redirect to :5514')).toBeVisible()
      await expectNoSidewaysScroll(page)
      await lookAt(page.getByRole('region', { name: 'Router log' }), testInfo, `router-log-${name}-${view}`)
    })
  }
}
