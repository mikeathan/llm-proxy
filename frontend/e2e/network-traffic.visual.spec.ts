import { expect, test, type Page } from '@playwright/test'
import { expectNoSidewaysScroll, expectTapArea, lookAt, mockAdminApi, setNetauditView } from './support/adminApi'

// Network → Traffic, by device: a list of devices (this computer, and each device of the router's table) and the picked device's page: a map, its
// places by company, and the route to any of them. With the helper ready and the action on it offers one button and shows the last look as a
// sentence (with this program's own page set apart); broadcast chatter is closed. The chart, totals and tables of operators and conversations are under Advanced. With
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
  await expect(tab.locator('[data-test="group-title"]')).toHaveText(['Google LLC', 'GitHub, Inc.', 'Cloudflare, Inc.'])
  await expect(tab.locator('[data-test="talk-network"] [data-test="talk-name"]')).toHaveText(['192.168.50.1', '192.168.50.251'])
  await expect(tab.locator('[data-test="talk-shared"] [data-test="talk-row"]')).toBeHidden()
  await expect(tab.locator('[data-test="traffic-chart"]')).toHaveCount(0)
  await expect(tab.locator('[data-test="traffic-tables-line"]')).toHaveText('Operator and city names are on.')
  await expectNoSidewaysScroll(page)
  if (testInfo.project.name === 'mobile') {
    await expectTapArea(tab.locator('[data-test="traffic-listen"]'))
    await tab.locator('[data-test="group-open"]').first().scrollIntoViewIfNeeded()
    await expectTapArea(tab.locator('[data-test="group-open"]').first())
  }
  await lookAt(page, testInfo, 'traffic-result.png', true)
})

test('a company opened lists its places, and a place\'s route opens under it and is drawn on the map', async ({ page }, testInfo) => {
  await mockAdminApi(page, { answers: { '/admin/api/helper': READY, '/netaudit/host-traffic': { summary: LOOK }, '/netaudit/route-trace': { supported: true, traces: [TRACE] }, ...TABLES } })
  await page.goto('network/traffic')
  const tab = page.locator('[data-test="traffic-tab"]')
  await tab.locator('[data-test="group-open"]', { hasText: 'Cloudflare' }).click()
  await expect(tab.locator('[data-test="place-where"]')).toContainText('registry.npmjs.org · Toronto, Canada')
  await tab.locator('[data-test="place-route"]').click()
  await expect(tab.locator('[data-test="place-route-panel"]')).toBeVisible()
  await expect(tab.locator('[data-test="route"] [data-test="route-hop"]')).toHaveCount(6)
  await expect(tab.locator('[data-test="route"] [data-test="route-hop-who"]').nth(3)).toHaveText('no answer')
  await expect(tab.locator('[data-test="route"] [data-test="route-outcome"]')).toHaveText('Reached in 6 steps.')
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
  await expect(page.locator('[data-test="traffic-never"]')).toContainText('No devices yet')
  await expect(page.locator('[data-test="traffic-listen"]')).toBeVisible()
  await expectNoSidewaysScroll(page)
})

// "Trace a route" works for any internet address, with no look needed: the steps fold the runs of silent routers, each router that answered says
// how steady it was, and the route is drawn on the map.
const ZAYO_NJ = owner(6461, 'ZAYO-6461 - Zayo Bandwidth', 'US', 'Newark', 'US', 40.7, -74.2)
const ZAYO_IN = owner(6461, 'ZAYO-6461 - Zayo Bandwidth', 'US', 'Columbia City', 'US', 41.2, -85.5)
const CANONICAL = owner(41231, 'CANONICAL-AS Canonical Group Limited', 'GB', 'London', 'GB', 51.5, -0.1)
const LONG_TRACE = {
  address: '91.189.91.61', state: 'done', started_at: new Date(Date.now() - 40_000).toISOString(), ended_at: new Date().toISOString(), reached: true, repeat: true, rounds: 10, rounds_done: 10,
  hops: [
    { ttl: 1, address: '192.168.50.1', sent: 3, answered: 3, best_ms: 1.6, where: 'this_network', stats: { sent: 10, answered: 10, best_ms: 1.4, avg_ms: 1.9, worst_ms: 3.2 } },
    { ttl: 2, address: '192.168.0.1', sent: 3, answered: 3, best_ms: 2.3, where: 'this_network', stats: { sent: 10, answered: 10, best_ms: 2.1, avg_ms: 2.6, worst_ms: 4.8 } },
    { ttl: 3, address: '90.212.129.56', sent: 3, answered: 3, best_ms: 2.4, where: 'internet', owner: ISP, stats: { sent: 10, answered: 9, best_ms: 2.2, avg_ms: 2.7, worst_ms: 6 } },
    { ttl: 4, sent: 3, answered: 0 }, { ttl: 5, sent: 3, answered: 0 },
    { ttl: 6, address: '64.125.29.128', sent: 3, answered: 3, best_ms: 73, where: 'internet', owner: ZAYO_NJ, stats: { sent: 10, answered: 10, best_ms: 72, avg_ms: 73, worst_ms: 76 } },
    { ttl: 7, address: '128.177.44.233', sent: 3, answered: 2, best_ms: 73, where: 'internet', owner: ZAYO_IN, stats: { sent: 10, answered: 8, best_ms: 72, avg_ms: 74, worst_ms: 90 } },
    { ttl: 8, sent: 3, answered: 0 }, { ttl: 9, sent: 3, answered: 0 }, { ttl: 10, sent: 3, answered: 0 },
    { ttl: 11, address: '91.189.91.61', sent: 3, answered: 3, best_ms: 97, where: 'internet', reached: true, owner: CANONICAL, stats: { sent: 10, answered: 10, best_ms: 96, avg_ms: 98, worst_ms: 104 } },
  ],
}
const LOOKUPS = {
  enabled: true, since: new Date(Date.now() - 6 * 86_400_000).toISOString(),
  devices: [
    { mac: 'aa:bb:cc:00:00:01', first: new Date(Date.now() - 4 * 86_400_000).toISOString(), last: new Date(Date.now() - 60_000).toISOString(), today: 41, total: 188, flags: ['new_place'], names: [
      { name: 'telemetry.vendor-cdn.net', domain: 'vendor-cdn.net', last: new Date(Date.now() - 60_000).toISOString(), count: 3, flag: 'new_place' },
      { name: 'api.smartthings.com', domain: 'smartthings.com', last: new Date(Date.now() - 120_000).toISOString(), count: 30 },
      { name: 'fw-update.smartthings.com', domain: 'smartthings.com', last: new Date(Date.now() - 3_600_000).toISOString(), count: 2 },
    ] },
    { mac: 'aa:bb:cc:00:00:02', first: new Date(Date.now() - 4 * 86_400_000).toISOString(), last: new Date(Date.now() - 600_000).toISOString(), today: 12, total: 60, flags: [], names: [
      { name: 'time.apple.com', domain: 'apple.com', last: new Date(Date.now() - 600_000).toISOString(), count: 12 },
    ] },
  ],
}

test('the trace card traces a typed address, folds the silent steps, shows the steadiness and draws the route', async ({ page }, testInfo) => {
  await mockAdminApi(page, { answers: { '/admin/api/helper': READY, '/netaudit/host-traffic': { summary: null }, '/netaudit/route-trace': { supported: true, traces: [LONG_TRACE] }, '/netaudit/device-lookups': LOOKUPS, ...TABLES } })
  await page.goto('network/traffic')
  const card = page.locator('[data-test="trace-card"]')
  await expect(card.locator('[data-test="trace-result"]')).toHaveCount(0)
  await card.locator('[data-test="trace-address"]').fill('91.189.91.61')
  if (testInfo.project.name === 'mobile') {
    await card.locator('[data-test="trace-go"]').evaluate((el) => el.scrollIntoView({ block: 'center' }))
    await expectTapArea(card.locator('[data-test="trace-go"]'))
    await card.locator('[data-test="trace-preset"]').first().evaluate((el) => el.scrollIntoView({ block: 'center' }))
    await expectTapArea(card.locator('[data-test="trace-preset"]').first())
  }
  await card.locator('[data-test="trace-go"]').click()
  await expect(card.locator('[data-test="route-summary"]')).toContainText('11 steps, 6 answered')
  await expect(card.locator('[data-test="route-summary"]')).toContainText('United Kingdom, United States')
  await expect(card.locator('[data-test="route-silent-run"]')).toHaveCount(2)
  await expect(card.locator('[data-test="route-hop-stats"]').first()).toContainText('Answered 10/10')
  await expect(card.locator('[data-test="map-route-point"]')).toHaveCount(4)
  await expectNoSidewaysScroll(page)
  await lookAt(page, testInfo, 'traffic-trace-card.png', true)
})

// Choosing a device: on a wide screen it is in the list; on a phone the list opens from one button.
async function pickDevice(page: Page, name: string) {
  const toggle = page.locator('[data-test="picker-toggle"]')
  if (await toggle.isVisible()) await toggle.click()
  await page.locator('[data-test="rail-device"]', { hasText: name }).click()
}

test('a device that only asked for names is in the list, and shows what it looked up, grouped by domain', async ({ page }, testInfo) => {
  await mockAdminApi(page, { answers: { '/admin/api/helper': READY, '/netaudit/host-traffic': { summary: null }, '/netaudit/route-trace': { supported: true, traces: [] }, '/netaudit/device-lookups': LOOKUPS, '/netaudit/devices': { devices: [{ mac: 'aa:bb:cc:00:00:01', ip: '192.168.50.31', name: 'Living room hub', first_seen: new Date().toISOString(), last_seen: new Date().toISOString(), approved: true, randomized: false, new: false }] } } })
  await page.goto('network/traffic')
  const tab = page.locator('[data-test="traffic-tab"]')
  await expect(tab.locator('[data-test="device-name"]')).toContainText('Living room hub')
  await expect(tab.locator('[data-test="device-empty"]')).toContainText('has not been read yet')
  await expect(tab.locator('[data-test="names-group"]')).toHaveCount(2)
  await expect(tab.locator('[data-test="names-group"]').nth(1)).toContainText('api.smartthings.com, fw-update.smartthings.com')
  if (testInfo.project.name === 'mobile') {
    await tab.locator('[data-test="picker-toggle"]').evaluate((el) => el.scrollIntoView({ block: 'center' }))
    await expectTapArea(tab.locator('[data-test="picker-toggle"]'))
  }
  await pickDevice(page, 'Unknown device')
  await expect(tab.locator('[data-test="names-group"]')).toHaveText([/apple\.com/])
  await expectNoSidewaysScroll(page)
  await lookAt(page, testInfo, 'traffic-lookups.png', true)
})

// The router's connection table: after the setup (three steps) a press reads it, and each device opens to the internet places it connected to,
// with who runs them, and a place can be sent to the route trace card.
const ROUTER_READ = {
  address: '192.168.50.1', user: 'admin', key_saved: true, watching: true, fingerprint: 'SHA256:3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855',
  last: {
    at: new Date(Date.now() - 20_000).toISOString(), lines: 245, kept: 31,
    devices: [
      { address: '192.168.50.220', total: 18, places: [
        { address: '52.48.210.116', port: 8883, proto: 'tcp', state: 'ESTABLISHED', count: 17, owner: owner(16509, 'AMAZON-02', 'US', 'Dublin', 'IE', 53.3, -6.3) },
        { address: '8.8.8.8', port: 53, proto: 'udp', count: 1, owner: GOOGLE, new: true },
      ] },
      { address: '192.168.50.31', total: 13, places: [{ address: '140.82.1.1', port: 443, proto: 'tcp', state: 'ESTABLISHED', count: 13, owner: GITHUB }] },
    ],
  },
}

test('the router\'s devices are in the list, with a map and company groups for each, and a route opens under a place', async ({ page }, testInfo) => {
  const inventory = { devices: [{ mac: 'aa:bb:cc:00:00:03', ip: '192.168.50.220', name: 'Doorbell', first_seen: new Date().toISOString(), last_seen: new Date().toISOString(), approved: true, randomized: false, new: false }] }
  await mockAdminApi(page, { answers: { '/admin/api/helper': READY, '/netaudit/host-traffic': { summary: null }, '/netaudit/route-trace': { supported: true, traces: [] }, '/netaudit/device-lookups': LOOKUPS, '/netaudit/router-connections': ROUTER_READ, '/netaudit/devices': inventory } })
  await page.goto('network/traffic')
  const tab = page.locator('[data-test="traffic-tab"]')
  await expect(tab.locator('[data-test="router-when"]')).toContainText('245 lines, 31 connections')
  await expect(tab.locator('[data-test="device-name"]')).toContainText('Doorbell')
  await expect(tab.locator('[data-test="device-source"]')).toHaveText('The router\u2019s table')
  await expect(tab.locator('[data-test="map-dot"]')).toHaveCount(2)
  await expect(tab.locator('[data-test="group-title"]')).toHaveText(['AMAZON-02', 'Google LLC'])
  await expect(tab.locator('[data-test="router-watch-on"]')).toBeVisible()
  await expect(tab.locator('[data-test="stat-fresh"]')).toHaveText('1')
  await expect(tab.locator('[data-test="group-new"]')).toHaveCount(1)
  await tab.getByRole('radio', { name: 'New today 1' }).click()
  await expect(tab.locator('[data-test="group-title"]')).toHaveText(['Google LLC'])
  await tab.getByRole('radio', { name: 'All 2' }).click()
  await tab.locator('[data-test="group-open"]').first().click()
  await tab.locator('[data-test="place-route"]').first().click()
  await expect(tab.locator('[data-test="place-route-from"]')).toContainText('traced from this computer')
  if (testInfo.project.name === 'mobile') {
    await tab.locator('[data-test="place-route"]').first().evaluate((el) => el.scrollIntoView({ block: 'center' }))
    await expectTapArea(tab.locator('[data-test="place-route"]').first())
  }
  await pickDevice(page, 'Device 192.168.50.31')
  await expect(tab.locator('[data-test="group-title"]')).toHaveText(['GitHub, Inc.'])
  await expectNoSidewaysScroll(page)
  await lookAt(page, testInfo, 'traffic-router-conn.png', true)
})

test('before the router is set up it walks the three steps and never shows a key', async ({ page }, testInfo) => {
  await mockAdminApi(page, { answers: { '/admin/api/helper': READY, '/netaudit/host-traffic': { summary: null }, '/netaudit/route-trace': { supported: true, traces: [] }, '/netaudit/device-lookups': LOOKUPS, '/netaudit/router-connections': { address: '192.168.50.1', user: 'admin', key_saved: false } } })
  await page.goto('network/traffic')
  await expect(page.locator('[data-test="router-step"]')).toContainText('Step 2 of 3')
  await expect(page.locator('[data-test="router-key"]')).toBeVisible()
  await expectNoSidewaysScroll(page)
  await lookAt(page, testInfo, 'traffic-router-setup.png', true)
})
