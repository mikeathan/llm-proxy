import { expect, test, type Page } from '@playwright/test'
import { expectNoSidewaysScroll, lookAt, mockAdminApi, setNetauditView } from './support/adminApi'

// Settings → Network scan → Decoy services, off, on and with a device that touched one: at both widths and in Simple and
// Advanced nothing may scroll sideways and the switch row is a comfortable touch target.
const scope = { ranges: [{ cidr: '192.168.1.0/24', devices: 254 }], allow_unconfined: false, device_timeout_minutes: 0, suggestions: [] }
const ports = [{ number: 2222, name: 'SSH login' }, { number: 4445, name: 'File sharing' }, { number: 8023, name: 'Telnet' }]
const last = new Date(Date.now() - 20 * 60_000).toISOString()
const STATES = {
  off: { enabled: false, listening: [], ports, touches: [] },
  on: { enabled: true, listening: [2222, 4445, 8023], ports, touches: [] },
  touched: { enabled: true, listening: [2222, 4445], problem: 'port 8023 is in use or not allowed', ports, touches: [{ source: '192.168.1.77', port: 2222, first: last, last, count: 3 }] },
} as const

async function open(page: Page, view: 'simple' | 'advanced', decoys: object) {
  await setNetauditView(page, view)
  await mockAdminApi(page, { answers: { '/netaudit/scope': scope, '/netaudit/decoys': decoys } })
  await page.goto('settings/network-scan')
  await expect(page.getByRole('heading', { name: 'Decoy services' })).toBeVisible()
}

for (const view of ['simple', 'advanced'] as const) {
  for (const name of Object.keys(STATES) as (keyof typeof STATES)[]) {
    test(`decoy services card, ${name} (${view}), fits the screen`, async ({ page }, testInfo) => {
      await open(page, view, STATES[name])
      await expectNoSidewaysScroll(page)
      const card = page.locator('[data-test="decoys"]')
      const toggle = card.getByRole('switch', { name: 'Run decoy services' })
      await expect(toggle).toBeVisible()
      if (name === 'touched') {
        await expect(card.getByText('192.168.1.77')).toBeVisible()
        await expect(card.getByText(/cannot start/)).toBeVisible()
      }
      if (view === 'advanced') await expect(card.getByText('SSH login')).toBeVisible()
      else await expect(card.getByText('SSH login')).toHaveCount(0)
      await lookAt(page.getByRole('region', { name: 'Decoy services' }), testInfo, `decoys-${name}-${view}`)
    })
  }
}
