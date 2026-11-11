import { expect, test } from '@playwright/test'
import { expectNoSidewaysScroll, expectTapArea, lookAt, mockAdminApi } from './support/adminApi'

// Network → Traffic: a look at this computer's own traffic. With the helper ready and the action on it offers one button and shows the last
// look as a sentence, ranked places, kinds, a chart and the conversations; with the action off it says why and where the fix is. At both
// widths nothing scrolls sideways, and on a phone the controls are 44 px targets. The data is invented; the shape is the server's.
const MB = 1024 * 1024
const READY = {
  supported: true, installed: true, ready: true, declared_ranges: [], install_command: 'sudo x', help_path: 'docs/service_setup.md',
  actions: [{ id: 'capture_host', on: true, installed: true }],
}
const SECONDS = Array.from({ length: 60 }, (_, i) => (i > 12 && i < 24 ? 2 * MB - Math.abs(18 - i) * 200_000 : 60_000 + (i % 7) * 10_000))
const LOOK = {
  at: new Date(Date.now() - 90_000).toISOString(), interface: 'wlp2s0', stopped_by: 'window', elapsed_ms: 60_000, frames: 4100, unreadable: 0, kernel_seen: 4100, kernel_dropped: 0, sample: 4,
  bytes_in: 38 * MB, bytes_out: 4 * MB, bytes_through: 0, internet_bytes: 41 * MB, local_network_bytes: MB, lookups: 212,
  kinds: [{ kind: 'https', bytes: 40 * MB }, { kind: 'dns', bytes: MB }, { kind: 'udp', bytes: MB }],
  per_second: SECONDS,
  destinations: [
    { name: 'generativelanguage.googleapis.com', address: '142.250.1.1', bytes: 22 * MB, internet: true },
    { name: 'github.com', address: '140.82.1.1', bytes: 10 * MB, internet: true },
    { name: 'registry.npmjs.org', address: '104.16.1.1', bytes: 3 * MB, internet: true },
    { address: '34.120.1.1', bytes: MB, internet: true },
  ],
  flows: [
    { address: '142.250.1.1', name: 'generativelanguage.googleapis.com', port: 443, proto: 'tcp', kind: 'https', internet: true, bytes_out: 2 * MB, bytes_in: 20 * MB },
    { address: '140.82.1.1', name: 'github.com', port: 443, proto: 'tcp', kind: 'https', internet: true, bytes_out: MB, bytes_in: 9 * MB },
    { address: '192.168.50.1', port: 53, proto: 'udp', kind: 'dns', internet: false, bytes_out: 100_000, bytes_in: 900_000 },
  ],
  other_flows: 19, other_bytes: 2 * MB,
}

test('the traffic tab shows the last look and fits the screen', async ({ page }, testInfo) => {
  await mockAdminApi(page, { answers: { '/admin/api/helper': READY, '/netaudit/host-traffic': { summary: LOOK } } })
  await page.goto('network/traffic')
  const tab = page.locator('[data-test="traffic-tab"]')
  await expect(tab.locator('[data-test="traffic-headline"]')).toContainText('sent 4 MB and received 38 MB')
  await expect(tab.locator('[data-test="destination"]')).toHaveCount(4)
  await expect(tab.locator('[data-test="destination-name"]').first()).toHaveText('generativelanguage.googleapis.com')
  await expect(tab.locator('[data-test="traffic-chart"] rect')).toHaveCount(60)
  await expect(tab.locator('[data-test="flow"]')).toHaveCount(3)
  await expect(tab.locator('[data-test="traffic-notes"]')).toContainText('Estimated')
  await expectNoSidewaysScroll(page)
  if (testInfo.project.name === 'mobile') await expectTapArea(tab.locator('[data-test="traffic-listen"]'))
  await lookAt(page, testInfo, 'traffic-result.png', true)
})

test('with the action switched off the tab says why and offers the fix, and keeps the last look readable', async ({ page }, testInfo) => {
  await mockAdminApi(page, { answers: { '/admin/api/helper': { ...READY, actions: [{ id: 'capture_host', on: false, installed: true }] }, '/netaudit/host-traffic': { summary: LOOK } } })
  await page.goto('network/traffic')
  const tab = page.locator('[data-test="traffic-tab"]')
  await expect(tab.locator('[data-test="traffic-unavailable"]')).toContainText('switched off')
  await expect(tab.locator('[data-test="traffic-listen"]')).toHaveCount(0)
  await expect(tab.locator('[data-test="traffic-headline"]')).toBeVisible()
  await expectNoSidewaysScroll(page)
  if (testInfo.project.name === 'mobile') await expectTapArea(tab.locator('[data-test="traffic-fix"]'))
  await tab.locator('[data-test="traffic-fix"]').click()
  await expect(page).toHaveURL(/network\/settings/)
})

test('before the first look the tab invites one', async ({ page }) => {
  await mockAdminApi(page, { answers: { '/admin/api/helper': READY, '/netaudit/host-traffic': { summary: null } } })
  await page.goto('network/traffic')
  await expect(page.locator('[data-test="traffic-never"]')).toContainText('Nothing looked at yet')
  await expect(page.locator('[data-test="traffic-listen"]')).toBeVisible()
  await expectNoSidewaysScroll(page)
})

test('with the table of address owners the tab names who runs each place, ranks holders and countries, and fits the screen', async ({ page }, testInfo) => {
  const OWNED = {
    ...LOOK, owners_used: true,
    owners: [
      { asn: 15169, name: 'GOOGLE - Google LLC, with a name long enough to wrap on a phone', country: 'US', bytes: 22 * MB },
      { asn: 36459, name: 'GITHUB', country: 'US', bytes: 10 * MB },
      { asn: 13335, name: 'CLOUDFLARENET - Cloudflare, Inc.', country: 'DE', bytes: 3 * MB },
    ],
    countries: [{ country: 'US', bytes: 32 * MB }, { country: 'DE', bytes: 3 * MB }],
    destinations: LOOK.destinations.map((d, i) => (i < 3 ? { ...d, name_from: i === 1 ? 'tls' : 'dns', owner: { asn: 15169, name: 'GOOGLE - Google LLC', country: 'US' } } : d)),
  }
  await mockAdminApi(page, { answers: { '/admin/api/helper': READY, '/netaudit/host-traffic': { summary: OWNED }, '/netaudit/destination-data': { ranges: 579453, loaded_at: new Date().toISOString(), source: 'iptoasn.com' }, '/netaudit/destination-places': { ranges: 0, source: 'db-ip.com' } } })
  await page.goto('network/traffic')
  const tab = page.locator('[data-test="traffic-tab"]')
  await expect(tab.locator('[data-test="destination-owner"]').first()).toContainText('GOOGLE - Google LLC · United States')
  // The operators and the countries are two views of the same panel: one is open at a time.
  await tab.getByRole('radio', { name: 'Operators' }).click()
  await expect(tab.locator('[data-test="where-operators"] [data-test="where-row"]')).toHaveCount(3)
  await tab.getByRole('radio', { name: 'Countries' }).click()
  await expect(tab.locator('[data-test="where-countries"] [data-test="where-row"]')).toHaveCount(2)
  await expect(tab.locator('[data-test="destination-data-line"]')).toContainText('579,453 address ranges')
  await expectNoSidewaysScroll(page)
  if (testInfo.project.name === 'mobile') {
    // The tap check reads the viewport, and the clicks above scrolled the button away.
    await tab.locator('[data-test="destination-data-download"]').scrollIntoViewIfNeeded()
    await expectTapArea(tab.locator('[data-test="destination-data-download"]'))
  }
  await lookAt(page, testInfo, 'traffic-owners.png', true)
})
