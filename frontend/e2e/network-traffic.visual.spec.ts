import { expect, test } from '@playwright/test'
import { expectNoSidewaysScroll, expectTapArea, lookAt, mockAdminApi, setNetauditView } from './support/adminApi'

// Network → Traffic: a look at this computer's own traffic. With the helper ready and the action on it offers one button and shows the last
// look as a sentence (with this program's own page set apart), a map of the internet traffic, and who the computer talked to (broadcast chatter
// closed); a place opens its detail and the route to it. The chart, totals and tables of operators and conversations are under Advanced. With
// the action off it says why and where the fix is. At both widths nothing scrolls sideways, and on a phone the controls are 44 px targets. The data
// is invented; the shape is the server's, after the direction fix (plan network-traffic-view-redesign).
const MB = 1024 * 1024
const READY = {
  supported: true, installed: true, ready: true, declared_ranges: [], install_command: 'sudo x', help_path: 'docs/service_setup.md',
  actions: [{ id: 'capture_host', on: true, installed: true }],
}
const SECONDS = Array.from({ length: 60 }, (_, i) => (i > 12 && i < 24 ? 2 * MB - Math.abs(18 - i) * 200_000 : 60_000 + (i % 7) * 10_000))
const owner = (asn: number, name: string, country: string, city: string, place: string, lat: number, lon: number) =>
  ({ asn, name, country, place_country: place, city, region: '', point: { lat, lon } })
const GOOGLE = owner(15169, 'GOOGLE - Google LLC', 'US', 'Mountain View', 'US', 37.4, -122.1)
const GITHUB = owner(36459, 'GITHUB - GitHub, Inc.', 'US', 'Frankfurt am Main', 'DE', 50.1, 8.7)
const CLOUDFLARE = owner(13335, 'CLOUDFLARENET - Cloudflare, Inc.', 'US', 'Toronto', 'CA', 43.7, -79.4)
const LOOK = {
  at: new Date(Date.now() - 90_000).toISOString(), interface: 'wlp2s0', stopped_by: 'window', elapsed_ms: 60_000, frames: 4100, unreadable: 0, kernel_seen: 4100, kernel_dropped: 0, sample: 4,
  bytes_in: 38 * MB, bytes_out: 6 * MB, bytes_through: 0, self_bytes: 2 * MB, internet_bytes: 41 * MB, local_network_bytes: 3 * MB, lookups: 212,
  kinds: [{ kind: 'https', bytes: 40 * MB }, { kind: 'dns', bytes: MB }, { kind: 'tcp', bytes: 2 * MB }, { kind: 'discovery', bytes: 100_000 }],
  per_second: SECONDS, owners_used: true,
  owners: [{ ...GOOGLE, bytes: 22 * MB }, { ...GITHUB, bytes: 10 * MB }, { ...CLOUDFLARE, bytes: 3 * MB }],
  countries: [{ country: 'US', bytes: 35 * MB }],
  destinations: [
    { name: 'generativelanguage.googleapis.com', name_from: 'tls', address: '142.250.1.1', bytes: 22 * MB, internet: true, owner: GOOGLE },
    { name: 'github.com', name_from: 'dns', address: '140.82.1.1', bytes: 10 * MB, internet: true, owner: GITHUB },
    { name: 'registry.npmjs.org', name_from: 'tls', address: '104.16.1.1', bytes: 3 * MB, internet: true, owner: CLOUDFLARE },
    { address: '192.168.50.251', bytes: 2 * MB + 40_000, internet: false },
    { address: '224.0.0.251', bytes: 100_000, internet: false },
  ],
  flows: [
    { address: '142.250.1.1', name: 'generativelanguage.googleapis.com', name_from: 'tls', port: 443, proto: 'tcp', kind: 'https', internet: true, bytes_out: 2 * MB, bytes_in: 20 * MB, owner: GOOGLE },
    { address: '140.82.1.1', name: 'github.com', name_from: 'dns', port: 443, proto: 'tcp', kind: 'https', internet: true, bytes_out: MB, bytes_in: 9 * MB, owner: GITHUB },
    { address: '192.168.50.251', port: 4001, served: true, self: true, proto: 'tcp', kind: 'tcp', internet: false, bytes_out: 2 * MB - 100_000, bytes_in: 100_000 },
    { address: '192.168.50.251', port: 22, served: true, proto: 'tcp', kind: 'ssh', internet: false, bytes_out: 30_000, bytes_in: 10_000 },
    { address: '192.168.50.1', port: 53, proto: 'udp', kind: 'dns', internet: false, bytes_out: 100_000, bytes_in: 900_000 },
    { address: '224.0.0.251', port: 5353, proto: 'udp', kind: 'discovery', internet: false, bytes_out: 50_000, bytes_in: 50_000 },
  ],
  other_flows: 19, other_bytes: 2 * MB,
}
const ISP = owner(5607, 'BSKYB-BROADBAND-AS', 'GB', 'London', 'GB', 51.5, -0.1)
const TRACE = {
  address: '104.16.1.1', state: 'done', started_at: new Date(Date.now() - 30_000).toISOString(), ended_at: new Date().toISOString(), reached: true,
  hops: [
    { ttl: 1, address: '192.168.50.1', sent: 3, answered: 3, best_ms: 4.4, where: 'this_network' },
    { ttl: 2, address: '192.168.0.1', sent: 3, answered: 3, best_ms: 6.1, where: 'this_network' },
    { ttl: 3, address: '90.212.129.56', sent: 3, answered: 3, best_ms: 8.5, where: 'internet', owner: ISP },
    { ttl: 4, sent: 3, answered: 0 },
    { ttl: 5, address: '2.120.11.152', sent: 3, answered: 2, best_ms: 17, where: 'internet', owner: ISP },
    { ttl: 6, address: '104.16.1.1', sent: 3, answered: 3, best_ms: 92, where: 'internet', reached: true, owner: CLOUDFLARE },
  ],
}
const TABLES = {
  '/netaudit/destination-data': { ranges: 579453, loaded_at: new Date().toISOString(), source: 'iptoasn.com' },
  '/netaudit/destination-places': { ranges: 7403297, loaded_at: new Date().toISOString(), source: 'db-ip.com' },
}

test('the traffic tab shows the last look as a sentence, a map and who it talked to, and fits the screen', async ({ page }, testInfo) => {
  await mockAdminApi(page, { answers: { '/admin/api/helper': READY, '/netaudit/host-traffic': { summary: LOOK }, '/netaudit/route-trace': { supported: true, traces: [] }, ...TABLES } })
  await page.goto('network/traffic')
  const tab = page.locator('[data-test="traffic-tab"]')
  await expect(tab.locator('[data-test="traffic-headline"]')).toContainText('sent 6 MB and received 38 MB')
  await expect(tab.locator('[data-test="traffic-self"]')).toContainText('own page and API')
  await expect(tab.locator('[data-test="map-dot"]')).toHaveCount(3)
  await expect(tab.locator('[data-test="talk-internet"] [data-test="talk-row"]')).toHaveCount(3)
  await expect(tab.locator('[data-test="talk-network"] [data-test="talk-name"]')).toHaveText(['192.168.50.1', '192.168.50.251'])
  await expect(tab.locator('[data-test="talk-shared"] [data-test="talk-row"]')).toBeHidden()
  await expect(tab.locator('[data-test="traffic-chart"]')).toHaveCount(0)
  await expect(tab.locator('[data-test="traffic-tables-line"]')).toHaveText('Operator and city names are on.')
  await expectNoSidewaysScroll(page)
  if (testInfo.project.name === 'mobile') {
    await expectTapArea(tab.locator('[data-test="traffic-listen"]'))
    await tab.locator('[data-test="talk-open"]').first().scrollIntoViewIfNeeded()
    await expectTapArea(tab.locator('[data-test="talk-open"]').first())
  }
  await lookAt(page, testInfo, 'traffic-result.png', true)
})

test('a place opened says who runs it, where it is used and registered, and shows its route on the map', async ({ page }, testInfo) => {
  await mockAdminApi(page, { answers: { '/admin/api/helper': READY, '/netaudit/host-traffic': { summary: LOOK }, '/netaudit/route-trace': { supported: true, traces: [TRACE] }, ...TABLES } })
  await page.goto('network/traffic')
  const tab = page.locator('[data-test="traffic-tab"]')
  await tab.locator('[data-test="talk-open"]', { hasText: 'registry.npmjs.org' }).click()
  await expect(tab.locator('[data-test="place-location"]')).toContainText('used in Toronto, Canada')
  await expect(tab.locator('[data-test="place-location"]')).toContainText('registered in United States')
  await expect(tab.locator('[data-test="route-hop"]')).toHaveCount(6)
  await expect(tab.locator('[data-test="route-hop-who"]').nth(3)).toHaveText('no answer')
  await expect(tab.locator('[data-test="route-outcome"]')).toHaveText('Reached in 6 steps.')
  await expect(tab.locator('[data-test="map-route"]')).toHaveCount(1)
  await expectNoSidewaysScroll(page)
  if (testInfo.project.name === 'mobile') {
    await tab.locator('[data-test="route-start"]').scrollIntoViewIfNeeded()
    await expectTapArea(tab.locator('[data-test="route-start"]'))
  }
  await lookAt(page, testInfo, 'traffic-route.png', true)
})

test('Advanced adds the totals, the chart, the operators and the conversations', async ({ page }, testInfo) => {
  await mockAdminApi(page, { answers: { '/admin/api/helper': READY, '/netaudit/host-traffic': { summary: LOOK }, '/netaudit/route-trace': { supported: true, traces: [] }, ...TABLES } })
  await setNetauditView(page, 'advanced')
  await page.goto('network/traffic')
  const tab = page.locator('[data-test="traffic-tab"]')
  await expect(tab.locator('[data-test="traffic-chart"] rect')).toHaveCount(60)
  await expect(tab.locator('[data-test="where-operators"] [data-test="where-row"]')).toHaveCount(3)
  await tab.getByRole('radio', { name: 'Countries' }).click()
  await expect(tab.locator('[data-test="where-countries"] [data-test="where-row"]')).toHaveCount(1)
  await expect(tab.locator('[data-test="flow"]')).toHaveCount(6)
  await expect(tab.locator('[data-test="traffic-notes"]')).toContainText('Estimated')
  await expectNoSidewaysScroll(page)
  await lookAt(page, testInfo, 'traffic-advanced.png', true)
})

test('without the tables the tab says operator and city names are off, and links to where they are turned on', async ({ page }) => {
  const plain = { ...LOOK, owners_used: false, owners: null, countries: null, destinations: LOOK.destinations.map(({ owner: _o, ...d }) => d), flows: LOOK.flows.map(({ owner: _o, ...f }) => f) }
  await mockAdminApi(page, { answers: { '/admin/api/helper': READY, '/netaudit/host-traffic': { summary: plain }, '/netaudit/route-trace': { supported: true, traces: [] }, '/netaudit/destination-data': { ranges: 0, source: 'iptoasn.com' }, '/netaudit/destination-places': { ranges: 0, source: 'db-ip.com' } } })
  await page.goto('network/traffic')
  const tab = page.locator('[data-test="traffic-tab"]')
  await expect(tab.locator('[data-test="map-empty"]')).toContainText('Download the table')
  await expect(tab.locator('[data-test="traffic-tables-line"]')).toContainText('are off')
  await tab.locator('[data-test="traffic-tables-link"]').click()
  await expect(page).toHaveURL(/network\/settings\?group=data/)
  await expect(page.locator('[data-test="destination-data"]')).toBeVisible()
  await expect(page.locator('[data-test="destination-places"]')).toBeVisible()
})

test('with the action switched off the tab says why and offers the fix, and keeps the last look readable', async ({ page }, testInfo) => {
  await mockAdminApi(page, { answers: { '/admin/api/helper': { ...READY, actions: [{ id: 'capture_host', on: false, installed: true }] }, '/netaudit/host-traffic': { summary: LOOK }, '/netaudit/route-trace': { supported: true, traces: [] } } })
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
