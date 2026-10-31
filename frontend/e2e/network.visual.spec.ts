import { expect, test } from '@playwright/test'
import { expectNoSidewaysScroll, lookAt, mockAdminApi, setNetauditView } from './support/adminApi'

// Network → Overview: one verdict, four figures with their trend, and what changed. At both widths and in both views
// nothing may scroll sideways; the tabs and the actions are comfortable touch targets. The screenshots are for looking at.
const seen = new Date(Date.now() - 5 * 60_000).toISOString()
const day = (offset: number, up: number, devices: number, findings: number) => ({
  date: new Date(Date.now() - offset * 86_400_000).toISOString().slice(0, 10), devices_up: up, devices, unnamed: 3, findings, serious: findings ? 1 : 0, audited: true,
})
const series = [day(4, 7, 10, 6), day(3, 8, 11, 5), day(2, 8, 11, 5), day(1, 9, 12, 4), day(0, 9, 12, 4)]
const STATES = {
  quiet: {
    posture: 'quiet', devices: { known: 12, unnamed: 3, new: 0, active: 9 }, findings: { scanned: true, scanned_at: seen, open: 4, serious: 0, new: 0 },
    gateway_changed: false, shared_address: false, presence: { last_check: seen }, series,
    recent: [{ kind: 'new_device', at: seen, ip: '192.168.1.77' }],
  },
  attention: {
    posture: 'attention', devices: { known: 12, unnamed: 3, new: 2, active: 9 }, findings: { scanned: true, scanned_at: seen, open: 4, serious: 1, new: 1 },
    gateway_changed: true, shared_address: true, presence: { last_check: seen, problem: 'this computer cannot list the devices on its network (only Linux can)' }, series,
    recent: [{ kind: 'address_changed', at: seen, ip: '192.168.1.9' }, { kind: 'new_device', at: seen, ip: '192.168.1.77' }],
  },
  empty: {
    posture: 'unknown', devices: { known: 0, unnamed: 0, new: 0, active: 0 }, findings: { scanned: false, open: 0, serious: 0, new: 0 },
    gateway_changed: false, shared_address: false, presence: {}, series: [], recent: [],
  },
} as const

for (const view of ['simple', 'advanced'] as const) {
  for (const name of Object.keys(STATES) as (keyof typeof STATES)[]) {
    test(`network overview, ${name} (${view}), fits the screen`, async ({ page }, testInfo) => {
      await setNetauditView(page, view)
      await mockAdminApi(page, { answers: { '/netaudit/overview': STATES[name] } })
      await page.goto('network')
      await expect(page.getByRole('heading', { name: 'Network', level: 1 })).toBeVisible()
      await expect(page.locator('[data-test="posture"]')).toBeVisible()
      await expectNoSidewaysScroll(page)
      if (testInfo.project.name === 'mobile') {
        for (const tab of await page.getByRole('link', { name: /^(Overview|Devices|Findings)$/ }).all()) {
          expect((await tab.boundingBox())?.height).toBeGreaterThanOrEqual(44)
        }
      }
      if (name === 'attention') await expect(page.getByText("The router's hardware address changed")).toBeVisible()
      await lookAt(page, testInfo, `network-${name}-${view}`)
    })
  }
}

test('the findings tab without an audit says where to set one up', async ({ page }) => {
  await mockAdminApi(page)
  await page.goto('network/findings')
  await expect(page.getByText('No findings yet')).toBeVisible()
  await expectNoSidewaysScroll(page)
})
